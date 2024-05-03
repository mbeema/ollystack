package tenant

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	quotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ollystack_tenant_quota_usage_percent",
			Help: "Quota usage percentage per tenant",
		},
		[]string{"tenant", "quota_type"},
	)
	quotaExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_tenant_quota_exceeded_total",
			Help: "Number of times quota was exceeded",
		},
		[]string{"tenant", "quota_type"},
	)
	tenantEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_tenant_events_total",
			Help: "Total events per tenant",
		},
		[]string{"tenant", "data_type"},
	)
	tenantBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_tenant_bytes_total",
			Help: "Total bytes per tenant",
		},
		[]string{"tenant"},
	)
)

// Plan represents a subscription plan
type Plan string

const (
	PlanFree       Plan = "free"
	PlanStartup    Plan = "startup"
	PlanGrowth     Plan = "growth"
	PlanEnterprise Plan = "enterprise"
)

// Quota defines resource limits for a tenant
type Quota struct {
	// Rate limits
	MaxEventsPerSec   int64 // Events per second
	MaxBytesPerSec    int64 // Bytes per second

	// Daily limits
	MaxEventsPerDay   int64
	MaxBytesPerDay    int64

	// Monthly limits
	MaxEventsPerMonth int64
	MaxBytesPerMonth  int64

	// Storage limits
	MaxStorageBytes   int64
	RetentionDays     int

	// Feature limits
	MaxServices       int
	MaxDashboards     int
	MaxAlertRules     int
	MaxUsers          int

	// Sampling
	BaseSamplingRate  float64 // 0.1 = 10%
	ForceFullSampling bool    // Enterprise: 100% sampling

	// Features
	AllowedFeatures   []string // ["metrics", "logs", "traces", "rum", "profiling"]
}

// DefaultQuotas for each plan
var DefaultQuotas = map[Plan]Quota{
	PlanFree: {
		MaxEventsPerSec:   100,
		MaxBytesPerSec:    100 * 1024,        // 100KB/s
		MaxEventsPerDay:   1_000_000,          // 1M events
		MaxBytesPerDay:    1 * 1024 * 1024 * 1024, // 1GB
		MaxEventsPerMonth: 10_000_000,
		MaxBytesPerMonth:  10 * 1024 * 1024 * 1024,
		MaxStorageBytes:   5 * 1024 * 1024 * 1024, // 5GB
		RetentionDays:     1,
		MaxServices:       5,
		MaxDashboards:     3,
		MaxAlertRules:     5,
		MaxUsers:          2,
		BaseSamplingRate:  0.01, // 1%
		AllowedFeatures:   []string{"metrics", "logs"},
	},
	PlanStartup: {
		MaxEventsPerSec:   1000,
		MaxBytesPerSec:    1 * 1024 * 1024,    // 1MB/s
		MaxEventsPerDay:   100_000_000,         // 100M events
		MaxBytesPerDay:    10 * 1024 * 1024 * 1024, // 10GB
		MaxEventsPerMonth: 3_000_000_000,
		MaxBytesPerMonth:  300 * 1024 * 1024 * 1024,
		MaxStorageBytes:   100 * 1024 * 1024 * 1024, // 100GB
		RetentionDays:     7,
		MaxServices:       50,
		MaxDashboards:     20,
		MaxAlertRules:     50,
		MaxUsers:          10,
		BaseSamplingRate:  0.1, // 10%
		AllowedFeatures:   []string{"metrics", "logs", "traces"},
	},
	PlanGrowth: {
		MaxEventsPerSec:   10000,
		MaxBytesPerSec:    10 * 1024 * 1024,   // 10MB/s
		MaxEventsPerDay:   1_000_000_000,       // 1B events
		MaxBytesPerDay:    100 * 1024 * 1024 * 1024, // 100GB
		MaxEventsPerMonth: 30_000_000_000,
		MaxBytesPerMonth:  3 * 1024 * 1024 * 1024 * 1024, // 3TB
		MaxStorageBytes:   1 * 1024 * 1024 * 1024 * 1024, // 1TB
		RetentionDays:     30,
		MaxServices:       500,
		MaxDashboards:     100,
		MaxAlertRules:     500,
		MaxUsers:          50,
		BaseSamplingRate:  0.5, // 50%
		AllowedFeatures:   []string{"metrics", "logs", "traces", "rum"},
	},
	PlanEnterprise: {
		MaxEventsPerSec:   1000000,              // 1M/s
		MaxBytesPerSec:    100 * 1024 * 1024,    // 100MB/s
		MaxEventsPerDay:   -1,                    // Unlimited
		MaxBytesPerDay:    -1,                    // Unlimited
		MaxEventsPerMonth: -1,
		MaxBytesPerMonth:  -1,
		MaxStorageBytes:   -1,
		RetentionDays:     90,
		MaxServices:       -1, // Unlimited
		MaxDashboards:     -1,
		MaxAlertRules:     -1,
		MaxUsers:          -1,
		BaseSamplingRate:  1.0, // 100%
		ForceFullSampling: true,
		AllowedFeatures:   []string{"metrics", "logs", "traces", "rum", "profiling", "synthetic"},
	},
}

