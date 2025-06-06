# OllyStack Roadmap
## Integrated Architecture - Strategic Build & Integrate Approach

**Version:** 2.0
**Last Updated:** 2026-02-03

---

## Vision Statement

> **"Correlation from the first byte, insights in seconds - powered by the best OSS tools"**

OllyStack combines best-in-class open source observability tools with unique correlation intelligence and AI-powered insights. We don't reinvent the wheel - we make it spin faster.

---

## Strategic Approach: Build vs Integrate

### What We BUILD (Our Competitive Moat)

| Component | Why It's Unique |
|-----------|-----------------|
| **OTel Correlation Processor** | No existing processor does correlation ID enrichment |
| **Correlation Engine** | Single API to get ALL signals for one request |
| **AI/ML Insights** | LLM-powered RCA, NLQ, anomaly detection |
| **Correlation Explorer UI** | Unified view that Grafana can't provide |

### What We INTEGRATE (Best-in-Class OSS)

| Component | Tool | Why |
|-----------|------|-----|
| Visualization | **Grafana** | 10+ years of refinement, 1000+ plugins |
| Collection | **OTel Collector** | Industry standard, 100+ receivers |
| Log Shipping | **Vector** | Rust-based, efficient, battle-tested |
| Alert Routing | **Alertmanager** | Proven routing, silencing, grouping |
| Metrics | **Prometheus** | De facto standard |
| Zero-Code Tracing | **Grafana Beyla** | eBPF-based auto-instrumentation |
| Storage | **ClickHouse** | Already chosen - keep it |

### Engineering Time Saved by Integrating

| Would Build | Integrate Instead | Time Saved |
|-------------|-------------------|------------|
| Dashboard system | Grafana | 6+ months |
| Alert routing | Alertmanager | 2+ months |
| Log collection | Vector | 2+ months |
| Trace receivers | OTel Collector | 3+ months |
| 100+ integrations | OTel ecosystem | Years |

**Total: 12+ months redirected to differentiation**

---

## Roadmap Phases

### Phase 1: Foundation (Weeks 1-4)
**Goal:** Establish correlation ID infrastructure and integrate core OSS tools

```
Week 1-2: Database & OTel Processor
├── Add correlation_id columns to ClickHouse (traces, logs)
├── Create correlation_summary materialized view
├── Build OTel Collector Correlation Processor
├── Test processor with sample data
└── Create custom collector Docker image

Week 3-4: Integrations Setup
├── Deploy OTel Collector with correlation processor
├── Configure Grafana with ClickHouse datasource
├── Deploy Vector for log collection
├── Set up Alertmanager with OllyStack templates
└── Configure Prometheus for metrics scraping
```

**Deliverables:**
- Custom OTel Collector with `ollystack_correlation` processor
- Grafana with ClickHouse, Tempo, Prometheus datasources
- Vector DaemonSet collecting logs with correlation_id extraction
- Alertmanager with Slack/PagerDuty integration

---

### Phase 2: Correlation Engine (Weeks 5-8)
**Goal:** Build the unique correlation API and initial UI

```
Week 5-6: Backend API
├── Create correlation package (generation, validation)
├── Implement Correlation Engine (parallel ClickHouse queries)
├── Add correlation middleware to API server
├── Create /api/v1/correlate/{id} endpoint
├── Add /api/v1/correlate/{id}/timeline endpoint
└── Implement Redis caching for correlation queries

Week 7-8: Integration & Testing
├── Create Grafana correlation dashboard
├── Configure trace-to-logs linking in Grafana
├── Set up exemplar linking for metrics
├── Test end-to-end correlation flow
└── Performance testing and optimization
```

**Deliverables:**
- `GET /api/v1/correlate/{correlation_id}` - Returns traces + logs + metrics + timeline
- Grafana dashboard with correlation lookup
- Sub-200ms correlation queries
- 95%+ correlation coverage in test environment

---

### Phase 3: AI/ML Engine (Weeks 9-12)
**Goal:** Add AI-powered insights and natural language queries

