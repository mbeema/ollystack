package clickhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	rowsWritten = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_clickhouse_rows_written_total",
			Help: "Total rows written to ClickHouse",
		},
		[]string{"table"},
	)
	batchesWritten = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_clickhouse_batches_written_total",
			Help: "Total batches written to ClickHouse",
		},
		[]string{"table", "status"},
	)
	writeLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_clickhouse_write_latency_seconds",
			Help:    "ClickHouse write latency",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		},
		[]string{"table"},
	)
	batchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_clickhouse_batch_size",
			Help:    "Batch sizes written to ClickHouse",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000},
		},
		[]string{"table"},
	)
	bufferSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ollystack_clickhouse_buffer_size",
			Help: "Current buffer size",
		},
		[]string{"table"},
	)
)

// Config holds ClickHouse connection configuration
type Config struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	Compression     string        `mapstructure:"compression"`
	BatchSize       int           `mapstructure:"batch_size"`
	FlushInterval   time.Duration `mapstructure:"flush_interval"`
	MaxRetries      int           `mapstructure:"max_retries"`
	RetryInterval   time.Duration `mapstructure:"retry_interval"`
}

// DefaultConfig returns default ClickHouse configuration
func DefaultConfig() Config {
	return Config{
		Host:          "localhost",
		Port:          9000,
		Database:      "ollystack",
		Username:      "default",
		Password:      "",
		MaxOpenConns:  10,
		MaxIdleConns:  5,
		DialTimeout:   10 * time.Second,
		ReadTimeout:   60 * time.Second,
		WriteTimeout:  60 * time.Second,
		Compression:   "lz4",
		BatchSize:     10000,
		FlushInterval: 1 * time.Second,
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
	}
}

// Writer handles batched writes to ClickHouse
type Writer struct {
	conn   driver.Conn
	config Config
	logger *zap.Logger

	// Buffers for batching
	metricsBuffer []MetricRow
	logsBuffer    []LogRow
	tracesBuffer  []TraceRow

	// Mutexes for thread-safe buffering
	metricsMu sync.Mutex
	logsMu    sync.Mutex
	tracesMu  sync.Mutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MetricRow represents a metric to be inserted
type MetricRow struct {
	TenantID    string
	Timestamp   time.Time
	MetricName  string
	MetricType  string
	Value       float64
	Labels      map[string]string
	ServiceName string
	Host        string
	Environment string
	SampleRate  float32
}

// LogRow represents a log to be inserted
type LogRow struct {
	TenantID        string
	Timestamp       time.Time
	TraceID         string
	SpanID          string
	Severity        string
	SeverityNumber  uint8
	Body            string
	Attributes      map[string]string
	ServiceName     string
	Host            string
	PatternHash     string
	OccurrenceCount uint32
	SampleRate      float32
}

// TraceRow represents a span to be inserted
type TraceRow struct {
	TenantID       string
	Timestamp      time.Time
	TraceID        string
	SpanID         string
	ParentSpanID   string
	SpanName       string
	SpanKind       string
	ServiceName    string
	DurationNs     int64
	StatusCode     string
	StatusMessage  string
	Attributes     map[string]string
	HttpMethod     string
	HttpStatusCode uint16
	HttpUrl        string
	DbSystem       string
	SampleRate     float32
	IsError        bool
	IsSlow         bool
}

// NewWriter creates a new ClickHouse writer
func NewWriter(cfg Config, logger *zap.Logger) (*Writer, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: cfg.DialTimeout,
		ReadTimeout: cfg.ReadTimeout,
		Compression: &clickhouse.Compression{
			Method: parseCompression(cfg.Compression),
		},
		MaxOpenConns: cfg.MaxOpenConns,
		MaxIdleConns: cfg.MaxIdleConns,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	writerCtx, writerCancel := context.WithCancel(context.Background())

	w := &Writer{
		conn:          conn,
		config:        cfg,
		logger:        logger,
		metricsBuffer: make([]MetricRow, 0, cfg.BatchSize),
		logsBuffer:    make([]LogRow, 0, cfg.BatchSize),
		tracesBuffer:  make([]TraceRow, 0, cfg.BatchSize),
		ctx:           writerCtx,
		cancel:        writerCancel,
	}

	// Start flush loops
	w.wg.Add(3)
	go w.flushLoop("metrics")
	go w.flushLoop("logs")
	go w.flushLoop("traces")

	logger.Info("Connected to ClickHouse",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
	)

	return w, nil
}

// WriteMetric adds a metric to the buffer
func (w *Writer) WriteMetric(row MetricRow) {
	w.metricsMu.Lock()
	w.metricsBuffer = append(w.metricsBuffer, row)
	size := len(w.metricsBuffer)
	w.metricsMu.Unlock()

	bufferSize.WithLabelValues("metrics").Set(float64(size))

	// Flush if buffer is full
	if size >= w.config.BatchSize {
		go w.flushMetrics()
	}
}

// WriteLog adds a log to the buffer
func (w *Writer) WriteLog(row LogRow) {
	w.logsMu.Lock()
	w.logsBuffer = append(w.logsBuffer, row)
	size := len(w.logsBuffer)
	w.logsMu.Unlock()

	bufferSize.WithLabelValues("logs").Set(float64(size))

	if size >= w.config.BatchSize {
		go w.flushLogs()
	}
}

// WriteTrace adds a trace to the buffer
func (w *Writer) WriteTrace(row TraceRow) {
	w.tracesMu.Lock()
	w.tracesBuffer = append(w.tracesBuffer, row)
	size := len(w.tracesBuffer)
	w.tracesMu.Unlock()

	bufferSize.WithLabelValues("traces").Set(float64(size))

	if size >= w.config.BatchSize {
		go w.flushTraces()
	}
}

// flushLoop periodically flushes buffers
func (w *Writer) flushLoop(table string) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// Final flush
			switch table {
			case "metrics":
				w.flushMetrics()
			case "logs":
				w.flushLogs()
			case "traces":
				w.flushTraces()
			}
			return
		case <-ticker.C:
			switch table {
			case "metrics":
				w.flushMetrics()
			case "logs":
				w.flushLogs()
			case "traces":
				w.flushTraces()
			}
		}
	}
}

