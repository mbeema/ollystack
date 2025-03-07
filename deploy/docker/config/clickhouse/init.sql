-- OllyStack ClickHouse Schema
-- Optimized for OpenTelemetry data with time-series queries

-- Create database
CREATE DATABASE IF NOT EXISTS ollystack;

USE ollystack;

-- =============================================================================
-- TRACES TABLE
-- Stores OpenTelemetry spans with optimized indexing for trace queries
-- =============================================================================

CREATE TABLE IF NOT EXISTS otel_traces
(
    -- Core identifiers
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),

    -- Span info
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),

    -- Service info
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ServiceNamespace LowCardinality(String) CODEC(ZSTD(1)),
    ServiceInstanceId String CODEC(ZSTD(1)),

    -- Timing
    Duration Int64 CODEC(Delta, ZSTD(1)),
    StartTime DateTime64(9) CODEC(Delta, ZSTD(1)),
    EndTime DateTime64(9) CODEC(Delta, ZSTD(1)),

    -- Status
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),

    -- Attributes (as nested for flexibility)
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Events
    Events Nested(
        Timestamp DateTime64(9),
        Name LowCardinality(String),
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),

    -- Links
    Links Nested(
        TraceId String,
        SpanId String,
        TraceState String,
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),

    -- HTTP attributes (denormalized for fast queries)
    HttpMethod LowCardinality(String) CODEC(ZSTD(1)),
    HttpUrl String CODEC(ZSTD(1)),
    HttpStatusCode Int32 CODEC(Delta, ZSTD(1)),
    HttpRoute LowCardinality(String) CODEC(ZSTD(1)),

    -- Database attributes
    DbSystem LowCardinality(String) CODEC(ZSTD(1)),
    DbName LowCardinality(String) CODEC(ZSTD(1)),
    DbStatement String CODEC(ZSTD(1)),

    -- RPC attributes
    RpcSystem LowCardinality(String) CODEC(ZSTD(1)),
    RpcService LowCardinality(String) CODEC(ZSTD(1)),
    RpcMethod LowCardinality(String) CODEC(ZSTD(1)),

    -- Infrastructure
    HostName LowCardinality(String) CODEC(ZSTD(1)),
    ContainerName LowCardinality(String) CODEC(ZSTD(1)),
    K8sPodName String CODEC(ZSTD(1)),
    K8sNamespace LowCardinality(String) CODEC(ZSTD(1)),
    K8sDeployment LowCardinality(String) CODEC(ZSTD(1)),

    -- OllyStack enrichments
    IsError UInt8 DEFAULT 0,
    IsRootSpan UInt8 DEFAULT 0,
    AnomalyScore Float32 DEFAULT 0
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp, TraceId)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Secondary indexes for common queries
ALTER TABLE otel_traces ADD INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE otel_traces ADD INDEX idx_status_code StatusCode TYPE set(0) GRANULARITY 1;
ALTER TABLE otel_traces ADD INDEX idx_http_status HttpStatusCode TYPE minmax GRANULARITY 1;
ALTER TABLE otel_traces ADD INDEX idx_duration Duration TYPE minmax GRANULARITY 1;

-- =============================================================================
-- METRICS TABLE
-- Stores OpenTelemetry metrics with efficient time-series compression
-- =============================================================================

