# OllyStack Production Architecture

## Design Principles

1. **Never lose data** - Kafka provides durability
2. **Real-time alerts** - Stream processor for <1 second alerting
3. **Horizontal scale** - Every component scales independently
4. **Graceful degradation** - If one component fails, others continue
5. **Cost efficient** - Optimize for high volume, low cost per GB

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CUSTOMER HOSTS                                  │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                      Unified Agent (100MB)                           │  │
│   │  • Collect metrics, logs, traces                                    │  │
│   │  • Local aggregation (90% reduction)                                │  │
│   │  • Adaptive sampling                                                │  │
│   │  • Cardinality control                                              │  │
│   └──────────────────────────────┬──────────────────────────────────────┘  │
│                                  │                                         │
└──────────────────────────────────┼─────────────────────────────────────────┘
                                   │ OTLP (compressed, sampled)
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           INGESTION LAYER                                    │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                    Ingestion Gateway (Go)                            │  │
│   │  • OTLP receiver (gRPC/HTTP)                                        │  │
│   │  • Protocol validation                                              │  │
│   │  • Rate limiting per customer                                       │  │
│   │  • Tenant isolation                                                 │  │
│   │  • Write to Kafka                                                   │  │
│   └──────────────────────────────┬──────────────────────────────────────┘  │
│                                  │                                         │
└──────────────────────────────────┼─────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         KAFKA (Durability Layer)                            │
│                                                                             │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│   │   metrics   │  │    logs     │  │   traces    │  │   alerts    │      │
│   │   topic     │  │   topic     │  │   topic     │  │   topic     │      │
│   │  (7 days)   │  │  (3 days)   │  │  (3 days)   │  │  (30 days)  │      │
│   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────────┘      │
│          │                │                │                               │
│          └────────────────┼────────────────┘                               │
│                           │                                                │
└───────────────────────────┼────────────────────────────────────────────────┘
                            │
            ┌───────────────┼───────────────┐
            │               │               │
            ▼               ▼               ▼
┌───────────────────┐ ┌───────────────┐ ┌───────────────────┐
│  Storage Consumer │ │Real-time Proc.│ │   ML Pipeline     │
│                   │ │               │ │                   │
│  • Batch insert   │ │ • Alert eval  │ │ • Anomaly detect  │
│  • ClickHouse     │ │ • <1s latency │ │ • Pattern learn   │
│  • 99.99% durable │ │ • Stateless   │ │ • Batch process   │
└─────────┬─────────┘ └───────┬───────┘ └─────────┬─────────┘
          │                   │                   │
          ▼                   ▼                   ▼
┌───────────────────┐ ┌───────────────┐ ┌───────────────────┐
│    ClickHouse     │ │ Alert Manager │ │    AI Engine      │
│    (Storage)      │ │  (PagerDuty,  │ │  (Models, LLM)    │
│                   │ │   Slack, etc) │ │                   │
└───────────────────┘ └───────────────┘ └───────────────────┘
          │                                       │
          └───────────────────┬───────────────────┘
                              ▼
                    ┌───────────────────┐
                    │    API Server     │
                    │   (Query Layer)   │
                    └─────────┬─────────┘
                              │
                              ▼
                    ┌───────────────────┐
                    │      Web UI       │
                    └───────────────────┘
```

## Why Kafka?

### 1. Durability (Never Lose Data)

```
Without Kafka:
  Agent → Stream Processor → ClickHouse
              ↓
         If crashes, data LOST

With Kafka:
  Agent → Kafka → Stream Processor
              ↓         ↓
         If crashes, replay from Kafka
```

### 2. Decoupled Scaling

```
Each consumer scales independently:

Kafka Partitions: 64
  ├── Storage Consumers: 8 instances (batch writes)
  ├── Real-time Processors: 16 instances (alert evaluation)
  └── ML Pipeline: 4 instances (anomaly detection)

Traffic spike? Scale consumers, not Kafka.
```

### 3. Replay Capability

```
Scenarios where replay saves you:
  • Bug in processing code → fix and replay
  • ClickHouse maintenance → pause consumer, replay after
  • New feature → backfill by replaying
  • Customer asks "what happened 3 days ago" → replay to debug
```

### 4. Multi-Consumer Pattern

```
Same data, multiple uses:

metrics-topic
  ├── Consumer 1: ClickHouse (storage)
  ├── Consumer 2: Real-time alerting
  ├── Consumer 3: Anomaly detection
  ├── Consumer 4: Usage metering (billing)
  └── Consumer 5: Debug/troubleshooting
