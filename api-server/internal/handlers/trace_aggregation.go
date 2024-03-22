package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mbeema/ollystack/api-server/internal/services"
)

// ================== ERROR FINGERPRINTING ==================

// ErrorFingerprint represents a unique error pattern
type ErrorFingerprint struct {
	Fingerprint   string    `json:"fingerprint"`
	ErrorType     string    `json:"errorType"`
	NormalizedMsg string    `json:"normalizedMessage"`
	SampleMessage string    `json:"sampleMessage"`
	Count         int       `json:"count"`
	FirstSeen     time.Time `json:"firstSeen"`
	LastSeen      time.Time `json:"lastSeen"`
	Services      []string  `json:"services"`
	Operations    []string  `json:"operations"`
	TraceIDs      []string  `json:"traceIds"`
	Trend         string    `json:"trend"` // "increasing", "stable", "decreasing"
}

// Patterns to normalize in error messages
var normalizationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), // UUID
	regexp.MustCompile(`[0-9a-fA-F]{24,}`),                                                             // Hex IDs
	regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),                                       // IP addresses
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),                          // Emails
	regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`),                                       // Timestamps
	regexp.MustCompile(`\b\d{10,13}\b`),                                                                 // Unix timestamps
	regexp.MustCompile(`\b\d+\b`),                                                                       // Plain numbers
}

// normalizeErrorMessage removes variable parts from error messages
func normalizeErrorMessage(msg string) string {
	normalized := msg
	for _, pattern := range normalizationPatterns {
		normalized = pattern.ReplaceAllString(normalized, "<*>")
	}
	normalized = regexp.MustCompile(`(<\*>\s*)+`).ReplaceAllString(normalized, "<*>")
	return strings.TrimSpace(normalized)
}

// generateFingerprint creates a unique fingerprint for an error
func generateFingerprint(errorType, normalizedMsg, service, operation string) string {
	data := fmt.Sprintf("%s|%s|%s|%s", errorType, normalizedMsg, service, operation)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// GetErrorFingerprints returns grouped error fingerprints
func GetErrorFingerprints(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
		endTime := time.Now()
		startTime := endTime.Add(-time.Duration(hours) * time.Hour)
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

		// Get error traces using the service List method
		traces, err := svc.Traces.List(ctx, services.TraceListParams{
			Start: startTime,
			End:   endTime,
			Limit: 1000,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Group errors by fingerprint
		fingerprintMap := make(map[string]*ErrorFingerprint)
		serviceSet := make(map[string]map[string]bool)
		operationSet := make(map[string]map[string]bool)

		for _, traceInterface := range traces {
			trace, ok := traceInterface.(map[string]interface{})
			if !ok {
				continue
			}

			// Check if this is an error trace
			status, _ := trace["status"].(string)
			if status != "error" {
				continue
			}

			serviceName, _ := trace["serviceName"].(string)
			operationName, _ := trace["operationName"].(string)
			traceID, _ := trace["traceId"].(string)
			timestamp, _ := trace["timestamp"].(time.Time)

			// For errors, use status as error type and operation as message
			errorType := "Error"
			errorMsg := fmt.Sprintf("%s: %s failed", serviceName, operationName)
			normalizedMsg := normalizeErrorMessage(errorMsg)
			fp := generateFingerprint(errorType, normalizedMsg, serviceName, operationName)

			if _, exists := fingerprintMap[fp]; !exists {
				fingerprintMap[fp] = &ErrorFingerprint{
					Fingerprint:   fp,
					ErrorType:     errorType,
					NormalizedMsg: normalizedMsg,
					SampleMessage: errorMsg,
					Count:         0,
					FirstSeen:     timestamp,
					LastSeen:      timestamp,
					Services:      []string{},
					Operations:    []string{},
					TraceIDs:      []string{},
					Trend:         "stable",
				}
				serviceSet[fp] = make(map[string]bool)
				operationSet[fp] = make(map[string]bool)
			}

			efp := fingerprintMap[fp]
			efp.Count++
			if timestamp.Before(efp.FirstSeen) {
				efp.FirstSeen = timestamp
			}
			if timestamp.After(efp.LastSeen) {
				efp.LastSeen = timestamp
			}
			serviceSet[fp][serviceName] = true
			operationSet[fp][operationName] = true
			if len(efp.TraceIDs) < 10 {
				efp.TraceIDs = append(efp.TraceIDs, traceID)
			}
		}

		// Convert to slice
		fingerprints := make([]ErrorFingerprint, 0, len(fingerprintMap))
		for fp, efp := range fingerprintMap {
			for svc := range serviceSet[fp] {
				efp.Services = append(efp.Services, svc)
			}
			for op := range operationSet[fp] {
				efp.Operations = append(efp.Operations, op)
			}

			// Trend calculation
			hoursSinceLast := time.Since(efp.LastSeen).Hours()
			if hoursSinceLast < 1 && efp.Count > 5 {
				efp.Trend = "increasing"
			} else if hoursSinceLast > float64(hours)/2 {
				efp.Trend = "decreasing"
			}

			fingerprints = append(fingerprints, *efp)
		}

		sort.Slice(fingerprints, func(i, j int) bool {
			return fingerprints[i].Count > fingerprints[j].Count
		})

		if len(fingerprints) > limit {
			fingerprints = fingerprints[:limit]
		}

		c.JSON(http.StatusOK, gin.H{
			"fingerprints": fingerprints,
			"total":        len(fingerprints),
			"timeRange": gin.H{
				"start": startTime,
				"end":   endTime,
				"hours": hours,
			},
		})
	}
}

// ================== TRACE AGGREGATION (RED METRICS) ==================

// ServiceOperationMetrics contains RED metrics for a service/operation pair
type ServiceOperationMetrics struct {
	Service      string  `json:"service"`
	Operation    string  `json:"operation"`
	RequestCount int     `json:"requestCount"`
	ErrorCount   int     `json:"errorCount"`
	ErrorRate    float64 `json:"errorRate"`
	P50LatencyMs float64 `json:"p50LatencyMs"`
	P90LatencyMs float64 `json:"p90LatencyMs"`
	P99LatencyMs float64 `json:"p99LatencyMs"`
	MinLatencyMs float64 `json:"minLatencyMs"`
	MaxLatencyMs float64 `json:"maxLatencyMs"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	Throughput   float64 `json:"throughputPerSec"`
}

