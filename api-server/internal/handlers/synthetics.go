package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Canary represents a synthetic monitoring canary
type Canary struct {
	ID          string `json:"id"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	CanaryType  string `json:"canary_type" binding:"required,oneof=api browser heartbeat"`

	// Configuration
	Script         string   `json:"script,omitempty"`         // For browser canaries
	TargetURL      string   `json:"target_url" binding:"required"`
	TargetService  string   `json:"target_service"`
	Schedule       string   `json:"schedule" binding:"required"` // Cron expression
	TimeoutSeconds int      `json:"timeout_seconds"`
	Regions        []string `json:"regions"`

	// Thresholds
	SuccessThreshold    float64 `json:"success_threshold"`
	LatencyThresholdMs  int     `json:"latency_threshold_ms"`

	// HTTP Configuration (for API canaries)
	HTTPMethod  string            `json:"http_method,omitempty"`
	HTTPHeaders map[string]string `json:"http_headers,omitempty"`
	HTTPBody    string            `json:"http_body,omitempty"`

	// Assertions
	Assertions []Assertion `json:"assertions,omitempty"`

	// Status
	Status         string    `json:"status"`
	LastRunAt      time.Time `json:"last_run_at,omitempty"`
	LastRunSuccess bool      `json:"last_run_success"`

	// Alerting
	AlertEnabled  bool     `json:"alert_enabled"`
	AlertChannels []string `json:"alert_channels,omitempty"`

	// Metadata
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Assertion represents a test assertion for a canary
type Assertion struct {
	Type     string `json:"type"`     // status_code, body_contains, header, latency, json_path
	Target   string `json:"target"`   // e.g., "$.data.id" for json_path
	Operator string `json:"operator"` // eq, ne, contains, gt, lt
	Value    string `json:"value"`
}

// CanaryRun represents a single run of a canary
type CanaryRun struct {
	ID       string    `json:"id"`
	CanaryID string    `json:"canary_id"`
	Timestamp time.Time `json:"timestamp"`
	Region    string    `json:"region"`

	// Results
	Success    bool `json:"success"`
	StatusCode int  `json:"status_code,omitempty"`

	// Timing
	DurationMs        float64 `json:"duration_ms"`
	DNSLookupMs       float64 `json:"dns_lookup_ms,omitempty"`
	TCPConnectMs      float64 `json:"tcp_connect_ms,omitempty"`
	TLSHandshakeMs    float64 `json:"tls_handshake_ms,omitempty"`
	TimeToFirstByteMs float64 `json:"time_to_first_byte_ms,omitempty"`

	// Steps (for multi-step browser tests)
	Steps []CanaryStep `json:"steps,omitempty"`

	// Error
	ErrorType    string `json:"error_type,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Trace correlation
	TraceID string `json:"trace_id,omitempty"`
}

// CanaryStep represents a step in a multi-step canary
type CanaryStep struct {
	Number        int     `json:"number"`
	Name          string  `json:"name"`
	DurationMs    float64 `json:"duration_ms"`
	Success       bool    `json:"success"`
	ErrorMessage  string  `json:"error_message,omitempty"`
	ScreenshotURL string  `json:"screenshot_url,omitempty"`
}

// CanaryStats represents statistics for a canary
type CanaryStats struct {
	CanaryID    string  `json:"canary_id"`
	TimeRange   string  `json:"time_range"`
	TotalRuns   int64   `json:"total_runs"`
	SuccessRuns int64   `json:"success_runs"`
	FailedRuns  int64   `json:"failed_runs"`
	SuccessRate float64 `json:"success_rate"`

	// Latency stats
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P90LatencyMs float64 `json:"p90_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`

	// By region
	ByRegion map[string]RegionStats `json:"by_region"`
}