// Tenant represents a customer account
type Tenant struct {
	ID            string
	Name          string
	Plan          Plan
	Quota         Quota
	CustomQuota   *Quota // Override default quota
	CreatedAt     time.Time
	Status        string // active, suspended, deleted

	// Billing
	OverageRate   float64 // $/GB over quota
	AllowOverage  bool
}

// TenantState tracks real-time usage
type TenantState struct {
	mu sync.RWMutex

	// Real-time counters (reset every second)
	eventsThisSec atomic.Int64
	bytesThisSec  atomic.Int64

	// Daily counters (reset at midnight UTC)
	eventsToday atomic.Int64
	bytesToday  atomic.Int64
	dayStart    time.Time

	// Monthly counters
	eventsThisMonth atomic.Int64
	bytesThisMonth  atomic.Int64
	monthStart      time.Time

	// Warnings sent
	warningSent80  bool
	warningSent90  bool
	warningSent100 bool
}

// QuotaManager manages tenant quotas and enforces limits
type QuotaManager struct {
	logger *zap.Logger

	// Tenant configuration
	tenants sync.Map // map[tenantID]*Tenant

	// Real-time state
	state sync.Map // map[tenantID]*TenantState

	// Callbacks
	onQuotaExceeded func(tenantID string, quotaType string, usage, limit int64)
	onQuotaWarning  func(tenantID string, quotaType string, percentUsed float64)
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(logger *zap.Logger) *QuotaManager {
	qm := &QuotaManager{
		logger: logger,
	}

	// Start background cleanup
	go qm.cleanupLoop()
	go qm.resetLoop()

	return qm
}

// RegisterTenant registers a new tenant
func (qm *QuotaManager) RegisterTenant(tenant *Tenant) {
	qm.tenants.Store(tenant.ID, tenant)
	qm.state.Store(tenant.ID, &TenantState{
		dayStart:   startOfDay(time.Now()),
		monthStart: startOfMonth(time.Now()),
	})

	qm.logger.Info("Registered tenant",
		zap.String("tenant", tenant.ID),
		zap.String("plan", string(tenant.Plan)),
	)
}

// GetTenant retrieves tenant configuration
func (qm *QuotaManager) GetTenant(tenantID string) (*Tenant, bool) {
	t, ok := qm.tenants.Load(tenantID)
	if !ok {
		return nil, false
	}
	return t.(*Tenant), true
}

// CheckQuota checks if a request should be allowed
func (qm *QuotaManager) CheckQuota(ctx context.Context, tenantID string, eventCount int64, byteCount int64) (bool, string) {
	tenant, ok := qm.GetTenant(tenantID)
	if !ok {
		// Unknown tenant - use default free quota
		qm.RegisterTenant(&Tenant{
			ID:     tenantID,
			Name:   "Unknown",
			Plan:   PlanFree,
			Quota:  DefaultQuotas[PlanFree],
			Status: "active",
		})
		tenant, _ = qm.GetTenant(tenantID)
	}

	if tenant.Status != "active" {
		quotaExceeded.WithLabelValues(tenantID, "suspended").Inc()
		return false, "tenant suspended"
	}

	// Get effective quota
	quota := tenant.Quota
	if tenant.CustomQuota != nil {
		quota = *tenant.CustomQuota
	}

	// Get or create state
	stateI, _ := qm.state.LoadOrStore(tenantID, &TenantState{
		dayStart:   startOfDay(time.Now()),
		monthStart: startOfMonth(time.Now()),
	})
	state := stateI.(*TenantState)

	// Check rate limit (per second)
	currentEventsPerSec := state.eventsThisSec.Add(eventCount)
	currentBytesPerSec := state.bytesThisSec.Add(byteCount)

	if quota.MaxEventsPerSec > 0 && currentEventsPerSec > quota.MaxEventsPerSec {
		quotaExceeded.WithLabelValues(tenantID, "rate_limit").Inc()
		return false, "rate limit exceeded"
	}

	if quota.MaxBytesPerSec > 0 && currentBytesPerSec > quota.MaxBytesPerSec {
		quotaExceeded.WithLabelValues(tenantID, "rate_limit").Inc()
		return false, "rate limit exceeded"
	}

	// Check daily limit
	currentEventsToday := state.eventsToday.Add(eventCount)
	currentBytesToday := state.bytesToday.Add(byteCount)

	if quota.MaxEventsPerDay > 0 && currentEventsToday > quota.MaxEventsPerDay {
		quotaExceeded.WithLabelValues(tenantID, "daily_limit").Inc()
		if !tenant.AllowOverage {
			return false, "daily event limit exceeded"
		}
	}

	if quota.MaxBytesPerDay > 0 && currentBytesToday > quota.MaxBytesPerDay {
		quotaExceeded.WithLabelValues(tenantID, "daily_limit").Inc()
		if !tenant.AllowOverage {
			return false, "daily byte limit exceeded"
		}
	}

	// Check monthly limit
	currentEventsMonth := state.eventsThisMonth.Add(eventCount)
	currentBytesMonth := state.bytesThisMonth.Add(byteCount)

	if quota.MaxEventsPerMonth > 0 && currentEventsMonth > quota.MaxEventsPerMonth {
		quotaExceeded.WithLabelValues(tenantID, "monthly_limit").Inc()
		if !tenant.AllowOverage {
			return false, "monthly event limit exceeded"
		}
	}

	if quota.MaxBytesPerMonth > 0 && currentBytesMonth > quota.MaxBytesPerMonth {
		quotaExceeded.WithLabelValues(tenantID, "monthly_limit").Inc()
		if !tenant.AllowOverage {
			return false, "monthly byte limit exceeded"
		}
	}

	// Update metrics
	tenantEvents.WithLabelValues(tenantID, "total").Add(float64(eventCount))
	tenantBytes.WithLabelValues(tenantID).Add(float64(byteCount))

	// Calculate and report usage percentages
	if quota.MaxBytesPerDay > 0 {
		usage := float64(currentBytesToday) / float64(quota.MaxBytesPerDay) * 100
		quotaUsage.WithLabelValues(tenantID, "daily_bytes").Set(usage)

		// Send warnings
		qm.checkAndSendWarnings(state, tenantID, usage)
	}

	return true, ""
}

// checkAndSendWarnings sends quota warnings at 80%, 90%, 100%
func (qm *QuotaManager) checkAndSendWarnings(state *TenantState, tenantID string, percentUsed float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if percentUsed >= 100 && !state.warningSent100 {
		state.warningSent100 = true
		if qm.onQuotaWarning != nil {
			qm.onQuotaWarning(tenantID, "daily_bytes", percentUsed)
		}
		qm.logger.Warn("Tenant quota 100% used",
			zap.String("tenant", tenantID),
			zap.Float64("percent", percentUsed),
		)
	} else if percentUsed >= 90 && !state.warningSent90 {
		state.warningSent90 = true
		if qm.onQuotaWarning != nil {
			qm.onQuotaWarning(tenantID, "daily_bytes", percentUsed)
		}
	} else if percentUsed >= 80 && !state.warningSent80 {
		state.warningSent80 = true
		if qm.onQuotaWarning != nil {
			qm.onQuotaWarning(tenantID, "daily_bytes", percentUsed)
		}
	}
}

// GetUsage returns current usage for a tenant
func (qm *QuotaManager) GetUsage(tenantID string) map[string]interface{} {
	stateI, ok := qm.state.Load(tenantID)
	if !ok {
		return nil
	}
	state := stateI.(*TenantState)

	tenant, ok := qm.GetTenant(tenantID)
	if !ok {
		return nil
	}

	quota := tenant.Quota
	if tenant.CustomQuota != nil {
		quota = *tenant.CustomQuota
	}

	return map[string]interface{}{
		"events_today":     state.eventsToday.Load(),
		"bytes_today":      state.bytesToday.Load(),
		"events_this_month": state.eventsThisMonth.Load(),
		"bytes_this_month": state.bytesThisMonth.Load(),
		"quota": map[string]interface{}{
			"max_events_per_day":   quota.MaxEventsPerDay,
			"max_bytes_per_day":    quota.MaxBytesPerDay,
			"max_events_per_month": quota.MaxEventsPerMonth,
			"max_bytes_per_month":  quota.MaxBytesPerMonth,
		},
	}
}

// SetQuotaExceededCallback sets callback for quota exceeded events
func (qm *QuotaManager) SetQuotaExceededCallback(cb func(tenantID string, quotaType string, usage, limit int64)) {
	qm.onQuotaExceeded = cb
}

// SetQuotaWarningCallback sets callback for quota warning events
func (qm *QuotaManager) SetQuotaWarningCallback(cb func(tenantID string, quotaType string, percentUsed float64)) {
	qm.onQuotaWarning = cb
}

// resetLoop resets counters at appropriate intervals
func (qm *QuotaManager) resetLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for t := range ticker.C {
		qm.state.Range(func(key, value interface{}) bool {
			state := value.(*TenantState)

			// Reset per-second counters
			state.eventsThisSec.Store(0)
			state.bytesThisSec.Store(0)

			// Reset daily counters at midnight UTC
			if t.UTC().Day() != state.dayStart.Day() {
				state.mu.Lock()
				state.eventsToday.Store(0)
				state.bytesToday.Store(0)
				state.dayStart = startOfDay(t)
				state.warningSent80 = false
				state.warningSent90 = false
				state.warningSent100 = false
				state.mu.Unlock()
			}

			// Reset monthly counters
			if t.UTC().Month() != state.monthStart.Month() {
				state.mu.Lock()
				state.eventsThisMonth.Store(0)
				state.bytesThisMonth.Store(0)
				state.monthStart = startOfMonth(t)
				state.mu.Unlock()
			}

			return true
		})
	}
}

// cleanupLoop removes stale tenant state
func (qm *QuotaManager) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Cleanup logic for inactive tenants
	}
}

// Helper functions

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// FormatBytes formats bytes to human readable
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
