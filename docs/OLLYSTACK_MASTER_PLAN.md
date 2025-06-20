# OllyStack Master Plan
## AI/ML-Native Observability Platform with Strategic Integrations

**Version:** 2.1
**Status:** MVP Deployed (Single VM)
**Last Updated:** 2026-02-03

### Current Deployment
- **Environment:** AWS EC2 t3.medium (on-demand)
- **Region:** us-east-1c
- **Services Running:** API Server, Web UI, OTel Collector, ClickHouse, Redis
- **Access:** http://18.209.179.35:3000 (Web UI), http://18.209.179.35:8080 (API)

---

## Executive Summary

OllyStack is an AI/ML-native observability platform that solves the fundamental problem existing platforms ignore: **correlation from the very first request**. Rather than rebuilding the entire observability stack, OllyStack strategically integrates best-in-class open source tools while building unique differentiation in **correlation intelligence** and **AI-powered insights**.

### Core Philosophy

1. **Correlation First**: Every signal (trace, log, metric, event) is correlated from inception
2. **Integrate, Don't Reinvent**: Use Grafana, OTel Collector, Vector, Alertmanager where they excel
3. **Build the Differentiators**: Focus engineering on correlation engine and AI insights
4. **AI-Native**: Built with ML/LLM integration at the core, not bolted on
5. **Zero-Code Priority**: eBPF and auto-instrumentation before manual SDKs
6. **OTel Native**: OpenTelemetry as the universal data format

---

## Strategic Integration Decisions

### Build vs Integrate Matrix

| Component | Decision | Tool | Rationale |
|-----------|----------|------|-----------|
| Dashboards | **INTEGRATE** | Grafana | 10+ years of refinement, 1000+ plugins |
| Trace Collection | **INTEGRATE** | OTel Collector | Industry standard, 100+ receivers |
| Log Shipping | **INTEGRATE** | Vector | Rust-based, efficient, battle-tested |
| Alert Routing | **INTEGRATE** | Alertmanager | Proven routing, silencing, grouping |
| Metrics Scraping | **INTEGRATE** | Prometheus | De facto standard |
| Storage | **KEEP** | ClickHouse | Right choice - fast, cheap, OTel-native |
| Correlation Engine | **BUILD** | OllyStack | **Core differentiator** |
| AI/ML Insights | **BUILD** | OllyStack | **Core differentiator** |
| Correlation UI | **BUILD** | OllyStack | Grafana can't do this |
| OTel Processor | **BUILD** | OllyStack | Custom correlation logic |

### What This Saves

| Build from Scratch | Integrate Instead | Engineering Time Saved |
|-------------------|-------------------|----------------------|
| Dashboard system | Grafana | 6+ months |
| Alert routing | Alertmanager | 2+ months |
| Log collection | Vector | 2+ months |
| Trace receivers | OTel Collector | 3+ months |
| 100+ integrations | OTel ecosystem | Years |

**Total: 12+ months redirected to differentiation**

---

## Problem Statement

### What Current Platforms Get Wrong

| Platform | Limitation |
|----------|------------|
| **Datadog** | Correlation requires APM + Logs + RUM licenses separately; correlation is query-time, not ingestion-time |
| **Splunk** | Powerful search, but trace-log correlation requires manual field extraction |
| **Elastic** | Great for logs, but distributed tracing is a separate product (Elastic APM) |
| **CloudWatch** | AWS-centric, poor cross-service correlation, X-Ray separate from Logs |
| **New Relic** | Good correlation but expensive at scale, proprietary agents |
| **Dynatrace** | Excellent auto-discovery but vendor lock-in, opaque pricing |
| **Grafana Stack** | Great visualization but no native correlation engine |

### The Missing Piece: Correlation ID from Day Zero

```
Current State (Even with Grafana/Elastic/etc):
┌─────────┐    ┌─────────┐    ┌─────────┐
│ Service │ → │ Service │ → │ Service │
│    A    │    │    B    │    │    C    │
└────┬────┘    └────┬────┘    └────┬────┘
     │              │              │
     ▼              ▼              ▼
  Traces         Traces         Traces    ← Different trace IDs!
  Logs           Logs           Logs      ← No correlation!
  Metrics        Metrics        Metrics   ← Lost context!
```

