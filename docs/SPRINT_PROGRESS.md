# OllyStack Sprint Progress Tracker

**Current Sprint:** Sprint 5 - Advanced AI & Custom OTel (COMPLETE)
**Started:** 2026-02-03
**Last Updated:** 2026-02-03

---

## RESUME INSTRUCTIONS FOR CLAUDE

If session crashes, read this file first to understand where to resume.

### Current Status
- **Working On:** Sprint 6 planning
- **Last Completed Task:** Sprint 5 complete - Causal analysis, predictive alerts, trends API
- **Next Task:** Choose Sprint 6 focus (Kubernetes, Multi-tenancy, Alert Rules, or Collector Build)

### Infrastructure Status (as of 2026-02-03)
- **Instance Type:** t3.medium (on-demand, was spot)
- **Swap:** 2GB configured
- **API Config:** `/opt/ollystack/config/api/config.yaml`
- **All Services:** Running and healthy

### Files Created This Session (Sprint 3)
- [x] `sample-services/docker-compose.yaml` - Full stack deployment
- [x] `sample-services/services/api-gateway/` - API Gateway service
- [x] `sample-services/services/order-service/` - Order service
- [x] `sample-services/services/payment-service/` - Payment service
- [x] `sample-services/services/inventory-service/` - Inventory service
- [x] `sample-services/services/notification-service/` - Notification service
- [x] `sample-services/services/traffic-generator/` - Automatic load generator
- [x] `sample-services/services/ai-engine/` - Python AI/ML service
- [x] `sample-services/configs/` - OTel, PostgreSQL, Grafana configs

### Remote Server Info
- **Host:** ec2-user@18.209.179.35
- **SSH Key:** ~/.ssh/mb2025.pem
- **OllyStack Dir:** /home/ec2-user/ollystack
- **Sample Services:** /home/ec2-user/ollystack/sample-services

### Service Ports
| Service | Port | Description |
|---------|------|-------------|
| API Gateway | 8081 | Entry point for all requests |
| Order Service | 8082 | Order processing |
| Payment Service | 8083 | Payment processing |
| Inventory Service | 8084 | Stock management |
| Notification Service | 8085 | Email/SMS/Push notifications |
| AI Engine | 8090 | ML-powered RCA & anomaly detection |
| OllyStack API | 8080 | Correlation API |
| Grafana | 3000 | Dashboard (admin/ollystack) |

### ClickHouse Connection
- **Host:** ollystack-clickhouse:9000 (native), 8123 (HTTP)
- **Database:** ollystack
- **Username:** ollystack
- **Password:** ollystack123

---

## Sprint 1 Progress - COMPLETE

- ClickHouse schema with correlation_id
- OTel Correlation Processor
- Custom collector build configuration
- Grafana datasource and dashboard configs

---

## Sprint 2 Progress - COMPLETE

- Correlation API endpoints
- Deployment scripts
- Go services on remote

---

## Sprint 3 Progress - COMPLETE

### 3.1 Sample Microservices

| Task | Status | Notes |
|------|--------|-------|
| Create API Gateway | [x] DONE | Go, OTel instrumented |
| Create Order Service | [x] DONE | PostgreSQL integration |
| Create Payment Service | [x] DONE | Simulates 5% failure rate |
| Create Inventory Service | [x] DONE | Redis caching, 3% failure rate |
| Create Notification Service | [x] DONE | Email/SMS/Push simulation |
| Create Traffic Generator | [x] DONE | 2 RPS continuous load |

### 3.2 AI/ML Engine

| Task | Status | Notes |
|------|--------|-------|
| FastAPI service | [x] DONE | Python with OTEL |
| Service health endpoint | [x] DONE | Real-time health metrics |
| Insights endpoint | [x] DONE | Automated recommendations |
| Anomaly detection | [x] DONE | Z-score based detection |
| RCA endpoint | [x] DONE | Root cause analysis |

### 3.3 AI Engine Endpoints

```
GET  /api/v1/services/health           - Service health status
GET  /api/v1/insights                  - AI-generated insights
POST /api/v1/analyze                   - Analyze correlation ID
POST /api/v1/rca                       - Root cause analysis
POST /api/v1/anomalies/detect          - Anomaly detection
```

### 3.4 Docker Compose Services

```yaml
# Infrastructure (using existing ollystack network):
- postgres:5432      # Orders database

# Sample Microservices:
- api-gateway:8080
- order-service:8080
- payment-service:8080
- inventory-service:8080
- notification-service:8080
- traffic-generator (no port, generates load)

# AI/ML:
- ai-engine:8080
```

---

## Quick Commands

