# OllyStack - Single VM MVP Deployment

Low-cost single VM deployment using AWS EC2 Spot Instance.

## Cost Estimate

| Resource | Monthly Cost |
|----------|-------------|
| EC2 Spot (t3.medium) | ~$7-10 |
| EBS Root (20GB gp3) | ~$1.60 |
| EBS Data (50GB gp3) | ~$4 |
| Elastic IP | ~$3.60 |
| **Total** | **~$15-20/month** |

Compare to EKS setup: ~$150-300+/month

## Prerequisites

1. AWS CLI configured with credentials
2. Terraform >= 1.5.0
3. An SSH key pair

## Quick Start

```bash
# 1. Navigate to this directory
cd deploy/aws/terraform-single-vm

# 2. Copy and edit variables
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your SSH key and preferences

# 3. Initialize Terraform
terraform init

# 4. Review the plan
terraform plan

# 5. Deploy
terraform apply

# 6. Get connection info
terraform output
```

## Connecting

```bash
# SSH into the instance
ssh -i <your-key.pem> ec2-user@<public-ip>

# Check services
cd /opt/ollystack
docker-compose ps
docker-compose logs -f
```

## Sending Telemetry

Configure your applications to send OTLP data to:

- **gRPC**: `<public-ip>:4317`
- **HTTP**: `http://<public-ip>:4318`

Example with OpenTelemetry SDK:
```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://<public-ip>:4318"
export OTEL_SERVICE_NAME="my-service"
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| ClickHouse | 8123 | HTTP interface |
| ClickHouse | 9000 | Native protocol |
| Redis | 6379 | Caching |
| OTLP gRPC | 4317 | Telemetry ingestion |
| OTLP HTTP | 4318 | Telemetry ingestion |
| API Server | 8080 | Query API |
| Web UI | 3000 | Dashboard |

## Data Persistence

- **Data volume** (`/data`): Persists ClickHouse and Redis data
- Data volume is **not deleted** when instance terminates
- On spot interruption, instance stops but data is preserved

## Adding API Server & Web UI

The user-data script sets up the infrastructure. To add your custom services:

```bash
# SSH into instance
ssh -i <key.pem> ec2-user@<ip>

# Build and push your images to ECR, or build locally
cd /opt/ollystack

# Edit docker-compose.yml to add your images
# Then restart
docker-compose up -d
```

## Spot Instance Notes

- Uses **persistent** spot request with **stop** behavior
- On interruption: instance stops, EIP detaches, data preserved
- When capacity returns: instance restarts automatically
- Elastic IP provides stable DNS/IP

## Destroying

```bash
terraform destroy
```

**Note**: Data volume is preserved by default. Delete manually if needed.
