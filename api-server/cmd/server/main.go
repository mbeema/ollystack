// Package main is the entry point for the OllyStack API Server.
// It provides REST, GraphQL, and gRPC APIs for querying observability data.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/config"
	"github.com/mbeema/ollystack/api-server/internal/handlers"
	"github.com/mbeema/ollystack/api-server/internal/middleware"
	"github.com/mbeema/ollystack/api-server/internal/services"
	"github.com/mbeema/ollystack/api-server/internal/telemetry"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var cfgFile string

func main() {
	rootCmd := &cobra.Command{
		Use:   "ollystack-api",
		Short: "OllyStack API Server",
		Long: `OllyStack API Server provides REST, GraphQL, and gRPC APIs
for querying metrics, logs, traces, and topology data.`,
		RunE: runServer,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("OllyStack API Server\n")
			fmt.Printf("  Version:    %s\n", Version)
			fmt.Printf("  Build Time: %s\n", BuildTime)
			fmt.Printf("  Git Commit: %s\n", GitCommit)
		},
	}

	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting OllyStack API Server",
		zap.String("version", Version),
	)

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize OpenTelemetry
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		otlpEndpoint = "localhost:4317"
	}
	telemetryCfg := telemetry.Config{
		ServiceName:    "ollystack-api-server",
		ServiceVersion: Version,
		Environment:    os.Getenv("ENVIRONMENT"),
		OTLPEndpoint:   otlpEndpoint,
		Enabled:        os.Getenv("OTEL_ENABLED") != "false",
	}
	tp, err := telemetry.Init(ctx, telemetryCfg)
	if err != nil {
		logger.Warn("Failed to initialize telemetry", zap.Error(err))
	} else {
		defer tp.Shutdown(ctx)
		logger.Info("Telemetry initialized", zap.String("endpoint", otlpEndpoint))
	}

	// Initialize services
	svc, err := services.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// Set up Gin router
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware("ollystack-api-server"))
	router.Use(middleware.Logger(logger))
	router.Use(middleware.CORS(cfg.Server.CORS))
	router.Use(middleware.RequestID())

	// Health check
	router.GET("/health", handlers.HealthCheck)
	router.GET("/ready", handlers.ReadyCheck(svc))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Authentication (optional)
		if cfg.Auth.Enabled {
			v1.Use(middleware.Auth(cfg.Auth))
		}

		// Metrics endpoints
		metrics := v1.Group("/metrics")
		{
			metrics.GET("/query", handlers.QueryMetrics(svc))
			metrics.GET("/query_range", handlers.QueryMetricsRange(svc))
			metrics.GET("/labels", handlers.ListLabels(svc))
			metrics.GET("/label/:name/values", handlers.ListLabelValues(svc))
			metrics.GET("/series", handlers.ListSeries(svc))
		}

		// Logs endpoints
		logs := v1.Group("/logs")
		{
			logs.GET("/query", handlers.QueryLogs(svc))
			logs.GET("/tail", handlers.TailLogs(svc))
			logs.GET("/labels", handlers.ListLogLabels(svc))
		}

		// Traces endpoints
		traces := v1.Group("/traces")
		{
			traces.GET("", handlers.ListTraces(svc))
			traces.GET("/stats", handlers.GetTraceStats(svc))
			traces.GET("/errors/patterns", handlers.GetErrorPatterns(svc))
			traces.GET("/errors/fingerprints", handlers.GetErrorFingerprints(svc))
			traces.GET("/aggregate", handlers.GetTraceAggregation(svc))
			traces.GET("/group", handlers.GetDynamicGrouping(svc))
			traces.GET("/:traceId", handlers.GetTrace(svc))
			traces.GET("/:traceId/spans", handlers.GetTraceSpans(svc))
			traces.GET("/:traceId/analyze", handlers.AnalyzeTrace(svc))
			traces.GET("/search", handlers.SearchTraces(svc))
		}

		// Topology endpoints
		topology := v1.Group("/topology")
		{
			topology.GET("/services", handlers.ListServices(svc))
			topology.GET("/edges", handlers.ListEdges(svc))
			topology.GET("/graph", handlers.GetTopologyGraph(svc))
		}

		// Alerts endpoints
		alerts := v1.Group("/alerts")
		{
			alerts.GET("", handlers.ListAlerts(svc))
			alerts.POST("", handlers.CreateAlert(svc))
			alerts.GET("/:id", handlers.GetAlert(svc))
			alerts.PUT("/:id", handlers.UpdateAlert(svc))
			alerts.DELETE("/:id", handlers.DeleteAlert(svc))
		}

		// Dashboards endpoints
		dashboards := v1.Group("/dashboards")
		{
			dashboards.GET("", handlers.ListDashboards(svc))
			dashboards.POST("", handlers.CreateDashboard(svc))
			dashboards.GET("/:id", handlers.GetDashboard(svc))
			dashboards.PUT("/:id", handlers.UpdateDashboard(svc))
			dashboards.DELETE("/:id", handlers.DeleteDashboard(svc))
		}

		// Query endpoint (unified)
		v1.POST("/query", handlers.ExecuteQuery(svc))

		// AI endpoints
		ai := v1.Group("/ai")
		{
			ai.POST("/ask", handlers.AskAI(svc))
			ai.POST("/analyze", handlers.AnalyzeAnomaly(svc))
			ai.GET("/suggestions", handlers.GetSuggestions(svc))
		}

		// Correlation endpoints (OllyStack core feature)
		correlate := v1.Group("/correlate")
		{
			correlate.GET("", handlers.ListRecentCorrelations(svc))
			correlate.POST("/search", handlers.SearchCorrelations(svc))
			correlate.GET("/:correlationId", handlers.GetCorrelation(svc))
			correlate.GET("/:correlationId/traces", handlers.GetCorrelationTraces(svc))
			correlate.GET("/:correlationId/logs", handlers.GetCorrelationLogs(svc))
			correlate.GET("/:correlationId/timeline", handlers.GetCorrelationTimeline(svc))
		}
	}

	// WebSocket endpoint for real-time updates
	router.GET("/ws", handlers.WebSocket(svc))

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Server listening", zap.String("address", cfg.Server.Address))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
		return err
	}

	if err := svc.Close(); err != nil {
		logger.Error("Failed to close services", zap.Error(err))
	}

	logger.Info("Server shutdown complete")
	return nil
}
