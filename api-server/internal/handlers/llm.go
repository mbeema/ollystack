package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// LLMRequest represents an LLM API request
type LLMRequest struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	TraceID         string            `json:"trace_id,omitempty"`
	SpanID          string            `json:"span_id,omitempty"`
	ServiceName     string            `json:"service_name"`
	Environment     string            `json:"environment"`
	Provider        string            `json:"provider"`
	Model           string            `json:"model"`
	RequestType     string            `json:"request_type"`
	PromptTokens    int               `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens     int               `json:"total_tokens"`
	CostUSD         float64           `json:"cost_usd"`
	DurationMs      float64           `json:"duration_ms"`
	Status          string            `json:"status"`
	IsStreaming     bool              `json:"is_streaming"`
	HasToolCalls    bool              `json:"has_tool_calls"`
	HasRAGContext   bool              `json:"has_rag_context"`
	QualityScore    float64           `json:"quality_score,omitempty"`
	SafetyFlagged   bool              `json:"safety_flagged"`
	UserID          string            `json:"user_id,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// LLMEventsRequest represents a batch of LLM events
type LLMEventsRequest struct {
	Events   []map[string]interface{} `json:"events" binding:"required"`
	Metadata LLMEventsMetadata        `json:"metadata"`
}

// LLMEventsMetadata contains metadata about the events
type LLMEventsMetadata struct {
	ServiceName  string `json:"service_name"`
	Environment  string `json:"environment"`
	Version      string `json:"version"`
	SDKVersion   string `json:"sdk_version"`
}

// LLMStatsResponse represents LLM usage statistics
type LLMStatsResponse struct {
	TimeRange         string             `json:"time_range"`
	TotalRequests     int                `json:"total_requests"`
	TotalTokens       int                `json:"total_tokens"`
	TotalCostUSD      float64            `json:"total_cost_usd"`
	AvgLatencyMs      float64            `json:"avg_latency_ms"`
	P95LatencyMs      float64            `json:"p95_latency_ms"`
	ErrorRate         float64            `json:"error_rate"`
	ByModel           map[string]ModelStats `json:"by_model"`
	ByProvider        map[string]int     `json:"by_provider"`
	TopUsers          []UserStats        `json:"top_users"`
	QualityMetrics    QualityMetrics     `json:"quality_metrics"`
}

