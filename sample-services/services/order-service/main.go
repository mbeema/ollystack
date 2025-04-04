package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	db          *sql.DB
	redisClient *redis.Client
	httpClient  *http.Client
	serviceName = "order-service"

	// Metrics with exemplar support
	requestLatency metric.Float64Histogram // Request duration
	requestCounter metric.Int64Counter     // Total requests
	errorCounter   metric.Int64Counter     // Total errors
	dbQueryLatency metric.Float64Histogram // Database query duration
	orderCounter   metric.Int64Counter     // Orders created
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

	// Create metric exporter for exemplar support
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

	tracer = tp.Tracer("order-service")

	// Create and set meter provider with exemplar support
	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(meterProvider)
	meter = meterProvider.Meter("order-service")

	// Initialize metrics with exemplar support
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

	dbQueryLatency, _ = meter.Float64Histogram(
		"db.query.duration",
		metric.WithDescription("Database query duration in milliseconds"),
		metric.WithUnit("ms"),
	)

	orderCounter, _ = meter.Int64Counter(
		"orders.created.total",
		metric.WithDescription("Total number of orders created"),
		metric.WithUnit("{order}"),
	)

	logInfo("", "Telemetry initialized", map[string]interface{}{
		"endpoint": endpoint,
		"tracer":   "enabled",
		"metrics":  "enabled",
	})

	return tp, nil
}

func main() {
	ctx := context.Background()

	tp, err := initTelemetry(ctx)
	if err != nil {
		logError("", "Failed to initialize telemetry", err, nil)
		os.Exit(1)
	}
	defer func() {
		_ = tp.Shutdown(ctx)
		if meterProvider != nil {
			_ = meterProvider.Shutdown(ctx)
		}
	}()

	// Initialize PostgreSQL
	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		postgresURL = "postgres://ollystack:ollystack123@localhost:5432/orders?sslmode=disable"
	}
	db, err = sql.Open("postgres", postgresURL)
	if err != nil {
		logError("", "Failed to connect to database", err, nil)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisClient = redis.NewClient(&redis.Options{Addr: redisURL})

	// Initialize HTTP client
	httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/orders", ordersHandler)

	handler := otelhttp.NewHandler(mux, "order-service")

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logInfo("", "Order Service starting", map[string]interface{}{"port": 8080})
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
	correlationID := r.Header.Get("X-Correlation-ID")

	logInfo(correlationID, "Received request", map[string]interface{}{
		"method": r.Method,
		"path":   "/orders",
	})

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	// Metric attributes
	metricAttrs := metric.WithAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/orders"),
		attribute.String("service.name", serviceName),
	)

	// Record request count with exemplar
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
		errorCounter.Add(ctx, 1, metricAttrs)
	}

	// Record request duration with exemplar
	duration := float64(time.Since(startTime).Milliseconds())
	requestLatency.Record(ctx, duration, metricAttrs,
		metric.WithAttributes(attribute.Int("http.status_code", statusCode)),
	)
}

