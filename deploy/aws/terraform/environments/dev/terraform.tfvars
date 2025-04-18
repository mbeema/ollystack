# OllyStack - Development Environment
# Optimized for minimum cost (~$150-200/month)

aws_region   = "us-east-2" # Ohio - typically cheapest
environment  = "dev"
project_name = "ollystack"
cluster_name = "ollystack-dev"

# Kubernetes
kubernetes_version = "1.29"

# VPC - Minimal setup
vpc_cidr        = "10.0.0.0/16"
private_subnets = ["10.0.10.0/24", "10.0.11.0/24"]
public_subnets  = ["10.0.0.0/24", "10.0.1.0/24"]

# Cost optimizations
enable_nat_gateway   = true
single_nat_gateway   = true  # Single NAT saves ~$32/month per extra NAT
enable_vpc_endpoints = false # Skip VPC endpoints in dev

# Instance configuration
use_spot_instances = true
spot_instance_types = [
  "t4g.small",  # $0.004/hr spot
  "t4g.medium", # $0.008/hr spot
]
ondemand_instance_types = [
  "t4g.large",  # For single-node ClickHouse in dev
  "r7g.large",  # If more memory needed
]
