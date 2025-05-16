# OllyStack CI/CD Setup Guide

This guide explains how to set up the GitHub Actions CI/CD pipeline for OllyStack.

## Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CI/CD Pipeline                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Push to Branch          Push to Main           Push Tag (v*)           │
│       │                       │                       │                  │
│       ▼                       ▼                       ▼                  │
│  ┌─────────┐            ┌─────────┐            ┌─────────┐              │
│  │   CI    │            │   CI    │            │ Release │              │
│  │  Test   │            │  Test   │            │  Build  │              │
│  │  Lint   │            │  Build  │            │  Deploy │              │
│  └─────────┘            │  Push   │            │  Prod   │              │
│                         └────┬────┘            └─────────┘              │
│                              │                                           │
│                              ▼                                           │
│                         ┌─────────┐                                      │
│                         │ Deploy  │                                      │
│                         │   Dev   │                                      │
│                         └─────────┘                                      │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Workflows

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci.yml` | Push/PR | Run tests, lint, security scans |
| `build-push.yml` | Push to main | Build & push Docker images to ECR |
| `deploy-infra.yml` | Push to main (terraform/) | Deploy AWS infrastructure |
| `deploy-app.yml` | After build-push | Deploy application to EKS |
| `release.yml` | Tag v* | Create release, deploy to production |

## Prerequisites

### 1. AWS Account Setup

Create an IAM role for GitHub Actions with OIDC authentication:

```bash
# Create OIDC provider for GitHub
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1
```

### 2. Create IAM Role

Create `github-actions-role.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:YOUR_ORG/ollystack:*"
        }
      }
    }
  ]
}
```

```bash
# Create the role
aws iam create-role \
  --role-name ollystack-github-actions \
  --assume-role-policy-document file://github-actions-role.json

# Attach policies
aws iam attach-role-policy \
  --role-name ollystack-github-actions \
  --policy-arn arn:aws:iam::aws:policy/AmazonEKSClusterPolicy

aws iam attach-role-policy \
  --role-name ollystack-github-actions \
  --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser
```

Create custom policy `ollystack-deploy-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "eks:DescribeCluster",
        "eks:ListClusters",
        "eks:AccessKubernetesApi"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken",
        "ecr:BatchCheckLayerAvailability",
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchGetImage",
        "ecr:PutImage",
        "ecr:InitiateLayerUpload",
        "ecr:UploadLayerPart",
        "ecr:CompleteLayerUpload"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::ollystack-terraform-state",
        "arn:aws:s3:::ollystack-terraform-state/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:DeleteItem"
      ],
      "Resource": "arn:aws:dynamodb:*:*:table/ollystack-terraform-lock"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ec2:*",
        "elasticloadbalancing:*",
        "autoscaling:*",
        "iam:PassRole",
        "iam:GetRole",
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:CreateInstanceProfile",
        "iam:DeleteInstanceProfile",
        "iam:AddRoleToInstanceProfile",
        "iam:RemoveRoleFromInstanceProfile"
      ],
      "Resource": "*"
    }
  ]
}
```

```bash
aws iam put-role-policy \
  --role-name ollystack-github-actions \
  --policy-name ollystack-deploy-policy \
  --policy-document file://ollystack-deploy-policy.json
```

### 3. Create Terraform State Backend

```bash
# Create S3 bucket for state
aws s3 mb s3://ollystack-terraform-state --region us-east-2
aws s3api put-bucket-versioning \
  --bucket ollystack-terraform-state \
  --versioning-configuration Status=Enabled

# Create DynamoDB table for locking
aws dynamodb create-table \
  --table-name ollystack-terraform-lock \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region us-east-2
