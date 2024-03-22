package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Investigation represents a proactive AI investigation
type Investigation struct {
	ID                string       `json:"id"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	TriggerType       string       `json:"trigger_type"`
	TriggerID         string       `json:"trigger_id,omitempty"`
	TriggerTimestamp  *time.Time   `json:"trigger_timestamp,omitempty"`
	ServiceName       string       `json:"service_name,omitempty"`
	OperationName     string       `json:"operation_name,omitempty"`
	Environment       string       `json:"environment"`
	InvestigationStart *time.Time  `json:"investigation_start,omitempty"`
	InvestigationEnd   *time.Time  `json:"investigation_end,omitempty"`
	Status            string       `json:"status"`
	Phase             string       `json:"phase"`
	ProgressPercent   int          `json:"progress_percent"`
	Title             string       `json:"title"`
	Summary           string       `json:"summary"`
	Severity          string       `json:"severity"`
	AffectedServices  []string     `json:"affected_services"`
	AffectedEndpoints []string     `json:"affected_endpoints"`
	ErrorCount        int          `json:"error_count"`
	AffectedTraceCount int         `json:"affected_trace_count"`
	Hypotheses        []Hypothesis `json:"hypotheses"`
	Timeline          []TimelineEvent `json:"timeline"`
	EvidenceCount     int          `json:"evidence_count"`
	OverallConfidence float64      `json:"overall_confidence"`
	Labels            map[string]string `json:"labels"`
	CreatedBy         string       `json:"created_by"`
	ResolvedAt        *time.Time   `json:"resolved_at,omitempty"`
	ResolvedBy        string       `json:"resolved_by,omitempty"`
	Resolution        string       `json:"resolution,omitempty"`
}

// Hypothesis represents a root cause hypothesis
type Hypothesis struct {
	ID                string    `json:"id"`
	InvestigationID   string    `json:"investigation_id"`
	Rank              int       `json:"rank"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Category          string    `json:"category"`
	Confidence        float64   `json:"confidence"`
	Reasoning         string    `json:"reasoning"`
	RelatedServices   []string  `json:"related_services"`
	RelatedTraceIDs   []string  `json:"related_trace_ids"`
	RelatedMetrics    []string  `json:"related_metrics"`
	RelatedDeployments []string `json:"related_deployments"`
	SuggestedActions  []string  `json:"suggested_actions"`
	RunbookURL        string    `json:"runbook_url,omitempty"`
	Verified          bool      `json:"verified"`
	VerifiedBy        string    `json:"verified_by,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	VerificationNotes string    `json:"verification_notes,omitempty"`
}

// TimelineEvent represents an event in the investigation timeline
type TimelineEvent struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	EventType     string    `json:"event_type"`
	EventSource   string    `json:"event_source"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Severity      string    `json:"severity"`
	ImpactScore   float64   `json:"impact_score"`
	ServiceName   string    `json:"service_name,omitempty"`
	TraceID       string    `json:"trace_id,omitempty"`
	MetricName    string    `json:"metric_name,omitempty"`
	MetricValue   *float64  `json:"metric_value,omitempty"`
	DeepLinkURL   string    `json:"deep_link_url,omitempty"`
}

// InvestigationTriggerConfig represents trigger configuration
type InvestigationTriggerConfig struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	TriggerType         string   `json:"trigger_type"`
	Enabled             bool     `json:"enabled"`
	ServiceFilter       string   `json:"service_filter,omitempty"`
	Threshold           float64  `json:"threshold"`
	Operator            string   `json:"operator"`
	Duration            string   `json:"duration"`
	AutoStart           bool     `json:"auto_start"`
	InvestigationWindow string   `json:"investigation_window"`
	Priority            string   `json:"priority"`
	NotifyOnStart       bool     `json:"notify_on_start"`
	NotifyChannels      []string `json:"notify_channels"`
}

// Request/Response types

