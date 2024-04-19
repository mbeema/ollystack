package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ollystack/storage-consumer/internal/batcher"
	"github.com/ollystack/storage-consumer/internal/clickhouse"
	"github.com/ollystack/storage-consumer/internal/config"
	"github.com/ollystack/storage-consumer/internal/kafka"
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
		Use:   "storage-consumer",
		Short: "OllyStack Storage Consumer - Kafka to ClickHouse",
		Long: `High-throughput consumer that reads telemetry from Kafka
and batch writes to ClickHouse for persistent storage.`,
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

	logger.Info("Starting OllyStack Storage Consumer",
		zap.String("version", "1.0.0"),
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("clickhouse_host", cfg.ClickHouse.Host),
	)

	// Initialize ClickHouse writer
	chWriter, err := clickhouse.NewWriter(cfg.ClickHouse, logger)
	if err != nil {
		return fmt.Errorf("failed to create ClickHouse writer: %w", err)
	}
	defer chWriter.Close()

	// Initialize batchers for each data type
	metricsBatcher := batcher.NewBatcher(
		"metrics",
		cfg.Batcher,
		func(batch [][]byte) error {
			return chWriter.WriteMetrics(batch)
		},
		logger,
	)

	logsBatcher := batcher.NewBatcher(
		"logs",
		cfg.Batcher,
		func(batch [][]byte) error {
			return chWriter.WriteLogs(batch)
		},
		logger,
	)

	tracesBatcher := batcher.NewBatcher(
		"traces",
		cfg.Batcher,
		func(batch [][]byte) error {
			return chWriter.WriteTraces(batch)
		},
		logger,
	)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start batchers
	metricsBatcher.Start(ctx)
	logsBatcher.Start(ctx)
	tracesBatcher.Start(ctx)

	// Create Kafka consumer
	consumer, err := kafka.NewConsumer(cfg.Kafka, logger)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	// Define message handlers
	handlers := map[string]kafka.MessageHandler{
		cfg.Kafka.MetricsTopic: func(msg []byte) error {
			metricsBatcher.Add(msg)
			return nil
		},
		cfg.Kafka.LogsTopic: func(msg []byte) error {
			logsBatcher.Add(msg)
			return nil
		},
		cfg.Kafka.TracesTopic: func(msg []byte) error {
			tracesBatcher.Add(msg)
			return nil
		},
	}

	// Start metrics server
	go startMetricsServer(cfg.Server.MetricsPort)

	// Start consuming
	errChan := make(chan error, 1)
	go func() {
		if err := consumer.Start(ctx, handlers); err != nil {
			errChan <- err
		}
	}()

	logger.Info("Storage Consumer started successfully",
		zap.Int("metrics_port", cfg.Server.MetricsPort),
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

	// Flush remaining batches
	metricsBatcher.Stop()
	logsBatcher.Stop()
	tracesBatcher.Stop()

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
