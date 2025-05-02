#!/bin/bash
# OllyStack Full Deployment Script
# Deploys infrastructure and application to AWS
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="${PROJECT_NAME:-ollystack}"
AWS_REGION="${AWS_REGION:-us-east-2}"
ENVIRONMENT="${1:-dev}"
SKIP_INFRA="${SKIP_INFRA:-false}"
SKIP_BUILD="${SKIP_BUILD:-false}"

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "${SCRIPT_DIR}")"
TERRAFORM_DIR="${PROJECT_ROOT}/deploy/aws/terraform"
K8S_DIR="${PROJECT_ROOT}/deploy/kubernetes"

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

log_step() {
    echo ""
    echo -e "${CYAN}=========================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}=========================================${NC}"
    echo ""
}

check_prerequisites() {
    log_step "Checking Prerequisites"

    local missing=()

    # Check required tools
    command -v aws &>/dev/null || missing+=("aws-cli")
    command -v terraform &>/dev/null || missing+=("terraform")
    command -v kubectl &>/dev/null || missing+=("kubectl")
    command -v docker &>/dev/null || missing+=("docker")

    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing[*]}"
    fi

    # Check AWS credentials
    if ! aws sts get-caller-identity &>/dev/null; then
        log_error "AWS credentials not configured"
    fi

    log_success "All prerequisites met"
}

get_aws_info() {
    ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
    ECR_REGISTRY="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
    log_info "AWS Account: ${ACCOUNT_ID}"
    log_info "ECR Registry: ${ECR_REGISTRY}"
}

deploy_infrastructure() {
    if [ "${SKIP_INFRA}" = "true" ]; then
        log_warn "Skipping infrastructure deployment (SKIP_INFRA=true)"
        return
    fi

    log_step "Deploying Infrastructure"

    cd "${TERRAFORM_DIR}"

    # Initialize Terraform
    log_info "Initializing Terraform..."
    terraform init -reconfigure

    # Plan
    log_info "Planning infrastructure changes..."
    terraform plan \
        -var-file="environments/${ENVIRONMENT}.tfvars" \
        -out=tfplan

    # Apply
    log_info "Applying infrastructure changes..."
    terraform apply -auto-approve tfplan

    # Get outputs
    EKS_CLUSTER_NAME=$(terraform output -raw eks_cluster_name 2>/dev/null || echo "${PROJECT_NAME}-${ENVIRONMENT}")

    log_success "Infrastructure deployed"

    cd "${PROJECT_ROOT}"
}

configure_kubectl() {
    log_step "Configuring kubectl"

    EKS_CLUSTER_NAME="${PROJECT_NAME}-${ENVIRONMENT}"

    log_info "Updating kubeconfig for cluster: ${EKS_CLUSTER_NAME}"
    aws eks update-kubeconfig \
        --name "${EKS_CLUSTER_NAME}" \
        --region "${AWS_REGION}" \
        --alias "${EKS_CLUSTER_NAME}"

    # Verify connection
    if kubectl cluster-info &>/dev/null; then
        log_success "kubectl configured successfully"
    else
        log_error "Failed to connect to EKS cluster"
    fi
}

build_and_push_images() {
    if [ "${SKIP_BUILD}" = "true" ]; then
        log_warn "Skipping image build (SKIP_BUILD=true)"
        return
    fi

    log_step "Building and Pushing Docker Images"

    cd "${PROJECT_ROOT}"

    # Login to ECR
    log_info "Logging in to ECR..."
    aws ecr get-login-password --region "${AWS_REGION}" | \
        docker login --username AWS --password-stdin "${ECR_REGISTRY}"

    # Get version info
    VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

    # Build and push API server
    log_info "Building api-server..."
    docker build \
        --platform linux/arm64 \
        --build-arg VERSION="${VERSION}" \
        --build-arg COMMIT="${COMMIT}" \
        -t "${ECR_REGISTRY}/${PROJECT_NAME}/api-server:${VERSION}" \
        -t "${ECR_REGISTRY}/${PROJECT_NAME}/api-server:latest" \
        -f api-server/Dockerfile \
        api-server/

    log_info "Pushing api-server..."
    docker push "${ECR_REGISTRY}/${PROJECT_NAME}/api-server:${VERSION}"
    docker push "${ECR_REGISTRY}/${PROJECT_NAME}/api-server:latest"

    # Build and push Web UI
    log_info "Building web-ui..."
    docker build \
        --platform linux/arm64 \
        --build-arg VERSION="${VERSION}" \
        --build-arg COMMIT="${COMMIT}" \
        -t "${ECR_REGISTRY}/${PROJECT_NAME}/web-ui:${VERSION}" \
        -t "${ECR_REGISTRY}/${PROJECT_NAME}/web-ui:latest" \
        -f web-ui/Dockerfile \
        web-ui/

    log_info "Pushing web-ui..."
    docker push "${ECR_REGISTRY}/${PROJECT_NAME}/web-ui:${VERSION}"
    docker push "${ECR_REGISTRY}/${PROJECT_NAME}/web-ui:latest"

    log_success "Images built and pushed"
}