type StartInvestigationRequest struct {
	TriggerType         string `json:"trigger_type" binding:"required"`
	TriggerID           string `json:"trigger_id"`
	ServiceName         string `json:"service_name"`
	OperationName       string `json:"operation_name"`
	InvestigationWindow string `json:"investigation_window"`
	Description         string `json:"description"`
}

type InvestigationListResponse struct {
	Investigations []Investigation `json:"investigations"`
	Total          int            `json:"total"`
	Page           int            `json:"page"`
	PageSize       int            `json:"page_size"`
}

type VerifyHypothesisRequest struct {
	Verified bool   `json:"verified" binding:"required"`
	Notes    string `json:"notes"`
}

type ResolveInvestigationRequest struct {
	Resolution string `json:"resolution" binding:"required"`
	ResolvedBy string `json:"resolved_by"`
}

type InvestigationStatsResponse struct {
	TimeRange                string            `json:"time_range"`
	TotalInvestigations      int               `json:"total_investigations"`
	ByStatus                 map[string]int    `json:"by_status"`
	ByTriggerType            map[string]int    `json:"by_trigger_type"`
	BySeverity               map[string]int    `json:"by_severity"`
	AverageDurationSeconds   float64           `json:"average_duration_seconds"`
	TopAffectedServices      map[string]int    `json:"top_affected_services"`
	TotalHypothesesGenerated int               `json:"total_hypotheses_generated"`
	VerifiedHypotheses       int               `json:"verified_hypotheses"`
}

// InvestigationHandler handles investigation API requests
type InvestigationHandler struct {
	// In production, this would have storage, cache, and AI engine references
	investigations map[string]*Investigation
	triggers       map[string]*InvestigationTriggerConfig
}

// NewInvestigationHandler creates a new handler
func NewInvestigationHandler() *InvestigationHandler {
	h := &InvestigationHandler{
		investigations: make(map[string]*Investigation),
		triggers:       make(map[string]*InvestigationTriggerConfig),
	}
	h.initDefaultTriggers()
	return h
}

func (h *InvestigationHandler) initDefaultTriggers() {
	defaults := []InvestigationTriggerConfig{
		{
			ID:                  "default-anomaly-high",
			Name:                "High Anomaly Score",
			Description:         "Triggers when anomaly score exceeds 0.9",
			TriggerType:         "anomaly",
			Enabled:             true,
			Threshold:           0.9,
			Operator:            "gt",
			Duration:            "2m",
			AutoStart:           true,
			InvestigationWindow: "30m",
			Priority:            "high",
			NotifyOnStart:       true,
		},
		{
			ID:                  "default-error-spike",
			Name:                "Error Rate Spike",
			Description:         "Triggers when error rate exceeds 10%",
			TriggerType:         "error_spike",
			Enabled:             true,
			Threshold:           0.1,
			Operator:            "gt",
			Duration:            "5m",
			AutoStart:           true,
			InvestigationWindow: "1h",
			Priority:            "high",
			NotifyOnStart:       true,
		},
		{
			ID:                  "default-latency-spike",
			Name:                "Latency Spike",
			Description:         "Triggers when p99 latency exceeds 5 seconds",
			TriggerType:         "latency_spike",
			Enabled:             true,
			Threshold:           5000,
			Operator:            "gt",
			Duration:            "5m",
			AutoStart:           true,
			InvestigationWindow: "1h",
			Priority:            "normal",
			NotifyOnStart:       true,
		},
		{
			ID:                  "default-slo-breach",
			Name:                "SLO Breach",
			Description:         "Triggers when SLO error budget is exhausted",
			TriggerType:         "slo_breach",
			Enabled:             true,
			Threshold:           0,
			Operator:            "lte",
			Duration:            "1m",
			AutoStart:           true,
			InvestigationWindow: "2h",
			Priority:            "high",
			NotifyOnStart:       true,
		},
	}

	for _, t := range defaults {
		tc := t
		h.triggers[tc.ID] = &tc
	}
}

