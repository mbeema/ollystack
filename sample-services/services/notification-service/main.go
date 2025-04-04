package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
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
	serviceName = "notification-service"
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

	tracer = tp.Tracer("notification-service")
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

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisClient = redis.NewClient(&redis.Options{Addr: redisURL})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/send", sendHandler)
	mux.HandleFunc("/status", statusHandler)

	handler := otelhttp.NewHandler(mux, "notification-service")

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logInfo("", "Notification Service starting", map[string]interface{}{"port": 8080})
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

func sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	correlationID := r.Header.Get("X-Correlation-ID")

	ctx, span := tracer.Start(ctx, "send-notification")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	logInfo(correlationID, "Sending notification", nil)

	var req struct {
		Type          string `json:"type"`
		OrderID       int    `json:"orderId"`
		CustomerID    string `json:"customerId"`
		CorrelationID string `json:"correlationId"`
		Message       string `json:"message"`
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

	logInfo(correlationID, "Notification request parsed", map[string]interface{}{
		"type":        req.Type,
		"order_id":    req.OrderID,
		"customer_id": req.CustomerID,
	})

	span.SetAttributes(
		attribute.String("notification_type", req.Type),
		attribute.Int("order_id", req.OrderID),
		attribute.String("customer_id", req.CustomerID),
	)

	// Simulate sending notification through different channels
	notificationID := generateNotificationID()

	// Email notification
	logInfo(correlationID, "Sending email notification", map[string]interface{}{"customer_id": req.CustomerID})
	if err := sendEmail(ctx, req.CustomerID, req.Type, req.OrderID); err != nil {
		logError(correlationID, "Email notification failed", err, nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Email notification failed")
	} else {
		logInfo(correlationID, "Email sent successfully", nil)
	}

	// SMS notification (simulated)
	logInfo(correlationID, "Sending SMS notification", map[string]interface{}{"customer_id": req.CustomerID})
	if err := sendSMS(ctx, req.CustomerID, req.Type); err != nil {
		logWarn(correlationID, "SMS notification failed", map[string]interface{}{"error": err.Error()})
		span.RecordError(err)
		span.AddEvent("SMS notification failed", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	} else {
		logInfo(correlationID, "SMS sent successfully", nil)
	}

	// Push notification (simulated)
	logInfo(correlationID, "Sending push notification", map[string]interface{}{"customer_id": req.CustomerID})
	sendPush(ctx, req.CustomerID, req.Type, req.OrderID)
	logInfo(correlationID, "Push notification sent", nil)

	// Store notification in Redis
	notificationData, _ := json.Marshal(map[string]interface{}{
		"id":            notificationID,
		"type":          req.Type,
		"orderId":       req.OrderID,
		"customerId":    req.CustomerID,
		"correlationId": correlationID,
		"status":        "sent",
		"sentAt":        time.Now().Format(time.RFC3339),
	})
	redisClient.Set(ctx, "notification:"+notificationID, notificationData, 24*time.Hour)
	redisClient.LPush(ctx, "notifications:"+req.CustomerID, notificationID)

	logInfo(correlationID, "Notification completed", map[string]interface{}{
		"notification_id": notificationID,
		"channels":        []string{"email", "sms", "push"},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notificationId": notificationID,
		"status":         "sent",
		"channels":       []string{"email", "sms", "push"},
		"correlationId":  correlationID,
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	notificationID := r.URL.Query().Get("id")

	ctx, span := tracer.Start(ctx, "get-notification-status")
	defer span.End()

	span.SetAttributes(attribute.String("notification_id", notificationID))

	data, err := redisClient.Get(ctx, "notification:"+notificationID).Bytes()
	if err != nil {
		span.RecordError(err)
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func sendEmail(ctx context.Context, customerID, notificationType string, orderID int) error {
	ctx, span := tracer.Start(ctx, "send-email")
	defer span.End()

	span.SetAttributes(
		attribute.String("email.type", notificationType),
		attribute.String("email.recipient", customerID),
	)

	// Simulate email sending latency
	time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)

	// Simulate occasional failures (2%)
	if rand.Float64() < 0.02 {
		err := &NotificationError{Channel: "email", Message: "SMTP connection timeout"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.String("email.status", "delivered"))
	return nil
}

func sendSMS(ctx context.Context, customerID, notificationType string) error {
	ctx, span := tracer.Start(ctx, "send-sms")
	defer span.End()

	span.SetAttributes(
		attribute.String("sms.type", notificationType),
		attribute.String("sms.recipient", customerID),
	)

	// Simulate SMS sending latency
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

	// Simulate occasional failures (5%)
	if rand.Float64() < 0.05 {
		err := &NotificationError{Channel: "sms", Message: "SMS gateway unavailable"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.String("sms.status", "delivered"))
	return nil
}

func sendPush(ctx context.Context, customerID, notificationType string, orderID int) {
	ctx, span := tracer.Start(ctx, "send-push")
	defer span.End()

	span.SetAttributes(
		attribute.String("push.type", notificationType),
		attribute.String("push.recipient", customerID),
		attribute.Int("push.order_id", orderID),
	)

	// Simulate push notification
	time.Sleep(time.Duration(20+rand.Intn(50)) * time.Millisecond)

	span.SetAttributes(attribute.String("push.status", "delivered"))
}

func generateNotificationID() string {
	return "notif_" + randomString(12)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type NotificationError struct {
	Channel string
	Message string
}

func (e *NotificationError) Error() string {
	return e.Channel + ": " + e.Message
}
