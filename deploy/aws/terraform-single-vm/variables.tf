# =============================================================================
# Project Configuration
# =============================================================================

variable "project_name" {
  description = "Project name used for resource naming"
  type        = string
  default     = "ollystack"
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

# =============================================================================
# Instance Configuration
# =============================================================================

variable "instance_type" {
  description = "EC2 instance type (t3.medium recommended for MVP)"
  type        = string
  default     = "t3.medium" # 2 vCPU, 4GB RAM - ~$0.01/hr spot
}

variable "spot_max_price" {
  description = "Maximum hourly price for spot instance (empty = on-demand price)"
  type        = string
  default     = "0.02" # ~$15/month max
}

variable "root_volume_size" {
  description = "Root volume size in GB"
  type        = number
  default     = 20
}

variable "data_volume_size" {
  description = "Data volume size for ClickHouse in GB"
  type        = number
  default     = 30
}

# =============================================================================
# SSH Configuration
# =============================================================================

variable "ssh_public_key" {
  description = "SSH public key for EC2 access (leave empty to use existing_key_pair)"
  type        = string
  default     = ""
}

variable "existing_key_pair" {
  description = "Name of existing AWS key pair (used if ssh_public_key is empty)"
  type        = string
  default     = ""
}

# =============================================================================
# Network Access Control
# =============================================================================

variable "ssh_allowed_cidr" {
  description = "CIDR blocks allowed for SSH access"
  type        = list(string)
  default     = ["0.0.0.0/0"] # Restrict in production!
}

variable "app_allowed_cidr" {
  description = "CIDR blocks allowed for app access (Web UI, API)"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "otlp_allowed_cidr" {
  description = "CIDR blocks allowed for OTLP ingestion"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

# =============================================================================
# Application Configuration
# =============================================================================

variable "clickhouse_password" {
  description = "ClickHouse password"
  type        = string
  default     = "changeme"
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT secret for API authentication"
  type        = string
  default     = "change-me-in-production-use-random-string"
  sensitive   = true
}

variable "domain_name" {
  description = "Domain name for the application (optional)"
  type        = string
  default     = "ollystack.com"
}

variable "letsencrypt_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
  default     = ""
}

variable "preferred_az" {
  description = "Preferred availability zone (e.g., us-east-1a, us-east-1b)"
  type        = string
  default     = ""
}
