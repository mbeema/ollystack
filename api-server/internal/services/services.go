// Package services provides business logic and data access.
package services

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/mbeema/ollystack/api-server/internal/config"
	"github.com/mbeema/ollystack/api-server/internal/storage"
	"go.uber.org/zap"
)

// Services holds all service instances.
type Services struct {
	config      *config.Config
	logger      *zap.Logger
	clickhouse  *storage.ClickHouseClient
	Metrics     *MetricsService
	Logs        *LogsService
	Traces      *TracesService
	Topology    *TopologyService
	Alerts      *AlertsService
	Dashboards  *DashboardsService
	Query       *QueryService
	AI          *AIService
	PubSub      *PubSubService
	Correlation *CorrelationService
}

// New creates a new Services instance.
func New(cfg *config.Config, logger *zap.Logger) (*Services, error) {
	s := &Services{
		config: cfg,
		logger: logger,
	}

	// Initialize ClickHouse client
	chCfg := parseClickHouseEndpoint(cfg.Storage.Traces.Endpoint, cfg.Storage.Traces.Database, cfg.Storage.Traces.Username, cfg.Storage.Traces.Password)
	ch, err := storage.NewClickHouseClient(chCfg, logger)
	if err != nil {
		logger.Warn("Failed to connect to ClickHouse, traces will be unavailable", zap.Error(err))
	} else {
		s.clickhouse = ch
	}

	s.Metrics = NewMetricsService(cfg, logger, s.clickhouse)
	s.Logs = NewLogsService(cfg, logger, s.clickhouse)
	s.Traces = NewTracesService(cfg, logger, s.clickhouse)
	s.Topology = NewTopologyService(cfg, logger, s.clickhouse)
	s.Alerts = NewAlertsService(cfg, logger)
	s.Dashboards = NewDashboardsService(cfg, logger)
	s.Query = NewQueryService(cfg, logger)
	s.AI = NewAIService(cfg, logger)
	s.PubSub = NewPubSubService()
	s.Correlation = NewCorrelationService(cfg, logger, s.clickhouse)

	return s, nil
}

// parseClickHouseEndpoint parses endpoint like "clickhouse:9000" into config
func parseClickHouseEndpoint(endpoint, database, username, password string) storage.ClickHouseConfig {
	host := "localhost"
	port := 9000

	// Parse host:port from endpoint
	if endpoint != "" {
		// Remove tcp:// prefix if present
		endpoint = strings.TrimPrefix(endpoint, "tcp://")
		h, p, err := net.SplitHostPort(endpoint)
		if err == nil {
			host = h
			if pInt, err := strconv.Atoi(p); err == nil {
				port = pInt
			}
		} else {
			host = endpoint
		}
	}

	return storage.ClickHouseConfig{
		Host:     host,
		Port:     port,
		Database: database,
		Username: username,
		Password: password,
	}
}

// HealthCheck checks the health of all services.
func (s *Services) HealthCheck(ctx context.Context) error {
	if s.clickhouse != nil {
		if err := s.clickhouse.Conn().Ping(ctx); err != nil {
			return fmt.Errorf("clickhouse: %w", err)
		}
	}
	return nil
}

// Close closes all service connections.
func (s *Services) Close() error {
	if s.clickhouse != nil {
		return s.clickhouse.Close()
	}
	return nil
}

// TraceListParams holds parameters for listing traces.
type TraceListParams struct {
	Service     string
	Operation   string
	Tags        []string
	MinDuration time.Duration
	MaxDuration time.Duration
	Start       time.Time
	End         time.Time
	Limit       int
}

// TracesService provides trace-related operations.
type TracesService struct {
	config     *config.Config
	logger     *zap.Logger
	clickhouse *storage.ClickHouseClient
}

func NewTracesService(cfg *config.Config, logger *zap.Logger, ch *storage.ClickHouseClient) *TracesService {
	return &TracesService{config: cfg, logger: logger, clickhouse: ch}
}