```
OllyStack Vision:
┌─────────────────────────────────────────────────────────────────┐
│                    Correlation ID: olly-2k8f9x3-a7b2c4d1        │
├─────────┬───────────┬───────────┬───────────┬──────────────────┤
│ Browser │ Gateway   │ Service A │ Service B │ Database         │
│ RUM     │ Entry     │ Business  │ Payment   │ Postgres         │
├─────────┴───────────┴───────────┴───────────┴──────────────────┤
│ Traces  : correlation_id propagated through all spans          │
│ Logs    : correlation_id in every log line                     │
│ Metrics : exemplars linked to correlation_id                   │
│ Events  : user actions tagged with correlation_id              │
│ Errors  : exception fingerprints grouped by correlation        │
└─────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────┐
│  GET /api/v1/correlate/olly-2k8f9x3-a7b2c4d1                    │
│  → Returns ALL signals in one response                          │
│  → AI analysis: "Root cause: DB connection pool exhausted"      │
│  → Timeline: Chronological view of entire request flow          │
└─────────────────────────────────────────────────────────────────┘
```

---

## Architecture Overview (Integrated Approach)

### System Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                              DATA SOURCES                                 │
├──────────────────────────────────────────────────────────────────────────┤
│  Browser/Mobile    │  Applications     │  Infrastructure  │  External    │
│  (OTel Web SDK)    │  (OTel SDK/Auto)  │  (Node Exporter) │  (Webhooks)  │
└─────────┬──────────┴─────────┬─────────┴────────┬─────────┴──────┬───────┘
          │                    │                   │                │
          ▼                    ▼                   ▼                ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         COLLECTION LAYER                                  │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐   │
│  │ Grafana Beyla   │  │ Vector          │  │ Prometheus              │   │
│  │ (eBPF - Zero    │  │ (Log Collection │  │ (Metrics Scraping)      │   │
│  │  Code Tracing)  │  │  & Transform)   │  │                         │   │
│  └────────┬────────┘  └────────┬────────┘  └────────────┬────────────┘   │
│           │                    │                        │                 │
│           └────────────────────┼────────────────────────┘                 │
│                                ▼                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │                   OpenTelemetry Collector                          │  │
│  │  ┌──────────────────────────────────────────────────────────────┐  │  │
│  │  │           OLLYSTACK CORRELATION PROCESSOR                    │  │  │
│  │  │  • Extract correlation_id from headers/baggage               │  │  │
│  │  │  • Generate correlation_id if missing                        │  │  │
│  │  │  • Enrich all signals (traces, logs, metrics)                │  │  │
│  │  │  • Propagate via W3C baggage                                 │  │  │
│  │  └──────────────────────────────────────────────────────────────┘  │  │
│  │                                                                    │  │
│  │  Built-in: batch, memory_limiter, resource detection              │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                                                           │
└──────────────────────────────────┬────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                          STORAGE LAYER                                    │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐     │
│  │                        ClickHouse                                │     │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │     │
│  │  │   Traces    │  │    Logs     │  │       Metrics           │  │     │
│  │  │ correlation │  │ correlation │  │  metric_exemplars       │  │     │
│  │  │ _id indexed │  │ _id indexed │  │  (trace linkage)        │  │     │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │     │
│  │                                                                  │     │
│  │  Materialized Views:                                             │     │
│  │  • correlation_summary (pre-aggregated correlation stats)        │     │
│  │  • service_topology (auto-discovered dependencies)               │     │
│  │  • error_fingerprints (grouped error patterns)                   │     │
│  │  • metric_anomalies (z-score deviations)                         │     │
│  └─────────────────────────────────────────────────────────────────┘     │
│                                                                           │
│  ┌─────────────────┐                                                     │
│  │     Redis       │  Cache layer for hot queries                        │
│  └─────────────────┘                                                     │
│                                                                           │
└──────────────────────────────────┬────────────────────────────────────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│                     │  │                     │  │                     │
│   OLLYSTACK API     │  │      GRAFANA        │  │   ALERTMANAGER      │
│   (Go)              │  │   (Visualization)   │  │   (Alert Routing)   │
│                     │  │                     │  │                     │
│  Unique Endpoints:  │  │  • Dashboards       │  │  • PagerDuty        │
│  • /correlate/{id}  │  │  • Explore          │  │  • Slack            │
│  • /ai/analyze      │  │  • Alerting UI      │  │  • Email            │
│  • /ai/rca          │  │  • Tempo traces     │  │  • Grouping         │
│  • /ai/nlq          │  │  • Loki logs        │  │  • Silencing        │
│                     │  │                     │  │                     │
└──────────┬──────────┘  └──────────┬──────────┘  └─────────────────────┘
           │                        │
           │                        │
           ▼                        ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                       OLLYSTACK AI ENGINE (Python)                        │
