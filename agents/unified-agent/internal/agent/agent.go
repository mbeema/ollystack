// Package agent implements the unified observability agent
package agent

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ollystack/unified-agent/internal/aggregator"
	"github.com/ollystack/unified-agent/internal/collector"
	"github.com/ollystack/unified-agent/internal/config"
	"github.com/ollystack/unified-agent/internal/exporter"
	"github.com/ollystack/unified-agent/internal/pipeline"
	"github.com/ollystack/unified-agent/internal/receiver"
)

// Agent is the unified observability agent
type Agent struct {
	config *config.Config
	logger *zap.Logger

	// Collectors
	metricsCollector *collector.MetricsCollector
	logsCollector    *collector.LogsCollector

	// Receivers (for traces from apps)
	otlpReceiver *receiver.OTLPReceiver

	// Processing pipeline
	pipeline *pipeline.Pipeline

	// Aggregator
	aggregator *aggregator.Aggregator

	// Exporter
	exporter *exporter.OTLPExporter

	// Health server
	healthServer *http.Server

	// State
	mu      sync.RWMutex
	running bool
	stats   AgentStats
}

// AgentStats tracks agent statistics
type AgentStats struct {
	StartTime           time.Time
	MetricsCollected    int64
	LogsCollected       int64
	TracesReceived      int64
	DataPointsDropped   int64
	DataPointsExported  int64
	BytesSent           int64
	LastExportTime      time.Time
	LastError           string
	LastErrorTime       time.Time
	CurrentMemoryBytes  int64
	AggregationSavings  float64 // Percentage saved by aggregation
}

// New creates a new agent instance
func New(cfg *config.Config, logger *zap.Logger) (*Agent, error) {
	// Apply resource limits
	if cfg.Resources.MaxProcs > 0 {
		runtime.GOMAXPROCS(cfg.Resources.MaxProcs)
	}

	a := &Agent{
		config: cfg,
		logger: logger,
		stats: AgentStats{
			StartTime: time.Now(),
		},
	}

	// Initialize components
	if err := a.initComponents(); err != nil {
		return nil, fmt.Errorf("init components: %w", err)
	}

	return a, nil
}

