# OllyStack AWS Variables

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-2" # Ohio
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

variable "project_name" {
  description = "Project name"
  type        = string
  default     = "ollystack"
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
  default     = "ollystack-cluster"
}

variable "kubernetes_version" {
  description = "Kubernetes version"
  type        = string
  default     = "1.29"
}

# =============================================================================
# VPC
# =============================================================================

variable "vpc_cidr" {
  description = "VPC CIDR block"
  type        = string
  default     = "10.0.0.0/16"
}

variable "private_subnets" {
  description = "Private subnet CIDRs"
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.11.0/24", "10.0.12.0/24"]
}

variable "public_subnets" {
  description = "Public subnet CIDRs"
  type        = list(string)
  default     = ["10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24"]
}

variable "enable_nat_gateway" {
  description = "Enable NAT Gateway"
  type        = bool
  default     = true
}

variable "single_nat_gateway" {
  description = "Use single NAT Gateway (cost saving)"
  type        = bool
  default     = true # Set false for HA in prod
}

variable "enable_vpc_endpoints" {
  description = "Enable VPC endpoints for ECR (reduces NAT costs)"
  type        = bool
  default     = false # Enable in prod to save on NAT data transfer
}

# =============================================================================
# Cost Optimization
# =============================================================================

variable "use_spot_instances" {
  description = "Use Spot instances for stateless workloads"
  type        = bool
  default     = true
}

variable "spot_instance_types" {
  description = "Instance types for Spot node pool"
  type        = list(string)
  default     = ["t4g.medium", "t4g.large", "m7g.medium", "m7g.large"]
}

variable "ondemand_instance_types" {
  description = "Instance types for On-Demand node pool (ClickHouse)"
  type        = list(string)
  default     = ["r7g.large", "r7g.xlarge"]
}
