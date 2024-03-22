package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/services"
)

// TraceAnalysis represents AI-powered trace analysis results.
type TraceAnalysis struct {
	TraceID          string                 `json:"traceId"`
	Summary          string                 `json:"summary"`
	RootCause        *RootCauseAnalysis     `json:"rootCause,omitempty"`
	AnomalyScore     float64                `json:"anomalyScore"`
	Insights         []Insight              `json:"insights"`
	SimilarErrors    []SimilarError         `json:"similarErrors,omitempty"`
	Recommendations  []string               `json:"recommendations"`
	CriticalPath     []CriticalPathSpan     `json:"criticalPath"`
	ServiceBreakdown []ServiceMetrics       `json:"serviceBreakdown"`
	Timeline         []TraceTimelineEvent   `json:"timeline"`
}

// RootCauseAnalysis identifies the likely root cause of an error.
type RootCauseAnalysis struct {
	SpanID       string   `json:"spanId"`
	ServiceName  string   `json:"serviceName"`
	OperationName string  `json:"operationName"`
	ErrorType    string   `json:"errorType"`
	ErrorMessage string   `json:"errorMessage"`
	Confidence   float64  `json:"confidence"`
	Evidence     []string `json:"evidence"`
}

// Insight represents an AI-generated insight about the trace.
type Insight struct {
	Type        string `json:"type"`     // "error", "performance", "pattern", "anomaly"
	Severity    string `json:"severity"` // "critical", "warning", "info"
	Title       string `json:"title"`
	Description string `json:"description"`
	SpanID      string `json:"spanId,omitempty"`
}

// SimilarError represents a similar error found in historical traces.
type SimilarError struct {
	TraceID     string    `json:"traceId"`
	Timestamp   time.Time `json:"timestamp"`
	Similarity  float64   `json:"similarity"`
	ServiceName string    `json:"serviceName"`
	ErrorType   string    `json:"errorType"`
}

// CriticalPathSpan represents a span on the critical path.
type CriticalPathSpan struct {
	SpanID        string  `json:"spanId"`
	ServiceName   string  `json:"serviceName"`
	OperationName string  `json:"operationName"`
	Duration      int64   `json:"duration"`
	Percentage    float64 `json:"percentage"`
	IsBottleneck  bool    `json:"isBottleneck"`
}

// ServiceMetrics shows per-service metrics in the trace.
type ServiceMetrics struct {
	ServiceName  string  `json:"serviceName"`
	SpanCount    int     `json:"spanCount"`
	TotalTime    int64   `json:"totalTime"`
	ErrorCount   int     `json:"errorCount"`
	AvgDuration  float64 `json:"avgDuration"`
	P99Duration  int64   `json:"p99Duration"`
	Percentage   float64 `json:"percentage"`
}

