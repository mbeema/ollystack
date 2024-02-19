-- Service Metrics Time-Series Aggregation
-- Pre-aggregated metrics for sparklines and dashboards
-- Date: 2026-02-04

-- ============================================================================
-- SERVICE METRICS 1-MINUTE AGGREGATION TABLE
-- ============================================================================
-- This table stores pre-aggregated metrics per service per minute
-- Used for sparklines, dashboards, and time-series charts

CREATE TABLE IF NOT EXISTS ollystack.service_metrics_1m (
    timestamp DateTime,
    service_name LowCardinality(String),
    request_count UInt64,
    error_count UInt64,
    total_duration_ns UInt64,
    min_duration_ns UInt64,
    max_duration_ns UInt64,
    p50_duration_ns UInt64,
    p95_duration_ns UInt64,
    p99_duration_ns UInt64
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, timestamp)
TTL timestamp + INTERVAL 7 DAY;

-- ============================================================================
-- MATERIALIZED VIEW TO AUTO-POPULATE FROM TRACES
-- ============================================================================
-- Automatically aggregates incoming trace data into 1-minute buckets

CREATE MATERIALIZED VIEW IF NOT EXISTS ollystack.service_metrics_1m_mv
TO ollystack.service_metrics_1m
AS SELECT
    toStartOfMinute(Timestamp) as timestamp,
    ServiceName as service_name,
    count() as request_count,
    countIf(StatusCode = 'STATUS_CODE_ERROR') as error_count,
    sum(Duration) as total_duration_ns,
    min(Duration) as min_duration_ns,
    max(Duration) as max_duration_ns,
    quantile(0.50)(Duration) as p50_duration_ns,
    quantile(0.95)(Duration) as p95_duration_ns,
    quantile(0.99)(Duration) as p99_duration_ns
FROM ollystack.traces
WHERE ServiceName != ''
GROUP BY timestamp, service_name;

-- ============================================================================
-- BACKFILL EXISTING DATA (run once after creating the table)
-- ============================================================================
-- Uncomment and run to populate from existing trace data:
--
-- INSERT INTO ollystack.service_metrics_1m
-- SELECT
--     toStartOfMinute(Timestamp) as timestamp,
--     ServiceName as service_name,
--     count() as request_count,
--     countIf(StatusCode = 'STATUS_CODE_ERROR') as error_count,
--     sum(Duration) as total_duration_ns,
--     min(Duration) as min_duration_ns,
--     max(Duration) as max_duration_ns,
--     quantile(0.50)(Duration) as p50_duration_ns,
--     quantile(0.95)(Duration) as p95_duration_ns,
--     quantile(0.99)(Duration) as p99_duration_ns
-- FROM ollystack.traces
-- WHERE ServiceName != ''
-- GROUP BY timestamp, service_name;

-- ============================================================================
-- USEFUL QUERIES
-- ============================================================================
-- Get service metrics for last hour with 5-minute buckets:
--
-- SELECT
--     service_name,
--     toStartOfFiveMinutes(timestamp) as bucket,
--     sum(request_count) as requests,
--     sum(error_count) as errors,
--     avg(total_duration_ns / request_count) / 1000000 as avg_latency_ms
-- FROM ollystack.service_metrics_1m
-- WHERE timestamp >= now() - INTERVAL 1 HOUR
-- GROUP BY service_name, bucket
-- ORDER BY service_name, bucket;
