package topology

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// ServiceNode represents a service in the topology.
type ServiceNode struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace,omitempty"`
	Type       string            `json:"type,omitempty"` // http, grpc, database, cache, queue
	Attributes map[string]string `json:"attributes,omitempty"`
	LastSeen   time.Time         `json:"last_seen"`
	SpanCount  int64             `json:"span_count"`
	ErrorCount int64             `json:"error_count"`
	AvgLatency float64           `json:"avg_latency_ms"`
}

// ServiceEdge represents a connection between services.
type ServiceEdge struct {
	Source       string    `json:"source"`
	Target       string    `json:"target"`
	Protocol     string    `json:"protocol,omitempty"` // http, grpc, kafka, redis, etc.
	LastSeen     time.Time `json:"last_seen"`
	RequestCount int64     `json:"request_count"`
	ErrorCount   int64     `json:"error_count"`
	AvgLatency   float64   `json:"avg_latency_ms"`
	P99Latency   float64   `json:"p99_latency_ms"`
}

// Topology represents the complete service topology.
type Topology struct {
	Services map[string]*ServiceNode `json:"services"`
	Edges    map[string]*ServiceEdge `json:"edges"` // key: "source->target"
	mu       sync.RWMutex
}

// topologyProcessor discovers and tracks service topology.
type topologyProcessor struct {
	logger       *zap.Logger
	cfg          *Config
	nextConsumer consumer.Traces
	topology     *Topology

	stopChan chan struct{}
	wg       sync.WaitGroup
}

func newTopologyProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextConsumer consumer.Traces,
) (*topologyProcessor, error) {
	p := &topologyProcessor{
		logger:       logger,
		cfg:          cfg,
		nextConsumer: nextConsumer,
		topology: &Topology{
			Services: make(map[string]*ServiceNode),
			Edges:    make(map[string]*ServiceEdge),
		},
		stopChan: make(chan struct{}),
	}

	return p, nil
}

// ConsumeTraces processes traces and extracts topology information.
func (p *topologyProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		serviceName := p.extractServiceName(rs.Resource())

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				p.processSpan(serviceName, span, td)
			}
		}
	}

	// Forward to next consumer
	return p.nextConsumer.ConsumeTraces(ctx, td)
}

// extractServiceName gets the service name from resource attributes.
func (p *topologyProcessor) extractServiceName(resource pcommon.Resource) string {
	if val, exists := resource.Attributes().Get("service.name"); exists {
		return val.Str()
	}
	return "unknown"
}

// processSpan extracts topology information from a span.
func (p *topologyProcessor) processSpan(serviceName string, span ptrace.Span, td ptrace.Traces) {
	p.topology.mu.Lock()
	defer p.topology.mu.Unlock()

	// Update service node
	service, exists := p.topology.Services[serviceName]
	if !exists {
		service = &ServiceNode{
			Name:       serviceName,
			Attributes: make(map[string]string),
		}
		p.topology.Services[serviceName] = service
	}

	service.LastSeen = time.Now()
	service.SpanCount++

	if span.Status().Code() == ptrace.StatusCodeError {
		service.ErrorCount++
	}

	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
	service.AvgLatency = updateAverage(service.AvgLatency, float64(duration.Milliseconds()), service.SpanCount)

	// Detect service type
	service.Type = p.detectServiceType(span)

	// Extract edges from span relationships
	p.extractEdges(serviceName, span, td)
}

// detectServiceType determines the type of service from span attributes.
func (p *topologyProcessor) detectServiceType(span ptrace.Span) string {
	attrs := span.Attributes()

	// Check for database
	if _, exists := attrs.Get("db.system"); exists {
		return "database"
	}

	// Check for messaging
	if _, exists := attrs.Get("messaging.system"); exists {
		return "queue"
	}

	// Check for HTTP
	if _, exists := attrs.Get("http.method"); exists {
		return "http"
	}

	// Check for gRPC
	if _, exists := attrs.Get("rpc.system"); exists {
		if val, _ := attrs.Get("rpc.system"); val.Str() == "grpc" {
			return "grpc"
		}
		return "rpc"
	}

	return "service"
}

