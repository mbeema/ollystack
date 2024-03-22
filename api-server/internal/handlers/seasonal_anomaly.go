// Package handlers provides HTTP handlers for the OllyStack API.
package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// ClickHouseClient defines the interface for ClickHouse database operations.
type ClickHouseClient interface {
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) Row
	Exec(ctx context.Context, query string, args ...any) error
}

// Rows represents database rows.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
}

// Row represents a single database row.
type Row interface {
	Scan(dest ...any) error
}

// SeasonalAnomalyHandler handles seasonal anomaly detection endpoints.
type SeasonalAnomalyHandler struct {
	clickhouse ClickHouseClient
}

// NewSeasonalAnomalyHandler creates a new seasonal anomaly handler.
func NewSeasonalAnomalyHandler(ch ClickHouseClient) *SeasonalAnomalyHandler {
	return &SeasonalAnomalyHandler{clickhouse: ch}
}

// RegisterRoutes registers the seasonal anomaly routes.
func (h *SeasonalAnomalyHandler) RegisterRoutes(r *gin.RouterGroup) {
	seasonal := r.Group("/seasonal")
	{
		seasonal.POST("/detect", h.DetectSeasonalAnomalies)
		seasonal.GET("/baseline/:service/:metric", h.GetSeasonalBaseline)
		seasonal.GET("/analyze/:service/:metric", h.AnalyzeSeasonality)
		seasonal.GET("/expected/:service/:metric", h.GetExpectedValue)
		seasonal.POST("/compare", h.CompareToBaseline)
		seasonal.POST("/holidays", h.AddHoliday)
		seasonal.GET("/holidays", h.ListHolidays)
	}
}

// SeasonalDetectRequest is the request for seasonal anomaly detection.
type SeasonalDetectRequest struct {
	Service        string  `json:"service" binding:"required"`
	Metric         string  `json:"metric" binding:"required"`
	TimeRange      string  `json:"time_range"`
	BaselineWindow string  `json:"baseline_window"`
	Sensitivity    float64 `json:"sensitivity"`
}

// SeasonalAnomaly represents a detected seasonal anomaly.
type SeasonalAnomaly struct {
	Timestamp          time.Time         `json:"timestamp"`
	Service            string            `json:"service"`
	Metric             string            `json:"metric"`
	Value              float64           `json:"value"`
	ExpectedValue      float64           `json:"expected_value"`
	ExpectedStd        float64           `json:"expected_std"`
	DeviationSigma     float64           `json:"deviation_sigma"`
	Score              float64           `json:"score"`
	HourOfDay          int               `json:"hour_of_day"`
	DayOfWeek          int               `json:"day_of_week"`
	SeasonalContext    map[string]any    `json:"seasonal_context"`
	ContributingFactors []string         `json:"contributing_factors"`
	IsHoliday          bool              `json:"is_holiday"`
	HolidayName        string            `json:"holiday_name,omitempty"`
	Description        string            `json:"description"`
}

// SeasonalDetectResponse is the response for seasonal anomaly detection.
type SeasonalDetectResponse struct {
	Anomalies        []SeasonalAnomaly `json:"anomalies"`
	TotalCount       int               `json:"total_count"`
	SeasonalPatterns map[string]any    `json:"seasonal_patterns"`
	BaselineInfo     map[string]any    `json:"baseline_info"`
	Summary          string            `json:"summary"`
}

// SeasonalBaseline represents a seasonal baseline for a metric.
type SeasonalBaseline struct {
	Service       string    `json:"service"`
	Metric        string    `json:"metric"`
	HourlyMeans   []float64 `json:"hourly_means"`
	HourlyStds    []float64 `json:"hourly_stds"`
	DailyMeans    []float64 `json:"daily_means"`
	DailyStds     []float64 `json:"daily_stds"`
	WeeklyPattern []float64 `json:"weekly_pattern,omitempty"`
	WeeklyStds    []float64 `json:"weekly_stds,omitempty"`
	GlobalMean    float64   `json:"global_mean"`
	GlobalStd     float64   `json:"global_std"`
	SampleCount   int64     `json:"sample_count"`
	LastUpdated   time.Time `json:"last_updated"`
}

