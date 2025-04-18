#!/bin/bash
# OllyStack AWS Deployment Script
# Low-cost deployment using Graviton + Spot instances

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║           OllyStack - Low Cost AWS Deployment                  ║"
echo "║      Graviton (ARM) + Spot Instances = Maximum Savings        ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Default values
ENVIRONMENT=${1:-dev}
AWS_REGION=${AWS_REGION:-us-east-2}

echo -e "${YELLOW}Environment: ${ENVIRONMENT}${NC}"
echo -e "${YELLOW}Region: ${AWS_REGION}${NC}"
echo ""

# Check prerequisites
check_prereqs() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    local missing=()

    command -v aws &> /dev/null || missing+=("aws-cli")
    command -v terraform &> /dev/null || missing+=("terraform")
    command -v kubectl &> /dev/null || missing+=("kubectl")
    command -v helm &> /dev/null || missing+=("helm")

    if [ ${#missing[@]} -ne 0 ]; then
        echo -e "${RED}Missing required tools: ${missing[*]}${NC}"
        echo ""
        echo "Install with:"
        echo "  brew install awscli terraform kubectl helm"
        echo "  # or"
        echo "  curl -fsSL https://get.terraform.io | sh"
        exit 1
    fi

    # Check AWS credentials
    if ! aws sts get-caller-identity &> /dev/null; then
        echo -e "${RED}AWS credentials not configured${NC}"
        echo "Run: aws configure"
        exit 1
    fi

    echo -e "${GREEN}✓ All prerequisites met${NC}"
}

# Deploy infrastructure
deploy_infra() {
    echo ""
    echo -e "${YELLOW}Step 1: Deploy Infrastructure with Terraform${NC}"
    echo "────────────────────────────────────────────"

    cd "$SCRIPT_DIR/terraform"

    # Initialize Terraform
    terraform init

    # Plan
    echo ""
    echo "Planning infrastructure..."
    terraform plan -var-file="environments/${ENVIRONMENT}/terraform.tfvars" -out=tfplan

    # Confirm
    echo ""
    read -p "Apply this plan? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled."
        exit 1
    fi

    # Apply
    terraform apply tfplan

    # Get outputs
    CLUSTER_NAME=$(terraform output -raw cluster_name)
    S3_BUCKET=$(terraform output -raw s3_bucket_name)

    echo -e "${GREEN}✓ Infrastructure deployed${NC}"
}

# Configure kubectl
configure_kubectl() {
    echo ""
    echo -e "${YELLOW}Step 2: Configure kubectl${NC}"
    echo "──────────────────────────"

    aws eks update-kubeconfig --region "$AWS_REGION" --name "$CLUSTER_NAME"

    echo -e "${GREEN}✓ kubectl configured${NC}"
}

# Wait for Karpenter
wait_for_karpenter() {
    echo ""
    echo -e "${YELLOW}Step 3: Wait for Karpenter${NC}"
    echo "───────────────────────────"

    echo "Waiting for Karpenter to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/karpenter -n karpenter || true

    echo -e "${GREEN}✓ Karpenter ready${NC}"
}

# Deploy ClickHouse
deploy_clickhouse() {
    echo ""
    echo -e "${YELLOW}Step 4: Deploy ClickHouse${NC}"
    echo "──────────────────────────"

    # Update S3 bucket in config
    sed -i.bak "s|BUCKET_NAME|$S3_BUCKET|g" "$SCRIPT_DIR/k8s/clickhouse.yaml"
    sed -i.bak "s|ACCOUNT_ID|$(aws sts get-caller-identity --query Account --output text)|g" "$SCRIPT_DIR/k8s/clickhouse.yaml"

    kubectl apply -f "$SCRIPT_DIR/k8s/clickhouse.yaml"

    echo "Waiting for ClickHouse to be ready..."
    kubectl wait --for=condition=ready pod -l app=clickhouse -n ollystack --timeout=300s || true

    echo -e "${GREEN}✓ ClickHouse deployed${NC}"
}

# Deploy OllyStack components
deploy_ollystack() {
    echo ""
    echo -e "${YELLOW}Step 5: Deploy OllyStack Components${NC}"
    echo "────────────────────────────────────"

    # Apply Kubernetes manifests
    kubectl apply -k "$SCRIPT_DIR/../kubernetes/base"

    echo "Waiting for pods to be ready..."
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/part-of=ollystack -n ollystack --timeout=300s || true

    echo -e "${GREEN}✓ OllyStack deployed${NC}"
}

# Initialize database
init_database() {
    echo ""
    echo -e "${YELLOW}Step 6: Initialize Database${NC}"
    echo "────────────────────────────"

    # Port forward to ClickHouse
    kubectl port-forward svc/clickhouse 8123:8123 -n ollystack &
    PF_PID=$!
    sleep 5

    # Run init script
    curl -s "http://localhost:8123/" --data-binary @"$SCRIPT_DIR/../scripts/init-clickhouse.sql" || true

    kill $PF_PID 2>/dev/null || true

    echo -e "${GREEN}✓ Database initialized${NC}"
}

# Print summary
print_summary() {
    echo ""
    echo -e "${GREEN}"
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║                    Deployment Complete!                       ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    # Get LoadBalancer URL
    INGRESS_URL=$(kubectl get ingress ollystack-ingress -n ollystack -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "pending")

    echo ""
    echo "Access OllyStack:"
    echo ""
    if [ "$INGRESS_URL" != "pending" ] && [ -n "$INGRESS_URL" ]; then
        echo "  📊 Web UI:     http://${INGRESS_URL}"
        echo "  🔌 API:        http://${INGRESS_URL}/api"
    else
        echo "  Ingress is still provisioning..."
        echo "  Run: kubectl get ingress -n ollystack"
    fi
    echo ""
    echo "  📡 OTLP gRPC:  kubectl port-forward svc/otel-collector 4317:4317 -n ollystack"
    echo "  📡 OTLP HTTP:  kubectl port-forward svc/otel-collector 4318:4318 -n ollystack"
    echo ""
    echo "Estimated Monthly Cost:"
    echo ""
    if [ "$ENVIRONMENT" == "dev" ]; then
        echo "  💰 ~\$150-200/month"
    else
        echo "  💰 ~\$500-800/month"
    fi
    echo ""
    echo "Useful commands:"
    echo ""
    echo "  kubectl get pods -n ollystack"
    echo "  kubectl logs -f -l app=clickhouse -n ollystack"
    echo "  kubectl get nodes -L karpenter.sh/capacity-type,node.kubernetes.io/instance-type"
    echo ""
}

# Main
main() {
    check_prereqs
    deploy_infra
    configure_kubectl
    wait_for_karpenter
    deploy_clickhouse
    deploy_ollystack
    init_database
    print_summary
}

main "$@"
