#!/bin/bash
# OllyStack AWS Bootstrap Script
# Creates required AWS resources for first-time setup
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="${PROJECT_NAME:-ollystack}"
AWS_REGION="${AWS_REGION:-us-east-2}"
ENVIRONMENT="${1:-dev}"

# Resource names
TF_STATE_BUCKET="${PROJECT_NAME}-terraform-state-${AWS_REGION}"
TF_LOCK_TABLE="${PROJECT_NAME}-terraform-lock"
ECR_REPOS=("${PROJECT_NAME}/api-server" "${PROJECT_NAME}/web-ui" "${PROJECT_NAME}/collector")

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check AWS CLI
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI is not installed. Please install it first."
    fi

    # Check AWS credentials
    if ! aws sts get-caller-identity &> /dev/null; then
        log_error "AWS credentials not configured. Please set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY."
    fi

    # Check Terraform
    if ! command -v terraform &> /dev/null; then
        log_error "Terraform is not installed. Please install it first."
    fi

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_warn "kubectl is not installed. You'll need it for deployments."
    fi

    log_success "All prerequisites met!"
}

get_account_info() {
    log_info "Getting AWS account information..."
    ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
    log_success "AWS Account ID: ${ACCOUNT_ID}"
    log_success "AWS Region: ${AWS_REGION}"
}

create_terraform_backend() {
    log_info "Setting up Terraform state backend..."

    # Create S3 bucket for state
    if aws s3api head-bucket --bucket "${TF_STATE_BUCKET}" 2>/dev/null; then
        log_warn "S3 bucket ${TF_STATE_BUCKET} already exists"
    else
        log_info "Creating S3 bucket: ${TF_STATE_BUCKET}"

        if [ "${AWS_REGION}" = "us-east-1" ]; then
            aws s3api create-bucket \
                --bucket "${TF_STATE_BUCKET}" \
                --region "${AWS_REGION}"
        else
            aws s3api create-bucket \
                --bucket "${TF_STATE_BUCKET}" \
                --region "${AWS_REGION}" \
                --create-bucket-configuration LocationConstraint="${AWS_REGION}"
        fi

        # Enable versioning
        aws s3api put-bucket-versioning \
            --bucket "${TF_STATE_BUCKET}" \
            --versioning-configuration Status=Enabled

        # Enable encryption
        aws s3api put-bucket-encryption \
            --bucket "${TF_STATE_BUCKET}" \
            --server-side-encryption-configuration '{
                "Rules": [{
                    "ApplyServerSideEncryptionByDefault": {
                        "SSEAlgorithm": "AES256"
                    }
                }]
            }'

        # Block public access
        aws s3api put-public-access-block \
            --bucket "${TF_STATE_BUCKET}" \
            --public-access-block-configuration \
                "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"

        log_success "S3 bucket created: ${TF_STATE_BUCKET}"
    fi

    # Create DynamoDB table for state locking
    if aws dynamodb describe-table --table-name "${TF_LOCK_TABLE}" --region "${AWS_REGION}" 2>/dev/null; then
        log_warn "DynamoDB table ${TF_LOCK_TABLE} already exists"
    else
        log_info "Creating DynamoDB table: ${TF_LOCK_TABLE}"

        aws dynamodb create-table \
            --table-name "${TF_LOCK_TABLE}" \
            --attribute-definitions AttributeName=LockID,AttributeType=S \
            --key-schema AttributeName=LockID,KeyType=HASH \
            --billing-mode PAY_PER_REQUEST \
            --region "${AWS_REGION}"

        log_info "Waiting for table to be active..."
        aws dynamodb wait table-exists --table-name "${TF_LOCK_TABLE}" --region "${AWS_REGION}"

        log_success "DynamoDB table created: ${TF_LOCK_TABLE}"
    fi
}

create_ecr_repositories() {
    log_info "Creating ECR repositories..."

    for repo in "${ECR_REPOS[@]}"; do
        if aws ecr describe-repositories --repository-names "${repo}" --region "${AWS_REGION}" 2>/dev/null; then
            log_warn "ECR repository ${repo} already exists"
        else
            log_info "Creating ECR repository: ${repo}"

            aws ecr create-repository \
                --repository-name "${repo}" \
                --region "${AWS_REGION}" \
                --image-scanning-configuration scanOnPush=true \
                --encryption-configuration encryptionType=AES256

            # Set lifecycle policy to clean up old images
            aws ecr put-lifecycle-policy \
                --repository-name "${repo}" \
                --region "${AWS_REGION}" \
                --lifecycle-policy-text '{
                    "rules": [
                        {
                            "rulePriority": 1,
                            "description": "Keep last 10 images",
                            "selection": {
                                "tagStatus": "any",
                                "countType": "imageCountMoreThan",
                                "countNumber": 10
                            },
                            "action": {
                                "type": "expire"
                            }
                        }
                    ]
                }'

            log_success "ECR repository created: ${repo}"
        fi
    done
}

