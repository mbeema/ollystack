#!/bin/bash
# OllyStack Deployment Script
# Usage: ./scripts/deploy.sh [component]
# Components: all, api, collector, schema

set -e

# Configuration
REMOTE_HOST="ec2-user@18.209.179.35"
SSH_KEY="~/.ssh/mb2025.pem"
REMOTE_DIR="/home/ec2-user/ollystack"
LOCAL_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# SSH command wrapper
ssh_cmd() {
    ssh -i $SSH_KEY $REMOTE_HOST "$@"
}

# SCP command wrapper
scp_cmd() {
    scp -i $SSH_KEY -r "$@"
}

# Create remote directory structure
setup_remote() {
    log_info "Setting up remote directory structure..."
    ssh_cmd "mkdir -p $REMOTE_DIR/{api-server,otel-collector-custom,otel-processor-correlation,schema,deploy}"
}

# Deploy ClickHouse schema
deploy_schema() {
    log_info "Deploying ClickHouse schema..."
    scp_cmd "$LOCAL_DIR/schema/clickhouse_schema.sql" "$REMOTE_HOST:$REMOTE_DIR/schema/"

    log_info "Applying schema to ClickHouse..."
    ssh_cmd "cat $REMOTE_DIR/schema/clickhouse_schema.sql | docker exec -i ollystack-clickhouse clickhouse-client --multiquery"

    log_info "Verifying schema..."
    ssh_cmd "docker exec ollystack-clickhouse clickhouse-client --query 'SHOW TABLES FROM ollystack'"
}

# Deploy API server
deploy_api() {
    log_info "Deploying API server..."

    # Sync api-server directory
    log_info "Syncing api-server files..."
    rsync -avz --progress -e "ssh -i $SSH_KEY" \
        --exclude 'vendor' \
        --exclude '*.test' \
        "$LOCAL_DIR/api-server/" "$REMOTE_HOST:$REMOTE_DIR/api-server/"

    # Build on remote
    log_info "Building API server on remote..."
    ssh_cmd "cd $REMOTE_DIR/api-server && go build -o ollystack-api ./cmd/server"

    # Restart service
    log_info "Restarting API server..."
    ssh_cmd "cd $REMOTE_DIR && docker-compose restart api-server 2>/dev/null || docker restart ollystack-api 2>/dev/null || echo 'Manual restart may be needed'"
}

# Deploy OTel Collector
deploy_collector() {
    log_info "Deploying OTel Collector..."

    # Sync processor
    log_info "Syncing correlation processor..."
    rsync -avz --progress -e "ssh -i $SSH_KEY" \
        "$LOCAL_DIR/otel-processor-correlation/" "$REMOTE_HOST:$REMOTE_DIR/otel-processor-correlation/"

    # Sync collector config
    log_info "Syncing collector configuration..."
    rsync -avz --progress -e "ssh -i $SSH_KEY" \
        "$LOCAL_DIR/otel-collector-custom/" "$REMOTE_HOST:$REMOTE_DIR/otel-collector-custom/"

    # Build collector on remote
    log_info "Building custom collector on remote..."
    ssh_cmd "cd $REMOTE_DIR/otel-collector-custom && make build 2>/dev/null || echo 'OCB not installed, using config only'"

    # Restart collector
    log_info "Restarting OTel Collector..."
    ssh_cmd "docker restart otel-collector 2>/dev/null || echo 'Manual restart may be needed'"
}

# Deploy Grafana configs
deploy_grafana() {
    log_info "Deploying Grafana configurations..."

    rsync -avz --progress -e "ssh -i $SSH_KEY" \
        "$LOCAL_DIR/deploy/grafana/" "$REMOTE_HOST:$REMOTE_DIR/deploy/grafana/"

    # Copy to Grafana container volumes if needed
    ssh_cmd "docker cp $REMOTE_DIR/deploy/grafana/dashboards/. grafana:/var/lib/grafana/dashboards/ 2>/dev/null || echo 'Grafana container not found'"
    ssh_cmd "docker restart grafana 2>/dev/null || echo 'Manual restart may be needed'"
}

# Run tests on remote
run_remote_tests() {
    log_info "Running tests on remote..."

    # Test API
    log_info "Testing API server..."
    ssh_cmd "cd $REMOTE_DIR/api-server && go test -v ./... 2>&1 | head -50"

    # Test processor
    log_info "Testing correlation processor..."
    ssh_cmd "cd $REMOTE_DIR/otel-processor-correlation && go test -v ./... 2>&1 | head -50"
}

# Health check
health_check() {
    log_info "Running health checks..."

    # Check ClickHouse
    log_info "Checking ClickHouse..."
    ssh_cmd "docker exec ollystack-clickhouse clickhouse-client --query 'SELECT 1'" && log_info "ClickHouse: OK" || log_error "ClickHouse: FAILED"

    # Check API
    log_info "Checking API server..."
    ssh_cmd "curl -s http://localhost:8080/health 2>/dev/null" && log_info "API: OK" || log_warn "API: Not responding"

    # Check OTel Collector
    log_info "Checking OTel Collector..."
    ssh_cmd "curl -s http://localhost:13133/health 2>/dev/null" && log_info "Collector: OK" || log_warn "Collector: Not responding"
}

# Deploy all components
deploy_all() {
    setup_remote
    deploy_schema
    deploy_api
    deploy_collector
    deploy_grafana
    run_remote_tests
    health_check
}

# Main
case "${1:-all}" in
    schema)
        deploy_schema
        ;;
    api)
        deploy_api
        ;;
    collector)
        deploy_collector
        ;;
    grafana)
        deploy_grafana
        ;;
    test)
        run_remote_tests
        ;;
    health)
        health_check
        ;;
    all)
        deploy_all
        ;;
    *)
        echo "Usage: $0 {all|schema|api|collector|grafana|test|health}"
        exit 1
        ;;
esac

log_info "Deployment complete!"