```
Week 9-10: AI Foundation
├── Create AI Engine Python project structure
├── Implement LLM provider abstraction (OpenAI, Claude, Ollama)
├── Build baseline calculator for anomaly detection
├── Implement critical path analyzer
└── Create root cause analyzer with LLM integration

Week 11-12: NLQ & Integration
├── Implement natural language query parser
├── Build SQL generator for ClickHouse
├── Add /api/v1/ai/analyze endpoint
├── Add /api/v1/ai/nlq endpoint
├── Add /api/v1/ai/rca endpoint
└── Integrate AI insights with correlation API
```

**Deliverables:**
- `POST /api/v1/ai/nlq` - "Show errors in payments last hour" → SQL + results + summary
- `POST /api/v1/ai/rca` - Automatic root cause analysis
- Anomaly detection with seasonal adjustments
- LLM-powered explanations in correlation responses

---

### Phase 4: Correlation Explorer UI (Weeks 13-16)
**Goal:** Build the unique UI that Grafana can't provide

```
Week 13-14: Core UI Components
├── Create Correlation Explorer page
├── Build unified timeline component
├── Implement service flow visualization
├── Add traces/logs tabs with filtering
└── Create summary cards component

Week 15-16: AI Integration & Polish
├── Add AI Insights panel
├── Implement NLQ chat interface
├── Add anomaly highlighting
├── Embed Grafana dashboards
├── Create recommendations panel
└── Mobile-responsive design
```

**Deliverables:**
- Correlation Explorer page with full context view
- Interactive timeline with zoom/pan
- Service flow diagram
- AI Insights panel with recommendations
- NLQ chat interface

---

### Phase 5: Production Hardening (Weeks 17-20)
**Goal:** Enterprise-ready deployment

```
Week 17-18: Operations
├── Multi-tenancy support
├── RBAC implementation
├── Rate limiting and quotas
├── Audit logging
└── Backup and disaster recovery

Week 19-20: Scale & Performance
├── Horizontal scaling for all components
├── Query optimization
├── Caching strategy refinement
├── Load testing at production scale
└── Documentation and runbooks
```

**Deliverables:**
- Multi-tenant deployment
- RBAC with SSO integration
- Production deployment guide
- SLA-ready architecture

---

## Implementation Status

### Completed Features ✅

| Feature | Sprint | Date | Details |
|---------|--------|------|---------|
| Correlation ID Infrastructure | 1-4 | 2026-02-01 | Database schema, correlation processor |
| Service Flow Visualization | 4 | 2026-02-02 | Interactive service dependency graph |
| NLQ Widget | 4 | 2026-02-02 | Natural language query interface |
| Causal Analysis | 5 | 2026-02-03 | Hybrid causal graph + LLM RCA |
| Predictive Alerts | 5 | 2026-02-03 | ML-based anomaly prediction |
| Metric Exemplars | 6 | 2026-02-03 | Trace-to-metric linking with API endpoints |
| **Service Map Visualization** | 7 | 2026-02-03 | D3.js force-directed graph with real-time topology |

### In Progress 🚧

| Feature | Target | Notes |
|---------|--------|-------|
| W3C Baggage Migration | Sprint 7 | Replace X-Correlation-ID with standard baggage |
| OpenTelemetry Profiling | Sprint 8 | CPU/memory correlation with traces |
| Grafana LGTM Integration | Sprint 7 | Full Grafana stack integration |

---

## Immediate Priority: Sprint 1 Tasks

### This Week (Top 5 Tasks)

1. **Database: Add correlation_id columns**
   ```sql
   ALTER TABLE ollystack.traces ADD COLUMN correlation_id String DEFAULT '';
   ALTER TABLE ollystack.logs ADD COLUMN correlation_id String DEFAULT '';
   ```

2. **Build OTel Correlation Processor**
   - Create `otel-processor-correlation/` project
   - Implement correlation ID extraction/generation
   - Build custom collector with processor

3. **Deploy Grafana**
   - Install with Helm
   - Configure ClickHouse datasource
   - Create initial correlation dashboard

4. **Deploy Vector**
   - Deploy as DaemonSet
   - Configure correlation_id extraction
   - Route logs to OTel Collector

5. **Create Correlation API Endpoint**
   - Implement `/api/v1/correlate/{id}`
   - Parallel fetch traces + logs + metrics
   - Build timeline response

