package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer     trace.Tracer
	httpClient *http.Client
	// Service URLs
	apiGatewayURL      string
	databaseServiceURL string
	storageServiceURL  string
	cacheServiceURL    string
	emailServiceURL    string
)

// correlationIDKey is the context key for correlation ID
type correlationIDKeyType struct{}

var correlationIDKey = correlationIDKeyType{}

// generateCorrelationID creates a unique correlation ID for tracing requests across services
func generateCorrelationID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// withCorrelationID adds a correlation ID to the context
func withCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// getCorrelationID retrieves the correlation ID from context
func getCorrelationID(ctx context.Context) string {
	if v := ctx.Value(correlationIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// injectCorrelationID adds correlation ID to HTTP request headers
func injectCorrelationID(ctx context.Context, req *http.Request) {
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}
}

// logJSON outputs structured JSON logs with correlation ID
func logJSON(level, correlationID, msg string, extra map[string]interface{}) {
	logEntry := map[string]interface{}{
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":          level,
		"message":        msg,
		"service":        "traffic-generator",
		"correlation_id": correlationID,
	}
	for k, v := range extra {
		logEntry[k] = v
	}
	data, _ := json.Marshal(logEntry)
	fmt.Println(string(data))
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

	tracer = tp.Tracer("traffic-generator")
	return tp, nil
}

func main() {
	ctx := context.Background()

	tp, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() { _ = tp.Shutdown(ctx) }()

	httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}

	apiGatewayURL = os.Getenv("API_GATEWAY_URL")
	if apiGatewayURL == "" {
		apiGatewayURL = "http://localhost:8082"
	}

	databaseServiceURL = os.Getenv("DATABASE_SERVICE_URL")
	if databaseServiceURL == "" {
		databaseServiceURL = "http://localhost:8086"
	}

	storageServiceURL = os.Getenv("STORAGE_SERVICE_URL")
	if storageServiceURL == "" {
		storageServiceURL = "http://localhost:8087"
	}

	cacheServiceURL = os.Getenv("CACHE_SERVICE_URL")
	if cacheServiceURL == "" {
		cacheServiceURL = "http://localhost:8088"
	}

	emailServiceURL = os.Getenv("EMAIL_SERVICE_URL")
	if emailServiceURL == "" {
		emailServiceURL = "http://localhost:8089"
	}

	rps := 5.0
	if rpsStr := os.Getenv("REQUESTS_PER_SECOND"); rpsStr != "" {
		if r, err := strconv.ParseFloat(rpsStr, 64); err == nil {
			rps = r
		}
	}

	interval := time.Duration(float64(time.Second) / rps)

	log.Printf("Traffic Generator starting: api=%s, db=%s, storage=%s, cache=%s, email=%s, rps=%.1f",
		apiGatewayURL, databaseServiceURL, storageServiceURL, cacheServiceURL, emailServiceURL, rps)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	requestCount := 0
	successCount := 0
	errorCount := 0

	// Stats ticker
	statsTicker := time.NewTicker(10 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-ticker.C:
			requestCount++
			go func(reqNum int) {
				success := generateTraffic(ctx, apiGatewayURL, reqNum)
				if success {
					successCount++
				} else {
					errorCount++
				}
			}(requestCount)

		case <-statsTicker.C:
			log.Printf("Stats: total=%d, success=%d, errors=%d, error_rate=%.2f%%",
				requestCount, successCount, errorCount,
				float64(errorCount)/float64(requestCount)*100)

		case <-quit:
			log.Printf("Shutting down. Final stats: total=%d, success=%d, errors=%d",
				requestCount, successCount, errorCount)
			return
		}
	}
}

func generateTraffic(ctx context.Context, baseURL string, reqNum int) bool {
	// Generate correlation ID for this request flow
	correlationID := generateCorrelationID()
	ctx = withCorrelationID(ctx, correlationID)

	ctx, span := tracer.Start(ctx, "generate-request")
	defer span.End()

	span.SetAttributes(
		attribute.Int("request_number", reqNum),
		attribute.String("correlation_id", correlationID),
	)

	// Randomly choose request type
	requestType := chooseRequestType()
	span.SetAttributes(attribute.String("request_type", requestType))

	logJSON("info", correlationID, "Starting request", map[string]interface{}{
		"request_number": reqNum,
		"request_type":   requestType,
	})

	var success bool
	var err error

	switch requestType {
	case "list_orders":
		success, err = listOrders(ctx, baseURL)
	case "create_order":
		success, err = createOrder(ctx, baseURL)
	case "list_products":
		success, err = listProducts(ctx, baseURL)
	case "browse_products":
		success, err = browseProducts(ctx, baseURL)
	case "database_query":
		success, err = databaseQuery(ctx)
	case "database_transaction":
		success, err = databaseTransaction(ctx)
	case "storage_upload":
		success, err = storageUpload(ctx)
	case "storage_download":
		success, err = storageDownload(ctx)
	case "cache_operations":
		success, err = cacheOperations(ctx)
	case "send_email":
		success, err = sendEmail(ctx)
	}

	if err != nil {
		span.RecordError(err)
		return false
	}

	return success
}

func chooseRequestType() string {
	r := rand.Float64()
	switch {
	case r < 0.20:
		return "list_products"
	case r < 0.30:
		return "browse_products"
	case r < 0.40:
		return "list_orders"
	case r < 0.50:
		return "create_order"
	case r < 0.60:
		return "database_query"
	case r < 0.70:
		return "database_transaction"
	case r < 0.80:
		return "cache_operations"
	case r < 0.90:
		return "storage_download"
	case r < 0.95:
		return "storage_upload"
	default:
		return "send_email"
	}
}

