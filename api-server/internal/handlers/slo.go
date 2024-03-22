package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SLO represents a Service Level Objective
type SLO struct {
	ID          string            `json:"id"`
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	ServiceName string            `json:"service_name" binding:"required"`
	Operation   string            `json:"operation"`

	// SLI Definition
	SLIType      string  `json:"sli_type" binding:"required,oneof=latency error_rate availability throughput"`
	SLIThreshold float64 `json:"sli_threshold" binding:"required"`
	SLIOperator  string  `json:"sli_operator" binding:"required,oneof=lt gt lte gte"`

	// SLO Target
	TargetPercentage float64 `json:"target_percentage" binding:"required,min=0,max=100"`
	WindowType       string  `json:"window_type" binding:"required,oneof=rolling calendar"`
	WindowDays       int     `json:"window_days" binding:"required,min=1,max=90"`
	EvaluationType   string  `json:"evaluation_type" binding:"required,oneof=period_based request_based"`

	// Burn Rate Alerting
	BurnRateFast  float64  `json:"burn_rate_fast"`
	BurnRateSlow  float64  `json:"burn_rate_slow"`
	AlertEnabled  bool     `json:"alert_enabled"`
	AlertChannels []string `json:"alert_channels"`

	// Metadata
	Labels    map[string]string `json:"labels"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// SLOStatus represents the current status of an SLO
type SLOStatus struct {
	SLOID       string `json:"slo_id"`
	SLOName     string `json:"slo_name"`
	ServiceName string `json:"service_name"`

	// Current state
	CurrentSLI        float64 `json:"current_sli"`
	CurrentAttainment float64 `json:"current_attainment"`
	TargetAttainment  float64 `json:"target_attainment"`
	IsMet             bool    `json:"is_met"`

	// Error budget
	ErrorBudgetTotal           float64   `json:"error_budget_total"`
	ErrorBudgetRemaining       float64   `json:"error_budget_remaining"`
	ErrorBudgetRemainingPct    float64   `json:"error_budget_remaining_percent"`
	ErrorBudgetConsumedPct     float64   `json:"error_budget_consumed_percent"`
	ProjectedBudgetExhaustion  time.Time `json:"projected_budget_exhaustion,omitempty"`

	// Burn rates
	BurnRate1h  float64 `json:"burn_rate_1h"`
	BurnRate6h  float64 `json:"burn_rate_6h"`
	BurnRate24h float64 `json:"burn_rate_24h"`

	// Alert
	AlertStatus  string `json:"alert_status"` // ok, warning, critical
	AlertMessage string `json:"alert_message,omitempty"`

	// Time range
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SLOHistory represents SLO measurements over time
type SLOHistory struct {
	SLOID      string         `json:"slo_id"`
	TimeRange  string         `json:"time_range"`
	Resolution string         `json:"resolution"`
	DataPoints []SLODataPoint `json:"data_points"`
}

// SLODataPoint represents a single measurement
type SLODataPoint struct {
	Timestamp               time.Time `json:"timestamp"`
	TotalCount              int64     `json:"total_count"`
	GoodCount               int64     `json:"good_count"`
	BadCount                int64     `json:"bad_count"`
	SLIValue                float64   `json:"sli_value"`
	IsGood                  bool      `json:"is_good"`
	ErrorBudgetRemainingPct float64   `json:"error_budget_remaining_percent"`
	BurnRate                float64   `json:"burn_rate"`
}

// SLOHandler handles SLO-related API requests
type SLOHandler struct {
	// In production, this would have a storage service
}

// NewSLOHandler creates a new SLO handler
func NewSLOHandler() *SLOHandler {
	return &SLOHandler{}
}

// ListSLOs returns all SLOs
// GET /api/v1/slos
func (h *SLOHandler) ListSLOs(c *gin.Context) {
	// Query parameters
	service := c.Query("service")
	status := c.DefaultQuery("status", "active")

	// Mock response - in production, query from ClickHouse
	slos := []SLO{
		{
			ID:               uuid.New().String(),
			Name:             "API Gateway Latency",
			Description:      "99th percentile latency should be under 500ms",
			ServiceName:      "api-gateway",
			SLIType:          "latency",
			SLIThreshold:     500,
			SLIOperator:      "lte",
			TargetPercentage: 99.9,
			WindowType:       "rolling",
			WindowDays:       30,
			EvaluationType:   "request_based",
			BurnRateFast:     14.4,
			BurnRateSlow:     6.0,
			AlertEnabled:     true,
			Status:           "active",
			CreatedAt:        time.Now().Add(-30 * 24 * time.Hour),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               uuid.New().String(),
			Name:             "Payment Service Availability",
			Description:      "Service should be available 99.95% of the time",
			ServiceName:      "payment-service",
			SLIType:          "availability",
			SLIThreshold:     1,
			SLIOperator:      "gte",
			TargetPercentage: 99.95,
			WindowType:       "rolling",
			WindowDays:       30,
			EvaluationType:   "request_based",
			AlertEnabled:     true,
			Status:           "active",
			CreatedAt:        time.Now().Add(-15 * 24 * time.Hour),
			UpdatedAt:        time.Now(),
		},
	}

	// Filter by service if provided
	if service != "" {
		filtered := []SLO{}
		for _, slo := range slos {
			if slo.ServiceName == service {
				filtered = append(filtered, slo)
			}
		}
		slos = filtered
	}

	// Filter by status
	if status != "all" {
		filtered := []SLO{}
		for _, slo := range slos {
			if slo.Status == status {
				filtered = append(filtered, slo)
			}
		}
		slos = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"slos":  slos,
		"total": len(slos),
	})
}

// GetSLO returns a single SLO by ID
// GET /api/v1/slos/:id
func (h *SLOHandler) GetSLO(c *gin.Context) {
	id := c.Param("id")

	// Mock response
	slo := SLO{
		ID:               id,
		Name:             "API Gateway Latency",
		Description:      "99th percentile latency should be under 500ms",
		ServiceName:      "api-gateway",
		SLIType:          "latency",
		SLIThreshold:     500,
		SLIOperator:      "lte",
		TargetPercentage: 99.9,
		WindowType:       "rolling",
		WindowDays:       30,
		EvaluationType:   "request_based",
		BurnRateFast:     14.4,
		BurnRateSlow:     6.0,
		AlertEnabled:     true,
		Status:           "active",
		CreatedAt:        time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt:        time.Now(),
	}

	c.JSON(http.StatusOK, slo)
}

// CreateSLO creates a new SLO
// POST /api/v1/slos
func (h *SLOHandler) CreateSLO(c *gin.Context) {
	var slo SLO
	if err := c.ShouldBindJSON(&slo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	slo.ID = uuid.New().String()
	slo.Status = "active"
	slo.CreatedAt = time.Now()
	slo.UpdatedAt = time.Now()

	if slo.BurnRateFast == 0 {
		slo.BurnRateFast = 14.4 // 2% budget in 1 hour
	}
	if slo.BurnRateSlow == 0 {
		slo.BurnRateSlow = 6.0 // 5% budget in 6 hours
	}

	// In production, save to ClickHouse

	c.JSON(http.StatusCreated, slo)
}

// UpdateSLO updates an existing SLO
// PUT /api/v1/slos/:id
func (h *SLOHandler) UpdateSLO(c *gin.Context) {
	id := c.Param("id")

	var slo SLO
	if err := c.ShouldBindJSON(&slo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slo.ID = id
	slo.UpdatedAt = time.Now()

	// In production, update in ClickHouse

	c.JSON(http.StatusOK, slo)
}

// DeleteSLO deletes an SLO
// DELETE /api/v1/slos/:id
func (h *SLOHandler) DeleteSLO(c *gin.Context) {
	id := c.Param("id")

	// In production, soft delete in ClickHouse

	c.JSON(http.StatusOK, gin.H{
		"message": "SLO deleted",
		"id":      id,
	})
}

// GetSLOStatus returns the current status of an SLO
// GET /api/v1/slos/:id/status
func (h *SLOHandler) GetSLOStatus(c *gin.Context) {
	id := c.Param("id")

	// Mock status - in production, calculate from measurements
	now := time.Now()
	windowStart := now.Add(-30 * 24 * time.Hour)

	status := SLOStatus{
		SLOID:       id,
		SLOName:     "API Gateway Latency",
		ServiceName: "api-gateway",

		CurrentSLI:        485.2, // Current p99 latency in ms
		CurrentAttainment: 99.92,
		TargetAttainment:  99.9,
		IsMet:             true,

		ErrorBudgetTotal:        43.2, // minutes of allowed downtime
		ErrorBudgetRemaining:    38.5,
		ErrorBudgetRemainingPct: 89.1,
		ErrorBudgetConsumedPct:  10.9,

		BurnRate1h:  0.8,
		BurnRate6h:  1.2,
		BurnRate24h: 1.5,

		AlertStatus: "ok",

		WindowStart: windowStart,
		WindowEnd:   now,
		UpdatedAt:   now,
	}

	c.JSON(http.StatusOK, status)
}

// GetSLOHistory returns historical measurements for an SLO
// GET /api/v1/slos/:id/history
func (h *SLOHandler) GetSLOHistory(c *gin.Context) {
	id := c.Param("id")
	timeRange := c.DefaultQuery("range", "24h")
	resolution := c.DefaultQuery("resolution", "1h")

	// Generate mock data points
	dataPoints := []SLODataPoint{}
	now := time.Now()

	// Parse time range
	var duration time.Duration
	switch timeRange {
	case "1h":
		duration = time.Hour
	case "6h":
		duration = 6 * time.Hour
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		duration = 24 * time.Hour
	}

	// Parse resolution
	var step time.Duration
	switch resolution {
	case "1m":
		step = time.Minute
	case "5m":
		step = 5 * time.Minute
	case "1h":
		step = time.Hour
	case "1d":
		step = 24 * time.Hour
	default:
		step = time.Hour
	}

	// Generate data points
	for t := now.Add(-duration); t.Before(now); t = t.Add(step) {
		// Simulate varying SLI values
		sliValue := 99.5 + (float64(t.Unix()%10) * 0.05)
		goodPct := sliValue / 100
		totalCount := int64(10000)
		goodCount := int64(float64(totalCount) * goodPct)

		dataPoints = append(dataPoints, SLODataPoint{
			Timestamp:               t,
			TotalCount:              totalCount,
			GoodCount:               goodCount,
			BadCount:                totalCount - goodCount,
			SLIValue:                sliValue,
			IsGood:                  sliValue >= 99.9,
			ErrorBudgetRemainingPct: 85 + float64(t.Unix()%10),
			BurnRate:                1.0 + float64(t.Unix()%5)*0.2,
		})
	}

	history := SLOHistory{
		SLOID:      id,
		TimeRange:  timeRange,
		Resolution: resolution,
		DataPoints: dataPoints,
	}

	c.JSON(http.StatusOK, history)
}

// GetSLOBurnRate returns burn rate analysis for an SLO
// GET /api/v1/slos/:id/burn-rate
func (h *SLOHandler) GetSLOBurnRate(c *gin.Context) {
	id := c.Param("id")

	// Multi-window, multi-burn-rate analysis
	// Based on Google SRE book recommendations
	burnRateAnalysis := gin.H{
		"slo_id": id,
		"windows": []gin.H{
			{
				"name":              "1h / 5m",
				"long_window":       "1h",
				"short_window":      "5m",
				"burn_rate_long":    2.5,
				"burn_rate_short":   3.2,
				"threshold":         14.4,
				"budget_consumed":   "2.0%",
				"alert_firing":      false,
				"severity":          "critical",
				"time_to_exhaustion": "40h",
			},
			{
				"name":              "6h / 30m",
				"long_window":       "6h",
				"short_window":      "30m",
				"burn_rate_long":    1.8,
				"burn_rate_short":   2.1,
				"threshold":         6.0,
				"budget_consumed":   "5.0%",
				"alert_firing":      false,
				"severity":          "warning",
				"time_to_exhaustion": "166h",
			},
			{
				"name":              "3d / 6h",
				"long_window":       "3d",
				"short_window":      "6h",
				"burn_rate_long":    1.2,
				"burn_rate_short":   1.5,
				"threshold":         1.0,
				"budget_consumed":   "10%",
				"alert_firing":      true,
				"severity":          "info",
				"time_to_exhaustion": "720h",
			},
		},
		"recommendations": []string{
			"Current burn rate is sustainable for the SLO window",
			"Monitor the 3d/6h window - burn rate exceeds threshold",
			"Consider investigating recent latency increases",
		},
	}

	c.JSON(http.StatusOK, burnRateAnalysis)
}

// GetSLOSummary returns a summary of all SLOs
// GET /api/v1/slos/summary
func (h *SLOHandler) GetSLOSummary(c *gin.Context) {
	summary := gin.H{
		"total_slos":     15,
		"meeting_target": 12,
		"at_risk":        2,
		"breached":       1,
		"by_status": gin.H{
			"ok":       12,
			"warning":  2,
			"critical": 1,
		},
		"by_service": []gin.H{
			{"service": "api-gateway", "total": 3, "healthy": 3},
			{"service": "user-service", "total": 4, "healthy": 3},
			{"service": "payment-service", "total": 3, "healthy": 2},
			{"service": "order-service", "total": 5, "healthy": 4},
		},
		"error_budget_status": gin.H{
			"healthy":   10, // >50% remaining
			"at_risk":   3,  // 20-50% remaining
			"exhausted": 2,  // <20% remaining
		},
		"updated_at": time.Now(),
	}

	c.JSON(http.StatusOK, summary)
}

// RegisterSLORoutes registers all SLO routes
func RegisterSLORoutes(r *gin.RouterGroup) {
	h := NewSLOHandler()

	slos := r.Group("/slos")
	{
		slos.GET("", h.ListSLOs)
		slos.GET("/summary", h.GetSLOSummary)
		slos.POST("", h.CreateSLO)
		slos.GET("/:id", h.GetSLO)
		slos.PUT("/:id", h.UpdateSLO)
		slos.DELETE("/:id", h.DeleteSLO)
		slos.GET("/:id/status", h.GetSLOStatus)
		slos.GET("/:id/history", h.GetSLOHistory)
		slos.GET("/:id/burn-rate", h.GetSLOBurnRate)
	}
}
