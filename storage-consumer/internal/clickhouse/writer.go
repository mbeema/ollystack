package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ollystack/storage-consumer/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
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
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"table"},
	)
)

// Writer handles batch writes to ClickHouse
type Writer struct {
	conn   driver.Conn
	config config.ClickHouseConfig
	logger *zap.Logger
}

// NewWriter creates a new ClickHouse writer
func NewWriter(cfg config.ClickHouseConfig, logger *zap.Logger) (*Writer, error) {
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
		DialTimeout:  time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
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
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	logger.Info("Connected to ClickHouse",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
	)

	return &Writer{
		conn:   conn,
		config: cfg,
		logger: logger,
	}, nil
}

// WriteMetrics writes a batch of metrics to ClickHouse
func (w *Writer) WriteMetrics(batch [][]byte) error {
	start := time.Now()
	table := "otel_metrics"

	ctx := context.Background()
	batchWriter, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO otel_metrics (
			Timestamp, MetricName, MetricType, Value, Labels,
			ServiceName, Host, Environment
		)
	`)
	if err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	rowCount := 0
	for _, data := range batch {
		var req metricsv1.ExportMetricsServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			w.logger.Warn("Failed to unmarshal metrics", zap.Error(err))
			continue
		}

		for _, rm := range req.ResourceMetrics {
			// Extract resource attributes
			serviceName := extractAttribute(rm.Resource, "service.name")
			host := extractAttribute(rm.Resource, "host.name")
			env := extractAttribute(rm.Resource, "deployment.environment")

			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					metricName := m.Name
					metricType := getMetricType(m)

					// Handle different metric types
					dataPoints := extractDataPoints(m)
					for _, dp := range dataPoints {
						labels := make(map[string]string)
						for _, attr := range dp.Attributes {
							labels[attr.Key] = attr.Value.GetStringValue()
						}

						if err := batchWriter.Append(
							time.Unix(0, int64(dp.TimeUnixNano)),
							metricName,
							metricType,
							dp.Value,
							labels,
							serviceName,
							host,
							env,
						); err != nil {
							w.logger.Warn("Failed to append metric",
								zap.String("metric", metricName),
								zap.Error(err),
							)
							continue
						}
						rowCount++
					}
				}
			}
		}
	}

	if err := batchWriter.Send(); err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to send batch: %w", err)
	}

	batchesWritten.WithLabelValues(table, "success").Inc()
	rowsWritten.WithLabelValues(table).Add(float64(rowCount))
	writeLatency.WithLabelValues(table).Observe(time.Since(start).Seconds())

	w.logger.Debug("Wrote metrics batch",
		zap.Int("rows", rowCount),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}

// WriteLogs writes a batch of logs to ClickHouse
func (w *Writer) WriteLogs(batch [][]byte) error {
	start := time.Now()
	table := "otel_logs"

	ctx := context.Background()
	batchWriter, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO otel_logs (
			Timestamp, TraceId, SpanId, SeverityText, SeverityNumber,
			Body, Attributes, ServiceName, Host, PatternHash, OccurrenceCount
		)
	`)
	if err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	rowCount := 0
	for _, data := range batch {
		var req v1.ExportLogsServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			w.logger.Warn("Failed to unmarshal logs", zap.Error(err))
			continue
		}

		for _, rl := range req.ResourceLogs {
			serviceName := extractAttribute(rl.Resource, "service.name")
			host := extractAttribute(rl.Resource, "host.name")

			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					attributes := make(map[string]string)
					for _, attr := range lr.Attributes {
						attributes[attr.Key] = attr.Value.GetStringValue()
					}

					traceID := fmt.Sprintf("%x", lr.TraceId)
					spanID := fmt.Sprintf("%x", lr.SpanId)
					body := lr.Body.GetStringValue()

					if err := batchWriter.Append(
						time.Unix(0, int64(lr.TimeUnixNano)),
						traceID,
						spanID,
						lr.SeverityText,
						uint8(lr.SeverityNumber),
						body,
						attributes,
						serviceName,
						host,
						"", // PatternHash computed by ClickHouse or separate process
						uint32(1),
					); err != nil {
						w.logger.Warn("Failed to append log", zap.Error(err))
						continue
					}
					rowCount++
				}
			}
		}
	}

	if err := batchWriter.Send(); err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to send batch: %w", err)
	}

	batchesWritten.WithLabelValues(table, "success").Inc()
	rowsWritten.WithLabelValues(table).Add(float64(rowCount))
	writeLatency.WithLabelValues(table).Observe(time.Since(start).Seconds())

	w.logger.Debug("Wrote logs batch",
		zap.Int("rows", rowCount),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}

