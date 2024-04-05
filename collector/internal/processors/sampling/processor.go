package sampling

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// samplingProcessor implements tail-based sampling.
type samplingProcessor struct {
	logger       *zap.Logger
	cfg          *Config
	nextConsumer consumer.Traces

	// Trace buffer for making sampling decisions
	traces    map[pcommon.TraceID]*traceData
	tracesMu  sync.RWMutex

	// Decision cache
	decisions    map[pcommon.TraceID]bool
	decisionsMu  sync.RWMutex

	// Rate limiter state
	rateLimiter *rateLimiter

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// traceData holds buffered span data for a trace.
type traceData struct {
	spans       []ptrace.Span
	receivedAt  time.Time
	hasError    bool
	maxLatency  time.Duration
	serviceSet  map[string]bool
}

// rateLimiter implements token bucket rate limiting.
type rateLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func newSamplingProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextConsumer consumer.Traces,
) (*samplingProcessor, error) {
	p := &samplingProcessor{
		logger:       logger,
		cfg:          cfg,
		nextConsumer: nextConsumer,
		traces:       make(map[pcommon.TraceID]*traceData),
		decisions:    make(map[pcommon.TraceID]bool),
		stopChan:     make(chan struct{}),
	}

	// Initialize rate limiter
	for _, policy := range cfg.Policies {
		if policy.Type == "rate_limiting" {
			p.rateLimiter = &rateLimiter{
				tokens:     float64(policy.SpansPerSecond),
				maxTokens:  float64(policy.SpansPerSecond),
				refillRate: float64(policy.SpansPerSecond),
				lastRefill: time.Now(),
			}
			break
		}
	}

	return p, nil
}

// ConsumeTraces processes incoming traces.
func (p *samplingProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	// Buffer spans and make sampling decisions
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				traceID := span.TraceID()

				// Check if we already have a decision for this trace
				p.decisionsMu.RLock()
				decision, exists := p.decisions[traceID]
				p.decisionsMu.RUnlock()

				if exists {
					if decision {
						// Already decided to sample, forward immediately
						continue
					} else {
						// Already decided not to sample, drop
						continue
					}
				}

				// Buffer the span
				p.bufferSpan(traceID, span)

				// Check if we should make a decision now
				if p.shouldMakeDecision(traceID, span) {
					sample := p.evaluate(traceID)
					p.recordDecision(traceID, sample)
				}
			}
		}
	}

	// Forward sampled traces
	return p.forwardSampledTraces(ctx, td)
}

// bufferSpan adds a span to the trace buffer.
func (p *samplingProcessor) bufferSpan(traceID pcommon.TraceID, span ptrace.Span) {
	p.tracesMu.Lock()
	defer p.tracesMu.Unlock()

	td, exists := p.traces[traceID]
	if !exists {
		td = &traceData{
			spans:      make([]ptrace.Span, 0),
			receivedAt: time.Now(),
			serviceSet: make(map[string]bool),
		}
		p.traces[traceID] = td
	}

	td.spans = append(td.spans, span)

	// Track error status
	if span.Status().Code() == ptrace.StatusCodeError {
		td.hasError = true
	}

	// Track latency
	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
	if duration > td.maxLatency {
		td.maxLatency = duration
	}
}

// shouldMakeDecision determines if we should make a sampling decision now.
func (p *samplingProcessor) shouldMakeDecision(traceID pcommon.TraceID, span ptrace.Span) bool {
	// Make immediate decision for root spans with errors
	if span.ParentSpanID().IsEmpty() && span.Status().Code() == ptrace.StatusCodeError {
		return true
	}

	// Check if decision wait time has elapsed
	p.tracesMu.RLock()
	td, exists := p.traces[traceID]
	p.tracesMu.RUnlock()

	if !exists {
		return false
	}

	decisionWait, _ := time.ParseDuration(p.cfg.DecisionWait)
	return time.Since(td.receivedAt) >= decisionWait
}