---

## Architecture Quick Reference

```
┌─────────────────────────────────────────────────────────────────────┐
│                          DATA SOURCES                                │
│  Apps (OTel SDK) │ Infra (Beyla/eBPF) │ Logs (Vector) │ Metrics     │
└─────────┬────────┴─────────┬──────────┴───────┬───────┴──────┬──────┘
          │                  │                  │              │
          ▼                  ▼                  ▼              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    OpenTelemetry Collector                           │
│              + OLLYSTACK CORRELATION PROCESSOR                       │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         ClickHouse                                   │
│            (correlation_id indexed on all tables)                    │
└────────────┬────────────────┬────────────────┬──────────────────────┘
             │                │                │
             ▼                ▼                ▼
┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  OLLYSTACK API │  │     GRAFANA     │  │  ALERTMANAGER   │
│  (Correlation) │  │ (Visualization) │  │ (Alert Routing) │
└───────┬────────┘  └─────────────────┘  └─────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     OLLYSTACK AI ENGINE                              │
│         Anomaly Detection │ RCA │ Natural Language Query             │
└─────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      OLLYSTACK WEB UI                                │
│   Correlation Explorer │ Embedded Grafana │ AI Insights Panel        │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Success Metrics

### Technical KPIs

| Metric | Target | How to Measure |
|--------|--------|----------------|
| Correlation Coverage | >95% | % of traces with correlation_id |
| Query Latency (p95) | <200ms | Correlation lookup time |
| AI Accuracy | >80% | Correct RCA identification |
| Data Completeness | >99% | Span count vs expected |
| Uptime | 99.9% | Service availability |

### Business KPIs

| Metric | Target | How to Measure |
|--------|--------|----------------|
| MTTR | -50% | Time from alert to resolution |
| Debug Time | -60% | Time spent investigating |
| False Positives | <5% | Alert noise reduction |
| Developer Adoption | +30% | Weekly active users |

---

## Competitive Comparison

| Feature | OllyStack | Grafana Stack | Datadog | Elastic |
|---------|-----------|---------------|---------|---------|
| Correlation ID Native | **Built-in** | Manual | Manual | Manual |
| Cross-Signal Join | **Automatic** | Query-time | Query-time | Manual |
| AI Root Cause | **LLM-native** | None | Watchdog | None |
| Natural Language Query | **Yes** | None | None | None |
| Zero-Code (eBPF) | Beyla | Beyla | Limited | None |
| Dashboards | **Grafana** | Grafana | Proprietary | Kibana |
| Cost at Scale | **Low** | Low | High | Medium |
| Self-Hostable | **Yes** | Yes | No | Yes |

**OllyStack = Grafana's visualization + Datadog's APM + Splunk's search + AI-native insights**

---

## Technology Stack

| Layer | Technology | Status |
|-------|------------|--------|
| Collection | OTel Collector + Grafana Beyla | INTEGRATE |
| Logs | Vector → OTel Collector | INTEGRATE |
| Metrics | Prometheus → OTel Collector | INTEGRATE |
| Processing | OllyStack Correlation Processor | **BUILD** |
| Storage | ClickHouse + Redis | KEEP |
| Visualization | Grafana | INTEGRATE |
| Alerting | Alertmanager | INTEGRATE |
| API | OllyStack API (Go) | **BUILD** |
| AI/ML | OllyStack AI Engine (Python) | **BUILD** |
| Frontend | OllyStack UI (React) | **BUILD** |
| LLM | OpenAI / Claude / Ollama | INTEGRATE |

---

## Key Decisions Made

### ADR-001: Correlation ID Format
**Decision:** `olly-{timestamp_base36}-{random_8hex}`
**Example:** `olly-2k8f9x3-a7b2c4d1`

### ADR-002: Integrate Grafana
**Decision:** Use Grafana for dashboards, embed in OllyStack UI
**Rationale:** 10+ years of refinement, huge plugin ecosystem

### ADR-003: OTel Collector Processor
**Decision:** Build custom processor, not gateway replacement
**Rationale:** Leverage OTel ecosystem, only add correlation logic

### ADR-004: Vector for Logs
**Decision:** Use Vector instead of Fluent Bit/Logstash
**Rationale:** Rust-based efficiency, excellent transforms, OTel sink

### ADR-005: LLM Provider Abstraction
**Decision:** Support OpenAI, Claude, and local (Ollama)
**Rationale:** Flexibility for enterprise requirements

---

## Getting Started

### Prerequisites
- Kubernetes cluster
- ClickHouse (existing)
- Helm 3.x

### Quick Deploy
```bash
# Add Helm repos
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add vector https://helm.vector.dev

