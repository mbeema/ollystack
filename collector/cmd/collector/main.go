// Package main is the entry point for the OllyStack Collector.
// This is an enhanced OpenTelemetry Collector with custom processors
// for OllyStack-specific functionality.
package main

import (
	"log"

	"github.com/ollystack/ollystack/collector/internal/components"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	info := component.BuildInfo{
		Command:     "ollystack-collector",
		Description: "OllyStack Collector - Enhanced OpenTelemetry Collector",
		Version:     Version,
	}

	factories, err := components.Components()
	if err != nil {
		log.Fatalf("Failed to build components: %v", err)
	}

	params := otelcol.CollectorSettings{
		BuildInfo: info,
		Factories: factories,
	}

	cmd := otelcol.NewCommand(params)
	if err := cmd.Execute(); err != nil {
		log.Fatalf("Collector failed: %v", err)
	}
}