// SeasonalityAnalysis represents the analysis of seasonal patterns.
type SeasonalityAnalysis struct {
	Service         string              `json:"service"`
	Metric          string              `json:"metric"`
	HasHourly       bool                `json:"has_hourly"`
	HasDaily        bool                `json:"has_daily"`
	HasWeekly       bool                `json:"has_weekly"`
	HourlyStrength  float64             `json:"hourly_strength"`
	DailyStrength   float64             `json:"daily_strength"`
	WeeklyStrength  float64             `json:"weekly_strength"`
	DominantPeriod  string              `json:"dominant_period"`
	DetectedPeriods []map[string]any    `json:"detected_periods"`
}

// DetectSeasonalAnomalies detects anomalies using seasonal awareness.
func (h *SeasonalAnomalyHandler) DetectSeasonalAnomalies(c *gin.Context) {
	var req SeasonalDetectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if req.TimeRange == "" {
		req.TimeRange = "1h"
	}
	if req.BaselineWindow == "" {
		req.BaselineWindow = "7d"
	}
	if req.Sensitivity == 0 {
		req.Sensitivity = 0.8
	}

	ctx := c.Request.Context()

	// Get seasonal baseline
	baseline, err := h.getSeasonalBaseline(ctx, req.Service, req.Metric, req.BaselineWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if we have enough data
	if baseline.SampleCount < 168 {
		c.JSON(http.StatusOK, SeasonalDetectResponse{
			Anomalies:        []SeasonalAnomaly{},
			TotalCount:       0,
			SeasonalPatterns: map[string]any{"insufficient_data": true},
			BaselineInfo:     map[string]any{"sample_count": baseline.SampleCount},
			Summary:          "Insufficient data for seasonal analysis (need at least 1 week of hourly data)",
		})
		return
	}

	// Get current values
	values, timestamps, err := h.getMetricSeries(ctx, req.Service, req.Metric, req.TimeRange)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Detect anomalies
	threshold := 3.0 * (2 - req.Sensitivity)
	anomalies := []SeasonalAnomaly{}

	for i, value := range values {
		ts := timestamps[i]
		hour := ts.Hour()
		day := int(ts.Weekday())
		if day == 0 {
			day = 6 // Sunday
		} else {
			day-- // Adjust to Monday=0
		}

		// Calculate expected value
		hourlyExpected := baseline.HourlyMeans[hour]
		hourlyStd := baseline.HourlyStds[hour]
		dailyExpected := baseline.DailyMeans[day]
		dailyStd := baseline.DailyStds[day]

		// Weighted combination
		expected := 0.5*hourlyExpected + 0.3*dailyExpected + 0.2*baseline.GlobalMean
		expectedVar := 0.5*math.Pow(hourlyStd, 2) + 0.3*math.Pow(dailyStd, 2) + 0.2*math.Pow(baseline.GlobalStd, 2)
		expectedStd := math.Sqrt(expectedVar)
		if expectedStd < baseline.GlobalStd*0.1 {
			expectedStd = baseline.GlobalStd * 0.1
		}

		// Calculate deviation
		deviation := math.Abs(value - expected)
		deviationSigma := deviation / expectedStd
		score := math.Min(deviationSigma/threshold, 1.0)

		// Check if anomalous
		if deviationSigma > threshold {
			factors := []string{}
			hourlyDeviation := math.Abs(value-hourlyExpected) / math.Max(hourlyStd, 0.0001)
			dailyDeviation := math.Abs(value-dailyExpected) / math.Max(dailyStd, 0.0001)

			if hourlyDeviation > threshold {
				factors = append(factors, fmt.Sprintf("Unusual for hour %d:00 (expected ~%.2f)", hour, hourlyExpected))
			}
			if dailyDeviation > threshold {
				dayNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
				factors = append(factors, fmt.Sprintf("Unusual for %s (expected ~%.2f)", dayNames[day], dailyExpected))
			}

			// Check for holiday
			isHoliday, holidayName := h.isHoliday(ctx, ts)

			anomaly := SeasonalAnomaly{
				Timestamp:      ts,
				Service:        req.Service,
				Metric:         req.Metric,
				Value:          value,
				ExpectedValue:  expected,
				ExpectedStd:    expectedStd,
				DeviationSigma: deviationSigma,
				Score:          score,
				HourOfDay:      hour,
				DayOfWeek:      day,
				SeasonalContext: map[string]any{
					"hour":            hour,
					"day_of_week":     day,
					"hourly_expected": hourlyExpected,
					"daily_expected":  dailyExpected,
				},
				ContributingFactors: factors,
				IsHoliday:           isHoliday,
				HolidayName:         holidayName,
				Description: fmt.Sprintf("Value %.2f is %.1fσ from expected %.2f at %s %02d:00",
					value, deviationSigma, expected,
					[]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}[day], hour),
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Sort by score
	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].Score > anomalies[j].Score
	})

	// Analyze seasonality
	seasonalityInfo := h.analyzeSeasonality(values)

	// Build response
	response := SeasonalDetectResponse{
		Anomalies:  anomalies,
		TotalCount: len(anomalies),
		SeasonalPatterns: map[string]any{
			"has_hourly":       seasonalityInfo.HasHourly,
			"has_daily":        seasonalityInfo.HasDaily,
			"has_weekly":       seasonalityInfo.HasWeekly,
			"hourly_strength":  seasonalityInfo.HourlyStrength,
			"daily_strength":   seasonalityInfo.DailyStrength,
			"weekly_strength":  seasonalityInfo.WeeklyStrength,
			"dominant_period":  seasonalityInfo.DominantPeriod,
		},
		BaselineInfo: map[string]any{
			"sample_count": baseline.SampleCount,
			"global_mean":  baseline.GlobalMean,
			"global_std":   baseline.GlobalStd,
			"last_updated": baseline.LastUpdated,
		},
		Summary: h.generateSummary(anomalies, seasonalityInfo),
	}

	c.JSON(http.StatusOK, response)
}

