// Package handlers provides HTTP handlers for the API server.
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/storage"
)

// ExemplarHandlers provides handlers for metric exemplars
// Exemplars link metrics to traces, enabling drill-down from metric spikes to traces
type ExemplarHandlers struct {
	clickhouse *storage.ClickHouseClient
}

// NewExemplarHandlers creates a new ExemplarHandlers instance
func NewExemplarHandlers(ch *storage.ClickHouseClient) *ExemplarHandlers {
	return &ExemplarHandlers{clickhouse: ch}
}

// GetMetricsWithExemplars retrieves metrics with their trace-linked exemplars
// GET /api/v1/metrics/exemplars
// Query params: metricName, serviceName, start, end, limit
func (h *ExemplarHandlers) GetMetricsWithExemplars() gin.HandlerFunc {
	return func(c *gin.Context) {
		metricName := c.Query("metricName")
		serviceName := c.Query("serviceName")

		start, end := parseTimeRange(c)
		limit := parseLimit(c, 100)

		metrics, err := h.clickhouse.GetMetricsWithExemplars(c.Request.Context(), metricName, serviceName, start, end, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"metrics": metrics,
			"count":   len(metrics),
			"query": gin.H{
				"metricName":  metricName,
				"serviceName": serviceName,
				"start":       start,
				"end":         end,
				"limit":       limit,
			},
		})
	}
}

// GetExemplarsForMetric retrieves all exemplars for a specific metric
// GET /api/v1/metrics/:metricName/exemplars
func (h *ExemplarHandlers) GetExemplarsForMetric() gin.HandlerFunc {
	return func(c *gin.Context) {
		metricName := c.Param("metricName")
		if metricName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "metricName is required"})
			return
		}

		start, end := parseTimeRange(c)
		limit := parseLimit(c, 50)

		exemplars, err := h.clickhouse.GetExemplarsForMetric(c.Request.Context(), metricName, start, end, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"exemplars":  exemplars,
			"count":      len(exemplars),
			"metricName": metricName,
		})
	}
}

// GetExemplarsInRange retrieves exemplars within a value range
// Useful for finding traces associated with latency spikes
// GET /api/v1/metrics/:metricName/exemplars/range
// Query params: minValue, maxValue, start, end, limit
func (h *ExemplarHandlers) GetExemplarsInRange() gin.HandlerFunc {
	return func(c *gin.Context) {
		metricName := c.Param("metricName")
		if metricName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "metricName is required"})
			return
		}

		minValue, _ := strconv.ParseFloat(c.DefaultQuery("minValue", "0"), 64)
		maxValue, _ := strconv.ParseFloat(c.DefaultQuery("maxValue", "999999"), 64)

		start, end := parseTimeRange(c)
		limit := parseLimit(c, 50)

		exemplars, err := h.clickhouse.GetExemplarsInRange(c.Request.Context(), metricName, minValue, maxValue, start, end, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"exemplars":  exemplars,
			"count":      len(exemplars),
			"metricName": metricName,
			"range": gin.H{
				"min": minValue,
				"max": maxValue,
			},
		})
	}
}

// GetHighLatencyExemplars retrieves exemplars for high latency requests
// Perfect for investigating P99 latency spikes
// GET /api/v1/exemplars/high-latency
// Query params: serviceName, thresholdMs, start, end, limit
func (h *ExemplarHandlers) GetHighLatencyExemplars() gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceName := c.Query("serviceName")
		thresholdMs, _ := strconv.ParseFloat(c.DefaultQuery("thresholdMs", "1000"), 64)

		start, end := parseTimeRange(c)
		limit := parseLimit(c, 50)

		exemplars, err := h.clickhouse.GetHighLatencyExemplars(c.Request.Context(), serviceName, thresholdMs, start, end, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"exemplars":   exemplars,
			"count":       len(exemplars),
			"thresholdMs": thresholdMs,
			"serviceName": serviceName,
			"message":     "Click on a traceId to see the full trace for this slow request",
		})
	}
}

// GetTraceFromExemplar retrieves the full trace for an exemplar's trace ID
// GET /api/v1/exemplars/trace/:traceId
func (h *ExemplarHandlers) GetTraceFromExemplar() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Param("traceId")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
			return
		}

		spans, err := h.clickhouse.GetTraceFromExemplar(c.Request.Context(), traceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if len(spans) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"traceId":   traceID,
			"spans":     spans,
			"spanCount": len(spans),
		})
	}
}

// parseTimeRange extracts start/end times from query params
func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
	now := time.Now()

	// Default to last hour
	start := now.Add(-1 * time.Hour)
	end := now

	if startStr := c.Query("start"); startStr != "" {
		if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			start = time.Unix(ts, 0)
		} else if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}

	if endStr := c.Query("end"); endStr != "" {
		if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = time.Unix(ts, 0)
		} else if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}

	return start, end
}

// parseLimit extracts limit from query params with a default
func parseLimit(c *gin.Context, defaultLimit int) int {
	limit := defaultLimit
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000 // Cap at 1000
	}
	return limit
}
