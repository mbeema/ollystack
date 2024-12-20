// Package pipeline handles telemetry processing with sampling, cardinality control, and enrichment
package pipeline

import (
	"context"
	"hash/fnv"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/ollystack/unified-agent/internal/aggregator"
	"github.com/ollystack/unified-agent/internal/exporter"
	"github.com/ollystack/unified-agent/internal/types"
)

// Config configures the processing pipeline
type Config struct {
	SamplingConfig    SamplingConfig
	CardinalityConfig CardinalityConfig
	EnrichmentConfig  EnrichmentConfig
	Aggregator        *aggregator.Aggregator
	Exporter          *exporter.OTLPExporter
}

// SamplingConfig controls adaptive sampling
type SamplingConfig struct {
	Enabled            bool
	TargetRate         int64 // bytes/second
	TraceRate          float64
	AlwaysSampleErrors bool
	SlowThreshold      time.Duration
	LogInfoRate        float64
	LogDebugRate       float64
	AlwaysKeepErrors   bool
}

// CardinalityConfig prevents metric explosion
type CardinalityConfig struct {
	Enabled            bool
	MaxSeriesPerMetric int
	MaxLabelValues     map[string]int
	DropLabels         []string
}

// EnrichmentConfig controls metadata enrichment
type EnrichmentConfig struct {
	AddHostname       bool
	Hostname          string
	Environment       string
	StaticTags        map[string]string
	KubernetesEnabled bool
	CloudEnabled      bool
	CloudProvider     string
}

// Pipeline processes telemetry data
type Pipeline struct {
	config     Config
	logger     *zap.Logger
	aggregator *aggregator.Aggregator
	exporter   *exporter.OTLPExporter

	// Enrichment data (cached)
	hostname     string
	environment  string
	staticTags   map[string]string
	k8sMetadata  map[string]string
	cloudMetadata map[string]string

	// Cardinality tracking
	cardinalityMu   sync.RWMutex
	seriesPerMetric map[string]map[string]struct{} // metric -> set of label combinations
	labelValues     map[string]map[string]struct{} // label -> set of values

	// Sampling state
	samplingMu    sync.RWMutex
	currentRate   float64
	bytesThisSecond int64
	lastRateCheck time.Time

	// Operation frequency tracking (for rare operation boost)
	operationMu    sync.RWMutex
	operationCounts map[string]int64

	// Stats
	stats PipelineStats
}

// PipelineStats tracks pipeline statistics
type PipelineStats struct {
	MetricsProcessed     int64
	LogsProcessed        int64
	TracesProcessed      int64
	DroppedByCardinality int64
	DroppedBySampling    int64
	AggregationRatio     float64
}

// New creates a new pipeline
func New(cfg Config, logger *zap.Logger) *Pipeline {
	hostname := cfg.EnrichmentConfig.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	p := &Pipeline{
		config:          cfg,
		logger:          logger,
		aggregator:      cfg.Aggregator,
		exporter:        cfg.Exporter,
		hostname:        hostname,
		environment:     cfg.EnrichmentConfig.Environment,
		staticTags:      cfg.EnrichmentConfig.StaticTags,
		seriesPerMetric: make(map[string]map[string]struct{}),
		labelValues:     make(map[string]map[string]struct{}),
		currentRate:     cfg.SamplingConfig.TraceRate,
		operationCounts: make(map[string]int64),
	}

	// Load cloud metadata if enabled
	if cfg.EnrichmentConfig.CloudEnabled {
		p.loadCloudMetadata()
	}

	// Load Kubernetes metadata if enabled
	if cfg.EnrichmentConfig.KubernetesEnabled {
		p.loadK8sMetadata()
	}

	return p
}

// Start begins pipeline processing
func (p *Pipeline) Start(ctx context.Context) error {
	// Start aggregation flush loop
	go p.flushLoop(ctx)

	// Start adaptive sampling adjustment
	if p.config.SamplingConfig.Enabled {
		go p.adjustSamplingLoop(ctx)
	}

	<-ctx.Done()
	return nil
}

