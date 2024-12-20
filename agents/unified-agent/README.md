# OllyStack Unified Agent

A lightweight, efficient observability agent that collects metrics, logs, and traces with local aggregation to minimize resource usage and network bandwidth.

## Key Features

| Feature | Benefit | Savings |
|---------|---------|---------|
| **Local Aggregation** | Aggregate metrics before sending | 60x data reduction |
| **Pattern Deduplication** | Group similar logs | 70-90% log reduction |
| **Adaptive Sampling** | Auto-adjust based on volume | Stay within budget |
| **Cardinality Control** | Prevent metric explosion | Cost protection |
| **Single Binary** | No external dependencies | Simple deployment |

## Resource Usage

**Target:** <100MB memory, <2% CPU

| Component | Memory | CPU |
|-----------|--------|-----|
| Metrics Collector | ~20MB | <0.5% |
| Logs Collector | ~15MB | <0.5% |
| Traces Receiver | ~10MB | <0.2% |
| Pipeline + Aggregator | ~30MB | <0.5% |
| Buffer (max) | 25MB | - |
| **Total** | **~100MB** | **<2%** |

## Comparison vs Alternatives

| Agent | Memory | Features | Complexity |
|-------|--------|----------|------------|
| **OllyStack Agent** | ~100MB | Unified + Aggregation | Low |
| OTel Collector | ~150MB | Flexible | Medium |
| Datadog Agent | ~200MB | Full APM | Medium |
| Vector | ~50MB | Logs only | Low |
| Telegraf | ~100MB | Metrics only | Low |

## Quick Start

### Docker

```bash
# Run with default config
docker run -d \
  --name ollystack-agent \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 8888:8888 \
  -v /var/log:/var/log:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e OLLYSTACK_ENDPOINT=your-collector:4317 \
  ollystack/unified-agent:latest
```

### Kubernetes

```bash
# Create namespace and deploy
kubectl apply -f deploy/kubernetes/daemonset.yaml

# Set your endpoint
kubectl -n ollystack create configmap ollystack-agent-config \
  --from-literal=endpoint=your-collector:4317

# Set API key (if required)
kubectl -n ollystack create secret generic ollystack-agent-secrets \
  --from-literal=api-key=YOUR_API_KEY
```

### Binary

```bash
# Download
curl -LO https://github.com/ollystack/unified-agent/releases/latest/download/ollystack-agent-linux-amd64

# Run
chmod +x ollystack-agent-linux-amd64
./ollystack-agent-linux-amd64 -c /etc/ollystack/agent.yaml
```

## Configuration

### Minimal Configuration

```yaml
# /etc/ollystack/agent.yaml
export:
  endpoint: your-collector:4317

metrics:
  enabled: true
  interval: 15s

logs:
  enabled: true
  sources:
    - type: file
      path: /var/log/app/*.log

traces:
  enabled: true
```

### Production Configuration

See [config/agent.yaml](config/agent.yaml) for a fully documented production configuration.

### Key Configuration Options

#### Local Aggregation (60x data reduction)

```yaml
aggregation:
  enabled: true
  window: 60s  # Aggregate over 1 minute
  metrics:
    aggregates: [min, max, avg, count, p95, p99]
    drop_raw: true  # Don't send individual points
```

**Before aggregation:** 1 data point/second = 60 points/minute
**After aggregation:** 7 aggregates/minute (min, max, avg, count, p95, p99, last)

#### Log Deduplication (70-90% reduction)

```yaml
logs:
  deduplication:
    enabled: true
    window: 60s
    max_patterns: 10000
```

Similar logs are grouped:
```
[100x] Connection timeout to database at <IP>:<NUM>
[50x] Request completed in <NUM>ms
```

#### Adaptive Sampling

```yaml
sampling:
  enabled: true
  target_rate: 1048576  # 1MB/s max

  traces:
    rate: 0.1  # 10% base rate
    always_sample_errors: true
    slow_threshold: 1s
```

Agent automatically adjusts sampling to stay within budget while capturing important traces.

#### Cardinality Control

```yaml
cardinality:
  enabled: true
  max_series_per_metric: 10000
  max_label_values:
    user_id: 0      # Never use as label
    endpoint: 1000  # Limit unique values
  drop_labels: [password, token, secret]
```