CREATE TABLE IF NOT EXISTS otel_metrics
(
    -- Core identifiers
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit LowCardinality(String) CODEC(ZSTD(1)),

    -- Metric type and aggregation
    MetricType LowCardinality(String) CODEC(ZSTD(1)), -- gauge, counter, histogram, summary
    AggregationTemporality LowCardinality(String) CODEC(ZSTD(1)),
    IsMonotonic UInt8 CODEC(ZSTD(1)),

    -- Service info
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ServiceNamespace LowCardinality(String) CODEC(ZSTD(1)),
    ServiceInstanceId String CODEC(ZSTD(1)),

    -- Values (support different metric types)
    Value Float64 CODEC(Gorilla, ZSTD(1)),
    ValueInt Int64 CODEC(Delta, ZSTD(1)),

    -- Histogram specific
    HistogramCount UInt64 CODEC(Delta, ZSTD(1)),
    HistogramSum Float64 CODEC(Gorilla, ZSTD(1)),
    HistogramMin Float64 CODEC(Gorilla, ZSTD(1)),
    HistogramMax Float64 CODEC(Gorilla, ZSTD(1)),
    HistogramBuckets Array(Float64) CODEC(ZSTD(1)),
    HistogramBucketCounts Array(UInt64) CODEC(ZSTD(1)),

    -- Exemplars (link to traces)
    ExemplarTraceId String CODEC(ZSTD(1)),
    ExemplarSpanId String CODEC(ZSTD(1)),
    ExemplarValue Float64 CODEC(Gorilla, ZSTD(1)),

    -- Attributes
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Infrastructure
    HostName LowCardinality(String) CODEC(ZSTD(1)),
    ContainerName LowCardinality(String) CODEC(ZSTD(1)),
    K8sPodName String CODEC(ZSTD(1)),
    K8sNamespace LowCardinality(String) CODEC(ZSTD(1)),

    -- OllyStack enrichments
    AnomalyScore Float32 DEFAULT 0,
    Baseline Float64 DEFAULT 0,
    UpperBound Float64 DEFAULT 0,
    LowerBound Float64 DEFAULT 0
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, MetricName, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Secondary indexes
ALTER TABLE otel_metrics ADD INDEX idx_metric_name MetricName TYPE set(0) GRANULARITY 1;
ALTER TABLE otel_metrics ADD INDEX idx_value Value TYPE minmax GRANULARITY 1;

-- =============================================================================
-- LOGS TABLE
-- Stores OpenTelemetry logs with full-text search capability
-- =============================================================================

CREATE TABLE IF NOT EXISTS otel_logs
(
    -- Core identifiers
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    ObservedTimestamp DateTime64(9) CODEC(Delta, ZSTD(1)),

    -- Trace context (for correlation)
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt8 CODEC(ZSTD(1)),

    -- Severity
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 CODEC(ZSTD(1)),

    -- Log content
    Body String CODEC(ZSTD(1)),

    -- Service info
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ServiceNamespace LowCardinality(String) CODEC(ZSTD(1)),
    ServiceInstanceId String CODEC(ZSTD(1)),

    -- Attributes
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Structured log fields (extracted from JSON)
    LogLevel LowCardinality(String) CODEC(ZSTD(1)),
    Logger LowCardinality(String) CODEC(ZSTD(1)),
    Caller String CODEC(ZSTD(1)),
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorStack String CODEC(ZSTD(1)),

    -- Infrastructure
    HostName LowCardinality(String) CODEC(ZSTD(1)),
    ContainerName LowCardinality(String) CODEC(ZSTD(1)),
    ContainerId String CODEC(ZSTD(1)),
    K8sPodName String CODEC(ZSTD(1)),
    K8sNamespace LowCardinality(String) CODEC(ZSTD(1)),

    -- Source
    LogFilePath String CODEC(ZSTD(1)),

    -- Full-text search tokens (for fast searching)
    BodyTokens Array(String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SeverityNumber, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Secondary indexes
ALTER TABLE otel_logs ADD INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE otel_logs ADD INDEX idx_severity SeverityText TYPE set(0) GRANULARITY 1;
ALTER TABLE otel_logs ADD INDEX idx_body Body TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;

-- =============================================================================
-- SERVICE TOPOLOGY TABLE
-- Stores service dependency graph for service map visualization
-- =============================================================================

CREATE TABLE IF NOT EXISTS service_topology
(
    -- Time window
    Timestamp DateTime CODEC(Delta, ZSTD(1)),
    WindowStart DateTime CODEC(Delta, ZSTD(1)),
    WindowEnd DateTime CODEC(Delta, ZSTD(1)),

    -- Source service
    SourceService LowCardinality(String) CODEC(ZSTD(1)),
    SourceNamespace LowCardinality(String) CODEC(ZSTD(1)),
    SourceOperation LowCardinality(String) CODEC(ZSTD(1)),

    -- Target service
    TargetService LowCardinality(String) CODEC(ZSTD(1)),
    TargetNamespace LowCardinality(String) CODEC(ZSTD(1)),
    TargetOperation LowCardinality(String) CODEC(ZSTD(1)),

    -- Connection type
    ConnectionType LowCardinality(String) CODEC(ZSTD(1)), -- http, grpc, database, messaging
    Protocol LowCardinality(String) CODEC(ZSTD(1)),

    -- Statistics
    RequestCount UInt64 CODEC(Delta, ZSTD(1)),
    ErrorCount UInt64 CODEC(Delta, ZSTD(1)),

    -- Latency percentiles
    LatencyP50 Float64 CODEC(Gorilla, ZSTD(1)),
    LatencyP90 Float64 CODEC(Gorilla, ZSTD(1)),
    LatencyP99 Float64 CODEC(Gorilla, ZSTD(1)),
    LatencyAvg Float64 CODEC(Gorilla, ZSTD(1)),
    LatencySum Float64 CODEC(Gorilla, ZSTD(1)),

    -- Health indicators
    ErrorRate Float32 CODEC(Gorilla, ZSTD(1)),
    AvailabilityScore Float32 CODEC(Gorilla, ZSTD(1))
)
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (Timestamp, SourceService, TargetService, SourceOperation, TargetOperation)
TTL Timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- ALERTS TABLE
-- Stores alert history and status
-- =============================================================================

CREATE TABLE IF NOT EXISTS alerts
(
    -- Alert identifiers
    AlertId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Alert info
    AlertName LowCardinality(String) CODEC(ZSTD(1)),
    AlertType LowCardinality(String) CODEC(ZSTD(1)), -- anomaly, threshold, pattern
    Severity LowCardinality(String) CODEC(ZSTD(1)), -- critical, warning, info
    Status LowCardinality(String) CODEC(ZSTD(1)), -- firing, resolved, acknowledged

    -- Context
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),

    -- Values
    CurrentValue Float64 CODEC(Gorilla, ZSTD(1)),
    ThresholdValue Float64 CODEC(Gorilla, ZSTD(1)),
    AnomalyScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Description
    Summary String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),

    -- Related entities
    RelatedTraceIds Array(String) CODEC(ZSTD(1)),
    RelatedLogIds Array(String) CODEC(ZSTD(1)),

    -- Labels
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Resolution
    ResolvedAt DateTime64(3) CODEC(Delta, ZSTD(1)),
    ResolvedBy String CODEC(ZSTD(1)),
    RootCause String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (Timestamp, AlertName, ServiceName)
TTL Timestamp + INTERVAL 365 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- ANOMALIES TABLE
-- Stores detected anomalies for analysis
-- =============================================================================

CREATE TABLE IF NOT EXISTS anomalies
(
    -- Identifiers
    AnomalyId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Type
    AnomalyType LowCardinality(String) CODEC(ZSTD(1)), -- latency, error_rate, throughput, pattern
    SignalType LowCardinality(String) CODEC(ZSTD(1)), -- trace, metric, log

    -- Context
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    OperationName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),

    -- Detection
    Score Float32 CODEC(Gorilla, ZSTD(1)),
    Confidence Float32 CODEC(Gorilla, ZSTD(1)),
    DetectionMethod LowCardinality(String) CODEC(ZSTD(1)), -- z_score, isolation_forest, lstm

    -- Values
    ObservedValue Float64 CODEC(Gorilla, ZSTD(1)),
    ExpectedValue Float64 CODEC(Gorilla, ZSTD(1)),
    Deviation Float64 CODEC(Gorilla, ZSTD(1)),

    -- Baseline
    BaselineMean Float64 CODEC(Gorilla, ZSTD(1)),
    BaselineStdDev Float64 CODEC(Gorilla, ZSTD(1)),
    BaselineWindow String CODEC(ZSTD(1)),

    -- Related data
    SampleTraceIds Array(String) CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (Timestamp, ServiceName, AnomalyType)
TTL Timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- SEASONAL BASELINES TABLE
-- Stores precomputed seasonal baselines for metrics
-- =============================================================================

CREATE TABLE IF NOT EXISTS seasonal_baselines
(
    -- Identifiers
    BaselineId UUID DEFAULT generateUUIDv4(),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),

    -- Timestamps
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),
    ValidFrom DateTime64(3) CODEC(Delta, ZSTD(1)),
    ValidTo DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Hourly patterns (24 values)
    HourlyMeans Array(Float64) CODEC(ZSTD(1)),
    HourlyStds Array(Float64) CODEC(ZSTD(1)),

    -- Daily patterns (7 values, Monday=0)
    DailyMeans Array(Float64) CODEC(ZSTD(1)),
    DailyStds Array(Float64) CODEC(ZSTD(1)),

    -- Weekly patterns (168 values = 24*7, hour-of-week)
    WeeklyMeans Array(Float64) CODEC(ZSTD(1)),
    WeeklyStds Array(Float64) CODEC(ZSTD(1)),

    -- Global statistics
    GlobalMean Float64 CODEC(Gorilla, ZSTD(1)),
    GlobalStd Float64 CODEC(Gorilla, ZSTD(1)),
    GlobalMin Float64 CODEC(Gorilla, ZSTD(1)),
    GlobalMax Float64 CODEC(Gorilla, ZSTD(1)),

    -- Seasonality detection results
    HasHourlyPattern UInt8 DEFAULT 0,
    HasDailyPattern UInt8 DEFAULT 0,
    HasWeeklyPattern UInt8 DEFAULT 0,
    HourlyStrength Float32 CODEC(Gorilla, ZSTD(1)),
    DailyStrength Float32 CODEC(Gorilla, ZSTD(1)),
    WeeklyStrength Float32 CODEC(Gorilla, ZSTD(1)),
    DominantPeriod LowCardinality(String) CODEC(ZSTD(1)),

    -- Metadata
    SampleCount UInt64 CODEC(Delta, ZSTD(1)),
    BaselineWindow String CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (ServiceName, MetricName, ValidFrom)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- SEASONAL ANOMALIES TABLE
-- Stores anomalies detected with seasonal awareness
-- =============================================================================

CREATE TABLE IF NOT EXISTS seasonal_anomalies
(
    -- Identifiers
    AnomalyId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Context
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),

    -- Time context
    HourOfDay UInt8 CODEC(Delta, ZSTD(1)),
    DayOfWeek UInt8 CODEC(Delta, ZSTD(1)),
    WeekIndex UInt16 CODEC(Delta, ZSTD(1)),  -- Hour-of-week (0-167)

    -- Values
    ObservedValue Float64 CODEC(Gorilla, ZSTD(1)),
    ExpectedValue Float64 CODEC(Gorilla, ZSTD(1)),
    ExpectedStd Float64 CODEC(Gorilla, ZSTD(1)),

    -- Deviation
    DeviationSigma Float64 CODEC(Gorilla, ZSTD(1)),
    AnomalyScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Seasonal context
    HourlyExpected Float64 CODEC(Gorilla, ZSTD(1)),
    DailyExpected Float64 CODEC(Gorilla, ZSTD(1)),
    WeeklyExpected Float64 CODEC(Gorilla, ZSTD(1)),

    -- Contributing factors
    ContributingFactors Array(String) CODEC(ZSTD(1)),

    -- Holiday/event context
    IsHoliday UInt8 DEFAULT 0,
    HolidayName LowCardinality(String) CODEC(ZSTD(1)),

    -- Description
    Description String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (Timestamp, ServiceName, MetricName)
TTL Timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- Index for fast lookups by time context
ALTER TABLE seasonal_anomalies ADD INDEX idx_hour_of_day HourOfDay TYPE minmax GRANULARITY 1;
ALTER TABLE seasonal_anomalies ADD INDEX idx_day_of_week DayOfWeek TYPE set(7) GRANULARITY 1;

-- =============================================================================
-- HOLIDAY CALENDAR TABLE
-- Stores holidays and special events for seasonal adjustment
-- =============================================================================

CREATE TABLE IF NOT EXISTS holiday_calendar
(
    HolidayId UUID DEFAULT generateUUIDv4(),
    Name String CODEC(ZSTD(1)),
    StartDate DateTime64(3) CODEC(Delta, ZSTD(1)),
    EndDate DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Scope
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),  -- Empty = all services
    Region LowCardinality(String) CODEC(ZSTD(1)),       -- For regional holidays

    -- Type
    HolidayType LowCardinality(String) CODEC(ZSTD(1)),  -- public_holiday, company_event, maintenance, campaign

    -- Adjustment
    ThresholdMultiplier Float32 DEFAULT 1.5 CODEC(Gorilla, ZSTD(1)),  -- How much to relax thresholds

    -- Recurrence
    IsRecurring UInt8 DEFAULT 0,
    RecurrencePattern LowCardinality(String) CODEC(ZSTD(1)),  -- yearly, monthly, weekly

    -- Metadata
    CreatedAt DateTime64(3) DEFAULT now64(),
    CreatedBy String CODEC(ZSTD(1)),
    Notes String CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(CreatedAt)
ORDER BY (StartDate, Name)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- MATERIALIZED VIEW: Hourly seasonal aggregation
-- Pre-computes hourly patterns for fast baseline queries
-- =============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS metric_hourly_patterns
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(Date)
ORDER BY (ServiceName, MetricName, HourOfDay, DayOfWeek)
AS SELECT
    toDate(Timestamp) AS Date,
    ServiceName,
    MetricName,
    toHour(Timestamp) AS HourOfDay,
    toDayOfWeek(Timestamp) AS DayOfWeek,
    count() AS SampleCount,
    sum(Value) AS ValueSum,
    sum(Value * Value) AS ValueSumSq,
    min(Value) AS ValueMin,
    max(Value) AS ValueMax
FROM otel_metrics
GROUP BY Date, ServiceName, MetricName, HourOfDay, DayOfWeek;

-- =============================================================================
-- LOG PATTERNS TABLE
-- Stores extracted log patterns/templates
-- =============================================================================

CREATE TABLE IF NOT EXISTS log_patterns
(
    -- Identifiers
    PatternId String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Pattern info
    Template String CODEC(ZSTD(1)),  -- Log template with <*> for variables
    Tokens Array(String) CODEC(ZSTD(1)),  -- Tokenized template
    TokenCount UInt16 CODEC(Delta, ZSTD(1)),

    -- Statistics
    TotalCount UInt64 CODEC(Delta, ZSTD(1)),
    FirstSeen DateTime64(3) CODEC(Delta, ZSTD(1)),
    LastSeen DateTime64(3) CODEC(Delta, ZSTD(1)),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    -- Severity distribution
    InfoCount UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    WarnCount UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    ErrorCount UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    FatalCount UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),

    -- Sample logs (for debugging)
    SampleLogs Array(String) CODEC(ZSTD(1)),

    -- Classification
    IsErrorPattern UInt8 DEFAULT 0,
    IsRarePattern UInt8 DEFAULT 0,
    Category LowCardinality(String) CODEC(ZSTD(1))  -- auth, http, db, system, etc.
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (ServiceName, PatternId)
SETTINGS index_granularity = 8192;

-- Full-text index on template for search
ALTER TABLE log_patterns ADD INDEX idx_template Template TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;

-- =============================================================================
-- LOG PATTERN OCCURRENCES TABLE
-- Stores individual pattern occurrences for frequency analysis
-- =============================================================================

CREATE TABLE IF NOT EXISTS log_pattern_occurrences
(
    -- Time
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Identifiers
    PatternId String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),

    -- Context
    Severity LowCardinality(String) CODEC(ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),

    -- Original message (truncated)
    Message String CODEC(ZSTD(1)),

    -- Extracted variables
    Variables Map(String, String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, PatternId, Timestamp)
TTL Timestamp + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- LOG ANOMALIES TABLE
-- Stores detected log anomalies
-- =============================================================================

CREATE TABLE IF NOT EXISTS log_anomalies
(
    -- Identifiers
    AnomalyId String CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Context
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    PatternId String CODEC(ZSTD(1)),
    PatternTemplate String CODEC(ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),

    -- Anomaly details
    AnomalyType LowCardinality(String) CODEC(ZSTD(1)),  -- new_pattern, frequency_spike, etc.
    Score Float32 CODEC(Gorilla, ZSTD(1)),
    Severity LowCardinality(String) CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),

    -- Original log
    LogMessage String CODEC(ZSTD(1)),

    -- Detection details (JSON)
    Details String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (Timestamp, ServiceName, AnomalyType)
TTL Timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Index for anomaly queries
ALTER TABLE log_anomalies ADD INDEX idx_anomaly_type AnomalyType TYPE set(20) GRANULARITY 1;
ALTER TABLE log_anomalies ADD INDEX idx_score Score TYPE minmax GRANULARITY 1;

-- =============================================================================
-- LOG PATTERN TRANSITIONS TABLE
-- Stores pattern transition statistics for sequence analysis
-- =============================================================================

CREATE TABLE IF NOT EXISTS log_pattern_transitions
(
    -- Identifiers
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    FromPatternId String CODEC(ZSTD(1)),
    ToPatternId String CODEC(ZSTD(1)),

    -- Statistics
    TransitionCount UInt64 CODEC(Delta, ZSTD(1)),
    LastSeen DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Timing statistics
    MeanGapSeconds Float64 CODEC(Gorilla, ZSTD(1)),
    StdGapSeconds Float64 CODEC(Gorilla, ZSTD(1)),
    MinGapSeconds Float64 CODEC(Gorilla, ZSTD(1)),
    MaxGapSeconds Float64 CODEC(Gorilla, ZSTD(1)),

    -- Updated timestamp
    UpdatedAt DateTime64(3) DEFAULT now64()
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (ServiceName, FromPatternId, ToPatternId)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- MATERIALIZED VIEW: Log pattern hourly counts
-- Pre-aggregates pattern counts per hour for frequency analysis
-- =============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS log_pattern_hourly
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(Hour)
ORDER BY (ServiceName, PatternId, Hour)
AS SELECT
    toStartOfHour(Timestamp) AS Hour,
    ServiceName,
    PatternId,
    count() AS OccurrenceCount,
    countIf(Severity = 'ERROR' OR Severity = 'FATAL') AS ErrorCount
FROM log_pattern_occurrences
GROUP BY Hour, ServiceName, PatternId;

-- =============================================================================
-- MATERIALIZED VIEW: Log anomaly summary
-- Daily summary of log anomalies by type
-- =============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS log_anomaly_daily
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(Date)
ORDER BY (ServiceName, Date, AnomalyType)
AS SELECT
    toDate(Timestamp) AS Date,
    ServiceName,
    AnomalyType,
    count() AS AnomalyCount,
    avg(Score) AS AvgScore,
    max(Score) AS MaxScore
FROM log_anomalies
GROUP BY Date, ServiceName, AnomalyType;

-- =============================================================================
-- DASHBOARDS TABLE
-- Stores user-defined dashboards
-- =============================================================================

CREATE TABLE IF NOT EXISTS dashboards
(
    DashboardId UUID DEFAULT generateUUIDv4(),
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    Name String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),

    -- Owner
    CreatedBy String CODEC(ZSTD(1)),
    Organization LowCardinality(String) CODEC(ZSTD(1)),

    -- Dashboard definition (JSON)
    Definition String CODEC(ZSTD(1)),

    -- Metadata
    Tags Array(String) CODEC(ZSTD(1)),
    IsPublic UInt8 DEFAULT 0,
    IsFavorite UInt8 DEFAULT 0
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (DashboardId)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- MATERIALIZED VIEWS FOR AGGREGATIONS
-- =============================================================================

-- Service metrics aggregation (1-minute resolution)
CREATE MATERIALIZED VIEW IF NOT EXISTS service_metrics_1m
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp)
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    count() AS RequestCount,
    countIf(StatusCode = 'ERROR') AS ErrorCount,
    avg(Duration) AS AvgDuration,
    quantile(0.5)(Duration) AS P50Duration,
    quantile(0.90)(Duration) AS P90Duration,
    quantile(0.99)(Duration) AS P99Duration,
    max(Duration) AS MaxDuration,
    min(Duration) AS MinDuration
FROM otel_traces
WHERE IsRootSpan = 1
GROUP BY Timestamp, ServiceName;

-- Error rate by service (5-minute resolution)
CREATE MATERIALIZED VIEW IF NOT EXISTS error_rates_5m
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp)
AS SELECT
    toStartOfFiveMinutes(Timestamp) AS Timestamp,
    ServiceName,
    count() AS TotalRequests,
    countIf(StatusCode = 'ERROR') AS ErrorRequests,
    countIf(HttpStatusCode >= 500) AS ServerErrors,
    countIf(HttpStatusCode >= 400 AND HttpStatusCode < 500) AS ClientErrors