// RegisterRoutes registers investigation API routes
func (h *InvestigationHandler) RegisterRoutes(r *gin.RouterGroup) {
	inv := r.Group("/investigations")
	{
		inv.POST("/start", h.StartInvestigation)
		inv.GET("", h.ListInvestigations)
		inv.GET("/:id", h.GetInvestigation)
		inv.POST("/:id/cancel", h.CancelInvestigation)
		inv.POST("/:id/resolve", h.ResolveInvestigation)

		// Hypotheses
		inv.GET("/:id/hypotheses", h.GetHypotheses)
		inv.POST("/:id/hypotheses/:hypothesis_id/verify", h.VerifyHypothesis)

		// Timeline
		inv.GET("/:id/timeline", h.GetTimeline)

		// Evidence
		inv.GET("/:id/evidence", h.GetEvidence)

		// Stats
		inv.GET("/stats/summary", h.GetStats)
		inv.GET("/stats/trigger-performance", h.GetTriggerStats)
	}

	// Trigger management
	triggers := r.Group("/investigations/triggers")
	{
		triggers.GET("/list", h.ListTriggers)
		triggers.GET("/:id", h.GetTrigger)
		triggers.POST("/create", h.CreateTrigger)
		triggers.PUT("/:id", h.UpdateTrigger)
		triggers.DELETE("/:id", h.DeleteTrigger)
		triggers.POST("/:id/enable", h.EnableTrigger)
		triggers.POST("/:id/disable", h.DisableTrigger)
		triggers.POST("/monitoring/start", h.StartMonitoring)
		triggers.POST("/monitoring/stop", h.StopMonitoring)
	}
}

// StartInvestigation starts a new investigation
func (h *InvestigationHandler) StartInvestigation(c *gin.Context) {
	var req StartInvestigationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	id := uuid.New().String()

	investigation := &Investigation{
		ID:                id,
		CreatedAt:         now,
		UpdatedAt:         now,
		TriggerType:       req.TriggerType,
		TriggerID:         req.TriggerID,
		ServiceName:       req.ServiceName,
		OperationName:     req.OperationName,
		Environment:       "production",
		Status:            "running",
		Phase:             "initializing",
		ProgressPercent:   0,
		Severity:          "medium",
		AffectedServices:  []string{},
		AffectedEndpoints: []string{},
		Hypotheses:        []Hypothesis{},
		Timeline:          []TimelineEvent{},
		Labels:            map[string]string{},
		CreatedBy:         "user",
	}

	h.investigations[id] = investigation

	// In production, this would kick off the async investigation process
	// For now, we return immediately

	c.JSON(http.StatusOK, investigation)
}

// GetInvestigation gets a specific investigation
func (h *InvestigationHandler) GetInvestigation(c *gin.Context) {
	id := c.Param("id")

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	c.JSON(http.StatusOK, investigation)
}

// ListInvestigations lists investigations with filters
func (h *InvestigationHandler) ListInvestigations(c *gin.Context) {
	status := c.Query("status")
	service := c.Query("service")
	triggerType := c.Query("trigger_type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var filtered []*Investigation
	for _, inv := range h.investigations {
		if status != "" && inv.Status != status {
			continue
		}
		if service != "" && inv.ServiceName != service {
			continue
		}
		if triggerType != "" && inv.TriggerType != triggerType {
			continue
		}
		filtered = append(filtered, inv)
	}

	total := len(filtered)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	result := make([]Investigation, 0)
	for _, inv := range filtered[start:end] {
		result = append(result, *inv)
	}

	c.JSON(http.StatusOK, InvestigationListResponse{
		Investigations: result,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	})
}

// CancelInvestigation cancels a running investigation
func (h *InvestigationHandler) CancelInvestigation(c *gin.Context) {
	id := c.Param("id")

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	if investigation.Status != "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Investigation is not running"})
		return
	}

	investigation.Status = "cancelled"
	investigation.UpdatedAt = time.Now().UTC()

	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "investigation_id": id})
}