// ModelStats represents statistics for a specific model
type ModelStats struct {
	Requests       int     `json:"requests"`
	Tokens         int     `json:"tokens"`
	CostUSD        float64 `json:"cost_usd"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	ErrorCount     int     `json:"error_count"`
}

// UserStats represents statistics for a user
type UserStats struct {
	UserID    string  `json:"user_id"`
	Requests  int     `json:"requests"`
	Tokens    int     `json:"tokens"`
	CostUSD   float64 `json:"cost_usd"`
}

// QualityMetrics represents quality metrics
type QualityMetrics struct {
	AvgQualityScore    float64 `json:"avg_quality_score"`
	AvgRelevanceScore  float64 `json:"avg_relevance_score"`
	SafetyFlaggedCount int     `json:"safety_flagged_count"`
	PIIDetectedCount   int     `json:"pii_detected_count"`
}

// LLMCostResponse represents cost breakdown
type LLMCostResponse struct {
	TimeRange       string                `json:"time_range"`
	TotalCostUSD    float64               `json:"total_cost_usd"`
	ByModel         map[string]float64    `json:"by_model"`
	ByProvider      map[string]float64    `json:"by_provider"`
	ByService       map[string]float64    `json:"by_service"`
	DailyBreakdown  []DailyCost           `json:"daily_breakdown"`
	ProjectedMonthly float64              `json:"projected_monthly_cost_usd"`
}

// DailyCost represents daily cost
type DailyCost struct {
	Date     string  `json:"date"`
	CostUSD  float64 `json:"cost_usd"`
	Requests int     `json:"requests"`
	Tokens   int     `json:"tokens"`
}

// LLMHandler handles LLM observability API requests
type LLMHandler struct {
	// In production, this would have storage references
	requests []LLMRequest
}

// NewLLMHandler creates a new LLM handler
func NewLLMHandler() *LLMHandler {
	return &LLMHandler{
		requests: make([]LLMRequest, 0),
	}
}

// RegisterRoutes registers LLM observability API routes
func (h *LLMHandler) RegisterRoutes(r *gin.RouterGroup) {
	llm := r.Group("/llm")
	{
		// Event ingestion
		llm.POST("/events", h.IngestEvents)

		// Requests
		llm.GET("/requests", h.ListRequests)
		llm.GET("/requests/:id", h.GetRequest)
		llm.GET("/requests/:id/prompts", h.GetPrompts)
		llm.GET("/requests/:id/completions", h.GetCompletions)

		// Chains
		llm.GET("/chains", h.ListChains)
		llm.GET("/chains/:id", h.GetChain)

		// Embeddings
		llm.GET("/embeddings", h.ListEmbeddings)

		// RAG
		llm.GET("/rag/retrievals", h.ListRetrievals)

		// Evaluations
		llm.GET("/evaluations", h.ListEvaluations)
		llm.POST("/evaluations", h.CreateEvaluation)

		// User feedback
		llm.POST("/feedback", h.RecordFeedback)

		// Analytics
		llm.GET("/stats", h.GetStats)
		llm.GET("/stats/cost", h.GetCostStats)
		llm.GET("/stats/quality", h.GetQualityStats)
		llm.GET("/stats/models", h.GetModelStats)
		llm.GET("/stats/latency", h.GetLatencyStats)

		// Alerts
		llm.GET("/alerts/cost", h.GetCostAlerts)
		llm.GET("/alerts/quality", h.GetQualityAlerts)
	}
}

// IngestEvents handles LLM event ingestion
func (h *LLMHandler) IngestEvents(c *gin.Context) {
	var req LLMEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processed := 0
	for _, event := range req.Events {
		eventType, _ := event["type"].(string)
		switch eventType {
		case "llm_request":
			h.processLLMRequest(event, req.Metadata)
			processed++
		case "embedding":
			h.processEmbedding(event)
			processed++
		case "rag_retrieval":
			h.processRetrieval(event)
			processed++
		case "chain":
			h.processChain(event)
			processed++
		case "evaluation":
			h.processEvaluation(event)
			processed++
		case "user_feedback":
			h.processFeedback(event)
			processed++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"processed": processed,
		"total":     len(req.Events),
	})
}

func (h *LLMHandler) processLLMRequest(event map[string]interface{}, metadata LLMEventsMetadata) {
	request := LLMRequest{
		ID:           getStringOrDefault(event, "id", uuid.New().String()),
		Timestamp:    time.Now().UTC(),
		TraceID:      getStringOrDefault(event, "trace_id", ""),
		SpanID:       getStringOrDefault(event, "span_id", ""),
		ServiceName:  getStringOrDefault(event, "service_name", metadata.ServiceName),
		Environment:  getStringOrDefault(event, "environment", metadata.Environment),
		Provider:     getStringOrDefault(event, "provider", "openai"),
		Model:        getStringOrDefault(event, "model", ""),
		RequestType:  getStringOrDefault(event, "request_type", "chat"),
		Status:       getStringOrDefault(event, "status", "success"),
		IsStreaming:  getBoolOrDefault(event, "is_streaming", false),
		HasToolCalls: getBoolOrDefault(event, "has_tool_calls", false),
		HasRAGContext: getBoolOrDefault(event, "has_rag_context", false),
		SafetyFlagged: getBoolOrDefault(event, "safety_flagged", false),
		UserID:       getStringOrDefault(event, "user_id", ""),
		SessionID:    getStringOrDefault(event, "session_id", ""),
	}

	// Parse tokens
	if tokens, ok := event["tokens"].(map[string]interface{}); ok {
		request.PromptTokens = getIntOrDefault(tokens, "prompt", 0)
		request.CompletionTokens = getIntOrDefault(tokens, "completion", 0)
		request.TotalTokens = getIntOrDefault(tokens, "total", 0)
	}

	request.CostUSD = getFloatOrDefault(event, "cost_usd", 0)
	request.DurationMs = getFloatOrDefault(event, "duration_ms", 0)
	request.QualityScore = getFloatOrDefault(event, "quality_score", 0)

	if labels, ok := event["labels"].(map[string]interface{}); ok {
		request.Labels = make(map[string]string)
		for k, v := range labels {
			if s, ok := v.(string); ok {
				request.Labels[k] = s
			}
		}
	}

	h.requests = append(h.requests, request)
}

func (h *LLMHandler) processEmbedding(event map[string]interface{}) {
	// Store embedding request
}

func (h *LLMHandler) processRetrieval(event map[string]interface{}) {
	// Store RAG retrieval
}

func (h *LLMHandler) processChain(event map[string]interface{}) {
	// Store chain execution
}

func (h *LLMHandler) processEvaluation(event map[string]interface{}) {
	// Store evaluation
}

func (h *LLMHandler) processFeedback(event map[string]interface{}) {
	// Store user feedback
}

// ListRequests returns a list of LLM requests
func (h *LLMHandler) ListRequests(c *gin.Context) {
	model := c.Query("model")
	provider := c.Query("provider")
	status := c.Query("status")

	var filtered []LLMRequest
	for _, req := range h.requests {
		if model != "" && req.Model != model {
			continue
		}
		if provider != "" && req.Provider != provider {
			continue
		}
		if status != "" && req.Status != status {
			continue
		}
		filtered = append(filtered, req)
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": filtered,
		"total":    len(filtered),
	})
}

// GetRequest returns a specific LLM request
func (h *LLMHandler) GetRequest(c *gin.Context) {
	id := c.Param("id")

	for _, req := range h.requests {
		if req.ID == id {
			c.JSON(http.StatusOK, req)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Request not found"})
}

// GetPrompts returns prompts for a request
func (h *LLMHandler) GetPrompts(c *gin.Context) {
	id := c.Param("id")
	// In production, would fetch from storage
	c.JSON(http.StatusOK, gin.H{
		"request_id": id,
		"prompts":    []interface{}{},
	})
}

// GetCompletions returns completions for a request
func (h *LLMHandler) GetCompletions(c *gin.Context) {
	id := c.Param("id")
	// In production, would fetch from storage
	c.JSON(http.StatusOK, gin.H{
		"request_id":   id,
		"completions":  []interface{}{},
	})
}

// ListChains returns a list of chain executions
func (h *LLMHandler) ListChains(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"chains": []interface{}{},
		"total":  0,
	})
}

// GetChain returns a specific chain
func (h *LLMHandler) GetChain(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "Chain not found"})
}

// ListEmbeddings returns a list of embedding requests
func (h *LLMHandler) ListEmbeddings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"embeddings": []interface{}{},
		"total":      0,
	})
}

// ListRetrievals returns a list of RAG retrievals
func (h *LLMHandler) ListRetrievals(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"retrievals": []interface{}{},
		"total":      0,
	})
}

// ListEvaluations returns a list of evaluations
func (h *LLMHandler) ListEvaluations(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"evaluations": []interface{}{},
		"total":       0,
	})
}

// CreateEvaluation creates a new evaluation
func (h *LLMHandler) CreateEvaluation(c *gin.Context) {
	var evaluation map[string]interface{}
	if err := c.ShouldBindJSON(&evaluation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	evaluation["id"] = uuid.New().String()
	evaluation["timestamp"] = time.Now().UTC()

	c.JSON(http.StatusOK, gin.H{
		"status":     "created",
		"evaluation": evaluation,
	})
}

// RecordFeedback records user feedback
func (h *LLMHandler) RecordFeedback(c *gin.Context) {
	var feedback map[string]interface{}
	if err := c.ShouldBindJSON(&feedback); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "recorded",
	})
}

// GetStats returns LLM usage statistics
func (h *LLMHandler) GetStats(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "24h")

	// Calculate stats
	var totalTokens int
	var totalCost float64
	var totalLatency float64
	var errorCount int
	modelStats := make(map[string]ModelStats)
	providerCounts := make(map[string]int)

	for _, req := range h.requests {
		totalTokens += req.TotalTokens
		totalCost += req.CostUSD
		totalLatency += req.DurationMs

		if req.Status == "error" {
			errorCount++
		}

		// Model stats
		ms := modelStats[req.Model]
		ms.Requests++
		ms.Tokens += req.TotalTokens
		ms.CostUSD += req.CostUSD
		ms.AvgLatencyMs = (ms.AvgLatencyMs*float64(ms.Requests-1) + req.DurationMs) / float64(ms.Requests)
		if req.Status == "error" {
			ms.ErrorCount++
		}
		modelStats[req.Model] = ms

		// Provider counts
		providerCounts[req.Provider]++
	}

	avgLatency := 0.0
	errorRate := 0.0
	if len(h.requests) > 0 {
		avgLatency = totalLatency / float64(len(h.requests))
		errorRate = float64(errorCount) / float64(len(h.requests))
	}

	c.JSON(http.StatusOK, LLMStatsResponse{
		TimeRange:      timeRange,
		TotalRequests:  len(h.requests),
		TotalTokens:    totalTokens,
		TotalCostUSD:   totalCost,
		AvgLatencyMs:   avgLatency,
		P95LatencyMs:   avgLatency * 1.5, // Placeholder
		ErrorRate:      errorRate,
		ByModel:        modelStats,
		ByProvider:     providerCounts,
		TopUsers:       []UserStats{},
		QualityMetrics: QualityMetrics{},
	})
}

// GetCostStats returns cost statistics
func (h *LLMHandler) GetCostStats(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	byModel := make(map[string]float64)
	byProvider := make(map[string]float64)
	byService := make(map[string]float64)
	var totalCost float64

	for _, req := range h.requests {
		totalCost += req.CostUSD
		byModel[req.Model] += req.CostUSD
		byProvider[req.Provider] += req.CostUSD
		byService[req.ServiceName] += req.CostUSD
	}

	c.JSON(http.StatusOK, LLMCostResponse{
		TimeRange:        timeRange,
		TotalCostUSD:     totalCost,
		ByModel:          byModel,
		ByProvider:       byProvider,
		ByService:        byService,
		DailyBreakdown:   []DailyCost{},
		ProjectedMonthly: totalCost * 30, // Simple projection
	})
}

// GetQualityStats returns quality statistics
func (h *LLMHandler) GetQualityStats(c *gin.Context) {
	var totalQuality float64
	var qualityCount int
	var safetyFlagged int

	for _, req := range h.requests {
		if req.QualityScore > 0 {
			totalQuality += req.QualityScore
			qualityCount++
		}
		if req.SafetyFlagged {
			safetyFlagged++
		}
	}

	avgQuality := 0.0
	if qualityCount > 0 {
		avgQuality = totalQuality / float64(qualityCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"avg_quality_score":     avgQuality,
		"evaluated_count":       qualityCount,
		"safety_flagged_count":  safetyFlagged,
		"quality_distribution":  map[string]int{},
	})
}

// GetModelStats returns per-model statistics
func (h *LLMHandler) GetModelStats(c *gin.Context) {
	modelStats := make(map[string]ModelStats)

	for _, req := range h.requests {
		ms := modelStats[req.Model]
		ms.Requests++
		ms.Tokens += req.TotalTokens
		ms.CostUSD += req.CostUSD
		ms.AvgLatencyMs = (ms.AvgLatencyMs*float64(ms.Requests-1) + req.DurationMs) / float64(ms.Requests)
		if req.Status == "error" {
			ms.ErrorCount++
		}
		modelStats[req.Model] = ms
	}

	c.JSON(http.StatusOK, gin.H{
		"models": modelStats,
	})
}

// GetLatencyStats returns latency statistics
func (h *LLMHandler) GetLatencyStats(c *gin.Context) {
	var latencies []float64
	for _, req := range h.requests {
		latencies = append(latencies, req.DurationMs)
	}

	// Calculate percentiles (simplified)
	avg := 0.0
	if len(latencies) > 0 {
		sum := 0.0
		for _, l := range latencies {
			sum += l
		}
		avg = sum / float64(len(latencies))
	}

	c.JSON(http.StatusOK, gin.H{
		"avg_ms": avg,
		"p50_ms": avg * 0.8,  // Placeholder
		"p90_ms": avg * 1.3,  // Placeholder
		"p95_ms": avg * 1.5,  // Placeholder
		"p99_ms": avg * 2.0,  // Placeholder
	})
}

// GetCostAlerts returns cost alerts
func (h *LLMHandler) GetCostAlerts(c *gin.Context) {
	// In production, would check against configured thresholds
	c.JSON(http.StatusOK, gin.H{
		"alerts": []interface{}{},
	})
}

// GetQualityAlerts returns quality alerts
func (h *LLMHandler) GetQualityAlerts(c *gin.Context) {
	// In production, would check against configured thresholds
	c.JSON(http.StatusOK, gin.H{
		"alerts": []interface{}{},
	})
}

// Helper functions

func getStringOrDefault(m map[string]interface{}, key, defaultVal string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return defaultVal
}

func getIntOrDefault(m map[string]interface{}, key string, defaultVal int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	return defaultVal
}

func getFloatOrDefault(m map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return defaultVal
}

func getBoolOrDefault(m map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return defaultVal
}
