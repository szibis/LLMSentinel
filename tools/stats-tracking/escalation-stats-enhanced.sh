#!/bin/bash
# Enhanced Escalation Stats Tracker
# Provides detailed session history, token estimation, and savings analysis

set -o pipefail

DATA_DIR="${HOME}/.claude/data/escalation"
STATS_DB="${DATA_DIR}/sessions.jsonl"  # JSON Lines format (one per line)
mkdir -p "$DATA_DIR"

# =============================================================================
# Session Tracking
# =============================================================================

log_session_event() {
    local event_type="$1"  # escalate, deescalate, success
    local from_model="$2"
    local to_model="$3"
    local task_type="$4"

    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local timestamp_unix=$(date +%s)

    # Estimate tokens based on model
    local token_est=0
    case "$to_model" in
        *opus*) token_est=500 ;;
        *sonnet*) token_est=200 ;;
        *haiku*) token_est=50 ;;
    esac

    # Calculate potential savings (vs using Opus)
    local opus_equivalent=500
    local savings=$(( opus_equivalent - token_est ))

    # Write session record
    printf '{"timestamp":"%s","epoch":%d,"type":"%s","from":"%s","to":"%s","task":"%s","tokens":%d,"savings":%d}\n' \
        "$timestamp" "$timestamp_unix" "$event_type" "$from_model" "$to_model" "$task_type" "$token_est" "$savings" >> "$STATS_DB"
}

get_session_history() {
    local limit="${1:-50}"

    if [ ! -f "$STATS_DB" ]; then
        echo "[]"
        return
    fi

    # Read and format as JSON array
    echo "["
    tail -"$limit" "$STATS_DB" | head -$((limit - 1)) | while read -r line; do
        echo "$line,"
    done
    tail -1 "$STATS_DB"
    echo "]"
}

get_session_summary() {
    if [ ! -f "$STATS_DB" ]; then
        cat << 'JSON'
{
  "totalSessions": 0,
  "totalEscalations": 0,
  "totalCascades": 0,
  "totalTokensSaved": 0,
  "avgSessionDuration": 0,
  "modelBreakdown": {"opus": 0, "sonnet": 0, "haiku": 0},
  "successRate": 0
}
JSON
        return
    fi

    local escalations=$(grep -c '"type":"escalate"' "$STATS_DB" 2>/dev/null || echo 0)
    local deescalations=$(grep -c '"type":"deescalate"' "$STATS_DB" 2>/dev/null || echo 0)
    local total_saved=$(grep -oP '"savings":\K[0-9]+' "$STATS_DB" | awk '{s+=$1} END {print s}' || echo 0)

    local opus_count=$(grep -c '"to":".*opus"' "$STATS_DB" 2>/dev/null || echo 0)
    local sonnet_count=$(grep -c '"to":".*sonnet"' "$STATS_DB" 2>/dev/null || echo 0)
    local haiku_count=$(grep -c '"to":".*haiku"' "$STATS_DB" 2>/dev/null || echo 0)

    local success_rate=$([ "$escalations" -eq 0 ] && echo 0 || echo $(( (deescalations * 100) / escalations )))

    cat << JSON
{
  "totalSessions": $escalations,
  "totalEscalations": $escalations,
  "totalCascades": $deescalations,
  "totalTokensSaved": $total_saved,
  "avgSessionDuration": 0,
  "modelBreakdown": {
    "opus": $opus_count,
    "sonnet": $sonnet_count,
    "haiku": $haiku_count
  },
  "successRate": $success_rate
}
JSON
}

get_cost_analysis() {
    if [ ! -f "$STATS_DB" ]; then
        cat << 'JSON'
{
  "actualTokensCost": 0,
  "estimatedWithoutEscalation": 0,
  "costSaved": 0,
  "costSavedPercent": 0,
  "avgCostPerSession": 0,
  "tokenBreakdown": {
    "opus": {"count": 0, "tokens": 0},
    "sonnet": {"count": 0, "tokens": 0},
    "haiku": {"count": 0, "tokens": 0}
  }
}
JSON
        return
    fi

    # Calculate totals
    local opus_count=$(grep -c '"to":".*opus"' "$STATS_DB" 2>/dev/null || echo 0)
    local sonnet_count=$(grep -c '"to":".*sonnet"' "$STATS_DB" 2>/dev/null || echo 0)
    local haiku_count=$(grep -c '"to":".*haiku"' "$STATS_DB" 2>/dev/null || echo 0)

    local opus_tokens=$(( opus_count * 500 ))
    local sonnet_tokens=$(( sonnet_count * 200 ))
    local haiku_tokens=$(( haiku_count * 50 ))
    local actual_total=$(( opus_tokens + sonnet_tokens + haiku_tokens ))

    # Without escalation: everything would be Opus (500 tokens each)
    local total_sessions=$(( opus_count + sonnet_count + haiku_count ))
    local without_escalation=$(( total_sessions * 500 ))
    local cost_saved=$(( without_escalation - actual_total ))
    local cost_saved_percent=$([ "$without_escalation" -eq 0 ] && echo 0 || echo $(( (cost_saved * 100) / without_escalation )))
    local avg_cost=$([ "$total_sessions" -eq 0 ] && echo 0 || echo $(( actual_total / total_sessions )))

    cat << JSON
{
  "actualTokensCost": $actual_total,
  "estimatedWithoutEscalation": $without_escalation,
  "costSaved": $cost_saved,
  "costSavedPercent": $cost_saved_percent,
  "avgCostPerSession": $avg_cost,
  "tokenBreakdown": {
    "opus": {"count": $opus_count, "tokens": $opus_tokens},
    "sonnet": {"count": $sonnet_count, "tokens": $sonnet_tokens},
    "haiku": {"count": $haiku_count, "tokens": $haiku_tokens}
  }
}
JSON
}

# Main commands
case "${1:-}" in
    log)
        log_session_event "$2" "$3" "$4" "$5"
        ;;
    history)
        get_session_history "$2"
        ;;
    summary)
        get_session_summary
        ;;
    cost)
        get_cost_analysis
        ;;
    *)
        echo "Usage: $0 {log|history|summary|cost}"
        exit 1
        ;;
esac
