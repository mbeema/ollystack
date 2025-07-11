# OllyStack MVP Deployment Guide

Deploy OllyStack with managed ClickHouse Cloud for a simple, cost-effective observability stack.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Your Applications                                │
│                    (Instrumented with OpenTelemetry)                    │
└─────────────────────────────────┬───────────────────────────────────────┘
                                  │ OTLP (gRPC/HTTP)
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      OllyStack (Self-Hosted)                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐  │
│  │  OTel Collector │  │   API Server    │  │       Web UI            │  │
│  │   (Ingestion)   │  │      (Go)       │  │      (React)            │  │
│  │  Port: 4317/18  │  │   Port: 8080    │  │    Port: 3000           │  │
│  └────────┬────────┘  └────────┬────────┘  └─────────────────────────┘  │
│           │                    │                                         │
│           └──────────┬─────────┘                                         │
│                      │                                                   │
│  ┌─────────────────┐ │                                                   │
│  │     Redis       │◄┘ (Cache/Sessions)                                  │
│  └─────────────────┘                                                     │
└──────────────────────┼──────────────────────────────────────────────────┘
                       │ HTTPS (Port 9440)
                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     ClickHouse Cloud (Managed)                           │
│                                                                          │
│   Traces │ Logs │ Metrics │ Service Topology │ Aggregations             │
│                                                                          │
│   $50-200/month depending on usage                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

## Cost Estimate

| Component | Monthly Cost |
|-----------|--------------|
| ClickHouse Cloud (Basic tier) | $50-200 |
| VPS/Compute (4 vCPU, 8GB RAM) | $40-80 |
| **Total** | **$90-280/month** |

## Quick Start (Docker Compose)

### Prerequisites

- Docker & Docker Compose
- ClickHouse Cloud account ([sign up here](https://clickhouse.cloud))

### Step 1: Create ClickHouse Cloud Service

1. Go to [ClickHouse Cloud Console](https://console.clickhouse.cloud)
2. Create a new service (Basic tier is fine for MVP)
3. Note down:
   - Host (e.g., `xxx.us-east-1.aws.clickhouse.cloud`)
   - Username (usually `default`)
   - Password

### Step 2: Run Setup Script

```bash
cd deploy/scripts
./setup-mvp.sh
```

The script will:
1. Check prerequisites
2. Configure your ClickHouse Cloud credentials
3. Initialize the database schema
4. Start all services

### Step 3: Access OllyStack

- **Web UI**: http://localhost:3000
- **API**: http://localhost:8080
- **OTLP gRPC**: localhost:4317
- **OTLP HTTP**: localhost:4318

## Manual Setup

### 1. Configure Environment

```bash
cd deploy/docker
cp .env.example .env
```

Edit `.env` with your ClickHouse Cloud credentials:

```env
CLICKHOUSE_HOST=your-service.region.clickhouse.cloud
CLICKHOUSE_PORT=9440
CLICKHOUSE_DATABASE=ollystack
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=your-password
```

### 2. Initialize Database Schema

```bash
# Using clickhouse-client (if installed)
clickhouse-client \
  --host your-service.region.clickhouse.cloud \
  --port 9440 --secure \
  --user default \
  --password 'your-password' \
  < ../scripts/init-clickhouse.sql

# Or using Docker
docker run --rm -i clickhouse/clickhouse-client:latest \
  --host your-service.region.clickhouse.cloud \
  --port 9440 --secure \
  --user default \
  --password 'your-password' \
  < ../scripts/init-clickhouse.sql
```

### 3. Start Services

```bash
docker compose -f docker-compose.mvp.yml up -d
```

### 4. Verify

```bash
# Check service health
docker compose -f docker-compose.mvp.yml ps

# View logs
docker compose -f docker-compose.mvp.yml logs -f
```

## Kubernetes Deployment

### Prerequisites

- Kubernetes cluster (EKS, AKS, GKE, or local)
- kubectl configured
- kustomize (or kubectl with kustomize support)

### Step 1: Configure Secrets

Edit `deploy/kubernetes/base/secrets.yaml` with your ClickHouse credentials:

```yaml
stringData:
  host: "your-service.region.clickhouse.cloud"
  password: "your-password"
```

> **For production**: Use [Sealed Secrets](https://sealed-secrets.netlify.app/) or [External Secrets Operator](https://external-secrets.io/)

### Step 2: Update Ingress

Edit `deploy/kubernetes/base/ingress.yaml`:

```yaml
spec:
  rules:
    - host: ollystack.yourdomain.com  # Change this
```

### Step 3: Deploy

```bash
# Development
kubectl apply -k deploy/kubernetes/base

# Production (with increased replicas)
kubectl apply -k deploy/kubernetes/overlays/prod
```

### Step 4: Verify

```bash
kubectl -n ollystack get pods
kubectl -n ollystack get svc
kubectl -n ollystack get ingress
```

## Sending Telemetry Data

### Configure Your Application

Set these environment variables in your application:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317  # or your ingress URL
export OTEL_SERVICE_NAME=my-service
export OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production
```

### Example: Node.js

```javascript
// tracing.js
const { NodeSDK } = require('@opentelemetry/sdk-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-grpc');
const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter({
    url: process.env.OTEL_EXPORTER_OTLP_ENDPOINT || 'http://localhost:4317',
  }),
  instrumentations: [getNodeAutoInstrumentations()],
});

sdk.start();
```

### Example: Python

```python
# tracing.py
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

trace.set_tracer_provider(TracerProvider())
tracer_provider = trace.get_tracer_provider()

otlp_exporter = OTLPSpanExporter(
    endpoint="localhost:4317",
    insecure=True,
)
tracer_provider.add_span_processor(BatchSpanProcessor(otlp_exporter))
```

### Example: Go

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTracer() (*trace.TracerProvider, error) {
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("localhost:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

## Operations

### View Logs

```bash
# All services
docker compose -f docker-compose.mvp.yml logs -f

# Specific service
docker compose -f docker-compose.mvp.yml logs -f collector
docker compose -f docker-compose.mvp.yml logs -f api-server
```

### Stop Services

```bash
docker compose -f docker-compose.mvp.yml down
```

### Update Services

```bash
docker compose -f docker-compose.mvp.yml pull
docker compose -f docker-compose.mvp.yml up -d
```

### Backup (ClickHouse Cloud)

ClickHouse Cloud handles backups automatically. To export data:

```sql
-- Export traces to file
SELECT * FROM traces
WHERE start_time > now() - INTERVAL 1 DAY
INTO OUTFILE 'traces_backup.csv' FORMAT CSV;
```

## Troubleshooting

### Connection to ClickHouse Failed

1. Verify credentials in `.env`
2. Check if ClickHouse Cloud service is running
3. Ensure port 9440 is accessible
4. Check collector logs: `docker compose logs collector`

### No Data Appearing

1. Verify OTLP endpoint is reachable
2. Check if your application is sending traces
3. Look at collector debug output:
   ```bash
   docker compose exec collector wget -qO- http://localhost:13133/
   ```

### High Memory Usage

1. Reduce batch size in collector config
2. Increase sampling rate
3. Check for memory leaks in custom processors

## Next Steps

1. **Add Authentication**: Configure JWT auth for API
2. **Set Up Alerts**: Create alerting rules in ClickHouse
3. **Add More Instrumentation**: Instrument more services
4. **Scale Up**: Move to Kubernetes for production
5. **Add AI Analysis**: Integrate AI engine for anomaly detection

## Support

- GitHub Issues: [github.com/ollystack/ollystack](https://github.com/ollystack/ollystack)
- Documentation: [docs.ollystack.io](https://docs.ollystack.io)