// ProcessMetric processes a single metric
func (p *Pipeline) ProcessMetric(m types.Metric) {
	atomic.AddInt64(&p.stats.MetricsProcessed, 1)

	// Apply cardinality control
	if p.config.CardinalityConfig.Enabled {
		if !p.checkCardinality(m) {
			atomic.AddInt64(&p.stats.DroppedByCardinality, 1)
			return
		}
	}

	// Enrich
	m = p.enrichMetric(m)

	// Send to aggregator
	if p.aggregator != nil && p.config.Aggregator != nil {
		p.aggregator.AddMetric(m)
	} else {
		// Direct export if no aggregation
		p.exporter.ExportMetric(m)
	}
}

// ProcessLog processes a single log record
func (p *Pipeline) ProcessLog(log types.LogRecord) {
	atomic.AddInt64(&p.stats.LogsProcessed, 1)

	// Apply sampling
	if p.config.SamplingConfig.Enabled {
		if !p.shouldSampleLog(log) {
			atomic.AddInt64(&p.stats.DroppedBySampling, 1)
			return
		}
	}

	// Enrich
	log = p.enrichLog(log)

	// Send to aggregator or exporter
	if p.aggregator != nil && p.config.Aggregator != nil {
		p.aggregator.AddLog(log)
	} else {
		p.exporter.ExportLog(log)
	}
}

// ProcessTrace processes a trace span
func (p *Pipeline) ProcessTrace(span types.Span) {
	atomic.AddInt64(&p.stats.TracesProcessed, 1)

	// Apply sampling
	if p.config.SamplingConfig.Enabled {
		if !p.shouldSampleTrace(span) {
			atomic.AddInt64(&p.stats.DroppedBySampling, 1)
			return
		}
	}

	// Track operation frequency
	p.trackOperation(span.Name)

	// Enrich
	span = p.enrichSpan(span)

	// Export directly (traces are not aggregated)
	p.exporter.ExportSpan(span)
}

// Cardinality control

func (p *Pipeline) checkCardinality(m types.Metric) bool {
	p.cardinalityMu.Lock()
	defer p.cardinalityMu.Unlock()

	// Check series per metric
	if p.config.CardinalityConfig.MaxSeriesPerMetric > 0 {
		if _, exists := p.seriesPerMetric[m.Name]; !exists {
			p.seriesPerMetric[m.Name] = make(map[string]struct{})
		}

		labelKey := p.labelKey(m.Labels)
		p.seriesPerMetric[m.Name][labelKey] = struct{}{}

		if len(p.seriesPerMetric[m.Name]) > p.config.CardinalityConfig.MaxSeriesPerMetric {
			p.logger.Warn("Cardinality limit exceeded",
				zap.String("metric", m.Name),
				zap.Int("series", len(p.seriesPerMetric[m.Name])),
			)
			return false
		}
	}

	// Check individual label values
	for label, value := range m.Labels {
		maxValues, hasLimit := p.config.CardinalityConfig.MaxLabelValues[label]
		if hasLimit {
			if maxValues == 0 {
				// Label should never be used
				delete(m.Labels, label)
				continue
			}

			if _, exists := p.labelValues[label]; !exists {
				p.labelValues[label] = make(map[string]struct{})
			}

			p.labelValues[label][value] = struct{}{}

			if len(p.labelValues[label]) > maxValues {
				// Replace with "other" bucket
				m.Labels[label] = "__other__"
			}
		}
	}

	// Drop configured labels
	for _, label := range p.config.CardinalityConfig.DropLabels {
		delete(m.Labels, label)
	}

	return true
}

func (p *Pipeline) labelKey(labels map[string]string) string {
	// Create deterministic key from labels
	h := fnv.New64a()
	for k, v := range labels {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}
	return string(h.Sum(nil))
}

// Sampling