# Deploy integrated stack
helm upgrade --install ollystack ./deploy/kubernetes/ollystack \
  --set grafana.enabled=true \
  --set vector.enabled=true \
  --set alertmanager.enabled=true \
  --set clickhouse.host=clickhouse.default.svc
```

### Test Correlation
```bash
# Send test trace with correlation ID
curl -X POST http://otel-collector:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{"resourceSpans":[...]}'

# Query correlation
curl http://ollystack-api:8080/api/v1/correlate/olly-test-12345678
```

---

## Contributing

See [IMPLEMENTATION_TASKS.md](./IMPLEMENTATION_TASKS.md) for detailed task breakdown.

### Priority Areas
1. OTel Correlation Processor (Go)
2. Correlation API endpoints (Go)
3. AI Engine (Python)
4. Correlation Explorer UI (React/TypeScript)

---

## Research-Based Improvements

Based on industry research, papers, and best practices from 2024-2025, here are key improvements to incorporate:

### 1. Use OpenTelemetry eBPF Instrumentation (OBI) Instead of Beyla Alone

**Finding:** Grafana donated Beyla to OpenTelemetry in 2025, creating "OpenTelemetry eBPF Instrumentation" (OBI).

**Improvement:**
- Use OBI (the upstream project) for better community support
- Beyla 2.5+ now vendors OBI code directly
- Supports MongoDB, JSON-RPC (blockchain), and manual span injection
- 100,000+ Docker pulls/month proves production readiness

**Source:** [Grafana Blog - OpenTelemetry eBPF Instrumentation](https://grafana.com/blog/2025/05/07/opentelemetry-ebpf-instrumentation-beyla-donation/)

---

### 2. Adopt OpenTelemetry Semantic Conventions

**Finding:** Semantic conventions are now stable for HTTP, databases, and more. Using standard attribute names enables cross-tool compatibility.

**Improvement:**
```yaml
# Use stable semantic conventions
http.request.method    # Not http.method (deprecated)
http.response.status_code
http.route             # Not http.target (avoids cardinality)
db.system              # database type
db.operation.name      # query type
service.name           # resource attribute
```

**Benefits:**
- Grafana, Datadog, and other tools auto-detect standard attributes
- Future-proof as conventions evolve
- Enables cross-vendor migration

**Source:** [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)

---

### 3. Implement Causal Graph for Root Cause Analysis

**Finding:** LLMs alone are insufficient for RCA - they hallucinate causes. Causal inference with dependency graphs provides reliable diagnosis.

**Improvement:**
```python
# Hybrid approach: Causal Graph + LLM
class EnhancedRCA:
    def __init__(self):
        self.causal_graph = ServiceDependencyGraph()  # From traces
        self.llm = LLMProvider()

    async def analyze(self, correlation_id: str):
        # 1. Build causal graph from service dependencies
        graph = self.causal_graph.build_from_traces(correlation_id)

        # 2. Use causal inference to identify root cause candidates
        candidates = self.causal_inference(graph, anomalies)

        # 3. Use LLM only for explanation and recommendations
        explanation = await self.llm.explain(candidates)

        return RCAResult(
            root_cause=candidates[0],  # From causal inference
            explanation=explanation,    # From LLM
            confidence=candidates[0].score
        )
