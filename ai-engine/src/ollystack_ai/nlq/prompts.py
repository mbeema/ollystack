"""
NLQ Prompts

System and user prompts for natural language query translation.
"""

SYSTEM_PROMPT = """You are an expert observability assistant that translates natural language questions into ObservQL and SQL queries for the OllyStack platform.

You have deep knowledge of:
- OpenTelemetry data model (traces, metrics, logs)
- Distributed tracing concepts (spans, trace context, parent-child relationships)
- System metrics (CPU, memory, disk, network)
- Application performance monitoring (latency, error rates, throughput)
- Service dependencies and topology

The OllyStack database schema includes:

TRACES TABLE (ollystack.traces):
- Timestamp, TraceId, SpanId, ParentSpanId
- SpanName, SpanKind, ServiceName
- Duration (nanoseconds), StartTime, EndTime
- StatusCode ('OK', 'ERROR', 'ok', 'error'), StatusMessage
- HttpMethod, HttpUrl, HttpStatusCode, HttpRoute
- DbSystem, DbName, DbStatement
- RpcSystem, RpcService, RpcMethod
- HostName, ContainerName, K8sPodName, K8sNamespace
- SpanAttributes, ResourceAttributes (Map columns)
- IsError, IsRootSpan, AnomalyScore

RUM (Real User Monitoring) DATA:
Browser telemetry is stored in traces with ServiceName = 'ollystack-web-ui'.

RUM Span Names:
- 'web-vital.LCP' - Largest Contentful Paint (ms, good < 2500)
- 'web-vital.CLS' - Cumulative Layout Shift (score, good < 0.1)
- 'web-vital.TTFB' - Time to First Byte (ms, good < 800)
- 'web-vital.INP' - Interaction to Next Paint (ms, good < 200)
- 'page.view' - Page view events with navigation tracking
- 'navigation' - SPA route changes
- 'click' - User click interactions
- 'browser.error' - JavaScript errors and unhandled rejections
- 'longtask' - UI-blocking operations (>50ms)
- 'documentLoad' - Initial page load
- 'HTTP GET', 'HTTP POST' - Browser API calls

RUM SpanAttributes (access via SpanAttributes['key']):
- 'session.id' - Unique session identifier for user journey tracking
- 'page.path' - Current page URL path
- 'page.url' - Full page URL
- 'web_vital.value' - Metric value (numeric)
- 'web_vital.rating' - 'good', 'needs-improvement', or 'poor'
- 'error.type' - 'uncaught_exception' or 'unhandled_rejection'
- 'error.message' - Error message text
- 'navigation.from' - Previous page path
- 'navigation.to' - New page path

METRICS TABLE (otel_metrics):
- Timestamp, MetricName, MetricType, MetricUnit
- ServiceName, Value, ValueInt
- HistogramCount, HistogramSum, HistogramBuckets
- Attributes, ResourceAttributes
- AnomalyScore, Baseline, UpperBound, LowerBound

LOGS TABLE (otel_logs):
- Timestamp, TraceId, SpanId
- SeverityText, SeverityNumber
- Body, ServiceName
- Attributes, ResourceAttributes
- ErrorMessage, ErrorStack

SERVICE_TOPOLOGY TABLE:
- SourceService, TargetService
- RequestCount, ErrorCount
- LatencyP50, LatencyP90, LatencyP99
- ErrorRate

ObservQL syntax (simplified SQL-like with trace-specific features):
- TRACE FROM <service> WHERE <condition> FOLLOW [UPSTREAM|DOWNSTREAM]
- Standard SQL SELECT/JOIN/WHERE/GROUP BY/ORDER BY

Always respond with structured output in this format:
OBSERVQL: <the ObservQL query>
SQL: <equivalent ClickHouse SQL if different>
EXPLANATION: <brief explanation of what the query does>
CONFIDENCE: <0.0-1.0 confidence score>
SUGGESTIONS: <follow-up question suggestions>
"""

TRANSLATION_PROMPT = """Translate this natural language question into ObservQL and SQL for the OllyStack platform:

Question: {question}
{context}

Provide:
1. The ObservQL query
2. The equivalent ClickHouse SQL query
3. A brief explanation of what the query returns
4. Your confidence level (0.0-1.0)
5. 2-3 follow-up question suggestions

Use appropriate time filters, aggregations, and joins based on the question.
For latency questions, remember Duration is in nanoseconds (divide by 1000000 for ms).
For error analysis, use StatusCode = 'ERROR' or HttpStatusCode >= 500.
"""