func (s *TracesService) List(ctx context.Context, params TraceListParams) ([]interface{}, error) {
	if s.clickhouse == nil {
		s.logger.Warn("ClickHouse not connected, returning empty traces")
		return []interface{}{}, nil
	}

	traces, err := s.clickhouse.ListTraces(ctx, params.Service, params.MinDuration, params.Start, params.End, params.Limit)
	if err != nil {
		s.logger.Error("Failed to list traces", zap.Error(err))
		return nil, err
	}

	// Convert to interface slice
	result := make([]interface{}, len(traces))
	for i, t := range traces {
		result[i] = map[string]interface{}{
			"traceId":       t.TraceId,
			"serviceName":   t.ServiceName,
			"operationName": t.OperationName,
			"duration":      t.Duration,
			"spanCount":     t.SpanCount,
			"status":        t.Status,
			"timestamp":     t.Timestamp,
		}
	}

	return result, nil
}

func (s *TracesService) Get(ctx context.Context, traceID string) (interface{}, error) {
	if s.clickhouse == nil {
		return nil, fmt.Errorf("clickhouse not connected")
	}

	spans, err := s.clickhouse.GetTraceSpans(ctx, traceID)
	if err != nil {
		return nil, err
	}

	if len(spans) == 0 {
		return nil, fmt.Errorf("trace not found")
	}

	// Convert spans to response format
	spanList := make([]map[string]interface{}, len(spans))
	services := make(map[string]bool)
	var hasError bool
	var minTime, maxTime time.Time

	for i, span := range spans {
		services[span.ServiceName] = true
		if span.StatusCode == "STATUS_CODE_ERROR" {
			hasError = true
		}

		if minTime.IsZero() || span.Timestamp.Before(minTime) {
			minTime = span.Timestamp
		}
		endTime := span.Timestamp.Add(time.Duration(span.Duration))
		if maxTime.IsZero() || endTime.After(maxTime) {
			maxTime = endTime
		}

		spanList[i] = map[string]interface{}{
			"spanId":        span.SpanId,
			"traceId":       span.TraceId,
			"parentSpanId":  span.ParentSpanId,
			"operationName": span.SpanName,
			"serviceName":   span.ServiceName,
			"startTime":     span.Timestamp,
			"duration":      span.Duration,
			"status":        span.StatusCode,
			"kind":          span.SpanKind,
			"attributes":    span.SpanAttributes,
		}
	}

	serviceList := make([]string, 0, len(services))
	for svc := range services {
		serviceList = append(serviceList, svc)
	}

	return map[string]interface{}{
		"traceId":   traceID,
		"spans":     spanList,
		"services":  serviceList,
		"spanCount": len(spans),
		"hasError":  hasError,
		"startTime": minTime,
		"duration":  maxTime.Sub(minTime).Nanoseconds(),
	}, nil
}

func (s *TracesService) GetSpans(ctx context.Context, traceID string) ([]interface{}, error) {
	if s.clickhouse == nil {
		return []interface{}{}, nil
	}

	spans, err := s.clickhouse.GetTraceSpans(ctx, traceID)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(spans))
	for i, span := range spans {
		result[i] = map[string]interface{}{
			"spanId":        span.SpanId,
			"traceId":       span.TraceId,
			"parentSpanId":  span.ParentSpanId,
			"operationName": span.SpanName,
			"serviceName":   span.ServiceName,
			"startTime":     span.Timestamp,
			"duration":      span.Duration,
			"status":        span.StatusCode,
			"kind":          span.SpanKind,
			"attributes":    span.SpanAttributes,
		}
	}

	return result, nil
}

func (s *TracesService) Search(ctx context.Context, query string, limit int) ([]interface{}, error) {
	// TODO: Implement search with query parsing
	return []interface{}{}, nil
}

// MetricsService provides metrics-related operations.
type MetricsService struct {
	config     *config.Config
	logger     *zap.Logger
	clickhouse *storage.ClickHouseClient
}

func NewMetricsService(cfg *config.Config, logger *zap.Logger, ch *storage.ClickHouseClient) *MetricsService {
	return &MetricsService{config: cfg, logger: logger, clickhouse: ch}
}

func (s *MetricsService) Query(ctx context.Context, query string) (interface{}, error) {
	if s.clickhouse == nil {
		return map[string]interface{}{"metrics": []interface{}{}, "total": 0}, nil
	}

	end := time.Now()
	start := end.Add(-1 * time.Hour)

	metrics, err := s.clickhouse.QueryMetrics(ctx, query, start, end, 100)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"metrics": metrics,
		"total":   len(metrics),
	}, nil
}