// GetSeasonalBaseline returns the seasonal baseline for a metric.
func (h *SeasonalAnomalyHandler) GetSeasonalBaseline(c *gin.Context) {
	service := c.Param("service")
	metric := c.Param("metric")
	window := c.DefaultQuery("window", "7d")

	ctx := c.Request.Context()
	baseline, err := h.getSeasonalBaseline(ctx, service, metric, window)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, baseline)
}

// AnalyzeSeasonality analyzes what seasonal patterns exist in a metric.
func (h *SeasonalAnomalyHandler) AnalyzeSeasonality(c *gin.Context) {
	service := c.Param("service")
	metric := c.Param("metric")
	timeRange := c.DefaultQuery("time_range", "7d")

	ctx := c.Request.Context()
	values, _, err := h.getMetricSeries(ctx, service, metric, timeRange)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	analysis := h.analyzeSeasonality(values)
	analysis.Service = service
	analysis.Metric = metric

	c.JSON(http.StatusOK, analysis)
}

// GetExpectedValue returns the expected value for a metric at a specific time.
func (h *SeasonalAnomalyHandler) GetExpectedValue(c *gin.Context) {
	service := c.Param("service")
	metric := c.Param("metric")

	now := time.Now().UTC()
	hour := now.Hour()
	day := int(now.Weekday())
	if day == 0 {
		day = 6
	} else {
		day--
	}

	ctx := c.Request.Context()
	baseline, err := h.getSeasonalBaseline(ctx, service, metric, "7d")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hourlyExpected := baseline.HourlyMeans[hour]
	hourlyStd := baseline.HourlyStds[hour]
	dailyExpected := baseline.DailyMeans[day]
	dailyStd := baseline.DailyStds[day]

	expected := 0.6*hourlyExpected + 0.4*dailyExpected
	expectedStd := math.Sqrt(0.6*math.Pow(hourlyStd, 2) + 0.4*math.Pow(dailyStd, 2))

	dayNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

	c.JSON(http.StatusOK, gin.H{
		"service":          service,
		"metric":           metric,
		"hour":             hour,
		"day_of_week":      day,
		"day_name":         dayNames[day],
		"expected_value":   expected,
		"expected_std":     expectedStd,
		"confidence_interval": gin.H{
			"lower": expected - 2*expectedStd,
			"upper": expected + 2*expectedStd,
		},
		"hourly_expected": hourlyExpected,
		"daily_expected":  dailyExpected,
	})
}

