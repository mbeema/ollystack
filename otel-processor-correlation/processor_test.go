// Copyright 2026 OllyStack
// SPDX-License-Identifier: Apache-2.0

package correlationprocessor

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func TestProcessTraces_GeneratesCorrelationID(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	// Create test trace data
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetName("test-span")

	// Process
	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("ProcessTraces failed: %v", err)
	}

	// Verify correlation_id was added
	resultSpan := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	corrID, ok := resultSpan.Attributes().Get("correlation_id")
	if !ok {
		t.Fatal("correlation_id attribute not found on span")
	}

	if !strings.HasPrefix(corrID.Str(), "olly-") {
		t.Errorf("Expected correlation_id to start with 'olly-', got: %s", corrID.Str())
	}

	// Verify propagated to resource
	resourceCorrID, ok := result.ResourceSpans().At(0).Resource().Attributes().Get("correlation_id")
	if !ok {
		t.Fatal("correlation_id not propagated to resource")
	}

	if corrID.Str() != resourceCorrID.Str() {
		t.Errorf("Resource and span correlation_id mismatch: %s vs %s", resourceCorrID.Str(), corrID.Str())
	}
}

func TestProcessTraces_ExtractsExistingCorrelationID(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	// Create test trace data with existing correlation_id
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetName("test-span")
	span.Attributes().PutStr("correlation_id", "existing-corr-123")

	// Process
	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("ProcessTraces failed: %v", err)
	}

	// Verify existing correlation_id was preserved
	resultSpan := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	corrID, _ := resultSpan.Attributes().Get("correlation_id")

	if corrID.Str() != "existing-corr-123" {
		t.Errorf("Expected existing correlation_id to be preserved, got: %s", corrID.Str())
	}
}

func TestProcessTraces_ExtractsFromHeader(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	// Create test trace data with X-Correlation-ID header
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetName("test-span")
	span.Attributes().PutStr("X-Correlation-ID", "header-corr-456")

	// Process
	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("ProcessTraces failed: %v", err)
	}

	// Verify correlation_id was extracted from header
	resultSpan := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	corrID, _ := resultSpan.Attributes().Get("correlation_id")

	if corrID.Str() != "header-corr-456" {
		t.Errorf("Expected correlation_id from header, got: %s", corrID.Str())
	}
}

func TestProcessLogs_GeneratesCorrelationID(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	// Create test log data
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	logRecord := sl.LogRecords().AppendEmpty()
	logRecord.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	logRecord.Body().SetStr("test log message")

	// Process
	result, err := proc.ProcessLogs(context.Background(), ld)
	if err != nil {
		t.Fatalf("ProcessLogs failed: %v", err)
	}

	// Verify correlation_id was added
	resultLog := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	corrID, ok := resultLog.Attributes().Get("correlation_id")
	if !ok {
		t.Fatal("correlation_id attribute not found on log")
	}

	if !strings.HasPrefix(corrID.Str(), "olly-") {
		t.Errorf("Expected correlation_id to start with 'olly-', got: %s", corrID.Str())
	}
}

func TestProcessLogs_ExtractsFromJSONBody(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	// Create test log data with JSON body containing correlation_id
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	logRecord := sl.LogRecords().AppendEmpty()
	body := logRecord.Body().SetEmptyMap()
	body.PutStr("correlation_id", "json-corr-789")
	body.PutStr("message", "test log")

	// Process
	result, err := proc.ProcessLogs(context.Background(), ld)
	if err != nil {
		t.Fatalf("ProcessLogs failed: %v", err)
	}

	// Verify correlation_id was extracted from body
	resultLog := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	corrID, _ := resultLog.Attributes().Get("correlation_id")

	if corrID.Str() != "json-corr-789" {
		t.Errorf("Expected correlation_id from JSON body, got: %s", corrID.Str())
	}
}

func TestGenerateFromTraceID_Consistency(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	proc := newCorrelationProcessor(cfg, zap.NewNop())

	traceID := pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})

	// Generate multiple times - suffix should be consistent (same trace ID prefix)
	id1 := proc.generateFromTraceID(traceID)
	id2 := proc.generateFromTraceID(traceID)

	// Both should have same suffix (derived from trace ID)
	parts1 := strings.Split(id1, "-")
	parts2 := strings.Split(id2, "-")

	if parts1[2] != parts2[2] {
		t.Errorf("Suffix should be consistent for same trace ID: %s vs %s", parts1[2], parts2[2])
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			cfg:     createDefaultConfig().(*Config),
			wantErr: false,
		},
		{
			name: "empty attribute_name",
			cfg: &Config{
				IDPrefix:      "olly",
				AttributeName: "",
			},
			wantErr: true,
		},
		{
			name: "empty id_prefix",
			cfg: &Config{
				IDPrefix:      "",
				AttributeName: "correlation_id",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
