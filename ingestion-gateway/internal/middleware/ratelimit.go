package middleware

import (
	"sync"
	"time"

	"github.com/ollystack/ingestion-gateway/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

var (
	rateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"tenant"},
	)
	rateLimitAllowed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_rate_limit_allowed_total",
			Help: "Total number of requests allowed through rate limit",
		},
		[]string{"tenant"},
	)
)

// RateLimiter provides per-tenant rate limiting
type RateLimiter struct {
	config   config.RateLimitConfig
	limiters sync.Map // map[tenantID]*tenantLimiter
	logger   *zap.Logger
}

type tenantLimiter struct {
	limiter   *rate.Limiter
	lastUsed  time.Time
	mu        sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg config.RateLimitConfig, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		config: cfg,
		logger: logger,
	}

	// Start cleanup goroutine to remove stale limiters
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request from the given tenant should be allowed
func (rl *RateLimiter) Allow(tenantID string) bool {
	if !rl.config.Enabled {
		return true
	}

	limiter := rl.getOrCreateLimiter(tenantID)

	limiter.mu.Lock()
	limiter.lastUsed = time.Now()
	limiter.mu.Unlock()

	allowed := limiter.limiter.Allow()

	if allowed {
		rateLimitAllowed.WithLabelValues(tenantID).Inc()
	} else {
		rateLimitHits.WithLabelValues(tenantID).Inc()
		rl.logger.Warn("Rate limit exceeded",
			zap.String("tenant", tenantID),
		)
	}

	return allowed
}

// AllowN checks if N events from the given tenant should be allowed
func (rl *RateLimiter) AllowN(tenantID string, n int) bool {
	if !rl.config.Enabled {
		return true
	}

	limiter := rl.getOrCreateLimiter(tenantID)

	limiter.mu.Lock()
	limiter.lastUsed = time.Now()
	limiter.mu.Unlock()

	allowed := limiter.limiter.AllowN(time.Now(), n)

	if allowed {
		rateLimitAllowed.WithLabelValues(tenantID).Add(float64(n))
	} else {
		rateLimitHits.WithLabelValues(tenantID).Add(float64(n))
	}

	return allowed
}

// SetTenantLimit sets a custom rate limit for a specific tenant
func (rl *RateLimiter) SetTenantLimit(tenantID string, rps float64, burst int) {
	limiter := rl.getOrCreateLimiter(tenantID)
	limiter.limiter.SetLimit(rate.Limit(rps))
	limiter.limiter.SetBurst(burst)

	rl.logger.Info("Updated tenant rate limit",
		zap.String("tenant", tenantID),
		zap.Float64("rps", rps),
		zap.Int("burst", burst),
	)
}

func (rl *RateLimiter) getOrCreateLimiter(tenantID string) *tenantLimiter {
	if existing, ok := rl.limiters.Load(tenantID); ok {
		return existing.(*tenantLimiter)
	}

	// Create new limiter with default settings
	newLimiter := &tenantLimiter{
		limiter:  rate.NewLimiter(rate.Limit(rl.config.DefaultRPS), rl.config.DefaultBurst),
		lastUsed: time.Now(),
	}

	// Try to store, but if another goroutine beat us, use theirs
	actual, loaded := rl.limiters.LoadOrStore(tenantID, newLimiter)
	if loaded {
		return actual.(*tenantLimiter)
	}

	rl.logger.Debug("Created rate limiter for tenant",
		zap.String("tenant", tenantID),
		zap.Float64("rps", rl.config.DefaultRPS),
		zap.Int("burst", rl.config.DefaultBurst),
	)

	return newLimiter
}

// cleanupLoop removes stale tenant limiters to prevent memory leaks
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *RateLimiter) cleanup() {
	staleThreshold := time.Now().Add(-30 * time.Minute)
	var staleKeys []string

	rl.limiters.Range(func(key, value interface{}) bool {
		limiter := value.(*tenantLimiter)
		limiter.mu.Lock()
		lastUsed := limiter.lastUsed
		limiter.mu.Unlock()

		if lastUsed.Before(staleThreshold) {
			staleKeys = append(staleKeys, key.(string))
		}
		return true
	})

	for _, key := range staleKeys {
		rl.limiters.Delete(key)
		rl.logger.Debug("Removed stale rate limiter",
			zap.String("tenant", key),
		)
	}

	if len(staleKeys) > 0 {
		rl.logger.Info("Cleaned up stale rate limiters",
			zap.Int("count", len(staleKeys)),
		)
	}
}

// Stats returns current rate limiter statistics
func (rl *RateLimiter) Stats() map[string]interface{} {
	var count int
	rl.limiters.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]interface{}{
		"enabled":      rl.config.Enabled,
		"default_rps":  rl.config.DefaultRPS,
		"default_burst": rl.config.DefaultBurst,
		"active_tenants": count,
	}
}
