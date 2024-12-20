// OllyStack Unified Agent
//
// A lightweight, efficient observability agent that collects metrics, logs, and traces
// with local aggregation to minimize resource usage and network bandwidth.
//
// Design Principles:
// 1. Single binary - no dependencies on external collectors
// 2. Local aggregation - reduce data before sending (90% reduction)
// 3. Adaptive sampling - stay within budget
// 4. Minimal footprint - target <100MB memory, <2% CPU
// 5. OTel native - standard protocols, no vendor lock-in

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ollystack/unified-agent/internal/agent"
	"github.com/ollystack/unified-agent/internal/config"
)

var (
	version   = "0.1.0"
	cfgFile   string
	logLevel  string
	dryRun    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "ollystack-agent",
		Short:   "OllyStack Unified Agent - Lightweight observability collection",
		Long: `OllyStack Unified Agent collects metrics, logs, and traces with
minimal resource overhead using local aggregation and adaptive sampling.

Features:
  • Host metrics (CPU, memory, disk, network)
  • Log collection with pattern deduplication
  • Trace reception and smart sampling
  • Local aggregation (90% data reduction)
  • Cardinality control (cost protection)
  • eBPF integration via Beyla (optional)`,
		Version: version,
		RunE:    run,
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "validate config without starting")

	// Subcommands
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Initialize logger
	logger, err := initLogger(logLevel)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting OllyStack Unified Agent",
		zap.String("version", version),
		zap.String("config", cfgFile),
	)

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if dryRun {
		logger.Info("Configuration valid", zap.Any("config", cfg))
		return nil
	}

	// Create and start agent
	ag, err := agent.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Handle shutdown gracefully
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Run agent
	if err := ag.Run(ctx); err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	logger.Info("Agent stopped gracefully")
	return nil
}

func initLogger(level string) (*zap.Logger, error) {
	var cfg zap.Config

	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	return cfg.Build()
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			fmt.Printf("Configuration valid:\n%+v\n", cfg)
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show agent status",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Connect to running agent and get status
			fmt.Println("Agent status: not implemented yet")
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("OllyStack Unified Agent v%s\n", version)
		},
	}
}
