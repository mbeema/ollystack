package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer      trace.Tracer
	db          *sql.DB
	redisClient *redis.Client
	failureRate float64
	serviceName = "inventory-service"
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

func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
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
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer("inventory-service")
	return tp, nil
}

func main() {
	ctx := context.Background()

	tp, err := initTracer(ctx)
	if err != nil {
		logError("", "Failed to initialize tracer", err, nil)
		os.Exit(1)
	}
	defer func() { _ = tp.Shutdown(ctx) }()

	// Parse failure rate
	failureRate = 0.03
	if fr := os.Getenv("FAILURE_RATE"); fr != "" {
		if f, err := strconv.ParseFloat(fr, 64); err == nil {
			failureRate = f
		}
	}

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

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/check", checkHandler)
	mux.HandleFunc("/reserve", reserveHandler)
	mux.HandleFunc("/release", releaseHandler)
	mux.HandleFunc("/stock", stockHandler)

	handler := otelhttp.NewHandler(mux, "inventory-service")

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logInfo("", "Inventory Service starting", map[string]interface{}{"port": 8080, "failure_rate": failureRate})
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

func checkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "check-inventory")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	logInfo(correlationID, "Checking inventory", nil)

	var req struct {
		Items []struct {
			ProductID int `json:"productId"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
		CorrelationID string `json:"correlationId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logError(correlationID, "Invalid request body", err, nil)
		span.RecordError(err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.CorrelationID != "" {
		correlationID = req.CorrelationID
	}

	logInfo(correlationID, "Inventory check request parsed", map[string]interface{}{"item_count": len(req.Items)})

	span.SetAttributes(attribute.Int("item_count", len(req.Items)))

	// Simulate occasional service failures
	if rand.Float64() < failureRate {
		logError(correlationID, "Service temporarily unavailable", errServiceUnavailable, nil)
		span.RecordError(errServiceUnavailable)
		span.SetStatus(codes.Error, "Service temporarily unavailable")
		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	// Simulate database latency
	queryTime := 10 + rand.Intn(50)
	time.Sleep(time.Duration(queryTime) * time.Millisecond)

	// Check inventory for each item
	available := true
	unavailableItems := []int{}

	for _, item := range req.Items {
		ctx, itemSpan := tracer.Start(ctx, "check-item-stock")
		itemSpan.SetAttributes(
			attribute.Int("product_id", item.ProductID),
			attribute.Int("requested_quantity", item.Quantity),
		)

		// Try cache first
		cacheKey := "inventory:" + strconv.Itoa(item.ProductID)
		cachedQty, err := redisClient.Get(ctx, cacheKey).Int()
		if err == nil {
			itemSpan.SetAttributes(attribute.Bool("cache_hit", true))
			if cachedQty < item.Quantity {
				available = false
				unavailableItems = append(unavailableItems, item.ProductID)
			}
			itemSpan.End()
			continue
		}

		// Query database
		var qty int
		err = db.QueryRowContext(ctx, `
			SELECT quantity - reserved
			FROM inventory
			WHERE product_id = $1
		`, item.ProductID).Scan(&qty)

		if err != nil || qty < item.Quantity {
			available = false
			unavailableItems = append(unavailableItems, item.ProductID)
			itemSpan.SetAttributes(attribute.Bool("available", false))
		} else {
			// Cache the result
			redisClient.Set(ctx, cacheKey, qty, 5*time.Minute)
			itemSpan.SetAttributes(
				attribute.Bool("available", true),
				attribute.Int("current_stock", qty),
			)
		}

		itemSpan.End()
	}

	span.SetAttributes(attribute.Bool("all_available", available))

	w.Header().Set("Content-Type", "application/json")
	if !available {
		logWarn(correlationID, "Inventory unavailable", map[string]interface{}{
			"unavailable_items": unavailableItems,
		})
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"available":        false,
			"unavailableItems": unavailableItems,
			"correlationId":    correlationID,
		})
		return
	}

	logInfo(correlationID, "Inventory available", map[string]interface{}{"all_available": true})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available":     true,
		"correlationId": correlationID,
	})
}

func reserveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "reserve-inventory")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	var req struct {
		Items []struct {
			ProductID int `json:"productId"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
		OrderID int `json:"orderId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Reserve inventory for each item
	for _, item := range req.Items {
		_, err := db.ExecContext(ctx, `
			UPDATE inventory
			SET reserved = reserved + $1
			WHERE product_id = $2 AND quantity - reserved >= $1
		`, item.Quantity, item.ProductID)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to reserve inventory")
			http.Error(w, "Failed to reserve inventory", http.StatusConflict)
			return
		}

		// Invalidate cache
		cacheKey := "inventory:" + strconv.Itoa(item.ProductID)
		redisClient.Del(ctx, cacheKey)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reserved":      true,
		"orderId":       req.OrderID,
		"correlationId": correlationID,
	})
}

func releaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "release-inventory")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	var req struct {
		Items []struct {
			ProductID int `json:"productId"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
		OrderID int `json:"orderId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Release reserved inventory
	for _, item := range req.Items {
		db.ExecContext(ctx, `
			UPDATE inventory
			SET reserved = GREATEST(0, reserved - $1)
			WHERE product_id = $2
		`, item.Quantity, item.ProductID)

		// Invalidate cache
		cacheKey := "inventory:" + strconv.Itoa(item.ProductID)
		redisClient.Del(ctx, cacheKey)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"released":      true,
		"orderId":       req.OrderID,
		"correlationId": correlationID,
	})
}

func stockHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "get-stock-levels")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.sku, p.name, i.quantity, i.reserved
		FROM products p
		JOIN inventory i ON p.id = i.product_id
		ORDER BY p.id
	`)
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stock []map[string]interface{}
	for rows.Next() {
		var id, quantity, reserved int
		var sku, name string
		if err := rows.Scan(&id, &sku, &name, &quantity, &reserved); err != nil {
			continue
		}
		stock = append(stock, map[string]interface{}{
			"productId": id,
			"sku":       sku,
			"name":      name,
			"quantity":  quantity,
			"reserved":  reserved,
			"available": quantity - reserved,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stock)
}

var errServiceUnavailable = &ServiceError{Message: "service temporarily unavailable"}

type ServiceError struct {
	Message string
}

func (e *ServiceError) Error() string {
	return e.Message
}
