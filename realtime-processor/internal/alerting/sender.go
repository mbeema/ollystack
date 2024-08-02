package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ollystack/realtime-processor/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	alertsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_alerts_sent_total",
			Help: "Total number of alerts sent",
		},
		[]string{"destination", "status"},
	)
	alertLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_alert_send_latency_seconds",
			Help:    "Alert sending latency",
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"destination"},
	)
)

// Alert represents an alert to be sent
type Alert struct {
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	Severity    string            `json:"severity"`
	ServiceName string            `json:"service_name"`
	MetricName  string            `json:"metric_name"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	FiredAt     time.Time         `json:"fired_at"`
}

// AlertManagerAlert is the Alertmanager API format
type AlertManagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt,omitempty"`
	GeneratorURL string           `json:"generatorURL,omitempty"`
}

// Sender sends alerts to various destinations
type Sender struct {
	config     config.AlertingConfig
	httpClient *http.Client
	logger     *zap.Logger
}

// NewSender creates a new alert sender
func NewSender(cfg config.AlertingConfig, logger *zap.Logger) *Sender {
	return &Sender{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Send sends an alert to all configured destinations
func (s *Sender) Send(ctx context.Context, alert *Alert) error {
	var lastErr error

	// Send to AlertManager
	if s.config.AlertManagerURL != "" {
		if err := s.sendToAlertManager(ctx, alert); err != nil {
			s.logger.Error("Failed to send to AlertManager", zap.Error(err))
			lastErr = err
		}
	}

	// Send to webhook
	if s.config.WebhookURL != "" {
		if err := s.sendToWebhook(ctx, alert); err != nil {
			s.logger.Error("Failed to send to webhook", zap.Error(err))
			lastErr = err
		}
	}

	// Send to Slack
	if s.config.SlackWebhook != "" {
		if err := s.sendToSlack(ctx, alert); err != nil {
			s.logger.Error("Failed to send to Slack", zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

func (s *Sender) sendToAlertManager(ctx context.Context, alert *Alert) error {
	start := time.Now()

	// Build AlertManager alert
	labels := map[string]string{
		"alertname": alert.RuleName,
		"severity":  alert.Severity,
		"service":   alert.ServiceName,
		"source":    "ollystack",
	}
	for k, v := range alert.Labels {
		labels[k] = v
	}
	for k, v := range s.config.Labels {
		labels[k] = v
	}

	annotations := map[string]string{
		"summary":     fmt.Sprintf("%s: %s", alert.RuleName, alert.ServiceName),
		"description": fmt.Sprintf("%s is %.2f (threshold: %.2f)", alert.MetricName, alert.Value, alert.Threshold),
	}
	for k, v := range alert.Annotations {
		annotations[k] = v
	}

	amAlert := AlertManagerAlert{
		Labels:      labels,
		Annotations: annotations,
		StartsAt:    alert.FiredAt,
	}

	// Send to AlertManager
	body, err := json.Marshal([]AlertManagerAlert{amAlert})
	if err != nil {
		alertsSent.WithLabelValues("alertmanager", "error").Inc()
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	url := fmt.Sprintf("%s/api/v2/alerts", s.config.AlertManagerURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		alertsSent.WithLabelValues("alertmanager", "error").Inc()
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		alertsSent.WithLabelValues("alertmanager", "error").Inc()
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		alertsSent.WithLabelValues("alertmanager", "error").Inc()
		return fmt.Errorf("AlertManager returned status %d", resp.StatusCode)
	}

	alertsSent.WithLabelValues("alertmanager", "success").Inc()
	alertLatency.WithLabelValues("alertmanager").Observe(time.Since(start).Seconds())

	return nil
}

func (s *Sender) sendToWebhook(ctx context.Context, alert *Alert) error {
	start := time.Now()

	body, err := json.Marshal(alert)
	if err != nil {
		alertsSent.WithLabelValues("webhook", "error").Inc()
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		alertsSent.WithLabelValues("webhook", "error").Inc()
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		alertsSent.WithLabelValues("webhook", "error").Inc()
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		alertsSent.WithLabelValues("webhook", "error").Inc()
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	alertsSent.WithLabelValues("webhook", "success").Inc()
	alertLatency.WithLabelValues("webhook").Observe(time.Since(start).Seconds())

	return nil
}

func (s *Sender) sendToSlack(ctx context.Context, alert *Alert) error {
	start := time.Now()

	// Build Slack message
	color := "#36a64f" // green
	switch alert.Severity {
	case "critical":
		color = "#ff0000"
	case "warning":
		color = "#ffcc00"
	}

	slackMsg := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":      color,
				"title":      fmt.Sprintf("🚨 %s", alert.RuleName),
				"text":       fmt.Sprintf("Service: %s\nMetric: %s\nValue: %.2f (threshold: %.2f)", alert.ServiceName, alert.MetricName, alert.Value, alert.Threshold),
				"footer":     "OllyStack",
				"ts":         alert.FiredAt.Unix(),
			},
		},
	}

	body, err := json.Marshal(slackMsg)
	if err != nil {
		alertsSent.WithLabelValues("slack", "error").Inc()
		return fmt.Errorf("failed to marshal slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.SlackWebhook, bytes.NewReader(body))
	if err != nil {
		alertsSent.WithLabelValues("slack", "error").Inc()
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		alertsSent.WithLabelValues("slack", "error").Inc()
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		alertsSent.WithLabelValues("slack", "error").Inc()
		return fmt.Errorf("Slack returned status %d", resp.StatusCode)
	}

	alertsSent.WithLabelValues("slack", "success").Inc()
	alertLatency.WithLabelValues("slack").Observe(time.Since(start).Seconds())

	return nil
}
