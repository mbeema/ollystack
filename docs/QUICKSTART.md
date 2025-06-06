# OllyStack Quick Start Guide

**Author:** Madhukar Beema, Distinguished Engineer

Get OllyStack running in 10 minutes.

## Option 1: Local Development (Docker Compose)

```bash
# Clone and run
git clone https://github.com/yourorg/ollystack.git
cd ollystack
make dev-up

# Access
open http://localhost:3000   # Web UI
curl http://localhost:8080/health  # API
```

## Option 2: AWS Deployment

### Prerequisites

1. AWS CLI configured with credentials
2. Docker installed
3. Terraform installed

### Deploy

```bash
# 1. Set credentials
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
export AWS_REGION="us-east-2"

# 2. Bootstrap (first time only)
make aws-bootstrap ENV=dev

# 3. Deploy
make aws-deploy-all ENV=dev

# 4. Get access URL
kubectl get ingress -n ollystack
```

## Option 3: GitHub Actions (Recommended)

### Setup (5 minutes)

1. Fork the repository
2. Go to **Settings > Secrets > Actions**
3. Add secrets:
   - `AWS_ACCESS_KEY_ID`
   - `AWS_SECRET_ACCESS_KEY`
   - `AWS_REGION` (e.g., `us-east-2`)
   - `AWS_ACCOUNT_ID`
   - `ECR_REGISTRY` (e.g., `123456789.dkr.ecr.us-east-2.amazonaws.com`)
   - `TF_STATE_BUCKET` (e.g., `ollystack-terraform-state-us-east-2`)
   - `TF_LOCK_TABLE` (e.g., `ollystack-terraform-lock`)

4. Run bootstrap manually:
   ```bash
   ./scripts/bootstrap-aws.sh dev
   ```

5. Push to main branch - deployment starts automatically!

### Or Use Interactive Setup

```bash
# Requires GitHub CLI
./scripts/setup-github.sh
```

## Verify Deployment

```bash
# Check pods
kubectl get pods -n ollystack

# View logs
kubectl logs -f deployment/api-server -n ollystack

# Port forward for local access
kubectl port-forward svc/web-ui 3000:80 -n ollystack

# Open browser
open http://localhost:3000
```

## Send Test Data

```bash
# Send trace data via OTLP
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "test-service"}}]},
      "scopeSpans": [{
        "spans": [{
          "traceId": "5B8EFFF798038103D269B633813FC60C",
          "spanId": "EEE19B7EC3C1B174",
          "name": "test-span",
          "startTimeUnixNano": "1544712660000000000",
          "endTimeUnixNano": "1544712661000000000"
        }]
      }]
    }]
  }'
```

## Cleanup

```bash
# Destroy all AWS resources
make aws-destroy ENV=dev
```

## Cost Estimate

| Environment | Monthly Cost |
|-------------|-------------|
| Dev | ~$150-200 |
| Prod | ~$400-600 |

*Using Graviton + Spot instances for maximum savings*

## Next Steps

1. [Full Deployment Guide](./DEPLOYMENT.md)
2. [Architecture Overview](./ARCHITECTURE.md)
3. [CI/CD Setup](./../.github/CICD-SETUP.md)

## Troubleshooting

### Pods not starting?
```bash
kubectl describe pod <pod-name> -n ollystack
kubectl logs <pod-name> -n ollystack
```

### Can't connect to cluster?
```bash
aws eks update-kubeconfig --region us-east-2 --name ollystack-dev
```

### Image pull errors?
```bash
# Re-authenticate ECR
aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin \
  $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com
```