├──────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐   │
│  │ Anomaly         │  │ Root Cause      │  │ Natural Language        │   │
│  │ Detection       │  │ Analysis        │  │ Query (NLQ)             │   │
│  │                 │  │                 │  │                         │   │
│  │ • Z-score       │  │ • Critical path │  │ • Intent parsing        │   │
│  │ • Seasonal      │  │ • Error origin  │  │ • Query generation      │   │
│  │ • Isolation     │  │ • LLM reasoning │  │ • Result summarization  │   │
│  │   Forest        │  │                 │  │                         │   │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────┘   │
│                                                                           │
│  LLM Providers: OpenAI / Claude / Local (Ollama)                         │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                          PRESENTATION LAYER                               │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │                      OLLYSTACK WEB UI (React)                      │  │
│  ├─────────────────────┬─────────────────────┬────────────────────────┤  │
│  │                     │                     │                        │  │
│  │  CORRELATION        │  EMBEDDED GRAFANA   │  AI INSIGHTS           │  │
│  │  EXPLORER           │  DASHBOARDS         │  PANEL                 │  │
│  │  (UNIQUE)           │  (INTEGRATED)       │  (UNIQUE)              │  │
│  │                     │                     │                        │  │
│  │  • Unified timeline │  • <iframe> embeds  │  • Root cause display  │  │
│  │  • Cross-signal     │  • Custom panels    │  • Recommendations     │  │
│  │    navigation       │  • Explore links    │  • NLQ chat interface  │  │
│  │  • Service flow     │                     │  • Anomaly highlights  │  │
│  │                     │                     │                        │  │
│  └─────────────────────┴─────────────────────┴────────────────────────┘  │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Component Deep Dive

### 1. OpenTelemetry Collector with OllyStack Processor

The OTel Collector is the central nervous system. We add a **custom processor** for correlation.

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  prometheus:
    config:
      scrape_configs:
        - job_name: 'kubernetes-pods'
          kubernetes_sd_configs:
            - role: pod

  # Receive from Vector (logs)
  fluentforward:
    endpoint: 0.0.0.0:8006