// evaluate evaluates all policies for a trace.
func (p *samplingProcessor) evaluate(traceID pcommon.TraceID) bool {
	p.tracesMu.RLock()
	td, exists := p.traces[traceID]
	p.tracesMu.RUnlock()

	if !exists {
		return false
	}

	for _, policy := range p.cfg.Policies {
		switch policy.Type {
		case "always_sample":
			return true

		case "status_code":
			if td.hasError {
				for _, code := range policy.StatusCodes {
					if code == "ERROR" {
						return true
					}
				}
			}

		case "latency":
			if td.maxLatency.Milliseconds() >= policy.LatencyMS {
				return true
			}

		case "rate_limiting":
			if p.rateLimiter != nil && p.rateLimiter.allow() {
				return true
			}

		case "string_attribute":
			if p.matchesAttribute(td, policy) {
				return true
			}
		}
	}

	return false
}

// matchesAttribute checks if any span matches the attribute policy.
func (p *samplingProcessor) matchesAttribute(td *traceData, policy PolicyConfig) bool {
	for _, span := range td.spans {
		val, exists := span.Attributes().Get(policy.Attribute)
		if !exists {
			continue
		}

		strVal := val.Str()
		for _, v := range policy.Values {
			match := strVal == v
			if policy.InvertMatch {
				match = !match
			}
			if match {
				return true
			}
		}
	}
	return false
}

// recordDecision records a sampling decision.
func (p *samplingProcessor) recordDecision(traceID pcommon.TraceID, sample bool) {
	p.decisionsMu.Lock()
	p.decisions[traceID] = sample
	p.decisionsMu.Unlock()

	// Clean up buffer
	p.tracesMu.Lock()
	delete(p.traces, traceID)
	p.tracesMu.Unlock()
}

// forwardSampledTraces forwards traces that should be sampled.
func (p *samplingProcessor) forwardSampledTraces(ctx context.Context, td ptrace.Traces) error {
	// Create new trace data with only sampled spans
	sampled := ptrace.NewTraces()

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		var newRS ptrace.ResourceSpans

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			var newSS ptrace.ScopeSpans

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				traceID := span.TraceID()

				p.decisionsMu.RLock()
				decision, exists := p.decisions[traceID]
				p.decisionsMu.RUnlock()

				if exists && decision {
					if newRS.ResourceSpans().Len() == 0 {
						newRS = sampled.ResourceSpans().AppendEmpty()
						rs.Resource().CopyTo(newRS.Resource())
					}
					if newSS.ScopeSpans().Len() == 0 {
						newSS = newRS.ScopeSpans().AppendEmpty()
						ss.Scope().CopyTo(newSS.Scope())
					}
					span.CopyTo(newSS.Spans().AppendEmpty())
				}
			}
		}
	}

	if sampled.ResourceSpans().Len() > 0 {
		return p.nextConsumer.ConsumeTraces(ctx, sampled)
	}

	return nil
}

// allow checks if the rate limiter allows a request.
func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// Capabilities returns the processor capabilities.
func (p *samplingProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// Start starts the processor.
func (p *samplingProcessor) Start(ctx context.Context, host interface{}) error {
	// Start background goroutine for decision making
	p.wg.Add(1)
	go p.decisionLoop()
	return nil
}

// decisionLoop periodically makes decisions for buffered traces.
func (p *samplingProcessor) decisionLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.processBufferedTraces()
		}
	}
}

// processBufferedTraces evaluates buffered traces that are ready for decision.
func (p *samplingProcessor) processBufferedTraces() {
	decisionWait, _ := time.ParseDuration(p.cfg.DecisionWait)

	p.tracesMu.RLock()
	var readyTraces []pcommon.TraceID
	for traceID, td := range p.traces {
		if time.Since(td.receivedAt) >= decisionWait {
			readyTraces = append(readyTraces, traceID)
		}
	}
	p.tracesMu.RUnlock()

	for _, traceID := range readyTraces {
		sample := p.evaluate(traceID)
		p.recordDecision(traceID, sample)
	}
}

// Shutdown shuts down the processor.
func (p *samplingProcessor) Shutdown(ctx context.Context) error {
	close(p.stopChan)
	p.wg.Wait()
	return nil
}
