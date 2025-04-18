# OllyStack - Low-Cost AWS Architecture

## Cost Optimization Strategy

| Strategy | Savings |
|----------|---------|
| **Graviton (ARM) instances** | 20-40% vs x86 |
| **Spot instances** (stateless) | Up to 90% vs On-Demand |
| **Self-hosted ClickHouse** | 50-70% vs managed |
| **S3 cold storage** | 90% vs SSD |
| **Reserved/Savings Plans** | 30-60% |
| **Karpenter autoscaling** | Right-sizing |

## Architecture Overview

```
                                    ┌──────────────────────────────────────┐
                                    │           Route 53                    │
                                    │      ollystack.yourdomain.com          │
                                    └───────────────┬──────────────────────┘
                                                    │
                                    ┌───────────────▼──────────────────────┐
                                    │     Application Load Balancer         │
                                    │         (pay per LCU ~$20/mo)         │
                                    └───────────────┬──────────────────────┘
                                                    │
┌───────────────────────────────────────────────────┴───────────────────────────────────────┐
│                                    VPC (10.0.0.0/16)                                       │
│                                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                              EKS Cluster (Auto Mode)                                 │  │
│  │                                  ~$73/month                                          │  │
│  │                                                                                      │  │
│  │  ┌─────────────────────────────────────────────────────────────────────────────┐   │  │
│  │  │                    Karpenter Managed Node Pools                              │   │  │
│  │  │                                                                              │   │  │
│  │  │   ┌─────────────────────┐    ┌─────────────────────┐                        │   │  │
│  │  │   │ SPOT + Graviton     │    │ On-Demand Graviton  │                        │   │  │
│  │  │   │ (Stateless Apps)    │    │ (ClickHouse/Redis)  │                        │   │  │
│  │  │   │                     │    │                     │                        │   │  │
│  │  │   │ • Web UI            │    │ • ClickHouse        │                        │   │  │
│  │  │   │ • API Server        │    │   (3 replicas)      │                        │   │  │
│  │  │   │ • OTel Collector    │    │ • Redis             │                        │   │  │
│  │  │   │                     │    │                     │                        │   │  │
│  │  │   │ t4g.medium x 2-5    │    │ r7g.xlarge x 3      │                        │   │  │
│  │  │   │ ~$0.008/hr SPOT     │    │ ~$0.20/hr           │                        │   │  │
│  │  │   └─────────────────────┘    └─────────────────────┘                        │   │  │
│  │  │                                                                              │   │  │
│  │  └──────────────────────────────────────────────────────────────────────────────┘   │  │
│  │                                                                                      │  │
│  └──────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                            │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                                  Storage Layer                                      │   │
│  │                                                                                     │   │
│  │   ┌──────────────────────┐    ┌──────────────────────┐    ┌───────────────────┐   │   │
│  │   │   EBS gp3 (Hot)      │    │    S3 (Warm/Cold)    │    │  S3 Glacier       │   │   │
│  │   │   500GB x 3 nodes    │    │    Tiered Storage    │    │  (Archive)        │   │   │
│  │   │   $0.08/GB = $120    │    │    $0.023/GB         │    │  $0.004/GB        │   │   │
│  │   └──────────────────────┘    └──────────────────────┘    └───────────────────┘   │   │
│  │                                                                                     │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                            │
└────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Component Breakdown & Costs

### Tier 1: Minimal (~$150-200/month)

For small teams, <50k spans/day, <10GB logs/day

| Component | Instance | Quantity | Cost/Month |
|-----------|----------|----------|------------|
| EKS Control Plane | - | 1 | $73 |
| App Nodes (Spot) | t4g.small | 2 | ~$7 |
| ClickHouse | t4g.xlarge | 1 | ~$50 |
| EBS Storage | gp3 200GB | 1 | ~$16 |
| ALB | - | 1 | ~$20 |
| S3 | 100GB | - | ~$3 |
| Data Transfer | 50GB | - | ~$5 |
| **TOTAL** | | | **~$175** |

### Tier 2: Small Production (~$400-600/month)

For startups, <500k spans/day, <50GB logs/day

| Component | Instance | Quantity | Cost/Month |
|-----------|----------|----------|------------|
| EKS Control Plane | - | 1 | $73 |
| App Nodes (Spot) | t4g.medium | 3 | ~$25 |
| ClickHouse | r7g.large | 3 | ~$200 |
| EBS Storage | gp3 500GB | 3 | ~$120 |
| ALB | - | 1 | ~$25 |
| S3 | 500GB | - | ~$12 |
| Data Transfer | 200GB | - | ~$18 |
| NAT Gateway | - | 1 | ~$45 |
| **TOTAL** | | | **~$520** |

### Tier 3: Medium Production (~$1,000-1,500/month)

For growth stage, <5M spans/day, <200GB logs/day

| Component | Instance | Quantity | Cost/Month |
|-----------|----------|----------|------------|
| EKS Control Plane | - | 1 | $73 |
| App Nodes (Spot) | m7g.large | 5 | ~$100 |
| ClickHouse | r7g.xlarge | 3 | ~$450 |
| EBS Storage | gp3 1TB | 3 | ~$240 |
| ALB | - | 1 | ~$35 |
| S3 | 2TB | - | ~$46 |
| Data Transfer | 500GB | - | ~$45 |
| NAT Gateway | - | 2 | ~$90 |
| **TOTAL** | | | **~$1,080** |

## Instance Selection Guide

### Graviton (ARM) Instances - Use These!

| Instance | vCPU | RAM | Spot Price* | Use Case |
|----------|------|-----|-------------|----------|
| t4g.small | 2 | 2GB | $0.004/hr | Dev, small workloads |
| t4g.medium | 2 | 4GB | $0.008/hr | API servers, collectors |
| t4g.large | 2 | 8GB | $0.016/hr | Web UI, larger APIs |
| m7g.medium | 1 | 4GB | $0.020/hr | General compute |
| m7g.large | 2 | 8GB | $0.040/hr | General compute |
| r7g.large | 2 | 16GB | $0.050/hr | ClickHouse (memory) |
| r7g.xlarge | 4 | 32GB | $0.100/hr | ClickHouse production |
| r7g.2xlarge | 8 | 64GB | $0.200/hr | Large ClickHouse |

*Spot prices are approximate and vary by region/AZ

### Region Selection (Cheapest to Most Expensive)

1. **us-east-2** (Ohio) - Often cheapest
2. **us-east-1** (N. Virginia) - Most spot capacity
3. **us-west-2** (Oregon) - Good balance
4. **eu-west-1** (Ireland) - Cheapest EU

## Key Cost Optimizations

### 1. Use Spot for Stateless Workloads

```yaml
# Karpenter NodePool for Spot instances
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: spot-arm64
spec:
  template:
    spec:
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["arm64"]
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot"]
        - key: node.kubernetes.io/instance-type
          operator: In
          values: ["t4g.medium", "t4g.large", "m7g.medium", "m7g.large"]
      nodeClassRef:
        name: default
  limits:
    cpu: 100
    memory: 200Gi
  disruption:
    consolidationPolicy: WhenEmpty
    consolidateAfter: 30s
