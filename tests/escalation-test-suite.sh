#!/bin/bash
# =============================================================================
# COMPREHENSIVE ESCALATION HOOK TEST SUITE
# Tests all promises and features of escalate-model.sh and de-escalate-model.sh
# =============================================================================

set -o pipefail

# Test infrastructure
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TEST_LOG="/tmp/escalation-tests.log"
SETTINGS_BACKUP="/tmp/escalation-test-settings-backup.json"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test utilities
echo_test() { echo -e "${BLUE}[TEST]${NC} $1"; }
echo_pass() { echo -e "${GREEN}[PASS]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED+1)); }
echo_fail() { echo -e "${RED}[FAIL]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED+1)); }
echo_skip() { echo -e "${YELLOW}[SKIP]${NC} $1"; }

# Setup/Teardown
setup_test() {
    TESTS_RUN=$((TESTS_RUN+1))
    SETTINGS_FILE="${HOME}/.claude/settings.json"
    cp "$SETTINGS_FILE" "$SETTINGS_BACKUP" 2>/dev/null || true
    rm -rf /tmp/.escalation_$(id -u) /tmp/.response_analysis_$(id -u)
}

teardown_test() {
    [ -f "$SETTINGS_BACKUP" ] && cp "$SETTINGS_BACKUP" "$SETTINGS_FILE" 2>/dev/null || true
}

cleanup() {
    rm -f "$SETTINGS_BACKUP"
    rm -rf /tmp/.escalation_$(id -u) /tmp/.response_analysis_$(id -u)
}

# Assertion utilities
assert_model() {
    local expected="$1"
    local actual=$(jq -r '.model // empty' "$SETTINGS_FILE" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        echo_pass "Model is $expected"
    else
        echo_fail "Expected model $expected, got $actual"
    fi
}

assert_effort() {
    local expected="$1"
    local actual=$(jq -r '.effortLevel // empty' "$SETTINGS_FILE" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        echo_pass "Effort is $expected"
    else
        echo_fail "Expected effort $expected, got $actual"
    fi
}

assert_session_exists() {
    if [ -f "/tmp/.escalation_$(id -u)/escalation_session" ]; then
        echo_pass "Escalation session created"
    else
        echo_fail "Escalation session not found"
    fi
}

assert_session_cleared() {
    if [ ! -f "/tmp/.escalation_$(id -u)/escalation_session" ]; then
        echo_pass "Escalation session cleared"
    else
        echo_fail "Escalation session still exists"
    fi
}

# Hook test executor
test_escalate_hook() {
    local prompt="$1"
    local hook_script="${HOME}/.claude/hooks/escalate-model.sh"

    if [ ! -f "$hook_script" ]; then
        echo_skip "escalate-model.sh not found"
        return 1
    fi

    echo "{\"prompt\":\"$prompt\"}" | bash "$hook_script" > /dev/null 2>&1
}

test_deescalate_hook() {
    local prompt="$1"
    local hook_script="${HOME}/.claude/hooks/de-escalate-model.sh"

    if [ ! -f "$hook_script" ]; then
        echo_skip "de-escalate-model.sh not found"
        return 1
    fi

    echo "{\"prompt\":\"$prompt\"}" | bash "$hook_script" > /dev/null 2>&1
}

# =============================================================================
# TEST SUITE 1: ESCALATE COMMAND PARSING
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 1: ESCALATE COMMAND PARSING ===${NC}"

test_escalate_to_sonnet() {
    echo_test "Escalate to Sonnet (default)"
    setup_test

    test_escalate_hook "/escalate"
    assert_model "claude-sonnet-4-6"
    assert_effort "high"
    assert_session_exists

    teardown_test
}

test_escalate_explicit_sonnet() {
    echo_test "Escalate to Sonnet (explicit)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    assert_model "claude-sonnet-4-6"
    assert_effort "high"

    teardown_test
}

test_escalate_to_opus() {
    echo_test "Escalate to Opus"
    setup_test

    test_escalate_hook "/escalate to opus"
    assert_model "claude-opus-4-6"
    assert_effort "high"

    teardown_test
}

test_escalate_to_haiku() {
    echo_test "Escalate to Haiku (downgrade)"
    setup_test

    # First set to Sonnet
    test_escalate_hook "/escalate to sonnet"
    # Then downgrade to Haiku
    test_escalate_hook "/escalate to haiku"
    assert_model "claude-haiku-4-5-20251001"
    assert_effort "low"

    teardown_test
}

test_escalate_case_insensitive() {
    echo_test "Escalate command case-insensitive"
    setup_test

    test_escalate_hook "/ESCALATE to SONNET"
    assert_model "claude-sonnet-4-6"

    teardown_test
}

# =============================================================================
# TEST SUITE 2: MODEL MAPPING
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 2: MODEL MAPPING ===${NC}"

test_model_id_opus() {
    echo_test "Model ID mapping: Opus"
    setup_test

    test_escalate_hook "/escalate to opus"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-opus-4-6" ]; then
        echo_pass "Opus mapped to claude-opus-4-6"
    else
        echo_fail "Opus not mapped correctly: $model"
    fi

    teardown_test
}

test_model_id_sonnet() {
    echo_test "Model ID mapping: Sonnet"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-sonnet-4-6" ]; then
        echo_pass "Sonnet mapped to claude-sonnet-4-6"
    else
        echo_fail "Sonnet not mapped correctly: $model"
    fi

    teardown_test
}

test_model_id_haiku() {
    echo_test "Model ID mapping: Haiku"
    setup_test

    test_escalate_hook "/escalate to haiku"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-haiku-4-5-20251001" ]; then
        echo_pass "Haiku mapped to claude-haiku-4-5-20251001"
    else
        echo_fail "Haiku not mapped correctly: $model"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 3: DE-ESCALATION SUCCESS SIGNALS
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 3: DE-ESCALATION SUCCESS SIGNALS ===${NC}"

test_deescalate_phrase_works_great() {
    echo_test "De-escalate on 'works great'"
    setup_test

    # Set to Sonnet first
    test_escalate_hook "/escalate to sonnet"
    sleep 1

    # Test de-escalation
    test_deescalate_hook "Thanks! That works great."

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    if [ "$model" = "claude-haiku-4-5-20251001" ]; then
        echo_pass "De-escalated to Haiku on 'works great'"
    else
        echo_fail "Failed to de-escalate on 'works great': $model"
    fi

    teardown_test
}

test_deescalate_phrase_thanks() {
    echo_test "De-escalate on 'thanks for'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    test_deescalate_hook "Thanks for the help!"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    if [ "$model" = "claude-haiku-4-5-20251001" ]; then
        echo_pass "De-escalated on 'thanks for'"
    else
        echo_fail "Failed to de-escalate: $model"
    fi

    teardown_test
}

test_deescalate_phrase_that_fixed() {
    echo_test "De-escalate on 'that fixed it'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    test_deescalate_hook "Perfect! That fixed it."

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_perfect() {
    echo_test "De-escalate on 'perfect'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    test_deescalate_hook "That's perfect."

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_solved() {
    echo_test "De-escalate on 'solved'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    test_deescalate_hook "The issue is solved!"

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_fixed() {
    echo_test "De-escalate on 'fixed'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    test_deescalate_hook "That is fixed now."

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

# =============================================================================
# TEST SUITE 4: DE-ESCALATION FALSE POSITIVE GUARDS
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 4: DE-ESCALATION FALSE POSITIVE GUARDS ===${NC}"

test_no_deescalate_thanks_but() {
    echo_test "Don't de-escalate on 'thanks but' (guard)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    local model_before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "Thanks but it still doesn't work."
    local model_after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model_before" = "$model_after" ]; then
        echo_pass "Correctly ignored 'thanks but' (false positive guard)"
    else
        echo_fail "Incorrectly de-escalated on 'thanks but'"
    fi

    teardown_test
}

test_no_deescalate_thanks_however() {
    echo_test "Don't de-escalate on 'thanks however' (guard)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    local model_before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "Thanks, however we still have issues."
    local model_after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model_before" = "$model_after" ]; then
        echo_pass "Correctly ignored 'thanks however'"
    else
        echo_fail "Incorrectly de-escalated on 'thanks however'"
    fi

    teardown_test
}

test_no_deescalate_fixed_needed() {
    echo_test "Don't de-escalate on 'need to fix' (guard)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1

    local model_before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "We still need to fix this part."
    local model_after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model_before" = "$model_after" ]; then
        echo_pass "Correctly ignored 'need to fix'"
    else
        echo_fail "Incorrectly de-escalated on 'need to fix'"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 5: CASCADE DE-ESCALATION (Opus → Sonnet → Haiku)
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 5: CASCADE DE-ESCALATION ===${NC}"

test_cascade_opus_to_sonnet() {
    echo_test "Cascade: Opus → Sonnet"
    setup_test

    test_escalate_hook "/escalate to opus"
    sleep 1

    test_deescalate_hook "Thanks! Works great!"
    assert_model "claude-sonnet-4-6"

    teardown_test
}

test_cascade_opus_to_sonnet_to_haiku() {
    echo_test "Cascade: Opus → Sonnet → Haiku"
    setup_test

    test_escalate_hook "/escalate to opus"
    sleep 1

    # First de-escalation: Opus → Sonnet
    test_deescalate_hook "Good, that works!"
    sleep 1
    assert_model "claude-sonnet-4-6"

    # Second de-escalation: Sonnet → Haiku
    test_deescalate_hook "Perfect! All solved!"
    sleep 1
    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

# =============================================================================
# TEST SUITE 6: SESSION TRACKING (30-MINUTE WINDOW)
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 6: SESSION TRACKING ===${NC}"

test_session_created_on_escalate() {
    echo_test "Session created on escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    if [ -f "/tmp/.escalation_$(id -u)/escalation_session" ]; then
        echo_pass "Session file created"
    else
        echo_fail "Session file not created"
    fi

    teardown_test
}

test_session_cleared_on_deescalate() {
    echo_test "Session cleared after de-escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1
    test_deescalate_hook "Thanks! Works perfectly!"

    assert_session_cleared

    teardown_test
}

test_session_timestamp_valid() {
    echo_test "Session timestamp is valid"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    local session_time=$(cat "/tmp/.escalation_$(id -u)/escalation_session" 2>/dev/null)
    local current_time=$(date +%s)
    local diff=$((current_time - session_time))

    if [ "$diff" -lt 5 ]; then
        echo_pass "Session timestamp within 5 seconds of current time"
    else
        echo_fail "Session timestamp difference too large: $diff seconds"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 7: METADATA CONSISTENCY
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 7: METADATA CONSISTENCY ===${NC}"

test_opus_effort_high() {
    echo_test "Opus always has effort=high"
    setup_test

    test_escalate_hook "/escalate to opus"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [[ "$model" == *"opus"* ]] && [ "$effort" = "high" ]; then
        echo_pass "Opus model has effort=high"
    else
        echo_fail "Opus effort mismatch: model=$model, effort=$effort"
    fi

    teardown_test
}

test_sonnet_effort_high() {
    echo_test "Sonnet always has effort=high when escalated"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [[ "$model" == *"sonnet"* ]] && [ "$effort" = "high" ]; then
        echo_pass "Sonnet model has effort=high"
    else
        echo_fail "Sonnet effort mismatch: model=$model, effort=$effort"
    fi

    teardown_test
}

test_haiku_effort_low() {
    echo_test "Haiku always has effort=low"
    setup_test

    test_escalate_hook "/escalate to haiku"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [[ "$model" == *"haiku"* ]] && [ "$effort" = "low" ]; then
        echo_pass "Haiku model has effort=low"
    else
        echo_fail "Haiku effort mismatch: model=$model, effort=$effort"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 8: SETTINGS.JSON ATOMICITY
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 8: SETTINGS.JSON ATOMICITY ===${NC}"

test_settings_valid_json_after_escalate() {
    echo_test "settings.json remains valid JSON after escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    if jq . "$SETTINGS_FILE" > /dev/null 2>&1; then
        echo_pass "settings.json is valid JSON"
    else
        echo_fail "settings.json is invalid JSON after escalation"
    fi

    teardown_test
}

test_settings_valid_json_after_deescalate() {
    echo_test "settings.json remains valid JSON after de-escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1
    test_deescalate_hook "Works perfectly!"

    if jq . "$SETTINGS_FILE" > /dev/null 2>&1; then
        echo_pass "settings.json is valid JSON after de-escalation"
    else
        echo_fail "settings.json is invalid JSON after de-escalation"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 9: NO-OP BEHAVIOR
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 9: NO-OP BEHAVIOR ===${NC}"

test_non_escalate_prompt_unchanged() {
    echo_test "Non-escalate prompt doesn't change settings"
    setup_test

    local before=$(jq '.model' "$SETTINGS_FILE")

    test_escalate_hook "regular prompt without /escalate command"

    local after=$(jq '.model' "$SETTINGS_FILE")

    if [ "$before" = "$after" ]; then
        echo_pass "Settings unchanged on non-escalate prompt"
    else
        echo_fail "Settings changed unexpectedly: before=$before, after=$after"
    fi

    teardown_test
}

test_escalate_without_session_no_deescalate() {
    echo_test "De-escalate requires active escalation session"
    setup_test

    # Don't escalate, just try to de-escalate
    local before=$(jq '.model' "$SETTINGS_FILE")
    test_deescalate_hook "Thanks! Works great!"
    local after=$(jq '.model' "$SETTINGS_FILE")

    if [ "$before" = "$after" ]; then
        echo_pass "De-escalate correctly blocked without escalation session"
    else
        echo_fail "De-escalate incorrectly triggered without session"
    fi

    teardown_test
}

# =============================================================================
# TEST SUITE 10: EFFORT LEVEL TRANSITIONS
# =============================================================================

echo -e "\n${BLUE}=== TEST SUITE 10: EFFORT LEVEL TRANSITIONS ===${NC}"

test_effort_transition_low_to_high() {
    echo_test "Effort transitions: low → high (escalation)"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [ "$effort" = "high" ]; then
        echo_pass "Effort transitioned to high on escalation"
    else
        echo_fail "Effort not updated: $effort"
    fi

    teardown_test
}

test_effort_transition_high_to_low() {
    echo_test "Effort transitions: high → low (de-escalation)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 1
    test_deescalate_hook "Perfect!"

    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [ "$effort" = "low" ]; then
        echo_pass "Effort transitioned to low on de-escalation"
    else
        echo_fail "Effort not updated: $effort"
    fi

    teardown_test
}

# =============================================================================
# RUN ALL TESTS
# =============================================================================

main() {
    echo -e "${BLUE}"
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║     COMPREHENSIVE ESCALATION HOOK TEST SUITE                   ║"
    echo "║     Testing all promises and features                          ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    # Test suites
    test_escalate_to_sonnet
    test_escalate_explicit_sonnet
    test_escalate_to_opus
    test_escalate_to_haiku
    test_escalate_case_insensitive

    test_model_id_opus
    test_model_id_sonnet
    test_model_id_haiku

    test_deescalate_phrase_works_great
    test_deescalate_phrase_thanks
    test_deescalate_phrase_that_fixed
    test_deescalate_word_perfect
    test_deescalate_word_solved
    test_deescalate_word_fixed

    test_no_deescalate_thanks_but
    test_no_deescalate_thanks_however
    test_no_deescalate_fixed_needed

    test_cascade_opus_to_sonnet
    test_cascade_opus_to_sonnet_to_haiku

    test_session_created_on_escalate
    test_session_cleared_on_deescalate
    test_session_timestamp_valid

    test_opus_effort_high
    test_sonnet_effort_high
    test_haiku_effort_low

    test_settings_valid_json_after_escalate
    test_settings_valid_json_after_deescalate

    test_non_escalate_prompt_unchanged
    test_escalate_without_session_no_deescalate

    test_effort_transition_low_to_high
    test_effort_transition_high_to_low

    # Summary
    echo -e "\n${BLUE}════════════════════════════════════════════════════════════════${NC}"
    echo -e "Total Tests: $TESTS_RUN"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "\n${GREEN}✅ ALL TESTS PASSED${NC}"
        exit 0
    else
        echo -e "\n${RED}❌ SOME TESTS FAILED${NC}"
        exit 1
    fi

    cleanup
}

main "$@"