processors:
  # OLLYSTACK CUSTOM PROCESSOR
  ollystack_correlation:
    # Generate correlation_id if not present
    generate_if_missing: true
    # Format: olly-{timestamp_base36}-{random}
    id_prefix: "olly"
    # Headers to check for existing correlation ID
    extract_from_headers:
      - X-Correlation-ID
      - X-Request-ID
      - x-correlation-id
    # Also check baggage
    extract_from_baggage: true
    # Add to all signals
    enrich_spans: true
    enrich_logs: true
    enrich_metrics: true  # As resource attribute

  batch:
    timeout: 5s
    send_batch_size: 10000
    send_batch_max_size: 11000

  memory_limiter:
    check_interval: 1s
    limit_mib: 2000
    spike_limit_mib: 400

  resource:
    attributes:
      - key: deployment.environment
        from_attribute: k8s.namespace.name
        action: upsert

  # Tail-based sampling (keep errors, slow traces)
  tail_sampling:
    decision_wait: 10s
    policies:
      - name: errors
        type: status_code
        status_code: {status_codes: [ERROR]}
      - name: slow-traces
        type: latency
        latency: {threshold_ms: 1000}
      - name: probabilistic
        type: probabilistic
        probabilistic: {sampling_percentage: 10}

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
    traces_table_name: traces
    logs_table_name: logs
    metrics_table_name: metrics
    ttl_days: 30

  # For Grafana Tempo (optional, for native Grafana trace view)
  otlp/tempo:
    endpoint: tempo:4317
    tls:
      insecure: true

  # For Prometheus remote write (Grafana Mimir/Prometheus)
  prometheusremotewrite:
    endpoint: http://mimir:9009/api/v1/push

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, ollystack_correlation, tail_sampling, batch]
      exporters: [clickhouse, otlp/tempo]

    logs:
      receivers: [otlp, fluentforward]
      processors: [memory_limiter, ollystack_correlation, batch]
      exporters: [clickhouse]

    metrics:
      receivers: [otlp, prometheus]
      processors: [memory_limiter, ollystack_correlation, batch]
      exporters: [clickhouse, prometheusremotewrite]
```

### 2. Vector Configuration (Log Collection)

```yaml
# vector.yaml
sources:
  kubernetes_logs:
    type: kubernetes_logs

  docker_logs:
    type: docker_logs

transforms:
  parse_json:
    type: remap
    inputs: ["kubernetes_logs", "docker_logs"]
    source: |
      # Try to parse as JSON
      . = parse_json!(.message) ?? .

      # Extract correlation_id if present in message
      .correlation_id = .correlation_id ?? .correlationId ?? .request_id ?? ""

      # Add Kubernetes metadata
      .k8s.namespace = .kubernetes.pod_namespace
      .k8s.pod = .kubernetes.pod_name
      .k8s.container = .kubernetes.container_name

  enrich:
    type: remap
    inputs: ["parse_json"]
    source: |
      # Add timestamp if missing
      .timestamp = .timestamp ?? now()

      # Normalize severity
      .severity = downcase(.level ?? .severity ?? "info")

sinks:
  otel_collector:
    type: opentelemetry
    inputs: ["enrich"]
    endpoint: http://otel-collector:4317
    protocol: grpc
```

### 3. Grafana Integration

```yaml
# grafana-datasources.yaml
apiVersion: 1

datasources:
  # ClickHouse for OllyStack queries
  - name: OllyStack-ClickHouse
    type: grafana-clickhouse-datasource
    url: http://clickhouse:8123
    jsonData:
      defaultDatabase: ollystack

  # Tempo for native trace visualization
  - name: Tempo
    type: tempo
    url: http://tempo:3200
    jsonData:
      tracesToLogs:
        datasourceUid: 'OllyStack-ClickHouse'
        tags: ['correlation_id']
        mappedTags: [{ key: 'correlation_id', value: 'correlation_id' }]
        mapTagNamesEnabled: true
        spanStartTimeShift: '-1h'
        spanEndTimeShift: '1h'
        filterByTraceID: true
        filterBySpanID: false

  # Prometheus/Mimir for metrics
  - name: Prometheus
    type: prometheus
    url: http://mimir:9009/prometheus
    jsonData:
      exemplarTraceIdDestinations:
        - name: trace_id
          datasourceUid: Tempo
        - name: correlation_id
          datasourceUid: OllyStack-ClickHouse
          urlDisplayLabel: 'View in OllyStack'

  # OllyStack API as custom datasource
  - name: OllyStack-API
    type: marcusolsson-json-datasource
    url: http://ollystack-api:8080
    jsonData:
      queryParams: ''
```

### 4. Alertmanager Configuration

```yaml
# alertmanager.yaml
global:
  resolve_timeout: 5m
  slack_api_url: '${SLACK_WEBHOOK_URL}'

route:
  group_by: ['alertname', 'service', 'correlation_id']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'default'

  routes:
    - match:
        severity: critical
      receiver: 'pagerduty-critical'

    - match:
        severity: warning
      receiver: 'slack-warnings'

