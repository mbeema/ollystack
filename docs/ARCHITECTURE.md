# OllyStack Architecture - Leveraging Best-in-Class OSS

## Philosophy
**Build where we differentiate, integrate where commoditized.**

We leverage proven, high-performance open-source agents for data collection, and focus our engineering on:
1. **Real-time stream processing & analytics**
2. **AI-powered insights**
3. **Unified query layer**
4. **End-to-end correlation**

---

## Collection Layer (USE EXISTING)

### 1. OpenTelemetry Collector (Primary Gateway)

The OTel Collector is our primary ingestion point. It's:
- Battle-tested at massive scale (used by all major cloud providers)
- Highly efficient (Go, minimal allocations)
- Extensible via processors and exporters

```yaml
# deploy/otel-collector/config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  # Host metrics (replaces our universal-agent)
  hostmetrics:
    collection_interval: 10s
    scrapers:
      cpu:
      memory:
      disk:
      filesystem:
      network:
      load:
      processes:

  # Prometheus scraping
  prometheus:
    config:
      scrape_configs:
        - job_name: 'kubernetes-pods'
          kubernetes_sd_configs:
            - role: pod

  # Kubernetes events
  k8s_events:
    auth_type: serviceAccount

processors:
  batch:
    send_batch_size: 10000
    timeout: 10s

  memory_limiter:
    limit_mib: 1500
    spike_limit_mib: 500

  # Our custom processors (this is where we add value)
  ollystack_enrichment:
    kubernetes:
      enabled: true
    cloud:
      enabled: true
      provider: auto

  ollystack_sampling:
    policies:
      - name: errors
        type: status_code
        status_codes: [ERROR]
      - name: high-latency
        type: latency
        latency_ms: 1000
      - name: rate-limit
        type: rate_limiting
        spans_per_second: 100

exporters:
  otlp:
    endpoint: ollystack-stream-processor:4317

  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
    ttl_days: 30

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, ollystack_enrichment, ollystack_sampling]
      exporters: [otlp, clickhouse]
    metrics:
      receivers: [otlp, hostmetrics, prometheus]
      processors: [memory_limiter, batch, ollystack_enrichment]
      exporters: [otlp, clickhouse]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch, ollystack_enrichment]
      exporters: [otlp, clickhouse]
```

### 2. Grafana Beyla (eBPF Auto-Instrumentation)

Instead of building custom eBPF probes, use **Grafana Beyla**:
- Zero-code HTTP/gRPC/SQL instrumentation
- Automatic trace context propagation
- Maintained by Grafana Labs
- Apache 2.0 licensed

```yaml
# deploy/beyla/config.yaml
open_port: 8080  # Application port to instrument
service_name: my-service

otel_traces_export:
  endpoint: http://otel-collector:4318/v1/traces

otel_metrics_export:
  endpoint: http://otel-collector:4318/v1/metrics

# Protocol detection
protocols:
  http:
    enabled: true
  grpc:
    enabled: true
  sql:
    enabled: true
  redis:
    enabled: true
```

### 3. Vector (High-Performance Log Collection)

For logs, use **Vector** instead of custom agents:
- Written in Rust (extremely fast)
- 10x more efficient than Fluentd
- Native OTLP support
- VRL for transformation

```toml
# deploy/vector/vector.toml
[sources.kubernetes_logs]
type = "kubernetes_logs"

[sources.file_logs]
type = "file"
include = ["/var/log/**/*.log"]

[transforms.parse_logs]
type = "remap"
inputs = ["kubernetes_logs", "file_logs"]
source = '''
  # Parse JSON logs
  . = parse_json!(.message) ?? .

  # Extract trace context
  .trace_id = .traceId ?? .trace_id
  .span_id = .spanId ?? .span_id

  # Detect severity
  .severity = if contains(string!(.message), "ERROR") {
    "ERROR"
  } else if contains(string!(.message), "WARN") {
    "WARN"
  } else {
    "INFO"
  }
'''

[sinks.otlp]
type = "opentelemetry"
inputs = ["parse_logs"]
endpoint = "http://otel-collector:4317"
protocol = "grpc"
```

### 4. OpenTelemetry SDKs (Application Instrumentation)

For application-level instrumentation, use OTel SDKs with auto-instrumentation:

```bash
# Java (auto-instrumentation)
java -javaagent:opentelemetry-javaagent.jar \
  -Dotel.service.name=my-service \
  -Dotel.exporter.otlp.endpoint=http://otel-collector:4317 \
  -jar myapp.jar

# Python (auto-instrumentation)
opentelemetry-instrument \
  --service_name my-service \
  --exporter_otlp_endpoint http://otel-collector:4317 \
  python myapp.py

# Node.js (auto-instrumentation)
node --require @opentelemetry/auto-instrumentations-node/register app.js

# Go (manual, but with helpers)
# Use go.opentelemetry.io/contrib/instrumentation packages
```

---

## What We Build (Our Differentiation)

### 1. Custom OTel Collector Processors

We build custom processors that plug into the standard OTel Collector:

```
collector/
├── processors/
│   ├── ollystack_enrichment/     # K8s, cloud, geo-IP enrichment
│   ├── ollystack_sampling/       # Intelligent tail-based sampling
│   ├── ollystack_topology/       # Service map discovery
│   └── ollystack_correlation/    # Cross-signal correlation
└── exporters/
    └── ollystack_clickhouse/     # Optimized ClickHouse exporter
```

### 2. Stream Processor (Real-time Analytics)

This is where we add significant value:

