package sampling

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	samplingDecisions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_sampling_decisions_total",
			Help: "Sampling decisions by reason",
		},
		[]string{"tenant", "data_type", "decision", "reason"},
	)
	samplingRateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ollystack_sampling_rate",
			Help: "Current sampling rate per tenant",
		},
		[]string{"tenant", "data_type"},
	)
	dataReduction = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_data_reduction_bytes_total",
			Help: "Bytes saved through sampling",
		},
		[]string{"tenant"},
	)
)

// Decision represents a sampling decision
type Decision int

const (
	Keep Decision = iota
	Sample
	Drop
	Aggregate
)

// Reason explains why a sampling decision was made
type Reason string

const (
	ReasonError       Reason = "error"
	ReasonSlow        Reason = "slow"
	ReasonAnomaly     Reason = "anomaly"
	ReasonFirstSeen   Reason = "first_seen"
	ReasonImportant   Reason = "important"
	ReasonSampled     Reason = "sampled"
	ReasonQuotaExceed Reason = "quota_exceeded"
	ReasonHighVolume  Reason = "high_volume"
	ReasonDuplicate   Reason = "duplicate"
)

// Config holds sampling configuration
type Config struct {
	// Base sampling rates
	DefaultSampleRate     float64 `mapstructure:"default_sample_rate"`     // 0.1 = 10%
	ErrorSampleRate       float64 `mapstructure:"error_sample_rate"`       // 1.0 = 100%
	SlowRequestSampleRate float64 `mapstructure:"slow_request_sample_rate"` // 1.0 = 100%

	// Thresholds
	SlowThresholdMs     int64   `mapstructure:"slow_threshold_ms"`     // 1000ms
	HighVolumeThreshold int64   `mapstructure:"high_volume_threshold"` // events/sec
	AnomalyZScoreThreshold float64 `mapstructure:"anomaly_zscore_threshold"` // 3.0

	// Adaptive sampling
	AdaptiveEnabled     bool    `mapstructure:"adaptive_enabled"`
	TargetEventsPerSec  int64   `mapstructure:"target_events_per_sec"`
	MinSampleRate       float64 `mapstructure:"min_sample_rate"` // 0.01 = 1%
	MaxSampleRate       float64 `mapstructure:"max_sample_rate"` // 1.0 = 100%
}

// DefaultConfig returns default sampling configuration
func DefaultConfig() Config {
	return Config{
		DefaultSampleRate:     0.1,  // 10%
		ErrorSampleRate:       1.0,  // 100%
		SlowRequestSampleRate: 1.0,  // 100%
		SlowThresholdMs:       1000, // 1 second
		HighVolumeThreshold:   10000,
		AnomalyZScoreThreshold: 3.0,
		AdaptiveEnabled:       true,
		TargetEventsPerSec:    1000,
		MinSampleRate:         0.01, // 1%
		MaxSampleRate:         1.0,  // 100%
	}
}

// IntelligentSampler makes smart sampling decisions
type IntelligentSampler struct {
	config Config
	logger *zap.Logger

	// Per-tenant state
	tenantState sync.Map // map[tenantID]*tenantSamplingState

	// Pattern tracking for first-seen detection
	seenPatterns sync.Map // map[patternHash]bool

	// Metrics baselines for anomaly detection
	baselines sync.Map // map[metricKey]*baseline
}

type tenantSamplingState struct {
	mu              sync.RWMutex
	currentRate     float64
	eventsThisSecond atomic.Int64
	bytesThisSecond  atomic.Int64
	lastAdjustment   time.Time

	// Rolling stats for adaptive sampling
	eventCounts []int64 // Last 60 seconds
	currentIdx  int
}

type baseline struct {
	mu      sync.RWMutex
	mean    float64
	stddev  float64
	count   int64
	lastUpdate time.Time
}

// DataPoint represents telemetry data to be sampled
type DataPoint struct {
	TenantID    string
	Type        string // "metric", "log", "trace"
	Timestamp   time.Time
	TraceID     string
	ServiceName string
	Name        string // metric name, span name, etc.

	// For sampling decisions
	IsError      bool
	DurationMs   int64
	Value        float64
	SeverityNum  int
	PatternHash  string
	Labels       map[string]string

	// Size for quota tracking
	SizeBytes int
}

// SamplingResult contains the sampling decision and metadata
type SamplingResult struct {
	Decision   Decision
	Reason     Reason
	SampleRate float64
	Metadata   map[string]interface{}
}

// NewIntelligentSampler creates a new intelligent sampler
func NewIntelligentSampler(config Config, logger *zap.Logger) *IntelligentSampler {
	s := &IntelligentSampler{
		config: config,
		logger: logger,
	}

	// Start background goroutine for adaptive rate adjustment
	if config.AdaptiveEnabled {
		go s.adaptiveRateLoop()
	}

	return s
}