receivers:
  - name: 'default'
    slack_configs:
      - channel: '#alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: |
          {{ range .Alerts }}
          *Service:* {{ .Labels.service }}
          *Correlation ID:* {{ .Labels.correlation_id }}
          *Description:* {{ .Annotations.description }}

          <{{ .GeneratorURL }}|View in OllyStack>
          {{ end }}

  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: '${PAGERDUTY_KEY}'
        description: '{{ .GroupLabels.alertname }}: {{ .Annotations.summary }}'
        details:
          correlation_id: '{{ .Labels.correlation_id }}'
          ollystack_url: 'https://ollystack.example.com/correlate/{{ .Labels.correlation_id }}'

  - name: 'slack-warnings'
    slack_configs:
      - channel: '#warnings'
```

---

## OllyStack Unique Components (What We Build)

### 1. Correlation Engine

The heart of OllyStack - what no other platform does well.

```go
// internal/correlation/engine.go
package correlation

type CorrelationEngine struct {
    db     *clickhouse.Conn
    cache  *redis.Client
    ai     *ai.Engine
}

// GetFullContext returns ALL signals for a correlation ID
func (e *CorrelationEngine) GetFullContext(ctx context.Context, correlationID string) (*CorrelatedContext, error) {
    // Check cache first
    if cached, err := e.cache.Get(ctx, "corr:"+correlationID).Result(); err == nil {
        var result CorrelatedContext
        json.Unmarshal([]byte(cached), &result)
        return &result, nil
    }

    // Parallel fetch from ClickHouse
    var (
        traces   []TraceSpan
        logs     []LogEntry
        metrics  []MetricPoint
        wg       sync.WaitGroup
        errChan  = make(chan error, 3)
    )

    wg.Add(3)
    go func() {
        defer wg.Done()
        var err error
        traces, err = e.fetchTraces(ctx, correlationID)
        if err != nil {
            errChan <- err
        }
    }()
    go func() {
        defer wg.Done()
        var err error
        logs, err = e.fetchLogs(ctx, correlationID)
        if err != nil {
            errChan <- err
        }
    }()
    go func() {
        defer wg.Done()
        var err error
        metrics, err = e.fetchMetricExemplars(ctx, correlationID)
        if err != nil {
            errChan <- err
        }
    }()
    wg.Wait()
    close(errChan)

    // Build unified timeline
    timeline := e.buildTimeline(traces, logs, metrics)

    // Extract service topology
    services := e.extractServiceTopology(traces)

    // Identify errors
    errors := e.extractErrors(traces, logs)

    result := &CorrelatedContext{
        CorrelationID: correlationID,
        TimeRange:     e.calculateTimeRange(traces, logs),
        Traces:        traces,
        Logs:          logs,
        Metrics:       metrics,
        Timeline:      timeline,
        Services:      services,
        Errors:        errors,
    }

    // Cache for 5 minutes
    data, _ := json.Marshal(result)
    e.cache.Set(ctx, "corr:"+correlationID, data, 5*time.Minute)

    return result, nil
}
```

### 2. AI/ML Engine

```python
# ai-engine/ollystack/rca/analyzer.py
from typing import Optional
from dataclasses import dataclass
import openai

@dataclass
class RCAResult:
    root_cause: str
    confidence: float
    evidence: list[str]
    affected_services: list[str]
    recommendations: list[str]
    timeline_summary: str

