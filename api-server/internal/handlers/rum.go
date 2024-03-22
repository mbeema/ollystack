package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RUMEvent represents a Real User Monitoring event
type RUMEvent struct {
	Type        string                 `json:"type"`
	Timestamp   int64                  `json:"timestamp"`
	SessionID   string                 `json:"sessionId"`
	TraceID     string                 `json:"traceId,omitempty"`
	SpanID      string                 `json:"spanId,omitempty"`
	Application string                 `json:"application"`
	Version     string                 `json:"version,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	URL         string                 `json:"url"`
	UserAgent   string                 `json:"userAgent"`
	User        *RUMUser               `json:"user,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Data        map[string]interface{} `json:"data"`
}

// RUMUser represents user information
type RUMUser struct {
	ID       string `json:"id,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
}

// RUMEventsRequest represents a batch of RUM events
type RUMEventsRequest struct {
	Events   []RUMEvent         `json:"events" binding:"required"`
	Metadata RUMEventsMetadata  `json:"metadata"`
}

// RUMEventsMetadata contains SDK information
type RUMEventsMetadata struct {
	SDKVersion string `json:"sdkVersion"`
	SDKName    string `json:"sdkName"`
}

// RUMSession represents a user session
type RUMSession struct {
	ID           string                 `json:"id"`
	ApplicationID string                `json:"application_id"`
	StartTime    time.Time              `json:"start_time"`
	LastActivity time.Time              `json:"last_activity"`
	UserID       string                 `json:"user_id,omitempty"`
	UserAgent    string                 `json:"user_agent"`
	Country      string                 `json:"country,omitempty"`
	City         string                 `json:"city,omitempty"`
	PageViews    int                    `json:"page_views"`
	Errors       int                    `json:"errors"`
	Tags         map[string]string      `json:"tags,omitempty"`
}

// RUMPageView represents a page view
type RUMPageView struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	Timestamp    time.Time `json:"timestamp"`
	URL          string    `json:"url"`
	Path         string    `json:"path"`
	Title        string    `json:"title"`
	Referrer     string    `json:"referrer"`
	LoadTime     float64   `json:"load_time,omitempty"`
	WebVitals    *WebVitals `json:"web_vitals,omitempty"`
}

// WebVitals represents Core Web Vitals metrics
type WebVitals struct {
	LCP  float64 `json:"lcp,omitempty"`  // Largest Contentful Paint
	FID  float64 `json:"fid,omitempty"`  // First Input Delay
	CLS  float64 `json:"cls,omitempty"`  // Cumulative Layout Shift
	FCP  float64 `json:"fcp,omitempty"`  // First Contentful Paint
	TTFB float64 `json:"ttfb,omitempty"` // Time to First Byte
	INP  float64 `json:"inp,omitempty"`  // Interaction to Next Paint
}

// RUMError represents a JavaScript error
type RUMError struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	Timestamp    time.Time `json:"timestamp"`
	Message      string    `json:"message"`
	Stack        string    `json:"stack,omitempty"`
	Type         string    `json:"type"`
	Filename     string    `json:"filename,omitempty"`
	LineNo       int       `json:"lineno,omitempty"`
	ColNo        int       `json:"colno,omitempty"`
	Handled      bool      `json:"handled"`
	URL          string    `json:"url"`
}

// RUMStatsResponse represents RUM statistics
type RUMStatsResponse struct {
	TimeRange          string                 `json:"time_range"`
	TotalSessions      int                    `json:"total_sessions"`
	TotalPageViews     int                    `json:"total_page_views"`
	TotalErrors        int                    `json:"total_errors"`
	UniqueUsers        int                    `json:"unique_users"`
	AverageSessionDuration float64            `json:"average_session_duration_seconds"`
	TopPages           []PageStats            `json:"top_pages"`
	TopErrors          []ErrorStats           `json:"top_errors"`
	WebVitalsAverage   *WebVitals             `json:"web_vitals_average"`
	ByBrowser          map[string]int         `json:"by_browser"`
	ByCountry          map[string]int         `json:"by_country"`
}

// PageStats represents page statistics
type PageStats struct {
	Path       string  `json:"path"`
	Views      int     `json:"views"`
	AvgLoadTime float64 `json:"avg_load_time"`
}

// ErrorStats represents error statistics
type ErrorStats struct {
	Message    string `json:"message"`
	Type       string `json:"type"`
	Count      int    `json:"count"`
	AffectedSessions int `json:"affected_sessions"`
}

// RUMHandler handles RUM API requests
type RUMHandler struct {
	// In production, this would have storage references
	sessions   map[string]*RUMSession
	pageViews  []RUMPageView
	errors     []RUMError
}

// NewRUMHandler creates a new RUM handler
func NewRUMHandler() *RUMHandler {
	return &RUMHandler{
		sessions:  make(map[string]*RUMSession),
		pageViews: make([]RUMPageView, 0),
		errors:    make([]RUMError, 0),
	}
}

// RegisterRoutes registers RUM API routes
func (h *RUMHandler) RegisterRoutes(r *gin.RouterGroup) {
	rum := r.Group("/rum")
	{
		// Event ingestion
		rum.POST("/events", h.IngestEvents)

		// Session management
		rum.GET("/sessions", h.ListSessions)
		rum.GET("/sessions/:id", h.GetSession)
		rum.GET("/sessions/:id/events", h.GetSessionEvents)
		rum.GET("/sessions/:id/replay", h.GetSessionReplay)

		// Page views
		rum.GET("/pageviews", h.ListPageViews)

		// Errors
		rum.GET("/errors", h.ListErrors)
		rum.GET("/errors/:id", h.GetError)

		// Analytics
		rum.GET("/stats", h.GetStats)
		rum.GET("/stats/web-vitals", h.GetWebVitalsStats)
		rum.GET("/stats/errors", h.GetErrorStats)
		rum.GET("/stats/pages", h.GetPageStats)
	}
}

// IngestEvents handles RUM event ingestion
func (h *RUMHandler) IngestEvents(c *gin.Context) {
	var req RUMEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processed := 0
	for _, event := range req.Events {
		if err := h.processEvent(event); err != nil {
			// Log error but continue processing
			continue
		}
		processed++
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"processed": processed,
		"total":     len(req.Events),
	})
}

func (h *RUMHandler) processEvent(event RUMEvent) error {
	// Update or create session
	session, exists := h.sessions[event.SessionID]
	if !exists {
		session = &RUMSession{
			ID:           event.SessionID,
			ApplicationID: event.Application,
			StartTime:    time.UnixMilli(event.Timestamp),
			LastActivity: time.UnixMilli(event.Timestamp),
			UserAgent:    event.UserAgent,
			Tags:         event.Tags,
		}
		h.sessions[event.SessionID] = session
	}
	session.LastActivity = time.UnixMilli(event.Timestamp)

	if event.User != nil && event.User.ID != "" {
		session.UserID = event.User.ID
	}

	// Process by event type
	switch event.Type {
	case "page_view":
		h.processPageView(event)
		session.PageViews++
	case "error":
		h.processError(event)
		session.Errors++
	case "performance":
		h.processPerformance(event)
	case "network":
		h.processNetwork(event)
	case "interaction":
		h.processInteraction(event)
	}

	return nil
}

func (h *RUMHandler) processPageView(event RUMEvent) {
	data := event.Data
	pageView := RUMPageView{
		ID:        uuid.New().String(),
		SessionID: event.SessionID,
		Timestamp: time.UnixMilli(event.Timestamp),
		URL:       event.URL,
		Path:      getStringValue(data, "path"),
		Title:     getStringValue(data, "title"),
		Referrer:  getStringValue(data, "referrer"),
	}

	if loadTime, ok := data["loadTime"].(float64); ok {
		pageView.LoadTime = loadTime
	}

	h.pageViews = append(h.pageViews, pageView)
}

func (h *RUMHandler) processError(event RUMEvent) {
	data := event.Data
	rumError := RUMError{
		ID:        uuid.New().String(),
		SessionID: event.SessionID,
		Timestamp: time.UnixMilli(event.Timestamp),
		Message:   getStringValue(data, "message"),
		Stack:     getStringValue(data, "stack"),
		Type:      getStringValue(data, "type"),
		Filename:  getStringValue(data, "filename"),
		URL:       event.URL,
	}

	if lineno, ok := data["lineno"].(float64); ok {
		rumError.LineNo = int(lineno)
	}
	if colno, ok := data["colno"].(float64); ok {
		rumError.ColNo = int(colno)
	}
	if handled, ok := data["handled"].(bool); ok {
		rumError.Handled = handled
	}

	h.errors = append(h.errors, rumError)
}

func (h *RUMHandler) processPerformance(event RUMEvent) {
	// Process performance data (web vitals, etc.)
	// In production, this would be stored in ClickHouse
}

func (h *RUMHandler) processNetwork(event RUMEvent) {
	// Process network request data
	// In production, this would be stored and correlated with backend traces
}

func (h *RUMHandler) processInteraction(event RUMEvent) {
	// Process user interaction data
	// In production, this would be used for session replay and heatmaps
}

// ListSessions returns a list of sessions
func (h *RUMHandler) ListSessions(c *gin.Context) {
	application := c.Query("application")

	sessions := make([]*RUMSession, 0)
	for _, s := range h.sessions {
		if application != "" && s.ApplicationID != application {
			continue
		}
		sessions = append(sessions, s)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// GetSession returns a specific session
func (h *RUMHandler) GetSession(c *gin.Context) {
	id := c.Param("id")

	session, exists := h.sessions[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// GetSessionEvents returns events for a session
func (h *RUMHandler) GetSessionEvents(c *gin.Context) {
	id := c.Param("id")

	// Get page views for session
	var pageViews []RUMPageView
	for _, pv := range h.pageViews {
		if pv.SessionID == id {
			pageViews = append(pageViews, pv)
		}
	}

	// Get errors for session
	var errors []RUMError
	for _, e := range h.errors {
		if e.SessionID == id {
			errors = append(errors, e)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": id,
		"page_views": pageViews,
		"errors":     errors,
	})
}

// GetSessionReplay returns session replay data
func (h *RUMHandler) GetSessionReplay(c *gin.Context) {
	id := c.Param("id")

	// In production, this would return recorded DOM mutations and interactions
	c.JSON(http.StatusOK, gin.H{
		"session_id": id,
		"replay":     nil,
		"available":  false,
		"message":    "Session replay not enabled for this session",
	})
}

// ListPageViews returns page views
func (h *RUMHandler) ListPageViews(c *gin.Context) {
	application := c.Query("application")
	path := c.Query("path")

	views := make([]RUMPageView, 0)
	for _, pv := range h.pageViews {
		if path != "" && pv.Path != path {
			continue
		}
		views = append(views, pv)
	}

	// In production, would filter by application
	_ = application

	c.JSON(http.StatusOK, gin.H{
		"page_views": views,
		"total":      len(views),
	})
}

// ListErrors returns errors
func (h *RUMHandler) ListErrors(c *gin.Context) {
	errorType := c.Query("type")

	errors := make([]RUMError, 0)
	for _, e := range h.errors {
		if errorType != "" && e.Type != errorType {
			continue
		}
		errors = append(errors, e)
	}

	c.JSON(http.StatusOK, gin.H{
		"errors": errors,
		"total":  len(errors),
	})
}

// GetError returns a specific error
func (h *RUMHandler) GetError(c *gin.Context) {
	id := c.Param("id")

	for _, e := range h.errors {
		if e.ID == id {
			c.JSON(http.StatusOK, e)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Error not found"})
}

// GetStats returns RUM statistics
func (h *RUMHandler) GetStats(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "24h")

	// Calculate unique users
	uniqueUsers := make(map[string]bool)
	for _, s := range h.sessions {
		if s.UserID != "" {
			uniqueUsers[s.UserID] = true
		}
	}

	stats := RUMStatsResponse{
		TimeRange:      timeRange,
		TotalSessions:  len(h.sessions),
		TotalPageViews: len(h.pageViews),
		TotalErrors:    len(h.errors),
		UniqueUsers:    len(uniqueUsers),
		TopPages:       h.calculateTopPages(),
		TopErrors:      h.calculateTopErrors(),
		WebVitalsAverage: &WebVitals{}, // Would calculate from actual data
		ByBrowser:      make(map[string]int),
		ByCountry:      make(map[string]int),
	}

	c.JSON(http.StatusOK, stats)
}

// GetWebVitalsStats returns Web Vitals statistics
func (h *RUMHandler) GetWebVitalsStats(c *gin.Context) {
	// In production, this would aggregate Web Vitals from ClickHouse
	c.JSON(http.StatusOK, gin.H{
		"lcp": gin.H{
			"p50": 2500.0,
			"p75": 3500.0,
			"p90": 4500.0,
			"p99": 6000.0,
		},
		"fid": gin.H{
			"p50": 50.0,
			"p75": 100.0,
			"p90": 150.0,
			"p99": 300.0,
		},
		"cls": gin.H{
			"p50": 0.05,
			"p75": 0.1,
			"p90": 0.15,
			"p99": 0.25,
		},
		"inp": gin.H{
			"p50": 100.0,
			"p75": 200.0,
			"p90": 300.0,
			"p99": 500.0,
		},
	})
}

// GetErrorStats returns error statistics
func (h *RUMHandler) GetErrorStats(c *gin.Context) {
	stats := h.calculateTopErrors()
	c.JSON(http.StatusOK, gin.H{
		"errors": stats,
		"total":  len(h.errors),
	})
}

// GetPageStats returns page statistics
func (h *RUMHandler) GetPageStats(c *gin.Context) {
	stats := h.calculateTopPages()
	c.JSON(http.StatusOK, gin.H{
		"pages": stats,
		"total": len(h.pageViews),
	})
}

func (h *RUMHandler) calculateTopPages() []PageStats {
	pageCounts := make(map[string]int)
	for _, pv := range h.pageViews {
		pageCounts[pv.Path]++
	}

	var stats []PageStats
	for path, count := range pageCounts {
		stats = append(stats, PageStats{
			Path:  path,
			Views: count,
		})
	}

	return stats
}

func (h *RUMHandler) calculateTopErrors() []ErrorStats {
	errorCounts := make(map[string]*ErrorStats)
	for _, e := range h.errors {
		key := e.Message
		if _, exists := errorCounts[key]; !exists {
			errorCounts[key] = &ErrorStats{
				Message: e.Message,
				Type:    e.Type,
			}
		}
		errorCounts[key].Count++
	}

	var stats []ErrorStats
	for _, s := range errorCounts {
		stats = append(stats, *s)
	}

	return stats
}

func getStringValue(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}