```

**Tools to consider:**
- **DoWhy** (AWS/Microsoft): Python library for causal ML
- **PyWhy**: Open organization for causal inference

**Sources:**
- [InfoQ - Causal Reasoning in Observability](https://www.infoq.com/articles/causal-reasoning-observability/)
- [AWS - Root Cause Analysis with DoWhy](https://aws.amazon.com/blogs/opensource/root-cause-analysis-with-dowhy-an-open-source-python-library-for-causal-machine-learning/)

---

### 4. Implement Metric Exemplars Properly ✅ IMPLEMENTED

**Finding:** Exemplars are the standard way to link metrics to traces, but require proper SDK configuration.

**Status:** Fully implemented and tested on 2026-02-03.

**Implementation Details:**

1. **Service Instrumentation** (api-gateway, order-service):
```go
// MeterProvider with OTLP metric exporter
meterProvider = sdkmetric.NewMeterProvider(
    sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
        sdkmetric.WithInterval(15*time.Second),
    )),
    sdkmetric.WithResource(res),
)

// Metrics recorded within spans get exemplars automatically
requestLatency.Record(ctx, duration, metricAttrs)
```

2. **Environment Variable** (enables experimental exemplar support):
```yaml
environment:
  - OTEL_GO_X_EXEMPLAR=true
```

3. **OTel Collector Config**:
```yaml
exporters:
  clickhouse/metrics:
    endpoint: tcp://clickhouse:9000
    metrics_table_name: otel_metrics
  prometheus:
    endpoint: "0.0.0.0:8889"
    enable_open_metrics: true  # Enables exemplar exposition
```

4. **API Endpoints** (api-server/internal/handlers/exemplars.go):
- `GET /api/v1/metrics/exemplars` - Metrics with trace-linked exemplars
- `GET /api/v1/metrics/:metricName/exemplars` - Exemplars for specific metric
- `GET /api/v1/exemplars/high-latency` - Find traces for latency spikes
- `GET /api/v1/exemplars/trace/:traceId` - Full trace from exemplar

5. **Verified Results:**
```
MetricName                      service        exemplar_count  trace_ids
http.server.request.duration    api-gateway    6               f2429b..., d386f3...
http.server.request.duration    order-service  6               f2429b..., d386f3...
```

**Source:** [OpenTelemetry Exemplars Documentation](https://opentelemetry.io/docs/languages/dotnet/metrics/exemplars/)

---

### 5. Service Map / Topology Visualization ✅ IMPLEMENTED

**Finding:** Professional observability platforms (Datadog, Jaeger, Grafana Tempo) provide interactive service maps showing service dependencies, traffic flow, and health status.

**Status:** Fully implemented and deployed on 2026-02-03.

**Implementation Details:**

1. **ClickHouse Topology Queries** (api-server/internal/storage/clickhouse.go):
```go
// GetServiceStats - aggregates per-service metrics
SELECT ServiceName, count() as total_spans,
       round(countIf(StatusCode = 'STATUS_CODE_ERROR') / count() * 100, 2) as error_rate,
       round(quantile(0.50)(Duration) / 1000000, 2) as p50_latency_ms,
       round(quantile(0.95)(Duration) / 1000000, 2) as p95_latency_ms,
       round(quantile(0.99)(Duration) / 1000000, 2) as p99_latency_ms
FROM traces WHERE Timestamp > now() - INTERVAL 1 HOUR
GROUP BY ServiceName

// GetServiceEdges - extracts service-to-service dependencies from parent spans
SELECT parent.ServiceName as source, child.ServiceName as target,
       count() as request_count,
       round(countIf(child.StatusCode = 'STATUS_CODE_ERROR') / count() * 100, 2) as error_rate,
       round(avg(child.Duration) / 1000000, 2) as avg_latency_ms
FROM traces child
INNER JOIN traces parent ON child.ParentSpanId = parent.SpanId
WHERE child.Timestamp > now() - INTERVAL 1 HOUR
  AND parent.ServiceName != child.ServiceName
