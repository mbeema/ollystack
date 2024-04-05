package enrichment

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// enrichmentProcessor enriches telemetry data with additional context.
type enrichmentProcessor struct {
	logger *zap.Logger
	cfg    *Config

	// Kubernetes client for metadata enrichment
	k8sClient *kubernetesClient

	// Cloud metadata cache
	cloudMetadata map[string]string
	cloudMu       sync.RWMutex

	nextTraces  consumer.Traces
	nextMetrics consumer.Metrics
	nextLogs    consumer.Logs
}

// kubernetesClient handles Kubernetes API interactions.
type kubernetesClient struct {
	enabled bool
	// Add Kubernetes client implementation
}

func newEnrichmentProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextConsumer consumer.Traces,
) (*enrichmentProcessor, error) {
	p := &enrichmentProcessor{
		logger:        logger,
		cfg:           cfg,
		nextTraces:    nextConsumer,
		cloudMetadata: make(map[string]string),
	}

	if cfg.Kubernetes.Enabled {
		k8s, err := newKubernetesClient(cfg.Kubernetes)
		if err != nil {
			logger.Warn("Failed to initialize Kubernetes client", zap.Error(err))
		} else {
			p.k8sClient = k8s
		}
	}

	if cfg.Cloud.Enabled {
		if err := p.loadCloudMetadata(); err != nil {
			logger.Warn("Failed to load cloud metadata", zap.Error(err))
		}
	}

	return p, nil
}

func newEnrichmentMetricsProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextConsumer consumer.Metrics,
) (*enrichmentProcessor, error) {
	p := &enrichmentProcessor{
		logger:        logger,
		cfg:           cfg,
		nextMetrics:   nextConsumer,
		cloudMetadata: make(map[string]string),
	}
	return p, nil
}

func newEnrichmentLogsProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextConsumer consumer.Logs,
) (*enrichmentProcessor, error) {
	p := &enrichmentProcessor{
		logger:      logger,
		cfg:         cfg,
		nextLogs:    nextConsumer,
		cloudMetadata: make(map[string]string),
	}
	return p, nil
}

func newKubernetesClient(cfg KubernetesConfig) (*kubernetesClient, error) {
	return &kubernetesClient{enabled: cfg.Enabled}, nil
}

func (p *enrichmentProcessor) loadCloudMetadata() error {
	p.cloudMu.Lock()
	defer p.cloudMu.Unlock()

	switch p.cfg.Cloud.Provider {
	case "aws", "auto":
		p.loadAWSMetadata()
	case "azure":
		p.loadAzureMetadata()
	case "gcp":
		p.loadGCPMetadata()
	}

	return nil
}

func (p *enrichmentProcessor) loadAWSMetadata() {
	// Fetch from EC2 metadata service
	// http://169.254.169.254/latest/meta-data/
	p.cloudMetadata["cloud.provider"] = "aws"
}

func (p *enrichmentProcessor) loadAzureMetadata() {
	// Fetch from Azure IMDS
	// http://169.254.169.254/metadata/instance
	p.cloudMetadata["cloud.provider"] = "azure"
}

func (p *enrichmentProcessor) loadGCPMetadata() {
	// Fetch from GCP metadata server
	// http://metadata.google.internal/computeMetadata/v1/
	p.cloudMetadata["cloud.provider"] = "gcp"
}

// ConsumeTraces processes trace data.
func (p *enrichmentProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		p.enrichResource(rs.Resource())

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				p.enrichSpan(span)
			}
		}
	}

	return p.nextTraces.ConsumeTraces(ctx, td)
}

// ConsumeMetrics processes metric data.
func (p *enrichmentProcessor) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		p.enrichResource(rm.Resource())
	}

	return p.nextMetrics.ConsumeMetrics(ctx, md)
}

// ConsumeLogs processes log data.
func (p *enrichmentProcessor) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		p.enrichResource(rl.Resource())
	}

	return p.nextLogs.ConsumeLogs(ctx, ld)
}

// enrichResource adds cloud and Kubernetes metadata to a resource.
func (p *enrichmentProcessor) enrichResource(resource pcommon.Resource) {
	attrs := resource.Attributes()

	// Add cloud metadata
	p.cloudMu.RLock()
	for k, v := range p.cloudMetadata {
		if _, exists := attrs.Get(k); !exists {
			attrs.PutStr(k, v)
		}
	}
	p.cloudMu.RUnlock()

	// Add Kubernetes metadata if available
	if p.k8sClient != nil && p.k8sClient.enabled {
		p.enrichWithKubernetesMetadata(attrs)
	}
}

// enrichSpan adds additional context to a span.
func (p *enrichmentProcessor) enrichSpan(span ptrace.Span) {
	attrs := span.Attributes()

	// Add span-specific enrichments
	// e.g., normalize HTTP attributes, add derived fields
	if httpMethod, exists := attrs.Get("http.method"); exists {
		// Normalize HTTP method to uppercase
		attrs.PutStr("http.method", normalizeHTTPMethod(httpMethod.Str()))
	}
}

// enrichWithKubernetesMetadata adds Kubernetes metadata to attributes.
func (p *enrichmentProcessor) enrichWithKubernetesMetadata(attrs pcommon.Map) {
	// This would use the Kubernetes client to fetch pod/node metadata
	// based on IP address or pod name in the attributes

	// Example attributes that would be added:
	// k8s.pod.name
	// k8s.pod.uid
	// k8s.namespace.name
	// k8s.node.name
	// k8s.deployment.name
	// k8s.container.name
}

func normalizeHTTPMethod(method string) string {
	switch method {
	case "get", "GET":
		return "GET"
	case "post", "POST":
		return "POST"
	case "put", "PUT":
		return "PUT"
	case "delete", "DELETE":
		return "DELETE"
	case "patch", "PATCH":
		return "PATCH"
	case "head", "HEAD":
		return "HEAD"
	case "options", "OPTIONS":
		return "OPTIONS"
	default:
		return method
	}
}

// Capabilities returns the processor capabilities.
func (p *enrichmentProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// Start starts the processor.
func (p *enrichmentProcessor) Start(ctx context.Context, host component.Host) error {
	return nil
}

// Shutdown shuts down the processor.
func (p *enrichmentProcessor) Shutdown(ctx context.Context) error {
	return nil
}

// component.Host is needed for the Start method
type component struct{}
type Host interface{}
