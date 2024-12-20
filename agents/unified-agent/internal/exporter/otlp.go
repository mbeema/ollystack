// Package exporter handles telemetry export to backends
package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ollystack/unified-agent/internal/types"
)

// Config configures the OTLP exporter
type Config struct {
	Endpoint     string
	UseGRPC      bool
	APIKey       string
	BearerToken  string
	TLS          TLSConfig
	BatchSize    int
	BatchTimeout time.Duration
	RetryConfig  RetryConfig
	BufferConfig BufferConfig
}

// TLSConfig configures TLS settings
type TLSConfig struct {
	Enabled    bool
	CertFile   string
	KeyFile    string
	CAFile     string
	SkipVerify bool
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
}

// BufferConfig configures disk buffering
type BufferConfig struct {
	Enabled bool
	Path    string
	MaxSize int64
}

// OTLPExporter exports telemetry via OTLP
type OTLPExporter struct {
	config Config
	logger *zap.Logger

	// gRPC connection
	grpcConn *grpc.ClientConn

	// HTTP client
	httpClient *http.Client

	// Batching
	metricsBatch []types.Metric
	logsBatch    []types.LogRecord
	spansBatch   []types.Span
	batchMu      sync.Mutex
	batchTimer   *time.Timer

	// Stats
	stats ExporterStats

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ExporterStats tracks export statistics
type ExporterStats struct {
	PointsExported int64
	BytesSent      int64
	LastExportTime time.Time
	Errors         int64
}

// NewOTLPExporter creates a new OTLP exporter
func NewOTLPExporter(cfg Config, logger *zap.Logger) (*OTLPExporter, error) {
	ctx, cancel := context.WithCancel(context.Background())

	e := &OTLPExporter{
		config:       cfg,
		logger:       logger,
		metricsBatch: make([]types.Metric, 0, cfg.BatchSize),
		logsBatch:    make([]types.LogRecord, 0, cfg.BatchSize),
		spansBatch:   make([]types.Span, 0, cfg.BatchSize),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize HTTP client
	e.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	return e, nil
}

// Start initializes the exporter
func (e *OTLPExporter) Start(ctx context.Context) error {
	if e.config.UseGRPC {
		if err := e.connectGRPC(); err != nil {
			return fmt.Errorf("gRPC connection failed: %w", err)
		}
	}

	// Start batch timer
	e.startBatchTimer()

	<-ctx.Done()
	e.shutdown()
	return nil
}

func (e *OTLPExporter) connectGRPC() error {
	var opts []grpc.DialOption

	if e.config.TLS.Enabled {
		creds, err := credentials.NewClientTLSFromFile(e.config.TLS.CAFile, "")
		if err != nil {
			return err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(e.config.Endpoint, opts...)
	if err != nil {
		return err
	}

	e.grpcConn = conn
	e.logger.Info("Connected to OTLP endpoint via gRPC", zap.String("endpoint", e.config.Endpoint))
	return nil
}

// ExportMetric adds a metric to the export batch
func (e *OTLPExporter) ExportMetric(m types.Metric) {
	e.batchMu.Lock()
	e.metricsBatch = append(e.metricsBatch, m)

	if len(e.metricsBatch) >= e.config.BatchSize {
		e.batchMu.Unlock()
		e.flushMetrics()
	} else {
		e.batchMu.Unlock()
	}
}

// ExportLog adds a log to the export batch
func (e *OTLPExporter) ExportLog(log types.LogRecord) {
	e.batchMu.Lock()
	e.logsBatch = append(e.logsBatch, log)

	if len(e.logsBatch) >= e.config.BatchSize {
		e.batchMu.Unlock()
		e.flushLogs()
	} else {
		e.batchMu.Unlock()
	}
}

// ExportSpan adds a span to the export batch
func (e *OTLPExporter) ExportSpan(span types.Span) {
	e.batchMu.Lock()
	e.spansBatch = append(e.spansBatch, span)

	if len(e.spansBatch) >= e.config.BatchSize {
		e.batchMu.Unlock()
		e.flushSpans()
	} else {
		e.batchMu.Unlock()
	}
}

func (e *OTLPExporter) startBatchTimer() {
	e.batchTimer = time.AfterFunc(e.config.BatchTimeout, func() {
		e.flushAll()
		e.startBatchTimer() // Restart timer
	})
}

func (e *OTLPExporter) flushAll() {
	e.flushMetrics()
	e.flushLogs()
	e.flushSpans()
}

func (e *OTLPExporter) flushMetrics() {
	e.batchMu.Lock()
	if len(e.metricsBatch) == 0 {
		e.batchMu.Unlock()
		return
	}

	batch := e.metricsBatch
	e.metricsBatch = make([]types.Metric, 0, e.config.BatchSize)
	e.batchMu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.sendMetrics(batch)
	}()
}

func (e *OTLPExporter) flushLogs() {
	e.batchMu.Lock()
	if len(e.logsBatch) == 0 {
		e.batchMu.Unlock()
		return
	}

	batch := e.logsBatch
	e.logsBatch = make([]types.LogRecord, 0, e.config.BatchSize)
	e.batchMu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.sendLogs(batch)
	}()
}

func (e *OTLPExporter) flushSpans() {
	e.batchMu.Lock()
	if len(e.spansBatch) == 0 {
		e.batchMu.Unlock()
		return
	}

	batch := e.spansBatch
	e.spansBatch = make([]types.Span, 0, e.config.BatchSize)
	e.batchMu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.sendSpans(batch)
	}()
}

func (e *OTLPExporter) sendMetrics(metrics []types.Metric) {
	// Convert to OTLP format
	payload := e.metricsToOTLP(metrics)

	if err := e.send("/v1/metrics", payload); err != nil {
		e.logger.Error("Failed to send metrics", zap.Error(err), zap.Int("count", len(metrics)))
		atomic.AddInt64(&e.stats.Errors, 1)
		// TODO: Implement retry and disk buffering
		return
	}

	atomic.AddInt64(&e.stats.PointsExported, int64(len(metrics)))
	e.stats.LastExportTime = time.Now()
}

func (e *OTLPExporter) sendLogs(logs []types.LogRecord) {
	payload := e.logsToOTLP(logs)

	if err := e.send("/v1/logs", payload); err != nil {
		e.logger.Error("Failed to send logs", zap.Error(err), zap.Int("count", len(logs)))
		atomic.AddInt64(&e.stats.Errors, 1)
		return
	}

	atomic.AddInt64(&e.stats.PointsExported, int64(len(logs)))
	e.stats.LastExportTime = time.Now()
}

func (e *OTLPExporter) sendSpans(spans []types.Span) {
	payload := e.spansToOTLP(spans)

	if err := e.send("/v1/traces", payload); err != nil {
		e.logger.Error("Failed to send spans", zap.Error(err), zap.Int("count", len(spans)))
		atomic.AddInt64(&e.stats.Errors, 1)
		return
	}

	atomic.AddInt64(&e.stats.PointsExported, int64(len(spans)))
	e.stats.LastExportTime = time.Now()
}

func (e *OTLPExporter) send(path string, payload []byte) error {
	url := fmt.Sprintf("http://%s%s", e.config.Endpoint, path)
	if e.config.TLS.Enabled {
		url = fmt.Sprintf("https://%s%s", e.config.Endpoint, path)
	}

	req, err := http.NewRequestWithContext(e.ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if e.config.APIKey != "" {
		req.Header.Set("X-API-Key", e.config.APIKey)
	}
	if e.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.BearerToken)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	atomic.AddInt64(&e.stats.BytesSent, int64(len(payload)))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

// OTLP format conversion (simplified JSON format)

func (e *OTLPExporter) metricsToOTLP(metrics []types.Metric) []byte {
	// Simplified OTLP JSON format
	type otlpMetric struct {
		Name       string            `json:"name"`
		Value      float64           `json:"value"`
		Timestamp  int64             `json:"timestamp"`
		Attributes map[string]string `json:"attributes"`
		Type       string            `json:"type"`
		Unit       string            `json:"unit,omitempty"`
	}

	type payload struct {
		ResourceMetrics []struct {
			ScopeMetrics []struct {
				Metrics []otlpMetric `json:"metrics"`
			} `json:"scope_metrics"`
		} `json:"resource_metrics"`
	}

	otlpMetrics := make([]otlpMetric, len(metrics))
	for i, m := range metrics {
		otlpMetrics[i] = otlpMetric{
			Name:       m.Name,
			Value:      m.Value,
			Timestamp:  m.Timestamp.UnixNano(),
			Attributes: m.Labels,
			Type:       metricTypeString(m.Type),
			Unit:       m.Unit,
		}
	}

	p := payload{
		ResourceMetrics: []struct {
			ScopeMetrics []struct {
				Metrics []otlpMetric `json:"metrics"`
			} `json:"scope_metrics"`
		}{
			{
				ScopeMetrics: []struct {
					Metrics []otlpMetric `json:"metrics"`
				}{
					{Metrics: otlpMetrics},
				},
			},
		},
	}

	data, _ := json.Marshal(p)
	return data
}

func (e *OTLPExporter) logsToOTLP(logs []types.LogRecord) []byte {
	type otlpLog struct {
		Timestamp         int64             `json:"timestamp"`
		Body              string            `json:"body"`
		SeverityText      string            `json:"severity_text"`
		SeverityNumber    int               `json:"severity_number"`
		Attributes        map[string]string `json:"attributes"`
		TraceID           string            `json:"trace_id,omitempty"`
		SpanID            string            `json:"span_id,omitempty"`
	}

	type payload struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []otlpLog `json:"log_records"`
			} `json:"scope_logs"`
		} `json:"resource_logs"`
	}

	otlpLogs := make([]otlpLog, len(logs))
	for i, l := range logs {
		otlpLogs[i] = otlpLog{
			Timestamp:      l.Timestamp.UnixNano(),
			Body:           l.Body,
			SeverityText:   l.Severity.String(),
			SeverityNumber: int(l.Severity),
			Attributes:     l.Attributes,
			TraceID:        l.TraceID,
			SpanID:         l.SpanID,
		}
	}

	p := payload{
		ResourceLogs: []struct {
			ScopeLogs []struct {
				LogRecords []otlpLog `json:"log_records"`
			} `json:"scope_logs"`
		}{
			{
				ScopeLogs: []struct {
					LogRecords []otlpLog `json:"log_records"`
				}{
					{LogRecords: otlpLogs},
				},
			},
		},
	}

	data, _ := json.Marshal(p)
	return data
}

