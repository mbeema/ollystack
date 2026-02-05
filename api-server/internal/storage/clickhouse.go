// Package storage provides database clients for the API server.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
)

// ClickHouseConfig holds ClickHouse connection configuration.
type ClickHouseConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

// ClickHouseClient wraps the ClickHouse connection.
type ClickHouseClient struct {
	conn   driver.Conn
	logger *zap.Logger
}

// NewClickHouseClient creates a new ClickHouse client.
func NewClickHouseClient(cfg ClickHouseConfig, logger *zap.Logger) (*ClickHouseClient, error) {
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
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
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

	logger.Info("Connected to ClickHouse", zap.String("host", cfg.Host), zap.String("database", cfg.Database))

	return &ClickHouseClient{
		conn:   conn,
		logger: logger,
	}, nil
}

// Close closes the ClickHouse connection.
func (c *ClickHouseClient) Close() error {
	return c.conn.Close()
}

// Conn returns the underlying connection for direct queries.
func (c *ClickHouseClient) Conn() driver.Conn {
	return c.conn
}

// TraceSpan represents a span from ClickHouse.
type TraceSpan struct {
	Timestamp          time.Time
	TraceId            string
	SpanId             string
	ParentSpanId       string
	SpanName           string
	SpanKind           string
	ServiceName        string
	Duration           int64
	StatusCode         string
	StatusMessage      string
	SpanAttributes     map[string]string
	ResourceAttributes map[string]string
}

// TraceSummary represents a trace summary for list view.
type TraceSummary struct {
	TraceId       string    `json:"traceId"`
	ServiceName   string    `json:"serviceName"`
	OperationName string    `json:"operationName"`
	Duration      int64     `json:"duration"`
	SpanCount     uint64    `json:"spanCount"`
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
}

// ListTraces returns a list of trace summaries.
func (c *ClickHouseClient) ListTraces(ctx context.Context, service string, minDuration time.Duration, start, end time.Time, limit int) ([]TraceSummary, error) {
	query := `
		SELECT
			TraceId,
			any(ServiceName) as ServiceName,
			any(SpanName) as OperationName,
			max(Duration) as Duration,
			count() as SpanCount,
			if(countIf(StatusCode = 'STATUS_CODE_ERROR') > 0, 'error', 'ok') as Status,
			min(Timestamp) as StartTime
		FROM traces
		WHERE Timestamp >= ? AND Timestamp <= ?
	`
	args := []interface{}{start, end}

	if service != "" {
		query += " AND ServiceName = ?"
		args = append(args, service)
	}

	if minDuration > 0 {
		query += " AND Duration >= ?"
		args = append(args, minDuration.Nanoseconds())
	}

	query += `
		GROUP BY TraceId
		ORDER BY StartTime DESC
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to query traces", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var traces []TraceSummary
	for rows.Next() {
		var t TraceSummary
		if err := rows.Scan(&t.TraceId, &t.ServiceName, &t.OperationName, &t.Duration, &t.SpanCount, &t.Status, &t.Timestamp); err != nil {
			c.logger.Error("Failed to scan trace row", zap.Error(err))
			continue
		}
		traces = append(traces, t)
	}

	return traces, nil
}

// GetTraceSpans returns all spans for a trace.
func (c *ClickHouseClient) GetTraceSpans(ctx context.Context, traceId string) ([]TraceSpan, error) {
	query := `
		SELECT
			Timestamp,
			TraceId,
			SpanId,
			ParentSpanId,
			SpanName,
			SpanKind,
			ServiceName,
			Duration,
			StatusCode,
			StatusMessage,
			SpanAttributes,
			ResourceAttributes
		FROM traces
		WHERE TraceId = ?
		ORDER BY Timestamp ASC
	`

	rows, err := c.conn.Query(ctx, query, traceId)
	if err != nil {
		c.logger.Error("Failed to query trace spans", zap.Error(err), zap.String("traceId", traceId))
		return nil, err
	}
	defer rows.Close()

	var spans []TraceSpan
	for rows.Next() {
		var s TraceSpan
		if err := rows.Scan(
			&s.Timestamp,
			&s.TraceId,
			&s.SpanId,
			&s.ParentSpanId,
			&s.SpanName,
			&s.SpanKind,
			&s.ServiceName,
			&s.Duration,
			&s.StatusCode,
			&s.StatusMessage,
			&s.SpanAttributes,
			&s.ResourceAttributes,
		); err != nil {
			c.logger.Error("Failed to scan span row", zap.Error(err))
			continue
		}
		// Normalize status code for UI display
		s.StatusCode = normalizeStatusCode(s.StatusCode)
		spans = append(spans, s)
	}

	return spans, nil
}

// normalizeStatusCode converts OTel status codes to user-friendly values
func normalizeStatusCode(status string) string {
	switch status {
	case "STATUS_CODE_ERROR":
		return "error"
	case "STATUS_CODE_OK", "STATUS_CODE_UNSET", "":
		return "ok"
	default:
		return status
	}
}

// GetServices returns a list of unique service names.
func (c *ClickHouseClient) GetServices(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT ServiceName FROM traces ORDER BY ServiceName`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		services = append(services, s)
	}

	return services, nil
}