```bash
# SSH to remote
ssh -i ~/.ssh/mb2025.pem ec2-user@18.209.179.35

# View all containers
docker ps --format 'table {{.Names}}\t{{.Status}}'

# Manage sample services
cd /home/ec2-user/ollystack/sample-services
docker-compose logs -f                    # View all logs
docker-compose logs -f traffic-generator  # View traffic gen logs
docker-compose restart ai-engine          # Restart AI engine

# Test AI Engine
curl http://localhost:8090/api/v1/services/health
curl 'http://localhost:8090/api/v1/insights?hours=1'
curl -X POST http://localhost:8090/api/v1/anomalies/detect \
  -H 'Content-Type: application/json' \
  -d '{"metric": "latency", "window_minutes": 30}'

# Check traces in ClickHouse
docker exec ollystack-clickhouse clickhouse-client \
  --user ollystack --password ollystack123 \
  --query "SELECT ServiceName, count() FROM ollystack.traces \
           WHERE Timestamp > now() - INTERVAL 5 MINUTE \
           GROUP BY ServiceName ORDER BY count() DESC"
```

---

## Session History

### Session 1 (2026-02-03)
- Created SPRINT_PROGRESS.md tracking document
- Applied fresh ClickHouse schema with correlation_id support
- Created OTel Correlation Processor
- Created custom collector build configuration
- Created Grafana datasource and dashboard configurations
- **Sprint 1 COMPLETE**

### Session 2 (2026-02-03)
- Created deployment scripts (deploy.sh, test-local.sh)
- Added correlation storage queries to ClickHouse client
- Created CorrelationService in services layer
- Created correlation API handlers
- Added correlation routes to main.go
- Installed Go 1.21.6 on remote server
- Built and deployed API server
- Fixed OTel processor API compatibility
- All OTel processor tests passing (7/7)
- Correlation API deployed and working
- **Sprint 2 COMPLETE**

### Session 3 (2026-02-03)
- Created sample microservices (api-gateway, order, payment, inventory, notification)
- Created traffic generator for continuous load
- Created AI/ML Engine Python service with FastAPI
  - Service health monitoring
  - AI-powered insights generation
  - Anomaly detection (Z-score based)
  - Root cause analysis
- Deployed all services to remote using docker-compose
- Integrated with existing ClickHouse and OTel collector
- Verified real trace data flowing through system
- AI Engine providing real-time insights and anomaly detection
- **Sprint 3 COMPLETE**

### Session 4 (2026-02-03) - Infrastructure Hardening
- Spot instance terminated due to capacity
- Switched to on-demand instance (t3.medium in us-east-1c)
- Fixed API server Dockerfile (removed incorrect `CMD ["serve"]`)
- Fixed API config issues (ClickHouse user: ollystack, password: ollystack123)
- Added 2GB swap to running instance and Terraform user-data
- Created persistent API config at `/opt/ollystack/config/api/config.yaml`
- Updated Terraform `user-data.sh` with:
  - Swap file creation on boot
  - Pre-configured API config template
- All services verified running and healthy:
  - ollystack-api (port 8080)
  - ollystack-web (port 3000)
  - ollystack-collector (ports 4317, 4318)
  - ollystack-clickhouse (ports 8123, 9000)
  - ollystack-redis (port 6379)
- **Infrastructure Hardening COMPLETE**

### Session 5 (2026-02-03) - Sprint 4: UI & Grafana
- Session crashed while updating docs, recovered successfully
- Fixed API server health check (config path, wget compatibility)
- Deployed AI Engine with curl for health checks
- Fixed Web UI AI_ENGINE_URL configuration
- Deployed all sample microservices:
  - api-gateway, order-service, payment-service
  - inventory-service, notification-service
  - traffic-generator (2 RPS)
- Fixed Go Dockerfiles (go mod tidy)
- Fixed PostgreSQL init.sql permissions
- Deployed Grafana with ClickHouse datasource (port 3001)
- Created ServiceFlowDiagram component for correlation visualization
- Added Flow tab to CorrelationsPage
- Created traces-overview.json Grafana dashboard
- Created NLQueryWidget for natural language queries on dashboard
- Added NLQ endpoint to AI Engine for SQL generation
- Deployed all updates to remote server
- All 14 services running and healthy
- 41,500+ traces in ClickHouse
- **Sprint 4 COMPLETE**

### Session 6 (2026-02-03) - Sprint 5: Advanced AI & OTel
- Added Causal Analysis endpoint (`/api/v1/causal/analyze`)
  - Traces cause-effect chains through services
  - Identifies root causes and propagation
  - Provides remediation recommendations
- Added Predictive Alerts endpoint (`/api/v1/alerts/predict`)
  - Time series forecasting for latency, error rate, throughput
  - Predicts threshold breaches before they occur
  - Configurable sensitivity and forecast window
- Added Trends API endpoint (`/api/v1/trends`)
  - Historical trend analysis by service
  - Latency/error rate change detection
- Created build-remote.sh script for remote deployments
- Updated Custom OTel Collector configuration:
  - Correlation processor code complete
  - Builder config for ocb (needs version alignment)
- Deployed AI Engine updates to remote
- All 14 services running and healthy
- 90,000+ traces in ClickHouse
- **Sprint 5 COMPLETE**

---

## Current Architecture

