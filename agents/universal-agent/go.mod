module github.com/ollystack/ollystack/agents/universal-agent

go 1.21

require (
	github.com/prometheus/client_golang v1.18.0
	github.com/shirou/gopsutil/v3 v3.24.1
	github.com/spf13/cobra v1.8.0
	github.com/spf13/viper v1.18.2
	go.opentelemetry.io/otel v1.23.1
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.23.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.23.1
	go.opentelemetry.io/otel/metric v1.23.1
	go.opentelemetry.io/otel/sdk v1.23.1
	go.opentelemetry.io/otel/sdk/metric v1.23.1
	go.opentelemetry.io/otel/trace v1.23.1
	go.uber.org/zap v1.26.0
	google.golang.org/grpc v1.61.0
	gopkg.in/yaml.v3 v3.0.1
)
