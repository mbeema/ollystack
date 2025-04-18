# OllyStack - Production Environment
# Balanced cost/reliability (~$500-800/month)

aws_region   = "us-east-2"
environment  = "prod"
project_name = "ollystack"
cluster_name = "ollystack-prod"

# Kubernetes
kubernetes_version = "1.29"

# VPC - Multi-AZ for reliability
vpc_cidr        = "10.0.0.0/16"
private_subnets = ["10.0.10.0/24", "10.0.11.0/24", "10.0.12.0/24"]
public_subnets  = ["10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24"]

# Still single NAT in prod (can upgrade to multi for HA)
enable_nat_gateway   = true
single_nat_gateway   = true  # Change to false for HA (~+$64/month)
enable_vpc_endpoints = true  # Reduces NAT data transfer costs

# Instance configuration
use_spot_instances = true
spot_instance_types = [
  "t4g.medium",
  "t4g.large",
  "m7g.medium",
  "m7g.large",
]
ondemand_instance_types = [
  "r7g.large",   # 2 vCPU, 16GB
  "r7g.xlarge",  # 4 vCPU, 32GB
  "r7g.2xlarge", # 8 vCPU, 64GB
]
