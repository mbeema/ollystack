-- OllyStack Enterprise Schema
--
-- Features:
-- 1. Tiered storage (hot → warm → cold)
-- 2. Automatic data rollup and compaction
-- 3. Multi-tenant isolation
-- 4. Cost-optimized storage policies
-- 5. Query optimization with materialized views
--
-- Cost savings: ~90% compared to keeping all data at full resolution

-- ============================================================================
-- STORAGE POLICIES (Hot/Warm/Cold Tiering)
-- ============================================================================

-- Note: In production, configure storage policies in config.xml:
-- <storage_configuration>
--   <disks>
--     <hot><path>/var/lib/clickhouse/hot/</path></hot>
--     <warm><path>/var/lib/clickhouse/warm/</path></warm>
--     <cold><type>s3</type><endpoint>...</endpoint></cold>
--   </disks>
--   <policies>
--     <tiered>
--       <volumes>
--         <hot><disk>hot</disk></hot>
--         <warm><disk>warm</disk></warm>
--         <cold><disk>cold</disk></cold>
--       </volumes>
--       <move_factor>0.1</move_factor>
--     </tiered>
--   </policies>
-- </storage_configuration>

-- ============================================================================
-- TENANT MANAGEMENT
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenants (
    tenant_id UUID DEFAULT generateUUIDv4(),
    name String,
    plan LowCardinality(String),  -- free, startup, growth, enterprise

    -- Quotas
    max_events_per_sec UInt64 DEFAULT 1000,
    max_bytes_per_day UInt64 DEFAULT 10737418240,  -- 10GB
    max_storage_bytes UInt64 DEFAULT 107374182400,  -- 100GB
    retention_days UInt16 DEFAULT 7,

    -- Sampling configuration
    sampling_rate Float32 DEFAULT 0.1,  -- 10%
    always_sample_errors Bool DEFAULT true,
    always_sample_slow Bool DEFAULT true,
    slow_threshold_ms UInt32 DEFAULT 1000,

    -- Feature flags
    features Array(LowCardinality(String)) DEFAULT ['metrics', 'logs', 'traces'],

    -- Billing
    overage_rate_per_gb Decimal(10, 4) DEFAULT 0.10,

    -- Metadata
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now(),
    status LowCardinality(String) DEFAULT 'active'
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (tenant_id);

-- Insert default tenant
INSERT INTO tenants (tenant_id, name, plan, max_events_per_sec, max_bytes_per_day, retention_days, sampling_rate)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'default',
    'enterprise',
    10000000,
    10995116277760,  -- 10TB
    90,
    1.0
);

-- ============================================================================
-- RAW DATA TABLES (Hot Tier - Full Resolution)
-- ============================================================================

