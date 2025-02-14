# OllyStack Control Plane

Central management for OpenTelemetry collectors using OpAMP (Open Agent Management Protocol).

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CONTROL PLANE                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      OpAMP Server (:4320)                            │   │
│  │  • Agent Registration & Health Monitoring                            │   │
│  │  • Central Configuration Management                                  │   │
│  │  • Fleet Status Dashboard                                            │   │
│  │  • REST API for Management                                           │   │
│  └─────────────────────────────┬───────────────────────────────────────┘   │
│                                │                                             │
│            ┌───────────────────┼───────────────────┐                        │
│            │                   │                   │                        │
│            ▼                   ▼                   ▼                        │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐              │
│  │ Redis           │ │ Config Store    │ │ Fleet Manager   │              │
│  │ (Cache/State)   │ │ (Configs)       │ │ (Monitoring)    │              │
│  └─────────────────┘ └─────────────────┘ └─────────────────┘              │
└─────────────────────────────────┼───────────────────────────────────────────┘
                                  │ OpAMP Protocol (WebSocket)
┌─────────────────────────────────┼───────────────────────────────────────────┐
│                           DATA PLANE                                         │
│                                 │                                            │
│  ┌──────────────────────────────▼──────────────────────────────────────┐   │
│  │                    Gateway Collector (:4317/:4318)                   │   │
│  │  • Central aggregation point                                         │   │
│  │  • Receives from agents & applications                               │   │
│  │  • Exports to backend storage                                        │   │
│  └──────────────────────────────▲──────────────────────────────────────┘   │
│                                 │                                            │
│         ┌───────────────────────┼───────────────────────┐                   │
│         │                       │                       │                   │
│  ┌──────┴──────┐  ┌─────────────┴───────────┐  ┌───────┴────────┐          │
│  │ Agent       │  │ Agent                   │  │ Beyla (eBPF)   │          │
│  │ Collector   │  │ Collector               │  │ (Zero-Code)    │          │
│  │ (Node 1)    │  │ (Node 2)                │  │                │          │
│  └──────┬──────┘  └─────────────┬───────────┘  └───────┬────────┘          │
│         │                       │                       │                   │
└─────────┼───────────────────────┼───────────────────────┼───────────────────┘
          │                       │                       │
┌─────────┼───────────────────────┼───────────────────────┼───────────────────┐
│         │              APPLICATION LAYER                │                   │
│  ┌──────▼──────┐  ┌─────────────▼───────────┐  ┌───────▼────────┐          │
│  │ Go Services │  │ Python Services         │  │ Any Service    │          │
│  │ (OTel SDK)  │  │ (Auto-instrumentation)  │  │ (eBPF capture) │          │
│  └─────────────┘  └─────────────────────────┘  └────────────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STORAGE LAYER                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ ClickHouse      │  │ Prometheus      │  │ Object Storage  │             │
│  │ (Traces/Logs)   │  │ (Metrics)       │  │ (Long-term)     │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Components

### Control Plane

| Component | Port | Description |
|-----------|------|-------------|
| **OpAMP Server** | 4320 | Central agent management, config distribution |
| **Redis** | 6379 | Configuration persistence, caching |

### Data Plane

| Component | Port | Description |
|-----------|------|-------------|
| **Gateway Collector** | 4317/4318 | OTLP receiver, aggregation, export |
| **Agent Collector** | - | Per-node collection, forwards to gateway |
| **Beyla (eBPF)** | - | Zero-code instrumentation (optional) |

## Quick Start

### Start Control Plane

```bash
cd control-plane
docker-compose up -d
```

### Start with eBPF Instrumentation

```bash
docker-compose --profile ebpf up -d
```

### Verify Status

```bash
# Check OpAMP server
curl http://localhost:4320/api/v1/health

# Check fleet status
curl http://localhost:4320/api/v1/fleet/status

# List connected agents
curl http://localhost:4320/api/v1/agents
```

