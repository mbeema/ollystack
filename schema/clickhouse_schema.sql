-- OllyStack ClickHouse Schema
-- POC Version with correlation_id support
-- Date: 2026-02-03

-- Create database
CREATE DATABASE IF NOT EXISTS ollystack;

-- ============================================================================
-- TRACES TABLE (with correlation_id)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.traces;

CREATE TABLE ollystack.traces (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Duration Int64 CODEC(ZSTD(1)),
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    `Events.Timestamp` Array(DateTime64(9)) CODEC(ZSTD(1)),
    `Events.Name` Array(LowCardinality(String)) CODEC(ZSTD(1)),
    `Events.Attributes` Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    `Links.TraceId` Array(String) CODEC(ZSTD(1)),
    `Links.SpanId` Array(String) CODEC(ZSTD(1)),
    `Links.TraceState` Array(String) CODEC(ZSTD(1)),
    `Links.Attributes` Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    -- OllyStack additions
    correlation_id String DEFAULT '' CODEC(ZSTD(1)),
    INDEX idx_trace_id TraceId TYPE bloom_filter GRANULARITY 3,
    INDEX idx_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3,
    INDEX idx_service_name ServiceName TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, toUnixTimestamp(Timestamp), TraceId)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- ============================================================================
-- LOGS TABLE (with correlation_id)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.logs;

CREATE TABLE ollystack.logs (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt32 CODEC(ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber Int32 CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(1)),
    ResourceSchemaUrl String CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeSchemaUrl String CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    -- OllyStack additions
    correlation_id String DEFAULT '' CODEC(ZSTD(1)),
    INDEX idx_trace_id TraceId TYPE bloom_filter GRANULARITY 3,
    INDEX idx_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3,
    INDEX idx_service_name ServiceName TYPE bloom_filter GRANULARITY 3,
    INDEX idx_severity SeverityText TYPE set(0) GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SeverityText, toUnixTimestamp(Timestamp))
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- ============================================================================
-- METRICS TABLES (with correlation_id / exemplars support)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.metrics_gauge;

CREATE TABLE ollystack.metrics_gauge (
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceSchemaUrl String CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
    ScopeSchemaUrl String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit String CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    StartTimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    Flags UInt32 CODEC(ZSTD(1)),
    -- Exemplar support for trace correlation
    `Exemplars.FilteredAttributes` Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    `Exemplars.TimeUnix` Array(DateTime64(9)) CODEC(ZSTD(1)),
    `Exemplars.Value` Array(Float64) CODEC(ZSTD(1)),
    `Exemplars.SpanId` Array(String) CODEC(ZSTD(1)),
    `Exemplars.TraceId` Array(String) CODEC(ZSTD(1)),
    INDEX idx_service_name ServiceName TYPE bloom_filter GRANULARITY 3,
    INDEX idx_metric_name MetricName TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, Attributes, toUnixTimestamp(TimeUnix))
TTL toDateTime(TimeUnix) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

DROP TABLE IF EXISTS ollystack.metrics_sum;

CREATE TABLE ollystack.metrics_sum (
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceSchemaUrl String CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
    ScopeSchemaUrl String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit String CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    StartTimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    Flags UInt32 CODEC(ZSTD(1)),
    AggregationTemporality Int32 CODEC(ZSTD(1)),
    IsMonotonic Bool CODEC(ZSTD(1)),
    -- Exemplar support
    `Exemplars.FilteredAttributes` Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    `Exemplars.TimeUnix` Array(DateTime64(9)) CODEC(ZSTD(1)),
    `Exemplars.Value` Array(Float64) CODEC(ZSTD(1)),
    `Exemplars.SpanId` Array(String) CODEC(ZSTD(1)),
    `Exemplars.TraceId` Array(String) CODEC(ZSTD(1)),
    INDEX idx_service_name ServiceName TYPE bloom_filter GRANULARITY 3,
    INDEX idx_metric_name MetricName TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, Attributes, toUnixTimestamp(TimeUnix))
TTL toDateTime(TimeUnix) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

DROP TABLE IF EXISTS ollystack.metrics_histogram;

CREATE TABLE ollystack.metrics_histogram (
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceSchemaUrl String CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
    ScopeSchemaUrl String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit String CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    StartTimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    Count UInt64 CODEC(ZSTD(1)),
    Sum Float64 CODEC(ZSTD(1)),
    BucketCounts Array(UInt64) CODEC(ZSTD(1)),
    ExplicitBounds Array(Float64) CODEC(ZSTD(1)),
    Min Float64 CODEC(ZSTD(1)),
    Max Float64 CODEC(ZSTD(1)),
    Flags UInt32 CODEC(ZSTD(1)),
    AggregationTemporality Int32 CODEC(ZSTD(1)),
    -- Exemplar support
    `Exemplars.FilteredAttributes` Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    `Exemplars.TimeUnix` Array(DateTime64(9)) CODEC(ZSTD(1)),
    `Exemplars.Value` Array(Float64) CODEC(ZSTD(1)),
    `Exemplars.SpanId` Array(String) CODEC(ZSTD(1)),
    `Exemplars.TraceId` Array(String) CODEC(ZSTD(1)),
    INDEX idx_service_name ServiceName TYPE bloom_filter GRANULARITY 3,
    INDEX idx_metric_name MetricName TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, Attributes, toUnixTimestamp(TimeUnix))