// ResolveInvestigation marks an investigation as resolved
func (h *InvestigationHandler) ResolveInvestigation(c *gin.Context) {
	id := c.Param("id")

	var req ResolveInvestigationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	now := time.Now().UTC()
	investigation.ResolvedAt = &now
	investigation.ResolvedBy = req.ResolvedBy
	if investigation.ResolvedBy == "" {
		investigation.ResolvedBy = "user"
	}
	investigation.Resolution = req.Resolution
	investigation.UpdatedAt = now

	c.JSON(http.StatusOK, gin.H{"status": "resolved", "investigation_id": id})
}

// GetHypotheses returns hypotheses for an investigation
func (h *InvestigationHandler) GetHypotheses(c *gin.Context) {
	id := c.Param("id")

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"investigation_id": id,
		"hypotheses":       investigation.Hypotheses,
	})
}

// VerifyHypothesis verifies or rejects a hypothesis
func (h *InvestigationHandler) VerifyHypothesis(c *gin.Context) {
	id := c.Param("id")
	hypothesisID := c.Param("hypothesis_id")

	var req VerifyHypothesisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	found := false
	now := time.Now().UTC()
	for i := range investigation.Hypotheses {
		if investigation.Hypotheses[i].ID == hypothesisID {
			investigation.Hypotheses[i].Verified = req.Verified
			investigation.Hypotheses[i].VerifiedBy = "user"
			investigation.Hypotheses[i].VerifiedAt = &now
			investigation.Hypotheses[i].VerificationNotes = req.Notes
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "Hypothesis not found"})
		return
	}

	investigation.UpdatedAt = now

	c.JSON(http.StatusOK, gin.H{
		"status":        "updated",
		"hypothesis_id": hypothesisID,
		"verified":      req.Verified,
	})
}

// GetTimeline returns the timeline for an investigation
func (h *InvestigationHandler) GetTimeline(c *gin.Context) {
	id := c.Param("id")

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"investigation_id": id,
		"timeline":         investigation.Timeline,
	})
}

// GetEvidence returns evidence for an investigation
func (h *InvestigationHandler) GetEvidence(c *gin.Context) {
	id := c.Param("id")

	investigation, exists := h.investigations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Investigation not found"})
		return
	}

	// In a real implementation, evidence would be stored separately
	c.JSON(http.StatusOK, gin.H{
		"investigation_id": id,
		"evidence":         []interface{}{},
		"total":            investigation.EvidenceCount,
	})
}

// GetStats returns investigation statistics
func (h *InvestigationHandler) GetStats(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "24h")

	statusCounts := map[string]int{
		"pending":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
		"cancelled": 0,
	}

	triggerCounts := map[string]int{
		"anomaly":       0,
		"alert":         0,
		"slo_breach":    0,
		"error_spike":   0,
		"latency_spike": 0,
		"manual":        0,
	}

	severityCounts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
		"info":     0,
	}

	serviceCounts := make(map[string]int)
	totalHypotheses := 0
	verifiedHypotheses := 0

	for _, inv := range h.investigations {
		statusCounts[inv.Status]++
		triggerCounts[inv.TriggerType]++
		severityCounts[inv.Severity]++

		for _, svc := range inv.AffectedServices {
			serviceCounts[svc]++
		}

		totalHypotheses += len(inv.Hypotheses)
		for _, hyp := range inv.Hypotheses {
			if hyp.Verified {
				verifiedHypotheses++
			}
		}
	}

	c.JSON(http.StatusOK, InvestigationStatsResponse{
		TimeRange:                timeRange,
		TotalInvestigations:      len(h.investigations),
		ByStatus:                 statusCounts,
		ByTriggerType:            triggerCounts,
		BySeverity:               severityCounts,
		AverageDurationSeconds:   0, // Would calculate from actual data
		TopAffectedServices:      serviceCounts,
		TotalHypothesesGenerated: totalHypotheses,
		VerifiedHypotheses:       verifiedHypotheses,
	})
}

