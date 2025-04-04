package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer      trace.Tracer
	meter       metric.Meter
	redisClient *redis.Client
	httpClient  *http.Client
	serviceName = "api-gateway"

	// Metrics with exemplar support - exemplars automatically attached when recorded within spans
	requestLatency   metric.Float64Histogram // HTTP request duration in ms
	requestCounter   metric.Int64Counter     // Total HTTP requests
	errorCounter     metric.Int64Counter     // Total HTTP errors
	activeRequests   metric.Int64UpDownCounter // Currently active requests
)

// Structured JSON logger
func logJSON(level, correlationID, msg string, extra map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":          level,
		"service":        serviceName,
		"correlation_id": correlationID,
		"message":        msg,
	}
	for k, v := range extra {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

func logInfo(correlationID, msg string, extra ...map[string]interface{}) {
	var e map[string]interface{}
	if len(extra) > 0 {
		e = extra[0]
	}
	logJSON("info", correlationID, msg, e)
}

func logError(correlationID, msg string, err error, extra ...map[string]interface{}) {
	e := map[string]interface{}{"error": err.Error()}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			e[k] = v
		}
	}
	logJSON("error", correlationID, msg, e)
}

func logWarn(correlationID, msg string, extra ...map[string]interface{}) {
	var e map[string]interface{}
	if len(extra) > 0 {
		e = extra[0]
	}
	logJSON("warn", correlationID, msg, e)
}

var meterProvider *sdkmetric.MeterProvider

func initTelemetry(ctx context.Context) (*sdktrace.TracerProvider, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	// Create trace exporter
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(os.Getenv("OTEL_SERVICE_NAME")),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create and set trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer("api-gateway")

	// Create and set meter provider with exemplar support
	// Exemplars are automatically attached when metrics are recorded within a span context
	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(meterProvider)
	meter = meterProvider.Meter("api-gateway")

	// Initialize metrics - exemplars are automatically attached when recorded within spans
	requestLatency, _ = meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in milliseconds"),
		metric.WithUnit("ms"),
	)

	requestCounter, _ = meter.Int64Counter(
		"http.server.request.total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)

	errorCounter, _ = meter.Int64Counter(
		"http.server.error.total",
		metric.WithDescription("Total number of HTTP errors"),
		metric.WithUnit("{error}"),
	)

	activeRequests, _ = meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of currently active HTTP requests"),
		metric.WithUnit("{request}"),
	)

	logInfo("", "Telemetry initialized", map[string]interface{}{
		"endpoint": endpoint,
		"tracer":   "enabled",
		"metrics":  "enabled",
	})

	return tp, nil
}

func generateCorrelationID() string {
	timestamp := time.Now().UnixMilli()
	suffix := fmt.Sprintf("%08x", rand.Uint32())
	return fmt.Sprintf("olly-%x-%s", timestamp, suffix)
}

func main() {
	ctx := context.Background()

	tp, err := initTelemetry(ctx)
	if err != nil {
		logError("", "Failed to initialize telemetry", err, nil)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			logError("", "Error shutting down tracer", err, nil)
		}
		if meterProvider != nil {
			if err := meterProvider.Shutdown(ctx); err != nil {
				logError("", "Error shutting down meter provider", err, nil)
			}
		}
	}()

	// Initialize Redis client
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisClient = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	// Initialize HTTP client with tracing
	httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/orders", ordersHandler)
	mux.HandleFunc("/api/orders/", orderDetailHandler)
	mux.HandleFunc("/api/products", productsHandler)

	// Wrap with OTEL instrumentation
	handler := otelhttp.NewHandler(mux, "api-gateway")

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logInfo("", "API Gateway starting", map[string]interface{}{"port": 8080})
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logError("", "Server error", err, nil)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logInfo("", "Shutting down server", nil)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func ordersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()

	// Generate or extract correlation ID
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = generateCorrelationID()
	}

	// Track active requests
	activeRequests.Add(ctx, 1)
	defer activeRequests.Add(ctx, -1)

	logInfo(correlationID, "Received request", map[string]interface{}{
		"method": r.Method,
		"path":   "/api/orders",
		"remote": r.RemoteAddr,
	})

	// Add to baggage for propagation
	member, _ := baggage.NewMember("correlation_id", correlationID)
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("correlation_id", correlationID),
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/api/orders"),
	)

	// Metric attributes for labeling
	metricAttrs := metric.WithAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/api/orders"),
		attribute.String("service.name", serviceName),
	)

	// Record request count - exemplar will be attached because we're inside a span
	requestCounter.Add(ctx, 1, metricAttrs)

	var statusCode int = http.StatusOK

	switch r.Method {
	case http.MethodGet:
		listOrders(ctx, w, r, correlationID)
	case http.MethodPost:
		createOrder(ctx, w, r, correlationID)
	default:
		statusCode = http.StatusMethodNotAllowed
		logWarn(correlationID, "Method not allowed", map[string]interface{}{"method": r.Method})
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		// Record error with exemplar (trace context attached)
		errorCounter.Add(ctx, 1, metricAttrs)
	}

	// Record request duration with exemplar
	// The trace_id and span_id will be automatically attached as exemplars
	// This enables clicking from a latency spike directly to the contributing trace
	duration := float64(time.Since(startTime).Milliseconds())
	requestLatency.Record(ctx, duration, metricAttrs,
		metric.WithAttributes(attribute.Int("http.status_code", statusCode)),
	)
}

func orderDetailHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = generateCorrelationID()
	}

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Order details", "correlationId": correlationID})
}

func productsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "get-products")
	defer span.End()

	// Simulate getting products from cache or DB
	products := []map[string]interface{}{
		{"id": 1, "sku": "LAPTOP-001", "name": "Gaming Laptop Pro", "price": 1999.99},
		{"id": 2, "sku": "PHONE-001", "name": "SmartPhone X", "price": 999.99},
		{"id": 3, "sku": "HEADPHONE-001", "name": "Wireless Headphones Pro", "price": 349.99},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func listOrders(ctx context.Context, w http.ResponseWriter, r *http.Request, correlationID string) {
	ctx, span := tracer.Start(ctx, "list-orders")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	logInfo(correlationID, "Listing orders", nil)

	// Metric attributes for downstream calls
	downstreamAttrs := metric.WithAttributes(
		attribute.String("downstream.service", "order-service"),
		attribute.String("operation", "list-orders"),
	)

	// Call order service
	orderServiceURL := os.Getenv("ORDER_SERVICE_URL")
	if orderServiceURL == "" {
		orderServiceURL = "http://localhost:8082"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", orderServiceURL+"/orders", nil)
	if err != nil {
		logError(correlationID, "Failed to create request", err, nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Record error metric with exemplar (links to this trace)
		errorCounter.Add(ctx, 1, downstreamAttrs)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header.Set("X-Correlation-ID", correlationID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	callStart := time.Now()
	resp, err := httpClient.Do(req)
	callDuration := float64(time.Since(callStart).Milliseconds())

	if err != nil {
		logError(correlationID, "Failed to call order service", err, map[string]interface{}{"url": orderServiceURL})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Record error with exemplar
		errorCounter.Add(ctx, 1, downstreamAttrs)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Record downstream call latency with exemplar
	requestLatency.Record(ctx, callDuration, downstreamAttrs,
		metric.WithAttributes(attribute.Int("http.status_code", resp.StatusCode)),
	)

	logInfo(correlationID, "Orders retrieved", map[string]interface{}{"status": resp.StatusCode})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Correlation-ID", correlationID)
	io.Copy(w, resp.Body)
}

func createOrder(ctx context.Context, w http.ResponseWriter, r *http.Request, correlationID string) {
	ctx, span := tracer.Start(ctx, "create-order")
	defer span.End()

	span.SetAttributes(
		attribute.String("correlation_id", correlationID),
		attribute.String("operation", "create_order"),
	)

	logInfo(correlationID, "Creating order", nil)

	// Metric attributes for order creation
	orderAttrs := metric.WithAttributes(
		attribute.String("operation", "create-order"),
		attribute.String("downstream.service", "order-service"),
	)

	// Parse request body
	var orderReq struct {
		CustomerID string `json:"customerId"`
		Items      []struct {
			ProductID int `json:"productId"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&orderReq); err != nil {
		logError(correlationID, "Invalid request body", err, nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid request body")
		// Record validation error with exemplar
		errorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("error.type", "validation_error"),
			attribute.String("operation", "create-order"),
		))
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if orderReq.CustomerID == "" {
		orderReq.CustomerID = uuid.New().String()
	}

	logInfo(correlationID, "Order request parsed", map[string]interface{}{
		"customer_id": orderReq.CustomerID,
		"item_count":  len(orderReq.Items),
	})

	span.SetAttributes(
		attribute.String("customer_id", orderReq.CustomerID),
		attribute.Int("item_count", len(orderReq.Items)),
	)

	// Forward to order service
	orderServiceURL := os.Getenv("ORDER_SERVICE_URL")
	if orderServiceURL == "" {
		orderServiceURL = "http://localhost:8082"
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"correlationId": correlationID,
		"customerId":    orderReq.CustomerID,
		"items":         orderReq.Items,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", orderServiceURL+"/orders", bytes.NewReader(reqBody))
	if err != nil {
		logError(correlationID, "Failed to create request", err, nil)
		span.RecordError(err)
		errorCounter.Add(ctx, 1, orderAttrs)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", correlationID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	logInfo(correlationID, "Forwarding to order service", map[string]interface{}{"url": orderServiceURL})

	callStart := time.Now()
	resp, err := httpClient.Do(req)
	callDuration := float64(time.Since(callStart).Milliseconds())

	if err != nil {
		logError(correlationID, "Order service call failed", err, map[string]interface{}{"url": orderServiceURL})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Record error with exemplar
		errorCounter.Add(ctx, 1, orderAttrs)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Record downstream call latency with exemplar
	requestLatency.Record(ctx, callDuration, orderAttrs,
		metric.WithAttributes(attribute.Int("http.status_code", resp.StatusCode)),
	)

	if resp.StatusCode >= 400 {
		logWarn(correlationID, "Order service returned error", map[string]interface{}{"status": resp.StatusCode})
		span.SetStatus(codes.Error, "Order service returned error")
		// Record error with exemplar - can click through to see the failing trace
		errorCounter.Add(ctx, 1, orderAttrs,
			metric.WithAttributes(attribute.Int("http.status_code", resp.StatusCode)),
		)
	} else {
		logInfo(correlationID, "Order created successfully", map[string]interface{}{"status": resp.StatusCode})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Correlation-ID", correlationID)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
