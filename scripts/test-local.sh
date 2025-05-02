#!/bin/bash
# OllyStack Local Test Script
# Run all tests locally before deploying

set -e

LOCAL_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; }

TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local name="$1"
    local cmd="$2"

    echo -n "Testing $name... "
    if eval "$cmd" > /tmp/test_output.txt 2>&1; then
        log_success "PASSED"
        ((TESTS_PASSED++))
    else
        log_fail "FAILED"
        cat /tmp/test_output.txt
        ((TESTS_FAILED++))
    fi
}

log_info "Running OllyStack local tests..."
echo ""

# Test 1: API Server Go Build
log_info "=== API Server Tests ==="
if [ -d "$LOCAL_DIR/api-server" ]; then
    run_test "API Go syntax check" "cd $LOCAL_DIR/api-server && go build -o /tmp/api-test ./cmd/server 2>&1"
    run_test "API Go vet" "cd $LOCAL_DIR/api-server && go vet ./... 2>&1"
    run_test "API Go tests" "cd $LOCAL_DIR/api-server && go test -short ./... 2>&1"
else
    log_error "api-server directory not found"
fi

echo ""

# Test 2: OTel Processor
log_info "=== OTel Processor Tests ==="
if [ -d "$LOCAL_DIR/otel-processor-correlation" ]; then
    run_test "Processor Go syntax" "cd $LOCAL_DIR/otel-processor-correlation && go build ./... 2>&1"
    run_test "Processor Go vet" "cd $LOCAL_DIR/otel-processor-correlation && go vet ./... 2>&1"
    run_test "Processor unit tests" "cd $LOCAL_DIR/otel-processor-correlation && go test -v ./... 2>&1"
else
    log_error "otel-processor-correlation directory not found"
fi

echo ""

# Test 3: Schema syntax
log_info "=== Schema Tests ==="
if [ -f "$LOCAL_DIR/schema/clickhouse_schema.sql" ]; then
    run_test "Schema file exists" "test -f $LOCAL_DIR/schema/clickhouse_schema.sql"
    run_test "Schema has correlation_id" "grep -q 'correlation_id' $LOCAL_DIR/schema/clickhouse_schema.sql"
    run_test "Schema has bloom_filter" "grep -q 'bloom_filter' $LOCAL_DIR/schema/clickhouse_schema.sql"
else
    log_error "Schema file not found"
fi

echo ""

# Test 4: Grafana dashboards
log_info "=== Grafana Dashboard Tests ==="
if [ -d "$LOCAL_DIR/deploy/grafana/dashboards" ]; then
    for dashboard in "$LOCAL_DIR/deploy/grafana/dashboards"/*.json; do
        if [ -f "$dashboard" ]; then
            name=$(basename "$dashboard")
            run_test "Dashboard JSON valid: $name" "python3 -m json.tool $dashboard > /dev/null 2>&1 || jq . $dashboard > /dev/null 2>&1"
        fi
    done
else
    log_error "Grafana dashboards directory not found"
fi

echo ""

# Test 5: Collector config
log_info "=== Collector Config Tests ==="
if [ -f "$LOCAL_DIR/otel-collector-custom/config.yaml" ]; then
    run_test "Collector config YAML valid" "python3 -c \"import yaml; yaml.safe_load(open('$LOCAL_DIR/otel-collector-custom/config.yaml'))\" 2>&1"
    run_test "Collector has correlation processor" "grep -q 'ollystack_correlation' $LOCAL_DIR/otel-collector-custom/config.yaml"
fi

echo ""
echo "========================================"
echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"
echo "========================================"

if [ $TESTS_FAILED -gt 0 ]; then
    log_error "Some tests failed! Fix issues before deploying."
    exit 1
else
    log_success "All tests passed! Ready to deploy."
    exit 0
fi