```

## Component Details

### 1. Unified Agent (Per Host)
- **Memory**: 100MB limit
- **Function**: Collect, aggregate, sample, export
- **Output**: OTLP to Ingestion Gateway
- **Reliability**: Disk buffer for network issues

### 2. Ingestion Gateway (Stateless, Horizontally Scaled)
- **Memory**: 512MB per instance
- **Instances**: Auto-scale based on traffic
- **Function**: Validate, rate limit, route to Kafka
- **Reliability**: Stateless, any instance handles any request

### 3. Kafka (Durability Backbone)
- **Retention**: 3-7 days (configurable per topic)
- **Partitions**: Based on throughput (start with 32)
- **Replication**: Factor of 3 (survive 2 failures)
- **Why not Pulsar/Redpanda**: Kafka has best ecosystem, proven at scale

### 4. Storage Consumer (Batch Writer)
- **Pattern**: Micro-batch (every 1-5 seconds)
- **Function**: Read from Kafka, batch insert to ClickHouse
- **Reliability**: At-least-once delivery, ClickHouse dedupes
- **Scaling**: 1 consumer per N partitions

### 5. Real-time Processor (Alert Evaluator)
- **Latency**: <1 second end-to-end
- **Function**: Evaluate alert rules, emit to alerts topic
- **State**: Minimal (use Redis for short-term state)
- **Scaling**: Stateless, scale horizontally

### 6. ML Pipeline (Async Analysis)
- **Pattern**: Batch processing
- **Function**: Train models, detect anomalies, update baselines
- **Frequency**: Every 1-5 minutes
- **Not in critical path**: Failures don't affect alerting

## Failure Scenarios

### Scenario 1: ClickHouse Down
```
Impact: Queries fail, dashboards empty
Data: Safe in Kafka (3-7 day retention)
Recovery: Fix ClickHouse, consumer replays from Kafka
Customer impact: Can't view historical data, but alerts still work
```

### Scenario 2: Real-time Processor Down
```
Impact: Alerts delayed
Data: Safe in Kafka, storage continues
Recovery: Restart processor, it catches up
Customer impact: Alerts may be late by minutes
```

### Scenario 3: Kafka Down (Rare - 3x Replicated)
```
Impact: Ingestion stops
Data: Agents buffer to disk (256MB)
Recovery: Kafka recovers, agents flush buffer
Customer impact: Temporary gap in data
```

### Scenario 4: Ingestion Gateway Down
```
Impact: None (stateless, load balanced)
Recovery: Other instances handle traffic
Customer impact: None (millisecond failover)
```

## Scaling Guidelines

### For 1M Agents (Startup Scale)
```
Ingestion Gateways: 3 instances
Kafka: 3 brokers, 32 partitions
Storage Consumers: 4 instances
Real-time Processors: 4 instances
ClickHouse: 3 node cluster
```

### For 10M Agents (Growth Scale)
```
Ingestion Gateways: 10 instances (auto-scale)
Kafka: 5 brokers, 128 partitions
Storage Consumers: 16 instances
Real-time Processors: 16 instances
ClickHouse: 6 node cluster (sharded)
```

### For 100M Agents (Enterprise Scale)
```
Ingestion Gateways: 50+ instances (auto-scale)
Kafka: 20 brokers, 512 partitions
Storage Consumers: 64 instances
Real-time Processors: 64 instances
ClickHouse: 20+ node cluster (multi-shard)
```

## Cost Analysis (AWS, 10M events/second)

| Component | Instances | Cost/Month |
|-----------|-----------|------------|
| Ingestion Gateway | 10 × c6g.large | $500 |
| Kafka (MSK) | 5 × kafka.m5.large | $1,500 |
| Storage Consumer | 16 × c6g.medium | $400 |
| Real-time Processor | 16 × c6g.medium | $400 |
| ClickHouse | 6 × r6g.2xlarge | $3,000 |
| **Total Compute** | | **$5,800** |
| Storage (S3 tiered) | 100TB | $2,300 |
| **Total** | | **~$8,000/month** |

Cost per million events: **$0.80**
(Compare to Datadog: ~$15 per million)

## Implementation Priority

### Phase 1: Foundation (Week 1-2)
1. ✅ Unified Agent (done)
2. Ingestion Gateway (simple OTLP → Kafka)
3. Kafka setup (Docker for dev, MSK for prod)
4. Storage Consumer (Kafka → ClickHouse)

### Phase 2: Real-time (Week 3-4)
1. Real-time Processor (alert evaluation)
2. Alert Manager integration
3. Alert rules API

### Phase 3: Intelligence (Week 5-6)
1. ML Pipeline (anomaly detection)
2. AI Engine integration
3. Automated investigation triggers

### Phase 4: Scale (Ongoing)
1. Multi-tenancy
2. Horizontal scaling
3. Global distribution
