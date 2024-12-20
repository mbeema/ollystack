// Package types defines core telemetry data types
package types

import "time"

// MetricType represents the type of metric
type MetricType int

const (
	MetricTypeGauge MetricType = iota
	MetricTypeCounter
	MetricTypeHistogram
	MetricTypeSummary
)

// Metric represents a single metric data point
type Metric struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
	Type      MetricType
	Unit      string
}

// Severity represents log severity level
type Severity int

const (
	SeverityUnspecified Severity = iota
	SeverityDebug
	SeverityInfo
	SeverityWarn
	SeverityError
	SeverityFatal
)

func (s Severity) String() string {
	switch s {
	case SeverityDebug:
		return "DEBUG"
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	case SeverityFatal:
		return "FATAL"
	default:
		return "UNSPECIFIED"
	}
}

// LogRecord represents a single log entry
type LogRecord struct {
	Timestamp  time.Time
	Body       string
	Severity   Severity
	Service    string
	TraceID    string
	SpanID     string
	Attributes map[string]string
}

// SpanStatus represents span status
type SpanStatus int

const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

// SpanKind represents the type of span
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindInternal
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// Span represents a trace span
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	Kind         SpanKind
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Status       SpanStatus
	StatusMessage string
	Service      string
	Attributes   map[string]string
	Events       []SpanEvent
}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]string
}