```

### 4. Create ECR Repositories

```bash
aws ecr create-repository --repository-name ollystack/api-server --region us-east-2
aws ecr create-repository --repository-name ollystack/web-ui --region us-east-2
```

## GitHub Configuration

### Required Secrets

Go to **Settings > Secrets and variables > Actions** and add:

| Secret | Description | Example |
|--------|-------------|---------|
| `AWS_ROLE_ARN` | IAM role ARN for OIDC | `arn:aws:iam::123456789:role/ollystack-github-actions` |
| `TF_STATE_BUCKET` | S3 bucket for Terraform state | `ollystack-terraform-state` |
| `TF_LOCK_TABLE` | DynamoDB table for TF locks | `ollystack-terraform-lock` |
| `CODECOV_TOKEN` | (Optional) Codecov token | `xxxx-xxxx-xxxx` |
| `SLACK_WEBHOOK_URL` | (Optional) Slack notifications | `https://hooks.slack.com/...` |

### Environments

Create environments for deployment approvals:

1. Go to **Settings > Environments**
2. Create `dev` environment
3. Create `prod` environment with:
   - Required reviewers (add team members)
   - Wait timer (optional, e.g., 5 minutes)
4. Create `prod-destroy` environment with:
   - Required reviewers
   - Protection rules

## Usage

### Automatic Deployments

1. **Push to `main`**:
   - Runs CI tests
   - Builds & pushes Docker images
   - Deploys to dev environment

2. **Create tag `v1.0.0`**:
   - Creates GitHub release
   - Builds production images
   - Deploys to production (with approval)

### Manual Deployments

```bash
# Trigger infrastructure deployment
gh workflow run deploy-infra.yml -f environment=dev -f action=apply

# Trigger application deployment
gh workflow run deploy-app.yml -f environment=prod -f image_tag=v1.0.0

# Trigger build for specific component
gh workflow run build-push.yml -f component=api-server
```

### Viewing Logs

```bash
# List recent workflow runs
gh run list --workflow=deploy-app.yml

# View specific run
gh run view <run-id>

# Watch running workflow
gh run watch <run-id>
```

## Workflow Details

### CI Pipeline (`ci.yml`)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   changes   │────▶│  api-server │────▶│   docker    │
│  detection  │     │   (Go)      │     │   build     │
└─────────────┘     └─────────────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   web-ui    │
                    │  (React)    │
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │  terraform  │
                    │  validate   │
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │  security   │
                    │   scan      │
                    └─────────────┘
```

### Deployment Pipeline

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Build &   │────▶│   Deploy    │────▶│   Smoke     │
│    Push     │     │    App      │     │   Tests     │
└─────────────┘     └─────────────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │  Database   │
                    │  Migrate    │
                    └─────────────┘
```

## Troubleshooting

### Common Issues

1. **OIDC authentication failed**
   - Verify the IAM role trust policy has correct repo name
   - Check the OIDC provider thumbprint

2. **Terraform state locked**
   ```bash
   aws dynamodb delete-item \
     --table-name ollystack-terraform-lock \
     --key '{"LockID": {"S": "ollystack/dev/terraform.tfstate"}}'
   ```

3. **ECR push failed**
   - Verify ECR repository exists
   - Check IAM permissions for ECR

4. **EKS deployment failed**
   - Verify cluster exists and is accessible
   - Check aws-auth ConfigMap includes the GitHub Actions role

### Adding GitHub Actions Role to EKS

```bash
# Get the role ARN
ROLE_ARN="arn:aws:iam::ACCOUNT_ID:role/ollystack-github-actions"

# Update aws-auth ConfigMap
kubectl edit configmap aws-auth -n kube-system
```

Add to `mapRoles`:
```yaml
- rolearn: arn:aws:iam::ACCOUNT_ID:role/ollystack-github-actions
  username: github-actions
  groups:
    - system:masters
```

## Cost Optimization

The CI/CD pipeline is designed to minimize costs:

1. **Conditional builds**: Only builds changed components
2. **Build caching**: Uses GitHub Actions cache for Docker layers
3. **Spot instances**: Deploys to Spot instances where possible
4. **Ephemeral runners**: Uses GitHub-hosted runners (free for public repos)

For private repos with high volume, consider:
- Self-hosted runners on Spot instances
- Larger runners for faster builds
- Caching artifacts in S3
