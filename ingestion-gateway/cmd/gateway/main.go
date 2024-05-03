package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ollystack/ingestion-gateway/internal/clickhouse"
	"github.com/ollystack/ingestion-gateway/internal/config"
	"github.com/ollystack/ingestion-gateway/internal/handler"
	"github.com/ollystack/ingestion-gateway/internal/health"
	"github.com/ollystack/ingestion-gateway/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	cfgFile string
	logger  *zap.Logger
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ingestion-gateway",
		Short: "OllyStack Ingestion Gateway - OTLP to ClickHouse",
		Long: `High-performance ingestion gateway that receives OTLP telemetry
and writes directly to ClickHouse for storage and real-time analytics.`,
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

	logger.Info("Starting OllyStack Ingestion Gateway",
		zap.String("version", "2.0.0"),
		zap.Int("grpc_port", cfg.Server.GRPCPort),
		zap.Int("http_port", cfg.Server.HTTPPort),
		zap.String("clickhouse_host", cfg.ClickHouse.Host),
	)

	// Initialize ClickHouse writer
	chConfig := clickhouse.Config{
		Host:          cfg.ClickHouse.Host,
		Port:          cfg.ClickHouse.Port,
		Database:      cfg.ClickHouse.Database,
		Username:      cfg.ClickHouse.Username,
		Password:      cfg.ClickHouse.Password,
		MaxOpenConns:  cfg.ClickHouse.MaxOpenConns,
		MaxIdleConns:  cfg.ClickHouse.MaxIdleConns,
		DialTimeout:   cfg.ClickHouse.DialTimeout,
		ReadTimeout:   cfg.ClickHouse.ReadTimeout,
		WriteTimeout:  cfg.ClickHouse.WriteTimeout,
		Compression:   cfg.ClickHouse.Compression,
		BatchSize:     cfg.ClickHouse.BatchSize,
		FlushInterval: cfg.ClickHouse.FlushInterval,
		MaxRetries:    cfg.ClickHouse.MaxRetries,
		RetryInterval: cfg.ClickHouse.RetryInterval,
	}

	writer, err := clickhouse.NewWriter(chConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to create ClickHouse writer: %w", err)
	}
	defer writer.Close()

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit, logger)

	// Initialize health checker
	healthChecker := health.NewChecker(writer, logger)

	// Create OTLP handler
	otlpHandler := handler.NewOTLPHandler(writer, rateLimiter, cfg, logger)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	errChan := make(chan error, 3)

	// Start gRPC server
	go func() {
		if err := startGRPCServer(cfg.Server.GRPCPort, otlpHandler); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Start HTTP server (for OTLP HTTP and health)
	go func() {
		if err := startHTTPServer(cfg.Server.HTTPPort, otlpHandler, healthChecker); err != nil {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start metrics server
	go func() {
		if err := startMetricsServer(cfg.Server.MetricsPort); err != nil {
			errChan <- fmt.Errorf("metrics server error: %w", err)
		}
	}()

	logger.Info("Ingestion Gateway started successfully",
		zap.Int("grpc_port", cfg.Server.GRPCPort),
		zap.Int("http_port", cfg.Server.HTTPPort),
		zap.Int("metrics_port", cfg.Server.MetricsPort),
	)

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("Server error", zap.Error(err))
	case <-ctx.Done():
	}

	// Graceful shutdown
	logger.Info("Shutting down...")

	// Give time for in-flight requests to complete and buffers to flush
	time.Sleep(5 * time.Second)

	logger.Info("Shutdown complete")
	return nil
}

func startGRPCServer(port int, handler *handler.OTLPHandler) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(16 * 1024 * 1024), // 16MB max message
		grpc.MaxSendMsgSize(16 * 1024 * 1024),
	)

	handler.RegisterGRPC(grpcServer)

	return grpcServer.Serve(lis)
}

func startHTTPServer(port int, handler *handler.OTLPHandler, healthChecker *health.Checker) error {
	mux := http.NewServeMux()

	// OTLP HTTP endpoints
	mux.HandleFunc("/v1/metrics", handler.HandleMetricsHTTP)
	mux.HandleFunc("/v1/logs", handler.HandleLogsHTTP)
	mux.HandleFunc("/v1/traces", handler.HandleTracesHTTP)

	// Health endpoints
	mux.HandleFunc("/health", healthChecker.HealthHandler)
	mux.HandleFunc("/ready", healthChecker.ReadyHandler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server.ListenAndServe()
}

func startMetricsServer(port int) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return server.ListenAndServe()
}