func listOrders(ctx context.Context, w http.ResponseWriter, r *http.Request, correlationID string) {
	ctx, span := tracer.Start(ctx, "db-query-orders")
	defer span.End()

	span.SetAttributes(attribute.String("db.system", "postgresql"))

	// DB metric attributes
	dbAttrs := metric.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.table", "orders"),
	)

	// Simulate occasional slow query
	isSlowQuery := rand.Float32() < 0.1
	if isSlowQuery {
		time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
		span.SetAttributes(attribute.Bool("slow_query", true))
	}

	queryStart := time.Now()
	rows, err := db.QueryContext(ctx, `
		SELECT id, correlation_id, customer_id, status, total_amount, created_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT 20
	`)
	queryDuration := float64(time.Since(queryStart).Milliseconds())

	// Record DB query latency with exemplar - links to the trace showing the slow query
	dbQueryLatency.Record(ctx, queryDuration, dbAttrs,
		metric.WithAttributes(attribute.Bool("slow_query", isSlowQuery)),
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Record error with exemplar
		errorCounter.Add(ctx, 1, dbAttrs)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []map[string]interface{}
	for rows.Next() {
		var id int
		var correlationID, customerID, status string
		var totalAmount float64
		var createdAt time.Time

		if err := rows.Scan(&id, &correlationID, &customerID, &status, &totalAmount, &createdAt); err != nil {
			continue
		}

		orders = append(orders, map[string]interface{}{
			"id":            id,
			"correlationId": correlationID,
			"customerId":    customerID,
			"status":        status,
			"totalAmount":   totalAmount,
			"createdAt":     createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func createOrder(ctx context.Context, w http.ResponseWriter, r *http.Request, correlationID string) {
	ctx, span := tracer.Start(ctx, "create-order")
	defer span.End()

	span.SetAttributes(
		attribute.String("correlation_id", correlationID),
		attribute.String("operation", "create_order"),
	)

	logInfo(correlationID, "Creating order", nil)

	// Order metric attributes
	orderAttrs := metric.WithAttributes(
		attribute.String("operation", "create_order"),
		attribute.String("service.name", serviceName),
	)

	var req struct {
		CorrelationID string `json:"correlationId"`
		CustomerID    string `json:"customerId"`
		Items         []struct {
			ProductID int `json:"productId"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logError(correlationID, "Invalid request body", err, nil)
		span.RecordError(err)
		// Record validation error with exemplar
		errorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("error.type", "validation_error"),
		))
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.CorrelationID != "" {
		correlationID = req.CorrelationID
	}

	logInfo(correlationID, "Order request parsed", map[string]interface{}{
		"customer_id": req.CustomerID,
		"item_count":  len(req.Items),
	})

	span.SetAttributes(
		attribute.String("customer_id", req.CustomerID),
		attribute.Int("item_count", len(req.Items)),
	)

	// Check inventory
	logInfo(correlationID, "Checking inventory", nil)
	inventoryOK, err := checkInventory(ctx, req.Items, correlationID)
	if err != nil {
		logError(correlationID, "Inventory check failed", err, nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Inventory check failed")
		http.Error(w, "Inventory check failed", http.StatusServiceUnavailable)
		return
	}

	if !inventoryOK {
		logWarn(correlationID, "Insufficient inventory", nil)
		span.SetStatus(codes.Error, "Insufficient inventory")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error":         "Insufficient inventory",
			"correlationId": correlationID,
		})
		return
	}

	logInfo(correlationID, "Inventory available", nil)

	// Calculate total amount
	totalAmount := calculateTotal(ctx, req.Items)

	// Insert order into database
	logInfo(correlationID, "Inserting order into database", map[string]interface{}{"total": totalAmount})
	var orderID int
	err = db.QueryRowContext(ctx, `
		INSERT INTO orders (correlation_id, customer_id, status, total_amount)
		VALUES ($1, $2, 'pending', $3)
		RETURNING id
	`, correlationID, req.CustomerID, totalAmount).Scan(&orderID)
	if err != nil {
		logError(correlationID, "Failed to insert order", err, nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, "Failed to create order", http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Order created", map[string]interface{}{"order_id": orderID})
	span.SetAttributes(attribute.Int("order_id", orderID))

	// Insert order items
	for _, item := range req.Items {
		_, err := db.ExecContext(ctx, `
			INSERT INTO order_items (order_id, product_id, quantity, unit_price)
			VALUES ($1, $2, $3, $4)
		`, orderID, item.ProductID, item.Quantity, 99.99)
		if err != nil {
			logError(correlationID, "Failed to insert order item", err, map[string]interface{}{"product_id": item.ProductID})
			span.RecordError(err)
		}
	}

	// Process payment
	logInfo(correlationID, "Processing payment", map[string]interface{}{"order_id": orderID, "amount": totalAmount})
	paymentOK, err := processPayment(ctx, orderID, totalAmount, correlationID)
	if err != nil || !paymentOK {
		logError(correlationID, "Payment failed", fmt.Errorf("payment declined"), map[string]interface{}{"order_id": orderID})
		span.RecordError(fmt.Errorf("payment failed"))
		span.SetStatus(codes.Error, "Payment failed")

		// Record payment error with exemplar - click through to see the failing trace
		errorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("error.type", "payment_failed"),
			attribute.String("operation", "create_order"),
		))

		// Update order status to failed
		db.ExecContext(ctx, "UPDATE orders SET status = 'payment_failed' WHERE id = $1", orderID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":         "Payment failed",
			"orderId":       orderID,
			"correlationId": correlationID,
		})
		return
	}

	logInfo(correlationID, "Payment successful", map[string]interface{}{"order_id": orderID})

	// Update order status to completed
	db.ExecContext(ctx, "UPDATE orders SET status = 'completed' WHERE id = $1", orderID)

	// Record successful order with exemplar - links to the trace for this order
	orderCounter.Add(ctx, 1, orderAttrs,
		metric.WithAttributes(
			attribute.String("status", "completed"),
			attribute.Float64("total_amount", totalAmount),
		),
	)

	// Send notification
	logInfo(correlationID, "Sending notification", map[string]interface{}{"order_id": orderID, "customer_id": req.CustomerID})
	go sendNotification(context.Background(), orderID, req.CustomerID, correlationID)

	logInfo(correlationID, "Order completed successfully", map[string]interface{}{
		"order_id": orderID,
		"total":    totalAmount,
		"status":   "completed",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"orderId":       orderID,
		"correlationId": correlationID,
		"status":        "completed",
		"totalAmount":   totalAmount,
	})
}

func checkInventory(ctx context.Context, items []struct {
	ProductID int `json:"productId"`
	Quantity  int `json:"quantity"`
}, correlationID string) (bool, error) {
	ctx, span := tracer.Start(ctx, "check-inventory")
	defer span.End()

	inventoryURL := os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryURL == "" {
		inventoryURL = "http://localhost:8084"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"items":         items,
		"correlationId": correlationID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", inventoryURL+"/check", bytes.NewReader(body))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", correlationID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result struct {
		Available bool `json:"available"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	span.SetAttributes(attribute.Bool("inventory_available", result.Available))
	return result.Available, nil
}

func calculateTotal(ctx context.Context, items []struct {
	ProductID int `json:"productId"`
	Quantity  int `json:"quantity"`
}) float64 {
	_, span := tracer.Start(ctx, "calculate-total")
	defer span.End()

	total := 0.0
	for _, item := range items {
		// Simulate price lookup
		price := 99.99 + float64(item.ProductID)*10
		total += price * float64(item.Quantity)
	}

	span.SetAttributes(attribute.Float64("total_amount", total))
	return total
}

func processPayment(ctx context.Context, orderID int, amount float64, correlationID string) (bool, error) {
	ctx, span := tracer.Start(ctx, "process-payment")
	defer span.End()

	paymentURL := os.Getenv("PAYMENT_SERVICE_URL")
	if paymentURL == "" {
		paymentURL = "http://localhost:8083"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"orderId":       orderID,
		"amount":        amount,
		"correlationId": correlationID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", paymentURL+"/process", bytes.NewReader(body))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", correlationID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, "Payment rejected")
		return false, nil
	}

	return true, nil
}

func sendNotification(ctx context.Context, orderID int, customerID, correlationID string) {
	ctx, span := tracer.Start(ctx, "send-notification")
	defer span.End()

	notificationURL := os.Getenv("NOTIFICATION_SERVICE_URL")
	if notificationURL == "" {
		notificationURL = "http://localhost:8085"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"type":          "order_completed",
		"orderId":       orderID,
		"customerId":    customerID,
		"correlationId": correlationID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", notificationURL+"/send", bytes.NewReader(body))
	if err != nil {
		span.RecordError(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", correlationID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, "Notification failed")
	}
}