```
stream-processor/
├── src/
│   ├── analytics/
│   │   ├── anomaly_detection.rs    # ML-based anomaly detection
│   │   ├── root_cause.rs           # Causal analysis
│   │   └── forecasting.rs          # Predictive analytics
│   ├── correlation/
│   │   ├── trace_assembler.rs      # Complete trace reconstruction
│   │   └── signal_correlator.rs    # Metrics ↔ Logs ↔ Traces
│   └── topology/
│       └── service_graph.rs        # Real-time dependency mapping
```

### 3. Query Engine (Unified Access)

Custom query layer for cross-signal queries:

```sql
-- ObservQL: Query across all signals
SELECT
  t.trace_id,
  t.duration,
  m.cpu_usage,
  l.error_message
FROM traces t
JOIN metrics m ON t.service = m.service
  AND m.timestamp BETWEEN t.start - 1m AND t.end + 1m
JOIN logs l ON t.trace_id = l.trace_id
WHERE t.duration > 1s AND l.severity = 'ERROR'
```

### 4. AI Engine

Natural language queries and intelligent analysis:

```
ai-engine/
├── nlq/                    # Natural language to ObservQL
├── anomaly_models/         # Trained ML models
└── rca/                    # Root cause analysis
```

### 5. Web UI

The visualization and interaction layer with AI copilot.

---

## Revised Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           APPLICATIONS                                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Java      │  │   Python    │  │   Node.js   │  │     Go      │        │
│  │ OTel Agent  │  │ OTel Auto   │  │ OTel Auto   │  │  OTel SDK   │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
└─────────┴────────────────┴───────────────┴───────────────┴──────────────────┘
          │                │               │               │
          │            OTLP (gRPC/HTTP)                    │
          │                │               │               │
          ▼                ▼               ▼               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    INFRASTRUCTURE AGENTS                                     │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │  Grafana Beyla  │  │     Vector      │  │  OTel Collector │             │
│  │  (eBPF auto-    │  │  (Log shipping) │  │  (hostmetrics)  │             │
│  │   instrument)   │  │                 │  │                 │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
└───────────┴─────────────────────┴─────────────────────┴─────────────────────┘
            │                     │                     │
            │                 OTLP                      │
            ▼                     ▼                     ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                     OPENTELEMETRY COLLECTOR                                  │
│                    (with OllyStack custom processors)                         │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  Receivers    │  Processors                      │  Exporters        │   │
│  │  ─────────    │  ──────────                      │  ─────────        │   │
│  │  • otlp       │  • batch                         │  • otlp           │   │
│  │  • prometheus │  • memory_limiter                │  • clickhouse     │   │
│  │  • hostmetrics│  • ollystack_enrichment    ◄──── CUSTOM              │   │
│  │  • k8s_events │  • ollystack_sampling      ◄──── CUSTOM              │   │
│  │               │  • ollystack_topology      ◄──── CUSTOM              │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                OTLP│
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      OLLYSTACK STREAM PROCESSOR                               │
│                         (Our Core Value-Add)                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │   Anomaly    │  │    Trace     │  │  Topology    │  │    Alert     │    │
│  │  Detection   │  │ Correlation  │  │  Discovery   │  │   Engine     │    │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          STORAGE LAYER                                       │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        ClickHouse                                     │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐      │   │
│  │  │   Traces   │  │   Metrics  │  │    Logs    │  │  Topology  │      │   │
│  │  └────────────┘  └────────────┘  └────────────┘  └────────────┘      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        QUERY & PRESENTATION                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │Query Engine  │  │  API Server  │  │    Web UI    │  │  AI Engine   │    │
│  │  (ObservQL)  │  │ (REST/gRPC)  │  │   (React)    │  │   (NLQ)      │    │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Comparison

| Component | Before (Custom) | After (OSS + Custom) | Benefit |
|-----------|-----------------|---------------------|---------|
| **Host Metrics** | Universal Agent (Go) | OTel Collector hostmetrics | Maintained by OTel community |
| **Log Collection** | Custom file tailer | Vector | 10x faster, battle-tested |
| **eBPF Instrumentation** | Custom Rust agent | Grafana Beyla | Maintained by Grafana, production-ready |
| **App Instrumentation** | Custom SDKs | OTel SDKs | Industry standard, all languages |
| **Collector** | Custom | OTel Collector + custom processors | Extensible, proven at scale |
| **Stream Processing** | **Custom (Rust)** | **Custom (Rust)** | Core differentiation |
| **Query Engine** | **Custom** | **Custom** | Core differentiation |
| **AI/ML** | **Custom** | **Custom** | Core differentiation |
| **Web UI** | **Custom (React)** | **Custom (React)** | Core differentiation |

---

## What We Focus Engineering On

### Must Build (Differentiation)
1. **Custom OTel Processors** - Enrichment, sampling, topology
2. **Stream Processor** - Real-time analytics, anomaly detection
3. **Query Engine** - ObservQL, cross-signal correlation
4. **AI Engine** - NLQ, root cause analysis
5. **Web UI** - Visualization, AI copilot

### Use Existing (Commoditized)
1. **OTel Collector** - Core collection/routing
2. **OTel SDKs** - Application instrumentation
3. **Grafana Beyla** - eBPF auto-instrumentation
4. **Vector/Fluent Bit** - Log shipping
5. **ClickHouse** - Storage

---

## Deployment Options

### Option 1: All-in-One (Small Scale)
```yaml
# Single binary with embedded OTel Collector
ollystack-all-in-one:
  includes:
    - otel-collector (embedded)
    - stream-processor
    - api-server
    - query-engine
```

### Option 2: Distributed (Production)
```yaml
# Kubernetes deployment
ollystack:
  otel-collector:
    mode: daemonset  # One per node
    replicas: auto
  beyla:
    mode: daemonset  # eBPF per node
  vector:
    mode: daemonset  # Logs per node
  stream-processor:
    replicas: 3
  api-server:
    replicas: 3
  web-ui:
    replicas: 2
```
