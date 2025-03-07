# OllyStack Docker Deployments

This directory contains Docker Compose configurations for different deployment scenarios.

## Deployment Options

| Deployment | Use Case | Components | Complexity |
|------------|----------|------------|------------|
| **Minimal** | Development, small teams | Agent → ClickHouse | Simple |
| **Simple** | Small production | Agent → Stream Processor → ClickHouse | Medium |
| **Production** | Large scale, high reliability | Agent → Ingestion Gateway → Kafka → Consumers → ClickHouse | Enterprise |

## Quick Start

### Development (Minimal)
```bash
docker-compose -f docker-compose.minimal.yml up -d
```
- Direct write to ClickHouse
- ClickHouse Materialized Views for real-time analytics
- Best for: development, POCs, small teams (<10 services)

### Production
```bash
docker-compose -f docker-compose.production.yml up -d
```
- Kafka-backed architecture
- Never lose data (7-day retention)
- Sub-second alerting
- Best for: production workloads, millions of events/sec

## Architecture Comparison

### Minimal Architecture
```
Agent → ClickHouse (with Materialized Views)
         ↓
     API Server
         ↓
       Web UI
```
- **Pros**: Simple, low resource usage, fast to deploy
- **Cons**: Single point of failure, limited replay capability

### Production Architecture
```
Agent → Ingestion Gateway → Kafka → Storage Consumer → ClickHouse
                               ↓
                      Real-time Processor → AlertManager
                               ↓
                         ML Pipeline → AI Engine
```
- **Pros**: Durable, scalable, real-time alerting, replay capability
- **Cons**: More complex, requires more resources

## Component Details

### Ingestion Gateway
- **Purpose**: Receives OTLP telemetry, validates, rate limits, writes to Kafka
- **Ports**: 4317 (gRPC), 4318 (HTTP), 8888 (metrics)
- **Memory**: 512MB

### Storage Consumer
- **Purpose**: Reads from Kafka, batch writes to ClickHouse
- **Ports**: 8889 (metrics)
- **Memory**: 512MB

### Real-time Processor
- **Purpose**: Evaluates alert rules in <1 second
- **Ports**: 8890 (metrics)
- **Memory**: 256MB

### Kafka
- **Purpose**: Durability backbone, fan-out to multiple consumers
- **Topics**: ollystack-metrics, ollystack-logs, ollystack-traces, ollystack-alerts
- **Retention**: 7 days (metrics), 3 days (logs/traces), 30 days (alerts)

## Configuration

### Environment Variables

```bash
# Kafka
KAFKA_BROKERS=kafka:29092

# ClickHouse
CLICKHOUSE_HOST=clickhouse
CLICKHOUSE_PORT=9000
CLICKHOUSE_USER=ollystack
CLICKHOUSE_PASSWORD=your-secure-password

# AI Engine (optional)
OPENAI_API_KEY=your-key
ANTHROPIC_API_KEY=your-key

# AlertManager
ALERTMANAGER_URL=http://alertmanager:9093
```

### Scaling

For higher throughput, scale horizontally:

```bash
# Scale storage consumers (more write throughput)
docker-compose -f docker-compose.production.yml up -d --scale storage-consumer=4

# Scale real-time processors (more alert evaluation capacity)
docker-compose -f docker-compose.production.yml up -d --scale realtime-processor=4
```

## Monitoring

### Prometheus Metrics

All components expose Prometheus metrics:
- Ingestion Gateway: `http://localhost:8888/metrics`
- Storage Consumer: `http://localhost:8889/metrics`
- Real-time Processor: `http://localhost:8890/metrics`

### Key Metrics

```promql
# Ingestion rate
rate(ollystack_ingestion_data_points_total[5m])

# Kafka consumer lag
ollystack_kafka_consumer_lag

# Alert evaluation latency
histogram_quantile(0.99, ollystack_rule_evaluation_latency_seconds_bucket)

# ClickHouse write latency
histogram_quantile(0.99, ollystack_clickhouse_write_latency_seconds_bucket)
```

### Grafana Dashboards

Access Grafana at `http://localhost:3001` (admin/admin)

Pre-configured datasources:
- ClickHouse (default)
- AlertManager

## Troubleshooting

### Kafka not starting
```bash
# Check Zookeeper
docker logs ollystack-zookeeper

# Check Kafka
docker logs ollystack-kafka
```

### High consumer lag
```bash
# Check consumer group status
docker exec ollystack-kafka kafka-consumer-groups \
  --bootstrap-server localhost:9092 \
  --describe --group ollystack-storage-consumer
```

### ClickHouse connection issues
```bash
# Test connection
docker exec ollystack-clickhouse clickhouse-client \
  --query "SELECT 1"
```

## Production Checklist

- [ ] Configure Kafka replication factor > 1
- [ ] Set up ClickHouse cluster (sharding)
- [ ] Configure AlertManager receivers (Slack, PagerDuty)
- [ ] Set up TLS for Kafka
- [ ] Configure backup for ClickHouse
- [ ] Set up monitoring alerts for components
- [ ] Review rate limits per tenant
- [ ] Configure log rotation for containers