FROM otel_traces
GROUP BY Timestamp, ServiceName;

-- Log severity counts (1-minute resolution)
CREATE MATERIALIZED VIEW IF NOT EXISTS log_severity_counts_1m
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SeverityText, Timestamp)
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    SeverityText,
    count() AS LogCount
FROM otel_logs
GROUP BY Timestamp, ServiceName, SeverityText;

-- =============================================================================
-- USEFUL VIEWS
-- =============================================================================

-- Complete trace view (for trace detail page)
CREATE VIEW IF NOT EXISTS trace_detail AS
SELECT
    t.TraceId,
    t.SpanId,
    t.ParentSpanId,
    t.SpanName,
    t.ServiceName,
    t.Duration,
    t.StartTime,
    t.EndTime,
    t.StatusCode,
    t.StatusMessage,
    t.SpanAttributes,
    t.HttpMethod,
    t.HttpUrl,
    t.HttpStatusCode,
    t.DbSystem,
    t.DbStatement,
    t.IsError,
    t.IsRootSpan
FROM otel_traces t
ORDER BY t.StartTime;

-- Service health overview
CREATE VIEW IF NOT EXISTS service_health AS
SELECT
    ServiceName,
    count() AS TotalSpans,
    countIf(StatusCode = 'ERROR') AS ErrorSpans,
    round(countIf(StatusCode = 'ERROR') / count() * 100, 2) AS ErrorRate,
    round(avg(Duration) / 1000000, 2) AS AvgLatencyMs,
    round(quantile(0.99)(Duration) / 1000000, 2) AS P99LatencyMs,
    max(Timestamp) AS LastSeen
