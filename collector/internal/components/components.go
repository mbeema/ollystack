// Package components provides the factory functions for all collector components.
package components

import (
	"github.com/ollystack/ollystack/collector/internal/processors/enrichment"
	"github.com/ollystack/ollystack/collector/internal/processors/sampling"
	"github.com/ollystack/ollystack/collector/internal/processors/topology"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/ballastextension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

// Components returns the component factories for the OllyStack Collector.
func Components() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	// Receivers
	factories.Receivers, err = receiver.MakeFactoryMap(
		otlpreceiver.NewFactory(),
		// Add more receivers here
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Processors
	factories.Processors, err = processor.MakeFactoryMap(
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		// Custom OllyStack processors
		enrichment.NewFactory(),
		sampling.NewFactory(),
		topology.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Exporters
	factories.Exporters, err = exporter.MakeFactoryMap(
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		debugexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Extensions
	factories.Extensions, err = extension.MakeFactoryMap(
		ballastextension.NewFactory(),
		zpagesextension.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Connectors
	factories.Connectors, err = connector.MakeFactoryMap()
	if err != nil {
		return otelcol.Factories{}, err
	}

	return factories, nil
}