func (p *Pipeline) shouldSampleLog(log types.LogRecord) bool {
	// Always keep errors
	if p.config.SamplingConfig.AlwaysKeepErrors {
		if log.Severity >= types.SeverityError {
			return true
		}
	}

	// Sample based on severity
	var rate float64
	switch {
	case log.Severity >= types.SeverityError:
		rate = 1.0 // Always keep errors
	case log.Severity >= types.SeverityWarn:
		rate = 1.0 // Always keep warnings
	case log.Severity >= types.SeverityInfo:
		rate = p.config.SamplingConfig.LogInfoRate
	default:
		rate = p.config.SamplingConfig.LogDebugRate
	}

	return rand.Float64() < rate
}

func (p *Pipeline) shouldSampleTrace(span types.Span) bool {
	// Always sample errors
	if p.config.SamplingConfig.AlwaysSampleErrors {
		if span.Status == types.SpanStatusError {
			return true
		}
	}

	// Always sample slow requests
	if p.config.SamplingConfig.SlowThreshold > 0 {
		if span.Duration >= p.config.SamplingConfig.SlowThreshold {
			return true
		}
	}

	// Check for rare operation boost
	rate := p.getCurrentSamplingRate()

	p.operationMu.RLock()
	opCount := p.operationCounts[span.Name]
	p.operationMu.RUnlock()

	if opCount < 100 { // New/rare operation
		rate = rate * p.config.SamplingConfig.TraceRate
		if rate > 1.0 {
			rate = 1.0
		}
	}

	// Consistent sampling based on trace ID
	return p.consistentSample(span.TraceID, rate)
}

func (p *Pipeline) consistentSample(traceID string, rate float64) bool {
	// Use trace ID for consistent sampling (same decision for all spans in trace)
	h := fnv.New64a()
	h.Write([]byte(traceID))
	hashValue := float64(h.Sum64()) / float64(^uint64(0))
	return hashValue < rate
}

func (p *Pipeline) getCurrentSamplingRate() float64 {
	p.samplingMu.RLock()
	defer p.samplingMu.RUnlock()
	return p.currentRate
}

func (p *Pipeline) trackOperation(name string) {
	p.operationMu.Lock()
	p.operationCounts[name]++
	p.operationMu.Unlock()
}

func (p *Pipeline) adjustSamplingLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.adjustSampling()
		}
	}
}

func (p *Pipeline) adjustSampling() {
	// Adjust sampling rate based on data volume vs target
	p.samplingMu.Lock()
	defer p.samplingMu.Unlock()

	currentBytes := atomic.LoadInt64(&p.bytesThisSecond)
	atomic.StoreInt64(&p.bytesThisSecond, 0)

	targetRate := p.config.SamplingConfig.TargetRate
	if targetRate <= 0 {
		return
	}

	if currentBytes > targetRate {
		// Reduce sampling
		p.currentRate = p.currentRate * 0.9
		if p.currentRate < 0.01 {
			p.currentRate = 0.01
		}
		p.logger.Debug("Reduced sampling rate",
			zap.Float64("rate", p.currentRate),
			zap.Int64("bytes", currentBytes),
		)
	} else if currentBytes < targetRate/2 {
		// Increase sampling
		p.currentRate = p.currentRate * 1.1
		if p.currentRate > 1.0 {
			p.currentRate = 1.0
		}
	}
}

// Enrichment

func (p *Pipeline) enrichMetric(m types.Metric) types.Metric {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}

	// Add hostname
	if p.config.EnrichmentConfig.AddHostname && p.hostname != "" {
		m.Labels["host"] = p.hostname
	}

	// Add environment
	if p.environment != "" {
		m.Labels["environment"] = p.environment
	}

	// Add static tags
	for k, v := range p.staticTags {
		if _, exists := m.Labels[k]; !exists {
			m.Labels[k] = v
		}
	}

	// Add cloud metadata
	for k, v := range p.cloudMetadata {
		if _, exists := m.Labels[k]; !exists {
			m.Labels[k] = v
		}
	}

	// Add K8s metadata
	for k, v := range p.k8sMetadata {
		if _, exists := m.Labels[k]; !exists {
			m.Labels[k] = v
		}
	}

	return m
}

