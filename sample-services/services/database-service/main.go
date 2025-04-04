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
	serviceName = "database-service"
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
			attribute.String("db.system", "postgresql"),
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

// Simulated database operations
func simulateQuery(ctx context.Context, query string, table string) (time.Duration, error) {
	_, span := tracer.Start(ctx, "db.query",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.sql.table", table),
			attribute.String("db.statement", query),
		))
	defer span.End()

	// Simulate query execution time (5-50ms for simple queries, longer for complex)
	baseLatency := 5 + rand.Intn(45)
	if rand.Float64() < 0.1 { // 10% chance of slow query
		baseLatency += 100 + rand.Intn(200)
		span.SetAttributes(attribute.Bool("db.slow_query", true))
	}
	latency := time.Duration(baseLatency) * time.Millisecond
	time.Sleep(latency)

	// Simulate random failures
	if rand.Float64() < failureRate {
		err := &DBError{Code: "23505", Message: "duplicate key value violates unique constraint"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateInsert(ctx context.Context, table string, data map[string]interface{}) (time.Duration, error) {
	_, span := tracer.Start(ctx, "db.insert",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "INSERT"),
			attribute.String("db.sql.table", table),
		))
	defer span.End()

	// Simulate insert time
	latency := time.Duration(10+rand.Intn(30)) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate {
		err := &DBError{Code: "23503", Message: "foreign key constraint violation"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	span.SetAttributes(attribute.Int("db.rows_affected", 1))
	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateUpdate(ctx context.Context, table string, condition string) (time.Duration, error) {
	_, span := tracer.Start(ctx, "db.update",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
			attribute.String("db.sql.table", table),
		))
	defer span.End()

	latency := time.Duration(15+rand.Intn(40)) * time.Millisecond
	time.Sleep(latency)

	if rand.Float64() < failureRate*0.5 { // Lower failure rate for updates
		err := &DBError{Code: "40001", Message: "deadlock detected"}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return latency, err
	}

	rowsAffected := 1 + rand.Intn(5)
	span.SetAttributes(attribute.Int("db.rows_affected", rowsAffected))
	span.SetStatus(codes.Ok, "")
	return latency, nil
}

func simulateTransaction(ctx context.Context, operations int) (time.Duration, error) {
	ctx, span := tracer.Start(ctx, "db.transaction",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "TRANSACTION"),
			attribute.Int("db.transaction.operations", operations),
		))
	defer span.End()

	var totalLatency time.Duration

	// Begin transaction
	time.Sleep(2 * time.Millisecond)
	totalLatency += 2 * time.Millisecond

	// Execute operations
	tables := []string{"orders", "inventory", "payments", "users", "products"}
	for i := 0; i < operations; i++ {
		table := tables[rand.Intn(len(tables))]
		lat, err := simulateQuery(ctx, "SELECT * FROM "+table, table)
		totalLatency += lat
		if err != nil {
			// Rollback on error
			span.SetAttributes(attribute.Bool("db.transaction.rolled_back", true))
			span.SetStatus(codes.Error, "transaction rolled back")
			return totalLatency, err
		}
	}

	// Commit
	time.Sleep(5 * time.Millisecond)
	totalLatency += 5 * time.Millisecond

	span.SetAttributes(attribute.Bool("db.transaction.committed", true))
	span.SetStatus(codes.Ok, "")
	return totalLatency, nil
}

type DBError struct {
	Code    string
	Message string
}

func (e *DBError) Error() string {
	return e.Code + ": " + e.Message
}

// HTTP Handlers
func queryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	table := r.URL.Query().Get("table")
	if table == "" {
		table = "orders"
	}

	query := "SELECT * FROM " + table + " WHERE id = $1"
	logInfo(correlationID, "Executing query", map[string]interface{}{"table": table})

	latency, err := simulateQuery(ctx, query, table)
	if err != nil {
		logError(correlationID, "Query failed", err, map[string]interface{}{"table": table})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Query completed", map[string]interface{}{
		"table":      table,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"table":      table,
		"latency_ms": latency.Milliseconds(),
		"rows":       rand.Intn(10) + 1,
	})
}

func insertHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	table := r.URL.Query().Get("table")
	if table == "" {
		table = "orders"
	}

	logInfo(correlationID, "Executing insert", map[string]interface{}{"table": table})

	latency, err := simulateInsert(ctx, table, map[string]interface{}{"id": rand.Int()})
	if err != nil {
		logError(correlationID, "Insert failed", err, map[string]interface{}{"table": table})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Insert completed", map[string]interface{}{
		"table":      table,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"table":      table,
		"latency_ms": latency.Milliseconds(),
	})
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	table := r.URL.Query().Get("table")
	if table == "" {
		table = "orders"
	}

	logInfo(correlationID, "Executing update", map[string]interface{}{"table": table})

	latency, err := simulateUpdate(ctx, table, "id = $1")
	if err != nil {
		logError(correlationID, "Update failed", err, map[string]interface{}{"table": table})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Update completed", map[string]interface{}{
		"table":      table,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"table":      table,
		"latency_ms": latency.Milliseconds(),
	})
}

func transactionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := getCorrelationID(r)

	// Set correlation_id on the current span
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	ops := 3
	if o := r.URL.Query().Get("operations"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			ops = parsed
		}
	}

	logInfo(correlationID, "Starting transaction", map[string]interface{}{"operations": ops})

	latency, err := simulateTransaction(ctx, ops)
	if err != nil {
		logError(correlationID, "Transaction failed", err, map[string]interface{}{"operations": ops})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logInfo(correlationID, "Transaction completed", map[string]interface{}{
		"operations": ops,
		"latency_ms": latency.Milliseconds(),
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"operations": ops,
		"latency_ms": latency.Milliseconds(),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": serviceName,
		"db":      "postgresql",
	})
}

func main() {
	ctx := context.Background()

	// Initialize failure rate
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
	mux.Handle("/query", otelhttp.NewHandler(http.HandlerFunc(queryHandler), "db.query"))
	mux.Handle("/insert", otelhttp.NewHandler(http.HandlerFunc(insertHandler), "db.insert"))
	mux.Handle("/update", otelhttp.NewHandler(http.HandlerFunc(updateHandler), "db.update"))
	mux.Handle("/transaction", otelhttp.NewHandler(http.HandlerFunc(transactionHandler), "db.transaction"))
	mux.HandleFunc("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logInfo("", "Database service starting", map[string]interface{}{"port": port})
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