// RegionStats represents stats for a specific region
type RegionStats struct {
	TotalRuns    int64   `json:"total_runs"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// SyntheticsHandler handles synthetics/canary API requests
type SyntheticsHandler struct{}

// NewSyntheticsHandler creates a new synthetics handler
func NewSyntheticsHandler() *SyntheticsHandler {
	return &SyntheticsHandler{}
}

// ListCanaries returns all canaries
// GET /api/v1/synthetics/canaries
func (h *SyntheticsHandler) ListCanaries(c *gin.Context) {
	status := c.DefaultQuery("status", "active")
	canaryType := c.Query("type")

	// Mock response
	canaries := []Canary{
		{
			ID:          uuid.New().String(),
			Name:        "Checkout API Health",
			Description: "Monitors the checkout API endpoint",
			CanaryType:  "api",
			TargetURL:   "https://api.example.com/v1/checkout/health",
			Schedule:    "*/5 * * * *", // Every 5 minutes
			HTTPMethod:  "GET",
			Regions:     []string{"us-east-1", "eu-west-1", "ap-southeast-1"},
			Assertions: []Assertion{
				{Type: "status_code", Operator: "eq", Value: "200"},
				{Type: "latency", Operator: "lt", Value: "500"},
				{Type: "body_contains", Operator: "contains", Value: "healthy"},
			},
			Status:           "active",
			LastRunAt:        time.Now().Add(-2 * time.Minute),
			LastRunSuccess:   true,
			SuccessThreshold: 0.95,
			AlertEnabled:     true,
			CreatedAt:        time.Now().Add(-30 * 24 * time.Hour),
			UpdatedAt:        time.Now(),
		},
		{
			ID:          uuid.New().String(),
			Name:        "Login Flow",
			Description: "End-to-end login flow test",
			CanaryType:  "browser",
			TargetURL:   "https://app.example.com/login",
			Schedule:    "*/15 * * * *", // Every 15 minutes
			Script: `
const { chromium } = require('playwright');

module.exports = async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  await page.goto('https://app.example.com/login');
  await page.fill('#email', 'test@example.com');
  await page.fill('#password', 'testpassword');
  await page.click('button[type="submit"]');
  await page.waitForSelector('.dashboard');

  await browser.close();
};`,
			Regions:            []string{"us-east-1"},
			Status:             "active",
			LastRunAt:          time.Now().Add(-10 * time.Minute),
			LastRunSuccess:     true,
			LatencyThresholdMs: 5000,
			AlertEnabled:       true,
			CreatedAt:          time.Now().Add(-7 * 24 * time.Hour),
			UpdatedAt:          time.Now(),
		},
	}

	// Filter by status and type
	filtered := []Canary{}
	for _, canary := range canaries {
		if status != "all" && canary.Status != status {
			continue
		}
		if canaryType != "" && canary.CanaryType != canaryType {
			continue
		}
		filtered = append(filtered, canary)
	}

	c.JSON(http.StatusOK, gin.H{
		"canaries": filtered,
		"total":    len(filtered),
	})
}

// GetCanary returns a single canary by ID
// GET /api/v1/synthetics/canaries/:id
func (h *SyntheticsHandler) GetCanary(c *gin.Context) {
	id := c.Param("id")

	canary := Canary{
		ID:          id,
		Name:        "Checkout API Health",
		Description: "Monitors the checkout API endpoint",
		CanaryType:  "api",
		TargetURL:   "https://api.example.com/v1/checkout/health",
		Schedule:    "*/5 * * * *",
		HTTPMethod:  "GET",
		Regions:     []string{"us-east-1", "eu-west-1", "ap-southeast-1"},
		Assertions: []Assertion{
			{Type: "status_code", Operator: "eq", Value: "200"},
			{Type: "latency", Operator: "lt", Value: "500"},
		},
		Status:         "active",
		LastRunAt:      time.Now().Add(-2 * time.Minute),
		LastRunSuccess: true,
		AlertEnabled:   true,
		CreatedAt:      time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt:      time.Now(),
	}

	c.JSON(http.StatusOK, canary)
}

// CreateCanary creates a new canary
// POST /api/v1/synthetics/canaries
func (h *SyntheticsHandler) CreateCanary(c *gin.Context) {
	var canary Canary
	if err := c.ShouldBindJSON(&canary); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	canary.ID = uuid.New().String()
	canary.Status = "active"
	canary.CreatedAt = time.Now()
	canary.UpdatedAt = time.Now()

	// Set defaults
	if canary.TimeoutSeconds == 0 {
		canary.TimeoutSeconds = 30
	}
	if canary.SuccessThreshold == 0 {
		canary.SuccessThreshold = 0.95
	}
	if len(canary.Regions) == 0 {
		canary.Regions = []string{"us-east-1"}
	}

	c.JSON(http.StatusCreated, canary)
}

// UpdateCanary updates an existing canary
// PUT /api/v1/synthetics/canaries/:id
func (h *SyntheticsHandler) UpdateCanary(c *gin.Context) {
	id := c.Param("id")

	var canary Canary
	if err := c.ShouldBindJSON(&canary); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	canary.ID = id
	canary.UpdatedAt = time.Now()

	c.JSON(http.StatusOK, canary)
}

// DeleteCanary deletes a canary
// DELETE /api/v1/synthetics/canaries/:id
func (h *SyntheticsHandler) DeleteCanary(c *gin.Context) {
	id := c.Param("id")

	c.JSON(http.StatusOK, gin.H{
		"message": "Canary deleted",
		"id":      id,
	})
}

// RunCanary triggers an immediate run of a canary
// POST /api/v1/synthetics/canaries/:id/run
func (h *SyntheticsHandler) RunCanary(c *gin.Context) {
	id := c.Param("id")

	// In production, trigger the canary execution
	runID := uuid.New().String()

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "Canary run triggered",
		"canary_id": id,
		"run_id":    runID,
		"status":    "pending",
	})
}

// GetCanaryRuns returns recent runs for a canary
// GET /api/v1/synthetics/canaries/:id/runs
func (h *SyntheticsHandler) GetCanaryRuns(c *gin.Context) {
	id := c.Param("id")
	limit := 20

	// Mock runs
	runs := []CanaryRun{}
	now := time.Now()

	for i := 0; i < limit; i++ {
		success := i%5 != 0 // 80% success rate
		run := CanaryRun{
			ID:                uuid.New().String(),
			CanaryID:          id,
			Timestamp:         now.Add(-time.Duration(i*5) * time.Minute),
			Region:            []string{"us-east-1", "eu-west-1", "ap-southeast-1"}[i%3],
			Success:           success,
			StatusCode:        200,
			DurationMs:        150 + float64(i*10),
			DNSLookupMs:       5 + float64(i),
			TCPConnectMs:      10 + float64(i),
			TLSHandshakeMs:    20 + float64(i),
			TimeToFirstByteMs: 100 + float64(i*5),
			TraceID:           uuid.New().String(),
		}

		if !success {
			run.StatusCode = 500
			run.ErrorType = "http_error"
			run.ErrorMessage = "Internal Server Error"
		}

		runs = append(runs, run)
	}

	c.JSON(http.StatusOK, gin.H{
		"runs":  runs,
		"total": len(runs),
	})
}

// GetCanaryStats returns statistics for a canary
// GET /api/v1/synthetics/canaries/:id/stats
func (h *SyntheticsHandler) GetCanaryStats(c *gin.Context) {
	id := c.Param("id")
	timeRange := c.DefaultQuery("range", "24h")

	stats := CanaryStats{
		CanaryID:     id,
		TimeRange:    timeRange,
		TotalRuns:    288,
		SuccessRuns:  275,
		FailedRuns:   13,
		SuccessRate:  95.5,
		AvgLatencyMs: 185.5,
		P50LatencyMs: 165.0,
		P90LatencyMs: 320.0,
		P99LatencyMs: 580.0,
		ByRegion: map[string]RegionStats{
			"us-east-1": {
				TotalRuns:    96,
				SuccessRate:  97.9,
				AvgLatencyMs: 120.5,
			},
			"eu-west-1": {
				TotalRuns:    96,
				SuccessRate:  94.8,
				AvgLatencyMs: 185.2,
			},
			"ap-southeast-1": {
				TotalRuns:    96,
				SuccessRate:  93.8,
				AvgLatencyMs: 250.8,
			},
		},
	}

	c.JSON(http.StatusOK, stats)
}

// GetSyntheticsSummary returns a summary of all canaries
// GET /api/v1/synthetics/summary
func (h *SyntheticsHandler) GetSyntheticsSummary(c *gin.Context) {
	summary := gin.H{
		"total_canaries":   12,
		"active_canaries":  10,
		"paused_canaries":  2,
		"healthy":          8,
		"degraded":         2,
		"failing":          2,
		"overall_success_rate": 94.5,
		"by_type": gin.H{
			"api":       6,
			"browser":   4,
			"heartbeat": 2,
		},
		"by_region": gin.H{
			"us-east-1":      gin.H{"success_rate": 96.2, "avg_latency_ms": 125},
			"eu-west-1":      gin.H{"success_rate": 94.8, "avg_latency_ms": 180},
			"ap-southeast-1": gin.H{"success_rate": 92.5, "avg_latency_ms": 245},
		},
		"recent_failures": []gin.H{
			{
				"canary_id":   "abc123",
				"canary_name": "Payment API",
				"timestamp":   time.Now().Add(-10 * time.Minute),
				"error":       "Connection timeout",
				"region":      "ap-southeast-1",
			},
			{
				"canary_id":   "def456",
				"canary_name": "Login Flow",
				"timestamp":   time.Now().Add(-25 * time.Minute),
				"error":       "Element not found: .dashboard",
				"region":      "us-east-1",
			},
		},
		"updated_at": time.Now(),
	}

	c.JSON(http.StatusOK, summary)
}

// RegisterSyntheticsRoutes registers all synthetics routes
func RegisterSyntheticsRoutes(r *gin.RouterGroup) {
	h := NewSyntheticsHandler()

	synthetics := r.Group("/synthetics")
	{
		synthetics.GET("/summary", h.GetSyntheticsSummary)

		canaries := synthetics.Group("/canaries")
		{
			canaries.GET("", h.ListCanaries)
			canaries.POST("", h.CreateCanary)
			canaries.GET("/:id", h.GetCanary)
			canaries.PUT("/:id", h.UpdateCanary)
			canaries.DELETE("/:id", h.DeleteCanary)
			canaries.POST("/:id/run", h.RunCanary)
			canaries.GET("/:id/runs", h.GetCanaryRuns)
			canaries.GET("/:id/stats", h.GetCanaryStats)
		}
	}
}
