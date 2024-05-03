package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ollystack/ingestion-gateway/internal/clickhouse"
	"go.uber.org/zap"
)

// Checker provides health check functionality
type Checker struct {
	writer    *clickhouse.Writer
	logger    *zap.Logger
	startTime time.Time
	ready     atomic.Bool
}

// Status represents the health check response
type Status struct {
	Status    string           `json:"status"`
	Timestamp string           `json:"timestamp"`
	Uptime    string           `json:"uptime"`
	Version   string           `json:"version"`
	Checks    map[string]Check `json:"checks"`
}

// Check represents an individual health check
type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// NewChecker creates a new health checker
func NewChecker(writer *clickhouse.Writer, logger *zap.Logger) *Checker {
	c := &Checker{
		writer:    writer,
		logger:    logger,
		startTime: time.Now(),
	}
	c.ready.Store(true)
	return c
}

// HealthHandler handles /health endpoint (liveness)
func (c *Checker) HealthHandler(w http.ResponseWriter, r *http.Request) {
	status := Status{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(c.startTime).String(),
		Version:   "2.0.0",
		Checks:    make(map[string]Check),
	}

	// Check ClickHouse connection
	if c.writer.IsHealthy() {
		status.Checks["clickhouse"] = Check{Status: "healthy"}
	} else {
		status.Checks["clickhouse"] = Check{
			Status:  "unhealthy",
			Message: "ClickHouse connection failed",
		}
		status.Status = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")

	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// ReadyHandler handles /ready endpoint (readiness)
func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !c.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
		return
	}

	if !c.writer.IsHealthy() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("clickhouse not ready"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// SetReady sets the ready state
func (c *Checker) SetReady(ready bool) {
	c.ready.Store(ready)
}

// IsReady returns the ready state
func (c *Checker) IsReady() bool {
	return c.ready.Load() && c.writer.IsHealthy()
}
