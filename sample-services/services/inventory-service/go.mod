module github.com/ollystack/sample-services/inventory-service

go 1.21

require (
	github.com/lib/pq v1.10.9
	github.com/redis/go-redis/v9 v9.4.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.47.0
	go.opentelemetry.io/otel v1.22.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.22.0
	go.opentelemetry.io/otel/sdk v1.22.0
	go.opentelemetry.io/otel/trace v1.22.0
)