// extractEdges extracts edges from span parent-child relationships.
func (p *topologyProcessor) extractEdges(serviceName string, span ptrace.Span, td ptrace.Traces) {
	// For client spans, create edge to the server
	if span.Kind() == ptrace.SpanKindClient {
		targetService := p.findTargetService(span, td)
		if targetService != "" && targetService != serviceName {
			edgeKey := serviceName + "->" + targetService
			edge, exists := p.topology.Edges[edgeKey]
			if !exists {
				edge = &ServiceEdge{
					Source: serviceName,
					Target: targetService,
				}
				p.topology.Edges[edgeKey] = edge
			}

			edge.LastSeen = time.Now()
			edge.RequestCount++

			if span.Status().Code() == ptrace.StatusCodeError {
				edge.ErrorCount++
			}

			duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
			edge.AvgLatency = updateAverage(edge.AvgLatency, float64(duration.Milliseconds()), edge.RequestCount)

			// Detect protocol
			edge.Protocol = p.detectProtocol(span)
		}
	}
}

// findTargetService attempts to find the target service for a client span.
func (p *topologyProcessor) findTargetService(span ptrace.Span, td ptrace.Traces) string {
	attrs := span.Attributes()

	// Check peer.service attribute
	if val, exists := attrs.Get("peer.service"); exists {
		return val.Str()
	}

	// Check for database
	if val, exists := attrs.Get("db.system"); exists {
		return val.Str()
	}

	// Check for messaging destination
	if val, exists := attrs.Get("messaging.destination"); exists {
		return val.Str()
	}

	// Check for HTTP host
	if val, exists := attrs.Get("http.host"); exists {
		return val.Str()
	}

	// Check for net.peer.name
	if val, exists := attrs.Get("net.peer.name"); exists {
		return val.Str()
	}

	return ""
}

// detectProtocol determines the protocol from span attributes.
func (p *topologyProcessor) detectProtocol(span ptrace.Span) string {
	attrs := span.Attributes()

	if val, exists := attrs.Get("db.system"); exists {
		return val.Str()
	}

	if val, exists := attrs.Get("messaging.system"); exists {
		return val.Str()
	}

	if val, exists := attrs.Get("rpc.system"); exists {
		return val.Str()
	}

	if _, exists := attrs.Get("http.method"); exists {
		return "http"
	}

	return "unknown"
}

// updateAverage calculates a running average.
func updateAverage(current float64, newValue float64, count int64) float64 {
	if count == 1 {
		return newValue
	}
	return current + (newValue-current)/float64(count)
}

// GetTopology returns the current topology.
func (p *topologyProcessor) GetTopology() *Topology {
	p.topology.mu.RLock()
	defer p.topology.mu.RUnlock()

	// Return a copy to avoid race conditions
	copy := &Topology{
		Services: make(map[string]*ServiceNode),
		Edges:    make(map[string]*ServiceEdge),
	}

	for k, v := range p.topology.Services {
		copy.Services[k] = v
	}
	for k, v := range p.topology.Edges {
		copy.Edges[k] = v
	}

	return copy
}

// Capabilities returns the processor capabilities.
func (p *topologyProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// Start starts the processor.
func (p *topologyProcessor) Start(ctx context.Context, host interface{}) error {
	// Start cleanup goroutine
	p.wg.Add(1)
	go p.cleanupLoop()
	return nil
}

// cleanupLoop removes stale entries from the topology.
func (p *topologyProcessor) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	staleThreshold := 24 * time.Hour

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.cleanup(staleThreshold)
		}
	}
}

// cleanup removes entries that haven't been seen recently.
func (p *topologyProcessor) cleanup(threshold time.Duration) {
	p.topology.mu.Lock()
	defer p.topology.mu.Unlock()

	now := time.Now()

	for key, service := range p.topology.Services {
		if now.Sub(service.LastSeen) > threshold {
			delete(p.topology.Services, key)
		}
	}

	for key, edge := range p.topology.Edges {
		if now.Sub(edge.LastSeen) > threshold {
			delete(p.topology.Edges, key)
		}
	}
}

// Shutdown shuts down the processor.
func (p *topologyProcessor) Shutdown(ctx context.Context) error {
	close(p.stopChan)
	p.wg.Wait()
	return nil
}
