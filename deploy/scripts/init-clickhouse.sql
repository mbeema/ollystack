-- OllyStack ClickHouse Schema
-- Run this script to initialize your ClickHouse Cloud database
--
-- Usage:
--   clickhouse-client --host your-service.clickhouse.cloud \
--     --port 9440 --secure --user default --password 'your-password' \
--     --database ollystack < init-clickhouse.sql

-- =============================================================================
-- Database
-- =============================================================================
CREATE DATABASE IF NOT EXISTS ollystack;

USE ollystack;

-- =============================================================================
-- Traces Table (OpenTelemetry format)
-- =============================================================================
CREATE TABLE IF NOT EXISTS traces
(
    -- Trace identification
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    parent_span_id String CODEC(ZSTD(1)),
    trace_state String CODEC(ZSTD(1)),

    -- Span info
    span_name LowCardinality(String),
    span_kind LowCardinality(String),

    -- Service info
    service_name LowCardinality(String),

    -- Timing
    start_time DateTime64(9) CODEC(Delta, ZSTD(1)),
    end_time DateTime64(9) CODEC(Delta, ZSTD(1)),
    duration_ns UInt64 CODEC(ZSTD(1)),

    -- Status
    status_code LowCardinality(String),
    status_message String CODEC(ZSTD(1)),

    -- Attributes (as JSON for flexibility)
    span_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Events
    events Nested(
        name LowCardinality(String),
        timestamp DateTime64(9),
        attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),

    -- Links
    links Nested(
        trace_id String,
        span_id String,
        trace_state String,
        attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),

    -- Metadata
    inserted_at DateTime DEFAULT now() CODEC(Delta, ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(start_time)
ORDER BY (service_name, span_name, start_time, trace_id)
TTL start_time + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Secondary index for trace_id lookups
ALTER TABLE traces ADD INDEX idx_trace_id trace_id TYPE bloom_filter() GRANULARITY 1;
ALTER TABLE traces ADD INDEX idx_status status_code TYPE set(3) GRANULARITY 1;

-- =============================================================================
-- Logs Table
-- =============================================================================
CREATE TABLE IF NOT EXISTS logs
(
    -- Timestamp and identification
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    observed_timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),

    -- Log content
    severity_text LowCardinality(String),
    severity_number UInt8 CODEC(ZSTD(1)),
    body String CODEC(ZSTD(1)),

    -- Service info
    service_name LowCardinality(String),

    -- Attributes
    log_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Metadata
    inserted_at DateTime DEFAULT now() CODEC(Delta, ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, severity_text, timestamp)
TTL timestamp + INTERVAL 14 DAY
SETTINGS index_granularity = 8192;

-- Full-text search index on log body
ALTER TABLE logs ADD INDEX idx_body body TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;
ALTER TABLE logs ADD INDEX idx_trace_id trace_id TYPE bloom_filter() GRANULARITY 1;

-- =============================================================================
-- Metrics Table (for raw metrics)
-- =============================================================================
CREATE TABLE IF NOT EXISTS metrics
(
    -- Metric identification
    metric_name LowCardinality(String),
    metric_type LowCardinality(String), -- gauge, counter, histogram, summary
    metric_unit LowCardinality(String),

    -- Service info
    service_name LowCardinality(String),

    -- Labels/dimensions
    labels Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- Values
    timestamp DateTime64(3) CODEC(Delta, ZSTD(1)),
    value Float64 CODEC(ZSTD(1)),

    -- For histograms
    histogram_count UInt64 CODEC(ZSTD(1)),
    histogram_sum Float64 CODEC(ZSTD(1)),
    histogram_bucket_counts Array(UInt64) CODEC(ZSTD(1)),
    histogram_explicit_bounds Array(Float64) CODEC(ZSTD(1)),

    -- Metadata
    inserted_at DateTime DEFAULT now() CODEC(Delta, ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (metric_name, service_name, timestamp)
TTL timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- Service Topology (derived from traces)
-- =============================================================================
CREATE TABLE IF NOT EXISTS service_topology
(
    -- Edge identification
    source_service LowCardinality(String),
    target_service LowCardinality(String),
    operation LowCardinality(String),

    -- Time bucket
    time_bucket DateTime CODEC(Delta, ZSTD(1)),

    -- Statistics
    request_count UInt64 CODEC(ZSTD(1)),
    error_count UInt64 CODEC(ZSTD(1)),
    total_duration_ns UInt64 CODEC(ZSTD(1)),
    min_duration_ns UInt64 CODEC(ZSTD(1)),
    max_duration_ns UInt64 CODEC(ZSTD(1)),
    p50_duration_ns UInt64 CODEC(ZSTD(1)),
    p95_duration_ns UInt64 CODEC(ZSTD(1)),
    p99_duration_ns UInt64 CODEC(ZSTD(1))
)
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(time_bucket)
ORDER BY (source_service, target_service, operation, time_bucket)
TTL time_bucket + INTERVAL 90 DAY;

-- =============================================================================
-- Materialized View: Service Topology from Traces
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS service_topology_mv
TO service_topology
AS
SELECT
    resource_attributes['service.name'] AS source_service,
    span_attributes['peer.service'] AS target_service,
    span_name AS operation,
    toStartOfMinute(start_time) AS time_bucket,
    count() AS request_count,
    countIf(status_code = 'ERROR') AS error_count,
    sum(duration_ns) AS total_duration_ns,
    min(duration_ns) AS min_duration_ns,
    max(duration_ns) AS max_duration_ns,
    quantile(0.5)(duration_ns) AS p50_duration_ns,
    quantile(0.95)(duration_ns) AS p95_duration_ns,
    quantile(0.99)(duration_ns) AS p99_duration_ns
FROM traces
WHERE span_kind IN ('CLIENT', 'PRODUCER')
  AND span_attributes['peer.service'] != ''
GROUP BY source_service, target_service, operation, time_bucket;

-- =============================================================================
-- Service Statistics (for dashboard)
-- =============================================================================
CREATE TABLE IF NOT EXISTS service_stats
(
    service_name LowCardinality(String),
    time_bucket DateTime CODEC(Delta, ZSTD(1)),

    -- Request metrics
    request_count UInt64 CODEC(ZSTD(1)),
    error_count UInt64 CODEC(ZSTD(1)),

    -- Latency metrics
    total_duration_ns UInt64 CODEC(ZSTD(1)),
    min_duration_ns UInt64 CODEC(ZSTD(1)),
    max_duration_ns UInt64 CODEC(ZSTD(1)),
    p50_duration_ns UInt64 CODEC(ZSTD(1)),
    p95_duration_ns UInt64 CODEC(ZSTD(1)),
    p99_duration_ns UInt64 CODEC(ZSTD(1))
)
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(time_bucket)
ORDER BY (service_name, time_bucket)
TTL time_bucket + INTERVAL 90 DAY;

-- Materialized View for service stats
CREATE MATERIALIZED VIEW IF NOT EXISTS service_stats_mv
TO service_stats
AS
SELECT
    service_name,
    toStartOfMinute(start_time) AS time_bucket,
    count() AS request_count,
    countIf(status_code = 'ERROR') AS error_count,
    sum(duration_ns) AS total_duration_ns,
    min(duration_ns) AS min_duration_ns,
    max(duration_ns) AS max_duration_ns,
    quantile(0.5)(duration_ns) AS p50_duration_ns,
    quantile(0.95)(duration_ns) AS p95_duration_ns,
    quantile(0.99)(duration_ns) AS p99_duration_ns
FROM traces
WHERE span_kind = 'SERVER'
GROUP BY service_name, time_bucket;

-- =============================================================================
-- Useful Views
-- =============================================================================

-- Recent errors view
CREATE VIEW IF NOT EXISTS recent_errors AS
SELECT
    trace_id,
    span_id,
    service_name,
    span_name,
    status_message,
    start_time,
    duration_ns / 1000000 AS duration_ms,
    span_attributes
FROM traces
WHERE status_code = 'ERROR'
  AND start_time > now() - INTERVAL 1 HOUR
ORDER BY start_time DESC
LIMIT 100;

-- Slow queries view (>1s)
CREATE VIEW IF NOT EXISTS slow_traces AS
SELECT
    trace_id,
    span_id,
    service_name,
    span_name,
    start_time,
    duration_ns / 1000000 AS duration_ms,
    span_attributes
FROM traces
WHERE duration_ns > 1000000000  -- >1 second
  AND start_time > now() - INTERVAL 1 HOUR
ORDER BY duration_ns DESC
LIMIT 100;

-- Service health summary
CREATE VIEW IF NOT EXISTS service_health AS
SELECT
    service_name,
    count() AS total_requests,
    countIf(status_code = 'ERROR') AS errors,
    round(countIf(status_code = 'ERROR') * 100.0 / count(), 2) AS error_rate,
    round(avg(duration_ns) / 1000000, 2) AS avg_duration_ms,
    round(quantile(0.95)(duration_ns) / 1000000, 2) AS p95_duration_ms,
    round(quantile(0.99)(duration_ns) / 1000000, 2) AS p99_duration_ms
FROM traces
WHERE start_time > now() - INTERVAL 1 HOUR
  AND span_kind = 'SERVER'
GROUP BY service_name
ORDER BY total_requests DESC;

-- =============================================================================
-- Grants (adjust for your users)
-- =============================================================================
-- GRANT SELECT, INSERT ON ollystack.* TO ollystack_writer;
-- GRANT SELECT ON ollystack.* TO ollystack_reader;