func listOrders(ctx context.Context, baseURL string) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-list-orders")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/orders", nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	return resp.StatusCode < 400, nil
}

func createOrder(ctx context.Context, baseURL string) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-create-order")
	defer span.End()

	// Generate random order
	items := generateRandomItems()
	customerID := generateCustomerID()

	span.SetAttributes(
		attribute.String("correlation_id", correlationID),
		attribute.String("customer_id", customerID),
		attribute.Int("item_count", len(items)),
	)

	body, _ := json.Marshal(map[string]interface{}{
		"customerId": customerID,
		"items":      items,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/orders", bytes.NewReader(body))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	// 201 Created or 409 Conflict (insufficient inventory) are expected
	return resp.StatusCode == 201 || resp.StatusCode == 409, nil
}

func listProducts(ctx context.Context, baseURL string) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-list-products")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/products", nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	return resp.StatusCode < 400, nil
}

func browseProducts(ctx context.Context, baseURL string) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "browse-session")
	defer span.End()

	// Simulate a user browsing session - multiple requests
	numRequests := 2 + rand.Intn(4)
	span.SetAttributes(
		attribute.String("correlation_id", correlationID),
		attribute.Int("browse_requests", numRequests),
	)

	allSuccess := true
	for i := 0; i < numRequests; i++ {
		// Random delay between requests
		time.Sleep(time.Duration(100+rand.Intn(300)) * time.Millisecond)

		success, err := listProducts(ctx, baseURL)
		if err != nil || !success {
			allSuccess = false
		}
	}

	return allSuccess, nil
}

func generateRandomItems() []map[string]interface{} {
	numItems := 1 + rand.Intn(3)
	items := make([]map[string]interface{}, numItems)

	for i := 0; i < numItems; i++ {
		items[i] = map[string]interface{}{
			"productId": 1 + rand.Intn(10),
			"quantity":  1 + rand.Intn(3),
		}
	}

	return items
}

func generateCustomerID() string {
	// Generate realistic looking customer IDs
	prefixes := []string{"cust", "user", "buyer"}
	prefix := prefixes[rand.Intn(len(prefixes))]
	return prefix + "_" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Infrastructure service calls

func databaseQuery(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-database-query")
	defer span.End()

	tables := []string{"orders", "users", "products", "inventory", "payments"}
	table := tables[rand.Intn(len(tables))]

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "GET", databaseServiceURL+"/query?table="+table, nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("db.table", table),
	)
	return resp.StatusCode < 400, nil
}

func databaseTransaction(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-database-transaction")
	defer span.End()

	operations := 2 + rand.Intn(4)
	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "POST",
		databaseServiceURL+"/transaction?operations="+strconv.Itoa(operations), nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.Int("db.operations", operations),
	)
	return resp.StatusCode < 400, nil
}

func storageUpload(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-storage-upload")
	defer span.End()

	buckets := []string{"app-data", "user-uploads", "backups", "logs"}
	bucket := buckets[rand.Intn(len(buckets))]
	key := "files/" + randomString(12) + ".dat"

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "POST",
		storageServiceURL+"/put?bucket="+bucket+"&key="+key, nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("s3.bucket", bucket),
		attribute.String("s3.key", key),
	)
	return resp.StatusCode < 400, nil
}

func storageDownload(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-storage-download")
	defer span.End()

	buckets := []string{"app-data", "user-uploads", "static-assets"}
	bucket := buckets[rand.Intn(len(buckets))]
	keys := []string{"config.json", "template.html", "logo.png", "data.csv"}
	key := keys[rand.Intn(len(keys))]

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	req, err := http.NewRequestWithContext(ctx, "GET",
		storageServiceURL+"/get?bucket="+bucket+"&key="+key, nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("s3.bucket", bucket),
		attribute.String("s3.key", key),
	)
	return resp.StatusCode < 400, nil
}

func cacheOperations(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-cache-operations")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	// Mix of GET, SET, and MGET operations
	operations := []string{"get", "set", "mget", "incr"}
	op := operations[rand.Intn(len(operations))]

	keyTypes := []string{"user:session:", "product:", "cart:", "rate_limit:"}
	keyType := keyTypes[rand.Intn(len(keyTypes))]
	key := keyType + randomString(8)

	var url string
	switch op {
	case "get":
		url = cacheServiceURL + "/get?key=" + key
	case "set":
		url = cacheServiceURL + "/set?key=" + key + "&ttl=3600"
	case "mget":
		url = cacheServiceURL + "/mget?count=" + strconv.Itoa(3+rand.Intn(8))
	case "incr":
		url = cacheServiceURL + "/incr?key=counter:" + randomString(6)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("cache.operation", op),
	)
	return resp.StatusCode < 400, nil
}

func sendEmail(ctx context.Context) (bool, error) {
	correlationID := getCorrelationID(ctx)
	ctx, span := tracer.Start(ctx, "request-send-email")
	defer span.End()

	span.SetAttributes(attribute.String("correlation_id", correlationID))

	templates := []string{"welcome", "order_confirmation", "password_reset", "newsletter"}
	template := templates[rand.Intn(len(templates))]
	email := "user" + strconv.Itoa(rand.Intn(10000)) + "@example.com"

	req, err := http.NewRequestWithContext(ctx, "POST",
		emailServiceURL+"/send?to="+email+"&template="+template, nil)
	if err != nil {
		return false, err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	injectCorrelationID(ctx, req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("email.template", template),
	)
	return resp.StatusCode < 400, nil
}