func (e *OTLPExporter) spansToOTLP(spans []types.Span) []byte {
	type otlpSpan struct {
		TraceID           string            `json:"trace_id"`
		SpanID            string            `json:"span_id"`
		ParentSpanID      string            `json:"parent_span_id,omitempty"`
		Name              string            `json:"name"`
		Kind              int               `json:"kind"`
		StartTimeUnixNano int64             `json:"start_time_unix_nano"`
		EndTimeUnixNano   int64             `json:"end_time_unix_nano"`
		Attributes        map[string]string `json:"attributes"`
		Status            struct {
			Code    int    `json:"code"`
			Message string `json:"message,omitempty"`
		} `json:"status"`
	}

	type payload struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []otlpSpan `json:"spans"`
			} `json:"scope_spans"`
		} `json:"resource_spans"`
	}

	otlpSpans := make([]otlpSpan, len(spans))
	for i, s := range spans {
		otlpSpans[i] = otlpSpan{
			TraceID:           s.TraceID,
			SpanID:            s.SpanID,
			ParentSpanID:      s.ParentSpanID,
			Name:              s.Name,
			Kind:              int(s.Kind),
			StartTimeUnixNano: s.StartTime.UnixNano(),
			EndTimeUnixNano:   s.EndTime.UnixNano(),
			Attributes:        s.Attributes,
			Status: struct {
				Code    int    `json:"code"`
				Message string `json:"message,omitempty"`
			}{
				Code:    int(s.Status),
				Message: s.StatusMessage,
			},
		}
	}

	p := payload{
		ResourceSpans: []struct {
			ScopeSpans []struct {
				Spans []otlpSpan `json:"spans"`
			} `json:"scope_spans"`
		}{
			{
				ScopeSpans: []struct {
					Spans []otlpSpan `json:"spans"`
				}{
					{Spans: otlpSpans},
				},
			},
		},
	}

	data, _ := json.Marshal(p)
	return data
}

func metricTypeString(t types.MetricType) string {
	switch t {
	case types.MetricTypeGauge:
		return "gauge"
	case types.MetricTypeCounter:
		return "counter"
	case types.MetricTypeHistogram:
		return "histogram"
	case types.MetricTypeSummary:
		return "summary"
	default:
		return "gauge"
	}
}

func (e *OTLPExporter) shutdown() {
	e.cancel()

	// Stop batch timer
	if e.batchTimer != nil {
		e.batchTimer.Stop()
	}

	// Flush remaining data
	e.flushAll()

	// Wait for pending exports
	e.wg.Wait()

	// Close gRPC connection
	if e.grpcConn != nil {
		e.grpcConn.Close()
	}
}

// Stats returns export statistics
func (e *OTLPExporter) Stats() ExporterStats {
	return ExporterStats{
		PointsExported: atomic.LoadInt64(&e.stats.PointsExported),
		BytesSent:      atomic.LoadInt64(&e.stats.BytesSent),
		LastExportTime: e.stats.LastExportTime,
		Errors:         atomic.LoadInt64(&e.stats.Errors),
	}
}