TTL toDateTime(TimeUnix) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- ============================================================================
-- CORRELATION SUMMARY (Materialized View for fast correlation lookups)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.correlation_summary;

CREATE TABLE ollystack.correlation_summary (
    correlation_id String,
    window_start DateTime,
    first_seen DateTime64(9),
    last_seen DateTime64(9),
    services Array(String),
    span_count UInt64,
    error_count UInt64,
    avg_duration Float64,
    max_duration Int64,
    root_service String,
    has_errors UInt8
) ENGINE = SummingMergeTree()
PARTITION BY toDate(window_start)
ORDER BY (correlation_id, window_start)
TTL window_start + INTERVAL 30 DAY;

-- Materialized view to populate correlation_summary from traces
DROP VIEW IF EXISTS ollystack.correlation_summary_mv;

CREATE MATERIALIZED VIEW ollystack.correlation_summary_mv
TO ollystack.correlation_summary
AS SELECT
    correlation_id,
    toStartOfMinute(Timestamp) as window_start,
    min(Timestamp) as first_seen,
    max(Timestamp) as last_seen,
    groupUniqArray(ServiceName) as services,
    count() as span_count,
    countIf(StatusCode = 'ERROR') as error_count,
    avg(Duration) as avg_duration,
    max(Duration) as max_duration,
    argMin(ServiceName, Timestamp) as root_service,
    if(countIf(StatusCode = 'ERROR') > 0, 1, 0) as has_errors
FROM ollystack.traces
WHERE correlation_id != ''
GROUP BY correlation_id, toStartOfMinute(Timestamp);

-- ============================================================================
-- METRIC EXEMPLARS TABLE (for linking metrics to traces)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.metric_exemplars;

CREATE TABLE ollystack.metric_exemplars (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    correlation_id String CODEC(ZSTD(1)),
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    INDEX idx_trace_id TraceId TYPE bloom_filter GRANULARITY 3,
    INDEX idx_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3,
    INDEX idx_metric_name MetricName TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, MetricName, toUnixTimestamp(Timestamp))
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- TRACE ID TO TIMESTAMP LOOKUP (for efficient trace retrieval)
-- ============================================================================
DROP TABLE IF EXISTS ollystack.traces_trace_id_ts;

CREATE TABLE ollystack.traces_trace_id_ts (
    TraceId String CODEC(ZSTD(1)),
    Start DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    End DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    correlation_id String CODEC(ZSTD(1)),
    INDEX idx_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3
) ENGINE = MergeTree()
PARTITION BY toDate(Start)
ORDER BY (TraceId, toUnixTimestamp(Start))
TTL toDateTime(Start) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

DROP VIEW IF EXISTS ollystack.traces_trace_id_ts_mv;

CREATE MATERIALIZED VIEW ollystack.traces_trace_id_ts_mv
TO ollystack.traces_trace_id_ts
AS SELECT
    TraceId,
    min(Timestamp) as Start,
    max(Timestamp) as End,
    any(correlation_id) as correlation_id
FROM ollystack.traces
WHERE TraceId != ''
GROUP BY TraceId;

-- ============================================================================
-- AI/ML TABLES
-- ============================================================================

-- Anomaly baselines per service/metric
DROP TABLE IF EXISTS ollystack.anomaly_baselines;

CREATE TABLE ollystack.anomaly_baselines (
    service_name LowCardinality(String),
    metric_name LowCardinality(String),
    hour_of_day UInt8,
    day_of_week UInt8,
    mean Float64,
    std Float64,
    p50 Float64,
    p95 Float64,
    p99 Float64,
    sample_count UInt64,
    updated_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (service_name, metric_name, hour_of_day, day_of_week);

-- Resolved incidents for similar incident detection
DROP TABLE IF EXISTS ollystack.resolved_incidents;

CREATE TABLE ollystack.resolved_incidents (
    id UUID DEFAULT generateUUIDv4(),
    correlation_id String,
    service_name LowCardinality(String),
    error_type String,
    error_message String,
    root_cause String,
    resolution String,
    symptoms Array(String),
    affected_services Array(String),
    duration_minutes UInt32,
    created_at DateTime DEFAULT now(),
    resolved_at DateTime,
    -- Embedding for similarity search (stored as array of floats)
    embedding Array(Float32)
) ENGINE = MergeTree()
ORDER BY (service_name, created_at)
TTL created_at + INTERVAL 365 DAY;

-- ============================================================================
-- VERIFICATION QUERIES
-- ============================================================================
-- Run these to verify schema is correct:
-- SELECT name, type FROM system.columns WHERE database = 'ollystack' AND table = 'traces';
-- SELECT name, type FROM system.columns WHERE database = 'ollystack' AND table = 'logs';
-- SELECT name FROM system.tables WHERE database = 'ollystack';