FROM otel_traces
WHERE Timestamp > now() - INTERVAL 1 HOUR
GROUP BY ServiceName
ORDER BY ErrorRate DESC;

-- =============================================================================
-- SERVICE LEVEL OBJECTIVES (SLOs)
-- Track SLIs and error budgets (AWS CloudWatch Application Signals inspired)
-- =============================================================================

CREATE TABLE IF NOT EXISTS slos
(
    -- Identifiers
    SLOId UUID DEFAULT generateUUIDv4(),
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    -- Basic info
    Name String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    OperationName LowCardinality(String) CODEC(ZSTD(1)),

    -- SLI Definition
    SLIType LowCardinality(String) CODEC(ZSTD(1)), -- latency, error_rate, availability, throughput
    SLIMetricQuery String CODEC(ZSTD(1)), -- SQL query for SLI calculation
    SLIThreshold Float64 CODEC(Gorilla, ZSTD(1)), -- e.g., 200ms for latency
    SLIOperator LowCardinality(String) CODEC(ZSTD(1)), -- lt, gt, lte, gte

    -- SLO Target
    TargetPercentage Float64 CODEC(Gorilla, ZSTD(1)), -- e.g., 99.9
    WindowType LowCardinality(String) CODEC(ZSTD(1)), -- rolling, calendar
    WindowDays UInt16 CODEC(ZSTD(1)), -- e.g., 30
    EvaluationType LowCardinality(String) CODEC(ZSTD(1)), -- period_based, request_based

    -- Burn Rate Alerting
    BurnRateFast Float64 DEFAULT 14.4 CODEC(Gorilla, ZSTD(1)), -- 2% budget in 1 hour
    BurnRateSlow Float64 DEFAULT 6.0 CODEC(Gorilla, ZSTD(1)), -- 5% budget in 6 hours
    AlertEnabled UInt8 DEFAULT 1,
    AlertChannels Array(String) CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) DEFAULT 'active' CODEC(ZSTD(1)), -- active, paused, deleted

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    CreatedBy String CODEC(ZSTD(1)),
    Organization LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (SLOId)
SETTINGS index_granularity = 8192;