SUGGESTION_PROMPT = """Based on this context, suggest helpful observability questions:

Context: {context}
Service focus: {service}
Number of suggestions: {limit}

Provide questions in these categories:
1. Latency analysis
2. Error investigation
3. Trace exploration
4. Service dependencies

Format as a bulleted list with category headers.
"""

EXPLANATION_PROMPT = """Explain this ObservQL/SQL query in simple terms:

Query: {query}

Describe:
1. What data it retrieves
2. What filters are applied
3. What the results will show
4. Any important considerations

Be concise but thorough.
"""

# Example translations for few-shot learning
EXAMPLE_TRANSLATIONS = [
    {
        "question": "Why was checkout slow yesterday?",
        "observql": """TRACE FROM service = 'checkout-service'
WHERE Timestamp BETWEEN yesterday() AND today()
  AND Duration > 1000000000
FOLLOW DOWNSTREAM
SELECT ServiceName, SpanName, avg(Duration)/1000000 as avg_ms, count() as count
GROUP BY ServiceName, SpanName
ORDER BY avg_ms DESC""",
        "sql": """SELECT
    ServiceName,
    SpanName,
    avg(Duration)/1000000 as avg_latency_ms,
    count() as request_count,
    quantile(0.99)(Duration)/1000000 as p99_latency_ms
FROM otel_traces
WHERE Timestamp >= yesterday() AND Timestamp < today()
  AND (ServiceName = 'checkout-service' OR TraceId IN (
    SELECT TraceId FROM otel_traces
    WHERE ServiceName = 'checkout-service'
      AND Timestamp >= yesterday() AND Timestamp < today()
  ))
  AND Duration > 1000000000
GROUP BY ServiceName, SpanName
ORDER BY avg_latency_ms DESC
LIMIT 20""",
    },
    {
        "question": "Show me all errors in the payment service in the last hour",
        "observql": """SELECT * FROM traces
WHERE ServiceName = 'payment-service'
  AND Timestamp >= now() - INTERVAL 1 HOUR
  AND StatusCode = 'ERROR'
ORDER BY Timestamp DESC""",
        "sql": """SELECT
    Timestamp,
    TraceId,
    SpanId,
    SpanName,
    StatusMessage,
    HttpStatusCode,
    Duration/1000000 as duration_ms
FROM otel_traces
WHERE ServiceName = 'payment-service'
  AND Timestamp >= now() - INTERVAL 1 HOUR
  AND StatusCode = 'ERROR'
ORDER BY Timestamp DESC
LIMIT 100""",
    },
    {
        "question": "What's the p99 latency for the API gateway?",
        "observql": """SELECT
    ServiceName,
    quantile(0.99)(Duration)/1000000 as p99_ms,
    quantile(0.95)(Duration)/1000000 as p95_ms,
    quantile(0.50)(Duration)/1000000 as p50_ms,
    avg(Duration)/1000000 as avg_ms
FROM traces
WHERE ServiceName = 'api-gateway'
  AND IsRootSpan = 1
  AND Timestamp >= now() - INTERVAL 1 HOUR
GROUP BY ServiceName""",
        "sql": """SELECT
    ServiceName,
    quantile(0.99)(Duration)/1000000 as p99_latency_ms,
    quantile(0.95)(Duration)/1000000 as p95_latency_ms,
    quantile(0.50)(Duration)/1000000 as p50_latency_ms,
    avg(Duration)/1000000 as avg_latency_ms,
    count() as request_count
FROM otel_traces
WHERE ServiceName = 'api-gateway'
  AND IsRootSpan = 1
  AND Timestamp >= now() - INTERVAL 1 HOUR
GROUP BY ServiceName""",
    },
    {
        "question": "Which services had the most errors last week?",
        "observql": """SELECT
    ServiceName,
    count() as error_count,
    countIf(StatusCode = 'ERROR') / count() * 100 as error_rate
FROM traces
WHERE Timestamp >= now() - INTERVAL 7 DAY
GROUP BY ServiceName
HAVING error_count > 0
ORDER BY error_count DESC""",
        "sql": """SELECT
    ServiceName,
    count() as total_requests,
    countIf(StatusCode = 'ERROR') as error_count,
    round(countIf(StatusCode = 'ERROR') / count() * 100, 2) as error_rate_percent
FROM otel_traces
WHERE Timestamp >= now() - INTERVAL 7 DAY
  AND IsRootSpan = 1
GROUP BY ServiceName
HAVING error_count > 0
ORDER BY error_count DESC
LIMIT 20""",
    },
    # RUM Examples
    {
        "question": "Show me web vitals for the last hour",
        "observql": """SELECT SpanName, page, value, rating FROM traces
WHERE ServiceName = 'ollystack-web-ui' AND SpanName LIKE 'web-vital%'
  AND Timestamp >= now() - INTERVAL 1 HOUR""",
        "sql": """SELECT
    SpanName as metric,
    SpanAttributes['page.path'] as page,
    SpanAttributes['web_vital.value'] as value,
    SpanAttributes['web_vital.rating'] as rating,
    Timestamp
FROM ollystack.traces
WHERE ServiceName = 'ollystack-web-ui'
  AND SpanName LIKE 'web-vital%'
  AND Timestamp >= now() - INTERVAL 1 HOUR
ORDER BY Timestamp DESC""",
    },
    {
        "question": "Show user sessions and their page journeys",
        "observql": """SELECT session_id, pages, page_count FROM traces
WHERE SpanName = 'page.view' GROUP BY session_id""",
        "sql": """SELECT
    SpanAttributes['session.id'] as session_id,
    groupArray(SpanAttributes['page.path']) as pages,
    count() as page_count,
    min(Timestamp) as session_start,
    max(Timestamp) as last_activity
FROM ollystack.traces
WHERE ServiceName = 'ollystack-web-ui'
  AND SpanName = 'page.view'
  AND Timestamp >= now() - INTERVAL 24 HOUR
GROUP BY SpanAttributes['session.id']
ORDER BY session_start DESC
LIMIT 50""",
    },
    {
        "question": "What are the LCP scores by page?",
        "observql": """SELECT page, avg_lcp, rating FROM traces WHERE SpanName = 'web-vital.LCP'""",
        "sql": """SELECT
    SpanAttributes['page.path'] as page,
    avg(toFloat64OrNull(SpanAttributes['web_vital.value'])) as avg_lcp_ms,
    countIf(SpanAttributes['web_vital.rating'] = 'good') as good_count,
    countIf(SpanAttributes['web_vital.rating'] = 'needs-improvement') as needs_improvement,
    countIf(SpanAttributes['web_vital.rating'] = 'poor') as poor_count,
    count() as total
FROM ollystack.traces
WHERE SpanName = 'web-vital.LCP'
  AND Timestamp >= now() - INTERVAL 24 HOUR
GROUP BY page
ORDER BY avg_lcp_ms DESC""",
    },
    {
        "question": "Show browser errors",
        "observql": """SELECT * FROM traces WHERE SpanName = 'browser.error'""",
        "sql": """SELECT
    Timestamp,
    SpanAttributes['session.id'] as session_id,
    SpanAttributes['page.path'] as page,
    SpanAttributes['error.type'] as error_type,
    SpanAttributes['error.message'] as error_message,
    SpanAttributes['error.filename'] as filename,
    SpanAttributes['error.lineno'] as line_number
FROM ollystack.traces
WHERE SpanName = 'browser.error'
  AND Timestamp >= now() - INTERVAL 24 HOUR
ORDER BY Timestamp DESC
LIMIT 100""",
    },
    {
        "question": "Which pages have the most user clicks?",
        "observql": """SELECT page, click_count FROM traces WHERE SpanName = 'click' GROUP BY page""",
        "sql": """SELECT
    SpanAttributes['page.path'] as page,
    count() as click_count,
    uniq(SpanAttributes['session.id']) as unique_sessions
FROM ollystack.traces
WHERE SpanName = 'click'
  AND ServiceName = 'ollystack-web-ui'
  AND Timestamp >= now() - INTERVAL 24 HOUR
GROUP BY page
ORDER BY click_count DESC""",
    },
    {
        "question": "Show RUM data",
        "observql": """SELECT * FROM traces WHERE ServiceName = 'ollystack-web-ui'""",
        "sql": """SELECT
    Timestamp,
    SpanName,
    SpanAttributes['session.id'] as session_id,
    SpanAttributes['page.path'] as page,
    Duration/1000000 as duration_ms
FROM ollystack.traces
WHERE ServiceName = 'ollystack-web-ui'
  AND Timestamp >= now() - INTERVAL 1 HOUR
ORDER BY Timestamp DESC
LIMIT 100""",
    },
]