func (p *Pipeline) enrichLog(log types.LogRecord) types.LogRecord {
	if log.Attributes == nil {
		log.Attributes = make(map[string]string)
	}

	if p.hostname != "" {
		log.Attributes["host"] = p.hostname
	}
	if p.environment != "" {
		log.Attributes["environment"] = p.environment
	}

	for k, v := range p.staticTags {
		log.Attributes[k] = v
	}

	return log
}

func (p *Pipeline) enrichSpan(span types.Span) types.Span {
	if span.Attributes == nil {
		span.Attributes = make(map[string]string)
	}

	if p.hostname != "" {
		span.Attributes["host"] = p.hostname
	}
	if p.environment != "" {
		span.Attributes["deployment.environment"] = p.environment
	}

	for k, v := range p.staticTags {
		span.Attributes[k] = v
	}

	return span
}

func (p *Pipeline) loadCloudMetadata() {
	p.cloudMetadata = make(map[string]string)

	provider := p.config.EnrichmentConfig.CloudProvider
	if provider == "auto" {
		provider = detectCloudProvider()
	}

	switch provider {
	case "aws":
		p.loadAWSMetadata()
	case "gcp":
		p.loadGCPMetadata()
	case "azure":
		p.loadAzureMetadata()
	}
}

func detectCloudProvider() string {
	// Check for cloud-specific files/endpoints
	if _, err := os.Stat("/sys/hypervisor/uuid"); err == nil {
		return "aws"
	}
	if _, err := os.Stat("/sys/class/dmi/id/product_name"); err == nil {
		data, _ := os.ReadFile("/sys/class/dmi/id/product_name")
		if string(data) == "Google Compute Engine\n" {
			return "gcp"
		}
	}
	return ""
}

func (p *Pipeline) loadAWSMetadata() {
	// TODO: Call EC2 metadata service
	// curl http://169.254.169.254/latest/meta-data/
	p.cloudMetadata["cloud.provider"] = "aws"
}

func (p *Pipeline) loadGCPMetadata() {
	// TODO: Call GCP metadata service
	p.cloudMetadata["cloud.provider"] = "gcp"
}

func (p *Pipeline) loadAzureMetadata() {
	// TODO: Call Azure IMDS
	p.cloudMetadata["cloud.provider"] = "azure"
}

func (p *Pipeline) loadK8sMetadata() {
	p.k8sMetadata = make(map[string]string)

	// Read from downward API if available
	if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		p.k8sMetadata["k8s.namespace"] = string(ns)
	}

	// Read from environment (set by K8s)
	if podName := os.Getenv("HOSTNAME"); podName != "" {
		p.k8sMetadata["k8s.pod.name"] = podName
	}
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		p.k8sMetadata["k8s.node.name"] = nodeName
	}
}

// Flush loop

func (p *Pipeline) flushLoop(ctx context.Context) {
	if p.aggregator == nil {
		return
	}

	ticker := time.NewTicker(p.config.Aggregator.config.Window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush
			p.flush()
			return
		case <-ticker.C:
			p.flush()
		}
	}
}

func (p *Pipeline) flush() {
	if p.aggregator == nil {
		return
	}

	// Flush metrics
	metrics := p.aggregator.Flush()
	for _, m := range metrics {
		p.exporter.ExportMetric(m)
	}

	// Flush logs
	logs := p.aggregator.FlushLogs()
	for _, l := range logs {
		p.exporter.ExportLog(l)
	}

	// Update stats
	aggStats := p.aggregator.Stats()
	p.stats.AggregationRatio = aggStats.ReductionPct / 100
}

// Stats returns pipeline statistics
func (p *Pipeline) Stats() PipelineStats {
	return PipelineStats{
		MetricsProcessed:     atomic.LoadInt64(&p.stats.MetricsProcessed),
		LogsProcessed:        atomic.LoadInt64(&p.stats.LogsProcessed),
		TracesProcessed:      atomic.LoadInt64(&p.stats.TracesProcessed),
		DroppedByCardinality: atomic.LoadInt64(&p.stats.DroppedByCardinality),
		DroppedBySampling:    atomic.LoadInt64(&p.stats.DroppedBySampling),
		AggregationRatio:     p.stats.AggregationRatio,
	}
}
