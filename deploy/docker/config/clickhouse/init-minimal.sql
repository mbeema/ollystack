-- OllyStack Minimal Schema
--
-- ClickHouse handles both storage AND real-time analytics
-- Materialized Views replace the stream processor
--
-- Tables:
-- 1. Raw data tables (metrics, logs, traces)
-- 2. Materialized Views for real-time aggregations
-- 3. Alert tables populated by MVs

-- ============================================================================
-- RAW DATA TABLES
-- ============================================================================

-- Metrics (OTLP format)
CREATE TABLE IF NOT EXISTS otel_metrics (
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    MetricName LowCardinality(String),
    MetricType LowCardinality(String),  -- gauge, counter, histogram
    Value Float64,
    Labels Map(LowCardinality(String), String),
    -- Common labels extracted for fast filtering
    ServiceName LowCardinality(String) DEFAULT Labels['service.name'],
    Host LowCardinality(String) DEFAULT Labels['host'],
    Environment LowCardinality(String) DEFAULT Labels['environment']
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, MetricName, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Logs (OTLP format)
CREATE TABLE IF NOT EXISTS otel_logs (
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    SeverityText LowCardinality(String),
    SeverityNumber UInt8,
    Body String CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String),
    -- Common attributes extracted
    ServiceName LowCardinality(String) DEFAULT Attributes['service.name'],
    Host LowCardinality(String) DEFAULT Attributes['host'],
    -- For deduplication tracking
    PatternHash String CODEC(ZSTD(1)),
    OccurrenceCount UInt32 DEFAULT 1,
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_body Body TYPE tokenbf_v1(10240, 3, 0) GRANULARITY 4
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, SeverityNumber, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 14 DAY
SETTINGS index_granularity = 8192;

-- Traces/Spans (OTLP format)
CREATE TABLE IF NOT EXISTS otel_traces (
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    SpanName LowCardinality(String),
    SpanKind LowCardinality(String),
    ServiceName LowCardinality(String),
    Duration Int64,  -- nanoseconds
    StatusCode LowCardinality(String),
    StatusMessage String CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String),
    Events Nested (
        Timestamp DateTime64(9),
        Name LowCardinality(String),
        Attributes Map(LowCardinality(String), String)
    ),
    -- Extracted for fast queries
    HttpMethod LowCardinality(String) DEFAULT Attributes['http.method'],
    HttpStatusCode UInt16 DEFAULT toUInt16OrZero(Attributes['http.status_code']),
    HttpUrl String DEFAULT Attributes['http.url'],
    DbSystem LowCardinality(String) DEFAULT Attributes['db.system'],
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- MATERIALIZED VIEWS - Replace Stream Processor
-- ============================================================================

-- 1. Service Topology (auto-discovered from traces)
CREATE TABLE IF NOT EXISTS service_topology (
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    SourceService LowCardinality(String),
    TargetService LowCardinality(String),
    CallCount UInt64,
    ErrorCount UInt64,
    AvgDurationMs Float64,
    P95DurationMs Float64
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (SourceService, TargetService, Timestamp)
TTL Timestamp + INTERVAL 30 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_service_topology
TO service_topology AS
SELECT
    toStartOfMinute(Timestamp) as Timestamp,
    ServiceName as SourceService,
    coalesce(Attributes['peer.service'], Attributes['net.peer.name'], 'unknown') as TargetService,
    count() as CallCount,
    countIf(StatusCode = 'ERROR') as ErrorCount,
    avg(Duration / 1000000) as AvgDurationMs,
    quantile(0.95)(Duration / 1000000) as P95DurationMs
FROM otel_traces
WHERE SpanKind IN ('CLIENT', 'PRODUCER')
GROUP BY Timestamp, SourceService, TargetService;

-- 2. Service Metrics (RED metrics per service)
CREATE TABLE IF NOT EXISTS service_metrics (
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    ServiceName LowCardinality(String),
    SpanName LowCardinality(String),
    RequestCount UInt64,
    ErrorCount UInt64,
    AvgDurationMs Float64,
    P50DurationMs Float64,
    P95DurationMs Float64,
    P99DurationMs Float64
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp)
TTL Timestamp + INTERVAL 30 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_service_metrics
TO service_metrics AS
SELECT
    toStartOfMinute(Timestamp) as Timestamp,
    ServiceName,
    SpanName,
    count() as RequestCount,
    countIf(StatusCode = 'ERROR') as ErrorCount,
    avg(Duration / 1000000) as AvgDurationMs,
    quantile(0.50)(Duration / 1000000) as P50DurationMs,
    quantile(0.95)(Duration / 1000000) as P95DurationMs,
    quantile(0.99)(Duration / 1000000) as P99DurationMs
FROM otel_traces
WHERE SpanKind = 'SERVER'
GROUP BY Timestamp, ServiceName, SpanName;

-- 3. Error Aggregation (for alerting)
CREATE TABLE IF NOT EXISTS error_aggregates (
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    ServiceName LowCardinality(String),
    ErrorType LowCardinality(String),
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorCount UInt64,
    FirstSeen DateTime,
    LastSeen DateTime,
    SampleTraceId String
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, ErrorType, Timestamp)
TTL Timestamp + INTERVAL 7 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_error_aggregates
TO error_aggregates AS
SELECT
    toStartOfMinute(Timestamp) as Timestamp,
    ServiceName,
    coalesce(Attributes['exception.type'], StatusCode) as ErrorType,
    coalesce(StatusMessage, Attributes['exception.message'], '') as ErrorMessage,
    count() as ErrorCount,
    min(Timestamp) as FirstSeen,
    max(Timestamp) as LastSeen,
    any(TraceId) as SampleTraceId
FROM otel_traces
WHERE StatusCode = 'ERROR'
GROUP BY Timestamp, ServiceName, ErrorType, ErrorMessage;

-- 4. Metric Anomalies (Z-score based)
CREATE TABLE IF NOT EXISTS metric_anomalies (
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    ServiceName LowCardinality(String),
    MetricName LowCardinality(String),
    CurrentValue Float64,
    AvgValue Float64,
    StdDev Float64,
    ZScore Float64,
    Severity LowCardinality(String)
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, MetricName, Timestamp)
TTL Timestamp + INTERVAL 7 DAY;

-- Note: This view detects anomalies by comparing recent values to historical baseline
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_metric_anomalies
TO metric_anomalies AS
WITH
    baseline AS (
        SELECT
            ServiceName,
            MetricName,
            avg(Value) as AvgValue,
            stddevPop(Value) as StdDev
        FROM otel_metrics
        WHERE Timestamp > now() - INTERVAL 1 HOUR
          AND Timestamp < now() - INTERVAL 5 MINUTE
        GROUP BY ServiceName, MetricName
    ),
    current AS (
        SELECT
            toStartOfMinute(Timestamp) as Timestamp,
            ServiceName,
            MetricName,
            avg(Value) as CurrentValue
        FROM otel_metrics
        WHERE Timestamp > now() - INTERVAL 5 MINUTE
        GROUP BY Timestamp, ServiceName, MetricName
    )
SELECT
    c.Timestamp,
    c.ServiceName,
    c.MetricName,
    c.CurrentValue,
    b.AvgValue,
    b.StdDev,
    if(b.StdDev > 0, (c.CurrentValue - b.AvgValue) / b.StdDev, 0) as ZScore,
    multiIf(
        abs(ZScore) > 4, 'CRITICAL',
        abs(ZScore) > 3, 'WARNING',
        'NORMAL'
    ) as Severity
FROM current c
JOIN baseline b ON c.ServiceName = b.ServiceName AND c.MetricName = b.MetricName
WHERE abs(ZScore) > 3;

-- 5. Log Pattern Aggregation
CREATE TABLE IF NOT EXISTS log_patterns (
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    ServiceName LowCardinality(String),
    SeverityText LowCardinality(String),
    PatternHash String,
    PatternTemplate String CODEC(ZSTD(1)),
    OccurrenceCount UInt64,
    SampleMessage String CODEC(ZSTD(1))
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (ServiceName, SeverityText, PatternHash, Timestamp)
TTL Timestamp + INTERVAL 7 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_log_patterns
TO log_patterns AS
SELECT
    toStartOfMinute(Timestamp) as Timestamp,
    ServiceName,
    SeverityText,
    PatternHash,
    any(Body) as PatternTemplate,
    sum(OccurrenceCount) as OccurrenceCount,
    any(Body) as SampleMessage
FROM otel_logs
WHERE PatternHash != ''
GROUP BY Timestamp, ServiceName, SeverityText, PatternHash;

-- ============================================================================
-- ALERT RULES TABLE (queried by alerting service)
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_rules (
    Id UUID DEFAULT generateUUIDv4(),
    Name String,
    Query String,
    Threshold Float64,
    Operator LowCardinality(String),  -- gt, lt, eq
    Severity LowCardinality(String),
    Labels Map(String, String),
    Enabled Bool DEFAULT true,
    CreatedAt DateTime DEFAULT now(),
    UpdatedAt DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (Name, Id);

-- Insert default alert rules
INSERT INTO alert_rules (Name, Query, Threshold, Operator, Severity, Labels) VALUES
    ('High Error Rate',
     'SELECT ServiceName, ErrorCount / RequestCount * 100 as ErrorRate FROM service_metrics WHERE Timestamp > now() - INTERVAL 5 MINUTE GROUP BY ServiceName HAVING ErrorRate > {threshold}',
     5.0, 'gt', 'warning', {'type': 'error_rate'}),
    ('High Latency P95',
     'SELECT ServiceName, P95DurationMs FROM service_metrics WHERE Timestamp > now() - INTERVAL 5 MINUTE AND P95DurationMs > {threshold}',
     1000.0, 'gt', 'warning', {'type': 'latency'}),
    ('Anomaly Detected',
     'SELECT * FROM metric_anomalies WHERE Timestamp > now() - INTERVAL 5 MINUTE AND Severity = ''CRITICAL''',
     0, 'gt', 'critical', {'type': 'anomaly'});

-- ============================================================================
-- VIEWS FOR QUERYING
-- ============================================================================

-- Recent errors view
CREATE VIEW IF NOT EXISTS v_recent_errors AS
SELECT
    Timestamp,
    ServiceName,
    SpanName,
    StatusMessage,
    TraceId,
    Duration / 1000000 as DurationMs
FROM otel_traces
WHERE StatusCode = 'ERROR'
  AND Timestamp > now() - INTERVAL 1 HOUR
ORDER BY Timestamp DESC
LIMIT 100;

-- Service health view
CREATE VIEW IF NOT EXISTS v_service_health AS
SELECT
    ServiceName,
    sum(RequestCount) as TotalRequests,
    sum(ErrorCount) as TotalErrors,
    sum(ErrorCount) / sum(RequestCount) * 100 as ErrorRate,
    avg(AvgDurationMs) as AvgLatency,
    max(P99DurationMs) as MaxP99Latency
FROM service_metrics
WHERE Timestamp > now() - INTERVAL 5 MINUTE
GROUP BY ServiceName
ORDER BY ErrorRate DESC;

-- Topology summary view
CREATE VIEW IF NOT EXISTS v_topology_summary AS
SELECT
    SourceService,
    TargetService,
    sum(CallCount) as TotalCalls,
    sum(ErrorCount) as TotalErrors,
    avg(AvgDurationMs) as AvgLatency
FROM service_topology
WHERE Timestamp > now() - INTERVAL 1 HOUR
GROUP BY SourceService, TargetService
ORDER BY TotalCalls DESC;
