-- OllyStack Log Correlation Indexes
-- Adds materialized columns and indexes for log-trace correlation

-- =============================================
-- LOGS TABLE: Materialized columns for JSON extraction
-- =============================================

-- Extract correlation_id from JSON body
ALTER TABLE ollystack.logs ADD COLUMN IF NOT EXISTS correlation_id String
    MATERIALIZED JSONExtractString(Body, 'correlation_id');

-- Extract log level
ALTER TABLE ollystack.logs ADD COLUMN IF NOT EXISTS log_level String
    MATERIALIZED JSONExtractString(Body, 'level');

-- Extract service name from JSON
ALTER TABLE ollystack.logs ADD COLUMN IF NOT EXISTS log_service String
    MATERIALIZED JSONExtractString(Body, 'service');

-- Extract message
ALTER TABLE ollystack.logs ADD COLUMN IF NOT EXISTS log_message String
    MATERIALIZED JSONExtractString(Body, 'message');

-- =============================================
-- LOGS TABLE: Skip indexes for fast filtering
-- =============================================

-- Bloom filter for correlation_id (fast lookups)
ALTER TABLE ollystack.logs ADD INDEX IF NOT EXISTS idx_correlation_id
    correlation_id TYPE bloom_filter GRANULARITY 4;

-- Set index for log_level (low cardinality: info, warn, error, debug)
ALTER TABLE ollystack.logs ADD INDEX IF NOT EXISTS idx_log_level
    log_level TYPE set(10) GRANULARITY 4;

-- Set index for log_service
ALTER TABLE ollystack.logs ADD INDEX IF NOT EXISTS idx_log_service
    log_service TYPE set(100) GRANULARITY 4;

-- Token bloom filter for text search in log_message
ALTER TABLE ollystack.logs ADD INDEX IF NOT EXISTS idx_log_message
    log_message TYPE tokenbf_v1(10240, 3, 0) GRANULARITY 4;

-- =============================================
-- TRACES TABLE: Correlation support
-- =============================================

-- Extract correlation_id from SpanAttributes for easier joins
ALTER TABLE ollystack.traces ADD COLUMN IF NOT EXISTS correlation_id String
    MATERIALIZED SpanAttributes['correlation_id'];

-- Bloom filter for correlation_id lookups
ALTER TABLE ollystack.traces ADD INDEX IF NOT EXISTS idx_trace_correlation
    correlation_id TYPE bloom_filter GRANULARITY 4;

-- =============================================
-- Materialize columns and indexes for existing data
-- =============================================

ALTER TABLE ollystack.logs MATERIALIZE COLUMN correlation_id;
ALTER TABLE ollystack.logs MATERIALIZE COLUMN log_level;
ALTER TABLE ollystack.logs MATERIALIZE COLUMN log_service;
ALTER TABLE ollystack.logs MATERIALIZE COLUMN log_message;
ALTER TABLE ollystack.traces MATERIALIZE COLUMN correlation_id;

ALTER TABLE ollystack.logs MATERIALIZE INDEX idx_correlation_id;
ALTER TABLE ollystack.logs MATERIALIZE INDEX idx_log_level;
ALTER TABLE ollystack.logs MATERIALIZE INDEX idx_log_service;
ALTER TABLE ollystack.logs MATERIALIZE INDEX idx_log_message;
ALTER TABLE ollystack.traces MATERIALIZE INDEX idx_trace_correlation;

-- =============================================
-- VIEW: Correlated events (traces + logs)
-- =============================================

CREATE OR REPLACE VIEW ollystack.correlated_events AS
SELECT
    correlation_id,
    Timestamp,
    'trace' as event_type,
    ServiceName as service,
    SpanName as operation,
    Duration / 1000000 as duration_ms,
    StatusCode as status,
    '' as log_level,
    '' as message,
    TraceId as trace_id,
    SpanId as span_id
FROM ollystack.traces
WHERE correlation_id != ''

UNION ALL

SELECT
    correlation_id,
    Timestamp,
    'log' as event_type,
    log_service as service,
    '' as operation,
    0 as duration_ms,
    '' as status,
    log_level,
    log_message as message,
    '' as trace_id,
    '' as span_id
FROM ollystack.logs
WHERE correlation_id != '';

-- =============================================
-- Example queries
-- =============================================

-- Get all events for a correlation ID (traces + logs)
-- SELECT * FROM ollystack.correlated_events
-- WHERE correlation_id = 'olly-xxx'
-- ORDER BY Timestamp;

-- Find logs by service and level
-- SELECT * FROM ollystack.logs
-- WHERE log_service = 'api-gateway' AND log_level = 'error'
-- ORDER BY Timestamp DESC LIMIT 100;

-- Search logs by message text
-- SELECT * FROM ollystack.logs
-- WHERE hasToken(log_message, 'payment')
-- ORDER BY Timestamp DESC LIMIT 100;

-- Get service latency from traces
-- SELECT ServiceName, avg(Duration)/1000000 as avg_ms
-- FROM ollystack.traces
-- WHERE Timestamp > now() - INTERVAL 1 HOUR
-- GROUP BY ServiceName;