create_terraform_backend_config() {
    log_info "Creating Terraform backend configuration..."

    BACKEND_FILE="deploy/aws/terraform/backend.tf"

    cat > "${BACKEND_FILE}" << EOF
# Auto-generated Terraform backend configuration
# Generated by bootstrap-aws.sh on $(date)

terraform {
  backend "s3" {
    bucket         = "${TF_STATE_BUCKET}"
    key            = "ollystack/${ENVIRONMENT}/terraform.tfstate"
    region         = "${AWS_REGION}"
    encrypt        = true
    dynamodb_table = "${TF_LOCK_TABLE}"
  }
}
EOF

    log_success "Backend configuration written to ${BACKEND_FILE}"
}

create_tfvars_file() {
    log_info "Creating terraform.tfvars file for ${ENVIRONMENT}..."

    TFVARS_FILE="deploy/aws/terraform/environments/${ENVIRONMENT}.tfvars"
    mkdir -p "deploy/aws/terraform/environments"

    if [ -f "${TFVARS_FILE}" ]; then
        log_warn "tfvars file already exists: ${TFVARS_FILE}"
        return
    fi

    cat > "${TFVARS_FILE}" << EOF
# OllyStack Terraform Variables - ${ENVIRONMENT}
# Auto-generated by bootstrap-aws.sh on $(date)

project_name = "${PROJECT_NAME}"
environment  = "${ENVIRONMENT}"
aws_region   = "${AWS_REGION}"

# EKS Configuration
eks_cluster_version = "1.29"
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
enable_nat_gateway = ${ENVIRONMENT == "prod" ? "true" : "false"}

# Tags
tags = {
  Project     = "${PROJECT_NAME}"
  Environment = "${ENVIRONMENT}"
  ManagedBy   = "terraform"
  CreatedBy   = "bootstrap-aws.sh"
}
EOF

    log_success "Created tfvars file: ${TFVARS_FILE}"
}

print_github_secrets() {
    log_info "GitHub Secrets to configure..."

    ECR_REGISTRY="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

    echo ""
    echo "==========================================="
    echo "  GitHub Repository Secrets Required"
    echo "==========================================="
    echo ""
    echo "Add these secrets to your GitHub repository:"
    echo "Settings > Secrets and variables > Actions > New repository secret"
    echo ""
    echo "AWS_ACCESS_KEY_ID      = <your-access-key-id>"
    echo "AWS_SECRET_ACCESS_KEY  = <your-secret-access-key>"
    echo "AWS_REGION             = ${AWS_REGION}"
    echo "AWS_ACCOUNT_ID         = ${ACCOUNT_ID}"
    echo "ECR_REGISTRY           = ${ECR_REGISTRY}"
    echo "TF_STATE_BUCKET        = ${TF_STATE_BUCKET}"
    echo "TF_LOCK_TABLE          = ${TF_LOCK_TABLE}"
    echo ""
    echo "Optional secrets:"
    echo "SLACK_WEBHOOK_URL      = <for-deployment-notifications>"
    echo "CODECOV_TOKEN          = <for-code-coverage-reports>"
    echo ""
    echo "==========================================="
}

print_next_steps() {
    echo ""
    echo "==========================================="
    echo "  Bootstrap Complete! Next Steps:"
    echo "==========================================="
    echo ""
    echo "1. Configure GitHub secrets (see above)"
    echo ""
    echo "2. Deploy infrastructure:"
    echo "   make aws-infra-init ENV=${ENVIRONMENT}"
    echo "   make aws-infra-plan ENV=${ENVIRONMENT}"
    echo "   make aws-infra-apply ENV=${ENVIRONMENT}"
    echo ""
    echo "3. Build and push Docker images:"
    echo "   make aws-ecr-login"
    echo "   make aws-push-images"
    echo ""
    echo "4. Deploy application:"
    echo "   make aws-configure-kubectl ENV=${ENVIRONMENT}"
    echo "   make aws-deploy-app ENV=${ENVIRONMENT}"
    echo ""
    echo "Or deploy everything at once:"
    echo "   make aws-deploy-all ENV=${ENVIRONMENT}"
    echo ""
    echo "==========================================="
}

# Main execution
main() {
    echo ""
    echo "==========================================="
    echo "  OllyStack AWS Bootstrap"
    echo "  Environment: ${ENVIRONMENT}"
    echo "==========================================="
    echo ""

    check_prerequisites
    get_account_info
    create_terraform_backend
    create_ecr_repositories
    create_terraform_backend_config
    create_tfvars_file
    print_github_secrets
    print_next_steps

    log_success "Bootstrap completed successfully!"
}

main "$@"
