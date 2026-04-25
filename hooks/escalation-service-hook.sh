#!/bin/bash
# =============================================================================
# Escalation Service Hook - Communicates with remote Docker service
# =============================================================================
# This hook sends escalation/de-escalation requests to a remote service
# instead of using the local binary. Enables distributed architecture.
#
# Configuration:
#   ESCALATION_SERVICE_URL - Default: http://localhost:9000
#   ESCALATION_SERVICE_TIMEOUT - Default: 5 seconds
# =============================================================================

set -o pipefail

SERVICE_URL="${ESCALATION_SERVICE_URL:-http://localhost:9000}"
TIMEOUT="${ESCALATION_SERVICE_TIMEOUT:-5}"

# Fallback to local binary if service unavailable
FALLBACK_BINARY="${HOME}/.local/bin/escalation-manager"

# =============================================================================
# Service API Wrappers
# =============================================================================

call_service() {
    local endpoint="$1"
    local data="$2"

    if [ -z "$data" ]; then
        curl -s -m "$TIMEOUT" "$SERVICE_URL$endpoint" 2>/dev/null
    else
        curl -s -m "$TIMEOUT" -X POST -H "Content-Type: application/json" \
            -d "$data" "$SERVICE_URL$endpoint" 2>/dev/null
    fi
}

health_check() {
    local response=$(curl -s -m 2 "$SERVICE_URL/health" 2>/dev/null)
    [ -n "$response" ] && [ "$(echo "$response" | jq -r '.status' 2>/dev/null)" = "healthy" ]
}

handle_escalate() {
    local target="$1"

    if health_check; then
        call_service "/api/escalate" "$(jq -n --arg target "$target" '{target: $target}')"
    else
        # Fallback to local binary
        "$FALLBACK_BINARY" escalate ${target:+--to "$target"}
    fi
}

handle_deescalate() {
    local reason="$1"

    if health_check; then
        call_service "/api/deescalate" "$(jq -n --arg reason "$reason" '{reason: $reason}')"
    else
        "$FALLBACK_BINARY" deescalate ${reason:+--reason "$reason"}
    fi
}

handle_effort() {
    local level="$1"

    if health_check; then
        call_service "/api/effort" "$(jq -n --arg level "$level" '{level: $level}')"
    else
        "$FALLBACK_BINARY" effort --set "$level"
    fi
}

get_status() {
    if health_check; then
        call_service "/api/status"
    else
        "$FALLBACK_BINARY" stats --format json
    fi
}

# =============================================================================
# Main Hook Handler
# =============================================================================

main() {
    local hook_type="$1"
    local user_prompt="$2"

    # Parse commands from user prompt
    case "$user_prompt" in
        /escalate*)
            local target=$(echo "$user_prompt" | grep -oP '/escalate\s+to\s+\K\w+' || echo "sonnet")
            handle_escalate "$target"
            ;;
        *)
            # Check for success signals
            if echo "$user_prompt" | grep -qi "works\|thanks\|perfect\|got it\|solved"; then
                handle_deescalate "success_signal"
            fi
            ;;
    esac
}

# =============================================================================
# CLI Interface
# =============================================================================

case "${1:-}" in
    escalate)
        handle_escalate "${2:-sonnet}"
        ;;
    deescalate)
        handle_deescalate "${2:-}"
        ;;
    effort)
        handle_effort "${2:-}"
        ;;
    status)
        get_status
        ;;
    health)
        if health_check; then
            echo '{"status":"healthy","url":"'"$SERVICE_URL"'"}'
        else
            echo '{"status":"unhealthy","url":"'"$SERVICE_URL"'"}'
        fi
        ;;
    *)
        echo "Usage: $0 {escalate|deescalate|effort|status|health}"
        exit 1
        ;;
esac
