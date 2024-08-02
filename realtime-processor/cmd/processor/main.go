package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ollystack/realtime-processor/internal/alerting"
	"github.com/ollystack/realtime-processor/internal/config"
	"github.com/ollystack/realtime-processor/internal/kafka"
	"github.com/ollystack/realtime-processor/internal/rules"
	"github.com/ollystack/realtime-processor/internal/state"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	cfgFile string
	logger  *zap.Logger
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "realtime-processor",
		Short: "OllyStack Real-time Processor - Sub-second Alert Evaluation",
		Long: `High-performance stream processor that evaluates alert rules
in real-time with <1 second latency.`,
		RunE: run,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Initialize logger
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("Starting OllyStack Real-time Processor",
		zap.String("version", "1.0.0"),
	)

	// Initialize state store (Redis for short-term state)
	stateStore, err := state.NewRedisStore(cfg.Redis, logger)
	if err != nil {
		return fmt.Errorf("failed to create state store: %w", err)
	}
	defer stateStore.Close()

	// Initialize alert sender
	alertSender := alerting.NewSender(cfg.Alerting, logger)

	// Initialize rules engine
	rulesEngine := rules.NewEngine(cfg.Rules, stateStore, alertSender, logger)

	// Load alert rules
	if err := rulesEngine.LoadRules(); err != nil {
		logger.Warn("Failed to load some rules", zap.Error(err))
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create Kafka consumer
	consumer, err := kafka.NewConsumer(cfg.Kafka, logger)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	// Define message handlers
	handlers := map[string]kafka.MessageHandler{
		cfg.Kafka.MetricsTopic: func(msg []byte) error {
			return rulesEngine.EvaluateMetrics(ctx, msg)
		},
		cfg.Kafka.LogsTopic: func(msg []byte) error {
			return rulesEngine.EvaluateLogs(ctx, msg)
		},
		cfg.Kafka.TracesTopic: func(msg []byte) error {
			return rulesEngine.EvaluateTraces(ctx, msg)
		},
	}

	// Start metrics server
	go startMetricsServer(cfg.Server.MetricsPort)

	// Start rules reload watcher
	go rulesEngine.WatchForUpdates(ctx, 30*time.Second)

	// Start consuming
	errChan := make(chan error, 1)
	go func() {
		if err := consumer.Start(ctx, handlers); err != nil {
			errChan <- err
		}
	}()

	logger.Info("Real-time Processor started successfully",
		zap.Int("metrics_port", cfg.Server.MetricsPort),
		zap.Int("rules_count", rulesEngine.RuleCount()),
	)

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("Consumer error", zap.Error(err))
	}

	// Graceful shutdown
	logger.Info("Shutting down...")
	cancel()
	consumer.Close()

	logger.Info("Shutdown complete")
	return nil
}

func startMetricsServer(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	server.ListenAndServe()
}
