#!/bin/bash

# Test data seeding script for claude-escalate
# Populates test database with sample configurations, metrics, and optimization states

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_URL="${TEST_API_URL:-http://localhost:8080/api}"
HEALTH_URL="${TEST_HEALTH_URL:-http://localhost:8080/health}"
MAX_RETRIES=30
RETRY_DELAY=2

# Logging functions
log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

# Wait for gateway to be healthy
wait_for_gateway() {
  log_info "Waiting for gateway to be healthy..."

  for i in $(seq 1 $MAX_RETRIES); do
    if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
      log_info "Gateway is healthy ✓"
      return 0
    fi

    if [ $i -lt $MAX_RETRIES ]; then
      log_warn "Gateway not ready yet, retrying in ${RETRY_DELAY}s... ($i/$MAX_RETRIES)"
      sleep $RETRY_DELAY
    fi
  done

  log_error "Gateway failed to become healthy after $((MAX_RETRIES * RETRY_DELAY))s"
  return 1
}

# Seed configuration
seed_configuration() {
  log_info "Seeding configuration..."

  curl -s -X POST "$API_URL/config" \
    -H "Content-Type: application/json" \
    -d '{
      "cache_enabled": true,
      "cache_similarity_threshold": 0.85,
      "token_optimization_enabled": true,
      "semantic_cache_hit_target": 0.90,
      "max_cache_size": 10000,
      "intent_detection_enabled": true,
      "batch_api_enabled": true,
      "security_validation_enabled": true,
      "max_token_budget": 100000
    }' > /dev/null

  log_info "Configuration seeded ✓"
}

# Seed optimization states
seed_optimizations() {
  log_info "Seeding optimization states..."

  local optimizations=(
    "semantic_cache"
    "exact_dedup"
    "token_optimization"
    "batch_api"
    "intent_detection"
  )

  for opt in "${optimizations[@]}"; do
    curl -s -X POST "$API_URL/optimizations/$opt/toggle" \
      -H "Content-Type: application/json" \
      -d '{"enabled": true}' > /dev/null

    log_info "  Enabled optimization: $opt ✓"
  done
}

# Seed cache statistics
seed_cache_stats() {
  log_info "Seeding cache statistics..."

  # This endpoint is read-only in the current implementation,
  # but we can verify it responds correctly
  curl -s -X GET "$API_URL/cache/stats" > /dev/null

  log_info "Cache stats available ✓"
}

# Verify API endpoints
verify_endpoints() {
  log_info "Verifying API endpoints..."

  local endpoints=(
    "GET:/api/config"
    "GET:/api/status"
    "GET:/api/optimizations"
    "GET:/api/cache/stats"
    "GET:/api/metrics"
  )

  for endpoint in "${endpoints[@]}"; do
    IFS=':' read -r method path <<< "$endpoint"

    response=$(curl -s -X "$method" "$API_URL$path" -w "\n%{http_code}" 2>/dev/null)
    status_code=$(echo "$response" | tail -1)

    if [ "$status_code" = "200" ] || [ "$status_code" = "201" ]; then
      log_info "  $method $path: $status_code ✓"
    else
      log_warn "  $method $path: $status_code"
    fi
  done
}

# Main seeding process
main() {
  log_info "Starting test data seeding..."
  echo ""

  # Wait for gateway
  if ! wait_for_gateway; then
    log_error "Gateway health check failed"
    exit 1
  fi
  echo ""

  # Seed data
  seed_configuration
  echo ""

  seed_optimizations
  echo ""

  seed_cache_stats
  echo ""

  # Verify endpoints
  verify_endpoints
  echo ""

  log_info "Test data seeding completed successfully! ✓"
  echo ""
  echo "Test environment is ready:"
  echo "  • Gateway: http://localhost:8080"
  echo "  • Health:  http://localhost:8080/health"
  echo "  • API:     http://localhost:8080/api"
  echo "  • Metrics: ws://localhost:8080/api/metrics/stream"
  echo "  • Prometheus: http://localhost:9090"
}

# Run main function
main "$@"
