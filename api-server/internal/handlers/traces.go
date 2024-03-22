// Package handlers provides HTTP handlers for the API server.
package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/services"
)

// TraceQuery represents trace search parameters.
type TraceQuery struct {
	Service     string   `form:"service"`
	Operation   string   `form:"operation"`
	Tags        []string `form:"tags"`
	MinDuration string   `form:"minDuration"`
	MaxDuration string   `form:"maxDuration"`
	Start       int64    `form:"start"`
	End         int64    `form:"end"`
	Limit       int      `form:"limit,default=20"`
}

// Trace represents a trace in the API response.
type Trace struct {
	TraceID   string    `json:"traceId"`
	RootSpan  *Span     `json:"rootSpan,omitempty"`
	Spans     []Span    `json:"spans"`
	Services  []string  `json:"services"`
	Duration  int64     `json:"durationMs"`
	SpanCount int       `json:"spanCount"`
	HasError  bool      `json:"hasError"`
	StartTime time.Time `json:"startTime"`
}

// Span represents a span in the API response.
type Span struct {
	SpanID        string            `json:"spanId"`
	TraceID       string            `json:"traceId"`
	ParentSpanID  string            `json:"parentSpanId,omitempty"`
	OperationName string            `json:"operationName"`
	ServiceName   string            `json:"serviceName"`
	StartTime     time.Time         `json:"startTime"`
	Duration      int64             `json:"durationMs"`
	Status        string            `json:"status"`
	StatusCode    int               `json:"statusCode"`
	Kind          string            `json:"kind"`
	Attributes    map[string]any    `json:"attributes,omitempty"`
	Events        []SpanEvent       `json:"events,omitempty"`
	Links         []SpanLink        `json:"links,omitempty"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanLink represents a link to another span.
type SpanLink struct {
	TraceID    string         `json:"traceId"`
	SpanID     string         `json:"spanId"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ListTraces returns a list of traces matching the query.
func ListTraces(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query TraceQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Set default time range if not provided
		if query.End == 0 {
			query.End = time.Now().UnixMilli()
		}
		if query.Start == 0 {
			query.Start = query.End - (60 * 60 * 1000) // Default 1 hour
		}

		traces, err := svc.Traces.List(c.Request.Context(), services.TraceListParams{
			Service:     query.Service,
			Operation:   query.Operation,
			Tags:        query.Tags,
			MinDuration: parseDuration(query.MinDuration),
			MaxDuration: parseDuration(query.MaxDuration),
			Start:       time.UnixMilli(query.Start),
			End:         time.UnixMilli(query.End),
			Limit:       query.Limit,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"traces": traces,
			"total":  len(traces),
		})
	}
}

// GetTrace returns a single trace by ID.
func GetTrace(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Param("traceId")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
			return
		}

		trace, err := svc.Traces.Get(c.Request.Context(), traceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		c.JSON(http.StatusOK, trace)
	}
}

// GetTraceSpans returns all spans for a trace.
func GetTraceSpans(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Param("traceId")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
			return
		}

		spans, err := svc.Traces.GetSpans(c.Request.Context(), traceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"spans": spans,
			"total": len(spans),
		})
	}
}

// SearchTraces searches for traces using advanced criteria.
func SearchTraces(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query struct {
			Query string `form:"query"` // ObservQL query
			Start int64  `form:"start"`
			End   int64  `form:"end"`
			Limit int    `form:"limit,default=100"`
		}
		if err := c.ShouldBindQuery(&query); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if query.Query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
			return
		}

		results, err := svc.Traces.Search(c.Request.Context(), query.Query, query.Limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"results": results,
			"total":   len(results),
		})
	}
}

// parseDuration parses a duration string like "100ms", "1s", "5m".
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, _ := time.ParseDuration(s)
	return d
}

// HealthCheck returns the health status.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
	})
}

// ReadyCheck returns the readiness status.
func ReadyCheck(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.HealthCheck(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().UTC(),
		})
	}
}
