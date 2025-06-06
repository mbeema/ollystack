# OllyStack Deployment Guide

**Author:** Madhukar Beema, Distinguished Engineer

Complete guide for deploying OllyStack to AWS using EKS, Graviton instances, and Spot pricing for cost optimization.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Manual Deployment Steps](#manual-deployment-steps)
- [GitHub Actions CI/CD](#github-actions-cicd)
- [Cost Optimization](#cost-optimization)
- [Monitoring & Troubleshooting](#monitoring--troubleshooting)
- [Security](#security)
- [Scaling](#scaling)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AWS Cloud                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────────┐                                                           │
│   │   Route 53  │  (Optional DNS)                                           │
│   └──────┬──────┘                                                           │
│          │                                                                   │
│          ▼                                                                   │
│   ┌─────────────────────────────────────────────────────────────────┐       │
│   │                    Application Load Balancer                     │       │
│   │                    (Ingress Controller)                          │       │
│   └──────────────────────────────┬──────────────────────────────────┘       │
│                                  │                                           │
│   ┌──────────────────────────────┴──────────────────────────────────┐       │
│   │                         EKS Cluster                              │       │
│   │  ┌─────────────────────────────────────────────────────────────┐│       │
│   │  │                    Karpenter NodePool                        ││       │
│   │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         ││       │
│   │  │  │   Spot      │  │   Spot      │  │  On-Demand  │         ││       │
│   │  │  │  Graviton   │  │  Graviton   │  │  Graviton   │         ││       │
│   │  │  │  (t4g/m6g)  │  │  (t4g/m6g)  │  │  (r6g)      │         ││       │
│   │  │  │             │  │             │  │             │         ││       │
│   │  │  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │         ││       │
│   │  │  │ │Web UI   │ │  │ │API      │ │  │ │ClickHouse│ │         ││       │
│   │  │  │ │Collector│ │  │ │Server   │ │  │ │  (Data)  │ │         ││       │
│   │  │  │ └─────────┘ │  │ └─────────┘ │  │ └─────────┘ │         ││       │
│   │  │  └─────────────┘  └─────────────┘  └─────────────┘         ││       │
│   │  └─────────────────────────────────────────────────────────────┘│       │
│   └─────────────────────────────────────────────────────────────────┘       │
│                                  │                                           │
│                                  ▼                                           │
│   ┌─────────────────────────────────────────────────────────────────┐       │
│   │                              VPC                                 │       │
│   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │       │
│   │  │  Private    │  │  Private    │  │   Public    │             │       │
│   │  │  Subnet 1   │  │  Subnet 2   │  │   Subnet    │             │       │
│   │  │  (Workloads)│  │  (Workloads)│  │   (NAT/LB)  │             │       │
│   │  └─────────────┘  └─────────────┘  └─────────────┘             │       │
│   └─────────────────────────────────────────────────────────────────┘       │
│                                                                              │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                        │
│   │     ECR     │  │     S3      │  │  DynamoDB   │                        │
│   │  (Images)   │  │ (TF State)  │  │  (TF Lock)  │                        │
│   └─────────────┘  └─────────────┘  └─────────────┘                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Description | Instance Type |
|-----------|-------------|---------------|
| Web UI | React frontend served by nginx | Spot t4g.small |
| API Server | Go REST/gRPC API | Spot t4g.medium |
| OTel Collector | Telemetry ingestion | Spot t4g.medium |
| ClickHouse | Time-series database | On-Demand r6g.large |
| Redis | Caching layer | On-Demand t4g.small |

---

## Prerequisites

### Required Tools

```bash
# AWS CLI v2
brew install awscli

# Terraform >= 1.6
brew install terraform

# kubectl >= 1.28
brew install kubectl

# Docker
brew install --cask docker

# GitHub CLI (optional, for automation)
brew install gh
```

### AWS Account Requirements

- AWS Account with admin access (or sufficient IAM permissions)
- IAM user with programmatic access (access key + secret key)
- Permissions needed:
  - EKS full access
  - EC2 full access
  - ECR full access
  - S3 full access
  - DynamoDB full access
  - IAM (for creating roles)
  - VPC/Networking

---

## Quick Start

### One-Command Deployment

```bash
# 1. Clone the repository
git clone https://github.com/yourorg/ollystack.git
cd ollystack

# 2. Configure AWS credentials
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-2"

# 3. Bootstrap AWS resources (first time only)
make aws-bootstrap ENV=dev

# 4. Deploy everything
make aws-deploy-all ENV=dev
```

This will:
1. Create S3 bucket for Terraform state
2. Create DynamoDB table for state locking
3. Create ECR repositories
4. Deploy VPC, EKS cluster with Karpenter
5. Build and push Docker images
6. Deploy all Kubernetes resources
7. Run smoke tests

### Estimated Deployment Time

| Stage | Duration |
|-------|----------|
| Bootstrap | 2-3 minutes |
| Infrastructure | 15-20 minutes |
| Build & Push | 5-10 minutes |
| K8s Deployment | 3-5 minutes |
| **Total** | **25-40 minutes** |

---

## Manual Deployment Steps

### Step 1: Bootstrap AWS

```bash
# Set environment variables
export PROJECT_NAME=ollystack
export AWS_REGION=us-east-2
export ENVIRONMENT=dev

# Run bootstrap script
./scripts/bootstrap-aws.sh dev
```

This creates:
- S3 bucket: `ollystack-terraform-state-us-east-2`
- DynamoDB table: `ollystack-terraform-lock`
- ECR repositories for api-server, web-ui, collector

### Step 2: Deploy Infrastructure

```bash
cd deploy/aws/terraform

# Initialize Terraform
terraform init \
  -backend-config="bucket=ollystack-terraform-state-us-east-2" \
  -backend-config="key=ollystack/dev/terraform.tfstate" \
  -backend-config="region=us-east-2" \
  -backend-config="dynamodb_table=ollystack-terraform-lock"

# Plan changes
terraform plan -var-file="environments/dev.tfvars" -out=tfplan

# Apply changes
terraform apply tfplan
```

### Step 3: Configure kubectl

```bash
aws eks update-kubeconfig \
  --region us-east-2 \
  --name ollystack-dev \
  --alias ollystack-dev
```

### Step 4: Build and Push Images

```bash
# Login to ECR
aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin \
  $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com

# Build and push API server
docker build -t ollystack/api-server:latest -f api-server/Dockerfile api-server/
docker tag ollystack/api-server:latest \
  $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com/ollystack/api-server:latest
docker push $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com/ollystack/api-server:latest

# Build and push Web UI
docker build -t ollystack/web-ui:latest -f web-ui/Dockerfile web-ui/
docker tag ollystack/web-ui:latest \
  $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com/ollystack/web-ui:latest
docker push $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com/ollystack/web-ui:latest
```

### Step 5: Deploy Kubernetes Resources

```bash
# Create namespace
kubectl apply -f deploy/kubernetes/base/namespace.yaml

# Deploy secrets (update with your values first)
kubectl apply -f deploy/kubernetes/base/secrets.yaml

# Deploy all components
kubectl apply -k deploy/kubernetes/overlays/dev

# Wait for deployments
kubectl rollout status deployment/api-server -n ollystack
kubectl rollout status deployment/web-ui -n ollystack
kubectl rollout status deployment/otel-collector -n ollystack
```

### Step 6: Verify Deployment

```bash
# Check pods
kubectl get pods -n ollystack

# Check services
kubectl get svc -n ollystack

# Check ingress
kubectl get ingress -n ollystack

# Test API
kubectl port-forward svc/api-server 8080:8080 -n ollystack &
curl http://localhost:8080/health
```

---

## GitHub Actions CI/CD

### Setting Up Secrets

Go to your GitHub repository: **Settings > Secrets and variables > Actions**

Add these secrets:

| Secret | Description | Example |
|--------|-------------|---------|
| `AWS_ACCESS_KEY_ID` | AWS access key | `AKIA...` |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | `wJalrXUtn...` |
| `AWS_REGION` | AWS region | `us-east-2` |
| `AWS_ACCOUNT_ID` | AWS account ID | `123456789012` |
| `ECR_REGISTRY` | ECR registry URL | `123456789012.dkr.ecr.us-east-2.amazonaws.com` |
| `TF_STATE_BUCKET` | Terraform state bucket | `ollystack-terraform-state-us-east-2` |
| `TF_LOCK_TABLE` | Terraform lock table | `ollystack-terraform-lock` |

Optional secrets:
| Secret | Description |
|--------|-------------|
| `SLACK_WEBHOOK_URL` | Slack notifications |
| `CODECOV_TOKEN` | Code coverage reports |

### Automated Setup

```bash
# Interactive setup (requires gh CLI)
./scripts/setup-github.sh

# Or verify existing configuration
./scripts/setup-github.sh --verify
```

### Workflow Triggers

| Workflow | Trigger | Description |
|----------|---------|-------------|
| CI | Push/PR to main | Tests, linting, security |
| Build & Push | Push to main | Build images, push to ECR |
| Deploy Infra | Push to main (terraform/) | Terraform apply |
| Deploy App | After Build & Push | Deploy to Kubernetes |
| Release | Tag v* | Production release |

### Manual Deployment

```bash
# Trigger infrastructure deployment
gh workflow run deploy-infra.yml -f environment=dev -f action=apply

# Trigger application deployment
gh workflow run deploy-app.yml -f environment=prod -f image_tag=v1.0.0

# Create release (triggers production deployment)
git tag v1.0.0
git push origin v1.0.0
```

---

## Cost Optimization

### Estimated Monthly Costs

#### Development Environment (~$150-200/month)

| Resource | Spec | Monthly Cost |
|----------|------|--------------|
| EKS Cluster | Control plane | $72 |
| Spot Nodes | 2x t4g.medium | ~$15 |
| On-Demand Node | 1x r6g.large (ClickHouse) | ~$50 |
| NAT Gateway | Single AZ | $32 |
| EBS Storage | 100GB gp3 | ~$8 |
| ECR Storage | ~5GB | ~$1 |
| Data Transfer | ~50GB | ~$5 |

#### Production Environment (~$400-600/month)

| Resource | Spec | Monthly Cost |
|----------|------|--------------|
| EKS Cluster | Control plane | $72 |
| Spot Nodes | 4x t4g.large | ~$60 |
| On-Demand Nodes | 2x r6g.xlarge (ClickHouse) | ~$200 |
| NAT Gateway | Multi-AZ | $64 |
| EBS Storage | 500GB gp3 | ~$40 |
| ALB | Application Load Balancer | ~$20 |
| Data Transfer | ~200GB | ~$20 |

### Cost Optimization Tips

1. **Use Spot Instances**: 60-90% savings on stateless workloads
2. **Use Graviton (ARM64)**: 20-40% savings vs x86
3. **Single NAT Gateway in Dev**: $32/month savings
4. **Karpenter Auto-scaling**: Right-size nodes automatically
5. **S3 Tiering**: Move old data to S3 Glacier

---

## Monitoring & Troubleshooting

### Useful Commands

```bash
# View all resources
kubectl get all -n ollystack

# Pod logs
kubectl logs -f deployment/api-server -n ollystack
kubectl logs -f deployment/otel-collector -n ollystack

# Describe pod issues
kubectl describe pod <pod-name> -n ollystack

# Check node status
kubectl get nodes -o wide

# Karpenter provisioner status
kubectl get nodepools
kubectl get nodeclaims

# Port forwarding for local access
kubectl port-forward svc/api-server 8080:8080 -n ollystack
kubectl port-forward svc/web-ui 3000:80 -n ollystack
kubectl port-forward svc/clickhouse 8123:8123 -n ollystack
```

### Common Issues

#### Pods Stuck in Pending

```bash
# Check node capacity
kubectl describe nodes | grep -A 5 "Allocated resources"

# Check Karpenter logs
kubectl logs -f deployment/karpenter -n karpenter

# Manually trigger scale-up
kubectl scale deployment api-server --replicas=5 -n ollystack
```

#### Image Pull Errors

```bash
# Verify ECR login
aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin \
  $(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com

# Check image exists
aws ecr describe-images --repository-name ollystack/api-server --region us-east-2

# Update kubectl secret
kubectl delete secret ecr-registry-secret -n ollystack
kubectl create secret docker-registry ecr-registry-secret \
  --docker-server=$(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-2.amazonaws.com \
  --docker-username=AWS \
  --docker-password=$(aws ecr get-login-password --region us-east-2) \
  -n ollystack
```

#### Terraform State Lock

```bash
# Force unlock (use with caution)
terraform force-unlock <LOCK_ID>

# Or delete from DynamoDB
aws dynamodb delete-item \
  --table-name ollystack-terraform-lock \
  --key '{"LockID": {"S": "ollystack/dev/terraform.tfstate"}}'
```

---

## Security

### Network Security

- All workloads run in private subnets
- Public access only through ALB
- Security groups restrict inter-service communication
- VPC endpoints for AWS services (reduce NAT costs)

### Secrets Management

```bash
# Create Kubernetes secret
kubectl create secret generic ollystack-secrets \
  --from-literal=clickhouse-password='your-password' \
  --from-literal=redis-password='your-password' \
  -n ollystack

# Reference in deployment
env:
  - name: CLICKHOUSE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: ollystack-secrets
        key: clickhouse-password
```

### IAM Best Practices

- Use IRSA (IAM Roles for Service Accounts) for pod permissions
- Principle of least privilege
- Rotate credentials regularly

---

## Scaling

### Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-server-hpa
  namespace: ollystack
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-server
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

### Karpenter Auto-scaling

Karpenter automatically provisions optimal nodes based on pending pods:

```yaml
# NodePool configuration (deploy/aws/terraform/karpenter.tf)
spec:
  requirements:
    - key: kubernetes.io/arch
      operator: In
      values: ["arm64"]  # Prefer Graviton
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["spot"]   # Prefer Spot
  limits:
    cpu: 100              # Max 100 vCPU across all nodes
```

### ClickHouse Scaling

For horizontal scaling, consider:
1. ClickHouse Keeper cluster (3 nodes minimum)
2. Sharded cluster with replication
3. Use ClickHouse Operator for management

---

## Environment Management

### Development

```bash
make aws-deploy-all ENV=dev
```

- Minimal resources
- Single ClickHouse node
- Spot instances only
- No production protection

### Production

```bash
# Requires GitHub environment approval
gh workflow run deploy-app.yml -f environment=prod -f image_tag=v1.0.0
```

- High availability (multi-AZ)
- ClickHouse with replication
- Mix of Spot and On-Demand
- Required reviewers for deployment

### Cleanup

```bash
# Destroy all resources (DANGEROUS!)
make aws-destroy ENV=dev

# Or manually
cd deploy/aws/terraform
terraform destroy -var-file="environments/dev.tfvars"
```

---

## Support

- GitHub Issues: https://github.com/yourorg/ollystack/issues
- Documentation: https://ollystack.dev/docs
- Slack: #ollystack-support
