// Package topology provides a processor that discovers and tracks
// service topology from trace data.
package topology

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

const (
	typeStr = "ollystack_topology"
)

// NewFactory creates a new factory for the topology processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
	)
}

// Config holds the configuration for the topology processor.
type Config struct {
	// UpdateInterval is how often to update the topology.
	UpdateInterval string `mapstructure:"update_interval"`

	// StorageEndpoint is where to store topology data.
	StorageEndpoint string `mapstructure:"storage_endpoint"`

	// MaxServices is the maximum number of services to track.
	MaxServices int `mapstructure:"max_services"`

	// MaxEdges is the maximum number of edges to track.
	MaxEdges int `mapstructure:"max_edges"`
}

func createDefaultConfig() component.Config {
	return &Config{
		UpdateInterval: "10s",
		MaxServices:    10000,
		MaxEdges:       100000,
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	return newTopologyProcessor(set.Logger, cfg.(*Config), nextConsumer)
}