// WriteTraces writes a batch of traces to ClickHouse
func (w *Writer) WriteTraces(batch [][]byte) error {
	start := time.Now()
	table := "otel_traces"

	ctx := context.Background()
	batchWriter, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO otel_traces (
			Timestamp, TraceId, SpanId, ParentSpanId, SpanName, SpanKind,
			ServiceName, Duration, StatusCode, StatusMessage, Attributes,
			HttpMethod, HttpStatusCode, HttpUrl, DbSystem
		)
	`)
	if err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	rowCount := 0
	for _, data := range batch {
		var req tracev1.ExportTraceServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			w.logger.Warn("Failed to unmarshal traces", zap.Error(err))
			continue
		}

		for _, rt := range req.ResourceSpans {
			serviceName := extractAttribute(rt.Resource, "service.name")

			for _, ss := range rt.ScopeSpans {
				for _, span := range ss.Spans {
					attributes := make(map[string]string)
					httpMethod := ""
					httpStatusCode := uint16(0)
					httpUrl := ""
					dbSystem := ""

					for _, attr := range span.Attributes {
						key := attr.Key
						value := attr.Value.GetStringValue()
						attributes[key] = value

						switch key {
						case "http.method":
							httpMethod = value
						case "http.status_code":
							httpStatusCode = uint16(attr.Value.GetIntValue())
						case "http.url":
							httpUrl = value
						case "db.system":
							dbSystem = value
						}
					}

					traceID := fmt.Sprintf("%x", span.TraceId)
					spanID := fmt.Sprintf("%x", span.SpanId)
					parentSpanID := fmt.Sprintf("%x", span.ParentSpanId)

					if err := batchWriter.Append(
						time.Unix(0, int64(span.StartTimeUnixNano)),
						traceID,
						spanID,
						parentSpanID,
						span.Name,
						span.Kind.String(),
						serviceName,
						int64(span.EndTimeUnixNano-span.StartTimeUnixNano),
						span.Status.Code.String(),
						span.Status.Message,
						attributes,
						httpMethod,
						httpStatusCode,
						httpUrl,
						dbSystem,
					); err != nil {
						w.logger.Warn("Failed to append span", zap.Error(err))
						continue
					}
					rowCount++
				}
			}
		}
	}

	if err := batchWriter.Send(); err != nil {
		batchesWritten.WithLabelValues(table, "error").Inc()
		return fmt.Errorf("failed to send batch: %w", err)
	}

	batchesWritten.WithLabelValues(table, "success").Inc()
	rowsWritten.WithLabelValues(table).Add(float64(rowCount))
	writeLatency.WithLabelValues(table).Observe(time.Since(start).Seconds())

	w.logger.Debug("Wrote traces batch",
		zap.Int("rows", rowCount),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}

// Close closes the ClickHouse connection
func (w *Writer) Close() error {
	return w.conn.Close()
}

// Helper functions

func extractAttribute(resource interface{}, key string) string {
	// This would extract attributes from the resource
	// Simplified for now
	return ""
}

func getMetricType(m interface{}) string {
	// Extract metric type from OTLP metric
	return "gauge"
}

type dataPoint struct {
	TimeUnixNano uint64
	Value        float64
	Attributes   []struct {
		Key   string
		Value interface{ GetStringValue() string }
	}
}

func extractDataPoints(m interface{}) []dataPoint {
	// Extract data points from OTLP metric
	// Simplified - would handle gauge, counter, histogram, etc.
	return nil
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