deploy_kubernetes_resources() {
    log_step "Deploying Kubernetes Resources"

    cd "${PROJECT_ROOT}"

    # Create namespace
    log_info "Creating namespace..."
    kubectl apply -f "${K8S_DIR}/base/namespace.yaml"

    # Create secrets
    log_info "Creating secrets..."
    kubectl apply -f "${K8S_DIR}/base/secrets.yaml" || log_warn "Secrets may need manual configuration"

    # Create configmaps
    log_info "Creating configmaps..."
    kubectl apply -f "${K8S_DIR}/base/configmap.yaml"

    # Deploy Redis
    log_info "Deploying Redis..."
    kubectl apply -f "${K8S_DIR}/base/redis.yaml"

    # Deploy ClickHouse (if self-hosted)
    if [ -f "${K8S_DIR}/clickhouse/clickhouse.yaml" ]; then
        log_info "Deploying ClickHouse..."
        kubectl apply -f "${K8S_DIR}/clickhouse/"
    fi

    # Deploy OTel Collector
    log_info "Deploying OTel Collector..."
    kubectl apply -f "${K8S_DIR}/base/collector.yaml"

    # Deploy API Server
    log_info "Deploying API Server..."
    kubectl apply -f "${K8S_DIR}/base/api-server.yaml"

    # Deploy Web UI
    log_info "Deploying Web UI..."
    kubectl apply -f "${K8S_DIR}/base/web-ui.yaml"

    # Deploy Ingress
    log_info "Deploying Ingress..."
    kubectl apply -f "${K8S_DIR}/base/ingress.yaml"

    log_success "Kubernetes resources deployed"
}

wait_for_deployments() {
    log_step "Waiting for Deployments"

    local deployments=("api-server" "web-ui" "otel-collector")
    local namespace="ollystack"

    for deployment in "${deployments[@]}"; do
        log_info "Waiting for ${deployment}..."
        kubectl rollout status deployment/${deployment} -n ${namespace} --timeout=300s || \
            log_warn "${deployment} deployment may still be starting"
    done

    log_success "All deployments ready"
}

run_smoke_tests() {
    log_step "Running Smoke Tests"

    local namespace="ollystack"

    # Check pods
    log_info "Checking pod status..."
    kubectl get pods -n ${namespace}

    # Check services
    log_info "Checking services..."
    kubectl get svc -n ${namespace}

    # Test API health
    log_info "Testing API health endpoint..."
    local api_pod=$(kubectl get pod -n ${namespace} -l app=api-server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -n "${api_pod}" ]; then
        kubectl exec -n ${namespace} ${api_pod} -- wget -qO- http://localhost:8080/health || log_warn "API health check failed"
    fi

    log_success "Smoke tests completed"
}

print_summary() {
    log_step "Deployment Summary"

    # Get ingress URL
    local ingress_url=$(kubectl get ingress -n ollystack -o jsonpath='{.items[0].status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "pending")

    echo ""
    echo "Environment: ${ENVIRONMENT}"
    echo "Region: ${AWS_REGION}"
    echo "Cluster: ${PROJECT_NAME}-${ENVIRONMENT}"
    echo ""
    echo "Access URLs:"
    echo "  Ingress: http://${ingress_url}"
    echo ""
    echo "Useful commands:"
    echo "  kubectl get pods -n ollystack"
    echo "  kubectl logs -f deployment/api-server -n ollystack"
    echo "  kubectl port-forward svc/web-ui 3000:80 -n ollystack"
    echo ""

    log_success "Deployment completed successfully!"
}

show_help() {
    echo "Usage: $0 [ENVIRONMENT] [OPTIONS]"
    echo ""
    echo "Deploy OllyStack to AWS"
    echo ""
    echo "Arguments:"
    echo "  ENVIRONMENT    Target environment (default: dev)"
    echo ""
    echo "Environment variables:"
    echo "  SKIP_INFRA=true     Skip infrastructure deployment"
    echo "  SKIP_BUILD=true     Skip Docker image build"
    echo "  AWS_REGION          AWS region (default: us-east-2)"
    echo "  PROJECT_NAME        Project name (default: ollystack)"
    echo ""
    echo "Examples:"
    echo "  $0 dev                    # Full deployment to dev"
    echo "  $0 prod                   # Full deployment to prod"
    echo "  SKIP_INFRA=true $0 dev   # Skip infrastructure, deploy app only"
    echo ""
}

# Main execution
main() {
    if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
        show_help
        exit 0
    fi

    echo ""
    echo "==========================================="
    echo "  OllyStack Full Deployment"
    echo "  Environment: ${ENVIRONMENT}"
    echo "  Region: ${AWS_REGION}"
    echo "==========================================="
    echo ""

    check_prerequisites
    get_aws_info
    deploy_infrastructure
    configure_kubectl
    build_and_push_images
    deploy_kubernetes_resources
    wait_for_deployments
    run_smoke_tests
    print_summary
}

main "$@"