// ShouldSample makes an intelligent sampling decision
func (s *IntelligentSampler) ShouldSample(ctx context.Context, dp *DataPoint) SamplingResult {
	// Get or create tenant state
	state := s.getOrCreateTenantState(dp.TenantID)

	// Update counters
	state.eventsThisSecond.Add(1)
	state.bytesThisSecond.Add(int64(dp.SizeBytes))

	// Decision priority (highest to lowest):
	// 1. Always keep errors
	// 2. Always keep slow requests
	// 3. Always keep anomalies
	// 4. Always keep first-seen patterns
	// 5. Sample based on adaptive rate
	// 6. Drop if over quota

	// 1. Always keep errors
	if dp.IsError {
		s.recordDecision(dp.TenantID, dp.Type, Keep, ReasonError)
		return SamplingResult{
			Decision:   Keep,
			Reason:     ReasonError,
			SampleRate: 1.0,
		}
	}

	// 2. Always keep slow requests
	if dp.DurationMs > 0 && dp.DurationMs > s.config.SlowThresholdMs {
		s.recordDecision(dp.TenantID, dp.Type, Keep, ReasonSlow)
		return SamplingResult{
			Decision:   Keep,
			Reason:     ReasonSlow,
			SampleRate: 1.0,
			Metadata: map[string]interface{}{
				"duration_ms": dp.DurationMs,
				"threshold":   s.config.SlowThresholdMs,
			},
		}
	}

	// 3. Check for anomalies (metrics)
	if dp.Type == "metric" && dp.Value != 0 {
		if s.isAnomaly(dp) {
			s.recordDecision(dp.TenantID, dp.Type, Keep, ReasonAnomaly)
			return SamplingResult{
				Decision:   Keep,
				Reason:     ReasonAnomaly,
				SampleRate: 1.0,
			}
		}
	}

	// 4. Keep first-seen patterns (logs)
	if dp.PatternHash != "" {
		if s.isFirstSeen(dp.PatternHash) {
			s.recordDecision(dp.TenantID, dp.Type, Keep, ReasonFirstSeen)
			return SamplingResult{
				Decision:   Keep,
				Reason:     ReasonFirstSeen,
				SampleRate: 1.0,
			}
		}
	}

	// 5. Probabilistic sampling based on adaptive rate
	state.mu.RLock()
	currentRate := state.currentRate
	state.mu.RUnlock()

	if currentRate <= 0 {
		currentRate = s.config.DefaultSampleRate
	}

	if s.shouldSampleProbabilistic(dp.TraceID, currentRate) {
		s.recordDecision(dp.TenantID, dp.Type, Sample, ReasonSampled)
		return SamplingResult{
			Decision:   Sample,
			Reason:     ReasonSampled,
			SampleRate: currentRate,
		}
	}

	// Drop this data point
	s.recordDecision(dp.TenantID, dp.Type, Drop, ReasonSampled)
	dataReduction.WithLabelValues(dp.TenantID).Add(float64(dp.SizeBytes))

	return SamplingResult{
		Decision:   Drop,
		Reason:     ReasonSampled,
		SampleRate: currentRate,
	}
}

// isAnomaly checks if a metric value is anomalous using Z-score
func (s *IntelligentSampler) isAnomaly(dp *DataPoint) bool {
	key := dp.TenantID + ":" + dp.ServiceName + ":" + dp.Name

	// Get or create baseline
	baselineI, _ := s.baselines.LoadOrStore(key, &baseline{
		mean:       dp.Value,
		count:      1,
		lastUpdate: time.Now(),
	})
	b := baselineI.(*baseline)

	b.mu.Lock()
	defer b.mu.Unlock()

	// Update baseline with exponential moving average
	alpha := 0.1 // Smoothing factor
	b.count++

	oldMean := b.mean
	b.mean = b.mean + alpha*(dp.Value-b.mean)

	// Update stddev using Welford's algorithm
	if b.count > 1 {
		b.stddev = math.Sqrt(alpha*(dp.Value-oldMean)*(dp.Value-b.mean) + (1-alpha)*b.stddev*b.stddev)
	}

	b.lastUpdate = time.Now()

	// Calculate Z-score
	if b.stddev > 0 && b.count > 10 {
		zScore := math.Abs(dp.Value-b.mean) / b.stddev
		return zScore > s.config.AnomalyZScoreThreshold
	}

	return false
}

// isFirstSeen checks if a pattern hash has been seen before
func (s *IntelligentSampler) isFirstSeen(patternHash string) bool {
	_, existed := s.seenPatterns.LoadOrStore(patternHash, true)
	return !existed
}