// AggregatedMetrics contains overall aggregated metrics
type AggregatedMetrics struct {
	Services   []ServiceOperationMetrics `json:"services"`
	TotalReqs  int                       `json:"totalRequests"`
	TotalErrs  int                       `json:"totalErrors"`
	OverallP50 float64                   `json:"overallP50Ms"`
	OverallP99 float64                   `json:"overallP99Ms"`
	TimeRange  struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
		Hours int       `json:"hours"`
	} `json:"timeRange"`
}

// GetTraceAggregation returns RED metrics aggregated by service/operation
func GetTraceAggregation(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
		endTime := time.Now()
		startTime := endTime.Add(-time.Duration(hours) * time.Hour)
		groupBy := c.DefaultQuery("groupBy", "service,operation")

		traces, err := svc.Traces.List(ctx, services.TraceListParams{
			Start: startTime,
			End:   endTime,
			Limit: 10000,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Aggregate by service/operation
		type aggregator struct {
			durations []float64
			errors    int
			total     int
		}
		metricsMap := make(map[string]*aggregator)

		for _, traceInterface := range traces {
			trace, ok := traceInterface.(map[string]interface{})
			if !ok {
				continue
			}

			serviceName, _ := trace["serviceName"].(string)
			operationName, _ := trace["operationName"].(string)
			duration, _ := trace["duration"].(int64)
			status, _ := trace["status"].(string)

			var key string
			switch groupBy {
			case "service":
				key = serviceName
			case "operation":
				key = operationName
			default:
				key = fmt.Sprintf("%s|%s", serviceName, operationName)
			}

			if _, exists := metricsMap[key]; !exists {
				metricsMap[key] = &aggregator{
					durations: make([]float64, 0),
				}
			}

			agg := metricsMap[key]
			agg.total++
			agg.durations = append(agg.durations, float64(duration)/1e6) // Convert ns to ms
			if status == "error" {
				agg.errors++
			}
		}

		// Calculate percentiles and build response
		timeDurationHours := float64(hours)
		results := make([]ServiceOperationMetrics, 0, len(metricsMap))
		var totalReqs, totalErrs int
		var allDurations []float64

		for key, agg := range metricsMap {
			parts := strings.Split(key, "|")
			service := parts[0]
			operation := ""
			if len(parts) > 1 {
				operation = parts[1]
			}

			sort.Float64s(agg.durations)
			allDurations = append(allDurations, agg.durations...)

			metrics := ServiceOperationMetrics{
				Service:      service,
				Operation:    operation,
				RequestCount: agg.total,
				ErrorCount:   agg.errors,
				ErrorRate:    float64(agg.errors) / float64(agg.total) * 100,
				P50LatencyMs: percentile(agg.durations, 50),
				P90LatencyMs: percentile(agg.durations, 90),
				P99LatencyMs: percentile(agg.durations, 99),
				MinLatencyMs: agg.durations[0],
				MaxLatencyMs: agg.durations[len(agg.durations)-1],
				AvgLatencyMs: avg(agg.durations),
				Throughput:   float64(agg.total) / (timeDurationHours * 3600),
			}

			totalReqs += agg.total
			totalErrs += agg.errors
			results = append(results, metrics)
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].RequestCount > results[j].RequestCount
		})

		sort.Float64s(allDurations)

		response := AggregatedMetrics{
			Services:   results,
			TotalReqs:  totalReqs,
			TotalErrs:  totalErrs,
			OverallP50: percentile(allDurations, 50),
			OverallP99: percentile(allDurations, 99),
		}
		response.TimeRange.Start = startTime
		response.TimeRange.End = endTime
		response.TimeRange.Hours = hours

		c.JSON(http.StatusOK, response)
	}
}

