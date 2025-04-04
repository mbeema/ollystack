#!/bin/bash
# OllyStack Sample Services Deployment Script
# Deploys all sample services to remote server

set -e

REMOTE_HOST="ec2-user@18.209.179.35"
SSH_KEY="~/.ssh/mb2025.pem"
REMOTE_DIR="/home/ec2-user/ollystack/sample-services"

echo "============================================="
echo "OllyStack Sample Services Deployment"
echo "============================================="

# Create remote directory
echo "Creating remote directory..."
ssh -i $SSH_KEY $REMOTE_HOST "mkdir -p $REMOTE_DIR"

# Sync all files
echo "Syncing files to remote server..."
scp -i $SSH_KEY -r \
    configs \
    services \
    docker-compose.yaml \
    $REMOTE_HOST:$REMOTE_DIR/

# Stop existing containers
echo "Stopping existing containers..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose down 2>/dev/null || true"

# Initialize Go modules on remote (generate go.sum files)
echo "Initializing Go modules..."
for service in api-gateway order-service payment-service inventory-service notification-service traffic-generator; do
    echo "  - $service"
    ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR/services/$service && /usr/local/go/bin/go mod tidy 2>/dev/null || true"
done

# Build and start services
echo "Building and starting services..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose build --parallel"

echo "Starting infrastructure services first..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose up -d clickhouse redis postgres"

echo "Waiting for infrastructure to be ready..."
sleep 15

echo "Starting OTel collector..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose up -d otel-collector"
sleep 5

echo "Starting sample microservices..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose up -d api-gateway order-service payment-service inventory-service notification-service"
sleep 10

echo "Starting AI engine and Grafana..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose up -d ai-engine grafana"
sleep 5

echo "Starting traffic generator..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR && docker-compose up -d traffic-generator"

echo ""
echo "============================================="
echo "Deployment Complete!"
echo "============================================="
echo ""
echo "Services:"
echo "  - API Gateway:    http://18.209.179.35:8081/health"
echo "  - Order Service:  http://18.209.179.35:8082/health"
echo "  - Payment:        http://18.209.179.35:8083/health"
echo "  - Inventory:      http://18.209.179.35:8084/health"
echo "  - Notification:   http://18.209.179.35:8085/health"
echo "  - AI Engine:      http://18.209.179.35:8090/health"
echo "  - Grafana:        http://18.209.179.35:3000 (admin/ollystack)"
echo "  - OllyStack API:  http://18.209.179.35:8080/health"
echo ""
echo "Check logs with:"
echo "  ssh -i $SSH_KEY $REMOTE_HOST 'cd $REMOTE_DIR && docker-compose logs -f'"