// TraceTimelineEvent represents an event in the trace timeline.
type TraceTimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"type"` // "span_start", "span_end", "error", "event"
	SpanID      string    `json:"spanId"`
	ServiceName string    `json:"serviceName"`
	Description string    `json:"description"`
	IsError     bool      `json:"isError"`
}

// AnalyzeTrace performs AI-powered analysis on a trace.
func AnalyzeTrace(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Param("traceId")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
			return
		}

		// Get trace spans from service
		traceData, err := svc.Traces.Get(c.Request.Context(), traceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		// Convert to map for processing
		trace, ok := traceData.(map[string]interface{})
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid trace data"})
			return
		}

		spans, _ := trace["spans"].([]map[string]interface{})
		if spans == nil {
			// Try alternative format
			if rawSpans, ok := trace["spans"].([]interface{}); ok {
				spans = make([]map[string]interface{}, len(rawSpans))
				for i, s := range rawSpans {
					if spanMap, ok := s.(map[string]interface{}); ok {
						spans[i] = spanMap
					}
				}
			}
		}

		analysis := analyzeTraceSpans(traceID, spans)

		c.JSON(http.StatusOK, analysis)
	}
}

// GetErrorPatterns returns common error patterns across traces.
func GetErrorPatterns(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Get recent error traces
		params := services.TraceListParams{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
			Limit: 1000,
		}

		traces, err := svc.Traces.List(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		patterns := analyzeErrorPatterns(traces)

		c.JSON(http.StatusOK, gin.H{
			"patterns":     patterns,
			"totalTraces":  len(traces),
			"analyzedFrom": params.Start,
			"analyzedTo":   params.End,
		})
	}
}

// GetTraceStats returns statistical analysis of traces.
func GetTraceStats(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := c.Query("service")
		hours := 24 // Default to 24 hours

		ctx := c.Request.Context()
		end := time.Now()
		start := end.Add(-time.Duration(hours) * time.Hour)

		params := services.TraceListParams{
			Service: service,
			Start:   start,
			End:     end,
			Limit:   10000,
		}

		traces, err := svc.Traces.List(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		stats := calculateTraceStats(traces)
		stats["timeRange"] = map[string]interface{}{
			"start": start,
			"end":   end,
			"hours": hours,
		}
		if service != "" {
			stats["service"] = service
		}

		c.JSON(http.StatusOK, stats)
	}
}

// analyzeTraceSpans performs detailed analysis on trace spans.
func analyzeTraceSpans(traceID string, spans []map[string]interface{}) *TraceAnalysis {
	analysis := &TraceAnalysis{
		TraceID:         traceID,
		Insights:        []Insight{},
		Recommendations: []string{},
		CriticalPath:    []CriticalPathSpan{},
		Timeline:        []TraceTimelineEvent{},
	}

	if len(spans) == 0 {
		analysis.Summary = "No spans found in trace"
		return analysis
	}

	// Collect metrics
	var totalDuration int64
	var errorSpans []map[string]interface{}
	serviceMetrics := make(map[string]*ServiceMetrics)
	var durations []int64

	for _, span := range spans {
		duration := getInt64(span, "duration")
		serviceName := getString(span, "serviceName")
		status := getString(span, "status")
		opName := getString(span, "operationName")

		totalDuration += duration
		durations = append(durations, duration)

		// Track service metrics
		if _, ok := serviceMetrics[serviceName]; !ok {
			serviceMetrics[serviceName] = &ServiceMetrics{ServiceName: serviceName}
		}
		sm := serviceMetrics[serviceName]
		sm.SpanCount++
		sm.TotalTime += duration

		// Track errors
		isError := status == "ERROR" || status == "STATUS_CODE_ERROR" || strings.Contains(strings.ToLower(status), "error")
		if isError {
			errorSpans = append(errorSpans, span)
			sm.ErrorCount++

			// Add timeline event for error
			analysis.Timeline = append(analysis.Timeline, TraceTimelineEvent{
				EventType:   "error",
				SpanID:      getString(span, "spanId"),
				ServiceName: serviceName,
				Description: fmt.Sprintf("Error in %s: %s", opName, status),
				IsError:     true,
			})
		}

		// Build critical path (simplified - use longest spans)
		analysis.CriticalPath = append(analysis.CriticalPath, CriticalPathSpan{
			SpanID:        getString(span, "spanId"),
			ServiceName:   serviceName,
			OperationName: opName,
			Duration:      duration,
		})
	}

	// Sort critical path by duration
	sort.Slice(analysis.CriticalPath, func(i, j int) bool {
		return analysis.CriticalPath[i].Duration > analysis.CriticalPath[j].Duration
	})

	// Mark top spans as bottlenecks and calculate percentages
	for i := range analysis.CriticalPath {
		if totalDuration > 0 {
			analysis.CriticalPath[i].Percentage = float64(analysis.CriticalPath[i].Duration) / float64(totalDuration) * 100
		}
		if i < 3 {
			analysis.CriticalPath[i].IsBottleneck = true
		}
	}

	// Keep only top 10 critical path spans
	if len(analysis.CriticalPath) > 10 {
		analysis.CriticalPath = analysis.CriticalPath[:10]
	}

	// Calculate service breakdown
	for _, sm := range serviceMetrics {
		if sm.SpanCount > 0 {
			sm.AvgDuration = float64(sm.TotalTime) / float64(sm.SpanCount)
		}
		if totalDuration > 0 {
			sm.Percentage = float64(sm.TotalTime) / float64(totalDuration) * 100
		}
		analysis.ServiceBreakdown = append(analysis.ServiceBreakdown, *sm)
	}

	// Sort service breakdown by total time
	sort.Slice(analysis.ServiceBreakdown, func(i, j int) bool {
		return analysis.ServiceBreakdown[i].TotalTime > analysis.ServiceBreakdown[j].TotalTime
	})

	// Calculate anomaly score
	analysis.AnomalyScore = calculateAnomalyScore(durations, len(errorSpans))

	// Generate insights
	analysis.Insights = generateInsights(spans, errorSpans, serviceMetrics, totalDuration)

	// Root cause analysis
	if len(errorSpans) > 0 {
		analysis.RootCause = findRootCause(errorSpans, spans)
	}

	// Generate summary
	analysis.Summary = generateSummary(len(spans), len(errorSpans), totalDuration, analysis.AnomalyScore)

	// Generate recommendations
	analysis.Recommendations = generateRecommendations(analysis)

	return analysis
}

func findRootCause(errorSpans []map[string]interface{}, allSpans []map[string]interface{}) *RootCauseAnalysis {
	if len(errorSpans) == 0 {
		return nil
	}

	// Find the earliest error span (likely the root cause)
	var rootError map[string]interface{}
	for _, span := range errorSpans {
		if rootError == nil {
			rootError = span
		}
		// Could also check parentSpanId to find true root
	}

	if rootError == nil {
		return nil
	}

	attrs := getMap(rootError, "attributes")
	errorType := "Unknown Error"
	errorMsg := ""

	// Try to extract error details from attributes
	if et, ok := attrs["exception.type"]; ok {
		errorType = fmt.Sprintf("%v", et)
	} else if et, ok := attrs["error.type"]; ok {
		errorType = fmt.Sprintf("%v", et)
	} else if status := getString(rootError, "status"); status != "" {
		errorType = status
	}

	if em, ok := attrs["exception.message"]; ok {
		errorMsg = fmt.Sprintf("%v", em)
	} else if em, ok := attrs["error.message"]; ok {
		errorMsg = fmt.Sprintf("%v", em)
	}

	evidence := []string{}
	if errorMsg != "" {
		evidence = append(evidence, fmt.Sprintf("Error message: %s", errorMsg))
	}
	evidence = append(evidence, fmt.Sprintf("Operation: %s", getString(rootError, "operationName")))
	evidence = append(evidence, fmt.Sprintf("Duration: %dms", getInt64(rootError, "duration")/1000000))

	return &RootCauseAnalysis{
		SpanID:        getString(rootError, "spanId"),
		ServiceName:   getString(rootError, "serviceName"),
		OperationName: getString(rootError, "operationName"),
		ErrorType:     errorType,
		ErrorMessage:  errorMsg,
		Confidence:    0.85, // Could be calculated based on evidence
		Evidence:      evidence,
	}
}

func generateInsights(spans []map[string]interface{}, errorSpans []map[string]interface{}, serviceMetrics map[string]*ServiceMetrics, totalDuration int64) []Insight {
	insights := []Insight{}

	// Error insights
	if len(errorSpans) > 0 {
		insights = append(insights, Insight{
			Type:        "error",
			Severity:    "critical",
			Title:       fmt.Sprintf("%d Error(s) Detected", len(errorSpans)),
			Description: fmt.Sprintf("Found %d spans with errors in this trace. Review the error details for root cause.", len(errorSpans)),
		})
	}

	// Performance insights
	avgDuration := float64(totalDuration) / float64(len(spans))
	for svc, metrics := range serviceMetrics {
		if metrics.AvgDuration > avgDuration*2 {
			insights = append(insights, Insight{
				Type:        "performance",
				Severity:    "warning",
				Title:       fmt.Sprintf("High Latency in %s", svc),
				Description: fmt.Sprintf("%s has %.2fms average duration, which is %.1fx higher than trace average", svc, metrics.AvgDuration/1000000, metrics.AvgDuration/avgDuration),
			})
		}
	}

	// Pattern insights
	if len(serviceMetrics) > 5 {
		insights = append(insights, Insight{
			Type:        "pattern",
			Severity:    "info",
			Title:       "High Service Fan-out",
			Description: fmt.Sprintf("This trace touches %d different services, which may indicate complex dependencies", len(serviceMetrics)),
		})
	}

	// Anomaly insights
	if totalDuration > 1000000000 { // > 1 second
		insights = append(insights, Insight{
			Type:        "anomaly",
			Severity:    "warning",
			Title:       "Slow Trace",
			Description: fmt.Sprintf("Total trace duration is %.2f seconds, which may indicate performance issues", float64(totalDuration)/1000000000),
		})
	}

	return insights
}

func generateSummary(spanCount int, errorCount int, totalDuration int64, anomalyScore float64) string {
	status := "successful"
	if errorCount > 0 {
		status = "failed"
	}

	durationStr := fmt.Sprintf("%.2fms", float64(totalDuration)/1000000)
	if totalDuration > 1000000000 {
		durationStr = fmt.Sprintf("%.2fs", float64(totalDuration)/1000000000)
	}

	anomalyLevel := "normal"
	if anomalyScore > 0.7 {
		anomalyLevel = "highly anomalous"
	} else if anomalyScore > 0.4 {
		anomalyLevel = "moderately anomalous"
	}

	return fmt.Sprintf("This %s trace contains %d spans with total duration of %s. Anomaly score: %.0f%% (%s).",
		status, spanCount, durationStr, anomalyScore*100, anomalyLevel)
}

func generateRecommendations(analysis *TraceAnalysis) []string {
	recs := []string{}

	if analysis.RootCause != nil {
		recs = append(recs, fmt.Sprintf("Investigate the error in %s service's %s operation",
			analysis.RootCause.ServiceName, analysis.RootCause.OperationName))
	}

	if len(analysis.CriticalPath) > 0 && analysis.CriticalPath[0].Percentage > 50 {
		recs = append(recs, fmt.Sprintf("Optimize %s in %s - it accounts for %.0f%% of trace duration",
			analysis.CriticalPath[0].OperationName, analysis.CriticalPath[0].ServiceName, analysis.CriticalPath[0].Percentage))
	}

	if analysis.AnomalyScore > 0.5 {
		recs = append(recs, "This trace shows anomalous behavior - compare with baseline traces")
	}

	if len(analysis.ServiceBreakdown) > 5 {
		recs = append(recs, "Consider reducing service dependencies to simplify the trace flow")
	}

	if len(recs) == 0 {
		recs = append(recs, "Trace appears healthy - no immediate actions required")
	}

	return recs
}

func calculateAnomalyScore(durations []int64, errorCount int) float64 {
	if len(durations) == 0 {
		return 0
	}

	// Base score from errors
	score := float64(errorCount) / float64(len(durations)) * 0.5

	// Calculate variance in durations
	var sum, sumSq float64
	for _, d := range durations {
		sum += float64(d)
		sumSq += float64(d) * float64(d)
	}
	mean := sum / float64(len(durations))
	variance := sumSq/float64(len(durations)) - mean*mean
	stdDev := math.Sqrt(variance)

	// Coefficient of variation
	if mean > 0 {
		cv := stdDev / mean
		if cv > 2 {
			score += 0.3
		} else if cv > 1 {
			score += 0.2
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

func analyzeErrorPatterns(traces []interface{}) []map[string]interface{} {
	patterns := make(map[string]int)
	patternDetails := make(map[string]map[string]interface{})

	for _, t := range traces {
		if trace, ok := t.(map[string]interface{}); ok {
			status := getString(trace, "status")
			service := getString(trace, "serviceName")
			op := getString(trace, "operationName")

			if status == "error" {
				key := fmt.Sprintf("%s:%s", service, op)
				patterns[key]++
				if _, exists := patternDetails[key]; !exists {
					patternDetails[key] = map[string]interface{}{
						"serviceName":   service,
						"operationName": op,
						"count":         0,
						"firstSeen":     trace["timestamp"],
						"lastSeen":      trace["timestamp"],
					}
				}
				patternDetails[key]["count"] = patterns[key]
				patternDetails[key]["lastSeen"] = trace["timestamp"]
			}
		}
	}

	result := []map[string]interface{}{}
	for _, details := range patternDetails {
		result = append(result, details)
	}

	// Sort by count
	sort.Slice(result, func(i, j int) bool {
		ci, _ := result[i]["count"].(int)
		cj, _ := result[j]["count"].(int)
		return ci > cj
	})

	return result
}

func calculateTraceStats(traces []interface{}) map[string]interface{} {
	if len(traces) == 0 {
		return map[string]interface{}{
			"totalTraces": 0,
		}
	}

	var durations []int64
	var errorCount int
	services := make(map[string]int)

	for _, t := range traces {
		if trace, ok := t.(map[string]interface{}); ok {
			if d, ok := trace["duration"].(int64); ok {
				durations = append(durations, d)
			} else if d, ok := trace["duration"].(float64); ok {
				durations = append(durations, int64(d))
			}
			if status := getString(trace, "status"); status == "error" {
				errorCount++
			}
			if svc := getString(trace, "serviceName"); svc != "" {
				services[svc]++
			}
		}
	}

	// Sort durations for percentiles
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	var p50, p90, p99 int64
	if len(durations) > 0 {
		p50 = durations[len(durations)*50/100]
		p90 = durations[len(durations)*90/100]
		p99 = durations[len(durations)*99/100]
	}

	return map[string]interface{}{
		"totalTraces":   len(traces),
		"errorCount":    errorCount,
		"errorRate":     float64(errorCount) / float64(len(traces)) * 100,
		"serviceCount":  len(services),
		"p50LatencyMs":  float64(p50) / 1000000,
		"p90LatencyMs":  float64(p90) / 1000000,
		"p99LatencyMs":  float64(p99) / 1000000,
		"services":      services,
	}
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]interface{}); ok {
			return mm
		}
	}
	return map[string]interface{}{}
}
