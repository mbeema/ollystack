# OllyStack Terraform Variables - dev
# Author: Madhukar Beema, Distinguished Engineer

project_name = "ollystack"
environment  = "dev"
aws_region   = "us-east-2"

# EKS Configuration
eks_cluster_version     = "1.29"
eks_node_instance_types = ["t4g.medium", "t4g.large"]

# Node group sizing
min_nodes     = 1
max_nodes     = 5
desired_nodes = 2

# ClickHouse Configuration (self-hosted)
clickhouse_instance_type = "r6g.large"
clickhouse_storage_size  = 100

# Cost optimization
use_spot_instances = true
enable_nat_gateway = true

# Tags
tags = {
  Project     = "ollystack"
  Environment = "dev"
  ManagedBy   = "terraform"
  Author      = "Madhukar Beema"
}