-- SLO Measurements (per-minute evaluation)
CREATE TABLE IF NOT EXISTS slo_measurements
(
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),
    SLOId UUID CODEC(ZSTD(1)),

    -- Raw counts
    TotalCount UInt64 CODEC(Delta, ZSTD(1)),
    GoodCount UInt64 CODEC(Delta, ZSTD(1)),
    BadCount UInt64 CODEC(Delta, ZSTD(1)),

    -- SLI value
    SLIValue Float64 CODEC(Gorilla, ZSTD(1)),
    IsGood UInt8 CODEC(ZSTD(1)),

    -- Error budget
    ErrorBudgetTotal Float64 CODEC(Gorilla, ZSTD(1)),
    ErrorBudgetConsumed Float64 CODEC(Gorilla, ZSTD(1)),
    ErrorBudgetRemaining Float64 CODEC(Gorilla, ZSTD(1)),
    ErrorBudgetRemainingPercent Float64 CODEC(Gorilla, ZSTD(1)),

    -- Burn rate
    BurnRate1h Float64 CODEC(Gorilla, ZSTD(1)),
    BurnRate6h Float64 CODEC(Gorilla, ZSTD(1)),
    BurnRate24h Float64 CODEC(Gorilla, ZSTD(1)),

    -- Alerts
    AlertFiring UInt8 DEFAULT 0,
    AlertSeverity LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (SLOId, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- SLO Status Summary (current state per SLO)
CREATE TABLE IF NOT EXISTS slo_status
(
    SLOId UUID CODEC(ZSTD(1)),
    UpdatedAt DateTime64(3) DEFAULT now64() CODEC(Delta, ZSTD(1)),

    -- Current state
    CurrentSLI Float64 CODEC(Gorilla, ZSTD(1)),
    CurrentAttainment Float64 CODEC(Gorilla, ZSTD(1)),
    TargetAttainment Float64 CODEC(Gorilla, ZSTD(1)),

    -- Error budget
    ErrorBudgetRemaining Float64 CODEC(Gorilla, ZSTD(1)),
    ErrorBudgetRemainingPercent Float64 CODEC(Gorilla, ZSTD(1)),
    ProjectedBudgetExhaustion DateTime64(3) CODEC(ZSTD(1)),

    -- Burn rates
    CurrentBurnRate Float64 CODEC(Gorilla, ZSTD(1)),
    BurnRate1h Float64 CODEC(Gorilla, ZSTD(1)),
    BurnRate6h Float64 CODEC(Gorilla, ZSTD(1)),
    BurnRate24h Float64 CODEC(Gorilla, ZSTD(1)),

    -- Alert status
    AlertStatus LowCardinality(String) CODEC(ZSTD(1)), -- ok, warning, critical
    AlertMessage String CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (SLOId)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- SYNTHETICS / CANARY MONITORING
-- Proactive monitoring with scripted tests
-- =============================================================================

CREATE TABLE IF NOT EXISTS synthetics_canaries
(
    CanaryId UUID DEFAULT generateUUIDv4(),
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    -- Basic info
    Name String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),
    CanaryType LowCardinality(String) CODEC(ZSTD(1)), -- api, browser, heartbeat

    -- Configuration
    Script String CODEC(ZSTD(1)), -- Playwright/Puppeteer script or API config
    Schedule String CODEC(ZSTD(1)), -- Cron expression
    TimeoutSeconds UInt32 DEFAULT 30 CODEC(ZSTD(1)),
    Regions Array(String) CODEC(ZSTD(1)), -- Regions to run from

    -- Target
    TargetUrl String CODEC(ZSTD(1)),
    TargetService LowCardinality(String) CODEC(ZSTD(1)),

    -- Thresholds
    SuccessThreshold Float64 DEFAULT 0.95 CODEC(Gorilla, ZSTD(1)),
    LatencyThresholdMs UInt32 DEFAULT 5000 CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) DEFAULT 'active' CODEC(ZSTD(1)), -- active, paused, deleted
    LastRunAt DateTime64(3) CODEC(ZSTD(1)),
    LastRunSuccess UInt8 CODEC(ZSTD(1)),

    -- Alerting
    AlertEnabled UInt8 DEFAULT 1,
    AlertChannels Array(String) CODEC(ZSTD(1)),

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    CreatedBy String CODEC(ZSTD(1)),
    Organization LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (CanaryId)
SETTINGS index_granularity = 8192;

-- Synthetics Run Results
CREATE TABLE IF NOT EXISTS synthetics_runs
(
    RunId UUID DEFAULT generateUUIDv4(),
    CanaryId UUID CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Execution context
    Region LowCardinality(String) CODEC(ZSTD(1)),
    ExecutionDurationMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Results
    Success UInt8 CODEC(ZSTD(1)),
    StatusCode Int32 CODEC(ZSTD(1)),

    -- Timing breakdown
    DnsLookupMs Float64 CODEC(Gorilla, ZSTD(1)),
    TcpConnectMs Float64 CODEC(Gorilla, ZSTD(1)),
    TlsHandshakeMs Float64 CODEC(Gorilla, ZSTD(1)),
    TimeToFirstByteMs Float64 CODEC(Gorilla, ZSTD(1)),
    ContentDownloadMs Float64 CODEC(Gorilla, ZSTD(1)),
    TotalDurationMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Steps (for multi-step scripts)
    Steps Nested(
        StepNumber UInt16,
        Name String,
        DurationMs Float64,
        Success UInt8,
        ErrorMessage String,
        ScreenshotUrl String
    ) CODEC(ZSTD(1)),

    -- Errors
    ErrorType LowCardinality(String) CODEC(ZSTD(1)),
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorStack String CODEC(ZSTD(1)),

    -- HAR data (optional, for debugging)
    HarDataUrl String CODEC(ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (CanaryId, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- REAL USER MONITORING (RUM)
-- Client-side performance and error tracking
-- =============================================================================

CREATE TABLE IF NOT EXISTS rum_sessions
(
    SessionId String CODEC(ZSTD(1)),
    StartTime DateTime64(3) CODEC(Delta, ZSTD(1)),
    EndTime DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- User context
    UserId String CODEC(ZSTD(1)),
    UserAnonymousId String CODEC(ZSTD(1)),

    -- Device info
    DeviceType LowCardinality(String) CODEC(ZSTD(1)), -- desktop, mobile, tablet
    Browser LowCardinality(String) CODEC(ZSTD(1)),
    BrowserVersion String CODEC(ZSTD(1)),
    OS LowCardinality(String) CODEC(ZSTD(1)),
    OSVersion String CODEC(ZSTD(1)),
    ScreenResolution String CODEC(ZSTD(1)),

    -- Location
    Country LowCardinality(String) CODEC(ZSTD(1)),
    Region String CODEC(ZSTD(1)),
    City String CODEC(ZSTD(1)),

    -- Application
    AppName LowCardinality(String) CODEC(ZSTD(1)),
    AppVersion String CODEC(ZSTD(1)),

    -- Session metrics
    PageViews UInt32 CODEC(ZSTD(1)),
    Interactions UInt32 CODEC(ZSTD(1)),
    Errors UInt32 CODEC(ZSTD(1)),
    DurationMs UInt64 CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(StartTime)
ORDER BY (AppName, SessionId, StartTime)
TTL toDateTime(StartTime) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS rum_page_views
(
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),

    -- Page info
    PageUrl String CODEC(ZSTD(1)),
    PagePath LowCardinality(String) CODEC(ZSTD(1)),
    PageTitle String CODEC(ZSTD(1)),
    Referrer String CODEC(ZSTD(1)),

    -- Core Web Vitals
    LCP Float64 CODEC(Gorilla, ZSTD(1)), -- Largest Contentful Paint (ms)
    FID Float64 CODEC(Gorilla, ZSTD(1)), -- First Input Delay (ms)
    CLS Float64 CODEC(Gorilla, ZSTD(1)), -- Cumulative Layout Shift
    TTFB Float64 CODEC(Gorilla, ZSTD(1)), -- Time to First Byte (ms)
    FCP Float64 CODEC(Gorilla, ZSTD(1)), -- First Contentful Paint (ms)
    INP Float64 CODEC(Gorilla, ZSTD(1)), -- Interaction to Next Paint (ms)

    -- Navigation timing
    DnsLookup Float64 CODEC(Gorilla, ZSTD(1)),
    TcpConnect Float64 CODEC(Gorilla, ZSTD(1)),
    TlsNegotiation Float64 CODEC(Gorilla, ZSTD(1)),
    DomContentLoaded Float64 CODEC(Gorilla, ZSTD(1)),
    WindowLoad Float64 CODEC(Gorilla, ZSTD(1)),

    -- Resource loading
    ResourceCount UInt32 CODEC(ZSTD(1)),
    TransferSize UInt64 CODEC(ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),

    -- Context
    AppName LowCardinality(String) CODEC(ZSTD(1)),
    Country LowCardinality(String) CODEC(ZSTD(1)),
    DeviceType LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (AppName, PagePath, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS rum_errors
(
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),

    -- Error info
    ErrorType LowCardinality(String) CODEC(ZSTD(1)), -- js_error, resource_error, api_error
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorStack String CODEC(ZSTD(1)),
    ErrorSource String CODEC(ZSTD(1)), -- Source file
    ErrorLine UInt32 CODEC(ZSTD(1)),
    ErrorColumn UInt32 CODEC(ZSTD(1)),

    -- Context
    PageUrl String CODEC(ZSTD(1)),
    UserAction String CODEC(ZSTD(1)), -- What user was doing

    -- API errors (if applicable)
    RequestUrl String CODEC(ZSTD(1)),
    RequestMethod LowCardinality(String) CODEC(ZSTD(1)),
    ResponseStatus Int32 CODEC(ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),

    -- Fingerprint for grouping
    ErrorFingerprint String CODEC(ZSTD(1)),

    -- Context
    AppName LowCardinality(String) CODEC(ZSTD(1)),
    AppVersion String CODEC(ZSTD(1)),
    Browser LowCardinality(String) CODEC(ZSTD(1)),
    OS LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (AppName, ErrorFingerprint, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- MATERIALIZED VIEWS FOR SLOs
-- =============================================================================

-- SLO attainment per minute (for latency SLOs)
CREATE MATERIALIZED VIEW IF NOT EXISTS slo_latency_1m
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, OperationName, Timestamp)
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    SpanName AS OperationName,
    count() AS TotalRequests,
    countIf(Duration <= 200000000) AS GoodRequests_200ms,
    countIf(Duration <= 500000000) AS GoodRequests_500ms,
    countIf(Duration <= 1000000000) AS GoodRequests_1s,
    quantile(0.50)(Duration) / 1000000 AS P50Ms,
    quantile(0.95)(Duration) / 1000000 AS P95Ms,
    quantile(0.99)(Duration) / 1000000 AS P99Ms
FROM otel_traces
WHERE IsRootSpan = 1
GROUP BY Timestamp, ServiceName, OperationName;

-- SLO attainment per minute (for availability SLOs)
CREATE MATERIALIZED VIEW IF NOT EXISTS slo_availability_1m
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, OperationName, Timestamp)
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    SpanName AS OperationName,
    count() AS TotalRequests,
    countIf(StatusCode != 'ERROR') AS SuccessRequests,
    countIf(StatusCode = 'ERROR') AS FailedRequests
FROM otel_traces
WHERE IsRootSpan = 1
GROUP BY Timestamp, ServiceName, OperationName;

-- =============================================================================
-- PROACTIVE AI INVESTIGATIONS
-- Auto-triggered investigations when anomalies/alerts occur
-- =============================================================================

CREATE TABLE IF NOT EXISTS investigations
(
    -- Identifiers
    InvestigationId UUID DEFAULT generateUUIDv4(),
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    -- Trigger info
    TriggerType LowCardinality(String) CODEC(ZSTD(1)), -- anomaly, alert, slo_breach, manual
    TriggerId String CODEC(ZSTD(1)), -- ID of the trigger (anomaly_id, alert_id, etc.)
    TriggerTimestamp DateTime64(3) CODEC(ZSTD(1)),

    -- Investigation scope
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    OperationName LowCardinality(String) CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1)),

    -- Time window being investigated
    InvestigationStart DateTime64(3) CODEC(ZSTD(1)),
    InvestigationEnd DateTime64(3) CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) DEFAULT 'running' CODEC(ZSTD(1)), -- running, completed, failed, cancelled
    Phase LowCardinality(String) CODEC(ZSTD(1)), -- gathering_data, analyzing, correlating, generating_hypotheses, complete

    -- Summary (AI-generated)
    Title String CODEC(ZSTD(1)),
    Summary String CODEC(ZSTD(1)),
    Severity LowCardinality(String) CODEC(ZSTD(1)), -- critical, high, medium, low

    -- Impact assessment
    AffectedServices Array(String) CODEC(ZSTD(1)),
    AffectedEndpoints Array(String) CODEC(ZSTD(1)),
    EstimatedUserImpact String CODEC(ZSTD(1)),
    ErrorCount UInt64 CODEC(ZSTD(1)),
    AffectedTraceCount UInt64 CODEC(ZSTD(1)),

    -- AI confidence
    OverallConfidence Float32 CODEC(Gorilla, ZSTD(1)),

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    CreatedBy String CODEC(ZSTD(1)), -- 'system' for auto-triggered
    Organization LowCardinality(String) CODEC(ZSTD(1)),

    -- Resolution
    ResolvedAt DateTime64(3) CODEC(ZSTD(1)),
    ResolvedBy String CODEC(ZSTD(1)),
    Resolution String CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (InvestigationId)
SETTINGS index_granularity = 8192;

-- Investigation hypotheses (AI-generated root cause candidates)
CREATE TABLE IF NOT EXISTS investigation_hypotheses
(
    HypothesisId UUID DEFAULT generateUUIDv4(),
    InvestigationId UUID CODEC(ZSTD(1)),
    CreatedAt DateTime64(3) DEFAULT now64(),

    -- Hypothesis details
    Rank UInt8 CODEC(ZSTD(1)), -- 1 = most likely
    Title String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),
    Category LowCardinality(String) CODEC(ZSTD(1)), -- infrastructure, code, dependency, configuration, capacity, external

    -- Confidence and reasoning
    Confidence Float32 CODEC(Gorilla, ZSTD(1)),
    Reasoning String CODEC(ZSTD(1)),

    -- Related entities
    RelatedServices Array(String) CODEC(ZSTD(1)),
    RelatedTraceIds Array(String) CODEC(ZSTD(1)),
    RelatedMetrics Array(String) CODEC(ZSTD(1)),
    RelatedLogs Array(String) CODEC(ZSTD(1)),
    RelatedDeployments Array(String) CODEC(ZSTD(1)),

    -- Suggested actions
    SuggestedActions Array(String) CODEC(ZSTD(1)),
    RunbookUrl String CODEC(ZSTD(1)),

    -- Verification status
    Verified UInt8 DEFAULT 0,
    VerifiedBy String CODEC(ZSTD(1)),
    VerifiedAt DateTime64(3) CODEC(ZSTD(1)),
    VerificationNotes String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
ORDER BY (InvestigationId, Rank)
SETTINGS index_granularity = 8192;

-- Investigation timeline events
CREATE TABLE IF NOT EXISTS investigation_timeline
(
    EventId UUID DEFAULT generateUUIDv4(),
    InvestigationId UUID CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Event details
    EventType LowCardinality(String) CODEC(ZSTD(1)), -- anomaly, error, deployment, config_change, metric_spike, log_pattern, alert
    EventSource LowCardinality(String) CODEC(ZSTD(1)), -- traces, metrics, logs, deployments, alerts
    Title String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),

    -- Severity and impact
    Severity LowCardinality(String) CODEC(ZSTD(1)),
    ImpactScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Related data
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    MetricName String CODEC(ZSTD(1)),
    MetricValue Float64 CODEC(Gorilla, ZSTD(1)),
    LogMessage String CODEC(ZSTD(1)),

    -- Links
    DeepLinkUrl String CODEC(ZSTD(1)),
    RawData String CODEC(ZSTD(1)) -- JSON blob of relevant data
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (InvestigationId, Timestamp)
SETTINGS index_granularity = 8192;

-- Investigation evidence (collected data points)
CREATE TABLE IF NOT EXISTS investigation_evidence
(
    EvidenceId UUID DEFAULT generateUUIDv4(),
    InvestigationId UUID CODEC(ZSTD(1)),
    HypothesisId UUID CODEC(ZSTD(1)), -- Optional, if evidence supports specific hypothesis
    CreatedAt DateTime64(3) DEFAULT now64(),

    -- Evidence details
    EvidenceType LowCardinality(String) CODEC(ZSTD(1)), -- trace, metric, log, deployment, config, topology
    Title String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),

    -- Relevance
    RelevanceScore Float32 CODEC(Gorilla, ZSTD(1)),
    IsSupporting UInt8 CODEC(ZSTD(1)), -- 1 = supports hypothesis, 0 = contradicts

    -- Source data
    SourceTimestamp DateTime64(3) CODEC(ZSTD(1)),
    SourceService LowCardinality(String) CODEC(ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    MetricName String CODEC(ZSTD(1)),
    MetricValue Float64 CODEC(Gorilla, ZSTD(1)),
    LogBody String CODEC(ZSTD(1)),

    -- Raw data
    RawData String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
ORDER BY (InvestigationId, HypothesisId, RelevanceScore)
SETTINGS index_granularity = 8192;

-- Deployment tracking (for correlation with incidents)
CREATE TABLE IF NOT EXISTS deployments
(
    DeploymentId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Deployment info
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1)),
    Version String CODEC(ZSTD(1)),
    PreviousVersion String CODEC(ZSTD(1)),

    -- Source control
    CommitHash String CODEC(ZSTD(1)),
    CommitMessage String CODEC(ZSTD(1)),
    CommitAuthor String CODEC(ZSTD(1)),
    Branch String CODEC(ZSTD(1)),
    Repository String CODEC(ZSTD(1)),
    PullRequestUrl String CODEC(ZSTD(1)),

    -- Deployment details
    DeploymentType LowCardinality(String) CODEC(ZSTD(1)), -- rolling, blue_green, canary
    DeploymentTool LowCardinality(String) CODEC(ZSTD(1)), -- kubernetes, ecs, lambda, etc.
    Duration UInt32 CODEC(ZSTD(1)), -- Seconds

    -- Status
    Status LowCardinality(String) CODEC(ZSTD(1)), -- success, failed, rolled_back
    RollbackOf UUID CODEC(ZSTD(1)), -- If this is a rollback, reference original

    -- Changes
    ChangedFiles Array(String) CODEC(ZSTD(1)),
    ConfigChanges Map(String, String) CODEC(ZSTD(1)),

    -- Metadata
    DeployedBy String CODEC(ZSTD(1)),
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- Investigation triggers configuration
CREATE TABLE IF NOT EXISTS investigation_triggers
(
    TriggerId UUID DEFAULT generateUUIDv4(),
    CreatedAt DateTime64(3) DEFAULT now64(),
    UpdatedAt DateTime64(3) DEFAULT now64(),

    -- Trigger configuration
    Name String CODEC(ZSTD(1)),
    Description String CODEC(ZSTD(1)),
    TriggerType LowCardinality(String) CODEC(ZSTD(1)), -- anomaly_score, error_rate, latency, slo_breach, alert
    Enabled UInt8 DEFAULT 1,

    -- Conditions
    ServiceFilter String CODEC(ZSTD(1)), -- Regex or specific service
    Threshold Float64 CODEC(Gorilla, ZSTD(1)),
    Operator LowCardinality(String) CODEC(ZSTD(1)), -- gt, lt, gte, lte, eq
    Duration String CODEC(ZSTD(1)), -- How long condition must persist

    -- Investigation settings
    AutoStart UInt8 DEFAULT 1,
    InvestigationWindow String DEFAULT '1h' CODEC(ZSTD(1)), -- How far back to look
    Priority LowCardinality(String) DEFAULT 'normal' CODEC(ZSTD(1)), -- high, normal, low

    -- Notification
    NotifyOnStart UInt8 DEFAULT 1,
    NotifyChannels Array(String) CODEC(ZSTD(1)),

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    CreatedBy String CODEC(ZSTD(1)),
    Organization LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = ReplacingMergeTree(UpdatedAt)
ORDER BY (TriggerId)
SETTINGS index_granularity = 8192;

-- =============================================================================
-- LLM OBSERVABILITY
-- Monitor AI/LLM applications: tokens, latency, cost, quality
-- =============================================================================

-- LLM Requests - Main table for LLM API calls
CREATE TABLE IF NOT EXISTS llm_requests
(
    -- Identifiers
    RequestId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),

    -- Service info
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1)),
    Version String CODEC(ZSTD(1)),

    -- LLM Provider info
    Provider LowCardinality(String) CODEC(ZSTD(1)), -- openai, anthropic, cohere, azure, bedrock, local
    Model LowCardinality(String) CODEC(ZSTD(1)), -- gpt-4, claude-3-opus, etc.
    ModelVersion String CODEC(ZSTD(1)),
    Endpoint String CODEC(ZSTD(1)),

    -- Request type
    RequestType LowCardinality(String) CODEC(ZSTD(1)), -- completion, chat, embedding, function_call, agent

    -- Token counts
    PromptTokens UInt32 CODEC(ZSTD(1)),
    CompletionTokens UInt32 CODEC(ZSTD(1)),
    TotalTokens UInt32 CODEC(ZSTD(1)),

    -- Cost tracking (in USD microdollars for precision)
    PromptCostMicros UInt64 CODEC(ZSTD(1)),
    CompletionCostMicros UInt64 CODEC(ZSTD(1)),
    TotalCostMicros UInt64 CODEC(ZSTD(1)),

    -- Timing
    DurationMs Float64 CODEC(Gorilla, ZSTD(1)),
    TimeToFirstTokenMs Float64 CODEC(Gorilla, ZSTD(1)), -- For streaming
    TokensPerSecond Float64 CODEC(Gorilla, ZSTD(1)),

    -- Request details
    Temperature Float32 CODEC(Gorilla, ZSTD(1)),
    MaxTokens UInt32 CODEC(ZSTD(1)),
    TopP Float32 CODEC(Gorilla, ZSTD(1)),
    FrequencyPenalty Float32 CODEC(Gorilla, ZSTD(1)),
    PresencePenalty Float32 CODEC(Gorilla, ZSTD(1)),
    StopSequences Array(String) CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) CODEC(ZSTD(1)), -- success, error, timeout, rate_limited
    StatusCode Int32 CODEC(ZSTD(1)),
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorType LowCardinality(String) CODEC(ZSTD(1)),

    -- Streaming
    IsStreaming UInt8 CODEC(ZSTD(1)),
    StreamChunks UInt32 CODEC(ZSTD(1)),

    -- Tool/Function calls
    HasToolCalls UInt8 CODEC(ZSTD(1)),
    ToolCallCount UInt32 CODEC(ZSTD(1)),
    ToolNames Array(String) CODEC(ZSTD(1)),

    -- RAG context
    HasRagContext UInt8 CODEC(ZSTD(1)),
    RagChunkCount UInt32 CODEC(ZSTD(1)),
    RagRetrievalTimeMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Quality signals (from evaluation)
    QualityScore Float32 CODEC(Gorilla, ZSTD(1)), -- 0-1 overall quality
    RelevanceScore Float32 CODEC(Gorilla, ZSTD(1)), -- Response relevance to prompt
    CoherenceScore Float32 CODEC(Gorilla, ZSTD(1)),
    FactualityScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Safety
    ContainsPII UInt8 CODEC(ZSTD(1)),
    SafetyFlagged UInt8 CODEC(ZSTD(1)),
    SafetyCategories Array(String) CODEC(ZSTD(1)),

    -- User context
    UserId String CODEC(ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),
    ConversationId String CODEC(ZSTD(1)),

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Organization LowCardinality(String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Model, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- LLM Prompts - Store prompts separately for analysis
CREATE TABLE IF NOT EXISTS llm_prompts
(
    PromptId UUID DEFAULT generateUUIDv4(),
    RequestId UUID CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Prompt details
    PromptType LowCardinality(String) CODEC(ZSTD(1)), -- system, user, assistant, function, tool
    Role LowCardinality(String) CODEC(ZSTD(1)),
    Content String CODEC(ZSTD(1)),
    ContentHash String CODEC(ZSTD(1)), -- For deduplication/caching analysis

    -- Template info (if using prompt templates)
    TemplateId String CODEC(ZSTD(1)),
    TemplateName String CODEC(ZSTD(1)),
    TemplateVersion String CODEC(ZSTD(1)),
    TemplateVariables Map(String, String) CODEC(ZSTD(1)),

    -- Tokens
    TokenCount UInt32 CODEC(ZSTD(1)),

    -- Position in conversation
    MessageIndex UInt32 CODEC(ZSTD(1)),

    -- Evaluation
    PromptQualityScore Float32 CODEC(Gorilla, ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, RequestId, MessageIndex)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- LLM Completions - Store completions for analysis
CREATE TABLE IF NOT EXISTS llm_completions
(
    CompletionId UUID DEFAULT generateUUIDv4(),
    RequestId UUID CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Completion details
    Content String CODEC(ZSTD(1)),
    ContentHash String CODEC(ZSTD(1)),
    FinishReason LowCardinality(String) CODEC(ZSTD(1)), -- stop, length, tool_calls, content_filter

    -- Tokens
    TokenCount UInt32 CODEC(ZSTD(1)),

    -- Choice info (for multiple completions)
    ChoiceIndex UInt32 CODEC(ZSTD(1)),

    -- Quality evaluation
    QualityScore Float32 CODEC(Gorilla, ZSTD(1)),
    RelevanceScore Float32 CODEC(Gorilla, ZSTD(1)),
    HallucinationScore Float32 CODEC(Gorilla, ZSTD(1)), -- Higher = more likely hallucination
    ToxicityScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- User feedback
    UserRating Int8 CODEC(ZSTD(1)), -- -1 to 1 (thumbs down/neutral/thumbs up)
    UserFeedback String CODEC(ZSTD(1)),
    WasEdited UInt8 CODEC(ZSTD(1)),
    EditedContent String CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, RequestId, ChoiceIndex)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- LLM Tool Calls - Track function/tool executions
CREATE TABLE IF NOT EXISTS llm_tool_calls
(
    ToolCallId UUID DEFAULT generateUUIDv4(),
    RequestId UUID CODEC(ZSTD(1)),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Tool info
    ToolName String CODEC(ZSTD(1)),
    ToolType LowCardinality(String) CODEC(ZSTD(1)), -- function, retrieval, code_interpreter, custom
    ToolDescription String CODEC(ZSTD(1)),

    -- Input/Output
    InputArguments String CODEC(ZSTD(1)), -- JSON
    OutputResult String CODEC(ZSTD(1)), -- JSON
    OutputTokenCount UInt32 CODEC(ZSTD(1)),

    -- Execution
    DurationMs Float64 CODEC(Gorilla, ZSTD(1)),
    Status LowCardinality(String) CODEC(ZSTD(1)), -- success, error, timeout
    ErrorMessage String CODEC(ZSTD(1)),

    -- Sequence
    CallIndex UInt32 CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, RequestId, CallIndex)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- LLM Embeddings - Track embedding operations
CREATE TABLE IF NOT EXISTS llm_embeddings
(
    EmbeddingId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1)),

    -- Provider
    Provider LowCardinality(String) CODEC(ZSTD(1)),
    Model LowCardinality(String) CODEC(ZSTD(1)),

    -- Request details
    InputType LowCardinality(String) CODEC(ZSTD(1)), -- text, document, query, image
    InputCount UInt32 CODEC(ZSTD(1)), -- Number of items embedded
    TotalTokens UInt32 CODEC(ZSTD(1)),
    Dimensions UInt32 CODEC(ZSTD(1)),

    -- Timing & Cost
    DurationMs Float64 CODEC(Gorilla, ZSTD(1)),
    CostMicros UInt64 CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) CODEC(ZSTD(1)),
    ErrorMessage String CODEC(ZSTD(1)),

    -- Usage context
    UseCase LowCardinality(String) CODEC(ZSTD(1)), -- rag_indexing, rag_query, similarity_search, clustering
    CollectionName String CODEC(ZSTD(1)), -- Vector DB collection

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Model, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- LLM RAG Retrievals - Track RAG pipeline retrieval operations
CREATE TABLE IF NOT EXISTS llm_rag_retrievals
(
    RetrievalId UUID DEFAULT generateUUIDv4(),
    RequestId UUID CODEC(ZSTD(1)), -- Links to llm_requests
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Query
    Query String CODEC(ZSTD(1)),
    QueryTokens UInt32 CODEC(ZSTD(1)),
    QueryEmbeddingTimeMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Vector search
    VectorStore LowCardinality(String) CODEC(ZSTD(1)), -- pinecone, weaviate, chroma, milvus, pgvector
    CollectionName String CODEC(ZSTD(1)),
    SearchType LowCardinality(String) CODEC(ZSTD(1)), -- similarity, mmr, hybrid
    TopK UInt32 CODEC(ZSTD(1)),
    SearchTimeMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Results
    ResultCount UInt32 CODEC(ZSTD(1)),
    TotalChunkTokens UInt32 CODEC(ZSTD(1)),

    -- Relevance scores
    AvgSimilarityScore Float32 CODEC(Gorilla, ZSTD(1)),
    MinSimilarityScore Float32 CODEC(Gorilla, ZSTD(1)),
    MaxSimilarityScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Reranking (if used)
    RerankerUsed UInt8 CODEC(ZSTD(1)),
    RerankerModel String CODEC(ZSTD(1)),
    RerankTimeMs Float64 CODEC(Gorilla, ZSTD(1)),

    -- Filtering
    MetadataFilters String CODEC(ZSTD(1)), -- JSON of filters applied

    -- Quality
    RetrievalQualityScore Float32 CODEC(Gorilla, ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, RequestId, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- LLM Chains/Agents - Track multi-step LLM workflows
CREATE TABLE IF NOT EXISTS llm_chains
(
    ChainId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Trace correlation
    TraceId String CODEC(ZSTD(1)),
    RootSpanId String CODEC(ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Environment LowCardinality(String) CODEC(ZSTD(1)),

    -- Chain info
    ChainType LowCardinality(String) CODEC(ZSTD(1)), -- sequential, parallel, agent, router, rag
    ChainName String CODEC(ZSTD(1)),

    -- Execution
    StepCount UInt32 CODEC(ZSTD(1)),
    TotalDurationMs Float64 CODEC(Gorilla, ZSTD(1)),
    LLMCallCount UInt32 CODEC(ZSTD(1)),
    ToolCallCount UInt32 CODEC(ZSTD(1)),
    RetrievalCount UInt32 CODEC(ZSTD(1)),

    -- Tokens & Cost
    TotalPromptTokens UInt32 CODEC(ZSTD(1)),
    TotalCompletionTokens UInt32 CODEC(ZSTD(1)),
    TotalCostMicros UInt64 CODEC(ZSTD(1)),

    -- Status
    Status LowCardinality(String) CODEC(ZSTD(1)),
    ErrorMessage String CODEC(ZSTD(1)),
    ErrorStep UInt32 CODEC(ZSTD(1)),

    -- Input/Output
    Input String CODEC(ZSTD(1)),
    Output String CODEC(ZSTD(1)),

    -- User context
    UserId String CODEC(ZSTD(1)),
    SessionId String CODEC(ZSTD(1)),
    ConversationId String CODEC(ZSTD(1)),

    -- Metadata
    Labels Map(LowCardinality(String), String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, ChainType, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- LLM Evaluations - Store evaluation results
CREATE TABLE IF NOT EXISTS llm_evaluations
(
    EvaluationId UUID DEFAULT generateUUIDv4(),
    Timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),

    -- Reference
    RequestId UUID CODEC(ZSTD(1)),
    CompletionId UUID CODEC(ZSTD(1)),
    ChainId UUID CODEC(ZSTD(1)),

    -- Service
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- Evaluation type
    EvaluationType LowCardinality(String) CODEC(ZSTD(1)), -- auto, human, llm_judge
    EvaluatorModel String CODEC(ZSTD(1)), -- For LLM-as-judge

    -- Scores (all 0-1)
    OverallScore Float32 CODEC(Gorilla, ZSTD(1)),
    RelevanceScore Float32 CODEC(Gorilla, ZSTD(1)),
    FaithfulnessScore Float32 CODEC(Gorilla, ZSTD(1)), -- For RAG
    CoherenceScore Float32 CODEC(Gorilla, ZSTD(1)),
    FluencyScore Float32 CODEC(Gorilla, ZSTD(1)),
    SafetyScore Float32 CODEC(Gorilla, ZSTD(1)),
    InstructionFollowingScore Float32 CODEC(Gorilla, ZSTD(1)),

    -- Custom scores
    CustomScores Map(String, Float32) CODEC(ZSTD(1)),

    -- Detailed feedback
    Feedback String CODEC(ZSTD(1)),
    FailureReasons Array(String) CODEC(ZSTD(1)),

    -- Human evaluation specific
    EvaluatorId String CODEC(ZSTD(1)),
    EvaluationTimeSeconds UInt32 CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, RequestId, Timestamp)
TTL toDateTime(Timestamp) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- LLM Cost aggregation view (per hour)
CREATE MATERIALIZED VIEW IF NOT EXISTS llm_cost_hourly
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Model, Provider, Timestamp)
AS SELECT
    toStartOfHour(Timestamp) AS Timestamp,
    ServiceName,
    Model,
    Provider,
    count() AS RequestCount,
    sum(TotalTokens) AS TotalTokens,
    sum(PromptTokens) AS PromptTokens,
    sum(CompletionTokens) AS CompletionTokens,
    sum(TotalCostMicros) AS TotalCostMicros,
    avg(DurationMs) AS AvgDurationMs,
    quantile(0.95)(DurationMs) AS P95DurationMs,
    countIf(Status = 'error') AS ErrorCount
FROM llm_requests
GROUP BY Timestamp, ServiceName, Model, Provider;

-- LLM Quality aggregation view (per hour)
CREATE MATERIALIZED VIEW IF NOT EXISTS llm_quality_hourly
ENGINE = SummingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Model, Timestamp)
AS SELECT
    toStartOfHour(Timestamp) AS Timestamp,
    ServiceName,
    Model,
    count() AS RequestCount,
    avg(QualityScore) AS AvgQualityScore,
    avg(RelevanceScore) AS AvgRelevanceScore,
    avg(CoherenceScore) AS AvgCoherenceScore,
    avg(FactualityScore) AS AvgFactualityScore,
    countIf(SafetyFlagged = 1) AS SafetyFlaggedCount,
    countIf(ContainsPII = 1) AS PIIDetectedCount
FROM llm_requests
WHERE QualityScore > 0
GROUP BY Timestamp, ServiceName, Model;

-- =============================================================================
-- GRANTS (for multi-tenancy, adjust as needed)
-- =============================================================================

-- Create read-only user for dashboards
-- CREATE USER IF NOT EXISTS ollystack_reader IDENTIFIED BY 'reader_password';
-- GRANT SELECT ON ollystack.* TO ollystack_reader;

-- Create writer user for collectors
-- CREATE USER IF NOT EXISTS ollystack_writer IDENTIFIED BY 'writer_password';
-- GRANT INSERT, SELECT ON ollystack.* TO ollystack_writer;
