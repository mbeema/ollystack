module github.com/ollystack/ollystack/collector

go 1.21

require (
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusexporter v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.93.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.93.0
	go.opentelemetry.io/collector/component v0.93.0
	go.opentelemetry.io/collector/confmap v0.93.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v0.93.0
	go.opentelemetry.io/collector/connector v0.93.0
	go.opentelemetry.io/collector/exporter v0.93.0
	go.opentelemetry.io/collector/exporter/debugexporter v0.93.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.93.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.93.0
	go.opentelemetry.io/collector/extension v0.93.0
	go.opentelemetry.io/collector/extension/ballastextension v0.93.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.93.0
	go.opentelemetry.io/collector/otelcol v0.93.0
	go.opentelemetry.io/collector/processor v0.93.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.93.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.93.0
	go.opentelemetry.io/collector/receiver v0.93.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.93.0
	go.uber.org/zap v1.26.0
)
