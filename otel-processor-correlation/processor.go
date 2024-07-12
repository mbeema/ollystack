// Copyright 2026 OllyStack
// SPDX-License-Identifier: Apache-2.0

package correlationprocessor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// correlationProcessor handles adding correlation IDs to telemetry data.
type correlationProcessor struct {
	config *Config
	logger *zap.Logger
}

// newCorrelationProcessor creates a new correlation processor.
func newCorrelationProcessor(config *Config, logger *zap.Logger) *correlationProcessor {
	return &correlationProcessor{
		config: config,
		logger: logger,
	}
}

// ProcessTraces adds correlation_id to all spans in the trace data.
func (p *correlationProcessor) ProcessTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()

		// Try to get correlation ID from resource attributes first
		correlationID := p.extractFromAttributes(rs.Resource().Attributes())

		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				// Try to extract from span attributes if not found at resource level
				if correlationID == "" {
					correlationID = p.extractFromAttributes(span.Attributes())
				}

				// Try to extract from baggage in trace state
				if correlationID == "" && p.config.ExtractFromBaggage {
					correlationID = p.extractFromTraceState(span.TraceState().AsRaw())
				}

				// Generate from trace ID if configured and still missing
				if correlationID == "" && p.config.DeriveFromTraceID {
					correlationID = p.generateFromTraceID(span.TraceID())
				}

				// Generate completely new if still missing
				if correlationID == "" && p.config.GenerateIfMissing {
					correlationID = p.generateNew()
				}

				// Set correlation_id attribute on span
				if correlationID != "" {
					span.Attributes().PutStr(p.config.AttributeName, correlationID)
				}
			}
		}

		// Propagate to resource attributes for consistency
		if correlationID != "" && p.config.PropagateToResource {
			rs.Resource().Attributes().PutStr(p.config.AttributeName, correlationID)
		}
	}

	return td, nil
}

// ProcessLogs adds correlation_id to all log records.
func (p *correlationProcessor) ProcessLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	resourceLogs := ld.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		rl := resourceLogs.At(i)
		scopeLogs := rl.ScopeLogs()

		// Try to get correlation ID from resource attributes
		correlationID := p.extractFromAttributes(rl.Resource().Attributes())

		for j := 0; j < scopeLogs.Len(); j++ {
			sl := scopeLogs.At(j)
			logs := sl.LogRecords()

			for k := 0; k < logs.Len(); k++ {
				logRecord := logs.At(k)

				// Try to extract from log attributes
				if correlationID == "" {
					correlationID = p.extractFromAttributes(logRecord.Attributes())
				}

				// Try to extract from log body if JSON
				if correlationID == "" {
					correlationID = p.extractFromLogBody(logRecord)
				}

				// Generate from trace ID if available
				if correlationID == "" && p.config.DeriveFromTraceID && !logRecord.TraceID().IsEmpty() {
					correlationID = p.generateFromTraceID(logRecord.TraceID())
				}

				// Generate new if still missing
				if correlationID == "" && p.config.GenerateIfMissing {
					correlationID = p.generateNew()
				}

				// Set correlation_id attribute
				if correlationID != "" {
					logRecord.Attributes().PutStr(p.config.AttributeName, correlationID)
				}
			}
		}

		// Propagate to resource
		if correlationID != "" && p.config.PropagateToResource {
			rl.Resource().Attributes().PutStr(p.config.AttributeName, correlationID)
		}
	}

	return ld, nil
}

// ProcessMetrics adds correlation_id to metrics via exemplars.
func (p *correlationProcessor) ProcessMetrics(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	resourceMetrics := md.ResourceMetrics()
	for i := 0; i < resourceMetrics.Len(); i++ {
		rm := resourceMetrics.At(i)
		scopeMetrics := rm.ScopeMetrics()

		// Try to get correlation ID from resource attributes
		correlationID := p.extractFromAttributes(rm.Resource().Attributes())

		for j := 0; j < scopeMetrics.Len(); j++ {
			sm := scopeMetrics.At(j)
			metrics := sm.Metrics()

			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)
				p.processMetricDataPoints(metric, correlationID)
			}
		}

		// Propagate to resource
		if correlationID != "" && p.config.PropagateToResource {
			rm.Resource().Attributes().PutStr(p.config.AttributeName, correlationID)
		}
	}

	return md, nil
}