class RootCauseAnalyzer:
    """
    AI-powered root cause analysis.
    Combines traditional analysis with LLM reasoning.
    """

    def __init__(self, llm_client, clickhouse_client):
        self.llm = llm_client
        self.db = clickhouse_client

    async def analyze(self, correlation_id: str) -> RCAResult:
        # 1. Fetch correlated context
        context = await self._fetch_context(correlation_id)

        # 2. Traditional analysis
        critical_path = self._find_critical_path(context.traces)
        error_origin = self._find_error_origin(context.traces, context.logs)
        anomalies = self._detect_anomalies(context)

        # 3. Build analysis prompt
        prompt = self._build_analysis_prompt(
            context=context,
            critical_path=critical_path,
            error_origin=error_origin,
            anomalies=anomalies
        )

        # 4. LLM reasoning
        llm_response = await self.llm.chat.completions.create(
            model="gpt-4o",  # or claude-3-opus
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": prompt}
            ],
            response_format={"type": "json_object"}
        )

        # 5. Parse and validate
        analysis = json.loads(llm_response.choices[0].message.content)

        return RCAResult(
            root_cause=analysis["root_cause"],
            confidence=analysis["confidence"],
            evidence=analysis["evidence"],
            affected_services=self._extract_affected_services(context),
            recommendations=analysis["recommendations"],
            timeline_summary=analysis["timeline_summary"]
        )

    def _build_analysis_prompt(self, context, critical_path, error_origin, anomalies):
        return f"""
Analyze this distributed system request and identify the root cause of any issues.

## Correlation ID: {context.correlation_id}

## Timeline Summary:
{self._format_timeline(context.timeline)}

## Critical Path (slowest execution path):
{self._format_critical_path(critical_path)}

## Error Origin:
{self._format_error_origin(error_origin)}

## Detected Anomalies:
{self._format_anomalies(anomalies)}

## Relevant Logs:
{self._format_logs(context.logs)}

Based on this information, provide:
1. The root cause of the issue
2. Your confidence level (0-1)
3. Evidence supporting your conclusion
4. Specific recommendations to fix the issue
5. A brief timeline summary for the on-call engineer
"""
```

### 3. Natural Language Query (NLQ)

```python
# ai-engine/ollystack/nlq/query.py

class NaturalLanguageQuery:
    """Convert natural language to ClickHouse SQL."""

    SCHEMA_CONTEXT = """
    Tables:
    - traces: Timestamp, TraceId, SpanId, ServiceName, SpanName, Duration, StatusCode, correlation_id
    - logs: Timestamp, TraceId, ServiceName, SeverityText, Body, correlation_id
    - metrics: Timestamp, MetricName, Value, Labels, correlation_id
    """

    async def query(self, question: str) -> QueryResult:
        # 1. Parse intent
        intent_response = await self.llm.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {"role": "system", "content": f"""
You are a SQL query generator for an observability system.
{self.SCHEMA_CONTEXT}

Parse the user's question and generate a ClickHouse SQL query.
Return JSON with: {{"intent": "...", "sql": "...", "explanation": "..."}}
"""},
                {"role": "user", "content": question}
            ]
        )

        result = json.loads(intent_response.choices[0].message.content)

        # 2. Execute query
        rows = await self.db.execute(result["sql"])

        # 3. Summarize results
        summary = await self._summarize_results(question, rows)

        return QueryResult(
            question=question,
            sql=result["sql"],
            rows=rows,
            summary=summary
        )
```

---

## Deployment Architecture

### Kubernetes Deployment

```yaml
# deploy/kubernetes/ollystack/values.yaml
global:
  environment: production

# Integrated Components
grafana:
  enabled: true
  persistence:
    enabled: true
  datasources:
    enabled: true
  dashboardProviders:
    enabled: true

alertmanager:
  enabled: true
  config:
    # See alertmanager.yaml above

otelCollector:
  mode: deployment
  replicas: 3
  config:
    # See otel-collector-config.yaml above
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 4Gi

vector:
  enabled: true
  role: Agent
  # DaemonSet on all nodes

# OllyStack Components
ollystack:
  api:
    replicas: 3
    image: ollystack/api:latest
    resources:
      requests:
        cpu: 500m
        memory: 512Mi

  aiEngine:
    replicas: 2
    image: ollystack/ai-engine:latest
    env:
      - name: OPENAI_API_KEY
        valueFrom:
          secretKeyRef:
            name: ollystack-secrets
            key: openai-api-key
    resources:
      requests:
        cpu: 1000m
        memory: 2Gi

  ui:
    replicas: 2
    image: ollystack/ui:latest

