"""
OllyStack AI/ML Engine
Provides root cause analysis, anomaly detection, and intelligent insights.
"""

import os
import json
import logging
from datetime import datetime, timedelta
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, asdict
from collections import defaultdict
import statistics

from fastapi import FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import clickhouse_connect
import redis
import uuid
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize OpenTelemetry
resource = Resource.create({"service.name": os.getenv("OTEL_SERVICE_NAME", "ai-engine")})
provider = TracerProvider(resource=resource)
otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
processor = BatchSpanProcessor(OTLPSpanExporter(endpoint=otlp_endpoint, insecure=True))
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)
tracer = trace.get_tracer(__name__)

# Initialize FastAPI
app = FastAPI(
    title="OllyStack AI Engine",
    description="AI/ML-powered root cause analysis and anomaly detection",
    version="1.0.0"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

FastAPIInstrumentor.instrument_app(app)

# ClickHouse connection
ch_client = None
redis_client = None


def get_clickhouse():
    global ch_client
    if ch_client is None:
        ch_client = clickhouse_connect.get_client(
            host=os.getenv("CLICKHOUSE_HOST", "localhost"),
            port=int(os.getenv("CLICKHOUSE_PORT", 9000)),
            username=os.getenv("CLICKHOUSE_USER", "ollystack"),
            password=os.getenv("CLICKHOUSE_PASSWORD", "ollystack123"),
            database=os.getenv("CLICKHOUSE_DB", "ollystack")
        )
    return ch_client


def get_redis():
    global redis_client
    if redis_client is None:
        redis_url = os.getenv("REDIS_URL", "redis://localhost:6379")
        redis_client = redis.from_url(redis_url)
    return redis_client


# Data Models
class AnalyzeRequest(BaseModel):
    correlation_id: str
    include_recommendations: bool = True


class AnomalyDetectionRequest(BaseModel):
    service: Optional[str] = None
    metric: str = "latency"
    window_minutes: int = 60
    sensitivity: float = 2.0


class RCARequest(BaseModel):
    correlation_id: str
    depth: int = 3


class NLQRequest(BaseModel):
    question: str
    context: Optional[str] = None


class ChatRequest(BaseModel):
    message: str
    conversation_id: Optional[str] = None
    include_context: bool = True


class CausalAnalysisRequest(BaseModel):
    correlation_id: Optional[str] = None
    trace_id: Optional[str] = None
    time_window_minutes: int = 30


class PredictiveAlertRequest(BaseModel):
    service: Optional[str] = None
    metrics: List[str] = ["latency", "error_rate", "throughput"]
    forecast_minutes: int = 30
    sensitivity: float = 0.8


# LLM Configuration - Groq API (fast inference)
GROQ_API_KEY = os.getenv("GROQ_API_KEY", "")
GROQ_MODEL = os.getenv("GROQ_MODEL", "llama-3.3-70b-versatile")
GROQ_BASE_URL = "https://api.groq.com/openai/v1"
LLM_ENABLED = bool(GROQ_API_KEY)

# Schema context for NLQ
CLICKHOUSE_SCHEMA = """
Tables in 'ollystack' database:

1. traces - Distributed tracing data
   - Timestamp: DateTime64(9) - When the span occurred
   - TraceId: String - Unique trace identifier
   - SpanId: String - Unique span identifier
   - ParentSpanId: String - Parent span (empty for root)
   - SpanName: String - Operation name
   - SpanKind: String - SPAN_KIND_SERVER, SPAN_KIND_CLIENT, etc.
   - ServiceName: String - Service that generated the span
   - Duration: Int64 - Duration in nanoseconds
   - StatusCode: String - STATUS_CODE_OK, STATUS_CODE_ERROR
   - SpanAttributes: Map(String, String) - Key-value attributes including 'correlation_id'

2. logs - Log entries
   - Timestamp: DateTime64(9)
   - TraceId: String - Associated trace
   - SpanId: String - Associated span
   - SeverityText: String - INFO, WARN, ERROR, etc.
   - Body: String - Log message
   - ServiceName: String
   - LogAttributes: Map(String, String) - Including 'correlation_id'

Common queries:
- Filter by time: WHERE Timestamp >= now() - INTERVAL 1 HOUR
- Filter by service: WHERE ServiceName = 'api-gateway'
- Get correlation_id: SpanAttributes['correlation_id']
- Error spans: WHERE StatusCode = 'STATUS_CODE_ERROR'
"""

NLQ_SYSTEM_PROMPT = f"""You are a SQL query generator for an observability platform using ClickHouse.

{CLICKHOUSE_SCHEMA}

Rules:
1. Generate valid ClickHouse SQL only
2. Always include reasonable LIMIT (default 100)
3. Use proper time filtering for recent data
4. Return JSON with: {{"sql": "...", "explanation": "...", "visualization": "table|chart|number"}}
5. For duration, divide by 1000000 to get milliseconds
6. Use SpanAttributes['correlation_id'] to access correlation IDs
"""

CHAT_SYSTEM_PROMPT = """You are an AI assistant for an observability platform called OllyStack.
You help users understand their distributed systems by analyzing traces, logs, and metrics.

You have access to real-time telemetry data and can:
- Explain service health and issues
- Identify root causes of problems
- Suggest optimizations
- Answer questions about system behavior

Be concise and actionable. Use technical terms appropriately.
When showing data, format it clearly with relevant metrics.
"""


@dataclass
class AnomalyResult:
    timestamp: str
    service: str
    metric: str
    value: float
    baseline: float
    deviation: float
    severity: str
    description: str


@dataclass
class RCAResult:
    correlation_id: str
    root_cause: str
    confidence: float
    contributing_factors: List[Dict[str, Any]]
    timeline: List[Dict[str, Any]]
    recommendations: List[str]


# Endpoints
@app.get("/health")
async def health():
    return {"status": "healthy", "service": "ai-engine"}


@app.get("/ready")
async def ready():
    try:
        get_clickhouse().command("SELECT 1")
        return {"status": "ready"}
    except Exception as e:
        raise HTTPException(status_code=503, detail=str(e))


@app.post("/api/v1/analyze")
async def analyze_correlation(request: AnalyzeRequest):
    """Analyze a correlation ID and provide insights."""
    with tracer.start_as_current_span("analyze_correlation") as span:
        span.set_attribute("correlation_id", request.correlation_id)

        ch = get_clickhouse()

        # Get traces for this correlation
        traces = ch.query(f"""
            SELECT
                TraceId,
                SpanId,
                ParentSpanId,
                SpanName,
                ServiceName,
                Duration,
                StatusCode,
                `SpanAttributes.keys` as attr_keys,
                `SpanAttributes.values` as attr_values,
                Timestamp
            FROM traces
            WHERE has(mapKeys(SpanAttributes), 'correlation_id')
              AND SpanAttributes['correlation_id'] = '{request.correlation_id}'
            ORDER BY Timestamp
        """).result_rows

        if not traces:
            raise HTTPException(status_code=404, detail="Correlation not found")

        # Analyze the trace data
        analysis = analyze_traces(traces)

        # Get logs for additional context
        logs = ch.query(f"""
            SELECT
                Timestamp,
                SeverityText,
                Body,
                ServiceName
            FROM logs
            WHERE has(mapKeys(LogAttributes), 'correlation_id')
              AND LogAttributes['correlation_id'] = '{request.correlation_id}'
            ORDER BY Timestamp
        """).result_rows

        analysis["log_analysis"] = analyze_logs(logs)

        if request.include_recommendations:
            analysis["recommendations"] = generate_recommendations(analysis)

        return analysis


@app.post("/api/v1/rca")
async def root_cause_analysis(request: RCARequest):
    """Perform root cause analysis for a correlation ID."""
    with tracer.start_as_current_span("root_cause_analysis") as span:
        span.set_attribute("correlation_id", request.correlation_id)
        span.set_attribute("depth", request.depth)

        ch = get_clickhouse()

        # Get all spans for this correlation
        query = f"""
            SELECT
                TraceId,
                SpanId,
                ParentSpanId,
                SpanName,
                ServiceName,
                Duration,
                StatusCode,
                `Events.Name` as event_names,
                `Events.Attributes` as event_attrs,
                Timestamp
            FROM traces
            WHERE has(mapKeys(SpanAttributes), 'correlation_id')
              AND SpanAttributes['correlation_id'] = '{request.correlation_id}'
            ORDER BY Timestamp
        """

        spans = ch.query(query).result_rows

        if not spans:
            raise HTTPException(status_code=404, detail="Correlation not found")

        # Build span tree and find root cause
        rca_result = perform_rca(spans, request.depth)

        return asdict(rca_result)


@app.post("/api/v1/anomalies/detect")
async def detect_anomalies(request: AnomalyDetectionRequest):
    """Detect anomalies in the specified metric."""
    with tracer.start_as_current_span("detect_anomalies") as span:
        span.set_attribute("metric", request.metric)
        span.set_attribute("window_minutes", request.window_minutes)

        ch = get_clickhouse()

        # Query recent metrics
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(minutes=request.window_minutes)

        service_filter = f"AND ServiceName = '{request.service}'" if request.service else ""

        query = f"""
            SELECT
                ServiceName,
                toStartOfMinute(Timestamp) as minute,
                avg(Duration) as avg_duration,
                quantile(0.95)(Duration) as p95_duration,
                count() as request_count,
                countIf(StatusCode = 'ERROR') as error_count
            FROM traces
            WHERE Timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
              AND Timestamp < '{end_time.strftime('%Y-%m-%d %H:%M:%S')}'
              {service_filter}
            GROUP BY ServiceName, minute
            ORDER BY minute
        """

        metrics = ch.query(query).result_rows

        # Detect anomalies using statistical methods
        anomalies = detect_statistical_anomalies(metrics, request.metric, request.sensitivity)

        return {
            "anomalies": [asdict(a) for a in anomalies],
            "total_anomalies": len(anomalies),
            "window": {
                "start": start_time.isoformat(),
                "end": end_time.isoformat(),
                "minutes": request.window_minutes
            }
        }


@app.get("/api/v1/services/health")
async def service_health(window_minutes: int = Query(default=5)):
    """Get health status of all services."""
    with tracer.start_as_current_span("service_health"):
        ch = get_clickhouse()

        end_time = datetime.utcnow()
        start_time = end_time - timedelta(minutes=window_minutes)

        query = f"""
            SELECT
                ServiceName,
                count() as total_requests,
                countIf(StatusCode = 'ERROR') as errors,
                avg(Duration) as avg_latency,
                quantile(0.95)(Duration) as p95_latency,
                quantile(0.99)(Duration) as p99_latency
            FROM traces
            WHERE Timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
            GROUP BY ServiceName
        """

        results = ch.query(query).result_rows

        services = []
        for row in results:
            service_name, total, errors, avg_lat, p95, p99 = row
            error_rate = (errors / total * 100) if total > 0 else 0

            # Determine health status
            if error_rate > 10 or p95 > 1000000000:  # > 10% errors or > 1s p95
                status = "critical"
            elif error_rate > 5 or p95 > 500000000:
                status = "degraded"
            else:
                status = "healthy"

            services.append({
                "service": service_name,
                "status": status,
                "metrics": {
                    "total_requests": total,
                    "errors": errors,
                    "error_rate": round(error_rate, 2),
                    "avg_latency_ms": round(avg_lat / 1000000, 2),
                    "p95_latency_ms": round(p95 / 1000000, 2),
                    "p99_latency_ms": round(p99 / 1000000, 2)
                }
            })

        return {
            "services": services,
            "window_minutes": window_minutes,
            "timestamp": datetime.utcnow().isoformat()
        }


@app.get("/api/v1/services/metrics/timeseries")
async def service_metrics_timeseries(
    service: Optional[str] = Query(default=None),
    window_minutes: int = Query(default=60),
    bucket_minutes: int = Query(default=1)
):
    """Get time-series metrics for services (for sparklines and charts)."""
    with tracer.start_as_current_span("service_metrics_timeseries"):
        ch = get_clickhouse()

        end_time = datetime.utcnow()
        start_time = end_time - timedelta(minutes=window_minutes)

        # Build service filter
        service_filter = f"AND service_name = '{service}'" if service else ""

        query = f"""
            SELECT
                service_name,
                toStartOfInterval(timestamp, INTERVAL {bucket_minutes} MINUTE) as bucket,
                sum(request_count) as requests,
                sum(error_count) as errors,
                avg(total_duration_ns / request_count) / 1000000 as avg_latency_ms,
                avg(p50_duration_ns) / 1000000 as p50_ms,
                avg(p95_duration_ns) / 1000000 as p95_ms,
                avg(p99_duration_ns) / 1000000 as p99_ms
            FROM service_metrics_1m
            WHERE timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
            {service_filter}
            GROUP BY service_name, bucket
            ORDER BY service_name, bucket
        """

        results = ch.query(query).result_rows

        # Group by service
        services_data = defaultdict(lambda: {
            "timestamps": [],
            "requests": [],
            "errors": [],
            "avg_latency_ms": [],
            "p50_ms": [],
            "p95_ms": [],
            "p99_ms": []
        })

        for row in results:
            svc, bucket, reqs, errs, avg_lat, p50, p95, p99 = row
            services_data[svc]["timestamps"].append(bucket.isoformat() if hasattr(bucket, 'isoformat') else str(bucket))
            services_data[svc]["requests"].append(int(reqs))
            services_data[svc]["errors"].append(int(errs))
            services_data[svc]["avg_latency_ms"].append(round(float(avg_lat or 0), 2))
            services_data[svc]["p50_ms"].append(round(float(p50 or 0), 2))
            services_data[svc]["p95_ms"].append(round(float(p95 or 0), 2))
            services_data[svc]["p99_ms"].append(round(float(p99 or 0), 2))

        return {
            "services": dict(services_data),
            "window_minutes": window_minutes,
            "bucket_minutes": bucket_minutes,
            "timestamp": datetime.utcnow().isoformat()
        }


@app.get("/api/v1/insights")
async def get_insights(hours: int = Query(default=1)):
    """Get AI-generated insights about the system."""
    with tracer.start_as_current_span("get_insights"):
        ch = get_clickhouse()

        end_time = datetime.utcnow()
        start_time = end_time - timedelta(hours=hours)

        # Gather various metrics
        insights = []

        # 1. Error patterns
        error_query = f"""
            SELECT
                ServiceName,
                SpanName,
                count() as error_count
            FROM traces
            WHERE Timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
              AND StatusCode = 'ERROR'
            GROUP BY ServiceName, SpanName
            ORDER BY error_count DESC
            LIMIT 10
        """
        errors = ch.query(error_query).result_rows

        if errors:
            top_error = errors[0]
            insights.append({
                "type": "error_pattern",
                "severity": "high" if top_error[2] > 100 else "medium",
                "title": f"High error rate in {top_error[0]}",
                "description": f"Operation '{top_error[1]}' has {top_error[2]} errors in the last {hours} hour(s)",
                "recommendation": f"Investigate error logs for {top_error[0]} service"
            })

        # 2. Latency outliers
        latency_query = f"""
            SELECT
                ServiceName,
                SpanName,
                avg(Duration) as avg_duration,
                max(Duration) as max_duration
            FROM traces
            WHERE Timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
            GROUP BY ServiceName, SpanName
            HAVING max_duration > avg_duration * 10
            ORDER BY max_duration DESC
            LIMIT 5
        """
        latency_outliers = ch.query(latency_query).result_rows

        for outlier in latency_outliers:
            service, span, avg_dur, max_dur = outlier
            insights.append({
                "type": "latency_spike",
                "severity": "medium",
                "title": f"Latency spike detected in {service}",
                "description": f"'{span}' has max latency {round(max_dur/1000000, 2)}ms vs avg {round(avg_dur/1000000, 2)}ms",
                "recommendation": "Check for resource contention or external dependencies"
            })

        # 3. Service dependency issues
        dep_query = f"""
            SELECT
                s1.ServiceName as caller,
                s2.ServiceName as callee,
                count() as call_count,
                countIf(s2.StatusCode = 'ERROR') as failed_calls
            FROM traces s1
            JOIN traces s2 ON s1.SpanId = s2.ParentSpanId AND s1.TraceId = s2.TraceId
            WHERE s1.Timestamp >= '{start_time.strftime('%Y-%m-%d %H:%M:%S')}'
              AND s1.ServiceName != s2.ServiceName
            GROUP BY caller, callee
            HAVING failed_calls > 0
            ORDER BY failed_calls DESC
            LIMIT 5
        """

        try:
            deps = ch.query(dep_query).result_rows
            for dep in deps:
                caller, callee, calls, failed = dep
                if failed / calls > 0.1:  # > 10% failure rate
                    insights.append({
                        "type": "dependency_issue",
                        "severity": "high" if failed / calls > 0.3 else "medium",
                        "title": f"Dependency issues: {caller} -> {callee}",
                        "description": f"{failed}/{calls} calls failed ({round(failed/calls*100, 1)}%)",
                        "recommendation": f"Check network connectivity and {callee} service health"
                    })
        except Exception:
            pass  # Query might fail if no parent-child relationships exist yet

        return {
            "insights": insights,
            "generated_at": datetime.utcnow().isoformat(),
            "window_hours": hours
        }


@app.post("/api/v1/nlq")
async def natural_language_query(request: NLQRequest):
    """Convert natural language to SQL and execute."""
    with tracer.start_as_current_span("nlq_query") as span:
        span.set_attribute("question", request.question[:100])

        ch = get_clickhouse()

        # Try LLM-based translation if available
        if LLM_ENABLED:
            try:
                import openai
                client = openai.OpenAI(api_key=GROQ_API_KEY, base_url=GROQ_BASE_URL)

                response = client.chat.completions.create(
                    model=GROQ_MODEL,
                    messages=[
                        {"role": "system", "content": NLQ_SYSTEM_PROMPT},
                        {"role": "user", "content": f"{request.question}\n\nRespond with valid JSON only."}
                    ],
                    temperature=0.1
                )

                content = response.choices[0].message.content
                # Try to parse JSON from response
                try:
                    # Handle markdown code blocks
                    if "```json" in content:
                        content = content.split("```json")[1].split("```")[0]
                    elif "```" in content:
                        content = content.split("```")[1].split("```")[0]
                    result = json.loads(content.strip())
                    sql = result.get("sql", "")
                    explanation = result.get("explanation", "")
                    visualization = result.get("visualization", "table")
                except json.JSONDecodeError:
                    # If JSON parsing fails, fall back to rule-based
                    logger.warning(f"Failed to parse LLM JSON response: {content[:200]}")
                    sql, explanation, visualization = translate_nlq_rules(request.question)

            except Exception as e:
                logger.error(f"LLM error: {e}")
                sql, explanation, visualization = translate_nlq_rules(request.question)
        else:
            sql, explanation, visualization = translate_nlq_rules(request.question)

        if not sql:
            return {
                "success": False,
                "error": "Could not translate query",
                "question": request.question
            }

        # Execute the query
        try:
            results = ch.query(sql).result_rows
            columns = ch.query(sql).column_names if hasattr(ch.query(sql), 'column_names') else []

            # Format results
            formatted_results = []
            for row in results[:100]:  # Limit results
                if columns:
                    formatted_results.append(dict(zip(columns, row)))
                else:
                    formatted_results.append(list(row))

            return {
                "success": True,
                "question": request.question,
                "sql": sql,
                "explanation": explanation,
                "visualization": visualization,
                "results": formatted_results,
                "count": len(results)
            }

        except Exception as e:
            return {
                "success": False,
                "question": request.question,
                "sql": sql,
                "error": str(e)
            }


@app.post("/api/v1/chat")
async def ai_chat(request: ChatRequest):
    """AI-powered chat for system insights."""
    with tracer.start_as_current_span("ai_chat") as span:
        span.set_attribute("message_length", len(request.message))

        ch = get_clickhouse()

        # Gather system context if requested
        context_data = {}
        if request.include_context:
            try:
                # Get recent service health
                health_query = """
                    SELECT ServiceName, count() as requests,
                           countIf(StatusCode = 'STATUS_CODE_ERROR') as errors,
                           avg(Duration)/1000000 as avg_ms
                    FROM traces
                    WHERE Timestamp >= now() - INTERVAL 5 MINUTE
                    GROUP BY ServiceName
                """
                health = ch.query(health_query).result_rows
                context_data["service_health"] = [
                    {"service": r[0], "requests": r[1], "errors": r[2], "avg_ms": round(r[3], 2)}
                    for r in health
                ]

                # Get recent errors
                error_query = """
                    SELECT ServiceName, SpanName, count() as cnt
                    FROM traces
                    WHERE Timestamp >= now() - INTERVAL 15 MINUTE
                      AND StatusCode = 'STATUS_CODE_ERROR'
                    GROUP BY ServiceName, SpanName
                    ORDER BY cnt DESC
                    LIMIT 5
                """
                errors = ch.query(error_query).result_rows
                context_data["recent_errors"] = [
                    {"service": r[0], "operation": r[1], "count": r[2]}
                    for r in errors
                ]

            except Exception as e:
                logger.warning(f"Failed to gather context: {e}")

        # Generate response
        if LLM_ENABLED:
            try:
                import openai
                client = openai.OpenAI(api_key=GROQ_API_KEY, base_url=GROQ_BASE_URL)

                messages = [
                    {"role": "system", "content": CHAT_SYSTEM_PROMPT}
                ]

                if context_data:
                    messages.append({
                        "role": "system",
                        "content": f"Current system state:\n{json.dumps(context_data, indent=2)}"
                    })

                messages.append({"role": "user", "content": request.message})

                response = client.chat.completions.create(
                    model=GROQ_MODEL,
                    messages=messages,
                    temperature=0.7
                )

                return {
                    "response": response.choices[0].message.content,
                    "context": context_data if request.include_context else None,
                    "model": GROQ_MODEL
                }

            except Exception as e:
                logger.error(f"LLM chat error: {e}")

        # Fallback response without LLM
        return generate_rule_based_response(request.message, context_data)


@app.post("/api/v1/rca/enhanced")
async def enhanced_rca(request: RCARequest):
    """LLM-enhanced root cause analysis."""
    with tracer.start_as_current_span("enhanced_rca") as span:
        span.set_attribute("correlation_id", request.correlation_id)

        # First, get basic RCA
        basic_rca = await root_cause_analysis(request)

        if not LLM_ENABLED:
            return basic_rca

        # Enhance with LLM analysis
        try:
            import openai
            client = openai.OpenAI(api_key=GROQ_API_KEY, base_url=GROQ_BASE_URL)

            prompt = f"""Analyze this distributed system issue and provide actionable insights:

Correlation ID: {request.correlation_id}
Root Cause (detected): {basic_rca.get('root_cause', 'Unknown')}
Contributing Factors: {json.dumps(basic_rca.get('contributing_factors', []), indent=2)}
Timeline: {json.dumps(basic_rca.get('timeline', [])[:5], indent=2)}

Provide:
1. A clear explanation of what happened
2. The most likely root cause
3. Specific actions to fix the issue
4. How to prevent this in the future

Be concise and actionable."""

            response = client.chat.completions.create(
                model=GROQ_MODEL,
                messages=[
                    {"role": "system", "content": "You are an expert SRE analyzing distributed system issues."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.3
            )

            basic_rca["llm_analysis"] = response.choices[0].message.content
            basic_rca["analysis_model"] = GROQ_MODEL

        except Exception as e:
            logger.error(f"Enhanced RCA error: {e}")
            basic_rca["llm_analysis"] = None

        return basic_rca


@app.post("/api/v1/causal/analyze")
async def causal_analysis(request: CausalAnalysisRequest):
    """Perform causal analysis to determine cause-effect relationships in a trace."""
    with tracer.start_as_current_span("causal_analysis") as span:
        ch = get_clickhouse()

        # Build query based on correlation_id or trace_id
        if request.correlation_id:
            filter_clause = f"SpanAttributes['correlation_id'] = '{request.correlation_id}'"
            span.set_attribute("correlation_id", request.correlation_id)
        elif request.trace_id:
            filter_clause = f"TraceId = '{request.trace_id}'"
            span.set_attribute("trace_id", request.trace_id)
        else:
            # Get recent traces with errors for analysis
            filter_clause = "StatusCode = 'STATUS_CODE_ERROR'"

        query = f"""
            SELECT
                TraceId,
                SpanId,
                ParentSpanId,
                SpanName,
                ServiceName,
                Duration,
                StatusCode,
                Timestamp,
                SpanAttributes
            FROM traces
            WHERE Timestamp >= now() - INTERVAL {request.time_window_minutes} MINUTE
              AND {filter_clause}
            ORDER BY Timestamp
            LIMIT 1000
        """

        spans = ch.query(query).result_rows

        if not spans:
            return {
                "success": False,
                "error": "No matching traces found",
                "causal_chain": [],
                "impact_analysis": {}
            }

        # Build causal graph
        causal_result = build_causal_graph(spans)

        # Analyze impact propagation
        impact = analyze_impact_propagation(spans)

        return {
            "success": True,
            "causal_chain": causal_result["chain"],
            "root_causes": causal_result["root_causes"],
            "impact_analysis": impact,
            "affected_services": causal_result["affected_services"],
            "recommendations": generate_causal_recommendations(causal_result, impact),
            "confidence": causal_result["confidence"]
        }


@app.post("/api/v1/alerts/predict")
async def predictive_alerts(request: PredictiveAlertRequest):
    """Predict potential issues using time series analysis."""
    with tracer.start_as_current_span("predictive_alerts") as span:
        ch = get_clickhouse()

        predictions = []
        service_filter = f"AND ServiceName = '{request.service}'" if request.service else ""

        # Get historical data for each metric
        for metric in request.metrics:
            # Get time series data
            query = f"""
                SELECT
                    ServiceName,
                    toStartOfMinute(Timestamp) as minute,
                    avg(Duration) / 1000000 as avg_latency_ms,
                    count() as request_count,
                    countIf(StatusCode = 'STATUS_CODE_ERROR') as error_count,
                    countIf(StatusCode = 'STATUS_CODE_ERROR') * 100.0 / count() as error_rate
                FROM traces
                WHERE Timestamp >= now() - INTERVAL 2 HOUR
                  {service_filter}
                GROUP BY ServiceName, minute
                ORDER BY ServiceName, minute
            """

            try:
                data = ch.query(query).result_rows

                # Group by service
                service_data = defaultdict(list)
                for row in data:
                    service, minute, latency, requests, errors, err_rate = row
                    service_data[service].append({
                        "minute": minute,
                        "latency": latency,
                        "requests": requests,
                        "error_rate": err_rate
                    })

                # Predict for each service
                for service, points in service_data.items():
                    if len(points) < 10:
                        continue

                    prediction = predict_metric_trend(
                        points,
                        metric,
                        request.forecast_minutes,
                        request.sensitivity
                    )

                    if prediction["alert"]:
                        predictions.append({
                            "service": service,
                            "metric": metric,
                            "current_value": prediction["current"],
                            "predicted_value": prediction["predicted"],
                            "trend": prediction["trend"],
                            "alert_type": prediction["alert_type"],
                            "severity": prediction["severity"],
                            "time_to_threshold": prediction["time_to_threshold"],
                            "confidence": prediction["confidence"],
                            "recommendation": prediction["recommendation"]
                        })

            except Exception as e:
                logger.error(f"Prediction error for {metric}: {e}")

        # Sort by severity
        severity_order = {"critical": 0, "warning": 1, "info": 2}
        predictions.sort(key=lambda x: severity_order.get(x["severity"], 99))

        return {
            "predictions": predictions,
            "forecast_window_minutes": request.forecast_minutes,
            "analyzed_services": len(set(p["service"] for p in predictions)),
            "alert_count": len(predictions),
            "generated_at": datetime.utcnow().isoformat()
        }


@app.get("/api/v1/trends")
async def get_trends(hours: int = Query(default=6), service: Optional[str] = None):
    """Get trend analysis for services."""
    with tracer.start_as_current_span("get_trends"):
        ch = get_clickhouse()

        service_filter = f"AND ServiceName = '{service}'" if service else ""

        query = f"""
            SELECT
                ServiceName,
                toStartOfFiveMinutes(Timestamp) as bucket,
                count() as requests,
                countIf(StatusCode = 'STATUS_CODE_ERROR') as errors,
                avg(Duration) / 1000000 as avg_latency_ms,
                quantile(0.95)(Duration) / 1000000 as p95_latency_ms,
                quantile(0.99)(Duration) / 1000000 as p99_latency_ms
            FROM traces
            WHERE Timestamp >= now() - INTERVAL {hours} HOUR
              {service_filter}
            GROUP BY ServiceName, bucket
            ORDER BY ServiceName, bucket
        """

        results = ch.query(query).result_rows

        # Organize by service
        trends = defaultdict(lambda: {"data_points": [], "summary": {}})

        for row in results:
            svc, bucket, requests, errors, avg_lat, p95, p99 = row
            error_rate = (errors / requests * 100) if requests > 0 else 0

            trends[svc]["data_points"].append({
                "timestamp": bucket.isoformat() if hasattr(bucket, 'isoformat') else str(bucket),
                "requests": requests,
                "errors": errors,
                "error_rate": round(error_rate, 2),
                "avg_latency_ms": round(avg_lat, 2),
                "p95_latency_ms": round(p95, 2),
                "p99_latency_ms": round(p99, 2)
            })

        # Calculate trend summaries
        for svc, data in trends.items():
            points = data["data_points"]
            if len(points) >= 2:
                first_half = points[:len(points)//2]
                second_half = points[len(points)//2:]

                first_avg_latency = sum(p["avg_latency_ms"] for p in first_half) / len(first_half)
                second_avg_latency = sum(p["avg_latency_ms"] for p in second_half) / len(second_half)

                first_error_rate = sum(p["error_rate"] for p in first_half) / len(first_half)
                second_error_rate = sum(p["error_rate"] for p in second_half) / len(second_half)

                latency_change = ((second_avg_latency - first_avg_latency) / first_avg_latency * 100) if first_avg_latency > 0 else 0
                error_change = second_error_rate - first_error_rate

                data["summary"] = {
                    "latency_trend": "increasing" if latency_change > 10 else "decreasing" if latency_change < -10 else "stable",
                    "latency_change_percent": round(latency_change, 1),
                    "error_trend": "increasing" if error_change > 1 else "decreasing" if error_change < -1 else "stable",
                    "error_change_points": round(error_change, 2),
                    "total_requests": sum(p["requests"] for p in points),
                    "total_errors": sum(p["errors"] for p in points)
                }

        return {
            "trends": dict(trends),
            "hours": hours,
            "generated_at": datetime.utcnow().isoformat()
        }


def build_causal_graph(spans: List) -> Dict:
    """Build a causal graph from spans to identify cause-effect relationships."""
    span_map = {}
    children = defaultdict(list)
    error_spans = []
    root_spans = []

    for span in spans:
        trace_id, span_id, parent_id, name, service, duration, status, ts, attrs = span

        span_data = {
            "span_id": span_id,
            "parent_id": parent_id,
            "trace_id": trace_id,
            "name": name,
            "service": service,
            "duration_ms": duration / 1000000,
            "status": status,
            "timestamp": ts,
            "is_error": status == "STATUS_CODE_ERROR"
        }
        span_map[span_id] = span_data

        if parent_id:
            children[parent_id].append(span_id)
        else:
            root_spans.append(span_id)

        if status == "STATUS_CODE_ERROR":
            error_spans.append(span_data)

    # Build causal chains
    causal_chains = []
    root_causes = []
    affected_services = set()

    for error in sorted(error_spans, key=lambda x: x["timestamp"]):
        # Trace back to find root cause
        chain = []
        current = error
        visited = set()

        while current and current["span_id"] not in visited:
            visited.add(current["span_id"])
            chain.append({
                "service": current["service"],
                "operation": current["name"],
                "duration_ms": round(current["duration_ms"], 2),
                "is_error": current["is_error"],
                "timestamp": str(current["timestamp"])
            })
            affected_services.add(current["service"])

            parent_id = current.get("parent_id")
            current = span_map.get(parent_id) if parent_id else None

        if chain:
            chain.reverse()  # Root to leaf order
            causal_chains.append(chain)

            # The deepest error in the chain is likely the root cause
            error_in_chain = [c for c in chain if c["is_error"]]
            if error_in_chain:
                root_causes.append({
                    "service": error_in_chain[0]["service"],
                    "operation": error_in_chain[0]["operation"],
                    "impact_count": len([s for s in error_spans if s["service"] == error_in_chain[0]["service"]])
                })

    # Deduplicate root causes
    unique_causes = {}
    for cause in root_causes:
        key = f"{cause['service']}.{cause['operation']}"
        if key not in unique_causes or cause["impact_count"] > unique_causes[key]["impact_count"]:
            unique_causes[key] = cause

    return {
        "chain": causal_chains[:10],  # Limit to 10 chains
        "root_causes": list(unique_causes.values()),
        "affected_services": list(affected_services),
        "confidence": 0.85 if error_spans else 0.5
    }


def analyze_impact_propagation(spans: List) -> Dict:
    """Analyze how errors propagate through the system."""
    service_impacts = defaultdict(lambda: {
        "error_count": 0,
        "affected_operations": set(),
        "downstream_impact": 0,
        "upstream_cause": 0
    })

    # Analyze each span
    for span in spans:
        trace_id, span_id, parent_id, name, service, duration, status, ts, attrs = span

        if status == "STATUS_CODE_ERROR":
            service_impacts[service]["error_count"] += 1
            service_impacts[service]["affected_operations"].add(name)

            # Check if this is a downstream effect (has parent with error)
            if parent_id:
                service_impacts[service]["upstream_cause"] += 1
            else:
                service_impacts[service]["downstream_impact"] += 1

    # Format output
    impact_summary = {}
    for service, impact in service_impacts.items():
        impact_summary[service] = {
            "error_count": impact["error_count"],
            "affected_operations": list(impact["affected_operations"]),
            "is_root_cause": impact["downstream_impact"] > impact["upstream_cause"],
            "propagation_score": round(impact["downstream_impact"] / max(impact["error_count"], 1), 2)
        }

    return impact_summary


def generate_causal_recommendations(causal: Dict, impact: Dict) -> List[str]:
    """Generate recommendations based on causal analysis."""
    recommendations = []

    # Based on root causes
    for cause in causal.get("root_causes", [])[:3]:
        recommendations.append(
            f"Investigate {cause['service']}.{cause['operation']} - identified as a root cause affecting {cause['impact_count']} downstream operations"
        )

    # Based on impact
    root_cause_services = [
        svc for svc, data in impact.items()
        if data.get("is_root_cause")
    ]

    if root_cause_services:
        recommendations.append(
            f"Focus remediation on services: {', '.join(root_cause_services)}"
        )

    # Generic recommendations
    if len(causal.get("affected_services", [])) > 3:
        recommendations.append(
            "Consider implementing circuit breakers to prevent cascading failures"
        )

    if not recommendations:
        recommendations.append("No critical issues detected in the analyzed timeframe")

    return recommendations


def predict_metric_trend(data_points: List[Dict], metric: str, forecast_minutes: int, sensitivity: float) -> Dict:
    """Predict future metric values using simple linear regression and anomaly detection."""
    # Extract the relevant metric values
    if metric == "latency":
        values = [p["latency"] for p in data_points]
        threshold_high = 500  # ms
        unit = "ms"
    elif metric == "error_rate":
        values = [p["error_rate"] for p in data_points]
        threshold_high = 5  # percent
        unit = "%"
    else:  # throughput
        values = [p["requests"] for p in data_points]
        threshold_high = None  # No threshold for throughput
        unit = "req/min"

    if len(values) < 5:
        return {"alert": False}

    # Calculate trend using simple linear regression
    n = len(values)
    x = list(range(n))
    x_mean = sum(x) / n
    y_mean = sum(values) / n

    numerator = sum((x[i] - x_mean) * (values[i] - y_mean) for i in range(n))
    denominator = sum((x[i] - x_mean) ** 2 for i in range(n))

    slope = numerator / denominator if denominator != 0 else 0
    intercept = y_mean - slope * x_mean

    # Predict future value
    future_x = n + (forecast_minutes // 5)  # Assuming 5-minute intervals
    predicted = slope * future_x + intercept
    current = values[-1]

    # Determine trend
    if slope > 0.1 * y_mean / n:
        trend = "increasing"
    elif slope < -0.1 * y_mean / n:
        trend = "decreasing"
    else:
        trend = "stable"

    # Check for alerts
    alert = False
    alert_type = None
    severity = "info"
    recommendation = ""
    time_to_threshold = None

    if threshold_high and predicted > threshold_high:
        alert = True
        alert_type = "threshold_breach"

        if predicted > threshold_high * 2:
            severity = "critical"
        else:
            severity = "warning"

        # Calculate time to threshold
        if slope > 0 and current < threshold_high:
            steps_to_threshold = (threshold_high - current) / slope
            time_to_threshold = int(steps_to_threshold * 5)  # minutes
            recommendation = f"Expected to breach threshold in ~{time_to_threshold} minutes. Consider scaling or investigating."
        else:
            recommendation = f"Already above threshold. Investigate immediately."

    elif metric == "latency" and trend == "increasing" and slope > 0.5:
        alert = True
        alert_type = "trend_warning"
        severity = "warning"
        recommendation = "Latency is trending upward. Check for resource contention or new deployments."

    elif metric == "error_rate" and current > 1 and trend == "increasing":
        alert = True
        alert_type = "error_increase"
        severity = "warning" if current < 5 else "critical"
        recommendation = "Error rate is increasing. Check recent deployments and downstream dependencies."

    # Calculate confidence based on data stability
    variance = sum((v - y_mean) ** 2 for v in values) / n
    stability = 1 / (1 + variance / (y_mean ** 2 + 0.01))
    confidence = round(min(stability * sensitivity, 0.95), 2)

    return {
        "alert": alert,
        "alert_type": alert_type,
        "severity": severity,
        "current": round(current, 2),
        "predicted": round(max(predicted, 0), 2),
        "trend": trend,
        "time_to_threshold": time_to_threshold,
        "confidence": confidence,
        "recommendation": recommendation
    }


def translate_nlq_rules(question: str) -> tuple:
    """Rule-based NLQ translation (fallback when no LLM)."""
    question_lower = question.lower()

    # Common patterns
    if "error" in question_lower and "service" in question_lower:
        sql = """
            SELECT ServiceName, count() as error_count
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 1 HOUR
              AND StatusCode = 'STATUS_CODE_ERROR'
            GROUP BY ServiceName
            ORDER BY error_count DESC
            LIMIT 20
        """
        return sql, "Showing error counts by service in the last hour", "table"

    elif "slow" in question_lower or "latency" in question_lower:
        sql = """
            SELECT ServiceName, SpanName,
                   avg(Duration)/1000000 as avg_ms,
                   max(Duration)/1000000 as max_ms,
                   count() as count
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 1 HOUR
            GROUP BY ServiceName, SpanName
            HAVING avg_ms > 100
            ORDER BY avg_ms DESC
            LIMIT 20
        """
        return sql, "Showing slow operations (>100ms avg) in the last hour", "table"

    elif "correlation" in question_lower:
        sql = """
            SELECT SpanAttributes['correlation_id'] as correlation_id,
                   min(Timestamp) as start_time,
                   count() as span_count,
                   countIf(StatusCode = 'STATUS_CODE_ERROR') as errors
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 1 HOUR
              AND length(SpanAttributes['correlation_id']) > 0
            GROUP BY correlation_id
            ORDER BY start_time DESC
            LIMIT 50
        """
        return sql, "Showing recent correlations with their span and error counts", "table"

    elif "service" in question_lower and ("health" in question_lower or "status" in question_lower):
        sql = """
            SELECT ServiceName,
                   count() as requests,
                   countIf(StatusCode = 'STATUS_CODE_ERROR') as errors,
                   round(avg(Duration)/1000000, 2) as avg_latency_ms,
                   round(quantile(0.95)(Duration)/1000000, 2) as p95_ms
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 5 MINUTE
            GROUP BY ServiceName
            ORDER BY requests DESC
        """
        return sql, "Showing service health metrics for the last 5 minutes", "table"

    elif "top" in question_lower or "most" in question_lower:
        sql = """
            SELECT ServiceName, SpanName, count() as count
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 1 HOUR
            GROUP BY ServiceName, SpanName
            ORDER BY count DESC
            LIMIT 20
        """
        return sql, "Showing most frequent operations in the last hour", "table"

    else:
        # Generic recent traces
        sql = """
            SELECT Timestamp, ServiceName, SpanName,
                   Duration/1000000 as duration_ms, StatusCode
            FROM traces
            WHERE Timestamp >= now() - INTERVAL 15 MINUTE
            ORDER BY Timestamp DESC
            LIMIT 50
        """
        return sql, "Showing recent traces from the last 15 minutes", "table"


def generate_rule_based_response(message: str, context: Dict) -> Dict:
    """Generate response without LLM using rules."""
    message_lower = message.lower()

    # Analyze context
    health = context.get("service_health", [])
    errors = context.get("recent_errors", [])

    total_requests = sum(s.get("requests", 0) for s in health)
    total_errors = sum(s.get("errors", 0) for s in health)
    error_rate = (total_errors / total_requests * 100) if total_requests > 0 else 0

    if "health" in message_lower or "status" in message_lower:
        if error_rate > 5:
            response = f"⚠️ System is experiencing elevated errors ({error_rate:.1f}% error rate).\n\n"
        else:
            response = f"✅ System is healthy with {error_rate:.1f}% error rate.\n\n"

        response += "**Service Health:**\n"
        for s in health[:5]:
            status = "🔴" if s["errors"] > 0 else "🟢"
            response += f"- {status} {s['service']}: {s['requests']} requests, {s['errors']} errors, {s['avg_ms']:.0f}ms avg\n"

        return {"response": response, "context": context, "model": "rule-based"}

    elif "error" in message_lower:
        if errors:
            response = "**Recent Errors:**\n"
            for e in errors:
                response += f"- {e['service']}.{e['operation']}: {e['count']} occurrences\n"
        else:
            response = "No recent errors detected in the last 15 minutes."

        return {"response": response, "context": context, "model": "rule-based"}

    else:
        response = f"""I can help you understand your system. Here's what I can tell you:

**Current Status:**
- Total requests (5min): {total_requests}
- Error rate: {error_rate:.1f}%
- Active services: {len(health)}

Try asking:
- "What's the system health?"
- "Show me recent errors"
- "Which services are slow?"
"""
        return {"response": response, "context": context, "model": "rule-based"}


# Analysis functions
def analyze_traces(traces: List) -> Dict[str, Any]:
    """Analyze trace data to extract insights."""
    services = defaultdict(lambda: {"count": 0, "errors": 0, "total_duration": 0})
    errors = []
    slow_spans = []

    for trace in traces:
        trace_id, span_id, parent_id, span_name, service, duration, status, attr_keys, attr_values, timestamp = trace

        services[service]["count"] += 1
        services[service]["total_duration"] += duration

        if status == "ERROR":
            services[service]["errors"] += 1
            errors.append({
                "service": service,
                "span": span_name,
                "timestamp": str(timestamp)
            })

        # Flag slow spans (> 500ms)
        if duration > 500000000:
            slow_spans.append({
                "service": service,
                "span": span_name,
                "duration_ms": round(duration / 1000000, 2),
                "timestamp": str(timestamp)
            })

    # Calculate aggregates
    service_stats = []
    for service, stats in services.items():
        avg_duration = stats["total_duration"] / stats["count"] if stats["count"] > 0 else 0
        error_rate = stats["errors"] / stats["count"] * 100 if stats["count"] > 0 else 0
        service_stats.append({
            "service": service,
            "request_count": stats["count"],
            "error_count": stats["errors"],
            "error_rate": round(error_rate, 2),
            "avg_duration_ms": round(avg_duration / 1000000, 2)
        })

    return {
        "total_spans": len(traces),
        "services": service_stats,
        "errors": errors,
        "slow_spans": slow_spans,
        "has_errors": len(errors) > 0,
        "has_latency_issues": len(slow_spans) > 0
    }


def analyze_logs(logs: List) -> Dict[str, Any]:
    """Analyze log data for patterns."""
    severity_counts = defaultdict(int)
    error_messages = []

    for log in logs:
        timestamp, severity, body, service = log
        severity_counts[severity] += 1

        if severity in ["ERROR", "FATAL", "CRITICAL"]:
            error_messages.append({
                "service": service,
                "message": body[:200] if body else "",
                "timestamp": str(timestamp)
            })

    return {
        "total_logs": len(logs),
        "by_severity": dict(severity_counts),
        "error_messages": error_messages[:10]  # Limit to 10
    }


def generate_recommendations(analysis: Dict) -> List[str]:
    """Generate actionable recommendations based on analysis."""
    recommendations = []

    if analysis.get("has_errors"):
        error_services = set(e["service"] for e in analysis.get("errors", []))
        for service in error_services:
            recommendations.append(f"Investigate error handling in {service} service")

    if analysis.get("has_latency_issues"):
        slow_services = set(s["service"] for s in analysis.get("slow_spans", []))
        for service in slow_services:
            recommendations.append(f"Profile and optimize slow operations in {service}")

    log_analysis = analysis.get("log_analysis", {})
    if log_analysis.get("by_severity", {}).get("ERROR", 0) > 10:
        recommendations.append("High error log volume detected - review error handling patterns")

    if not recommendations:
        recommendations.append("No immediate issues detected - system appears healthy")

    return recommendations


def perform_rca(spans: List, depth: int) -> RCAResult:
    """Perform root cause analysis on spans."""
    # Build span tree
    span_map = {}
    children = defaultdict(list)
    root_spans = []

    for span in spans:
        trace_id, span_id, parent_id, span_name, service, duration, status, event_names, event_attrs, timestamp = span

        span_data = {
            "span_id": span_id,
            "parent_id": parent_id,
            "name": span_name,
            "service": service,
            "duration": duration,
            "status": status,
            "timestamp": timestamp,
            "events": list(zip(event_names, event_attrs)) if event_names else []
        }
        span_map[span_id] = span_data

        if parent_id:
            children[parent_id].append(span_id)
        else:
            root_spans.append(span_id)

    # Find error spans and trace back
    error_spans = [s for s in span_map.values() if s["status"] == "ERROR"]

    if not error_spans:
        # No errors - look for slow spans
        avg_duration = sum(s["duration"] for s in span_map.values()) / len(span_map)
        slow_spans = [s for s in span_map.values() if s["duration"] > avg_duration * 2]

        if slow_spans:
            slowest = max(slow_spans, key=lambda x: x["duration"])
            return RCAResult(
                correlation_id=spans[0][0][:16] if spans else "",
                root_cause=f"Latency bottleneck in {slowest['service']}.{slowest['name']}",
                confidence=0.7,
                contributing_factors=[
                    {
                        "factor": "Slow operation",
                        "service": slowest["service"],
                        "span": slowest["name"],
                        "duration_ms": round(slowest["duration"] / 1000000, 2)
                    }
                ],
                timeline=[
                    {"time": str(s["timestamp"]), "event": f"{s['service']}.{s['name']}", "status": s["status"]}
                    for s in sorted(span_map.values(), key=lambda x: x["timestamp"])[:10]
                ],
                recommendations=[
                    f"Optimize {slowest['service']}.{slowest['name']} operation",
                    "Consider caching or async processing",
                    "Profile the operation for bottlenecks"
                ]
            )

    # Find the first error (likely root cause)
    first_error = min(error_spans, key=lambda x: x["timestamp"])

    # Trace the error path
    contributing_factors = []
    current = first_error
    while current and len(contributing_factors) < depth:
        contributing_factors.append({
            "factor": f"Error in {current['service']}",
            "service": current["service"],
            "span": current["name"],
            "status": current["status"]
        })
        parent_id = current.get("parent_id")
        current = span_map.get(parent_id)

    return RCAResult(
        correlation_id=spans[0][0][:16] if spans else "",
        root_cause=f"Error originated in {first_error['service']}.{first_error['name']}",
        confidence=0.85,
        contributing_factors=contributing_factors,
        timeline=[
            {"time": str(s["timestamp"]), "event": f"{s['service']}.{s['name']}", "status": s["status"]}
            for s in sorted(span_map.values(), key=lambda x: x["timestamp"])[:10]
        ],
        recommendations=[
            f"Review error handling in {first_error['service']}",
            "Check for external dependency failures",
            "Verify input validation",
            "Review recent deployments to this service"
        ]
    )


def detect_statistical_anomalies(metrics: List, metric_name: str, sensitivity: float) -> List[AnomalyResult]:
    """Detect anomalies using statistical methods (Z-score)."""
    if not metrics:
        return []

    # Group by service
    service_data = defaultdict(list)
    for row in metrics:
        service, minute, avg_dur, p95, count, errors = row

        if metric_name == "latency":
            value = avg_dur
        elif metric_name == "error_rate":
            value = (errors / count * 100) if count > 0 else 0
        else:
            value = count

        service_data[service].append((minute, value))

    anomalies = []

    for service, data_points in service_data.items():
        if len(data_points) < 5:
            continue

        values = [d[1] for d in data_points]
        mean = statistics.mean(values)
        stdev = statistics.stdev(values) if len(values) > 1 else 0

        if stdev == 0:
            continue

        for minute, value in data_points:
            z_score = (value - mean) / stdev

            if abs(z_score) > sensitivity:
                severity = "critical" if abs(z_score) > sensitivity * 2 else "warning"
                anomalies.append(AnomalyResult(
                    timestamp=str(minute),
                    service=service,
                    metric=metric_name,
                    value=round(value, 2),
                    baseline=round(mean, 2),
                    deviation=round(z_score, 2),
                    severity=severity,
                    description=f"{metric_name} is {round(abs(z_score), 1)} standard deviations from baseline"
                ))

    return anomalies


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)