func (w *Writer) flushMetrics() {
	w.metricsMu.Lock()
	if len(w.metricsBuffer) == 0 {
		w.metricsMu.Unlock()
		return
	}
	batch := w.metricsBuffer
	w.metricsBuffer = make([]MetricRow, 0, w.config.BatchSize)
	w.metricsMu.Unlock()

	bufferSize.WithLabelValues("metrics").Set(0)

	start := time.Now()
	err := w.insertMetrics(batch)

	if err != nil {
		batchesWritten.WithLabelValues("metrics_raw", "error").Inc()
		w.logger.Error("Failed to write metrics", zap.Error(err), zap.Int("count", len(batch)))
		// TODO: Write to disk buffer for retry
		return
	}

	batchesWritten.WithLabelValues("metrics_raw", "success").Inc()
	rowsWritten.WithLabelValues("metrics_raw").Add(float64(len(batch)))
	writeLatency.WithLabelValues("metrics_raw").Observe(time.Since(start).Seconds())
	batchSize.WithLabelValues("metrics_raw").Observe(float64(len(batch)))
}

func (w *Writer) flushLogs() {
	w.logsMu.Lock()
	if len(w.logsBuffer) == 0 {
		w.logsMu.Unlock()
		return
	}
	batch := w.logsBuffer
	w.logsBuffer = make([]LogRow, 0, w.config.BatchSize)
	w.logsMu.Unlock()

	bufferSize.WithLabelValues("logs").Set(0)

	start := time.Now()
	err := w.insertLogs(batch)

	if err != nil {
		batchesWritten.WithLabelValues("logs_raw", "error").Inc()
		w.logger.Error("Failed to write logs", zap.Error(err), zap.Int("count", len(batch)))
		return
	}

	batchesWritten.WithLabelValues("logs_raw", "success").Inc()
	rowsWritten.WithLabelValues("logs_raw").Add(float64(len(batch)))
	writeLatency.WithLabelValues("logs_raw").Observe(time.Since(start).Seconds())
	batchSize.WithLabelValues("logs_raw").Observe(float64(len(batch)))
}