func (s *MetricsService) QueryRange(ctx context.Context, query, startStr, endStr, step string) (interface{}, error) {
	if s.clickhouse == nil {
		return map[string]interface{}{"metrics": []interface{}{}, "total": 0}, nil
	}

	end := time.Now()
	start := end.Add(-1 * time.Hour)

	// Parse start/end if provided
	if startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}
	if endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}

	metrics, err := s.clickhouse.QueryMetrics(ctx, query, start, end, 1000)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"metrics": metrics,
		"total":   len(metrics),
	}, nil
}

func (s *MetricsService) ListLabels(ctx context.Context) ([]string, error) {
	if s.clickhouse == nil {
		return []string{}, nil
	}
	return s.clickhouse.GetMetricNames(ctx)
}

func (s *MetricsService) ListLabelValues(ctx context.Context, name string) ([]string, error) {
	return []string{}, nil
}

func (s *MetricsService) ListSeries(ctx context.Context, match []string) ([]interface{}, error) {
	return []interface{}{}, nil
}

// LogsService provides logs-related operations.
type LogsService struct {
	config     *config.Config
	logger     *zap.Logger
	clickhouse *storage.ClickHouseClient
}

func NewLogsService(cfg *config.Config, logger *zap.Logger, ch *storage.ClickHouseClient) *LogsService {
	return &LogsService{config: cfg, logger: logger, clickhouse: ch}
}

func (s *LogsService) Query(ctx context.Context, query, limit, direction string) (interface{}, error) {
	if s.clickhouse == nil {
		return map[string]interface{}{"logs": []interface{}{}, "total": 0}, nil
	}

	limitInt := 100
	if limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			limitInt = l
		}
	}

	end := time.Now()
	start := end.Add(-24 * time.Hour)

	logs, err := s.clickhouse.QueryLogs(ctx, "", "", query, start, end, limitInt)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	}, nil
}

func (s *LogsService) Tail(ctx context.Context, query string, callback func(interface{})) {
	// Stream logs - TODO: implement with polling
}

func (s *LogsService) ListLabels(ctx context.Context) ([]string, error) {
	if s.clickhouse == nil {
		return []string{}, nil
	}
	return s.clickhouse.GetLogServices(ctx)
}

// TopologyService provides topology-related operations.
type TopologyService struct {
	config     *config.Config
	logger     *zap.Logger
	clickhouse *storage.ClickHouseClient
}

func NewTopologyService(cfg *config.Config, logger *zap.Logger, ch *storage.ClickHouseClient) *TopologyService {
	return &TopologyService{config: cfg, logger: logger, clickhouse: ch}
}

func (s *TopologyService) ListServices(ctx context.Context) ([]interface{}, error) {
	if s.clickhouse == nil {
		return []interface{}{}, nil
	}

	services, err := s.clickhouse.GetServices(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(services))
	for i, svc := range services {
		result[i] = map[string]interface{}{
			"name": svc,
		}
	}
	return result, nil
}

func (s *TopologyService) ListEdges(ctx context.Context) ([]interface{}, error) {
	if s.clickhouse == nil {
		return []interface{}{}, nil
	}

	edges, err := s.clickhouse.GetServiceEdges(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(edges))
	for i, e := range edges {
		result[i] = map[string]interface{}{
			"source":       e.Source,
			"target":       e.Target,
			"requestCount": e.RequestCount,
			"errorRate":    e.ErrorRate,
			"avgLatency":   e.AvgLatencyMs,
			"p95Latency":   e.P95LatencyMs,
		}
	}
	return result, nil
}

func (s *TopologyService) GetGraph(ctx context.Context) (interface{}, error) {
	if s.clickhouse == nil {
		return map[string]interface{}{
			"services": map[string]interface{}{},
			"edges":    map[string]interface{}{},
		}, nil
	}

	// Get service statistics
	stats, err := s.clickhouse.GetServiceStats(ctx)
	if err != nil {
		s.logger.Warn("Failed to get service stats", zap.Error(err))
		stats = []storage.ServiceStats{}
	}

	// Get service-to-service edges
	edges, err := s.clickhouse.GetServiceEdges(ctx)
	if err != nil {
		s.logger.Warn("Failed to get service edges", zap.Error(err))
		edges = []storage.ServiceEdge{}
	}

	// Build services map
	svcMap := make(map[string]interface{})
	for _, stat := range stats {
		svcMap[stat.Name] = map[string]interface{}{
			"name":        stat.Name,
			"type":        stat.Type,
			"avgLatency":  stat.AvgLatencyMs,
			"p50Latency":  stat.P50LatencyMs,
			"p95Latency":  stat.P95LatencyMs,
			"p99Latency":  stat.P99LatencyMs,
			"errorRate":   stat.ErrorRate,
			"requestRate": stat.RequestsPerSec,
			"totalSpans":  stat.TotalSpans,
		}
	}

	// Build edges map with unique key
	edgeMap := make(map[string]interface{})
	for _, edge := range edges {
		key := fmt.Sprintf("%s->%s", edge.Source, edge.Target)
		edgeMap[key] = map[string]interface{}{
			"source":       edge.Source,
			"target":       edge.Target,
			"requestCount": edge.RequestCount,
			"errorRate":    edge.ErrorRate,
			"avgLatency":   edge.AvgLatencyMs,
			"p95Latency":   edge.P95LatencyMs,
		}
	}

	return map[string]interface{}{
		"services": svcMap,
		"edges":    edgeMap,
	}, nil
}

