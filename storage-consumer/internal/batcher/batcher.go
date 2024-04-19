package batcher

import (
	"context"
	"sync"
	"time"

	"github.com/ollystack/storage-consumer/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	batchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_batcher_batch_size",
			Help:    "Size of batches when flushed",
			Buckets: []float64{10, 50, 100, 500, 1000, 5000, 10000},
		},
		[]string{"name"},
	)
	batchBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_batcher_batch_bytes",
			Help:    "Bytes of batches when flushed",
			Buckets: []float64{1024, 10240, 102400, 1048576, 10485760},
		},
		[]string{"name"},
	)
	flushCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_batcher_flushes_total",
			Help: "Total number of batch flushes",
		},
		[]string{"name", "reason"},
	)
	flushErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_batcher_flush_errors_total",
			Help: "Total number of flush errors",
		},
		[]string{"name"},
	)
	pendingItems = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ollystack_batcher_pending_items",
			Help: "Number of items pending in batch",
		},
		[]string{"name"},
	)
)

// FlushFunc is the function called to flush a batch
type FlushFunc func(batch [][]byte) error

// Batcher accumulates items and flushes them in batches
type Batcher struct {
	name      string
	config    config.BatcherConfig
	flushFunc FlushFunc
	logger    *zap.Logger

	mu          sync.Mutex
	items       [][]byte
	currentSize int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBatcher creates a new batcher
func NewBatcher(name string, cfg config.BatcherConfig, flushFunc FlushFunc, logger *zap.Logger) *Batcher {
	return &Batcher{
		name:      name,
		config:    cfg,
		flushFunc: flushFunc,
		logger:    logger,
		items:     make([][]byte, 0, cfg.MaxSize),
	}
}

// Start starts the batcher background flush loop
func (b *Batcher) Start(ctx context.Context) {
	b.ctx, b.cancel = context.WithCancel(ctx)

	b.wg.Add(1)
	go b.flushLoop()

	b.logger.Info("Batcher started",
		zap.String("name", b.name),
		zap.Int("max_size", b.config.MaxSize),
		zap.Int("max_bytes", b.config.MaxBytes),
		zap.Int("flush_interval_ms", b.config.FlushIntervalMs),
	)
}

// Stop stops the batcher and flushes remaining items
func (b *Batcher) Stop() {
	b.cancel()
	b.wg.Wait()

	// Final flush
	b.mu.Lock()
	if len(b.items) > 0 {
		batch := b.items
		b.items = make([][]byte, 0, b.config.MaxSize)
		b.currentSize = 0
		b.mu.Unlock()

		if err := b.flushFunc(batch); err != nil {
			b.logger.Error("Final flush failed",
				zap.String("name", b.name),
				zap.Error(err),
			)
		} else {
			flushCount.WithLabelValues(b.name, "shutdown").Inc()
		}
	} else {
		b.mu.Unlock()
	}

	b.logger.Info("Batcher stopped", zap.String("name", b.name))
}

// Add adds an item to the batch
func (b *Batcher) Add(item []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.items = append(b.items, item)
	b.currentSize += len(item)
	pendingItems.WithLabelValues(b.name).Set(float64(len(b.items)))

	// Check if we need to flush due to size limits
	if len(b.items) >= b.config.MaxSize {
		b.flushLocked("size")
	} else if b.currentSize >= b.config.MaxBytes {
		b.flushLocked("bytes")
	}
}

// flushLoop runs the periodic flush timer
func (b *Batcher) flushLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(time.Duration(b.config.FlushIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.mu.Lock()
			if len(b.items) > 0 {
				b.flushLocked("timer")
			}
			b.mu.Unlock()
		}
	}
}

// flushLocked flushes the current batch (must be called with lock held)
func (b *Batcher) flushLocked(reason string) {
	if len(b.items) == 0 {
		return
	}

	batch := b.items
	batchLen := len(batch)
	batchSizeBytes := b.currentSize

	// Reset batch
	b.items = make([][]byte, 0, b.config.MaxSize)
	b.currentSize = 0
	pendingItems.WithLabelValues(b.name).Set(0)

	// Flush asynchronously to not block Add()
	go func() {
		start := time.Now()

		if err := b.flushFunc(batch); err != nil {
			b.logger.Error("Flush failed",
				zap.String("name", b.name),
				zap.String("reason", reason),
				zap.Int("batch_size", batchLen),
				zap.Error(err),
			)
			flushErrors.WithLabelValues(b.name).Inc()

			// TODO: Handle retry logic or dead letter queue
			return
		}

		flushCount.WithLabelValues(b.name, reason).Inc()
		batchSize.WithLabelValues(b.name).Observe(float64(batchLen))
		batchBytes.WithLabelValues(b.name).Observe(float64(batchSizeBytes))

		b.logger.Debug("Flushed batch",
			zap.String("name", b.name),
			zap.String("reason", reason),
			zap.Int("items", batchLen),
			zap.Int("bytes", batchSizeBytes),
			zap.Duration("duration", time.Since(start)),
		)
	}()
}

// Stats returns current batcher statistics
func (b *Batcher) Stats() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	return map[string]interface{}{
		"name":          b.name,
		"pending_items": len(b.items),
		"pending_bytes": b.currentSize,
		"max_size":      b.config.MaxSize,
		"max_bytes":     b.config.MaxBytes,
	}
}
