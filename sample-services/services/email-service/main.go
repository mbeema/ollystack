package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	serviceName = "email-service"
	failureRate float64
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
			attribute.String("messaging.system", "smtp"),
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

	return tp, nil
}

func getCorrelationID(r *http.Request) string {
	if cid := r.Header.Get("X-Correlation-ID"); cid != "" {
		return cid
	}
	return trace.SpanFromContext(r.Context()).SpanContext().TraceID().String()
}

// Simulated email operations
func simulateSendEmail(ctx context.Context, to, subject, template string) (time.Duration, string, error) {
	ctx, span := tracer.Start(ctx, "email.send",
		trace.WithAttributes(
			attribute.String("messaging.system", "smtp"),
			attribute.String("messaging.operation", "send"),
			attribute.String("email.to", to),
			attribute.String("email.subject", subject),
			attribute.String("email.template", template),
		))
	defer span.End()

	// Simulate DNS lookup
	_, dnsSpan := tracer.Start(ctx, "dns.lookup",
		trace.WithAttributes(
			attribute.String("dns.hostname", "smtp.provider.com"),
		))
	time.Sleep(time.Duration(2+rand.Intn(5)) * time.Millisecond)
	dnsSpan.End()

	// Simulate SMTP connection
	_, smtpSpan := tracer.Start(ctx, "smtp.connect",
		trace.WithAttributes(
			attribute.String("net.peer.name", "smtp.provider.com"),
			attribute.Int("net.peer.port", 587),
		))
	time.Sleep(time.Duration(20+rand.Intn(30)) * time.Millisecond)
	smtpSpan.End()

	// Simulate template rendering
	_, renderSpan := tracer.Start(ctx, "template.render",
		trace.WithAttributes(
			attribute.String("template.name", template),
		))
	time.Sleep(time.Duration(5+rand.Intn(10)) * time.Millisecond)
	renderSpan.End()

	// Simulate sending
	sendLatency := time.Duration(50+rand.Intn(100)) * time.Millisecond
	if rand.Float64() < 0.1 { // 10% slow sends
		sendLatency += time.Duration(200+rand.Intn(300)) * time.Millisecond
	}
	time.Sleep(sendLatency)

	totalLatency := sendLatency + 50*time.Millisecond // approximate total

	if rand.Float64() < failureRate {
		err := &EmailError{Code: "550", Message: "Mailbox not found"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return totalLatency, "", err
	}

	messageID := fmt.Sprintf("<%d.%d@ollystack.io>", time.Now().UnixNano(), rand.Int63())
	span.SetAttributes(attribute.String("email.message_id", messageID))
	span.SetStatus(codes.Ok, "")
	return totalLatency, messageID, nil
}

func simulateSendBulk(ctx context.Context, recipients int, template string) (time.Duration, int, int, error) {
	_, span := tracer.Start(ctx, "email.send_bulk",
		trace.WithAttributes(
			attribute.String("messaging.system", "smtp"),
			attribute.String("messaging.operation", "send_bulk"),
			attribute.Int("email.recipient_count", recipients),
			attribute.String("email.template", template),
		))
	defer span.End()

	var totalLatency time.Duration
	sent := 0
	failed := 0

	// Simulate batched sending
	batchSize := 10
	for i := 0; i < recipients; i += batchSize {
		batch := batchSize
		if i+batchSize > recipients {
			batch = recipients - i
		}

		// Simulate batch send
		batchLatency := time.Duration(100+rand.Intn(200)) * time.Millisecond
		time.Sleep(batchLatency)
		totalLatency += batchLatency

		// Simulate some failures in batch
		for j := 0; j < batch; j++ {
			if rand.Float64() < failureRate*2 { // Higher failure rate for bulk
				failed++
			} else {
				sent++
			}
		}
	}

	span.SetAttributes(
		attribute.Int("email.sent", sent),
		attribute.Int("email.failed", failed),
	)

	if failed > sent {
		span.SetStatus(codes.Error, "More failures than successes")
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return totalLatency, sent, failed, nil
}

func simulateValidateEmail(ctx context.Context, email string) (time.Duration, bool, error) {
	_, span := tracer.Start(ctx, "email.validate",
		trace.WithAttributes(
			attribute.String("email.address", email),
		))
	defer span.End()

	// Simulate MX lookup
	latency := time.Duration(10+rand.Intn(20)) * time.Millisecond
	time.Sleep(latency)

	// 95% valid emails
	valid := rand.Float64() < 0.95
	span.SetAttributes(attribute.Bool("email.valid", valid))
	span.SetStatus(codes.Ok, "")
	return latency, valid, nil
}

func simulateCheckBounce(ctx context.Context, messageID string) (time.Duration, bool, string, error) {
	_, span := tracer.Start(ctx, "email.check_bounce",
		trace.WithAttributes(
			attribute.String("email.message_id", messageID),
		))
	defer span.End()

	latency := time.Duration(5+rand.Intn(15)) * time.Millisecond
	time.Sleep(latency)

	// 2% bounce rate
	bounced := rand.Float64() < 0.02
	bounceType := ""
	if bounced {
		types := []string{"hard", "soft", "complaint"}
		bounceType = types[rand.Intn(len(types))]
		span.SetAttributes(attribute.String("email.bounce_type", bounceType))
	}

	span.SetAttributes(attribute.Bool("email.bounced", bounced))
	span.SetStatus(codes.Ok, "")
	return latency, bounced, bounceType, nil
}

type EmailError struct {
	Code    string
	Message string
}

func (e *EmailError) Error() string {
	return e.Code + ": " + e.Message
}

// HTTP Handlers
func sendHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	to := r.URL.Query().Get("to")
	if to == "" {
		to = "user@example.com"
	}
	subject := r.URL.Query().Get("subject")
	if subject == "" {
		subject = "Order Confirmation"
	}
	template := r.URL.Query().Get("template")
	if template == "" {
		template = "order_confirmation"
	}

	logInfo(correlationID, "Send email request", map[string]interface{}{
		"to":       to,
		"subject":  subject,
		"template": template,
	})

	latency, messageID, err := simulateSendEmail(ctx, to, subject, template)
	if err != nil {
		logError(correlationID, "Send email failed", err, map[string]interface{}{"to": to})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Email sent successfully", map[string]interface{}{
		"to":         to,
		"message_id": messageID,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"message_id": messageID,
		"to":         to,
		"latency_ms": latency.Milliseconds(),
	})
}

func sendBulkHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	recipients := 50
	if c := r.URL.Query().Get("count"); c != "" {
		if parsed, err := strconv.Atoi(c); err == nil {
			recipients = parsed
		}
	}
	template := r.URL.Query().Get("template")
	if template == "" {
		template = "newsletter"
	}

	logInfo(correlationID, "Bulk email request", map[string]interface{}{
		"recipients": recipients,
		"template":   template,
	})

	latency, sent, failed, err := simulateSendBulk(ctx, recipients, template)
	if err != nil {
		logError(correlationID, "Bulk email failed", err, map[string]interface{}{"recipients": recipients})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if failed > 0 {
		logWarn(correlationID, "Bulk email completed with failures", map[string]interface{}{
			"sent":       sent,
			"failed":     failed,
			"latency_ms": latency.Milliseconds(),
		})
	} else {
		logInfo(correlationID, "Bulk email completed", map[string]interface{}{
			"sent":       sent,
			"latency_ms": latency.Milliseconds(),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"sent":       sent,
		"failed":     failed,
		"latency_ms": latency.Milliseconds(),
	})
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	email := r.URL.Query().Get("email")
	if email == "" {
		email = "test@example.com"
	}

	logInfo(correlationID, "Validate email request", map[string]interface{}{"email": email})

	latency, valid, err := simulateValidateEmail(ctx, email)
	if err != nil {
		logError(correlationID, "Validate email failed", err, map[string]interface{}{"email": email})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Email validation completed", map[string]interface{}{
		"email":      email,
		"valid":      valid,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"email":      email,
		"valid":      valid,
		"latency_ms": latency.Milliseconds(),
	})
}

func bounceCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	messageID := r.URL.Query().Get("message_id")
	if messageID == "" {
		messageID = fmt.Sprintf("<%d@ollystack.io>", rand.Int63())
	}

	logInfo(correlationID, "Check bounce request", map[string]interface{}{"message_id": messageID})

	latency, bounced, bounceType, err := simulateCheckBounce(ctx, messageID)
	if err != nil {
		logError(correlationID, "Check bounce failed", err, map[string]interface{}{"message_id": messageID})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Bounce check completed", map[string]interface{}{
		"message_id":  messageID,
		"bounced":     bounced,
		"bounce_type": bounceType,
		"latency_ms":  latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"message_id":  messageID,
		"bounced":     bounced,
		"bounce_type": bounceType,
		"latency_ms":  latency.Milliseconds(),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": serviceName,
		"smtp":    "configured",
	})
}

func main() {
	ctx := context.Background()

	if fr := os.Getenv("FAILURE_RATE"); fr != "" {
		if f, err := strconv.ParseFloat(fr, 64); err == nil {
			failureRate = f
		}
	}
	if failureRate == 0 {
		failureRate = 0.02 // 2% default
	}

	tp, err := initTracer(ctx)
	if err != nil {
		logJSON("error", "", "Failed to initialize tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer tp.Shutdown(ctx)

	tracer = otel.Tracer(serviceName)

	mux := http.NewServeMux()
	mux.Handle("/send", otelhttp.NewHandler(http.HandlerFunc(sendHandler), "email.send"))
	mux.Handle("/send-bulk", otelhttp.NewHandler(http.HandlerFunc(sendBulkHandler), "email.send_bulk"))
	mux.Handle("/validate", otelhttp.NewHandler(http.HandlerFunc(validateHandler), "email.validate"))
	mux.Handle("/bounce-check", otelhttp.NewHandler(http.HandlerFunc(bounceCheckHandler), "email.bounce_check"))
	mux.HandleFunc("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logInfo("", "Email service starting", map[string]interface{}{"port": port})
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logJSON("error", "", "Server error", map[string]interface{}{"error": err.Error()})
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logInfo("", "Shutting down server", nil)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
