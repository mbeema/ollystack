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
	serviceName = "storage-service"
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
			attribute.String("cloud.provider", "aws"),
			attribute.String("cloud.service", "s3"),
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

// Simulated S3 operations
func simulateGetObject(ctx context.Context, bucket, key string, size int64) (time.Duration, error) {
	_, span := tracer.Start(ctx, "s3.GetObject",
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "GetObject"),
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
			attribute.Int64("aws.s3.content_length", size),
		))
	defer span.End()

	// Simulate download time based on size (1ms per 100KB)
	baseLatency := 20 + int(size/100000)
	if rand.Float64() < 0.05 { // 5% slow requests
		baseLatency += 200 + rand.Intn(300)
	}
	latency := time.Duration(baseLatency) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &S3Error{Code: "NoSuchKey", Message: "The specified key does not exist"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulatePutObject(ctx context.Context, bucket, key string, size int64) (time.Duration, error) {
	_, span := tracer.Start(ctx, "s3.PutObject",
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "PutObject"),
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
			attribute.Int64("aws.s3.content_length", size),
		))
	defer span.End()

	// Simulate upload time based on size
	baseLatency := 30 + int(size/50000)
	latency := time.Duration(baseLatency) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &S3Error{Code: "AccessDenied", Message: "Access Denied"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetAttributes(attribute.String("aws.s3.etag", fmt.Sprintf("\"%x\"", rand.Int63())))
	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateDeleteObject(ctx context.Context, bucket, key string) (time.Duration, error) {
	_, span := tracer.Start(ctx, "s3.DeleteObject",
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "DeleteObject"),
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
		))
	defer span.End()

	latency := time.Duration(15+rand.Intn(25)) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate*0.5 {
		err := &S3Error{Code: "AccessDenied", Message: "Access Denied"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateListObjects(ctx context.Context, bucket, prefix string) (time.Duration, int, error) {
	_, span := tracer.Start(ctx, "s3.ListObjectsV2",
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "ListObjectsV2"),
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.prefix", prefix),
		))
	defer span.End()

	latency := time.Duration(25+rand.Intn(50)) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &S3Error{Code: "NoSuchBucket", Message: "The specified bucket does not exist"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, 0, err
	}

	count := rand.Intn(100) + 1
	span.SetAttributes(attribute.Int("aws.s3.key_count", count))
	span.SetStatus(codes.Ok, "")
	return latency, count, nil
}

type S3Error struct {
	Code    string
	Message string
}

func (e *S3Error) Error() string {
	return e.Code + ": " + e.Message
}

// HTTP Handlers
func getObjectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "app-data"
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = "files/document.pdf"
	}
	size := int64(1024 * (100 + rand.Intn(900))) // 100KB - 1MB

	logInfo(correlationID, "GetObject request", map[string]interface{}{"bucket": bucket, "key": key})

	latency, err := simulateGetObject(ctx, bucket, key, size)
	if err != nil {
		logError(correlationID, "GetObject failed", err, map[string]interface{}{"bucket": bucket, "key": key})
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logInfo(correlationID, "GetObject completed", map[string]interface{}{
		"bucket":     bucket,
		"key":        key,
		"size":       size,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"bucket":     bucket,
		"key":        key,
		"size":       size,
		"latency_ms": latency.Milliseconds(),
	})
}

func putObjectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "app-data"
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = fmt.Sprintf("uploads/%d.dat", time.Now().UnixNano())
	}
	size := int64(1024 * (50 + rand.Intn(500))) // 50KB - 550KB

	logInfo(correlationID, "PutObject request", map[string]interface{}{"bucket": bucket, "key": key, "size": size})

	latency, err := simulatePutObject(ctx, bucket, key, size)
	if err != nil {
		logError(correlationID, "PutObject failed", err, map[string]interface{}{"bucket": bucket, "key": key})
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	logInfo(correlationID, "PutObject completed", map[string]interface{}{
		"bucket":     bucket,
		"key":        key,
		"size":       size,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"bucket":     bucket,
		"key":        key,
		"size":       size,
		"latency_ms": latency.Milliseconds(),
	})
}

func deleteObjectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "app-data"
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = "temp/cleanup.dat"
	}

	logInfo(correlationID, "DeleteObject request", map[string]interface{}{"bucket": bucket, "key": key})

	latency, err := simulateDeleteObject(ctx, bucket, key)
	if err != nil {
		logError(correlationID, "DeleteObject failed", err, map[string]interface{}{"bucket": bucket, "key": key})
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	logInfo(correlationID, "DeleteObject completed", map[string]interface{}{
		"bucket":     bucket,
		"key":        key,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"bucket":     bucket,
		"key":        key,
		"latency_ms": latency.Milliseconds(),
	})
}

func listObjectsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "app-data"
	}
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		prefix = "files/"
	}

	logInfo(correlationID, "ListObjects request", map[string]interface{}{"bucket": bucket, "prefix": prefix})

	latency, count, err := simulateListObjects(ctx, bucket, prefix)
	if err != nil {
		logError(correlationID, "ListObjects failed", err, map[string]interface{}{"bucket": bucket, "prefix": prefix})
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logInfo(correlationID, "ListObjects completed", map[string]interface{}{
		"bucket":     bucket,
		"prefix":     prefix,
		"count":      count,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"bucket":     bucket,
		"prefix":     prefix,
		"count":      count,
		"latency_ms": latency.Milliseconds(),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": serviceName,
		"storage": "s3-compatible",
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
		failureRate = 0.01 // 1% default
	}

	tp, err := initTracer(ctx)
	if err != nil {
		logJSON("error", "", "Failed to initialize tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer tp.Shutdown(ctx)

	tracer = otel.Tracer(serviceName)

	mux := http.NewServeMux()
	mux.Handle("/get", otelhttp.NewHandler(http.HandlerFunc(getObjectHandler), "s3.GetObject"))
	mux.Handle("/put", otelhttp.NewHandler(http.HandlerFunc(putObjectHandler), "s3.PutObject"))
	mux.Handle("/delete", otelhttp.NewHandler(http.HandlerFunc(deleteObjectHandler), "s3.DeleteObject"))
	mux.Handle("/list", otelhttp.NewHandler(http.HandlerFunc(listObjectsHandler), "s3.ListObjects"))
	mux.HandleFunc("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8087"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logInfo("", "Storage service starting", map[string]interface{}{"port": port})
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