-- Metrics with tenant isolation
CREATE TABLE IF NOT EXISTS metrics_raw (
    tenant_id UUID,
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    metric_name LowCardinality(String),
    metric_type LowCardinality(String),
    value Float64 CODEC(Gorilla, ZSTD(1)),
    labels Map(LowCardinality(String), String) CODEC(ZSTD(3)),

    -- Extracted for fast filtering
    service_name LowCardinality(String) MATERIALIZED labels['service.name'],
    host LowCardinality(String) MATERIALIZED labels['host'],
    environment LowCardinality(String) MATERIALIZED labels['environment'],

    -- Sampling metadata
    sample_rate Float32 DEFAULT 1.0,
    is_sampled Bool DEFAULT true,

    INDEX idx_service service_name TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_metric metric_name TYPE bloom_filter(0.01) GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, metric_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 24 HOUR TO VOLUME 'warm',
    toDateTime(timestamp) + INTERVAL 30 DAY TO VOLUME 'cold',
    toDateTime(timestamp) + INTERVAL 365 DAY DELETE
SETTINGS index_granularity = 8192,
         storage_policy = 'tiered';

-- Logs with deduplication
CREATE TABLE IF NOT EXISTS logs_raw (
    tenant_id UUID,
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    severity LowCardinality(String),
    severity_number UInt8,
    body String CODEC(ZSTD(3)),
    attributes Map(LowCardinality(String), String) CODEC(ZSTD(3)),

    -- Extracted fields
    service_name LowCardinality(String) MATERIALIZED attributes['service.name'],
    host LowCardinality(String) MATERIALIZED attributes['host'],

    -- Deduplication
    pattern_hash String CODEC(ZSTD(1)),
    occurrence_count UInt32 DEFAULT 1,

    -- Sampling
    sample_rate Float32 DEFAULT 1.0,

    INDEX idx_trace trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_body body TYPE tokenbf_v1(10240, 3, 0) GRANULARITY 4,
    INDEX idx_severity severity_number TYPE minmax GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, severity_number, timestamp)
TTL toDateTime(timestamp) + INTERVAL 24 HOUR TO VOLUME 'warm',
    toDateTime(timestamp) + INTERVAL 14 DAY TO VOLUME 'cold',
    toDateTime(timestamp) + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192,
         storage_policy = 'tiered';

-- Traces with full span data
CREATE TABLE IF NOT EXISTS traces_raw (
    tenant_id UUID,
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    parent_span_id String CODEC(ZSTD(1)),
    span_name LowCardinality(String),
    span_kind LowCardinality(String),
    service_name LowCardinality(String),
    duration_ns Int64 CODEC(Delta, ZSTD(1)),
    status_code LowCardinality(String),
    status_message String CODEC(ZSTD(1)),
    attributes Map(LowCardinality(String), String) CODEC(ZSTD(3)),
    events Array(Tuple(
        timestamp DateTime64(9),
        name LowCardinality(String),
        attributes Map(LowCardinality(String), String)
    )) CODEC(ZSTD(3)),

    -- Extracted for queries
    http_method LowCardinality(String) MATERIALIZED attributes['http.method'],
    http_status_code UInt16 MATERIALIZED toUInt16OrZero(attributes['http.status_code']),
    http_url String MATERIALIZED attributes['http.url'],
    db_system LowCardinality(String) MATERIALIZED attributes['db.system'],

    -- Sampling
    sample_rate Float32 DEFAULT 1.0,
    is_error Bool MATERIALIZED status_code = 'ERROR',
    is_slow Bool DEFAULT false,

    INDEX idx_trace trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_error is_error TYPE minmax GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, span_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 24 HOUR TO VOLUME 'warm',
    toDateTime(timestamp) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(timestamp) + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192,
         storage_policy = 'tiered';

-- ============================================================================
-- ROLLUP TABLES (Warm Tier - 1 minute resolution)
-- ============================================================================

CREATE TABLE IF NOT EXISTS metrics_1m (
    tenant_id UUID,
    timestamp DateTime CODEC(Delta, ZSTD(1)),
    metric_name LowCardinality(String),
    service_name LowCardinality(String),
    host LowCardinality(String),
    environment LowCardinality(String),

    -- Aggregates
    count UInt64,
    sum Float64,
    min Float64,
    max Float64,
    avg Float64,
    p50 Float64,
    p90 Float64,
    p95 Float64,
    p99 Float64
) ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, metric_name, timestamp)
TTL timestamp + INTERVAL 30 DAY TO VOLUME 'cold',
    timestamp + INTERVAL 365 DAY DELETE;

-- Automatic rollup from raw to 1m
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_metrics_1m TO metrics_1m AS
SELECT
    tenant_id,
    toStartOfMinute(timestamp) as timestamp,
    metric_name,
    service_name,
    host,
    environment,
    count() as count,
    sum(value) as sum,
    min(value) as min,
    max(value) as max,
    avg(value) as avg,
    quantile(0.50)(value) as p50,
    quantile(0.90)(value) as p90,
    quantile(0.95)(value) as p95,
    quantile(0.99)(value) as p99
FROM metrics_raw
GROUP BY tenant_id, timestamp, metric_name, service_name, host, environment;

-- Service metrics rollup (RED metrics)
CREATE TABLE IF NOT EXISTS service_metrics_1m (
    tenant_id UUID,
    timestamp DateTime CODEC(Delta, ZSTD(1)),
    service_name LowCardinality(String),
    span_name LowCardinality(String),

    -- Rate
    request_count UInt64,

    -- Errors
    error_count UInt64,
    error_rate Float32 MATERIALIZED if(request_count > 0, error_count / request_count * 100, 0),

    -- Duration
    duration_sum_ns Int64,
    duration_min_ns Int64,
    duration_max_ns Int64,
    duration_avg_ms Float64 MATERIALIZED duration_sum_ns / request_count / 1000000,
    duration_p50_ms Float64,
    duration_p95_ms Float64,
    duration_p99_ms Float64
) ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, span_name, timestamp)
TTL timestamp + INTERVAL 30 DAY TO VOLUME 'cold',
    timestamp + INTERVAL 365 DAY DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_service_metrics_1m TO service_metrics_1m AS
