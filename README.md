# OllyStack

**Universal Observability Platform** - End-to-end visibility from browser to database, across any infrastructure.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-Native-blueviolet)](https://opentelemetry.io/)

**Author:** Madhukar Beema, Distinguished Engineer

## Overview

OllyStack is an open-source observability platform that unifies metrics, logs, traces, and profiles into a single, coherent view. Built for the cloud-native era with AI-powered analytics.

### Philosophy

**Build where we differentiate, integrate where commoditized.**

We leverage proven, high-performance OSS agents for data collection and focus engineering on real-time analytics, AI-powered insights, and unified querying.

### Key Features

- **Zero-Code Instrumentation** - Grafana Beyla (eBPF) for automatic tracing
- **End-to-End Tracing** - Single trace from browser → Load Balancer → APIs → Database
- **Real-Time Analysis** - Stream processing with ML anomaly detection before storage
- **Multi-Platform** - VMs, App Services, Kubernetes, bare metal - any cloud
- **OpenTelemetry Native** - OTLP as the universal protocol
- **AI-Powered** - Natural language queries, root cause analysis, predictive alerting

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                           APPLICATIONS                                        │
│     Java (OTel Agent)  │  Python (OTel)  │  Node.js (OTel)  │  Go (OTel)     │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │ OTLP
┌───────────────────────────────┼──────────────────────────────────────────────┐
│                    COLLECTION LAYER (Best-in-Class OSS)                       │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐   │
│  │  Grafana Beyla  │  │     Vector      │  │   OpenTelemetry Collector   │   │
│  │  (eBPF auto-    │  │  (Log shipping) │  │   (hostmetrics, receivers)  │   │
│  │   instrument)   │  │    Rust-based   │  │                             │   │
│  └────────┬────────┘  └────────┬────────┘  └──────────────┬──────────────┘   │
└───────────┴─────────────────────┴─────────────────────────┴──────────────────┘
                                │ OTLP
                                ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                 OPENTELEMETRY COLLECTOR (with custom processors)              │
│  • Enrichment (K8s, cloud, geo-IP)     • Tail-based sampling                 │
│  • Topology discovery                   • Data transformation                │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                   OLLYSTACK STREAM PROCESSOR (Our Value-Add)                   │
│   Anomaly Detection  │  Trace Correlation  │  Service Topology  │  Alerting  │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                         STORAGE (ClickHouse)                                  │
│        Traces Table    │    Metrics Table    │    Logs Table                 │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                      QUERY & PRESENTATION (Our Value-Add)                     │
│   Query Engine (ObservQL)  │  API Server  │  Web UI  │  AI Copilot           │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Local Development

```bash
# Clone and start
git clone https://github.com/ollystack/ollystack.git
cd ollystack
make dev-up

# Access
open http://localhost:3000   # Web UI
curl http://localhost:8080/health  # API
```

### AWS Deployment (10 minutes)

```bash
# Set credentials
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
export AWS_REGION="us-east-2"

# Bootstrap and deploy
make aws-bootstrap ENV=dev
make aws-deploy-all ENV=dev
```

See [Quick Start Guide](docs/QUICKSTART.md) for more options.

## Documentation

| Document | Description |
|----------|-------------|
| [Quick Start](docs/QUICKSTART.md) | Get running in 10 minutes |
| [Deployment Guide](docs/DEPLOYMENT.md) | Complete AWS deployment guide |
| [CI/CD Setup](.github/CICD-SETUP.md) | GitHub Actions configuration |
| [Architecture](docs/ARCHITECTURE.md) | System design and components |

## Prerequisites

- Docker & Docker Compose
- Go 1.22+ (for building custom components)
- Rust 1.75+ (for stream processor)
- Node.js 20+ (for web UI)
- AWS CLI + Terraform (for cloud deployment)

## Components

### Leveraged OSS (Collection Layer)

| Component | Purpose | Why We Use It |
|-----------|---------|---------------|
| **OTel Collector** | Telemetry ingestion & routing | Industry standard, battle-tested |
| **Grafana Beyla** | eBPF auto-instrumentation | Zero-code HTTP/gRPC/SQL tracing |
| **Vector** | Log collection | Rust-based, 10x faster than alternatives |
| **OTel SDKs** | Application instrumentation | All languages supported |

### Custom Components (Our Value-Add)

| Component | Language | Description |
|-----------|----------|-------------|
| [Stream Processor](stream-processor) | Rust | Real-time anomaly detection, trace correlation |
| [API Server](api-server) | Go | REST/GraphQL API, query execution |
| [Query Engine](query-engine) | Rust | ObservQL - unified query language |
| [Web UI](web-ui) | TypeScript | React dashboard with AI copilot |
| [Custom Processors](collector/processors) | Go | OTel Collector enrichment & sampling |

## Instrumentation

### Option 1: Zero-Code with Beyla (eBPF)

No code changes required - Beyla automatically instruments HTTP, gRPC, SQL:

```bash
# Run alongside your application
docker run --rm --privileged \
  -e BEYLA_OPEN_PORT=8080 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
  --pid host \
  grafana/beyla:latest
```

### Option 2: OTel SDK Auto-Instrumentation

```bash
# Java
java -javaagent:opentelemetry-javaagent.jar \
  -Dotel.service.name=my-service \
  -Dotel.exporter.otlp.endpoint=http://otel-collector:4317 \
  -jar myapp.jar

# Python
opentelemetry-instrument \
  --service_name my-service \
  python myapp.py

# Node.js
node --require @opentelemetry/auto-instrumentations-node/register app.js
```

## Query Language (ObservQL)

```sql
-- Trace analysis
TRACE FROM service = 'api-gateway'
WHERE duration > 500ms
FOLLOW DOWNSTREAM
SELECT service, operation, duration, status

-- Cross-signal correlation
SELECT m.cpu_usage, l.message, t.trace_id
FROM metrics m
JOIN logs l ON m.timestamp ~ l.timestamp
JOIN traces t ON l.trace_id = t.trace_id
WHERE m.cpu_usage > 80 AND l.severity = 'ERROR'

-- Natural language (AI-powered)
ASK "Why was checkout slow yesterday?"
```

## Configuration

### OTel Collector

```yaml
# deploy/docker/config/otel-collector/config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
  hostmetrics:
    collection_interval: 10s
    scrapers:
      cpu:
      memory:
      disk:
      network:

processors:
  batch:
  tail_sampling:
    policies:
      - name: errors
        type: status_code
        status_codes: [ERROR]
      - name: high-latency
        type: latency
        latency:
          threshold_ms: 1000

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
```

### Vector (Logs)

```toml
# deploy/docker/config/vector/vector.toml
[sources.docker_logs]
type = "docker_logs"

[transforms.parse]
type = "remap"
inputs = ["docker_logs"]
source = '''
  .severity = if contains(.message, "ERROR") { "ERROR" } else { "INFO" }
'''

[sinks.otlp]
type = "opentelemetry"
inputs = ["parse"]
endpoint = "http://otel-collector:4317"
```

## Roadmap

- [x] Phase 1: Foundation (OTel Collector, Vector, ClickHouse, Basic UI)
- [x] Phase 2: Stream Processor (anomaly detection, correlation)
- [ ] Phase 3: AI Engine (NLQ, root cause analysis)
- [ ] Phase 4: Custom Query Engine (ObservQL)
- [ ] Phase 5: Enterprise Features (RBAC, SSO, multi-tenancy)

## Why OllyStack?

| Feature | Traditional Tools | OllyStack |
|---------|-------------------|----------|
| **Collection** | Custom agents | Best-in-class OSS (OTel, Beyla, Vector) |
| **Analysis** | Post-storage queries | Real-time stream processing |
| **Correlation** | Manual | Automatic cross-signal correlation |
| **AI** | None | Natural language queries, auto RCA |
| **Cost** | High (vendor lock-in) | Open source, ClickHouse storage |

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Key areas for contribution:
- Custom OTel Collector processors
- Stream processor analytics algorithms
- AI/ML models for anomaly detection
- Web UI components

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Community

- [Discord](https://discord.gg/ollystack)
- [GitHub Discussions](https://github.com/ollystack/ollystack/discussions)
- [Twitter](https://twitter.com/ollystack)
