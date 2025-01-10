// Package main is the entry point for the OllyStack Universal Agent.
// The Universal Agent collects metrics, logs, and traces from hosts
// and exports them via OTLP to the OllyStack collector.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/agent"
	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"github.com/spf13/cobra"
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
		Use:   "ollystack-agent",
		Short: "OllyStack Universal Agent",
		Long: `OllyStack Universal Agent collects metrics, logs, and traces from hosts
and exports them to the OllyStack collector using OTLP.

It provides:
- Host metrics (CPU, memory, disk, network)
- Log collection (files, journald, Windows Event Log)
- Process monitoring
- Container metrics (Docker, containerd)
- Auto-discovery of services`,
		RunE: runAgent,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/ollystack/agent.yaml)")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("OllyStack Universal Agent\n")
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

func runAgent(cmd *cobra.Command, args []string) error {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting OllyStack Universal Agent",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("git_commit", GitCommit),
	)

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger.Info("Configuration loaded",
		zap.String("collector_endpoint", cfg.Collector.Endpoint),
		zap.Bool("metrics_enabled", cfg.Metrics.Enabled),
		zap.Bool("logs_enabled", cfg.Logs.Enabled),
	)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start the agent
	a, err := agent.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown
	if err := a.Stop(ctx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
		return err
	}

	logger.Info("Agent shutdown complete")
	return nil
}