// GetTriggerStats returns trigger performance statistics
func (h *InvestigationHandler) GetTriggerStats(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	// In production, this would analyze historical trigger performance
	c.JSON(http.StatusOK, gin.H{
		"time_range":    timeRange,
		"trigger_stats": map[string]interface{}{},
	})
}

// Trigger management endpoints

// ListTriggers lists all trigger configurations
func (h *InvestigationHandler) ListTriggers(c *gin.Context) {
	triggers := make([]InvestigationTriggerConfig, 0)
	for _, t := range h.triggers {
		triggers = append(triggers, *t)
	}

	c.JSON(http.StatusOK, gin.H{
		"triggers": triggers,
		"total":    len(triggers),
	})
}

// GetTrigger gets a specific trigger
func (h *InvestigationHandler) GetTrigger(c *gin.Context) {
	id := c.Param("id")

	trigger, exists := h.triggers[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger not found"})
		return
	}

	c.JSON(http.StatusOK, trigger)
}

// CreateTrigger creates a new trigger
func (h *InvestigationHandler) CreateTrigger(c *gin.Context) {
	var trigger InvestigationTriggerConfig
	if err := c.ShouldBindJSON(&trigger); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trigger.ID = uuid.New().String()
	h.triggers[trigger.ID] = &trigger

	c.JSON(http.StatusOK, gin.H{
		"status":  "created",
		"trigger": trigger,
	})
}

// UpdateTrigger updates an existing trigger
func (h *InvestigationHandler) UpdateTrigger(c *gin.Context) {
	id := c.Param("id")

	_, exists := h.triggers[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger not found"})
		return
	}

	var trigger InvestigationTriggerConfig
	if err := c.ShouldBindJSON(&trigger); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trigger.ID = id
	h.triggers[id] = &trigger

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"trigger": trigger,
	})
}

// DeleteTrigger deletes a trigger
func (h *InvestigationHandler) DeleteTrigger(c *gin.Context) {
	id := c.Param("id")

	if _, exists := h.triggers[id]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger not found"})
		return
	}

	delete(h.triggers, id)

	c.JSON(http.StatusOK, gin.H{
		"status":     "deleted",
		"trigger_id": id,
	})
}

// EnableTrigger enables a trigger
func (h *InvestigationHandler) EnableTrigger(c *gin.Context) {
	id := c.Param("id")

	trigger, exists := h.triggers[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger not found"})
		return
	}

	trigger.Enabled = true

	c.JSON(http.StatusOK, gin.H{
		"status":     "enabled",
		"trigger_id": id,
	})
}

// DisableTrigger disables a trigger
func (h *InvestigationHandler) DisableTrigger(c *gin.Context) {
	id := c.Param("id")

	trigger, exists := h.triggers[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger not found"})
		return
	}

	trigger.Enabled = false

	c.JSON(http.StatusOK, gin.H{
		"status":     "disabled",
		"trigger_id": id,
	})
}

// StartMonitoring starts the trigger monitoring
func (h *InvestigationHandler) StartMonitoring(c *gin.Context) {
	// In production, this would start the background monitoring goroutine
	c.JSON(http.StatusOK, gin.H{
		"status":  "started",
		"message": "Trigger monitoring started",
	})
}

// StopMonitoring stops the trigger monitoring
func (h *InvestigationHandler) StopMonitoring(c *gin.Context) {
	// In production, this would stop the background monitoring goroutine
	c.JSON(http.StatusOK, gin.H{
		"status":  "stopped",
		"message": "Trigger monitoring stopped",
	})
}
