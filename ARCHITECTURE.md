# OllyStack Architecture

## Design Principles

1. **Simple** - Minimal components, easy to operate
2. **Fast** - Sub-second alerting, millisecond queries
3. **Cost-effective** - 10x cheaper than competitors
4. **Scalable** - Handles 1M+ events/second

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CUSTOMER HOSTS                                  │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                     Unified Agent (100MB)                           │  │
│   │  • Collect metrics, logs, traces                                   │  │
│   │  • Local aggregation (90% data reduction)                          │  │
│   │  • Intelligent sampling                                            │  │
│   │  • Disk buffer for reliability                                     │  │
│   └──────────────────────────────┬──────────────────────────────────────┘  │
│                                  │                                         │
└──────────────────────────────────┼─────────────────────────────────────────┘
                                   │ OTLP (gRPC/HTTP)
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           INGESTION LAYER                                    │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                    Ingestion Gateway (Go)                           │  │
│   │                                                                     │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │  │
│   │  │ OTLP Recv   │→│  Sampling   │→│   Quotas    │→│  Batching   │  │  │
│   │  │ gRPC/HTTP   │ │ (80% save)  │ │ Rate Limit  │ │ 10k rows    │  │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └──────┬──────┘  │  │
│   │                                                          │         │  │
│   └──────────────────────────────────────────────────────────┼─────────┘  │
│                                                              │             │
└──────────────────────────────────────────────────────────────┼─────────────┘
                                                               │
                                   ┌───────────────────────────┘
                                   │ Direct Write (Native Protocol)
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CLICKHOUSE (Storage + Analytics)                     │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                         Data Tables                                  │  │
│   │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐                │  │
│   │  │ metrics_raw  │ │  logs_raw    │ │ traces_raw   │                │  │
│   │  │ (hot tier)   │ │ (hot tier)   │ │ (hot tier)   │                │  │
│   │  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘                │  │
│   │         │                │                │                         │  │
│   │         ▼                ▼                ▼                         │  │
│   │  ┌─────────────────────────────────────────────────────────────┐   │  │
│   │  │              Materialized Views (Real-time)                  │   │  │
│   │  │  • service_metrics_1m  (RED metrics)                        │   │  │
│   │  │  • error_aggregates    (Error tracking)                     │   │  │
│   │  │  • service_topology    (Dependency map)                     │   │  │
│   │  │  • metric_anomalies    (Z-score detection)                  │   │  │
│   │  └─────────────────────────────────────────────────────────────┘   │  │
│   │                                                                     │  │
│   │  ┌─────────────────────────────────────────────────────────────┐   │  │
│   │  │                   Tiered Storage                             │   │  │
│   │  │  Hot (SSD): 0-24h     → Warm (HDD): 1-30d → Cold (S3): 30d+ │   │  │
│   │  └─────────────────────────────────────────────────────────────┘   │  │
│   │                                                                     │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Alert Engine   │     │   API Server    │     │   AI Engine     │
│                 │     │                 │     │                 │
│ • Query MVs     │     │ • REST API      │     │ • Anomaly ML    │
│ • <1s alerting  │     │ • GraphQL       │     │ • NLQ (LLM)     │
│ • AlertManager  │     │ • Auth/RBAC     │     │ • RCA           │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                      ┌─────────────────┐
                      │     Web UI      │
                      │                 │
                      │ • Dashboards    │
                      │ • Trace viewer  │
                      │ • Log explorer  │
                      │ • Service map   │
                      └─────────────────┘