// ================================================================================
// TOPOLOGY / SERVICE MAP QUERIES
// ================================================================================

// ServiceStats represents statistics for a service node in the topology.
type ServiceStats struct {
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	TotalSpans     uint64  `json:"totalSpans"`
	ErrorRate      float64 `json:"errorRate"`
	AvgLatencyMs   float64 `json:"avgLatency"`
	P50LatencyMs   float64 `json:"p50Latency"`
	P95LatencyMs   float64 `json:"p95Latency"`
	P99LatencyMs   float64 `json:"p99Latency"`
	RequestsPerSec float64 `json:"requestRate"`
}

// ServiceEdge represents a connection between two services in the topology.
type ServiceEdge struct {
	Source         string  `json:"source"`
	Target         string  `json:"target"`
	RequestCount   uint64  `json:"requestCount"`
	ErrorRate      float64 `json:"errorRate"`
	AvgLatencyMs   float64 `json:"avgLatency"`
	P95LatencyMs   float64 `json:"p95Latency"`
}

// GetServiceStats returns statistics for all services in the last hour.
func (c *ClickHouseClient) GetServiceStats(ctx context.Context) ([]ServiceStats, error) {
	query := `
		SELECT
			ServiceName,
			count() as total_spans,
			round(countIf(StatusCode = 'STATUS_CODE_ERROR') / count() * 100, 2) as error_rate,
			round(avg(Duration) / 1000000, 2) as avg_latency_ms,
			round(quantile(0.50)(Duration) / 1000000, 2) as p50_latency_ms,
			round(quantile(0.95)(Duration) / 1000000, 2) as p95_latency_ms,
			round(quantile(0.99)(Duration) / 1000000, 2) as p99_latency_ms,
			round(count() / 3600, 2) as requests_per_sec
		FROM traces
		WHERE Timestamp > now() - INTERVAL 1 HOUR
		GROUP BY ServiceName
		ORDER BY total_spans DESC
	`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		c.logger.Error("Failed to query service stats", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var stats []ServiceStats
	for rows.Next() {
		var s ServiceStats
		if err := rows.Scan(
			&s.Name,
			&s.TotalSpans,
			&s.ErrorRate,
			&s.AvgLatencyMs,
			&s.P50LatencyMs,
			&s.P95LatencyMs,
			&s.P99LatencyMs,
			&s.RequestsPerSec,
		); err != nil {
			c.logger.Error("Failed to scan service stats row", zap.Error(err))
			continue
		}
		// Determine service type based on name patterns
		s.Type = detectServiceType(s.Name)
		stats = append(stats, s)
	}

	return stats, nil
}

// GetServiceEdges returns service-to-service dependencies from trace data.
func (c *ClickHouseClient) GetServiceEdges(ctx context.Context) ([]ServiceEdge, error) {
	query := `
		WITH edges AS (
			SELECT
				parent.ServiceName as source,
				child.ServiceName as target,
				child.Duration / 1000000 as duration_ms,
				child.StatusCode
			FROM traces child
			INNER JOIN traces parent
				ON child.ParentSpanId = parent.SpanId
				AND child.TraceId = parent.TraceId
			WHERE child.Timestamp > now() - INTERVAL 1 HOUR
				AND parent.ServiceName != child.ServiceName
		)
		SELECT
			source,
			target,
			count() as request_count,
			round(countIf(StatusCode = 'STATUS_CODE_ERROR') / count() * 100, 2) as error_rate,
			round(avg(duration_ms), 2) as avg_latency_ms,
			round(quantile(0.95)(duration_ms), 2) as p95_latency_ms
		FROM edges
		GROUP BY source, target
		ORDER BY request_count DESC
	`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		c.logger.Error("Failed to query service edges", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var edges []ServiceEdge
	for rows.Next() {
		var e ServiceEdge
		if err := rows.Scan(
			&e.Source,
			&e.Target,
			&e.RequestCount,
			&e.ErrorRate,
			&e.AvgLatencyMs,
			&e.P95LatencyMs,
		); err != nil {
			c.logger.Error("Failed to scan service edge row", zap.Error(err))
			continue
		}
		edges = append(edges, e)
	}

	return edges, nil
}

// detectServiceType determines service type based on name patterns.
func detectServiceType(name string) string {
	switch {
	case contains(name, "postgres", "mysql", "redis", "clickhouse", "db", "database"):
		return "database"
	case contains(name, "kafka", "rabbitmq", "queue", "mq"):
		return "queue"
	case contains(name, "gateway", "api-gateway", "ingress"):
		return "gateway"
	case contains(name, "generator", "client", "traffic"):
		return "client"
	default:
		return "service"
	}
}

// contains checks if any of the patterns exist in the string (case-insensitive).
func contains(s string, patterns ...string) bool {
	lower := toLowerCase(s)
	for _, p := range patterns {
		if containsStr(lower, p) {
			return true
		}
	}
	return false
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// LogEntry represents a log entry from ClickHouse.
type LogEntry struct {
	Timestamp      time.Time         `json:"timestamp"`
	TraceId        string            `json:"traceId,omitempty"`
	SpanId         string            `json:"spanId,omitempty"`
	SeverityText   string            `json:"severityText"`
	SeverityNumber int32             `json:"severityNumber"`
	ServiceName    string            `json:"serviceName"`
	Body           string            `json:"body"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

// QueryLogs returns logs matching the query.
func (c *ClickHouseClient) QueryLogs(ctx context.Context, serviceName string, severities []string, search string, start, end time.Time, limit int) ([]LogEntry, error) {
	query := `
		SELECT
			Timestamp,
			TraceId,
			SpanId,
			SeverityText,
			SeverityNumber,
			ServiceName,
			Body,
			LogAttributes
		FROM logs
		WHERE Timestamp >= ? AND Timestamp <= ?
	`
	args := []interface{}{start, end}

	if serviceName != "" {
		query += " AND ServiceName = ?"
		args = append(args, serviceName)
	}

	if len(severities) > 0 {
		query += " AND SeverityText IN ?"
		args = append(args, severities)
	}

	if search != "" {
		query += " AND Body ILIKE ?"
		args = append(args, "%"+search+"%")
	}

	query += " ORDER BY Timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to query logs", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(
			&l.Timestamp,
			&l.TraceId,
			&l.SpanId,
			&l.SeverityText,
			&l.SeverityNumber,
			&l.ServiceName,
			&l.Body,
			&l.Attributes,
		); err != nil {
			c.logger.Error("Failed to scan log row", zap.Error(err))
			continue
		}
		logs = append(logs, l)
	}

	return logs, nil
}

// GetLogServices returns list of services with logs.
func (c *ClickHouseClient) GetLogServices(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT ServiceName FROM logs WHERE ServiceName != '' ORDER BY ServiceName`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		services = append(services, s)
	}

	return services, nil
}

// MetricSummary represents a metric summary.
type MetricSummary struct {
	MetricName string    `json:"metricName"`
	Value      float64   `json:"value"`
	Timestamp  time.Time `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// QueryMetrics returns metrics matching the query.
func (c *ClickHouseClient) QueryMetrics(ctx context.Context, metricName string, start, end time.Time, limit int) ([]MetricSummary, error) {
	query := `
		SELECT
			MetricName,
			Value,
			TimeUnix,
			Attributes
		FROM metrics_gauge
		WHERE TimeUnix >= ? AND TimeUnix <= ?
	`
	args := []interface{}{start, end}

	if metricName != "" {
		query += " AND MetricName ILIKE ?"
		args = append(args, "%"+metricName+"%")
	}

	query += " ORDER BY TimeUnix DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to query metrics", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var metrics []MetricSummary
	for rows.Next() {
		var m MetricSummary
		if err := rows.Scan(
			&m.MetricName,
			&m.Value,
			&m.Timestamp,
			&m.Attributes,
		); err != nil {
			c.logger.Error("Failed to scan metric row", zap.Error(err))
			continue
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// GetMetricNames returns list of available metric names.
func (c *ClickHouseClient) GetMetricNames(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT MetricName FROM metrics_gauge ORDER BY MetricName LIMIT 100`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		names = append(names, n)
	}

	return names, nil
}

// ================================================================================
// METRIC EXEMPLAR QUERIES
// Exemplars link metrics to traces for root cause analysis
// ================================================================================

// MetricExemplar represents a metric data point with trace context
type MetricExemplar struct {
	Timestamp       time.Time         `json:"timestamp"`
	MetricName      string            `json:"metricName"`
	Value           float64           `json:"value"`
	TraceID         string            `json:"traceId,omitempty"`
	SpanID          string            `json:"spanId,omitempty"`
	ServiceName     string            `json:"serviceName"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	HistogramBucket string            `json:"histogramBucket,omitempty"`
}

// MetricWithExemplars represents a metric with its associated exemplars
type MetricWithExemplars struct {
	MetricName  string            `json:"metricName"`
	Description string            `json:"description,omitempty"`
	Unit        string            `json:"unit,omitempty"`
	MetricType  string            `json:"metricType"`
	ServiceName string            `json:"serviceName"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	DataPoints  []MetricDataPoint `json:"dataPoints"`
	Exemplars   []MetricExemplar  `json:"exemplars"`
}

// MetricDataPoint represents a single metric data point
type MetricDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// GetMetricsWithExemplars retrieves metrics with their trace-linked exemplars
// This enables clicking from a metric spike directly to the contributing traces
func (c *ClickHouseClient) GetMetricsWithExemplars(ctx context.Context, metricName string, serviceName string, start, end time.Time, limit int) ([]MetricWithExemplars, error) {
	query := `
		SELECT
			MetricName,
			MetricDescription,
			MetricUnit,
			MetricType,
			ServiceName,
			Timestamp,
			Value,
			ExemplarTraceId,
			ExemplarSpanId,
			ExemplarValue,
			Attributes
		FROM otel_metrics
		WHERE Timestamp >= ? AND Timestamp <= ?
	`
	args := []interface{}{start, end}

	if metricName != "" {
		query += " AND MetricName = ?"
		args = append(args, metricName)
	}

	if serviceName != "" {
		query += " AND ServiceName = ?"
		args = append(args, serviceName)
	}

	query += " ORDER BY Timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to query metrics with exemplars", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	// Group by metric name and service
	metricsMap := make(map[string]*MetricWithExemplars)

	for rows.Next() {
		var mName, mDesc, mUnit, mType, svcName string
		var ts time.Time
		var value float64
		var traceID, spanID string
		var exemplarValue float64
		var attrs map[string]string

		if err := rows.Scan(&mName, &mDesc, &mUnit, &mType, &svcName, &ts, &value, &traceID, &spanID, &exemplarValue, &attrs); err != nil {
			c.logger.Error("Failed to scan metric row", zap.Error(err))
			continue
		}

		key := fmt.Sprintf("%s:%s", mName, svcName)
		if _, exists := metricsMap[key]; !exists {
			metricsMap[key] = &MetricWithExemplars{
				MetricName:  mName,
				Description: mDesc,
				Unit:        mUnit,
				MetricType:  mType,
				ServiceName: svcName,
				Attributes:  attrs,
				DataPoints:  []MetricDataPoint{},
				Exemplars:   []MetricExemplar{},
			}
		}

		// Add data point
		metricsMap[key].DataPoints = append(metricsMap[key].DataPoints, MetricDataPoint{
			Timestamp: ts,
			Value:     value,
		})

		// Add exemplar if trace context exists
		if traceID != "" {
			metricsMap[key].Exemplars = append(metricsMap[key].Exemplars, MetricExemplar{
				Timestamp:   ts,
				MetricName:  mName,
				Value:       exemplarValue,
				TraceID:     traceID,
				SpanID:      spanID,
				ServiceName: svcName,
				Attributes:  attrs,
			})
		}
	}

	// Convert map to slice
	result := make([]MetricWithExemplars, 0, len(metricsMap))
	for _, m := range metricsMap {
		result = append(result, *m)
	}

	return result, nil
}

// GetExemplarsForMetric retrieves all exemplars for a specific metric
// Returns trace IDs that can be used to investigate metric anomalies
func (c *ClickHouseClient) GetExemplarsForMetric(ctx context.Context, metricName string, start, end time.Time, limit int) ([]MetricExemplar, error) {
	query := `
		SELECT
			Timestamp,
			MetricName,
			Value,
			ExemplarTraceId,
			ExemplarSpanId,
			ExemplarValue,
			ServiceName,
			Attributes
		FROM otel_metrics
		WHERE MetricName = ?
			AND Timestamp >= ? AND Timestamp <= ?
			AND ExemplarTraceId != ''
		ORDER BY Timestamp DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, metricName, start, end, limit)
	if err != nil {
		c.logger.Error("Failed to query exemplars", zap.Error(err), zap.String("metricName", metricName))
		return nil, err
	}
	defer rows.Close()

	var exemplars []MetricExemplar
	for rows.Next() {
		var e MetricExemplar
		var exemplarValue float64
		if err := rows.Scan(&e.Timestamp, &e.MetricName, &e.Value, &e.TraceID, &e.SpanID, &exemplarValue, &e.ServiceName, &e.Attributes); err != nil {
			c.logger.Error("Failed to scan exemplar row", zap.Error(err))
			continue
		}
		if exemplarValue > 0 {
			e.Value = exemplarValue
		}
		exemplars = append(exemplars, e)
	}

	return exemplars, nil
}

// GetExemplarsInRange retrieves exemplars for metrics that exceed a threshold
// Useful for finding traces associated with latency spikes or error rates
func (c *ClickHouseClient) GetExemplarsInRange(ctx context.Context, metricName string, minValue, maxValue float64, start, end time.Time, limit int) ([]MetricExemplar, error) {
	query := `
		SELECT
			Timestamp,
			MetricName,
			Value,
			ExemplarTraceId,
			ExemplarSpanId,
			ExemplarValue,
			ServiceName,
			Attributes
		FROM otel_metrics
		WHERE MetricName = ?
			AND Timestamp >= ? AND Timestamp <= ?
			AND ExemplarTraceId != ''
			AND Value >= ? AND Value <= ?
		ORDER BY Value DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, metricName, start, end, minValue, maxValue, limit)
	if err != nil {
		c.logger.Error("Failed to query exemplars in range", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var exemplars []MetricExemplar
	for rows.Next() {
		var e MetricExemplar
		var exemplarValue float64
		if err := rows.Scan(&e.Timestamp, &e.MetricName, &e.Value, &e.TraceID, &e.SpanID, &exemplarValue, &e.ServiceName, &e.Attributes); err != nil {
			c.logger.Error("Failed to scan exemplar row", zap.Error(err))
			continue
		}
		if exemplarValue > 0 {
			e.Value = exemplarValue
		}
		exemplars = append(exemplars, e)
	}

	return exemplars, nil
}

// GetHighLatencyExemplars retrieves exemplars for requests exceeding a latency threshold
// Perfect for investigating P99 latency spikes
func (c *ClickHouseClient) GetHighLatencyExemplars(ctx context.Context, serviceName string, latencyThresholdMs float64, start, end time.Time, limit int) ([]MetricExemplar, error) {
	query := `
		SELECT
			Timestamp,
			MetricName,
			Value,
			ExemplarTraceId,
			ExemplarSpanId,
			ExemplarValue,
			ServiceName,
			Attributes
		FROM otel_metrics
		WHERE MetricName LIKE '%duration%' OR MetricName LIKE '%latency%'
			AND Timestamp >= ? AND Timestamp <= ?
			AND ExemplarTraceId != ''
			AND Value >= ?
	`
	args := []interface{}{start, end, latencyThresholdMs}

	if serviceName != "" {
		query += " AND ServiceName = ?"
		args = append(args, serviceName)
	}

	query += " ORDER BY Value DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to query high latency exemplars", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var exemplars []MetricExemplar
	for rows.Next() {
		var e MetricExemplar
		var exemplarValue float64
		if err := rows.Scan(&e.Timestamp, &e.MetricName, &e.Value, &e.TraceID, &e.SpanID, &exemplarValue, &e.ServiceName, &e.Attributes); err != nil {
			c.logger.Error("Failed to scan exemplar row", zap.Error(err))
			continue
		}
		if exemplarValue > 0 {
			e.Value = exemplarValue
		}
		exemplars = append(exemplars, e)
	}

	return exemplars, nil
}

// GetTraceFromExemplar retrieves the full trace for an exemplar's trace ID
// Enables drilling down from a metric spike to the actual request trace
func (c *ClickHouseClient) GetTraceFromExemplar(ctx context.Context, traceID string) ([]TraceSpan, error) {
	return c.GetTraceSpans(ctx, traceID)
}

// ================================================================================
// CORRELATION QUERIES
// ================================================================================

// CorrelationContext represents the full context for a correlation ID.
type CorrelationContext struct {
	CorrelationID string                   `json:"correlationId"`
	FirstSeen     time.Time                `json:"firstSeen"`
	LastSeen      time.Time                `json:"lastSeen"`
	Duration      int64                    `json:"duration"`
	Services      []string                 `json:"services"`
	SpanCount     uint64                   `json:"spanCount"`
	LogCount      uint64                   `json:"logCount"`
	ErrorCount    uint64                   `json:"errorCount"`
	HasErrors     bool                     `json:"hasErrors"`
	RootService   string                   `json:"rootService"`
	RootOperation string                   `json:"rootOperation"`
	Traces        []CorrelationTrace       `json:"traces,omitempty"`
	Logs          []CorrelationLog         `json:"logs,omitempty"`
	Timeline      []CorrelationTimelineItem `json:"timeline,omitempty"`
}

// CorrelationTrace represents a trace within a correlation.
type CorrelationTrace struct {
	TraceID       string            `json:"traceId"`
	SpanID        string            `json:"spanId"`
	ParentSpanID  string            `json:"parentSpanId"`
	ServiceName   string            `json:"serviceName"`
	OperationName string            `json:"operationName"`
	Duration      int64             `json:"duration"`
	StatusCode    string            `json:"statusCode"`
	StatusMessage string            `json:"statusMessage,omitempty"`
	Timestamp     time.Time         `json:"timestamp"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// CorrelationLog represents a log within a correlation.
type CorrelationLog struct {
	Timestamp    time.Time         `json:"timestamp"`
	ServiceName  string            `json:"serviceName"`
	SeverityText string            `json:"severityText"`
	Body         string            `json:"body"`
	TraceID      string            `json:"traceId,omitempty"`
	SpanID       string            `json:"spanId,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// CorrelationTimelineItem represents an item in the correlation timeline.
type CorrelationTimelineItem struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // "span_start", "span_end", "log", "error"
	ServiceName string    `json:"serviceName"`
	Description string    `json:"description"`
	Duration    int64     `json:"duration,omitempty"`
	IsError     bool      `json:"isError"`
}

// CorrelationSummary represents a summary for list view.
type CorrelationSummary struct {
	CorrelationID string    `json:"correlationId"`
	FirstSeen     time.Time `json:"firstSeen"`
	LastSeen      time.Time `json:"lastSeen"`
	Services      []string  `json:"services"`
	SpanCount     uint64    `json:"spanCount"`
	ErrorCount    uint64    `json:"errorCount"`
	HasErrors     bool      `json:"hasErrors"`
	RootService   string    `json:"rootService"`
	MaxDuration   int64     `json:"maxDuration"`
}

// GetCorrelation returns the full context for a correlation ID.
// The correlationID can be either an explicit correlation_id or a TraceId (for zero-code instrumented services).
func (c *ClickHouseClient) GetCorrelation(ctx context.Context, correlationID string) (*CorrelationContext, error) {
	// Get correlation summary - match by correlation_id OR TraceId
	summaryQuery := `
		SELECT
			if(correlation_id != '', correlation_id, TraceId) as corr_id,
			min(Timestamp) as first_seen,
			max(Timestamp) as last_seen,
			groupUniqArray(ServiceName) as services,
			count() as span_count,
			countIf(StatusCode = 'STATUS_CODE_ERROR') as error_count,
			argMin(ServiceName, Timestamp) as root_service,
			argMin(SpanName, Timestamp) as root_operation,
			max(Duration) as max_duration
		FROM traces
		WHERE correlation_id = ? OR TraceId = ?
		GROUP BY corr_id
	`

	var ctx_out CorrelationContext
	ctx_out.CorrelationID = correlationID

	row := c.conn.QueryRow(ctx, summaryQuery, correlationID, correlationID)
	var services []string
	err := row.Scan(
		&ctx_out.CorrelationID,
		&ctx_out.FirstSeen,
		&ctx_out.LastSeen,
		&services,
		&ctx_out.SpanCount,
		&ctx_out.ErrorCount,
		&ctx_out.RootService,
		&ctx_out.RootOperation,
		&ctx_out.Duration,
	)
	if err != nil {
		c.logger.Error("Failed to get correlation summary", zap.Error(err), zap.String("correlationId", correlationID))
		return nil, fmt.Errorf("correlation not found: %w", err)
	}

	ctx_out.Services = services
	ctx_out.HasErrors = ctx_out.ErrorCount > 0
	ctx_out.Duration = ctx_out.LastSeen.Sub(ctx_out.FirstSeen).Nanoseconds()

	// Get log count - match by correlation_id (MATERIALIZED column from Body JSON) OR TraceId
	logCountQuery := `SELECT count() FROM logs WHERE correlation_id = ? OR TraceId = ?`
	var logCount uint64
	if err := c.conn.QueryRow(ctx, logCountQuery, correlationID, correlationID).Scan(&logCount); err == nil {
		ctx_out.LogCount = logCount
	}

	return &ctx_out, nil
}

// GetCorrelationTraces returns all traces for a correlation ID.
// The correlationID can be either an explicit correlation_id or a TraceId (for zero-code instrumented services).
func (c *ClickHouseClient) GetCorrelationTraces(ctx context.Context, correlationID string) ([]CorrelationTrace, error) {
	// Match by SpanAttributes['correlation_id'] OR TraceId (supports both explicit and auto-instrumented traces)
	query := `
		SELECT
			TraceId,
			SpanId,
			ParentSpanId,
			ServiceName,
			SpanName,
			Duration,
			StatusCode,
			StatusMessage,
			Timestamp,
			SpanAttributes
		FROM traces
		WHERE SpanAttributes['correlation_id'] = ? OR TraceId = ?
		ORDER BY Timestamp ASC
	`

	rows, err := c.conn.Query(ctx, query, correlationID, correlationID)
	if err != nil {
		c.logger.Error("Failed to get correlation traces", zap.Error(err), zap.String("correlationId", correlationID))
		return nil, err
	}
	defer rows.Close()

	var traces []CorrelationTrace
	for rows.Next() {
		var t CorrelationTrace
		if err := rows.Scan(
			&t.TraceID,
			&t.SpanID,
			&t.ParentSpanID,
			&t.ServiceName,
			&t.OperationName,
			&t.Duration,
			&t.StatusCode,
			&t.StatusMessage,
			&t.Timestamp,
			&t.Attributes,
		); err != nil {
			c.logger.Error("Failed to scan correlation trace", zap.Error(err))
			continue
		}
		// Normalize status code for UI display
		t.StatusCode = normalizeStatusCode(t.StatusCode)
		traces = append(traces, t)
	}

	return traces, nil
}

// GetCorrelationLogs returns all logs for a correlation ID.
// It matches logs by:
// 1. Direct correlation_id match (MATERIALIZED column extracted from Body JSON)
// 2. TraceId match (OTEL auto-instrumented logs have TraceId from trace context)
func (c *ClickHouseClient) GetCorrelationLogs(ctx context.Context, correlationID string) ([]CorrelationLog, error) {
	// First, get all TraceIds associated with this correlation
	// This allows us to find logs by trace context (zero-code instrumentation)
	traceQuery := `
		SELECT DISTINCT TraceId
		FROM traces
		WHERE SpanAttributes['correlation_id'] = ? AND TraceId != ''
	`
	traceRows, err := c.conn.Query(ctx, traceQuery, correlationID)
	if err != nil {
		c.logger.Error("Failed to get trace IDs for correlation", zap.Error(err))
		return nil, err
	}

	var traceIDs []string
	for traceRows.Next() {
		var traceID string
		if err := traceRows.Scan(&traceID); err == nil && traceID != "" {
			traceIDs = append(traceIDs, traceID)
		}
	}
	traceRows.Close()

	// Build query to match logs by correlation_id (MATERIALIZED column) OR TraceId
	var query string
	var args []interface{}

	if len(traceIDs) > 0 {
		// Match by correlation_id (MATERIALIZED column) OR any of the TraceIds from the correlation
		query = `
			SELECT
				Timestamp,
				if(ServiceName != '', ServiceName, log_service) as ServiceName,
				if(SeverityText != '', SeverityText, log_level) as SeverityText,
				Body,
				TraceId,
				SpanId,
				LogAttributes
			FROM logs
			WHERE correlation_id = ? OR TraceId IN (?)
			ORDER BY Timestamp ASC
			LIMIT 500
		`
		args = []interface{}{correlationID, traceIDs}
	} else {
		// Fallback: just match by correlation_id (MATERIALIZED column)
		query = `
			SELECT
				Timestamp,
				if(ServiceName != '', ServiceName, log_service) as ServiceName,
				if(SeverityText != '', SeverityText, log_level) as SeverityText,
				Body,
				TraceId,
				SpanId,
				LogAttributes
			FROM logs
			WHERE correlation_id = ?
			ORDER BY Timestamp ASC
			LIMIT 500
		`
		args = []interface{}{correlationID}
	}

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to get correlation logs", zap.Error(err), zap.String("correlationId", correlationID))
		return nil, err
	}
	defer rows.Close()

	var logs []CorrelationLog
	for rows.Next() {
		var l CorrelationLog
		if err := rows.Scan(
			&l.Timestamp,
			&l.ServiceName,
			&l.SeverityText,
			&l.Body,
			&l.TraceID,
			&l.SpanID,
			&l.Attributes,
		); err != nil {
			c.logger.Error("Failed to scan correlation log", zap.Error(err))
			continue
		}
		logs = append(logs, l)
	}

	return logs, nil
}

// SearchCorrelations searches for correlations matching criteria.
// Uses correlation_id if available, falls back to TraceId for zero-code instrumented services.
func (c *ClickHouseClient) SearchCorrelations(ctx context.Context, service string, hasErrors bool, start, end time.Time, limit int) ([]CorrelationSummary, error) {
	// Use SpanAttributes['correlation_id'] if set, otherwise fall back to TraceId
	// This supports both explicit correlation IDs and OTEL auto-instrumented traces
	query := `
		SELECT
			if(SpanAttributes['correlation_id'] != '', SpanAttributes['correlation_id'], TraceId) as corr_id,
			min(Timestamp) as first_seen,
			max(Timestamp) as last_seen,
			groupUniqArray(ServiceName) as services,
			count() as span_count,
			countIf(StatusCode = 'STATUS_CODE_ERROR') as error_count,
			argMin(ServiceName, Timestamp) as root_service,
			max(Duration) as max_duration
		FROM traces
		WHERE Timestamp >= ?
			AND Timestamp <= ?
	`
	args := []interface{}{start, end}

	if service != "" {
		query += " AND ServiceName = ?"
		args = append(args, service)
	}

	query += " GROUP BY corr_id"

	if hasErrors {
		query += " HAVING error_count > 0"
	}

	query += " ORDER BY first_seen DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.logger.Error("Failed to search correlations", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var correlations []CorrelationSummary
	for rows.Next() {
		var cs CorrelationSummary
		var services []string
		if err := rows.Scan(
			&cs.CorrelationID,
			&cs.FirstSeen,
			&cs.LastSeen,
			&services,
			&cs.SpanCount,
			&cs.ErrorCount,
			&cs.RootService,
			&cs.MaxDuration,
		); err != nil {
			c.logger.Error("Failed to scan correlation summary", zap.Error(err))
			continue
		}
		cs.Services = services
		cs.HasErrors = cs.ErrorCount > 0
		correlations = append(correlations, cs)
	}

	return correlations, nil
}

// GetCorrelationTimeline builds a timeline of events for a correlation.
func (c *ClickHouseClient) GetCorrelationTimeline(ctx context.Context, correlationID string) ([]CorrelationTimelineItem, error) {
	var timeline []CorrelationTimelineItem

	// Get spans
	spans, err := c.GetCorrelationTraces(ctx, correlationID)
	if err != nil {
		return nil, err
	}

	for _, span := range spans {
		isError := span.StatusCode == "STATUS_CODE_ERROR"
		timeline = append(timeline, CorrelationTimelineItem{
			Timestamp:   span.Timestamp,
			Type:        "span",
			ServiceName: span.ServiceName,
			Description: span.OperationName,
			Duration:    span.Duration,
			IsError:     isError,
		})
	}

	// Get logs
	logs, err := c.GetCorrelationLogs(ctx, correlationID)
	if err != nil {
		return nil, err
	}

	for _, log := range logs {
		isError := log.SeverityText == "ERROR" || log.SeverityText == "FATAL"
		timeline = append(timeline, CorrelationTimelineItem{
			Timestamp:   log.Timestamp,
			Type:        "log",
			ServiceName: log.ServiceName,
			Description: truncateString(log.Body, 100),
			IsError:     isError,
		})
	}

	// Sort by timestamp (simple bubble sort for small arrays)
	for i := 0; i < len(timeline)-1; i++ {
		for j := 0; j < len(timeline)-i-1; j++ {
			if timeline[j].Timestamp.After(timeline[j+1].Timestamp) {
				timeline[j], timeline[j+1] = timeline[j+1], timeline[j]
			}
		}
	}

	return timeline, nil
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