SELECT
    tenant_id,
    toStartOfMinute(timestamp) as timestamp,
    service_name,
    span_name,
    count() as request_count,
    countIf(status_code = 'ERROR') as error_count,
    sum(duration_ns) as duration_sum_ns,
    min(duration_ns) as duration_min_ns,
    max(duration_ns) as duration_max_ns,
    quantile(0.50)(duration_ns / 1000000) as duration_p50_ms,
    quantile(0.95)(duration_ns / 1000000) as duration_p95_ms,
    quantile(0.99)(duration_ns / 1000000) as duration_p99_ms
FROM traces_raw
WHERE span_kind = 'SERVER'
GROUP BY tenant_id, timestamp, service_name, span_name;

-- ============================================================================
-- HOURLY ROLLUP TABLES (Cold Tier - 1 hour resolution)
-- ============================================================================

CREATE TABLE IF NOT EXISTS metrics_1h (
    tenant_id UUID,
    timestamp DateTime CODEC(Delta, ZSTD(1)),
    metric_name LowCardinality(String),
    service_name LowCardinality(String),

    count UInt64,
    sum Float64,
    min Float64,
    max Float64,
    avg Float64,
    p95 Float64,
    p99 Float64
) ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMM(timestamp))
ORDER BY (tenant_id, service_name, metric_name, timestamp)
TTL timestamp + INTERVAL 2 YEAR DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_metrics_1h TO metrics_1h AS
SELECT
    tenant_id,
    toStartOfHour(timestamp) as timestamp,
    metric_name,
    service_name,
    sum(count) as count,
    sum(sum) as sum,
    min(min) as min,
    max(max) as max,
    avg(avg) as avg,
    max(p95) as p95,
    max(p99) as p99
FROM metrics_1m
GROUP BY tenant_id, timestamp, metric_name, service_name;

-- ============================================================================
-- ERROR TRACKING (Always Keep Errors)
-- ============================================================================

CREATE TABLE IF NOT EXISTS errors_raw (
    tenant_id UUID,
    timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    trace_id String CODEC(ZSTD(1)),
    span_id String CODEC(ZSTD(1)),
    service_name LowCardinality(String),
    error_type LowCardinality(String),
    error_message String CODEC(ZSTD(3)),
    stack_trace String CODEC(ZSTD(3)),
    attributes Map(LowCardinality(String), String) CODEC(ZSTD(3)),

    -- Grouping
    error_hash String CODEC(ZSTD(1)),
    occurrence_count UInt32 DEFAULT 1,

    INDEX idx_trace trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_error_hash error_hash TYPE bloom_filter(0.01) GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, error_type, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Error aggregation for dashboard
CREATE TABLE IF NOT EXISTS error_aggregates (
    tenant_id UUID,
    timestamp DateTime CODEC(Delta, ZSTD(1)),
    service_name LowCardinality(String),
    error_type LowCardinality(String),
    error_hash String,

    count UInt64,
    first_seen DateTime,
    last_seen DateTime,
    sample_trace_id String,
    sample_message String
) ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, service_name, error_type, timestamp)
TTL timestamp + INTERVAL 30 DAY DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_error_aggregates TO error_aggregates AS
SELECT
    tenant_id,
    toStartOfMinute(timestamp) as timestamp,
    service_name,
    error_type,
    error_hash,
    sum(occurrence_count) as count,
    min(timestamp) as first_seen,
    max(timestamp) as last_seen,
    any(trace_id) as sample_trace_id,
    any(error_message) as sample_message
FROM errors_raw
GROUP BY tenant_id, timestamp, service_name, error_type, error_hash;

-- ============================================================================
-- SERVICE TOPOLOGY (Auto-discovered)
-- ============================================================================