```
                    ┌──────────────────────────────────────────┐
                    │              Traffic Generator           │
                    │            (2 requests/second)           │
                    └─────────────────┬────────────────────────┘
                                      │
                    ┌─────────────────▼────────────────────────┐
                    │              API Gateway                 │
                    │           (correlation_id gen)           │
                    └──┬─────────────────────────────────────┬─┘
                       │                                     │
         ┌─────────────▼──────────────┐     ┌───────────────▼───────────┐
         │        Order Service       │     │    Inventory Service      │
         │       (PostgreSQL)         │     │      (Redis cache)        │
         └──┬───────────────────────┬─┘     └───────────────────────────┘
            │                       │
  ┌─────────▼───────┐   ┌──────────▼──────────┐
  │ Payment Service │   │ Notification Service │
  │  (5% failures)  │   │   (email/sms/push)   │
  └─────────────────┘   └─────────────────────┘
            │
            │  OTLP (traces, logs, metrics)
            ▼
    ┌───────────────────┐      ┌────────────────┐
    │   OTel Collector  │─────▶│   ClickHouse   │
    │  (correlation)    │      │    (storage)   │
    └───────────────────┘      └───────┬────────┘
                                       │
                               ┌───────▼────────┐
                               │   AI Engine    │
                               │ (RCA, Anomaly) │
                               └────────────────┘
```

---

---

## Sprint 4 Progress - COMPLETE

### 4.1 Web UI Enhancements

| Task | Status | Notes |
|------|--------|-------|
| Correlation Explorer page | [x] DONE | Full-featured with search, filters, timeline/traces/logs tabs |
| AI Insights widget | [x] DONE | Service health, insights, anomalies tabs |
| Dashboard with stats | [x] DONE | Total traces, services, error rate, avg latency |
| Service Map page | [x] DONE | Interactive service dependency graph |
| Service Flow visualization | [x] DONE | Added to Correlations page with Flow tab |
| NLQ Widget | [x] DONE | Natural language query in Dashboard |

### 4.2 Grafana Integration

| Task | Status | Notes |
|------|--------|-------|
| Deploy Grafana | [x] DONE | Port 3001, admin/ollystack |
| ClickHouse datasource | [x] DONE | Auto-provisioned |
| Dashboard provisioning | [x] DONE | JSON dashboards mounted |
| Traces Overview dashboard | [x] DONE | Traces stats, latency, service summary |

### 4.3 AI Engine Enhancements

| Task | Status | Notes |
|------|--------|-------|
| Natural Language Query | [x] DONE | SQL generation from questions |
| Query execution | [x] DONE | Direct ClickHouse integration |

### 4.4 Service Ports Summary

| Service | Port | URL |
|---------|------|-----|
| Web UI | 3000 | http://18.209.179.35:3000 |
| Grafana | 3001 | http://18.209.179.35:3001 |
| API Server | 8080 | http://18.209.179.35:8080 |
| AI Engine | 8090 | http://18.209.179.35:8090 |
| Sample API Gateway | 8081 | http://18.209.179.35:8081 |
| ClickHouse HTTP | 8123 | http://18.209.179.35:8123 |
| OTel Collector gRPC | 4317 | - |
| OTel Collector HTTP | 4318 | - |

---

## Sprint 5 Progress - COMPLETE

### 5.1 Advanced AI Features

| Task | Status | Notes |
|------|--------|-------|
| Causal Analysis | [x] DONE | `/api/v1/causal/analyze` - traces cause-effect chains |
| Predictive Alerts | [x] DONE | `/api/v1/alerts/predict` - forecasts issues |
| Trends API | [x] DONE | `/api/v1/trends` - time series analysis |

### 5.2 Custom OTel Collector

| Task | Status | Notes |
|------|--------|-------|
| Correlation Processor | [x] DONE | Full Go implementation |
| Builder Config | [x] DONE | otel-collector-custom/builder-config.yaml |
| Dockerfile | [x] DONE | Multi-stage build with ocb |
| Build | [~] Partial | Dependency version issues - needs ocb version sync |

**Note:** Custom collector build requires matching OTel Collector Builder version with component versions. The processor code is complete and tested. Deploy with standard otelcol-contrib for now.

### 5.3 AI Engine Endpoints

```
GET  /api/v1/services/health    - Service health status
GET  /api/v1/insights           - AI-generated insights
GET  /api/v1/trends             - Time series trend analysis
POST /api/v1/analyze            - Analyze correlation ID
POST /api/v1/rca                - Root cause analysis
POST /api/v1/rca/enhanced       - LLM-enhanced RCA
POST /api/v1/anomalies/detect   - Anomaly detection
POST /api/v1/causal/analyze     - Causal chain analysis
POST /api/v1/alerts/predict     - Predictive alerts
POST /api/v1/nlq                - Natural language query
POST /api/v1/chat               - AI chat assistant
```

---

## Next Steps (Sprint 6)

Options:
1. **Kubernetes Deployment** - Helm charts, Karpenter integration
2. **Multi-tenancy/RBAC** - Organization support
3. **Custom Collector Production Build** - Fix ocb version alignment
4. **Alert Rules Engine** - Configurable alert thresholds
