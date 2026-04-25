# shellcheck shell=bash
# =============================================================================
# Escalation Status Module - Shows real-time model & effort from escalation-manager
# =============================================================================
# Displays current model (Haiku/Sonnet/Opus) and effort level (low/med/high)
# with cost indicator and cascade status
#
# Configuration options:
#   ESCALATION_ICON     - Icon (default: 🚀)
#   ESCALATION_COMPACT  - Compact mode (default: false)
# =============================================================================

module_escalation_status() {
    local icon=$(get_icon "${ESCALATION_ICON:-🚀}" "ESC:")
    local compact="${ESCALATION_COMPACT:-false}"

    # Try to get stats from escalation-manager binary
    local stats_json=""
    local escalation_binary="${HOME}/.claude/bin/escalation-manager"

    if [ -x "$escalation_binary" ]; then
        stats_json=$("$escalation_binary" stats 2>/dev/null)
    fi

    # Parse stats or use defaults
    local model_name="Sonnet"
    local full_model="claude-sonnet-4-6"
    local effort="medium"
    local model_color="🟠"  # Orange
    local cost_multiplier="8x"

    if [ -n "$stats_json" ]; then
        model_name=$(echo "$stats_json" | jq -r '.currentState.model // "Sonnet"' 2>/dev/null)
        full_model=$(echo "$stats_json" | jq -r '.currentState.fullModel // "claude-sonnet-4-6"' 2>/dev/null)
        effort=$(echo "$stats_json" | jq -r '.currentState.effort // "MEDIUM"' 2>/dev/null | tr '[:upper:]' '[:lower:]')
        cost_multiplier=$(echo "$stats_json" | jq -r '.currentState.modelCost // "8x"' 2>/dev/null)
    else
        # Fallback to reading from settings.json
        local settings_file="${HOME}/.claude/settings.json"
        if [ -f "$settings_file" ]; then
            full_model=$(jq -r '.model // "claude-sonnet-4-6"' "$settings_file" 2>/dev/null)
            effort=$(jq -r '.effortLevel // "medium"' "$settings_file" 2>/dev/null)

            # Determine display name and cost from model
            if [[ "$full_model" == *"opus"* ]]; then
                model_name="Opus"
                cost_multiplier="30x"
                model_color="🔴"  # Red
            elif [[ "$full_model" == *"sonnet"* ]]; then
                model_name="Sonnet"
                cost_multiplier="8x"
                model_color="🟠"  # Orange
            elif [[ "$full_model" == *"haiku"* ]]; then
                model_name="Haiku"
                cost_multiplier="1x"
                model_color="🟢"  # Green
            fi
        fi
    fi

    # Effort emoji and status
    local effort_emoji=""
    local effort_status=""
    case "$effort" in
        low)
            effort_emoji="⚡"
            effort_status=$(get_status 0 100 30)  # Green
            ;;
        medium)
            effort_emoji="⚙"
            effort_status=$(get_status 40 60 30)  # Yellow-green
            ;;
        high)
            effort_emoji="🔥"
            effort_status=$(get_status 60 40 30)  # Yellow-red
            ;;
        xhigh)
            effort_emoji="💎"
            effort_status=$(get_status 80 30 30)  # Red
            ;;
        *)
            effort_emoji="?"
            effort_status=""
            ;;
    esac

    # Check for active session (in de-escalation context)
    local session_indicator=""
    local session_dir="/tmp/.escalation_$(id -u)"
    if [ -f "$session_dir/escalation_session" ]; then
        local elapsed=$(( $(date +%s) - $(cat "$session_dir/escalation_session" 2>/dev/null || echo 0) ))
        local elapsed_min=$(( elapsed / 60 ))
        if [ "$elapsed_min" -lt 30 ]; then
            session_indicator=" ⏱${elapsed_min}m"
        fi
    fi

    # Build output
    local result=""
    if [ "$compact" = "true" ]; then
        # Compact: 🚀 Haiku(1x) ⚡ med
        result="${icon} ${model_color}${model_name}(${cost_multiplier}) ${effort_emoji}${effort}${session_indicator}${effort_status}"
    else
        # Full: 🚀 Escalation: Haiku(1x cost) • Effort: med ⚡
        result="${icon} $(bold Escalation:) ${model_color}${model_name}(${cost_multiplier}) • $(bold Effort:) ${effort_emoji}${effort}${session_indicator}${effort_status}"
    fi

    echo "$result"
}
