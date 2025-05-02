#!/bin/bash
# Build all components on remote server
# Usage: ./scripts/build-remote.sh

set -e

REMOTE_HOST="ec2-user@18.209.179.35"
SSH_KEY="~/.ssh/mb2025.pem"
REMOTE_DIR="/home/ec2-user/ollystack"

echo "=== OllyStack Remote Build Script ==="

# 1. Sync code to remote
echo "[1/5] Syncing code to remote server..."
rsync -avz --progress \
  -e "ssh -i $SSH_KEY" \
  --exclude 'node_modules' \
  --exclude 'dist' \
  --exclude '.git' \
  --exclude '*.tar.gz' \
  --exclude 'tfplan' \
  /Users/mbeema/Documents/devops/observ/ \
  $REMOTE_HOST:$REMOTE_DIR/

# 2. Build AI Engine
echo "[2/5] Building AI Engine..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR/sample-services && docker-compose build ai-engine"

# 3. Build Custom OTel Collector (if needed)
echo "[3/5] Building Custom OTel Collector..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR/otel-collector-custom && make docker-build || echo 'Collector build skipped'"

# 4. Restart services
echo "[4/5] Restarting services..."
ssh -i $SSH_KEY $REMOTE_HOST "cd $REMOTE_DIR/sample-services && docker-compose up -d ai-engine"

# 5. Verify
echo "[5/5] Verifying deployment..."
sleep 10
ssh -i $SSH_KEY $REMOTE_HOST "docker ps --format 'table {{.Names}}\t{{.Status}}' | head -20"

echo ""
echo "=== Build Complete ==="
echo "AI Engine: http://18.209.179.35:8090"
echo "Web UI: http://18.209.179.35:3000"
