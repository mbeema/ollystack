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
	serviceName = "cache-service"
	failureRate float64
	// Simulated cache storage
	cacheHitRate float64 = 0.85 // 85% cache hit rate
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
			attribute.String("db.system", "redis"),
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

// Simulated Redis operations
func simulateGet(ctx context.Context, key string) (time.Duration, bool, error) {
	_, span := tracer.Start(ctx, "redis.GET",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET"),
			attribute.String("db.redis.key", key),
		))
	defer span.End()

	// Redis is very fast - sub-millisecond typically
	latency := time.Duration(100+rand.Intn(900)) * time.Microsecond
	if rand.Float64() < 0.02 { // 2% slow requests
		latency = time.Duration(5+rand.Intn(20)) * time.Millisecond
	}
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &CacheError{Code: "CLUSTERDOWN", Message: "The cluster is down"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, false, err
	}

	// Simulate cache hit/miss
	hit := rand.Float64() < cacheHitRate
	span.SetAttributes(attribute.Bool("cache.hit", hit))
	span.SetStatus(codes.Ok, "")
	return latency, hit, nil
}

func simulateSet(ctx context.Context, key string, ttl int) (time.Duration, error) {
	_, span := tracer.Start(ctx, "redis.SET",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SET"),
			attribute.String("db.redis.key", key),
			attribute.Int("db.redis.ttl", ttl),
		))
	defer span.End()

	latency := time.Duration(200+rand.Intn(800)) * time.Microsecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &CacheError{Code: "OOM", Message: "Out of memory"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateDelete(ctx context.Context, key string) (time.Duration, error) {
	_, span := tracer.Start(ctx, "redis.DEL",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "DEL"),
			attribute.String("db.redis.key", key),
		))
	defer span.End()

	latency := time.Duration(100+rand.Intn(400)) * time.Microsecond
	time.Sleep(latency)

	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateMGet(ctx context.Context, keys []string) (time.Duration, int, error) {
	_, span := tracer.Start(ctx, "redis.MGET",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "MGET"),
			attribute.Int("db.redis.key_count", len(keys)),
		))
	defer span.End()

	// Latency scales with number of keys
	latency := time.Duration(200+len(keys)*50+rand.Intn(500)) * time.Microsecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &CacheError{Code: "CLUSTERDOWN", Message: "The cluster is down"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, 0, err
	}

	// Return number of hits
	hits := 0
	for range keys {
		if rand.Float64() < cacheHitRate {
			hits++
		}
	}
	span.SetAttributes(attribute.Int("cache.hits", hits))
	span.SetAttributes(attribute.Int("cache.misses", len(keys)-hits))
	span.SetStatus(codes.Ok, "")
	return latency, hits, nil
}

func simulateIncr(ctx context.Context, key string) (time.Duration, int64, error) {
	_, span := tracer.Start(ctx, "redis.INCR",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "INCR"),
			attribute.String("db.redis.key", key),
		))
	defer span.End()

	latency := time.Duration(100+rand.Intn(300)) * time.Microsecond
	time.Sleep(latency)

	value := rand.Int63n(1000000)
	span.SetAttributes(attribute.Int64("db.redis.value", value))
	span.SetStatus(codes.Ok, "")
	return latency, value, nil
}

type CacheError struct {
	Code    string
	Message string
}

func (e *CacheError) Error() string {
	return e.Code + ": " + e.Message
}

// HTTP Handlers
func getHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	key := r.URL.Query().Get("key")
	if key == "" {
		key = "user:session:12345"
	}

	logInfo(correlationID, "Cache GET request", map[string]interface{}{"key": key})

	latency, hit, err := simulateGet(ctx, key)
	if err != nil {
		logError(correlationID, "Cache GET failed", err, map[string]interface{}{"key": key})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Cache GET completed", map[string]interface{}{
		"key":        key,
		"hit":        hit,
		"latency_us": latency.Microseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"key":        key,
		"hit":        hit,
		"latency_us": latency.Microseconds(),
	})
}

func setHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	key := r.URL.Query().Get("key")
	if key == "" {
		key = "user:session:12345"
	}
	ttl := 3600
	if t := r.URL.Query().Get("ttl"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil {
			ttl = parsed
		}
	}

	logInfo(correlationID, "Cache SET request", map[string]interface{}{"key": key, "ttl": ttl})

	latency, err := simulateSet(ctx, key, ttl)
	if err != nil {
		logError(correlationID, "Cache SET failed", err, map[string]interface{}{"key": key})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Cache SET completed", map[string]interface{}{
		"key":        key,
		"ttl":        ttl,
		"latency_us": latency.Microseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"key":        key,
		"ttl":        ttl,
		"latency_us": latency.Microseconds(),
	})
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	key := r.URL.Query().Get("key")
	if key == "" {
		key = "user:session:12345"
	}

	logInfo(correlationID, "Cache DEL request", map[string]interface{}{"key": key})

	latency, err := simulateDelete(ctx, key)
	if err != nil {
		logError(correlationID, "Cache DEL failed", err, map[string]interface{}{"key": key})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Cache DEL completed", map[string]interface{}{
		"key":        key,
		"latency_us": latency.Microseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"key":        key,
		"latency_us": latency.Microseconds(),
	})
}

func mgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	count := 5
	if c := r.URL.Query().Get("count"); c != "" {
		if parsed, err := strconv.Atoi(c); err == nil {
			count = parsed
		}
	}

	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = "item:" + strconv.Itoa(rand.Intn(1000))
	}

	logInfo(correlationID, "Cache MGET request", map[string]interface{}{"count": count})

	latency, hits, err := simulateMGet(ctx, keys)
	if err != nil {
		logError(correlationID, "Cache MGET failed", err, map[string]interface{}{"count": count})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Cache MGET completed", map[string]interface{}{
		"count":      count,
		"hits":       hits,
		"misses":     count - hits,
		"latency_us": latency.Microseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"count":      count,
		"hits":       hits,
		"misses":     count - hits,
		"latency_us": latency.Microseconds(),
	})
}

func incrHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	key := r.URL.Query().Get("key")
	if key == "" {
		key = "counter:page_views"
	}

	logInfo(correlationID, "Cache INCR request", map[string]interface{}{"key": key})

	latency, value, err := simulateIncr(ctx, key)
	if err != nil {
		logError(correlationID, "Cache INCR failed", err, map[string]interface{}{"key": key})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Cache INCR completed", map[string]interface{}{
		"key":        key,
		"value":      value,
		"latency_us": latency.Microseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"key":        key,
		"value":      value,
		"latency_us": latency.Microseconds(),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": serviceName,
		"cache":   "redis-compatible",
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
		failureRate = 0.005 // 0.5% default - caches should be reliable
	}

	tp, err := initTracer(ctx)
	if err != nil {
		logJSON("error", "", "Failed to initialize tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer tp.Shutdown(ctx)

	tracer = otel.Tracer(serviceName)

	mux := http.NewServeMux()
	mux.Handle("/get", otelhttp.NewHandler(http.HandlerFunc(getHandler), "redis.GET"))
	mux.Handle("/set", otelhttp.NewHandler(http.HandlerFunc(setHandler), "redis.SET"))
	mux.Handle("/delete", otelhttp.NewHandler(http.HandlerFunc(deleteHandler), "redis.DEL"))
	mux.Handle("/mget", otelhttp.NewHandler(http.HandlerFunc(mgetHandler), "redis.MGET"))
	mux.Handle("/incr", otelhttp.NewHandler(http.HandlerFunc(incrHandler), "redis.INCR"))
	mux.HandleFunc("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logInfo("", "Cache service starting", map[string]interface{}{"port": port})
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
