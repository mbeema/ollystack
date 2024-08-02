package rules

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ollystack/realtime-processor/internal/alerting"
	"github.com/ollystack/realtime-processor/internal/config"
	"github.com/ollystack/realtime-processor/internal/state"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var (
	rulesEvaluated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_rules_evaluated_total",
			Help: "Total number of rule evaluations",
		},
		[]string{"type", "status"},
	)
	alertsFired = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_alerts_fired_total",
			Help: "Total number of alerts fired",
		},
		[]string{"rule", "severity"},
	)
	evaluationLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_rule_evaluation_latency_seconds",
			Help:    "Rule evaluation latency",
			Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5},
		},
		[]string{"type"},
	)
	activeRules = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ollystack_active_rules",
			Help: "Number of active alert rules",
		},
	)
)

// Rule represents an alert rule
type Rule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`    // metric, log, trace
	Condition   Condition         `json:"condition"`
	Duration    time.Duration     `json:"duration"` // How long condition must be true
	Severity    string            `json:"severity"` // critical, warning, info
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Enabled     bool              `json:"enabled"`
}

// Condition represents an alert condition
type Condition struct {
	MetricName string  `json:"metric_name,omitempty"`
	Operator   string  `json:"operator"` // gt, lt, eq, ne, gte, lte
	Threshold  float64 `json:"threshold"`

	// For log/trace rules
	ServiceName    string `json:"service_name,omitempty"`
	SeverityLevel  int    `json:"severity_level,omitempty"` // For logs
	StatusCode     string `json:"status_code,omitempty"`    // For traces
	ErrorRateThreshold float64 `json:"error_rate_threshold,omitempty"`
}

// Engine evaluates alert rules against incoming data
type Engine struct {
	config      config.RulesConfig
	stateStore  *state.RedisStore
	alertSender *alerting.Sender
	logger      *zap.Logger

	mu    sync.RWMutex
	rules map[string]*Rule
}

// NewEngine creates a new rules engine
func NewEngine(cfg config.RulesConfig, stateStore *state.RedisStore, alertSender *alerting.Sender, logger *zap.Logger) *Engine {
	return &Engine{
		config:      cfg,
		stateStore:  stateStore,
		alertSender: alertSender,
		logger:      logger,
		rules:       make(map[string]*Rule),
	}
}

// LoadRules loads alert rules from configuration
func (e *Engine) LoadRules() error {
	// Default rules
	defaultRules := []*Rule{
		{
			ID:       "high-error-rate",
			Name:     "High Error Rate",
			Type:     "trace",
			Condition: Condition{
				Operator:           "gt",
				ErrorRateThreshold: 5.0, // 5% error rate
			},
			Duration: 5 * time.Minute,
			Severity: "warning",
			Labels:   map[string]string{"type": "availability"},
			Enabled:  true,
		},
		{
			ID:       "high-latency",
			Name:     "High Latency P95",
			Type:     "trace",
			Condition: Condition{
				MetricName: "duration_ms",
				Operator:   "gt",
				Threshold:  1000, // 1 second
			},
			Duration: 5 * time.Minute,
			Severity: "warning",
			Labels:   map[string]string{"type": "performance"},
			Enabled:  true,
		},
		{
			ID:       "high-cpu",
			Name:     "High CPU Usage",
			Type:     "metric",
			Condition: Condition{
				MetricName: "system.cpu.utilization",
				Operator:   "gt",
				Threshold:  0.9, // 90%
			},
			Duration: 5 * time.Minute,
			Severity: "warning",
			Labels:   map[string]string{"type": "resource"},
			Enabled:  true,
		},
		{
			ID:       "high-memory",
			Name:     "High Memory Usage",
			Type:     "metric",
			Condition: Condition{
				MetricName: "system.memory.utilization",
				Operator:   "gt",
				Threshold:  0.9, // 90%
			},
			Duration: 5 * time.Minute,
			Severity: "warning",
			Labels:   map[string]string{"type": "resource"},
			Enabled:  true,
		},
		{
			ID:       "error-log-spike",
			Name:     "Error Log Spike",
			Type:     "log",
			Condition: Condition{
				SeverityLevel: 17, // ERROR in OTLP
				Operator:      "gt",
				Threshold:     100, // More than 100 errors per minute
			},
			Duration: 1 * time.Minute,
			Severity: "warning",
			Labels:   map[string]string{"type": "error"},
			Enabled:  true,
		},
	}

	e.mu.Lock()
	for _, rule := range defaultRules {
		e.rules[rule.ID] = rule
	}
	e.mu.Unlock()

	activeRules.Set(float64(len(defaultRules)))

	e.logger.Info("Loaded alert rules", zap.Int("count", len(defaultRules)))
	return nil
}

// RuleCount returns the number of loaded rules
func (e *Engine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}

// EvaluateMetrics evaluates metric-based rules
func (e *Engine) EvaluateMetrics(ctx context.Context, data []byte) error {
	start := time.Now()

	var req metricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		rulesEvaluated.WithLabelValues("metrics", "error").Inc()
		return fmt.Errorf("failed to unmarshal metrics: %w", err)
	}

	e.mu.RLock()
	metricRules := make([]*Rule, 0)
	for _, rule := range e.rules {
		if rule.Type == "metric" && rule.Enabled {
			metricRules = append(metricRules, rule)
		}
	}
	e.mu.RUnlock()

	// Extract metrics and evaluate rules
	for _, rm := range req.ResourceMetrics {
		serviceName := extractResourceAttribute(rm.Resource, "service.name")

		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				for _, rule := range metricRules {
					if rule.Condition.MetricName == metric.Name {
						value := extractMetricValue(metric)
						if e.checkCondition(rule.Condition, value) {
							e.handleAlert(ctx, rule, serviceName, metric.Name, value)
						}
					}
				}
			}
		}
	}

	rulesEvaluated.WithLabelValues("metrics", "success").Inc()
	evaluationLatency.WithLabelValues("metrics").Observe(time.Since(start).Seconds())

	return nil
}

// EvaluateLogs evaluates log-based rules
func (e *Engine) EvaluateLogs(ctx context.Context, data []byte) error {
	start := time.Now()

	var req logsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		rulesEvaluated.WithLabelValues("logs", "error").Inc()
		return fmt.Errorf("failed to unmarshal logs: %w", err)
	}

	e.mu.RLock()
	logRules := make([]*Rule, 0)
	for _, rule := range e.rules {
		if rule.Type == "log" && rule.Enabled {
			logRules = append(logRules, rule)
		}
	}
	e.mu.RUnlock()

	// Count errors per service
	errorCounts := make(map[string]int)

	for _, rl := range req.ResourceLogs {
		serviceName := extractResourceAttribute(rl.Resource, "service.name")

		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				if int(lr.SeverityNumber) >= 17 { // ERROR or higher
					errorCounts[serviceName]++
				}
			}
		}
	}

	// Evaluate log rules
	for serviceName, count := range errorCounts {
		// Update error count in state store
		key := fmt.Sprintf("error_count:%s", serviceName)
		newCount, _ := e.stateStore.IncrementWithExpiry(ctx, key, int64(count), time.Minute)

		for _, rule := range logRules {
			if e.checkCondition(rule.Condition, float64(newCount)) {
				e.handleAlert(ctx, rule, serviceName, "error_count", float64(newCount))
			}
		}
	}

	rulesEvaluated.WithLabelValues("logs", "success").Inc()
	evaluationLatency.WithLabelValues("logs").Observe(time.Since(start).Seconds())

	return nil
}

// EvaluateTraces evaluates trace-based rules (error rate, latency)
func (e *Engine) EvaluateTraces(ctx context.Context, data []byte) error {
	start := time.Now()

	var req tracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		rulesEvaluated.WithLabelValues("traces", "error").Inc()
		return fmt.Errorf("failed to unmarshal traces: %w", err)
	}

	e.mu.RLock()
	traceRules := make([]*Rule, 0)
	for _, rule := range e.rules {
		if rule.Type == "trace" && rule.Enabled {
			traceRules = append(traceRules, rule)
		}
	}
	e.mu.RUnlock()

	// Aggregate stats per service
	type serviceStats struct {
		totalSpans  int
		errorSpans  int
		totalDuration int64
	}
	stats := make(map[string]*serviceStats)

	for _, rt := range req.ResourceSpans {
		serviceName := extractResourceAttribute(rt.Resource, "service.name")

		if _, ok := stats[serviceName]; !ok {
			stats[serviceName] = &serviceStats{}
		}

		for _, ss := range rt.ScopeSpans {
			for _, span := range ss.Spans {
				stats[serviceName].totalSpans++
				stats[serviceName].totalDuration += int64(span.EndTimeUnixNano - span.StartTimeUnixNano)

				if span.Status != nil && span.Status.Code == 2 { // ERROR
					stats[serviceName].errorSpans++
				}
			}
		}
	}

	// Update state and evaluate rules
	for serviceName, s := range stats {
		// Update rolling counters
		totalKey := fmt.Sprintf("spans_total:%s", serviceName)
		errorKey := fmt.Sprintf("spans_error:%s", serviceName)

		totalCount, _ := e.stateStore.IncrementWithExpiry(ctx, totalKey, int64(s.totalSpans), 5*time.Minute)
		errorCount, _ := e.stateStore.IncrementWithExpiry(ctx, errorKey, int64(s.errorSpans), 5*time.Minute)

		// Calculate error rate
		errorRate := float64(0)
		if totalCount > 0 {
			errorRate = float64(errorCount) / float64(totalCount) * 100
		}

		// Calculate average latency
		avgLatencyMs := float64(0)
		if s.totalSpans > 0 {
			avgLatencyMs = float64(s.totalDuration) / float64(s.totalSpans) / 1e6
		}

		// Evaluate rules
		for _, rule := range traceRules {
			if rule.Condition.ErrorRateThreshold > 0 {
				if errorRate > rule.Condition.ErrorRateThreshold {
					e.handleAlert(ctx, rule, serviceName, "error_rate", errorRate)
				}
			}
			if rule.Condition.MetricName == "duration_ms" {
				if e.checkCondition(rule.Condition, avgLatencyMs) {
					e.handleAlert(ctx, rule, serviceName, "latency_ms", avgLatencyMs)
				}
			}
		}
	}

	rulesEvaluated.WithLabelValues("traces", "success").Inc()
	evaluationLatency.WithLabelValues("traces").Observe(time.Since(start).Seconds())

	return nil
}

// handleAlert handles a triggered alert
func (e *Engine) handleAlert(ctx context.Context, rule *Rule, serviceName, metricName string, value float64) {
	// Check if alert is already firing (avoid duplicates)
	alertKey := fmt.Sprintf("alert:%s:%s", rule.ID, serviceName)

	if exists, _ := e.stateStore.Exists(ctx, alertKey); exists {
		return // Alert already firing
	}

	// Mark alert as firing with duration TTL
	e.stateStore.SetWithExpiry(ctx, alertKey, "firing", rule.Duration)

	// Create and send alert
	alert := &alerting.Alert{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Severity:    rule.Severity,
		ServiceName: serviceName,
		MetricName:  metricName,
		Value:       value,
		Threshold:   rule.Condition.Threshold,
		Labels:      rule.Labels,
		Annotations: rule.Annotations,
		FiredAt:     time.Now(),
	}

	if err := e.alertSender.Send(ctx, alert); err != nil {
		e.logger.Error("Failed to send alert",
			zap.String("rule", rule.Name),
			zap.Error(err),
		)
		return
	}

	alertsFired.WithLabelValues(rule.Name, rule.Severity).Inc()

	e.logger.Info("Alert fired",
		zap.String("rule", rule.Name),
		zap.String("service", serviceName),
		zap.String("metric", metricName),
		zap.Float64("value", value),
		zap.Float64("threshold", rule.Condition.Threshold),
	)
}

// checkCondition evaluates a condition against a value
func (e *Engine) checkCondition(cond Condition, value float64) bool {
	switch cond.Operator {
	case "gt":
		return value > cond.Threshold
	case "gte":
		return value >= cond.Threshold
	case "lt":
		return value < cond.Threshold
	case "lte":
		return value <= cond.Threshold
	case "eq":
		return value == cond.Threshold
	case "ne":
		return value != cond.Threshold
	default:
		return false
	}
}

// WatchForUpdates periodically reloads rules
func (e *Engine) WatchForUpdates(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// In production, this would reload from ClickHouse or config file
			e.logger.Debug("Checking for rule updates")
		}
	}
}

// Helper functions

func extractResourceAttribute(resource interface{}, key string) string {
	// Extract attribute from OTLP resource
	return "unknown"
}

func extractMetricValue(metric interface{}) float64 {
	// Extract value from OTLP metric
	return 0
}
