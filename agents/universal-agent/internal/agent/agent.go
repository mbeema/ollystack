// Package agent provides the core agent implementation that orchestrates
// all collectors and exporters.
package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/collectors"
	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"github.com/ollystack/ollystack/agents/universal-agent/internal/exporters"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.uber.org/zap"
)

// Agent is the main agent that coordinates collectors and exporters.
type Agent struct {
	cfg    *config.Config
	logger *zap.Logger

	// Exporters
	metricExporter *exporters.MetricExporter
	logExporter    *exporters.LogExporter

	// Collectors
	metricsCollector *collectors.MetricsCollector
	logsCollector    *collectors.LogsCollector

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// New creates a new Agent instance.
func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*Agent, error) {
	a := &Agent{
		cfg:      cfg,
		logger:   logger,
		stopChan: make(chan struct{}),
	}

	// Build resource attributes
	res, err := a.buildResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build resource: %w", err)
	}

	// Initialize metric exporter
	if cfg.Metrics.Enabled {
		exporter, err := exporters.NewMetricExporter(ctx, cfg.Collector, res)
		if err != nil {
			return nil, fmt.Errorf("failed to create metric exporter: %w", err)
		}
		a.metricExporter = exporter

		// Initialize metrics collector
		collector, err := collectors.NewMetricsCollector(cfg.Metrics, exporter.Meter(), logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create metrics collector: %w", err)
		}
		a.metricsCollector = collector
	}

	// Initialize log exporter
	if cfg.Logs.Enabled {
		exporter, err := exporters.NewLogExporter(ctx, cfg.Collector, res)
		if err != nil {
			return nil, fmt.Errorf("failed to create log exporter: %w", err)
		}
		a.logExporter = exporter

		// Initialize logs collector
		collector, err := collectors.NewLogsCollector(cfg.Logs, exporter, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create logs collector: %w", err)
		}
		a.logsCollector = collector
	}

	return a, nil
}

// buildResource creates the resource with all attributes.
func (a *Agent) buildResource(ctx context.Context) (*resource.Resource, error) {
	hostname := a.cfg.Agent.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			a.logger.Warn("Failed to get hostname", zap.Error(err))
			hostname = "unknown"
		}
	}

	attrs := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceName("ollystack-agent"),
			semconv.ServiceVersion("1.0.0"),
			semconv.HostName(hostname),
		),
		resource.WithOS(),
		resource.WithHost(),
		resource.WithProcess(),
	}

	// Detect cloud metadata if enabled
	if a.cfg.Resource.Cloud.Enabled {
		attrs = append(attrs, resource.WithFromEnv())
	}

	return resource.New(ctx, attrs...)
}

// Start starts all collectors and exporters.
func (a *Agent) Start(ctx context.Context) error {
	a.logger.Info("Starting agent")

	// Start metrics collection
	if a.metricsCollector != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.runMetricsCollection(ctx)
		}()
	}

	// Start logs collection
	if a.logsCollector != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.runLogsCollection(ctx)
		}()
	}

	a.logger.Info("Agent started successfully")
	return nil
}

// runMetricsCollection runs the periodic metrics collection.
func (a *Agent) runMetricsCollection(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.Metrics.Interval)
	defer ticker.Stop()

	// Initial collection
	a.collectMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopChan:
			return
		case <-ticker.C:
			a.collectMetrics(ctx)
		}
	}
}

// collectMetrics collects all metrics.
func (a *Agent) collectMetrics(ctx context.Context) {
	if err := a.metricsCollector.Collect(ctx); err != nil {
		a.logger.Error("Failed to collect metrics", zap.Error(err))
	}
}

// runLogsCollection runs the log collection.
func (a *Agent) runLogsCollection(ctx context.Context) {
	if err := a.logsCollector.Start(ctx); err != nil {
		a.logger.Error("Failed to start logs collection", zap.Error(err))
		return
	}

	<-ctx.Done()
}

// Stop gracefully stops the agent.
func (a *Agent) Stop(ctx context.Context) error {
	a.logger.Info("Stopping agent")

	// Signal stop
	close(a.stopChan)

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Shutdown exporters
	if a.metricExporter != nil {
		if err := a.metricExporter.Shutdown(ctx); err != nil {
			a.logger.Error("Failed to shutdown metric exporter", zap.Error(err))
		}
	}

	if a.logExporter != nil {
		if err := a.logExporter.Shutdown(ctx); err != nil {
			a.logger.Error("Failed to shutdown log exporter", zap.Error(err))
		}
	}

	// Stop logs collector
	if a.logsCollector != nil {
		if err := a.logsCollector.Stop(); err != nil {
			a.logger.Error("Failed to stop logs collector", zap.Error(err))
		}
	}

	a.logger.Info("Agent stopped")
	return nil
}

// Meter returns the metric.Meter for registering metrics.
func (a *Agent) Meter() metric.Meter {
	if a.metricExporter != nil {
		return a.metricExporter.Meter()
	}
	return nil
}