// CompareRequest is the request for comparing a value to baseline.
type CompareRequest struct {
	Service   string    `json:"service" binding:"required"`
	Metric    string    `json:"metric" binding:"required"`
	Value     float64   `json:"value" binding:"required"`
	Timestamp time.Time `json:"timestamp"`
}

// CompareToBaseline compares a single value to its seasonal baseline.
func (h *SeasonalAnomalyHandler) CompareToBaseline(c *gin.Context) {
	var req CompareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ts := req.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	ctx := c.Request.Context()
	baseline, err := h.getSeasonalBaseline(ctx, req.Service, req.Metric, "7d")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hour := ts.Hour()
	day := int(ts.Weekday())
	if day == 0 {
		day = 6
	} else {
		day--
	}

	hourlyExpected := baseline.HourlyMeans[hour]
	hourlyStd := baseline.HourlyStds[hour]
	dailyExpected := baseline.DailyMeans[day]
	dailyStd := baseline.DailyStds[day]

	expected := 0.5*hourlyExpected + 0.3*dailyExpected + 0.2*baseline.GlobalMean
	expectedVar := 0.5*math.Pow(hourlyStd, 2) + 0.3*math.Pow(dailyStd, 2) + 0.2*math.Pow(baseline.GlobalStd, 2)
	expectedStd := math.Sqrt(expectedVar)
	if expectedStd < 0.0001 {
		expectedStd = baseline.GlobalStd
	}

	deviation := math.Abs(req.Value - expected)
	deviationSigma := deviation / expectedStd
	isAnomaly := deviationSigma > 3.0
	score := math.Min(deviationSigma/3.0, 1.0)

	c.JSON(http.StatusOK, gin.H{
		"service":          req.Service,
		"metric":           req.Metric,
		"value":            req.Value,
		"expected_value":   expected,
		"expected_std":     expectedStd,
		"deviation_sigma":  deviationSigma,
		"is_anomaly":       isAnomaly,
		"score":            score,
		"seasonal_context": gin.H{
			"hour":            hour,
			"day_of_week":     day,
			"hourly_expected": hourlyExpected,
			"daily_expected":  dailyExpected,
		},
	})
}

// HolidayRequest is the request for adding a holiday.
type HolidayRequest struct {
	Name      string    `json:"name" binding:"required"`
	StartDate time.Time `json:"start" binding:"required"`
	EndDate   time.Time `json:"end" binding:"required"`
	Service   string    `json:"service"`
	Type      string    `json:"type"`
}

// AddHoliday adds a holiday or special event to the calendar.
func (h *SeasonalAnomalyHandler) AddHoliday(c *gin.Context) {
	var req HolidayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" {
		req.Type = "custom"
	}

	ctx := c.Request.Context()
	query := `
		INSERT INTO holiday_calendar (Name, StartDate, EndDate, ServiceName, HolidayType)
		VALUES (?, ?, ?, ?, ?)
	`

	if err := h.clickhouse.Exec(ctx, query, req.Name, req.StartDate, req.EndDate, req.Service, req.Type); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Added holiday: %s", req.Name),
		"start":   req.StartDate,
		"end":     req.EndDate,
	})
}

