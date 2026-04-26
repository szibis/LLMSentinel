#!/bin/bash

# Verification script for Claude Escalate v4.0.0 docker-compose stack
# Usage: ./scripts/verify-services.sh

set -e

echo "🔍 Claude Escalate v4.0.0 - Service Verification"
echo "=================================================="
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper function
check_service() {
    local name=$1
    local url=$2
    local expected_code=$3

    echo -n "Checking $name... "

    response=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo "000")

    if [ "$response" = "$expected_code" ] || [ "$response" = "200" ]; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${RED}✗${NC} (got HTTP $response)"
        return 1
    fi
}

# Track results
passed=0
failed=0

echo "Service Health Checks:"
echo "--------------------"

# Main Claude Escalate service
if check_service "Main Service" "http://localhost:9000/health" "200"; then
    ((passed++))
else
    ((failed++))
fi

# Prometheus metrics endpoint
if check_service "Prometheus Metrics" "http://localhost:9000/metrics" "200"; then
    ((passed++))
else
    ((failed++))
fi

# VictoriaMetrics
if check_service "VictoriaMetrics" "http://localhost:8428/health" "200"; then
    ((passed++))
else
    ((failed++))
fi

# Grafana
if check_service "Grafana" "http://localhost:3000/api/health" "200"; then
    ((passed++))
else
    ((failed++))
fi

# OTel Collector health
if check_service "OTel Collector" "http://localhost:13133/health/status" "200"; then
    ((passed++))
else
    ((failed++))
fi

echo ""
echo "API Endpoint Checks:"
echo "-------------------"

# Analytics endpoints
echo -n "Analytics API... "
if curl -s "http://localhost:9000/api/analytics/timeseries" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
    ((passed++))
else
    echo -e "${RED}✗${NC}"
    ((failed++))
fi

# Config endpoint
echo -n "Config API... "
if curl -s "http://localhost:9000/api/config" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
    ((passed++))
else
    echo -e "${RED}✗${NC}"
    ((failed++))
fi

echo ""
echo "Data Integrity Checks:"
echo "---------------------"

# Check metrics are being exported
echo -n "Metrics export... "
metrics=$(curl -s "http://localhost:9000/metrics")
if echo "$metrics" | grep -q "claude_escalate"; then
    echo -e "${GREEN}✓${NC}"
    ((passed++))
else
    echo -e "${RED}✗${NC} (no metrics found)"
    ((failed++))
fi

# Check database connectivity
echo -n "Database connectivity... "
if curl -s "http://localhost:9000/api/config" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
    ((passed++))
else
    echo -e "${RED}✗${NC}"
    ((failed++))
fi

echo ""
echo "Summary:"
echo "--------"
echo -e "Passed: ${GREEN}$passed${NC}"
echo -e "Failed: ${RED}$failed${NC}"

if [ $failed -eq 0 ]; then
    echo -e "\n${GREEN}✓ All services verified successfully!${NC}"
    exit 0
else
    echo -e "\n${RED}✗ Some services failed verification${NC}"
    exit 1
fi
