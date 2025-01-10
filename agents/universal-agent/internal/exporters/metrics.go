// Package exporters provides OTLP exporters for telemetry data.
package exporters

import (
	"context"
	"fmt"
	"time"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// MetricExporter exports metrics via OTLP.
type MetricExporter struct {
	exporter       *otlpmetricgrpc.Exporter
	meterProvider  *sdkmetric.MeterProvider
	meter          metric.Meter
}

// NewMetricExporter creates a new MetricExporter.
func NewMetricExporter(ctx context.Context, cfg config.CollectorConfig, res *resource.Resource) (*MetricExporter, error) {
	// Create gRPC connection options
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithTimeout(cfg.Timeout),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	if cfg.Compression == "gzip" {
		opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
	}

	// Add headers if specified
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}

	// Add retry if enabled
	if cfg.Retry.Enabled {
		opts = append(opts, otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: cfg.Retry.InitialWait,
			MaxInterval:     cfg.Retry.MaxWait,
			MaxElapsedTime:  time.Duration(cfg.Retry.MaxAttempts) * cfg.Retry.MaxWait,
		}))
	}

	// Create exporter
	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				exporter,
				sdkmetric.WithInterval(10*time.Second),
			),
		),
	)

	// Set as global meter provider
	otel.SetMeterProvider(meterProvider)

	// Create meter
	meter := meterProvider.Meter(
		"ollystack-agent",
		metric.WithInstrumentationVersion("1.0.0"),
	)

	return &MetricExporter{
		exporter:      exporter,
		meterProvider: meterProvider,
		meter:         meter,
	}, nil
}

// Meter returns the metric.Meter for creating instruments.
func (e *MetricExporter) Meter() metric.Meter {
	return e.meter
}

// Shutdown gracefully shuts down the exporter.
func (e *MetricExporter) Shutdown(ctx context.Context) error {
	if err := e.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown meter provider: %w", err)
	}
	return nil
}

// createGRPCConnection creates a gRPC connection with the specified options.
func createGRPCConnection(endpoint string, insecureConn bool) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if insecureConn {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return conn, nil
}