// Alert represents an alert rule.
type Alert struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Query       string            `json:"query"`
	Condition   string            `json:"condition"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// AlertsService provides alert-related operations.
type AlertsService struct {
	config *config.Config
	logger *zap.Logger
}

func NewAlertsService(cfg *config.Config, logger *zap.Logger) *AlertsService {
	return &AlertsService{config: cfg, logger: logger}
}

func (s *AlertsService) List(ctx context.Context) ([]*Alert, error) {
	return []*Alert{}, nil
}

func (s *AlertsService) Create(ctx context.Context, alert *Alert) (*Alert, error) {
	return alert, nil
}

func (s *AlertsService) Get(ctx context.Context, id string) (*Alert, error) {
	return nil, nil
}

func (s *AlertsService) Update(ctx context.Context, id string, alert *Alert) (*Alert, error) {
	return alert, nil
}

func (s *AlertsService) Delete(ctx context.Context, id string) error {
	return nil
}

// Dashboard represents a dashboard.
type Dashboard struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Panels      []interface{} `json:"panels"`
	Variables   []interface{} `json:"variables"`
	TimeRange   interface{}   `json:"timeRange"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// DashboardsService provides dashboard-related operations.
type DashboardsService struct {
	config *config.Config
	logger *zap.Logger
}

func NewDashboardsService(cfg *config.Config, logger *zap.Logger) *DashboardsService {
	return &DashboardsService{config: cfg, logger: logger}
}

func (s *DashboardsService) List(ctx context.Context) ([]*Dashboard, error) {
	return []*Dashboard{}, nil
}

func (s *DashboardsService) Create(ctx context.Context, dashboard *Dashboard) (*Dashboard, error) {
	return dashboard, nil
}

func (s *DashboardsService) Get(ctx context.Context, id string) (*Dashboard, error) {
	return nil, nil
}

func (s *DashboardsService) Update(ctx context.Context, id string, dashboard *Dashboard) (*Dashboard, error) {
	return dashboard, nil
}

func (s *DashboardsService) Delete(ctx context.Context, id string) error {
	return nil
}

// QueryService provides query execution.
type QueryService struct {
	config *config.Config
	logger *zap.Logger
}

func NewQueryService(cfg *config.Config, logger *zap.Logger) *QueryService {
	return &QueryService{config: cfg, logger: logger}
}

func (s *QueryService) Execute(ctx context.Context, query string) (interface{}, error) {
	return nil, nil
}

// AIService provides AI-powered features.
type AIService struct {
	config *config.Config
	logger *zap.Logger
}

func NewAIService(cfg *config.Config, logger *zap.Logger) *AIService {
	return &AIService{config: cfg, logger: logger}
}

func (s *AIService) Ask(ctx context.Context, question string) (interface{}, error) {
	return nil, nil
}

func (s *AIService) AnalyzeAnomaly(ctx context.Context, traceID, metric string) (interface{}, error) {
	return nil, nil
}

func (s *AIService) GetSuggestions(ctx context.Context, context string) ([]string, error) {
	return []string{}, nil
}

// PubSubService provides publish-subscribe functionality.
type PubSubService struct {
	subscribers map[string][]func(interface{})
}

func NewPubSubService() *PubSubService {
	return &PubSubService{
		subscribers: make(map[string][]func(interface{})),
	}
}

func (s *PubSubService) Subscribe(channel string, callback func(interface{})) {
	s.subscribers[channel] = append(s.subscribers[channel], callback)
}