func (a *Agent) initComponents() error {
	var err error

	// Create exporter first (needed by pipeline)
	a.exporter, err = exporter.NewOTLPExporter(exporter.Config{
		Endpoint:    a.config.Export.Endpoint,
		UseGRPC:     a.config.Export.UseGRPC,
		APIKey:      a.config.Export.Auth.APIKey,
		BearerToken: a.config.Export.Auth.BearerToken,
		TLS: exporter.TLSConfig{
			Enabled:    a.config.Export.TLS.Enabled,
			CertFile:   a.config.Export.TLS.CertFile,
			KeyFile:    a.config.Export.TLS.KeyFile,
			CAFile:     a.config.Export.TLS.CAFile,
			SkipVerify: a.config.Export.TLS.SkipVerify,
		},
		BatchSize:    a.config.Export.Batch.MaxSize,
		BatchTimeout: a.config.Export.Batch.Timeout,
		RetryConfig: exporter.RetryConfig{
			Enabled:     a.config.Export.Retry.Enabled,
			MaxAttempts: a.config.Export.Retry.MaxAttempts,
			InitialWait: a.config.Export.Retry.InitialWait,
			MaxWait:     a.config.Export.Retry.MaxWait,
		},
		BufferConfig: exporter.BufferConfig{
			Enabled: a.config.Export.Buffer.Enabled,
			Path:    a.config.Export.Buffer.Path,
			MaxSize: a.config.Export.Buffer.MaxSize,
		},
	}, a.logger)
	if err != nil {
		return fmt.Errorf("create exporter: %w", err)
	}

	// Create aggregator
	a.aggregator = aggregator.New(aggregator.Config{
		Window:              a.config.Aggregation.Window,
		MetricAggregates:    a.config.Aggregation.Metrics.Aggregates,
		DropRawMetrics:      a.config.Aggregation.Metrics.DropRaw,
		GroupSimilarLogs:    a.config.Aggregation.Logs.GroupSimilar,
		LogSimilarityThresh: a.config.Aggregation.Logs.SimilarityThreshold,
	}, a.logger)

	// Create pipeline
	a.pipeline = pipeline.New(pipeline.Config{
		SamplingConfig: pipeline.SamplingConfig{
			Enabled:            a.config.Sampling.Enabled,
			TargetRate:         a.config.Sampling.TargetRate,
			TraceRate:          a.config.Sampling.Traces.Rate,
			AlwaysSampleErrors: a.config.Sampling.Traces.AlwaysSampleErrors,
			SlowThreshold:      a.config.Sampling.Traces.SlowThreshold,
			LogInfoRate:        a.config.Sampling.Logs.InfoRate,
			LogDebugRate:       a.config.Sampling.Logs.DebugRate,
			AlwaysKeepErrors:   a.config.Sampling.Logs.AlwaysKeepErrors,
		},
		CardinalityConfig: pipeline.CardinalityConfig{
			Enabled:            a.config.Cardinality.Enabled,
			MaxSeriesPerMetric: a.config.Cardinality.MaxSeriesPerMetric,
			MaxLabelValues:     a.config.Cardinality.MaxLabelValues,
			DropLabels:         a.config.Cardinality.DropLabels,
		},
		EnrichmentConfig: pipeline.EnrichmentConfig{
			AddHostname:    a.config.Enrichment.AddHostname,
			Hostname:       a.config.Agent.Hostname,
			Environment:    a.config.Agent.Environment,
			StaticTags:     a.config.Enrichment.StaticTags,
			KubernetesEnabled: a.config.Enrichment.Kubernetes.Enabled,
			CloudEnabled:      a.config.Enrichment.Cloud.Enabled,
			CloudProvider:     a.config.Enrichment.Cloud.Provider,
		},
		Aggregator: a.aggregator,
		Exporter:   a.exporter,
	}, a.logger)

	// Create metrics collector
	if a.config.Metrics.Enabled {
		a.metricsCollector, err = collector.NewMetricsCollector(collector.MetricsConfig{
			Interval:        a.config.Metrics.Interval,
			CollectCPU:      a.config.Metrics.Collectors.CPU,
			CollectMemory:   a.config.Metrics.Collectors.Memory,
			CollectDisk:     a.config.Metrics.Collectors.Disk,
			CollectNetwork:  a.config.Metrics.Collectors.Network,
			CollectFS:       a.config.Metrics.Collectors.Filesystem,
			CollectProcess:  a.config.Metrics.Collectors.Process,
			CollectContainer: a.config.Metrics.Collectors.Container,
			ProcessInclude:  a.config.Metrics.Process.Include,
			ProcessExclude:  a.config.Metrics.Process.Exclude,
			MaxProcesses:    a.config.Metrics.Process.MaxProcesses,
			DockerSocket:    a.config.Metrics.Container.DockerSocket,
		}, a.pipeline, a.logger)
		if err != nil {
			return fmt.Errorf("create metrics collector: %w", err)
		}
	}

	// Create logs collector
	if a.config.Logs.Enabled {
		a.logsCollector, err = collector.NewLogsCollector(collector.LogsConfig{
			Sources:             a.config.Logs.Sources,
			DeduplicationEnabled: a.config.Logs.Deduplication.Enabled,
			DeduplicationWindow: a.config.Logs.Deduplication.Window,
			MaxPatterns:         a.config.Logs.Deduplication.MaxPatterns,
			MultilinePattern:    a.config.Logs.Multiline.Pattern,
			MultilineMaxLines:   a.config.Logs.Multiline.MaxLines,
		}, a.pipeline, a.logger)
		if err != nil {
			return fmt.Errorf("create logs collector: %w", err)
		}
	}

	// Create OTLP receiver for traces
	if a.config.Traces.Enabled {
		a.otlpReceiver, err = receiver.NewOTLPReceiver(receiver.OTLPConfig{
			GRPCPort: a.config.Traces.OTLP.GRPCPort,
			HTTPPort: a.config.Traces.OTLP.HTTPPort,
		}, a.pipeline, a.logger)
		if err != nil {
			return fmt.Errorf("create OTLP receiver: %w", err)
		}
	}

	return nil
}

// Run starts the agent and blocks until context is cancelled
func (a *Agent) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.running = true
	a.mu.Unlock()

	a.logger.Info("Starting agent components")

	// Start health server
	a.startHealthServer()

	// Start components
	var wg sync.WaitGroup
	errCh := make(chan error, 4)

	// Start exporter
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.exporter.Start(ctx); err != nil {
			errCh <- fmt.Errorf("exporter: %w", err)
		}
	}()

	// Start pipeline
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.pipeline.Start(ctx); err != nil {
			errCh <- fmt.Errorf("pipeline: %w", err)
		}
	}()

	// Start metrics collector
	if a.metricsCollector != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.metricsCollector.Start(ctx); err != nil {
				errCh <- fmt.Errorf("metrics collector: %w", err)
			}
		}()
	}

	// Start logs collector
	if a.logsCollector != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.logsCollector.Start(ctx); err != nil {
				errCh <- fmt.Errorf("logs collector: %w", err)
			}
		}()
	}

	// Start OTLP receiver
	if a.otlpReceiver != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.otlpReceiver.Start(ctx); err != nil {
				errCh <- fmt.Errorf("OTLP receiver: %w", err)
			}
		}()
	}

	// Start stats collector
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.collectStats(ctx)
	}()

	a.logger.Info("Agent started successfully",
		zap.Bool("metrics", a.config.Metrics.Enabled),
		zap.Bool("logs", a.config.Logs.Enabled),
		zap.Bool("traces", a.config.Traces.Enabled),
	)

	// Wait for shutdown or error
	select {
	case <-ctx.Done():
		a.logger.Info("Shutting down agent")
	case err := <-errCh:
		a.logger.Error("Component error", zap.Error(err))
		return err
	}

	// Stop health server
	a.stopHealthServer()

	// Wait for all components to stop
	wg.Wait()

	a.mu.Lock()
	a.running = false
	a.mu.Unlock()

	return nil
}