```

## Key Design Decision: No Kafka

We write directly to ClickHouse instead of using Kafka as an intermediary. Here's why:

| Factor | With Kafka | Direct to ClickHouse |
|--------|-----------|---------------------|
| **Latency** | +50-100ms | Immediate |
| **Cost** | +$1,500-5,000/mo | $0 |
| **Complexity** | Kafka + ZK + Consumers | Just ClickHouse |
| **Ops Burden** | High (partitions, retention, consumer lag) | Low |
| **Data Loss Risk** | Very Low | Low (gateway buffers to disk) |
| **Max Throughput** | Unlimited | 1M+ inserts/sec |

**When you might need Kafka:**
- Multiple downstream consumers (analytics, ML, compliance)
- Strict exactly-once delivery requirements
- Multi-datacenter replication
- >10M events/second sustained

For most deployments (up to 1M events/sec), direct ClickHouse writes are simpler, cheaper, and fast enough.

## Components

### 1. Unified Agent
- **Memory**: 100MB limit
- **Function**: Collect, aggregate, sample, export
- **Output**: OTLP to Ingestion Gateway
- **Reliability**: Disk buffer (256MB) for network issues

### 2. Ingestion Gateway
- **Stateless**: Horizontally scalable
- **Sampling**: Intelligent sampling (keep errors, slow, anomalies)
- **Quotas**: Per-tenant rate limiting
- **Batching**: 10k rows per batch to ClickHouse
- **Buffer**: Disk buffer for ClickHouse downtime

### 3. ClickHouse
- **Storage**: Tiered (hot/warm/cold)
- **Real-time**: Materialized Views for instant aggregation
- **Compression**: 10-50x (Gorilla + LZ4 + Dictionary)
- **Scale**: 1M+ inserts/second

### 4. Alert Engine
- **Latency**: <1 second (queries Materialized Views)
- **Rules**: Configurable per tenant
- **Output**: AlertManager, Slack, PagerDuty, webhooks

### 5. API Server
- **Query**: SQL, PromQL, GraphQL
- **Auth**: JWT, API keys, SSO (SAML/OIDC)
- **Cache**: Redis for hot queries

### 6. AI Engine
- **Anomaly Detection**: Isolation Forest, LSTM
- **NLQ**: "Why is checkout slow?" → SQL → Answer
- **RCA**: Automatic root cause analysis

## Data Flow

```
1. Agent collects data (metrics, logs, traces)
2. Agent aggregates locally (60x reduction for metrics)
3. Agent samples (keep errors/slow, sample normal)
4. Agent sends OTLP to Gateway
5. Gateway validates, applies quotas
6. Gateway batches (10k rows)
7. Gateway writes directly to ClickHouse
8. ClickHouse MVs compute real-time aggregates
9. Alert Engine queries MVs every second
10. API serves queries from ClickHouse
```

## Scaling

| Scale | Gateways | ClickHouse | Memory |
|-------|----------|------------|--------|
| 10K events/sec | 1 | 1 node | 8GB |
| 100K events/sec | 2 | 3 nodes | 24GB |
| 1M events/sec | 5 | 6 nodes | 96GB |
| 10M events/sec | 20 | 12 nodes | 384GB |

## Cost (AWS)

| Scale | Monthly Cost | Per Million Events |
|-------|-------------|-------------------|
| Startup (100K/sec) | $500 | $0.02 |
| Growth (1M/sec) | $2,000 | $0.01 |
| Enterprise (10M/sec) | $10,000 | $0.005 |

**Comparison**: Datadog charges ~$0.10 per million = **10-20x more expensive**

## Reliability

| Failure | Impact | Recovery |
|---------|--------|----------|
| Gateway down | None (LB routes to others) | Instant |
| ClickHouse node down | None (replicas serve) | Instant |
| ClickHouse cluster down | Gateways buffer to disk | Auto-replay |
| Agent network issue | Agent buffers to disk | Auto-retry |

## Quick Start

### Development
```bash
docker-compose -f deploy/docker/docker-compose.minimal.yml up -d
```

### Production
```bash
docker-compose -f deploy/docker/docker-compose.yml up -d
```

### Kubernetes
```bash
helm install ollystack ./deploy/kubernetes/helm/ollystack \
  --namespace ollystack \
  --create-namespace \
  --values production-values.yaml
```

## Project Structure

```
/ollystack
├── agents/unified-agent/          # Collection agent
├── ingestion-gateway/             # OTLP receiver → ClickHouse
├── api-server/                    # Query API
├── ai-engine/                     # ML + LLM
├── realtime-processor/            # Alert evaluation
├── web-ui/                        # React frontend
└── deploy/
    ├── docker/                    # Docker Compose
    └── kubernetes/                # Helm charts
```

## Ports

| Service | Port | Protocol |
|---------|------|----------|
| Ingestion Gateway (gRPC) | 4317 | OTLP gRPC |
| Ingestion Gateway (HTTP) | 4318 | OTLP HTTP |
| Ingestion Gateway (Metrics) | 8888 | Prometheus |
| API Server | 8080 | HTTP/REST |
| AI Engine | 8081 | HTTP/REST |
| ClickHouse (HTTP) | 8123 | HTTP |
| ClickHouse (Native) | 9000 | TCP |
| Web UI | 3000 | HTTP |
| AlertManager | 9093 | HTTP |
| Grafana | 3001 | HTTP |
| Redis | 6379 | Redis |
