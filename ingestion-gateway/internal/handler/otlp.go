package handler

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/ollystack/ingestion-gateway/internal/clickhouse"
	"github.com/ollystack/ingestion-gateway/internal/config"
	"github.com/ollystack/ingestion-gateway/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	requestsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_ingestion_requests_total",
			Help: "Total number of ingestion requests received",
		},
		[]string{"type", "protocol", "status"},
	)
	bytesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_ingestion_bytes_total",
			Help: "Total bytes received",
		},
		[]string{"type", "protocol"},
	)
	requestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_ingestion_latency_seconds",
			Help:    "Request processing latency",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"type", "protocol"},
	)
	dataPointsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_ingestion_data_points_total",
			Help: "Total data points received",
		},
		[]string{"type", "tenant"},
	)
	dataPointsSampled = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_ingestion_data_points_sampled_total",
			Help: "Total data points after sampling",
		},
		[]string{"type", "tenant"},
	)
)

// OTLPHandler handles OTLP gRPC and HTTP requests
type OTLPHandler struct {
	writer      *clickhouse.Writer
	rateLimiter *middleware.RateLimiter
	config      *config.Config
	logger      *zap.Logger

	// Implement OTLP gRPC interfaces
	metricsv1.UnimplementedMetricsServiceServer
	logsv1.UnimplementedLogsServiceServer
	tracev1.UnimplementedTraceServiceServer
}

// NewOTLPHandler creates a new OTLP handler
func NewOTLPHandler(
	writer *clickhouse.Writer,
	rateLimiter *middleware.RateLimiter,
	cfg *config.Config,
	logger *zap.Logger,
) *OTLPHandler {
	return &OTLPHandler{
		writer:      writer,
		rateLimiter: rateLimiter,
		config:      cfg,
		logger:      logger,
	}
}

// RegisterGRPC registers the handler with a gRPC server
func (h *OTLPHandler) RegisterGRPC(server *grpc.Server) {
	metricsv1.RegisterMetricsServiceServer(server, h)
	logsv1.RegisterLogsServiceServer(server, h)
	tracev1.RegisterTraceServiceServer(server, h)
}

// ========== gRPC Handlers ==========

