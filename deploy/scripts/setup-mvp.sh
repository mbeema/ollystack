#!/bin/bash
# OllyStack MVP Setup Script
# This script helps you set up OllyStack with managed ClickHouse Cloud

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║                    OllyStack MVP Setup                         ║"
echo "║              Using Managed ClickHouse Cloud                   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check prerequisites
check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    local missing=()

    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi

    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        missing+=("docker-compose")
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        echo -e "${RED}Missing required tools: ${missing[*]}${NC}"
        echo "Please install them and try again."
        exit 1
    fi

    echo -e "${GREEN}✓ All prerequisites met${NC}"
}

# Setup ClickHouse Cloud
setup_clickhouse_cloud() {
    echo ""
    echo -e "${YELLOW}Step 1: ClickHouse Cloud Setup${NC}"
    echo "─────────────────────────────────"
    echo ""
    echo "If you haven't already, create a ClickHouse Cloud account:"
    echo "  → https://clickhouse.cloud"
    echo ""
    echo "Then create a new service and note down:"
    echo "  - Host (e.g., xxx.region.clickhouse.cloud)"
    echo "  - Username (usually 'default')"
    echo "  - Password"
    echo ""

    read -p "Do you have your ClickHouse Cloud credentials ready? (y/n) " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Please set up ClickHouse Cloud first, then run this script again."
        exit 1
    fi
}

# Configure environment
configure_environment() {
    echo ""
    echo -e "${YELLOW}Step 2: Configure Environment${NC}"
    echo "─────────────────────────────────"

    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    DOCKER_DIR="$(dirname "$SCRIPT_DIR")/docker"

    if [ -f "$DOCKER_DIR/.env" ]; then
        echo -e "${YELLOW}Existing .env file found.${NC}"
        read -p "Do you want to reconfigure? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            return
        fi
    fi

    echo ""
    read -p "ClickHouse Host (e.g., xxx.region.clickhouse.cloud): " CH_HOST
    read -p "ClickHouse Port [9440]: " CH_PORT
    CH_PORT=${CH_PORT:-9440}
    read -p "ClickHouse Database [ollystack]: " CH_DATABASE
    CH_DATABASE=${CH_DATABASE:-ollystack}
    read -p "ClickHouse Username [default]: " CH_USER
    CH_USER=${CH_USER:-default}
    read -sp "ClickHouse Password: " CH_PASSWORD
    echo ""

    # Generate JWT secret
    JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)

    # Write .env file
    cat > "$DOCKER_DIR/.env" << EOF
# OllyStack MVP Configuration
# Generated on $(date)

# ClickHouse Cloud
CLICKHOUSE_HOST=$CH_HOST
CLICKHOUSE_PORT=$CH_PORT
CLICKHOUSE_DATABASE=$CH_DATABASE
CLICKHOUSE_USER=$CH_USER
CLICKHOUSE_PASSWORD=$CH_PASSWORD

# API Server
LOG_LEVEL=info
JWT_SECRET=$JWT_SECRET
EOF

    echo -e "${GREEN}✓ Environment configured${NC}"
}

# Initialize ClickHouse schema
init_clickhouse_schema() {
    echo ""
    echo -e "${YELLOW}Step 3: Initialize ClickHouse Schema${NC}"
    echo "─────────────────────────────────────"

    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    DOCKER_DIR="$(dirname "$SCRIPT_DIR")/docker"

    source "$DOCKER_DIR/.env"

    echo "Initializing database schema..."

    # Check if clickhouse-client is available, otherwise use docker
    if command -v clickhouse-client &> /dev/null; then
        clickhouse-client \
            --host "$CLICKHOUSE_HOST" \
            --port "$CLICKHOUSE_PORT" \
            --secure \
            --user "$CLICKHOUSE_USER" \
            --password "$CLICKHOUSE_PASSWORD" \
            < "$SCRIPT_DIR/init-clickhouse.sql"
    else
        echo "Using Docker to run ClickHouse client..."
        docker run --rm -i \
            -v "$SCRIPT_DIR/init-clickhouse.sql:/init.sql:ro" \
            clickhouse/clickhouse-client:latest \
            --host "$CLICKHOUSE_HOST" \
            --port "$CLICKHOUSE_PORT" \
            --secure \
            --user "$CLICKHOUSE_USER" \
            --password "$CLICKHOUSE_PASSWORD" \
            < "$SCRIPT_DIR/init-clickhouse.sql"
    fi

    echo -e "${GREEN}✓ Schema initialized${NC}"
}

# Start services
start_services() {
    echo ""
    echo -e "${YELLOW}Step 4: Start Services${NC}"
    echo "───────────────────────"

    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    DOCKER_DIR="$(dirname "$SCRIPT_DIR")/docker"

    cd "$DOCKER_DIR"

    echo "Starting OllyStack services..."
    docker compose -f docker-compose.mvp.yml up -d

    echo ""
    echo "Waiting for services to be healthy..."
    sleep 10

    docker compose -f docker-compose.mvp.yml ps

    echo -e "${GREEN}✓ Services started${NC}"
}

# Print summary
print_summary() {
    echo ""
    echo -e "${GREEN}"
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║                    Setup Complete!                            ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    echo "OllyStack is now running! Access it at:"
    echo ""
    echo "  📊 Web UI:        http://localhost:3000"
    echo "  🔌 API Server:    http://localhost:8080"
    echo "  📡 OTLP (gRPC):   localhost:4317"
    echo "  📡 OTLP (HTTP):   localhost:4318"
    echo ""
    echo "To send test data, configure your application with:"
    echo ""
    echo "  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317"
    echo "  OTEL_SERVICE_NAME=my-service"
    echo ""
    echo "Useful commands:"
    echo ""
    echo "  # View logs"
    echo "  docker compose -f docker-compose.mvp.yml logs -f"
    echo ""
    echo "  # Stop services"
    echo "  docker compose -f docker-compose.mvp.yml down"
    echo ""
    echo "  # Restart services"
    echo "  docker compose -f docker-compose.mvp.yml restart"
    echo ""
}

# Main
main() {
    check_prerequisites
    setup_clickhouse_cloud
    configure_environment
    init_clickhouse_schema
    start_services
    print_summary
}

# Run main function
main "$@"