func (s *PubSubService) Unsubscribe(channel string) {
	delete(s.subscribers, channel)
}

func (s *PubSubService) Publish(channel string, data interface{}) {
	for _, callback := range s.subscribers[channel] {
		go callback(data)
	}
}

// ================================================================================
// CORRELATION SERVICE
// ================================================================================

// CorrelationService provides correlation-related operations.
type CorrelationService struct {
	config     *config.Config
	logger     *zap.Logger
	clickhouse *storage.ClickHouseClient
}

// NewCorrelationService creates a new correlation service.
func NewCorrelationService(cfg *config.Config, logger *zap.Logger, ch *storage.ClickHouseClient) *CorrelationService {
	return &CorrelationService{
		config:     cfg,
		logger:     logger,
		clickhouse: ch,
	}
}

// CorrelationSearchParams holds parameters for searching correlations.
type CorrelationSearchParams struct {
	Service   string
	HasErrors bool
	Start     time.Time
	End       time.Time
	Limit     int
}

// Get returns the full context for a correlation ID.
func (s *CorrelationService) Get(ctx context.Context, correlationID string) (*storage.CorrelationContext, error) {
	if s.clickhouse == nil {
		return nil, fmt.Errorf("clickhouse not connected")
	}

	corr, err := s.clickhouse.GetCorrelation(ctx, correlationID)
	if err != nil {
		s.logger.Error("Failed to get correlation", zap.Error(err), zap.String("correlationId", correlationID))
		return nil, err
	}

	return corr, nil
}

// GetWithDetails returns correlation with all traces, logs, and timeline.
func (s *CorrelationService) GetWithDetails(ctx context.Context, correlationID string) (*storage.CorrelationContext, error) {
	if s.clickhouse == nil {
		return nil, fmt.Errorf("clickhouse not connected")
	}

	// Get basic context
	corr, err := s.clickhouse.GetCorrelation(ctx, correlationID)
	if err != nil {
		return nil, err
	}

	// Get traces
	traces, err := s.clickhouse.GetCorrelationTraces(ctx, correlationID)
	if err != nil {
		s.logger.Warn("Failed to get correlation traces", zap.Error(err))
	} else {
		corr.Traces = traces
	}

	// Get logs
	logs, err := s.clickhouse.GetCorrelationLogs(ctx, correlationID)
	if err != nil {
		s.logger.Warn("Failed to get correlation logs", zap.Error(err))
	} else {
		corr.Logs = logs
	}

	// Get timeline
	timeline, err := s.clickhouse.GetCorrelationTimeline(ctx, correlationID)
	if err != nil {
		s.logger.Warn("Failed to get correlation timeline", zap.Error(err))
	} else {
		corr.Timeline = timeline
	}

	return corr, nil
}

// GetTraces returns traces for a correlation ID.
func (s *CorrelationService) GetTraces(ctx context.Context, correlationID string) ([]storage.CorrelationTrace, error) {
	if s.clickhouse == nil {
		return []storage.CorrelationTrace{}, nil
	}

	return s.clickhouse.GetCorrelationTraces(ctx, correlationID)
}

// GetLogs returns logs for a correlation ID.
func (s *CorrelationService) GetLogs(ctx context.Context, correlationID string) ([]storage.CorrelationLog, error) {
	if s.clickhouse == nil {
		return []storage.CorrelationLog{}, nil
	}

	return s.clickhouse.GetCorrelationLogs(ctx, correlationID)
}

// GetTimeline returns timeline for a correlation ID.
func (s *CorrelationService) GetTimeline(ctx context.Context, correlationID string) ([]storage.CorrelationTimelineItem, error) {
	if s.clickhouse == nil {
		return []storage.CorrelationTimelineItem{}, nil
	}

	return s.clickhouse.GetCorrelationTimeline(ctx, correlationID)
}

// Search searches for correlations matching criteria.
func (s *CorrelationService) Search(ctx context.Context, params CorrelationSearchParams) ([]storage.CorrelationSummary, error) {
	if s.clickhouse == nil {
		return []storage.CorrelationSummary{}, nil
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.End.IsZero() {
		params.End = time.Now()
	}
	if params.Start.IsZero() {
		params.Start = params.End.Add(-24 * time.Hour)
	}

	return s.clickhouse.SearchCorrelations(ctx, params.Service, params.HasErrors, params.Start, params.End, params.Limit)
}