func (w *Writer) flushTraces() {
	w.tracesMu.Lock()
	if len(w.tracesBuffer) == 0 {
		w.tracesMu.Unlock()
		return
	}
	batch := w.tracesBuffer
	w.tracesBuffer = make([]TraceRow, 0, w.config.BatchSize)
	w.tracesMu.Unlock()

	bufferSize.WithLabelValues("traces").Set(0)

	start := time.Now()
	err := w.insertTraces(batch)

	if err != nil {
		batchesWritten.WithLabelValues("traces_raw", "error").Inc()
		w.logger.Error("Failed to write traces", zap.Error(err), zap.Int("count", len(batch)))
		return
	}

	batchesWritten.WithLabelValues("traces_raw", "success").Inc()
	rowsWritten.WithLabelValues("traces_raw").Add(float64(len(batch)))
	writeLatency.WithLabelValues("traces_raw").Observe(time.Since(start).Seconds())
	batchSize.WithLabelValues("traces_raw").Observe(float64(len(batch)))
}

func (w *Writer) insertMetrics(batch []MetricRow) error {
	ctx := context.Background()

	batchInsert, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO metrics_raw (
			tenant_id, timestamp, metric_name, metric_type, value, labels,
			service_name, host, environment, sample_rate
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, row := range batch {
		if err := batchInsert.Append(
			row.TenantID,
			row.Timestamp,
			row.MetricName,
			row.MetricType,
			row.Value,
			row.Labels,
			row.ServiceName,
			row.Host,
			row.Environment,
			row.SampleRate,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batchInsert.Send()
}

func (w *Writer) insertLogs(batch []LogRow) error {
	ctx := context.Background()

	batchInsert, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO logs_raw (
			tenant_id, timestamp, trace_id, span_id, severity, severity_number,
			body, attributes, service_name, host, pattern_hash, occurrence_count, sample_rate
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, row := range batch {
		if err := batchInsert.Append(
			row.TenantID,
			row.Timestamp,
			row.TraceID,
			row.SpanID,
			row.Severity,
			row.SeverityNumber,
			row.Body,
			row.Attributes,
			row.ServiceName,
			row.Host,
			row.PatternHash,
			row.OccurrenceCount,
			row.SampleRate,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batchInsert.Send()
}

func (w *Writer) insertTraces(batch []TraceRow) error {
	ctx := context.Background()

	batchInsert, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO traces_raw (
			tenant_id, timestamp, trace_id, span_id, parent_span_id, span_name,
			span_kind, service_name, duration_ns, status_code, status_message,
			attributes, http_method, http_status_code, http_url, db_system,
			sample_rate, is_error, is_slow
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, row := range batch {
		if err := batchInsert.Append(
			row.TenantID,
			row.Timestamp,
			row.TraceID,
			row.SpanID,
			row.ParentSpanID,
			row.SpanName,
			row.SpanKind,
			row.ServiceName,
			row.DurationNs,
			row.StatusCode,
			row.StatusMessage,
			row.Attributes,
			row.HttpMethod,
			row.HttpStatusCode,
			row.HttpUrl,
			row.DbSystem,
			row.SampleRate,
			row.IsError,
			row.IsSlow,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batchInsert.Send()
}

// Close closes the writer
func (w *Writer) Close() error {
	w.cancel()
	w.wg.Wait()
	return w.conn.Close()
}

// IsHealthy checks if ClickHouse is reachable
func (w *Writer) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.conn.Ping(ctx) == nil
}

func parseCompression(compression string) clickhouse.CompressionMethod {
	switch compression {
	case "lz4":
		return clickhouse.CompressionLZ4
	case "zstd":
		return clickhouse.CompressionZSTD
	default:
		return clickhouse.CompressionNone
	}
}
