#!/bin/bash
# =============================================================================
# De-escalation Hook - Auto-downgrades when problems are solved
# Phase 3: Cost optimization after problem resolution
#
# Triggers on success signals when on expensive models.
# Supports both /escalate sessions AND auto-effort-routed sessions.
# Cascade-aware: Opus→Sonnet→Haiku across separate confirmations.
# =============================================================================

INPUT=$(cat)
PROMPT=$(echo "$INPUT" | jq -r '.prompt // ""' 2>/dev/null)

[ -z "$PROMPT" ] && echo '{"continue":true,"suppressOutput":true}' && exit 0

# Skip meta-commands — let escalate-model.sh handle those
echo "$PROMPT" | grep -qE "^/(escalate|effort|model)" && \
    echo '{"continue":true,"suppressOutput":true}' && exit 0

SETTINGS_FILE="${HOME}/.claude/settings.json"
CURRENT_MODEL=$(jq -r '.model // "unknown"' "$SETTINGS_FILE" 2>/dev/null)
SESSION_DIR="/tmp/.escalation_$(id -u)"
DATA_DIR="${HOME}/.claude/data/escalation"
mkdir -p "$SESSION_DIR" "$DATA_DIR"

# =============================================================================
# Success Signal Detection — word-boundary aware to avoid false positives
# =============================================================================

detect_success_signal() {
    local prompt_lower
    prompt_lower=$(echo "$1" | tr '[:upper:]' '[:lower:]')

    # Multi-word phrases first (more specific, fewer false positives)
    local phrase_matches=0
    for phrase in "works great" "works perfectly" "working now" "got it working" \
                  "that fixed it" "that works" "that solved" "issue resolved" \
                  "problem solved" "thank you" "thanks for" "thanks a lot" \
                  "that's it" "that's exactly" "no longer broken" "no longer failing" \
                  "all good" "looks good" "ship it" \
                  "exactly what i needed" "works like a charm" "problem solved for good" \
                  "no more errors" "successfully implemented" "got it working"; do
        echo "$prompt_lower" | grep -q "$phrase" && ((phrase_matches++))
    done

    # Strong single-word signals (with context guards)
    local word_matches=0
    # "perfect" alone is strong
    echo "$prompt_lower" | grep -qw "perfect" && ((word_matches++))
    # "solved" alone is strong
    echo "$prompt_lower" | grep -qw "solved" && ((word_matches++))
    # "fixed" alone is strong (but not "need to fix" or "should fix")
    echo "$prompt_lower" | grep -qE "(that |it |is |it's |got )fixed" && ((word_matches++))
    # "works" only when it's a confirmation, not a question
    echo "$prompt_lower" | grep -qE "(it|that|this) works" && ((word_matches++))
    # "appreciate" or "appreciate it"
    echo "$prompt_lower" | grep -qE "appreciat(e|ed)" && ((word_matches++))
    # "thanks" standalone (not "thanks but" or "thanks, however")
    if echo "$prompt_lower" | grep -qw "thanks"; then
        # Filter out "thanks but" patterns
        echo "$prompt_lower" | grep -qE "thanks.*(but|however|although|yet|still)" || ((word_matches++))
    fi

    # Need at least 1 phrase match OR 1 word match
    [ "$phrase_matches" -ge 1 ] || [ "$word_matches" -ge 1 ] && return 0
    return 1
}

# =============================================================================
# Session Tracking — supports both /escalate and auto-effort sessions
# =============================================================================

is_on_expensive_model() {
    [[ "$CURRENT_MODEL" == *"opus"* ]] || [[ "$CURRENT_MODEL" == *"sonnet"* ]]
}

# Check if there's a reason to de-escalate (active /escalate session ONLY)
# STRICT: Only /escalate commands create escalation context, not auto-effort
has_escalation_context() {
    local session_file="$SESSION_DIR/escalation_session"

    # ONLY Check 1: Explicit /escalate session (within 30 min)
    # Removed: Check 2 (auto-effort routing) - too permissive, caused false de-escalations
    if [ -f "$session_file" ]; then
        local escalated_time
        escalated_time=$(cat "$session_file" 2>/dev/null)

        # Verify timestamp is valid (must be a number)
        if ! [[ "$escalated_time" =~ ^[0-9]+$ ]]; then
            rm -f "$session_file"  # Corrupted, clear it
            return 1
        fi

        local elapsed=$(( $(date +%s) - escalated_time ))

        # Within 30-minute window
        if [ "$elapsed" -lt 1800 ]; then
            return 0
        else
            # Session expired, clean up
            rm -f "$session_file"
            return 1
        fi
    fi

    return 1
}

# Mark de-escalation (step down one tier, keep session for cascade)
step_down_model() {
    local session_file="$SESSION_DIR/escalation_session"

    if [[ "$CURRENT_MODEL" == *"opus"* ]]; then
        # Opus → Sonnet: keep session alive for possible Sonnet→Haiku cascade
        echo "claude-sonnet-4-6"
        date +%s > "$session_file"  # refresh session for cascade
        return 0
    fi

    if [[ "$CURRENT_MODEL" == *"sonnet"* ]]; then
        # Sonnet → Haiku: clear session (bottom of chain)
        echo "claude-haiku-4-5-20251001"
        rm -f "$session_file"
        return 0
    fi

    echo "$CURRENT_MODEL"
    return 1
}

# =============================================================================
# Main Logic
# =============================================================================

# Only act if on expensive model + success signal + escalation context
if is_on_expensive_model && detect_success_signal "$PROMPT" && has_escalation_context; then
    TARGET_MODEL=$(step_down_model)

    if [ "$TARGET_MODEL" != "$CURRENT_MODEL" ]; then
        # Determine cascade state for better messaging
        local cascade_status=""
        if [[ "$CURRENT_MODEL" == *"opus"* ]]; then
            cascade_status=" (continuing cascade)"
        elif [[ "$CURRENT_MODEL" == *"sonnet"* ]]; then
            cascade_status=" (cascade complete)"
        fi

        # Set effort level
        if [[ "$TARGET_MODEL" == *"haiku"* ]]; then
            NEW_EFFORT="low"
            LABEL="Haiku (cost-optimized)$cascade_status"
        elif [[ "$TARGET_MODEL" == *"sonnet"* ]]; then
            NEW_EFFORT="medium"
            LABEL="Sonnet (balanced)$cascade_status"
        else
            NEW_EFFORT="high"
            LABEL="$TARGET_MODEL$cascade_status"
        fi

        # Update settings atomically
        TMPFILE=$(mktemp)
        jq --arg model "$TARGET_MODEL" --arg effort "$NEW_EFFORT" \
            '.model = $model | .effortLevel = $effort' \
            "$SETTINGS_FILE" > "$TMPFILE" 2>/dev/null

        if [ $? -eq 0 ] && [ -s "$TMPFILE" ]; then
            mv "$TMPFILE" "$SETTINGS_FILE"
            # Signal auto-effort to skip this cycle (prevent override)
            date +%s > "$SESSION_DIR/deescalation_just_ran"
            echo "{\"continue\":true,\"suppressOutput\":true,\"hookSpecificOutput\":{\"hookEventName\":\"UserPromptSubmit\",\"additionalContext\":\"⬇️ Auto-downgrade: $LABEL (problem solved, saving cost)\"}}"
            exit 0
        fi
        rm -f "$TMPFILE"
    fi
fi

echo '{"continue":true,"suppressOutput":true}'