// ================== DYNAMIC GROUPING ==================

// GroupedResult represents a dynamically grouped result
type GroupedResult struct {
	GroupKey    map[string]string `json:"groupKey"`
	Count       int               `json:"count"`
	ErrorCount  int               `json:"errorCount"`
	ErrorRate   float64           `json:"errorRate"`
	AvgDuration float64           `json:"avgDurationMs"`
	P50Duration float64           `json:"p50DurationMs"`
	P99Duration float64           `json:"p99DurationMs"`
	MinDuration float64           `json:"minDurationMs"`
	MaxDuration float64           `json:"maxDurationMs"`
	SampleIDs   []string          `json:"sampleTraceIds"`
}

// GetDynamicGrouping allows grouping traces by any attribute
func GetDynamicGrouping(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
		endTime := time.Now()
		startTime := endTime.Add(-time.Duration(hours) * time.Hour)
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

		groupByStr := c.DefaultQuery("groupBy", "service")
		groupByFields := strings.Split(groupByStr, ",")
		for i := range groupByFields {
			groupByFields[i] = strings.TrimSpace(groupByFields[i])
		}

		filterService := c.Query("service")

		// Build params
		params := services.TraceListParams{
			Service: filterService,
			Start:   startTime,
			End:     endTime,
			Limit:   10000,
		}

		traces, err := svc.Traces.List(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Group by specified fields
		type groupAggregator struct {
			key       map[string]string
			durations []float64
			errors    int
			traceIDs  []string
		}
		groups := make(map[string]*groupAggregator)

		for _, traceInterface := range traces {
			trace, ok := traceInterface.(map[string]interface{})
			if !ok {
				continue
			}

			serviceName, _ := trace["serviceName"].(string)
			operationName, _ := trace["operationName"].(string)
			status, _ := trace["status"].(string)
			traceID, _ := trace["traceId"].(string)
			duration, _ := trace["duration"].(int64)

			keyParts := make(map[string]string)
			keyStr := ""

			for _, field := range groupByFields {
				var value string
				switch field {
				case "service":
					value = serviceName
				case "operation":
					value = operationName
				case "status":
					value = status
				default:
					value = "<unknown>"
				}
				if value == "" {
					value = "<unknown>"
				}
				keyParts[field] = value
				keyStr += value + "|"
			}

			if _, exists := groups[keyStr]; !exists {
				groups[keyStr] = &groupAggregator{
					key:       keyParts,
					durations: make([]float64, 0),
					traceIDs:  make([]string, 0),
				}
			}

			g := groups[keyStr]
			g.durations = append(g.durations, float64(duration)/1e6)
			if status == "error" {
				g.errors++
			}
			if len(g.traceIDs) < 5 {
				g.traceIDs = append(g.traceIDs, traceID)
			}
		}

		// Build results
		results := make([]GroupedResult, 0, len(groups))
		for _, g := range groups {
			if len(g.durations) == 0 {
				continue
			}
			sort.Float64s(g.durations)
			count := len(g.durations)

			result := GroupedResult{
				GroupKey:    g.key,
				Count:       count,
				ErrorCount:  g.errors,
				ErrorRate:   float64(g.errors) / float64(count) * 100,
				AvgDuration: avg(g.durations),
				P50Duration: percentile(g.durations, 50),
				P99Duration: percentile(g.durations, 99),
				MinDuration: g.durations[0],
				MaxDuration: g.durations[count-1],
				SampleIDs:   g.traceIDs,
			}
			results = append(results, result)
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Count > results[j].Count
		})

		if len(results) > limit {
			results = results[:limit]
		}

		c.JSON(http.StatusOK, gin.H{
			"groups":  results,
			"total":   len(results),
			"groupBy": groupByFields,
			"timeRange": gin.H{
				"start": startTime,
				"end":   endTime,
				"hours": hours,
			},
		})
	}
}

// ================== HELPER FUNCTIONS ==================

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	index := (p / 100) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