func (a *Agent) startHealthServer() {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Readiness check
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		running := a.running
		a.mu.RUnlock()

		if running {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Not Ready"))
		}
	})

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		stats := a.stats
		a.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# HELP ollystack_agent_metrics_collected_total Total metrics collected\n")
		fmt.Fprintf(w, "ollystack_agent_metrics_collected_total %d\n", stats.MetricsCollected)
		fmt.Fprintf(w, "# HELP ollystack_agent_logs_collected_total Total logs collected\n")
		fmt.Fprintf(w, "ollystack_agent_logs_collected_total %d\n", stats.LogsCollected)
		fmt.Fprintf(w, "# HELP ollystack_agent_traces_received_total Total traces received\n")
		fmt.Fprintf(w, "ollystack_agent_traces_received_total %d\n", stats.TracesReceived)
		fmt.Fprintf(w, "# HELP ollystack_agent_bytes_sent_total Total bytes sent\n")
		fmt.Fprintf(w, "ollystack_agent_bytes_sent_total %d\n", stats.BytesSent)
		fmt.Fprintf(w, "# HELP ollystack_agent_aggregation_savings_ratio Data reduction from aggregation\n")
		fmt.Fprintf(w, "ollystack_agent_aggregation_savings_ratio %.4f\n", stats.AggregationSavings)
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		stats := a.stats
		a.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"status": "running",
			"uptime_seconds": %.0f,
			"metrics_collected": %d,
			"logs_collected": %d,
			"traces_received": %d,
			"bytes_sent": %d,
			"aggregation_savings": "%.1f%%",
			"memory_mb": %.1f
		}`,
			time.Since(stats.StartTime).Seconds(),
			stats.MetricsCollected,
			stats.LogsCollected,
			stats.TracesReceived,
			stats.BytesSent,
			stats.AggregationSavings*100,
			float64(stats.CurrentMemoryBytes)/(1024*1024),
		)
	})

	a.healthServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.config.Agent.HealthPort),
		Handler: mux,
	}

	go func() {
		if err := a.healthServer.ListenAndServe(); err != http.ErrServerClosed {
			a.logger.Error("Health server error", zap.Error(err))
		}
	}()

	a.logger.Info("Health server started", zap.Int("port", a.config.Agent.HealthPort))
}

func (a *Agent) stopHealthServer() {
	if a.healthServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.healthServer.Shutdown(ctx)
	}
}

func (a *Agent) collectStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			a.mu.Lock()
			a.stats.CurrentMemoryBytes = int64(memStats.Alloc)

			// Get stats from components
			if a.pipeline != nil {
				pStats := a.pipeline.Stats()
				a.stats.MetricsCollected = pStats.MetricsProcessed
				a.stats.LogsCollected = pStats.LogsProcessed
				a.stats.TracesReceived = pStats.TracesProcessed
				a.stats.DataPointsDropped = pStats.DroppedByCardinality + pStats.DroppedBySampling
				a.stats.AggregationSavings = pStats.AggregationRatio
			}

			if a.exporter != nil {
				eStats := a.exporter.Stats()
				a.stats.DataPointsExported = eStats.PointsExported
				a.stats.BytesSent = eStats.BytesSent
				a.stats.LastExportTime = eStats.LastExportTime
			}
			a.mu.Unlock()

			// Check memory limit
			if a.config.Resources.MaxMemory > 0 && int64(memStats.Alloc) > a.config.Resources.MaxMemory {
				a.logger.Warn("Memory usage exceeds limit, forcing GC",
					zap.Int64("current", int64(memStats.Alloc)),
					zap.Int64("limit", a.config.Resources.MaxMemory),
				)
				runtime.GC()
			}
		}
	}
}

// Stats returns current agent statistics
func (a *Agent) Stats() AgentStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stats
}