Prevents cost explosion from high-cardinality labels.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    OllyStack Unified Agent                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Metrics   │  │    Logs     │  │   Traces    │         │
│  │  Collector  │  │  Collector  │  │  Receiver   │         │
│  │  (gopsutil) │  │  (file/jd)  │  │   (OTLP)    │         │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘         │
│         │                │                │                 │
│         └────────────────┼────────────────┘                 │
│                          ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                    Pipeline                          │   │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────┐         │   │
│  │  │ Sampling  │ │Cardinality│ │Enrichment │         │   │
│  │  │ (adaptive)│ │ Control   │ │(K8s/Cloud)│         │   │
│  │  └───────────┘ └───────────┘ └───────────┘         │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                   Aggregator                         │   │
│  │  • Metrics: min/max/avg/count/percentiles           │   │
│  │  • Logs: pattern deduplication                      │   │
│  │  • 60-90% data reduction                            │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                 OTLP Exporter                        │   │
│  │  • Batching (1000 items or 5s)                      │   │
│  │  • Retry with backoff                               │   │
│  │  • Disk buffer for reliability                      │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    OTLP (gRPC or HTTP)
                              │
                              ▼
              ┌───────────────────────────┐
              │   OTel Collector / Backend │
              └───────────────────────────┘
```

## Endpoints

| Port | Protocol | Purpose |
|------|----------|---------|
| 4317 | gRPC | OTLP traces/metrics/logs from apps |
| 4318 | HTTP | OTLP traces/metrics/logs from apps |
| 8888 | HTTP | Health checks and self-metrics |

### Health Endpoints

```bash
# Liveness check
curl http://localhost:8888/health

# Readiness check
curl http://localhost:8888/ready

# Prometheus metrics
curl http://localhost:8888/metrics

# Status JSON
curl http://localhost:8888/status
```

## Metrics Collected

### Host Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `system.cpu.utilization` | gauge | CPU usage percentage |
| `system.cpu.user` | gauge | User CPU time per core |
| `system.cpu.system` | gauge | System CPU time per core |
| `system.memory.used` | gauge | Memory used (bytes) |
| `system.memory.utilization` | gauge | Memory usage percentage |
| `system.disk.read_bytes` | counter | Disk read bytes |
| `system.disk.write_bytes` | counter | Disk write bytes |
| `system.network.bytes_recv` | counter | Network bytes received |
| `system.network.bytes_sent` | counter | Network bytes sent |
| `system.filesystem.utilization` | gauge | Filesystem usage percentage |

### Agent Self-Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ollystack.agent.memory_alloc` | gauge | Agent memory usage |
| `ollystack.agent.goroutines` | gauge | Active goroutines |
| `ollystack_agent_metrics_collected_total` | counter | Total metrics collected |
| `ollystack_agent_logs_collected_total` | counter | Total logs collected |
| `ollystack_agent_traces_received_total` | counter | Total traces received |
| `ollystack_agent_aggregation_savings_ratio` | gauge | Data reduction ratio |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OLLYSTACK_ENDPOINT` | Backend endpoint | `localhost:4317` |
| `OLLYSTACK_API_KEY` | API key for auth | - |
| `OLLYSTACK_ENVIRONMENT` | Environment tag | `production` |
| `OLLYSTACK_HOSTNAME` | Override hostname | auto-detected |

## Integrations

### Send Traces from Applications

Configure your application's OTLP exporter to send to the agent:

```yaml
# OpenTelemetry SDK configuration
OTEL_EXPORTER_OTLP_ENDPOINT: http://localhost:4317
# or for Kubernetes
OTEL_EXPORTER_OTLP_ENDPOINT: http://ollystack-agent.ollystack.svc:4317
```

### With Beyla (eBPF Auto-instrumentation)

For automatic instrumentation without code changes:

```yaml
traces:
  beyla:
    enabled: true
```

Requires running with `CAP_SYS_ADMIN` capability.

## Performance Tuning

### High-Volume Environments

```yaml
# Increase batch size
export:
  batch:
    max_size: 5000
    timeout: 10s

# More aggressive aggregation
aggregation:
  window: 120s

# Lower sampling
sampling:
  traces:
    rate: 0.01  # 1%
```

### Low-Latency Environments

```yaml
# Smaller batches, faster flush
export:
  batch:
    max_size: 100
    timeout: 1s

# Shorter aggregation window
aggregation:
  window: 15s
```

## Troubleshooting

### Check Agent Status

```bash
curl http://localhost:8888/status
```

### View Logs

```bash
# Docker
docker logs ollystack-agent

# Kubernetes
kubectl -n ollystack logs -l app.kubernetes.io/name=ollystack-agent

# Systemd
journalctl -u ollystack-agent
```

### Common Issues

**High Memory Usage**
- Reduce `max_patterns` in log deduplication
- Lower `max_series_per_metric`
- Increase aggregation window

**Missing Data**
- Check sampling configuration
- Verify endpoint connectivity
- Check cardinality limits

**High CPU Usage**
- Increase collection interval
- Disable process collector
- Enable more aggressive sampling

## Building from Source

```bash
# Clone repository
git clone https://github.com/ollystack/unified-agent.git
cd unified-agent

# Build binary
go build -o ollystack-agent ./cmd/agent

# Build Docker image
docker build -t ollystack/unified-agent:latest .

# Run tests
go test ./...
```

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