// Export implements the OTLP metrics gRPC service
func (h *OTLPHandler) Export(ctx context.Context, req *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	start := time.Now()
	tenantID := h.extractTenantID(ctx)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("metrics", "grpc", "rate_limited").Inc()
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Convert and write metrics
	var dataPoints int
	for _, rm := range req.ResourceMetrics {
		resourceAttrs := extractAttributes(rm.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]
		host := resourceAttrs["host.name"]
		environment := resourceAttrs["deployment.environment"]

		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				dataPoints++
				rows := h.convertMetric(tenantID, serviceName, host, environment, metric)
				for _, row := range rows {
					h.writer.WriteMetric(row)
				}
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("metrics", "grpc", "success").Inc()
	requestLatency.WithLabelValues("metrics", "grpc").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("metrics", tenantID).Add(float64(dataPoints))

	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

// ExportLogs implements the OTLP logs gRPC service
func (h *OTLPHandler) ExportLogs(ctx context.Context, req *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	start := time.Now()
	tenantID := h.extractTenantID(ctx)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("logs", "grpc", "rate_limited").Inc()
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Convert and write logs
	var logRecords int
	for _, rl := range req.ResourceLogs {
		resourceAttrs := extractAttributes(rl.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]
		host := resourceAttrs["host.name"]

		for _, sl := range rl.ScopeLogs {
			for _, log := range sl.LogRecords {
				logRecords++
				row := h.convertLog(tenantID, serviceName, host, log)
				h.writer.WriteLog(row)
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("logs", "grpc", "success").Inc()
	requestLatency.WithLabelValues("logs", "grpc").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("logs", tenantID).Add(float64(logRecords))

	return &logsv1.ExportLogsServiceResponse{}, nil
}

// ExportTraces implements the OTLP traces gRPC service
func (h *OTLPHandler) ExportTraces(ctx context.Context, req *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	start := time.Now()
	tenantID := h.extractTenantID(ctx)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("traces", "grpc", "rate_limited").Inc()
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Convert and write traces
	var spans int
	for _, rt := range req.ResourceSpans {
		resourceAttrs := extractAttributes(rt.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]

		for _, ss := range rt.ScopeSpans {
			for _, span := range ss.Spans {
				spans++
				row := h.convertSpan(tenantID, serviceName, span)
				h.writer.WriteTrace(row)
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("traces", "grpc", "success").Inc()
	requestLatency.WithLabelValues("traces", "grpc").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("traces", tenantID).Add(float64(spans))

	return &tracev1.ExportTraceServiceResponse{}, nil
}

// ========== HTTP Handlers ==========

// HandleMetricsHTTP handles OTLP metrics over HTTP
func (h *OTLPHandler) HandleMetricsHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	tenantID := h.extractTenantIDFromHTTP(r)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("metrics", "http", "rate_limited").Inc()
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		requestsReceived.WithLabelValues("metrics", "http", "error").Inc()
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse protobuf
	var req metricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		requestsReceived.WithLabelValues("metrics", "http", "error").Inc()
		http.Error(w, "Invalid protobuf", http.StatusBadRequest)
		return
	}

	// Convert and write metrics
	var dataPoints int
	for _, rm := range req.ResourceMetrics {
		resourceAttrs := extractAttributes(rm.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]
		host := resourceAttrs["host.name"]
		environment := resourceAttrs["deployment.environment"]

		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				dataPoints++
				rows := h.convertMetric(tenantID, serviceName, host, environment, metric)
				for _, row := range rows {
					h.writer.WriteMetric(row)
				}
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("metrics", "http", "success").Inc()
	bytesReceived.WithLabelValues("metrics", "http").Add(float64(len(body)))
	requestLatency.WithLabelValues("metrics", "http").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("metrics", tenantID).Add(float64(dataPoints))

	w.WriteHeader(http.StatusOK)
}

// HandleLogsHTTP handles OTLP logs over HTTP
func (h *OTLPHandler) HandleLogsHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	tenantID := h.extractTenantIDFromHTTP(r)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("logs", "http", "rate_limited").Inc()
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		requestsReceived.WithLabelValues("logs", "http", "error").Inc()
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse protobuf
	var req logsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		requestsReceived.WithLabelValues("logs", "http", "error").Inc()
		http.Error(w, "Invalid protobuf", http.StatusBadRequest)
		return
	}

	// Convert and write logs
	var logRecords int
	for _, rl := range req.ResourceLogs {
		resourceAttrs := extractAttributes(rl.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]
		host := resourceAttrs["host.name"]

		for _, sl := range rl.ScopeLogs {
			for _, log := range sl.LogRecords {
				logRecords++
				row := h.convertLog(tenantID, serviceName, host, log)
				h.writer.WriteLog(row)
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("logs", "http", "success").Inc()
	bytesReceived.WithLabelValues("logs", "http").Add(float64(len(body)))
	requestLatency.WithLabelValues("logs", "http").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("logs", tenantID).Add(float64(logRecords))

	w.WriteHeader(http.StatusOK)
}

// HandleTracesHTTP handles OTLP traces over HTTP
func (h *OTLPHandler) HandleTracesHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	tenantID := h.extractTenantIDFromHTTP(r)

	// Rate limit check
	if !h.rateLimiter.Allow(tenantID) {
		requestsReceived.WithLabelValues("traces", "http", "rate_limited").Inc()
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		requestsReceived.WithLabelValues("traces", "http", "error").Inc()
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse protobuf
	var req tracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		requestsReceived.WithLabelValues("traces", "http", "error").Inc()
		http.Error(w, "Invalid protobuf", http.StatusBadRequest)
		return
	}

	// Convert and write traces
	var spans int
	for _, rt := range req.ResourceSpans {
		resourceAttrs := extractAttributes(rt.Resource.GetAttributes())
		serviceName := resourceAttrs["service.name"]

		for _, ss := range rt.ScopeSpans {
			for _, span := range ss.Spans {
				spans++
				row := h.convertSpan(tenantID, serviceName, span)
				h.writer.WriteTrace(row)
			}
		}
	}

	// Metrics
	requestsReceived.WithLabelValues("traces", "http", "success").Inc()
	bytesReceived.WithLabelValues("traces", "http").Add(float64(len(body)))
	requestLatency.WithLabelValues("traces", "http").Observe(time.Since(start).Seconds())
	dataPointsReceived.WithLabelValues("traces", tenantID).Add(float64(spans))

	w.WriteHeader(http.StatusOK)
}

// ========== Converters ==========

func (h *OTLPHandler) convertMetric(tenantID, serviceName, host, environment string, metric *metricspb.Metric) []clickhouse.MetricRow {
	var rows []clickhouse.MetricRow

	// Determine metric type and extract data points
	switch data := metric.Data.(type) {
	case *metricspb.Metric_Gauge:
		for _, dp := range data.Gauge.DataPoints {
			rows = append(rows, clickhouse.MetricRow{
				TenantID:    tenantID,
				Timestamp:   time.Unix(0, int64(dp.TimeUnixNano)),
				MetricName:  metric.Name,
				MetricType:  "gauge",
				Value:       getNumberValue(dp),
				Labels:      extractAttributes(dp.Attributes),
				ServiceName: serviceName,
				Host:        host,
				Environment: environment,
				SampleRate:  1.0,
			})
		}
	case *metricspb.Metric_Sum:
		for _, dp := range data.Sum.DataPoints {
			rows = append(rows, clickhouse.MetricRow{
				TenantID:    tenantID,
				Timestamp:   time.Unix(0, int64(dp.TimeUnixNano)),
				MetricName:  metric.Name,
				MetricType:  "counter",
				Value:       getNumberValue(dp),
				Labels:      extractAttributes(dp.Attributes),
				ServiceName: serviceName,
				Host:        host,
				Environment: environment,
				SampleRate:  1.0,
			})
		}
	case *metricspb.Metric_Histogram:
		for _, dp := range data.Histogram.DataPoints {
			// For histograms, we store the sum and count
			labels := extractAttributes(dp.Attributes)
			labels["le"] = "+Inf"
			rows = append(rows, clickhouse.MetricRow{
				TenantID:    tenantID,
				Timestamp:   time.Unix(0, int64(dp.TimeUnixNano)),
				MetricName:  metric.Name + "_sum",
				MetricType:  "histogram",
				Value:       dp.GetSum(),
				Labels:      labels,
				ServiceName: serviceName,
				Host:        host,
				Environment: environment,
				SampleRate:  1.0,
			})
			rows = append(rows, clickhouse.MetricRow{
				TenantID:    tenantID,
				Timestamp:   time.Unix(0, int64(dp.TimeUnixNano)),
				MetricName:  metric.Name + "_count",
				MetricType:  "histogram",
				Value:       float64(dp.GetCount()),
				Labels:      labels,
				ServiceName: serviceName,
				Host:        host,
				Environment: environment,
				SampleRate:  1.0,
			})
		}
	}

	return rows
}

func (h *OTLPHandler) convertLog(tenantID, serviceName, host string, log *logspb.LogRecord) clickhouse.LogRow {
	return clickhouse.LogRow{
		TenantID:        tenantID,
		Timestamp:       time.Unix(0, int64(log.TimeUnixNano)),
		TraceID:         hex.EncodeToString(log.TraceId),
		SpanID:          hex.EncodeToString(log.SpanId),
		Severity:        log.SeverityText,
		SeverityNumber:  uint8(log.SeverityNumber),
		Body:            log.Body.GetStringValue(),
		Attributes:      extractAttributes(log.Attributes),
		ServiceName:     serviceName,
		Host:            host,
		PatternHash:     "", // Computed by ClickHouse MV
		OccurrenceCount: 1,
		SampleRate:      1.0,
	}
}

func (h *OTLPHandler) convertSpan(tenantID, serviceName string, span *tracepb.Span) clickhouse.TraceRow {
	attrs := extractAttributes(span.Attributes)

	// Extract common span attributes
	httpMethod := attrs["http.method"]
	httpUrl := attrs["http.url"]
	httpStatusCode := uint16(0)
	if code, ok := attrs["http.status_code"]; ok {
		if c, err := parseUint16(code); err == nil {
			httpStatusCode = c
		}
	}
	dbSystem := attrs["db.system"]

	// Determine if error
	isError := span.Status != nil && span.Status.Code == tracepb.Status_STATUS_CODE_ERROR

	// Determine if slow (>1s by default)
	durationNs := int64(span.EndTimeUnixNano - span.StartTimeUnixNano)
	isSlow := durationNs > h.config.Sampling.SlowThresholdMs*1000000

	return clickhouse.TraceRow{
		TenantID:       tenantID,
		Timestamp:      time.Unix(0, int64(span.StartTimeUnixNano)),
		TraceID:        hex.EncodeToString(span.TraceId),
		SpanID:         hex.EncodeToString(span.SpanId),
		ParentSpanID:   hex.EncodeToString(span.ParentSpanId),
		SpanName:       span.Name,
		SpanKind:       span.Kind.String(),
		ServiceName:    serviceName,
		DurationNs:     durationNs,
		StatusCode:     span.Status.GetCode().String(),
		StatusMessage:  span.Status.GetMessage(),
		Attributes:     attrs,
		HttpMethod:     httpMethod,
		HttpStatusCode: httpStatusCode,
		HttpUrl:        httpUrl,
		DbSystem:       dbSystem,
		SampleRate:     1.0,
		IsError:        isError,
		IsSlow:         isSlow,
	}
}

// ========== Helpers ==========

func (h *OTLPHandler) extractTenantID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if tenants := md.Get(h.config.Tenancy.TenantHeader); len(tenants) > 0 {
			return tenants[0]
		}
	}
	return h.config.Tenancy.DefaultTenantID
}

func (h *OTLPHandler) extractTenantIDFromHTTP(r *http.Request) string {
	tenantID := r.Header.Get(h.config.Tenancy.TenantHeader)
	if tenantID == "" {
		return h.config.Tenancy.DefaultTenantID
	}
	return tenantID
}

func extractAttributes(attrs []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string)
	for _, kv := range attrs {
		switch v := kv.Value.Value.(type) {
		case *commonpb.AnyValue_StringValue:
			result[kv.Key] = v.StringValue
		case *commonpb.AnyValue_IntValue:
			result[kv.Key] = string(rune(v.IntValue))
		case *commonpb.AnyValue_DoubleValue:
			result[kv.Key] = string(rune(int64(v.DoubleValue)))
		case *commonpb.AnyValue_BoolValue:
			if v.BoolValue {
				result[kv.Key] = "true"
			} else {
				result[kv.Key] = "false"
			}
		}
	}
	return result
}

// NumberDataPoint interface for gauge and sum data points
type numberDataPoint interface {
	GetAsDouble() float64
	GetAsInt() int64
}

func getNumberValue(dp numberDataPoint) float64 {
	if v := dp.GetAsDouble(); v != 0 {
		return v
	}
	return float64(dp.GetAsInt())
}

func parseUint16(s string) (uint16, error) {
	var result uint16
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		result = result*10 + uint16(c-'0')
	}
	return result, nil
}

// Type aliases for OTLP types
type logsv1 = v1
type logspb = struct {
	LogRecord interface{}
}
