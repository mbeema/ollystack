// Package handlers provides HTTP handlers for the API server.
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/services"
)

// GetCorrelation returns the full context for a correlation ID.
// GET /api/v1/correlate/:correlationId
func GetCorrelation(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.Param("correlationId")
		if correlationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "correlationId is required"})
			return
		}

		// Check if full details requested
		includeDetails := c.Query("details") == "true"

		var result interface{}
		var err error

		if includeDetails {
			result, err = svc.Correlation.GetWithDetails(c.Request.Context(), correlationID)
		} else {
			result, err = svc.Correlation.Get(c.Request.Context(), correlationID)
		}

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "correlation not found", "correlationId": correlationID})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// GetCorrelationTraces returns all traces for a correlation ID.
// GET /api/v1/correlate/:correlationId/traces
func GetCorrelationTraces(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.Param("correlationId")
		if correlationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "correlationId is required"})
			return
		}

		traces, err := svc.Correlation.GetTraces(c.Request.Context(), correlationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"correlationId": correlationID,
			"traces":        traces,
			"count":         len(traces),
		})
	}
}

// GetCorrelationLogs returns all logs for a correlation ID.
// GET /api/v1/correlate/:correlationId/logs
func GetCorrelationLogs(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.Param("correlationId")
		if correlationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "correlationId is required"})
			return
		}

		logs, err := svc.Correlation.GetLogs(c.Request.Context(), correlationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"correlationId": correlationID,
			"logs":          logs,
			"count":         len(logs),
		})
	}
}

// GetCorrelationTimeline returns the timeline for a correlation ID.
// GET /api/v1/correlate/:correlationId/timeline
func GetCorrelationTimeline(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.Param("correlationId")
		if correlationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "correlationId is required"})
			return
		}

		timeline, err := svc.Correlation.GetTimeline(c.Request.Context(), correlationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"correlationId": correlationID,
			"timeline":      timeline,
			"count":         len(timeline),
		})
	}
}

// SearchCorrelations searches for correlations matching criteria.
// POST /api/v1/correlate/search
func SearchCorrelations(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Service   string `json:"service"`
			HasErrors bool   `json:"hasErrors"`
			Start     string `json:"start"`
			End       string `json:"end"`
			Limit     int    `json:"limit"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			// If no body, try query params
			req.Service = c.Query("service")
			req.HasErrors = c.Query("hasErrors") == "true"
			req.Start = c.Query("start")
			req.End = c.Query("end")
			if limitStr := c.Query("limit"); limitStr != "" {
				req.Limit, _ = strconv.Atoi(limitStr)
			}
		}

		params := services.CorrelationSearchParams{
			Service:   req.Service,
			HasErrors: req.HasErrors,
			Limit:     req.Limit,
		}

		// Parse time range
		if req.Start != "" {
			if t, err := time.Parse(time.RFC3339, req.Start); err == nil {
				params.Start = t
			}
		}
		if req.End != "" {
			if t, err := time.Parse(time.RFC3339, req.End); err == nil {
				params.End = t
			}
		}

		correlations, err := svc.Correlation.Search(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"correlations": correlations,
			"count":        len(correlations),
			"params": gin.H{
				"service":   params.Service,
				"hasErrors": params.HasErrors,
				"start":     params.Start,
				"end":       params.End,
				"limit":     params.Limit,
			},
		})
	}
}

// ListRecentCorrelations returns recent correlations.
// GET /api/v1/correlate
func ListRecentCorrelations(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query params
		service := c.Query("service")
		hasErrors := c.Query("hasErrors") == "true"
		limit := 50
		if limitStr := c.Query("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		// Default time range: last hour
		end := time.Now()
		start := end.Add(-1 * time.Hour)

		if startStr := c.Query("start"); startStr != "" {
			if t, err := time.Parse(time.RFC3339, startStr); err == nil {
				start = t
			}
		}
		if endStr := c.Query("end"); endStr != "" {
			if t, err := time.Parse(time.RFC3339, endStr); err == nil {
				end = t
			}
		}

		params := services.CorrelationSearchParams{
			Service:   service,
			HasErrors: hasErrors,
			Start:     start,
			End:       end,
			Limit:     limit,
		}

		correlations, err := svc.Correlation.Search(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"correlations": correlations,
			"count":        len(correlations),
		})
	}
}
