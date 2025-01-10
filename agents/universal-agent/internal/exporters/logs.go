package exporters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// LogRecord represents a log record to be exported.
// This mirrors the collectors.LogRecord but is defined here to avoid circular imports.
type LogRecord interface {
	GetTimestamp() time.Time
	GetBody() string
	GetSeverity() string
	GetAttributes() map[string]string
}

// logRecordAdapter adapts the collectors.LogRecord to the LogRecord interface.
type logRecordAdapter struct {
	timestamp  time.Time
	body       string
	severity   string
	attributes map[string]string
}

func (l logRecordAdapter) GetTimestamp() time.Time       { return l.timestamp }
func (l logRecordAdapter) GetBody() string               { return l.body }
func (l logRecordAdapter) GetSeverity() string           { return l.severity }
func (l logRecordAdapter) GetAttributes() map[string]string { return l.attributes }

// LogExporter exports logs via OTLP.
type LogExporter struct {
	cfg      config.CollectorConfig
	resource *resource.Resource
	conn     *grpc.ClientConn

	// Batching
	batch     []interface{}
	batchMu   sync.Mutex
	batchSize int
	flushChan chan struct{}
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewLogExporter creates a new LogExporter.
func NewLogExporter(ctx context.Context, cfg config.CollectorConfig, res *resource.Resource) (*LogExporter, error) {
	var opts []grpc.DialOption

	if cfg.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, cfg.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	e := &LogExporter{
		cfg:       cfg,
		resource:  res,
		conn:      conn,
		batch:     make([]interface{}, 0, 100),
		batchSize: 100,
		flushChan: make(chan struct{}, 1),
		stopChan:  make(chan struct{}),
	}

	// Start background flusher
	e.wg.Add(1)
	go e.flushLoop(ctx)

	return e, nil
}

// Export exports a log record.
func (e *LogExporter) Export(ctx context.Context, record interface{}) error {
	e.batchMu.Lock()
	e.batch = append(e.batch, record)
	shouldFlush := len(e.batch) >= e.batchSize
	e.batchMu.Unlock()

	if shouldFlush {
		select {
		case e.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

// flushLoop periodically flushes the batch.
func (e *LogExporter) flushLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.flush(ctx)
			return
		case <-e.stopChan:
			e.flush(ctx)
			return
		case <-ticker.C:
			e.flush(ctx)
		case <-e.flushChan:
			e.flush(ctx)
		}
	}
}

// flush sends the current batch.
func (e *LogExporter) flush(ctx context.Context) {
	e.batchMu.Lock()
	if len(e.batch) == 0 {
		e.batchMu.Unlock()
		return
	}
	batch := e.batch
	e.batch = make([]interface{}, 0, e.batchSize)
	e.batchMu.Unlock()

	// Convert to OTLP format and send
	// This is a simplified implementation
	// In production, use the actual OTLP log exporter
	if err := e.sendBatch(ctx, batch); err != nil {
		// Log error but don't fail
		fmt.Printf("Failed to send log batch: %v\n", err)
	}
}

// sendBatch sends a batch of log records.
func (e *LogExporter) sendBatch(ctx context.Context, batch []interface{}) error {
	// This is a placeholder implementation
	// In production, this would use the OTLP logs protocol
	// The actual implementation would create OTLP log records and send them via gRPC

	// For now, we just acknowledge the batch
	// The full implementation would be:
	// 1. Convert batch to OTLP LogsData
	// 2. Call the OTLP LogsService.Export RPC
	// 3. Handle response and errors

	return nil
}

// Shutdown gracefully shuts down the exporter.
func (e *LogExporter) Shutdown(ctx context.Context) error {
	close(e.stopChan)
	e.wg.Wait()

	if e.conn != nil {
		if err := e.conn.Close(); err != nil {
			return fmt.Errorf("failed to close gRPC connection: %w", err)
		}
	}

	return nil
}