// processMetricDataPoints adds correlation_id to metric exemplars.
func (p *correlationProcessor) processMetricDataPoints(metric pmetric.Metric, correlationID string) {
	//nolint:exhaustive
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dps := metric.Gauge().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			p.processExemplars(dp.Exemplars(), correlationID)
		}
	case pmetric.MetricTypeSum:
		dps := metric.Sum().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			p.processExemplars(dp.Exemplars(), correlationID)
		}
	case pmetric.MetricTypeHistogram:
		dps := metric.Histogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			p.processExemplars(dp.Exemplars(), correlationID)
		}
	case pmetric.MetricTypeExponentialHistogram:
		dps := metric.ExponentialHistogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			p.processExemplars(dp.Exemplars(), correlationID)
		}
	}
}

// processExemplars adds correlation_id to exemplar filtered attributes.
func (p *correlationProcessor) processExemplars(exemplars pmetric.ExemplarSlice, correlationID string) {
	for i := 0; i < exemplars.Len(); i++ {
		exemplar := exemplars.At(i)

		// Try to derive correlation_id from exemplar's trace ID
		corrID := correlationID
		if corrID == "" && p.config.DeriveFromTraceID && !exemplar.TraceID().IsEmpty() {
			corrID = p.generateFromTraceID(exemplar.TraceID())
		}

		if corrID != "" {
			exemplar.FilteredAttributes().PutStr(p.config.AttributeName, corrID)
		}
	}
}

// extractFromAttributes looks for correlation ID in common attribute names.
func (p *correlationProcessor) extractFromAttributes(attrs pcommon.Map) string {
	// Check the configured attribute name first
	if v, ok := attrs.Get(p.config.AttributeName); ok {
		return v.Str()
	}

	// Check configured header names (as attributes)
	for _, header := range p.config.ExtractFromHeaders {
		// Try exact match
		if v, ok := attrs.Get(header); ok {
			return v.Str()
		}
		// Try lowercase
		if v, ok := attrs.Get(strings.ToLower(header)); ok {
			return v.Str()
		}
		// Try with http.request.header. prefix (OTel semantic convention)
		if v, ok := attrs.Get("http.request.header." + strings.ToLower(header)); ok {
			return v.Str()
		}
	}

	return ""
}

// extractFromTraceState looks for correlation ID in W3C trace state.
func (p *correlationProcessor) extractFromTraceState(traceState string) string {
	if traceState == "" {
		return ""
	}

	// Parse trace state format: key1=value1,key2=value2
	pairs := strings.Split(traceState, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == p.config.BaggageKey {
			return kv[1]
		}
	}

	return ""
}

// extractFromLogBody tries to extract correlation_id from JSON log body.
func (p *correlationProcessor) extractFromLogBody(logRecord plog.LogRecord) string {
	body := logRecord.Body()
	if body.Type() == pcommon.ValueTypeMap {
		if v, ok := body.Map().Get(p.config.AttributeName); ok {
			return v.Str()
		}
		// Also check common variations
		for _, key := range []string{"correlationId", "correlation_id", "requestId", "request_id"} {
			if v, ok := body.Map().Get(key); ok {
				return v.Str()
			}
		}
	}
	return ""
}

// generateFromTraceID creates a consistent correlation ID from trace ID.
// Format: {prefix}-{timestamp_base36}-{first4bytes_of_traceid_hex}
func (p *correlationProcessor) generateFromTraceID(traceID pcommon.TraceID) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	// Use first 4 bytes of trace ID for consistency within a trace
	suffix := hex.EncodeToString(traceID[:4])
	return fmt.Sprintf("%s-%s-%s", p.config.IDPrefix, ts, suffix)
}

// generateNew creates a completely new correlation ID.
// Format: {prefix}-{timestamp_base36}-{random_8hex}
func (p *correlationProcessor) generateNew() string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%s-%s-%s", p.config.IDPrefix, ts, hex.EncodeToString(random))
}
