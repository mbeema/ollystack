// Package enrichment provides a processor that enriches telemetry data
// with additional context such as Kubernetes metadata, cloud tags, and geo-IP.
package enrichment

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

const (
	typeStr = "ollystack_enrichment"
)

// NewFactory creates a new factory for the enrichment processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
		processor.WithMetrics(createMetricsProcessor, component.StabilityLevelBeta),
		processor.WithLogs(createLogsProcessor, component.StabilityLevelBeta),
	)
}

// Config holds the configuration for the enrichment processor.
type Config struct {
	// Kubernetes enrichment
	Kubernetes KubernetesConfig `mapstructure:"kubernetes"`

	// Cloud metadata enrichment
	Cloud CloudConfig `mapstructure:"cloud"`

	// Geo-IP enrichment
	GeoIP GeoIPConfig `mapstructure:"geoip"`
}

// KubernetesConfig holds Kubernetes enrichment settings.
type KubernetesConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	AuthType   string `mapstructure:"auth_type"` // serviceAccount, kubeConfig
	Kubeconfig string `mapstructure:"kubeconfig"`
}

// CloudConfig holds cloud metadata enrichment settings.
type CloudConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Provider string `mapstructure:"provider"` // auto, aws, azure, gcp
}

// GeoIPConfig holds geo-IP enrichment settings.
type GeoIPConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	DatabasePath string `mapstructure:"database_path"`
}

func createDefaultConfig() component.Config {
	return &Config{
		Kubernetes: KubernetesConfig{
			Enabled:  true,
			AuthType: "serviceAccount",
		},
		Cloud: CloudConfig{
			Enabled:  true,
			Provider: "auto",
		},
		GeoIP: GeoIPConfig{
			Enabled: false,
		},
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	return newEnrichmentProcessor(set.Logger, cfg.(*Config), nextConsumer)
}

func createMetricsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	return newEnrichmentMetricsProcessor(set.Logger, cfg.(*Config), nextConsumer)
}

func createLogsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (processor.Logs, error) {
	return newEnrichmentLogsProcessor(set.Logger, cfg.(*Config), nextConsumer)
}
