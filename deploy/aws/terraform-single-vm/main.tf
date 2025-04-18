# OllyStack - Single VM MVP Deployment (Spot Instance)
# Estimated cost: ~$10-15/month (t3.medium spot in us-east-1)

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "OllyStack"
      Environment = "mvp"
      ManagedBy   = "Terraform"
    }
  }
}

# =============================================================================
# Data Sources
# =============================================================================

data "aws_availability_zones" "available" {
  state = "available"
}

# Get latest Amazon Linux 2023 AMI
data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# =============================================================================
# VPC - Use default for simplicity
# =============================================================================

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }

  filter {
    name   = "availability-zone"
    values = ["us-east-1a", "us-east-1b", "us-east-1c", "us-east-1d", "us-east-1f"]
  }
}

# Get subnet for preferred AZ
data "aws_subnet" "preferred" {
  count = var.preferred_az != "" ? 1 : 0

  vpc_id            = data.aws_vpc.default.id
  availability_zone = var.preferred_az
}

# =============================================================================
# Security Group
# =============================================================================

resource "aws_security_group" "ollystack" {
  name_prefix = "${var.project_name}-"
  description = "Security group for OllyStack single VM"
  vpc_id      = data.aws_vpc.default.id

  # SSH access
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.ssh_allowed_cidr
  }

  # HTTP
  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # HTTPS
  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Web UI
  ingress {
    description = "Web UI"
    from_port   = 3000
    to_port     = 3000
    protocol    = "tcp"
    cidr_blocks = var.app_allowed_cidr
  }

  # API Server
  ingress {
    description = "API Server"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = var.app_allowed_cidr
  }

  # OTLP gRPC
  ingress {
    description = "OTLP gRPC"
    from_port   = 4317
    to_port     = 4317
    protocol    = "tcp"
    cidr_blocks = var.otlp_allowed_cidr
  }

  # OTLP HTTP
  ingress {
    description = "OTLP HTTP"
    from_port   = 4318
    to_port     = 4318
    protocol    = "tcp"
    cidr_blocks = var.otlp_allowed_cidr
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.project_name}-sg"
  }

  lifecycle {
    create_before_destroy = true
  }
}

# =============================================================================
# SSH Key Pair
# =============================================================================

resource "aws_key_pair" "ollystack" {
  count = var.ssh_public_key != "" ? 1 : 0

  key_name   = "${var.project_name}-key"
  public_key = var.ssh_public_key
}

# =============================================================================
# Launch Template for Spot Instance
# =============================================================================

resource "aws_launch_template" "ollystack" {
  name_prefix   = "${var.project_name}-"
  image_id      = data.aws_ami.amazon_linux.id
  instance_type = var.instance_type

  key_name = var.ssh_public_key != "" ? aws_key_pair.ollystack[0].key_name : var.existing_key_pair

  # On-demand instance (spot disabled for now)
  # instance_market_options {
  #   market_type = "spot"
  #   spot_options {
  #     max_price                      = var.spot_max_price
  #     spot_instance_type             = "persistent"
  #     instance_interruption_behavior = "stop"
  #   }
  # }

  block_device_mappings {
    device_name = "/dev/xvda"
    ebs {
      volume_type           = "gp3"
      volume_size           = var.root_volume_size
      delete_on_termination = true
      encrypted             = true
    }
  }

  # Data volume for ClickHouse
  block_device_mappings {
    device_name = "/dev/sdf"
    ebs {
      volume_type           = "gp3"
      volume_size           = var.data_volume_size
      delete_on_termination = false
      encrypted             = true
    }
  }

  user_data = base64encode(templatefile("${path.module}/user-data.sh", {
    clickhouse_password = var.clickhouse_password
    jwt_secret          = var.jwt_secret
    domain_name         = var.domain_name
    letsencrypt_email   = var.letsencrypt_email
  }))

  tag_specifications {
    resource_type = "instance"
    tags = {
      Name = "${var.project_name}-spot"
    }
  }

  tag_specifications {
    resource_type = "volume"
    tags = {
      Name = "${var.project_name}-volume"
    }
  }

  lifecycle {
    create_before_destroy = true
  }
}

# =============================================================================
# Spot Instance
# =============================================================================

resource "aws_instance" "ollystack" {
  launch_template {
    id      = aws_launch_template.ollystack.id
    version = "$Latest"
  }

  subnet_id                   = var.preferred_az != "" ? data.aws_subnet.preferred[0].id : data.aws_subnets.default.ids[0]
  availability_zone           = var.preferred_az != "" ? var.preferred_az : null
  vpc_security_group_ids      = [aws_security_group.ollystack.id]
  associate_public_ip_address = true

  tags = {
    Name = "${var.project_name}-spot"
  }

  lifecycle {
    ignore_changes = [ami, user_data]
  }
}

# =============================================================================
# Elastic IP (stable public IP - persists across spot interruptions)
# =============================================================================

resource "aws_eip" "ollystack" {
  domain = "vpc"

  tags = {
    Name = "${var.project_name}-eip"
  }
}

resource "aws_eip_association" "ollystack" {
  instance_id   = aws_instance.ollystack.id
  allocation_id = aws_eip.ollystack.id
}
