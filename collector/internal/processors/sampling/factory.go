// Package sampling provides an intelligent tail-based sampling processor
// that makes sampling decisions based on trace characteristics.
package sampling

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

const (
	typeStr = "ollystack_sampling"
)

// NewFactory creates a new factory for the sampling processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
	)
}

// Config holds the configuration for the sampling processor.
type Config struct {
	// DecisionWait is the time to wait for a complete trace before making a decision.
	DecisionWait string `mapstructure:"decision_wait"`

	// NumTraces is the number of traces to keep in memory.
	NumTraces uint64 `mapstructure:"num_traces"`

	// ExpectedNewTracesPerSec is the expected rate of new traces.
	ExpectedNewTracesPerSec uint64 `mapstructure:"expected_new_traces_per_sec"`

	// Policies define the sampling rules.
	Policies []PolicyConfig `mapstructure:"policies"`
}

// PolicyConfig defines a sampling policy.
type PolicyConfig struct {
	// Name of the policy
	Name string `mapstructure:"name"`

	// Type of policy: always_sample, latency, status_code, rate_limiting, string_attribute
	Type string `mapstructure:"type"`

	// Latency threshold in milliseconds (for latency policy)
	LatencyMS int64 `mapstructure:"latency_ms"`

	// Status codes to sample (for status_code policy)
	StatusCodes []string `mapstructure:"status_codes"`

	// Rate limit (for rate_limiting policy)
	SpansPerSecond int64 `mapstructure:"spans_per_second"`

	// Attribute matching (for string_attribute policy)
	Attribute      string   `mapstructure:"attribute"`
	Values         []string `mapstructure:"values"`
	InvertMatch    bool     `mapstructure:"invert_match"`
}

func createDefaultConfig() component.Config {
	return &Config{
		DecisionWait:            "30s",
		NumTraces:               100000,
		ExpectedNewTracesPerSec: 1000,
		Policies: []PolicyConfig{
			{
				Name: "errors",
				Type: "status_code",
				StatusCodes: []string{"ERROR"},
			},
			{
				Name:      "high-latency",
				Type:      "latency",
				LatencyMS: 1000, // 1 second
			},
			{
				Name:           "rate-limit",
				Type:           "rate_limiting",
				SpansPerSecond: 100,
			},
		},
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	return newSamplingProcessor(set.Logger, cfg.(*Config), nextConsumer)
}