# Storage
clickhouse:
  enabled: true
  shards: 2
  replicas: 2
  persistence:
    size: 500Gi

redis:
  enabled: true
  architecture: standalone
```

---

## API Specification

### OllyStack Unique Endpoints

```yaml
openapi: 3.0.0
info:
  title: OllyStack API
  version: 2.0.0

paths:
  /api/v1/correlate/{correlation_id}:
    get:
      summary: Get full correlated context
      description: Returns all signals (traces, logs, metrics) for a correlation ID
      parameters:
        - name: correlation_id
          in: path
          required: true
          schema:
            type: string
      responses:
        200:
          description: Correlated context
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CorrelatedContext'

  /api/v1/correlate/{correlation_id}/timeline:
    get:
      summary: Get chronological timeline
      description: Returns all events in chronological order

  /api/v1/correlate/{correlation_id}/impact:
    get:
      summary: Get impact analysis
      description: Returns affected services, users, transactions

  /api/v1/ai/analyze:
    post:
      summary: AI-powered analysis
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                correlation_id:
                  type: string
                analysis_type:
                  type: string
                  enum: [rca, anomaly, comparison]
      responses:
        200:
          description: AI analysis result

  /api/v1/ai/nlq:
    post:
      summary: Natural language query
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                question:
                  type: string
                  example: "Show me all errors in payment service last hour"
      responses:
        200:
          description: Query results with natural language summary

components:
  schemas:
    CorrelatedContext:
      type: object
      properties:
        correlation_id:
          type: string
        time_range:
          $ref: '#/components/schemas/TimeRange'
        summary:
          $ref: '#/components/schemas/CorrelationSummary'
        traces:
          type: array
          items:
            $ref: '#/components/schemas/TraceSpan'
        logs:
          type: array
          items:
            $ref: '#/components/schemas/LogEntry'
        metrics:
          type: array
          items:
            $ref: '#/components/schemas/MetricPoint'
        timeline:
          type: array
          items:
            $ref: '#/components/schemas/TimelineEvent'
        services:
          type: array
          items:
            type: string
        errors:
          type: array
          items:
            $ref: '#/components/schemas/ErrorInfo'
        ai_insights:
          $ref: '#/components/schemas/AIInsights'
```

---

## Technology Stack Summary

| Layer | Technology | Status |
|-------|------------|--------|
| **Collection - Traces** | OTel Collector + Grafana Beyla | INTEGRATE |
| **Collection - Logs** | Vector → OTel Collector | INTEGRATE |
| **Collection - Metrics** | Prometheus → OTel Collector | INTEGRATE |
| **Processing** | OllyStack Correlation Processor | BUILD |
| **Storage** | ClickHouse | KEEP |
| **Cache** | Redis | KEEP |
| **Visualization** | Grafana | INTEGRATE |
| **Alerting** | Alertmanager | INTEGRATE |
| **API** | OllyStack API (Go) | BUILD |
| **AI/ML** | OllyStack AI Engine (Python) | BUILD |
| **Frontend** | OllyStack UI (React) | BUILD |
| **LLM** | OpenAI/Claude/Ollama | INTEGRATE |

---

## Competitive Differentiation (Updated)

| Feature | OllyStack | Grafana Stack | Datadog | Elastic |
|---------|-----------|---------------|---------|---------|
| Correlation ID Native | **Built-in** | Manual | Manual | Manual |
| Cross-Signal Join | **Automatic** | Query-time | Query-time | Manual |
| AI Root Cause | **Native LLM** | None | Watchdog | None |
| Natural Language Query | **Yes** | None | None | None |
| Zero-Code (eBPF) | Beyla + Custom | Beyla | Limited | None |
| Dashboards | Grafana (best) | Grafana | Good | Kibana |
| Cost at Scale | **Low** | Low | High | Medium |
| Self-Hostable | **Yes** | Yes | No | Yes |

**OllyStack = Grafana's visualization + Datadog's APM + Splunk's search + AI-native insights**

---

*Document Version 2.0 - Integrated Architecture Approach*