GROUP BY source, target
```

2. **Topology Service** (api-server/internal/services/services.go):
```go
func (s *TopologyService) GetGraph(ctx context.Context) (interface{}, error) {
    stats, _ := s.clickhouse.GetServiceStats(ctx)
    edges, _ := s.clickhouse.GetServiceEdges(ctx)
    return map[string]interface{}{
        "services": buildServiceMap(stats),
        "edges":    buildEdgeMap(edges),
    }, nil
}
```

3. **D3.js Force-Directed Graph** (web-ui/src/pages/ServiceMapPage.tsx):
- Interactive force simulation with collision detection
- Arrow markers showing traffic direction
- Color-coded nodes: green (healthy), yellow (>5% errors), red (>10% errors)
- Service type icons: gateway, database, queue, service
- Edge labels with latency information
- Pulsing error indicator rings for unhealthy services
- Click to view detailed metrics panel

4. **Features (inspired by Jaeger, Datadog, Grafana Tempo):**
- Real-time topology from trace parent-child relationships
- Request rate, error rate, latency percentiles (P50/P95/P99)
- Service type detection from naming patterns
- Refresh button with last updated timestamp
- Responsive layout

**Live Demo:** http://18.209.179.35:3000/service-map

**Sources:**
- [Jaeger Service Dependencies](https://www.jaegertracing.io/docs/1.54/features/#service-dependencies)
- [Datadog Service Map](https://docs.datadoghq.com/tracing/services/service_map/)
- [Grafana Tempo Service Graph](https://grafana.com/docs/tempo/latest/metrics-generator/service-graphs/)

---

### 7. Security: Sanitize Incoming Trace Context

**Finding:** Malicious actors can forge trace headers to manipulate tracing data or exploit vulnerabilities.

**Improvement:**
```go
// In correlation processor - validate incoming context
func (p *correlationProcessor) validateContext(span ptrace.Span) bool {
    // Reject suspiciously long correlation IDs
    if len(correlationID) > 100 {
        return false
    }

    // Validate format if OllyStack-generated
    if strings.HasPrefix(correlationID, "olly-") {
        return p.validateOllyFormat(correlationID)
    }

    // For external IDs, sanitize but accept
    return p.sanitize(correlationID)
}
```

**Best Practices:**
- Ignore/sanitize context from untrusted external sources
- Don't propagate internal trace IDs to external/public endpoints
- Validate correlation ID format and length

**Source:** [OpenTelemetry Context Propagation](https://opentelemetry.io/docs/concepts/context-propagation/)

---

### 8. Use W3C Baggage for Correlation ID Propagation

**Finding:** W3C Baggage is the standard way to propagate custom context (like correlation_id) across services.

**Improvement:**
```yaml
# Propagate correlation_id via W3C Baggage header
baggage: correlation_id=olly-2k8f9x3-a7b2c4d1,tenant_id=acme

# Benefits over custom headers:
# - Standardized format
# - Auto-propagated by OTel SDKs
# - Works across languages
```

**Warning:** Don't put sensitive data in baggage - it's visible in network traffic.

**Source:** [OpenTelemetry Baggage](https://opentelemetry.io/docs/concepts/signals/baggage/)

---

### 9. Correlation ID vs Trace ID Clarification

**Finding:** Many systems use trace_id as correlation_id, which is valid but has limitations.

**Our Decision (Confirmed):**
```
Trace ID:        Request-scoped, changes per retry
Correlation ID:  Business-flow-scoped, survives retries/async

Example:
User checkout → trace_id_1, correlation_id=olly-abc
  └→ Payment retry → trace_id_2, correlation_id=olly-abc (same!)
  └→ Async email → trace_id_3, correlation_id=olly-abc (same!)
```

**Benefit:** Our correlation_id survives retries, async workflows, and message queues where trace_id would break.

**Source:** [Last9 - Correlation ID vs Trace ID](https://last9.io/blog/correlation-id-vs-trace-id/)

---

### 10. Add Profiling Signal (OTel 2025)

**Finding:** OpenTelemetry profiling is now stable in 2024-2025, enabling CPU/memory correlation with traces.

**Future Improvement (Phase 6):**
```yaml
# OTel Collector with profiling
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

pipelines:
  profiles:
    receivers: [otlp]
    processors: [batch]
    exporters: [clickhouse]
