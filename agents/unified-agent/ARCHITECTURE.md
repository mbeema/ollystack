# OllyStack Collection Architecture

## Recommended: Direct Agent → Backend

For most deployments, the unified agent sends directly to the OllyStack backend.
No intermediate collector needed.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PER HOST                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────┐                                                          │
│   │    Apps     │ (Java, Python, Go, Node.js, .NET)                        │
│   │  with OTel  │                                                          │
│   │    SDK      │                                                          │
│   └──────┬──────┘                                                          │
│          │ OTLP (traces, metrics, logs)                                    │
│          ▼                                                                 │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                     OllyStack Unified Agent                           │  │
│   │  ┌─────────────────────────────────────────────────────────────┐    │  │
│   │  │ Collection        │ Processing          │ Export             │    │  │
│   │  │ • Host metrics    │ • Aggregation (60x) │ • OTLP batching   │    │  │
│   │  │ • Log files       │ • Deduplication     │ • Retry + buffer  │    │  │
│   │  │ • OTLP receiver   │ • Sampling          │ • Compression     │    │  │
│   │  │ • Container stats │ • Cardinality ctrl  │                   │    │  │
│   │  └─────────────────────────────────────────────────────────────┘    │  │
│   │                          ~100MB memory                               │  │
│   └──────────────────────────────────┬──────────────────────────────────┘  │
│                                      │                                     │
└──────────────────────────────────────┼─────────────────────────────────────┘
                                       │ OTLP (aggregated, sampled)
                                       │ 90% less data than raw
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           OLLYSTACK BACKEND                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐       │
│   │ Stream Processor │    │   ClickHouse    │    │   API Server    │       │
│   │ • Anomaly detect │───▶│   (Storage)     │◀───│   (Query)       │       │
│   │ • Correlation    │    │                 │    │                 │       │
│   │ • Topology       │    │                 │    │                 │       │
│   └─────────────────┘    └─────────────────┘    └─────────────────┘       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Why No Collector?

### 1. Agent Already Does What Collector Does

| Feature | OTel Collector | Unified Agent |
|---------|---------------|---------------|
| OTLP receiver | ✓ | ✓ |
| Batching | ✓ | ✓ |
| Retry | ✓ | ✓ |
| Enrichment | ✓ | ✓ (K8s, Cloud) |
| Sampling | ✓ | ✓ (Adaptive) |
| **Aggregation** | ✗ | ✓ (60x reduction) |
| **Deduplication** | ✗ | ✓ (90% log reduction) |
| **Cardinality control** | Limited | ✓ (Full) |

### 2. Resource Savings

```
With Collector:     Agent (150MB) + Collector (150MB) = 300MB per host
Without Collector:  Agent (100MB) = 100MB per host

Savings: 200MB per host × 100 hosts = 20GB memory saved
```

### 3. Reduced Latency

```
With Collector:     App → Agent → Collector → Backend (3 hops)
Without Collector:  App → Agent → Backend (2 hops)
```

### 4. Simpler Operations

- One config file instead of two
- One process to monitor
- One set of logs to debug
- Fewer failure points

## When DO You Need a Collector?

### Scenario 1: Multiple Backends

If you need to send data to multiple systems (e.g., OllyStack + Datadog):

```
Agent → OTel Collector Gateway → OllyStack
                              → Datadog
                              → S3 Archive
```

### Scenario 2: Protocol Translation

If some apps use non-OTLP protocols:

```
Jaeger apps ─────┐
Zipkin apps ─────┼───▶ Collector (translation) ───▶ Agent ───▶ Backend
Prometheus  ─────┘
```

### Scenario 3: Central Processing

If you need centralized processing that can't run on each host:

```
Agent ───┐
Agent ───┼───▶ Collector Cluster ───▶ Backend
Agent ───┘     (tail sampling,
               cross-host correlation)
```

## Recommended Deployment

### For Most Users: Agent Only

```yaml
# docker-compose.yml
services:
  unified-agent:
    image: ollystack/unified-agent:latest
    environment:
      - OLLYSTACK_ENDPOINT=stream-processor:4317
    volumes:
      - /var/log:/var/log:ro
    ports:
      - "4317:4317"  # Apps send OTLP here
```

### For Enterprise: Agent + Gateway Collector

```yaml
# Per-host agent
unified-agent:
  environment:
    - OLLYSTACK_ENDPOINT=collector-gateway:4317

# Central gateway (1-3 instances, not per-host)
collector-gateway:
  image: otel/opentelemetry-collector-contrib
  # Handle routing, fanout, additional processing
```

## Migration Path

### From OTel Collector to Unified Agent

1. **Deploy agent alongside collector**
   ```bash
   # Agent collects host metrics/logs, sends to collector
   OLLYSTACK_ENDPOINT=otel-collector:4317
   ```

2. **Move app traces to agent**
   ```bash
   # Apps send to agent instead of collector
   OTEL_EXPORTER_OTLP_ENDPOINT=http://unified-agent:4317
   ```

3. **Remove collector**
   ```bash
   # Agent sends directly to backend
   OLLYSTACK_ENDPOINT=stream-processor:4317
   ```

## Summary

| Deployment | Architecture | Memory/Host |
|------------|--------------|-------------|
| **Simple** | Agent → Backend | 100MB |
| **Multi-backend** | Agent → Collector → Backends | 250MB |
| **Legacy protocols** | Apps → Collector → Agent → Backend | 300MB |

**Recommendation:** Start with Agent → Backend. Add collector only if you have specific requirements.