```

### 2. Self-Host ClickHouse on Graviton

**vs ClickHouse Cloud:**
- ClickHouse Cloud: ~$200-500/month for similar capacity
- Self-hosted on r7g.large x3: ~$150-200/month
- **Savings: 50-60%**

### 3. S3 Lifecycle for Cold Data

```json
{
  "Rules": [
    {
      "ID": "MoveToIA",
      "Status": "Enabled",
      "Transitions": [
        {"Days": 30, "StorageClass": "STANDARD_IA"},
        {"Days": 90, "StorageClass": "GLACIER_IR"}
      ]
    }
  ]
}
```

### 4. Reserved Instances / Savings Plans

For ClickHouse nodes (always running):
- **1-year Reserved**: 30-40% savings
- **3-year Reserved**: 50-60% savings
- **Compute Savings Plan**: 20-30% flexible savings

### 5. Minimize NAT Gateway Costs

NAT Gateway is expensive ($0.045/hr + $0.045/GB). Options:
- Use **VPC Endpoints** for S3, ECR ($10/month each)
- Use **NAT Instance** on t4g.nano (~$3/month)
- Pull images through ECR in same region

## vs ClickHouse Cloud Comparison

| Metric | Self-Hosted (AWS) | ClickHouse Cloud |
|--------|-------------------|------------------|
| **Small Setup** | ~$200/mo | ~$100-200/mo |
| **Medium Setup** | ~$500/mo | ~$500-1000/mo |
| **Large Setup** | ~$1,000/mo | ~$2,000-4,000/mo |
| **Engineering Time** | 5-10 hrs/mo | 0 hrs/mo |
| **Control** | Full | Limited |
| **Scaling** | Manual/Karpenter | Automatic |

**Recommendation:**
- **< $300/mo budget**: Use ClickHouse Cloud Basic
- **> $300/mo budget**: Self-host for better economics
- **> $1000/mo budget**: Definitely self-host

## Network Architecture

```
                    Internet
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Public Subnets                                │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│   │ AZ-a        │  │ AZ-b        │  │ AZ-c        │             │
│   │ 10.0.0.0/24 │  │ 10.0.1.0/24 │  │ 10.0.2.0/24 │             │
│   │             │  │             │  │             │             │
│   │ NAT GW      │  │ (failover)  │  │             │             │
│   │ ALB         │  │ ALB         │  │             │             │
│   └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Private Subnets                                │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│   │ AZ-a        │  │ AZ-b        │  │ AZ-c        │             │
│   │ 10.0.10.0/24│  │ 10.0.11.0/24│  │ 10.0.12.0/24│             │
│   │             │  │             │  │             │             │
│   │ EKS Nodes   │  │ EKS Nodes   │  │ EKS Nodes   │             │
│   │ ClickHouse  │  │ ClickHouse  │  │ ClickHouse  │             │
│   └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                   VPC Endpoints                                  │
│                                                                  │
│   S3 Gateway (Free)    ECR ($10/mo)    CloudWatch ($10/mo)      │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Cost Calculator

```
Monthly Cost =
  EKS Control Plane ($73)
  + App Compute (spot_instances × spot_price × 730)
  + ClickHouse Compute (instances × price × 730)
  + EBS Storage (GB × $0.08)
  + S3 Storage (GB × $0.023)
  + ALB (~$20-40)
  + NAT Gateway ($32 + data × $0.045)
  + Data Transfer Out (GB × $0.09)
```

## Sources

- [AWS EKS Auto Mode with Graviton and Spot](https://aws.amazon.com/blogs/containers/maximize-amazon-eks-efficiency-how-auto-mode-graviton-and-spot-work-together/)
- [Slash EKS Costs by 20-30% with Graviton](https://rafay.co/ai-and-cloud-native-blog/slash-eks-cluster-costs-by-20-30-instantly-with-aws-graviton)
- [Self-hosted ClickHouse Cost Analysis](https://www.tinybird.co/blog/self-hosted-clickhouse-cost)
- [ClickHouse Cloud Pricing](https://clickhouse.com/pricing)