## OpAMP API

### Agents

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/agents` | GET | List all connected agents |
| `/api/v1/agents/{id}` | GET | Get agent details |
| `/api/v1/agents/{id}/config` | PUT | Assign config to agent |

### Configurations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/configs` | GET | List all configurations |
| `/api/v1/configs` | POST | Create new configuration |
| `/api/v1/configs/{id}` | GET | Get configuration |
| `/api/v1/configs/{id}` | PUT | Update configuration |
| `/api/v1/configs/{id}` | DELETE | Delete configuration |

### Fleet Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/fleet/status` | GET | Get fleet health summary |
| `/api/v1/health` | GET | Server health check |

## Configuration Management

### Create a New Config

```bash
curl -X POST http://localhost:4320/api/v1/configs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-config",
    "description": "Production collector configuration",
    "config": "receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: 0.0.0.0:4317\n...",
    "labels": {
      "environment": "production"
    }
  }'
```

### Assign Config to Agent

```bash
curl -X PUT http://localhost:4320/api/v1/agents/agent-001/config \
  -H "Content-Type: application/json" \
  -d '{
    "config_id": "config-uuid"
  }'
```

## Instrumentation Methods

### 1. OTel SDK (Current - Requires Code)

Go services use the OTel SDK with manual instrumentation:

```go
import "go.opentelemetry.io/otel"

tracer := otel.Tracer("service-name")
ctx, span := tracer.Start(ctx, "operation")
defer span.End()
```

### 2. Auto-Instrumentation (Python/Java/Node.js)

For languages with runtime hooks:

```bash
# Python
opentelemetry-instrument python app.py

# Java
java -javaagent:opentelemetry-javaagent.jar -jar app.jar

# Node.js
node --require @opentelemetry/auto-instrumentations-node app.js
```

### 3. eBPF Zero-Code (Go/Rust/C++)

For compiled languages, use Grafana Beyla:

```yaml
# Enable in docker-compose
docker-compose --profile ebpf up -d
```

Beyla automatically instruments:
- HTTP/HTTPS requests (client & server)
- gRPC calls
- SQL database queries
- Redis operations

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | 4320 | OpAMP server port |
| `REDIS_URL` | localhost:6379 | Redis connection |
| `COLLECTOR_INSTANCE_ID` | auto | Unique collector ID |
| `DEPLOYMENT_ENV` | production | Environment label |
| `CLICKHOUSE_PASSWORD` | ollystack123 | ClickHouse password |

## Monitoring

### Prometheus Metrics

The OpAMP server exposes Prometheus metrics at `/metrics`:

```yaml
# Agent count by status
opamp_agents_total{status="healthy"} 5
opamp_agents_total{status="unhealthy"} 1
opamp_agents_total{status="disconnected"} 0

# Config distribution
opamp_config_updates_total 42
opamp_config_errors_total 0
```

### Health Checks

```bash
# OpAMP server health
curl http://localhost:4320/api/v1/health

# Collector health
curl http://localhost:13133/
```

## Troubleshooting

### Agent Not Connecting

1. Check OpAMP server is running:
   ```bash
   docker logs ollystack-opamp-server
   ```

2. Verify network connectivity:
   ```bash
   docker exec ollystack-gateway-collector wget -q -O - http://opamp-server:4320/api/v1/health
   ```

3. Check collector logs:
   ```bash
   docker logs ollystack-gateway-collector
   ```

### Config Not Applying

1. Verify config syntax:
   ```bash
   curl http://localhost:4320/api/v1/configs/{id}
   ```

2. Check agent capabilities:
   ```bash
   curl http://localhost:4320/api/v1/agents/{id}
   ```

### eBPF Not Working

1. Ensure privileged mode is enabled
2. Check kernel requirements (Linux 5.8+)
3. Verify Beyla logs:
   ```bash
   docker logs ollystack-beyla
   ```