```

**Benefits:**
- Link slow traces to CPU hotspots
- Correlate memory issues with specific requests
- Continuous profiling with low overhead

**Source:** [OpenTelemetry 2024-2025 Updates](https://opentelemetry.io/docs/concepts/signals/traces/)

---

### 11. Implement Chain-of-Event for Interpretable RCA

**Finding:** Academic research (FSE 2024) shows Chain-of-Event models outperform black-box ML for RCA.

**Improvement:**
```python
# Transform multi-modal data into event chains
class ChainOfEventRCA:
    """
    Interpretable RCA aligned with SRE operations experience.
    """
    def analyze(self, correlation_id: str):
        # 1. Transform traces/logs/metrics into events
        events = self.transform_to_events(correlation_id)

        # 2. Learn weighted causal graph
        causal_graph = self.learn_causal_graph(events)

        # 3. Traverse graph to find root cause
        root_cause = self.traverse_for_root_cause(causal_graph)

        # 4. Generate interpretable explanation
        return self.explain_chain(root_cause, causal_graph)
```

**Source:** [ACM FSE 2024 - Chain-of-Event](https://dl.acm.org/doi/10.1145/3663529.3663827)

---

### 12. Database Semantic Conventions (Now Stable)

**Finding:** Database semantic conventions are now stable as of 2025, providing consistent attributes.

**Improvement:**
```yaml
# Use stable database attributes
db.system: "postgresql"
db.collection.name: "orders"       # table name
db.operation.name: "SELECT"
db.query.text: "SELECT * FROM..."  # sanitized
db.response.status_code: "OK"

# Benefits:
# - Grafana auto-detects db operations
# - Consistent across all DB types
# - Enables db-specific dashboards
```

**Source:** [Grafana Blog - Database Observability Semantic Conventions](https://grafana.com/blog/2025/06/06/database-observability-how-opentelemetry-semantic-conventions-improve-consistency-across-signals/)

---

## Updated ADRs Based on Research

### ADR-006: Causal Inference for RCA
**Decision:** Use hybrid approach - Causal graphs for root cause identification, LLM for explanation only
**Rationale:** LLMs hallucinate causes; causal inference provides reliable diagnosis

### ADR-007: W3C Baggage for Correlation
**Decision:** Propagate correlation_id via W3C Baggage header, not custom X-Correlation-ID
**Rationale:** Standard format, auto-propagated by all OTel SDKs

### ADR-008: Semantic Conventions Compliance
**Decision:** Strictly follow OTel semantic conventions for all attributes
**Rationale:** Enables Grafana/tooling auto-detection, future-proof

### ADR-009: OpenTelemetry eBPF Instrumentation
**Decision:** Use OBI (upstream) via Beyla distribution for zero-code tracing
**Rationale:** Better community support, faster development, more protocols

---

## References & Sources

### Official Documentation
- [OpenTelemetry Traces](https://opentelemetry.io/docs/concepts/signals/traces/)
- [OpenTelemetry Context Propagation](https://opentelemetry.io/docs/concepts/context-propagation/)
- [OpenTelemetry Baggage](https://opentelemetry.io/docs/concepts/signals/baggage/)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)

### Industry Articles
- [Last9 - Correlation ID vs Trace ID](https://last9.io/blog/correlation-id-vs-trace-id/)
- [Last9 - Distributed Tracing with OpenTelemetry](https://last9.io/blog/distributed-tracing-with-opentelemetry/)
- [InfoQ - Causal Reasoning in Observability](https://www.infoq.com/articles/causal-reasoning-observability/)
- [DevOps.com - Why Generic AI Falls Short for RCA](https://devops.com/why-generic-ai-models-fall-short-for-root-cause-analysis/)

### Tools & Libraries
- [Grafana Beyla / OBI](https://grafana.com/oss/beyla-ebpf/)
- [DoWhy - Causal ML Library](https://aws.amazon.com/blogs/opensource/root-cause-analysis-with-dowhy-an-open-source-python-library-for-causal-machine-learning/)
- [OpenTelemetry eBPF Instrumentation](https://opentelemetry.io/blog/2025/obi-announcing-first-release/)

### Academic Papers
- [ACM KDD 2022 - Causal Inference-Based RCA](https://dl.acm.org/doi/10.1145/3534678.3539041)
- [ACM FSE 2024 - Chain-of-Event for RCA](https://dl.acm.org/doi/10.1145/3663529.3663827)
- [ACM FSE 2024 - LLM-Based Agents for RCA](https://dl.acm.org/doi/10.1145/3663529.3663841)

---

*OllyStack - The observability platform that actually correlates*