// shouldSampleProbabilistic uses consistent hashing for trace-aware sampling
func (s *IntelligentSampler) shouldSampleProbabilistic(traceID string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0 {
		return false
	}

	// Use trace ID for consistent sampling (same trace = same decision)
	if traceID != "" {
		h := fnv.New64a()
		h.Write([]byte(traceID))
		hashValue := float64(h.Sum64()) / float64(^uint64(0))
		return hashValue < rate
	}

	// Random sampling for non-trace data
	h := fnv.New64a()
	h.Write([]byte(time.Now().String()))
	hashValue := float64(h.Sum64()) / float64(^uint64(0))
	return hashValue < rate
}

// adaptiveRateLoop adjusts sampling rates based on volume
func (s *IntelligentSampler) adaptiveRateLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.tenantState.Range(func(key, value interface{}) bool {
			tenantID := key.(string)
			state := value.(*tenantSamplingState)

			// Get current events/second
			eventsPerSec := state.eventsThisSecond.Swap(0)
			state.bytesThisSecond.Swap(0)

			// Update rolling stats
			state.mu.Lock()
			state.eventCounts[state.currentIdx] = eventsPerSec
			state.currentIdx = (state.currentIdx + 1) % len(state.eventCounts)

			// Calculate average over last minute
			var total int64
			for _, count := range state.eventCounts {
				total += count
			}
			avgEventsPerSec := total / int64(len(state.eventCounts))

			// Adjust rate if needed
			if avgEventsPerSec > s.config.TargetEventsPerSec {
				// Too much traffic, reduce sampling rate
				reduction := float64(s.config.TargetEventsPerSec) / float64(avgEventsPerSec)
				newRate := state.currentRate * reduction

				if newRate < s.config.MinSampleRate {
					newRate = s.config.MinSampleRate
				}

				if newRate != state.currentRate {
					state.currentRate = newRate
					s.logger.Info("Adjusted sampling rate (reducing)",
						zap.String("tenant", tenantID),
						zap.Float64("new_rate", newRate),
						zap.Int64("events_per_sec", avgEventsPerSec),
					)
				}
			} else if avgEventsPerSec < s.config.TargetEventsPerSec/2 {
				// Traffic is low, increase sampling rate
				newRate := state.currentRate * 1.1

				if newRate > s.config.MaxSampleRate {
					newRate = s.config.MaxSampleRate
				}

				if newRate != state.currentRate {
					state.currentRate = newRate
				}
			}

			state.lastAdjustment = time.Now()
			state.mu.Unlock()

			// Update metrics
			samplingRateGauge.WithLabelValues(tenantID, "adaptive").Set(state.currentRate)

			return true
		})
	}
}

func (s *IntelligentSampler) getOrCreateTenantState(tenantID string) *tenantSamplingState {
	stateI, _ := s.tenantState.LoadOrStore(tenantID, &tenantSamplingState{
		currentRate:  s.config.DefaultSampleRate,
		eventCounts:  make([]int64, 60), // 60 seconds of history
	})
	return stateI.(*tenantSamplingState)
}

func (s *IntelligentSampler) recordDecision(tenantID, dataType string, decision Decision, reason Reason) {
	decisionStr := "drop"
	switch decision {
	case Keep:
		decisionStr = "keep"
	case Sample:
		decisionStr = "sample"
	case Aggregate:
		decisionStr = "aggregate"
	}

	samplingDecisions.WithLabelValues(tenantID, dataType, decisionStr, string(reason)).Inc()
}

// GetTenantRate returns the current sampling rate for a tenant
func (s *IntelligentSampler) GetTenantRate(tenantID string) float64 {
	stateI, ok := s.tenantState.Load(tenantID)
	if !ok {
		return s.config.DefaultSampleRate
	}

	state := stateI.(*tenantSamplingState)
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.currentRate
}

// SetTenantRate manually sets the sampling rate for a tenant
func (s *IntelligentSampler) SetTenantRate(tenantID string, rate float64) {
	state := s.getOrCreateTenantState(tenantID)

	state.mu.Lock()
	state.currentRate = rate
	state.mu.Unlock()

	samplingRateGauge.WithLabelValues(tenantID, "manual").Set(rate)
}

// Stats returns sampling statistics
func (s *IntelligentSampler) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	var tenantCount int
	s.tenantState.Range(func(key, value interface{}) bool {
		tenantCount++
		return true
	})

	var patternCount int
	s.seenPatterns.Range(func(key, value interface{}) bool {
		patternCount++
		return true
	})

	stats["tenant_count"] = tenantCount
	stats["pattern_count"] = patternCount
	stats["config"] = s.config

	return stats
}
