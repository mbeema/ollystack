module github.com/ollystack/ingestion-gateway

go 1.21

require (
	github.com/IBM/sarama v1.42.1
	github.com/prometheus/client_golang v1.18.0
	github.com/spf13/cobra v1.8.0
	github.com/spf13/viper v1.18.2
	go.opentelemetry.io/collector/pdata v1.0.0
	go.opentelemetry.io/proto/otlp v1.0.0
	go.uber.org/zap v1.26.0
	golang.org/x/time v0.5.0
	google.golang.org/grpc v1.60.1
	google.golang.org/protobuf v1.32.0
)
