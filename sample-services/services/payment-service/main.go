package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	redisClient *redis.Client
	failureRate float64
	serviceName = "payment-service"
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

	tracer = tp.Tracer("payment-service")
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
	failureRate = 0.05
	if fr := os.Getenv("FAILURE_RATE"); fr != "" {
		if f, err := strconv.ParseFloat(fr, 64); err == nil {
			failureRate = f
		}
	}

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisClient = redis.NewClient(&redis.Options{Addr: redisURL})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/process", processHandler)
	mux.HandleFunc("/refund", refundHandler)

	handler := otelhttp.NewHandler(mux, "payment-service")

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logInfo("", "Payment Service starting", map[string]interface{}{"port": 8080, "failure_rate": failureRate})
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

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "process-payment")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	logInfo(correlationID, "Processing payment request", nil)

	var req struct {
		OrderID       int     `json:"orderId"`
		Amount        float64 `json:"amount"`
		CorrelationID string  `json:"correlationId"`
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

	logInfo(correlationID, "Payment request parsed", map[string]interface{}{
		"order_id": req.OrderID,
		"amount":   req.Amount,
	})

	span.SetAttributes(
		attribute.Int("order_id", req.OrderID),
		attribute.Float64("amount", req.Amount),
	)

	// Simulate payment processing time
	processingTime := 50 + rand.Intn(200)
	time.Sleep(time.Duration(processingTime) * time.Millisecond)

	span.SetAttributes(attribute.Int("processing_time_ms", processingTime))

	// Simulate occasional failures
	if rand.Float64() < failureRate {
		logWarn(correlationID, "Payment declined", map[string]interface{}{
			"order_id": req.OrderID,
			"reason":   "insufficient_funds",
		})
		span.RecordError(errPaymentDeclined)
		span.SetStatus(codes.Error, "Payment declined")
		span.SetAttributes(
			attribute.String("payment_status", "declined"),
			attribute.String("decline_reason", "insufficient_funds"),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "declined",
			"reason":        "insufficient_funds",
			"orderId":       req.OrderID,
			"correlationId": correlationID,
		})
		return
	}

	// Simulate occasional slow payments
	if rand.Float64() < 0.1 {
		slowTime := 500 + rand.Intn(1000)
		logWarn(correlationID, "Slow payment processing", map[string]interface{}{
			"order_id":  req.OrderID,
			"slow_time": slowTime,
		})
		time.Sleep(time.Duration(slowTime) * time.Millisecond)
		span.SetAttributes(attribute.Bool("slow_payment", true))
	}

	// Generate transaction ID
	transactionID := generateTransactionID()
	span.SetAttributes(
		attribute.String("payment_status", "approved"),
		attribute.String("transaction_id", transactionID),
	)

	// Cache transaction in Redis
	redisClient.Set(ctx, "payment:"+transactionID, req.Amount, 24*time.Hour)

	logInfo(correlationID, "Payment approved", map[string]interface{}{
		"order_id":       req.OrderID,
		"transaction_id": transactionID,
		"amount":         req.Amount,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "approved",
		"transactionId": transactionID,
		"orderId":       req.OrderID,
		"amount":        req.Amount,
		"correlationId": correlationID,
	})
}

func refundHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "process-refund")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	var req struct {
		TransactionID string  `json:"transactionId"`
		Amount        float64 `json:"amount"`
		Reason        string  `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	span.SetAttributes(
		attribute.String("transaction_id", req.TransactionID),
		attribute.Float64("refund_amount", req.Amount),
		attribute.String("refund_reason", req.Reason),
	)

	// Simulate refund processing
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

	refundID := generateTransactionID()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "refunded",
		"refundId":      refundID,
		"transactionId": req.TransactionID,
		"amount":        req.Amount,
		"correlationId": correlationID,
	})
}

func generateTransactionID() string {
	return "txn_" + randomString(16)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

var errPaymentDeclined = &PaymentError{Message: "payment declined"}

type PaymentError struct {
	Message string
}

func (e *PaymentError) Error() string {
	return e.Message
}