CREATE TABLE IF NOT EXISTS service_topology (
    tenant_id UUID,
    timestamp DateTime CODEC(Delta, ZSTD(1)),
    source_service LowCardinality(String),
    target_service LowCardinality(String),
    protocol LowCardinality(String),  -- http, grpc, kafka, redis, etc.

    call_count UInt64,
    error_count UInt64,
    duration_sum_ns Int64,
    duration_p50_ms Float64,
    duration_p95_ms Float64
) ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMMDD(timestamp))
ORDER BY (tenant_id, source_service, target_service, timestamp)
TTL timestamp + INTERVAL 30 DAY DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_service_topology TO service_topology AS
SELECT
    tenant_id,
    toStartOfMinute(timestamp) as timestamp,
    service_name as source_service,
    coalesce(attributes['peer.service'], attributes['net.peer.name'], 'unknown') as target_service,
    coalesce(attributes['rpc.system'], attributes['db.system'], 'http') as protocol,
    count() as call_count,
    countIf(status_code = 'ERROR') as error_count,
    sum(duration_ns) as duration_sum_ns,
    quantile(0.50)(duration_ns / 1000000) as duration_p50_ms,
    quantile(0.95)(duration_ns / 1000000) as duration_p95_ms
FROM traces_raw
WHERE span_kind IN ('CLIENT', 'PRODUCER')
GROUP BY tenant_id, timestamp, source_service, target_service, protocol;

-- ============================================================================
-- USAGE METERING (For Billing)
-- ============================================================================

CREATE TABLE IF NOT EXISTS usage_metrics (
    tenant_id UUID,
    date Date,
    metric_type LowCardinality(String),  -- events_ingested, bytes_ingested, bytes_stored, queries

    count UInt64,
    bytes UInt64
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (tenant_id, date, metric_type);

-- Track ingestion
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_usage_ingestion TO usage_metrics AS
SELECT
    tenant_id,
    toDate(timestamp) as date,
    'events_ingested' as metric_type,
    count() as count,
    sum(length(body)) as bytes
FROM logs_raw
GROUP BY tenant_id, date;

-- ============================================================================
-- ALERT RULES (Configurable per tenant)
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_rules (
    tenant_id UUID,
    rule_id UUID DEFAULT generateUUIDv4(),
    name String,
    description String,

    -- Condition
    metric_type LowCardinality(String),  -- metric, log, trace, composite
    query String,  -- ClickHouse SQL for condition
    threshold Float64,
    operator LowCardinality(String),  -- gt, lt, eq, ne, gte, lte
    duration_seconds UInt32 DEFAULT 300,

    -- Alert config
    severity LowCardinality(String) DEFAULT 'warning',
    labels Map(String, String),
    annotations Map(String, String),

    -- Notification
    notification_channels Array(String),

    -- Status
    enabled Bool DEFAULT true,
    last_triggered DateTime,

    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (tenant_id, rule_id);

-- Insert default alert rules
INSERT INTO alert_rules (tenant_id, name, description, metric_type, query, threshold, operator, severity)
SELECT
    tenant_id,
    'High Error Rate',
    'Error rate exceeds 5% for any service',
    'trace',
    'SELECT service_name, error_rate FROM service_metrics_1m WHERE tenant_id = {tenant_id} AND timestamp > now() - INTERVAL 5 MINUTE GROUP BY service_name HAVING error_rate > {threshold}',
    5.0,
    'gt',
    'warning'
FROM tenants WHERE tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================================
-- QUERY VIEWS (Optimized for Dashboard)
-- ============================================================================

-- Service health dashboard
CREATE VIEW IF NOT EXISTS v_service_health AS
SELECT
    tenant_id,
    service_name,
    sum(request_count) as total_requests,
    sum(error_count) as total_errors,
    if(sum(request_count) > 0, sum(error_count) / sum(request_count) * 100, 0) as error_rate,
    avg(duration_avg_ms) as avg_latency_ms,
    max(duration_p99_ms) as p99_latency_ms
FROM service_metrics_1m
WHERE timestamp > now() - INTERVAL 5 MINUTE
GROUP BY tenant_id, service_name;

-- Recent errors view
CREATE VIEW IF NOT EXISTS v_recent_errors AS
SELECT
    tenant_id,
    service_name,
    error_type,
    count as occurrence_count,
    last_seen,
    sample_message,
    sample_trace_id
FROM error_aggregates
WHERE timestamp > now() - INTERVAL 1 HOUR
ORDER BY count DESC
LIMIT 100;

-- Cost tracking view
CREATE VIEW IF NOT EXISTS v_tenant_usage AS
SELECT
    tenant_id,
    date,
    sumIf(count, metric_type = 'events_ingested') as events_ingested,
    sumIf(bytes, metric_type = 'bytes_ingested') as bytes_ingested,
    sumIf(bytes, metric_type = 'bytes_stored') as bytes_stored
FROM usage_metrics
WHERE date >= today() - 30
GROUP BY tenant_id, date
ORDER BY tenant_id, date;