// ListHolidays lists all holidays in the calendar.
func (h *SeasonalAnomalyHandler) ListHolidays(c *gin.Context) {
	ctx := c.Request.Context()
	query := `
		SELECT Name, StartDate, EndDate, ServiceName, HolidayType, ThresholdMultiplier
		FROM holiday_calendar
		WHERE EndDate >= now() - INTERVAL 30 DAY
		ORDER BY StartDate
	`

	rows, err := h.clickhouse.Query(ctx, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	holidays := []map[string]any{}
	for rows.Next() {
		var name, service, holidayType string
		var startDate, endDate time.Time
		var multiplier float64

		if err := rows.Scan(&name, &startDate, &endDate, &service, &holidayType, &multiplier); err != nil {
			continue
		}

		holidays = append(holidays, map[string]any{
			"name":                 name,
			"start":                startDate,
			"end":                  endDate,
			"service":              service,
			"type":                 holidayType,
			"threshold_multiplier": multiplier,
		})
	}

	c.JSON(http.StatusOK, gin.H{"holidays": holidays, "count": len(holidays)})
}

// Helper methods

func (h *SeasonalAnomalyHandler) getSeasonalBaseline(ctx context.Context, service, metric, window string) (*SeasonalBaseline, error) {
	// Query for hourly patterns
	hourlyQuery := `
		SELECT
			toHour(Timestamp) as Hour,
			avg(Value) as Mean,
			stddevPop(Value) as Std
		FROM otel_metrics
		WHERE ServiceName = ?
		  AND MetricName = ?
		  AND Timestamp >= now() - INTERVAL ?
		GROUP BY Hour
		ORDER BY Hour
	`

	rows, err := h.clickhouse.Query(ctx, hourlyQuery, service, metric, window)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hourlyMeans := make([]float64, 24)
	hourlyStds := make([]float64, 24)
	for rows.Next() {
		var hour uint8
		var mean, std float64
		if err := rows.Scan(&hour, &mean, &std); err != nil {
			continue
		}
		hourlyMeans[hour] = mean
		hourlyStds[hour] = std
	}

	// Query for daily patterns
	dailyQuery := `
		SELECT
			toDayOfWeek(Timestamp) as Day,
			avg(Value) as Mean,
			stddevPop(Value) as Std
		FROM otel_metrics
		WHERE ServiceName = ?
		  AND MetricName = ?
		  AND Timestamp >= now() - INTERVAL ?
		GROUP BY Day
		ORDER BY Day
	`

	rows2, err := h.clickhouse.Query(ctx, dailyQuery, service, metric, window)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	dailyMeans := make([]float64, 7)
	dailyStds := make([]float64, 7)
	for rows2.Next() {
		var day uint8
		var mean, std float64
		if err := rows2.Scan(&day, &mean, &std); err != nil {
			continue
		}
		// ClickHouse uses 1=Monday, 7=Sunday
		idx := int(day - 1)
		if idx >= 0 && idx < 7 {
			dailyMeans[idx] = mean
			dailyStds[idx] = std
		}
	}

	// Query for global statistics
	globalQuery := `
		SELECT
			avg(Value) as Mean,
			stddevPop(Value) as Std,
			count() as Count
		FROM otel_metrics
		WHERE ServiceName = ?
		  AND MetricName = ?
		  AND Timestamp >= now() - INTERVAL ?
	`

	var globalMean, globalStd float64
	var sampleCount int64
	if err := h.clickhouse.QueryRow(ctx, globalQuery, service, metric, window).Scan(&globalMean, &globalStd, &sampleCount); err != nil {
		return nil, err
	}

	return &SeasonalBaseline{
		Service:     service,
		Metric:      metric,
		HourlyMeans: hourlyMeans,
		HourlyStds:  hourlyStds,
		DailyMeans:  dailyMeans,
		DailyStds:   dailyStds,
		GlobalMean:  globalMean,
		GlobalStd:   globalStd,
		SampleCount: sampleCount,
		LastUpdated: time.Now().UTC(),
	}, nil
}

func (h *SeasonalAnomalyHandler) getMetricSeries(ctx context.Context, service, metric, timeRange string) ([]float64, []time.Time, error) {
	query := `
		SELECT
			toStartOfHour(Timestamp) as Hour,
			avg(Value) as Value
		FROM otel_metrics
		WHERE ServiceName = ?
		  AND MetricName = ?
		  AND Timestamp >= now() - INTERVAL ?
		GROUP BY Hour
		ORDER BY Hour
	`

	rows, err := h.clickhouse.Query(ctx, query, service, metric, timeRange)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var values []float64
	var timestamps []time.Time
	for rows.Next() {
		var ts time.Time
		var value float64
		if err := rows.Scan(&ts, &value); err != nil {
			continue
		}
		values = append(values, value)
		timestamps = append(timestamps, ts)
	}

	return values, timestamps, nil
}

func (h *SeasonalAnomalyHandler) isHoliday(ctx context.Context, ts time.Time) (bool, string) {
	query := `
		SELECT Name
		FROM holiday_calendar
		WHERE StartDate <= ? AND EndDate >= ?
		LIMIT 1
	`

	var name string
	if err := h.clickhouse.QueryRow(ctx, query, ts, ts).Scan(&name); err != nil {
		return false, ""
	}
	return name != "", name
}

func (h *SeasonalAnomalyHandler) analyzeSeasonality(values []float64) *SeasonalityAnalysis {
	if len(values) < 48 {
		return &SeasonalityAnalysis{
			DominantPeriod: "none",
		}
	}

	// Calculate autocorrelation at different lags
	n := len(values)
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(n)

	if variance < 0.0001 {
		return &SeasonalityAnalysis{
			DominantPeriod: "none",
		}
	}

	// Check for daily pattern (lag 24)
	dailyCorr := h.autocorrelation(values, 24, mean, variance)
	// Check for 12-hour pattern
	hourlyCorr := h.autocorrelation(values, 12, mean, variance)
	// Check for weekly pattern (lag 168)
	weeklyCorr := 0.0
	if n >= 336 {
		weeklyCorr = h.autocorrelation(values, 168, mean, variance)
	}

	hasHourly := hourlyCorr > 0.3
	hasDaily := dailyCorr > 0.3
	hasWeekly := weeklyCorr > 0.3

	dominantPeriod := "none"
	maxStrength := 0.0
	if dailyCorr > maxStrength {
		maxStrength = dailyCorr
		dominantPeriod = "daily"
	}
	if weeklyCorr > maxStrength {
		maxStrength = weeklyCorr
		dominantPeriod = "weekly"
	}
	if hourlyCorr > maxStrength {
		dominantPeriod = "12-hour"
	}

	periods := []map[string]any{}
	if dailyCorr > 0.1 {
		periods = append(periods, map[string]any{"period": 24, "strength": dailyCorr})
	}
	if weeklyCorr > 0.1 {
		periods = append(periods, map[string]any{"period": 168, "strength": weeklyCorr})
	}
	if hourlyCorr > 0.1 {
		periods = append(periods, map[string]any{"period": 12, "strength": hourlyCorr})
	}

	return &SeasonalityAnalysis{
		HasHourly:       hasHourly,
		HasDaily:        hasDaily,
		HasWeekly:       hasWeekly,
		HourlyStrength:  hourlyCorr,
		DailyStrength:   dailyCorr,
		WeeklyStrength:  weeklyCorr,
		DominantPeriod:  dominantPeriod,
		DetectedPeriods: periods,
	}
}

func (h *SeasonalAnomalyHandler) autocorrelation(values []float64, lag int, mean, variance float64) float64 {
	if lag >= len(values) {
		return 0
	}

	n := len(values)
	cov := 0.0
	for i := lag; i < n; i++ {
		cov += (values[i] - mean) * (values[i-lag] - mean)
	}
	cov /= float64(n - lag)

	if variance < 0.0001 {
		return 0
	}
	return cov / variance
}

func (h *SeasonalAnomalyHandler) generateSummary(anomalies []SeasonalAnomaly, seasonality *SeasonalityAnalysis) string {
	var summary string

	if len(anomalies) > 0 {
		summary = fmt.Sprintf("Detected %d seasonal anomalies. ", len(anomalies))
	} else {
		summary = "No seasonal anomalies detected. "
	}

	patterns := []string{}
	if seasonality.HasDaily {
		patterns = append(patterns, fmt.Sprintf("daily (strength: %.2f)", seasonality.DailyStrength))
	}
	if seasonality.HasWeekly {
		patterns = append(patterns, fmt.Sprintf("weekly (strength: %.2f)", seasonality.WeeklyStrength))
	}

	if len(patterns) > 0 {
		summary += fmt.Sprintf("Seasonal patterns: %s.", joinStrings(patterns, ", "))
	} else {
		summary += "No significant seasonal patterns detected."
	}

	return summary
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
