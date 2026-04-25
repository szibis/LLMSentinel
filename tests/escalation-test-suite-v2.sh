#!/bin/bash
# =============================================================================
# ESCALATION HOOK TEST SUITE v2 - Improved isolation and comprehensive coverage
# =============================================================================

set -o pipefail

# Configuration
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
SETTINGS_FILE="${HOME}/.claude/settings.json"
ORIGINAL_SETTINGS=$(mktemp)
WORK_DIR="/tmp/escalation-test-work-$$"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test utilities
echo_test() { echo -e "${BLUE}[TEST]${NC} $1"; }
echo_pass() { echo -e "${GREEN}[✓]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED+1)); }
echo_fail() { echo -e "${RED}[✗]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED+1)); }

# Setup/Teardown with proper isolation
setup_test() {
    TESTS_RUN=$((TESTS_RUN+1))

    # Save original settings once
    [ ! -s "$ORIGINAL_SETTINGS" ] && cp "$SETTINGS_FILE" "$ORIGINAL_SETTINGS"

    # Create independent work directory
    TEST_DIR="$WORK_DIR/test-$$-$TESTS_RUN"
    mkdir -p "$TEST_DIR"

    # Use isolated temp location
    export SESSION_OVERRIDE="/tmp/.escalation_test_$$_$TESTS_RUN"
    mkdir -p "$SESSION_OVERRIDE"

    # Reset state
    rm -rf /tmp/.escalation_$(id -u) /tmp/.response_analysis_$(id -u)
}

teardown_test() {
    # Restore original settings
    cp "$ORIGINAL_SETTINGS" "$SETTINGS_FILE" 2>/dev/null

    # Clean test directory
    rm -rf "$TEST_DIR" "$SESSION_OVERRIDE" 2>/dev/null
}

cleanup_all() {
    rm -f "$ORIGINAL_SETTINGS"
    rm -rf "$WORK_DIR"
}

# Hook test executor with proper JSON handling
test_escalate_hook() {
    local prompt="$1"
    local hook="${HOME}/.claude/hooks/escalate-model.sh"

    if [ ! -f "$hook" ]; then return 1; fi

    # Use proper JSON escaping
    local json_prompt=$(echo "$prompt" | jq -R -s .)
    echo "{\"prompt\":$json_prompt}" | bash "$hook" > /dev/null 2>&1
}

test_deescalate_hook() {
    local prompt="$1"
    local hook="${HOME}/.claude/hooks/de-escalate-model.sh"

    if [ ! -f "$hook" ]; then return 1; fi

    local json_prompt=$(echo "$prompt" | jq -R -s .)
    echo "{\"prompt\":$json_prompt}" | bash "$hook" > /dev/null 2>&1
}

assert_model() {
    local expected="$1"
    local actual=$(jq -r '.model // empty' "$SETTINGS_FILE" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        echo_pass "Model is $expected"
    else
        echo_fail "Model check: expected $expected, got $actual"
    fi
}

assert_effort() {
    local expected="$1"
    local actual=$(jq -r '.effortLevel // empty' "$SETTINGS_FILE" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        echo_pass "Effort is $expected"
    else
        echo_fail "Effort check: expected $expected, got $actual"
    fi
}

# ============================================================================
# TEST SUITE: CORE ESCALATION
# ============================================================================

echo -e "\n${BLUE}=== CORE ESCALATION TESTS ===${NC}"

test_escalate_default_to_sonnet() {
    echo_test "/escalate defaults to Sonnet"
    setup_test

    test_escalate_hook "/escalate"
    assert_model "claude-sonnet-4-6"
    assert_effort "high"

    teardown_test
}

test_escalate_explicit() {
    echo_test "/escalate to sonnet (explicit)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    assert_model "claude-sonnet-4-6"

    teardown_test
}

test_escalate_to_opus() {
    echo_test "/escalate to opus"
    setup_test

    test_escalate_hook "/escalate to opus"
    assert_model "claude-opus-4-6"
    assert_effort "high"

    teardown_test
}

test_escalate_to_haiku_from_sonnet() {
    echo_test "/escalate to haiku (manual downgrade)"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    test_escalate_hook "/escalate to haiku"
    assert_model "claude-haiku-4-5-20251001"
    assert_effort "low"

    teardown_test
}

test_escalate_case_insensitive() {
    echo_test "Case-insensitive: /ESCALATE TO SONNET"
    setup_test

    test_escalate_hook "/ESCALATE TO SONNET"
    assert_model "claude-sonnet-4-6"

    teardown_test
}

# ============================================================================
# TEST SUITE: MODEL MAPPING
# ============================================================================

echo -e "\n${BLUE}=== MODEL MAPPING TESTS ===${NC}"

test_model_mapping_opus() {
    echo_test "Opus model mapping"
    setup_test

    test_escalate_hook "/escalate to opus"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-opus-4-6" ]; then
        echo_pass "Opus maps to claude-opus-4-6"
    else
        echo_fail "Opus mapping: got $model"
    fi

    teardown_test
}

test_model_mapping_sonnet() {
    echo_test "Sonnet model mapping"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-sonnet-4-6" ]; then
        echo_pass "Sonnet maps to claude-sonnet-4-6"
    else
        echo_fail "Sonnet mapping: got $model"
    fi

    teardown_test
}

test_model_mapping_haiku() {
    echo_test "Haiku model mapping"
    setup_test

    test_escalate_hook "/escalate to haiku"
    local model=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$model" = "claude-haiku-4-5-20251001" ]; then
        echo_pass "Haiku maps to claude-haiku-4-5-20251001"
    else
        echo_fail "Haiku mapping: got $model"
    fi

    teardown_test
}

# ============================================================================
# TEST SUITE: DE-ESCALATION WITH IMPROVED ISOLATION
# ============================================================================

echo -e "\n${BLUE}=== DE-ESCALATION TESTS (Improved Isolation) ===${NC}"

test_deescalate_phrase_works_great() {
    echo_test "De-escalate phrase: 'works great'"
    setup_test

    # Escalate
    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    # De-escalate
    test_deescalate_hook "Thanks! That works great."
    sleep 0.5

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_phrase_thanks_for() {
    echo_test "De-escalate phrase: 'thanks for'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    test_deescalate_hook "Thanks for your help!"
    sleep 0.5

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_perfect() {
    echo_test "De-escalate word: 'perfect'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    test_deescalate_hook "That's perfect!"
    sleep 0.5

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_solved() {
    echo_test "De-escalate word: 'solved'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    test_deescalate_hook "The issue is solved!"
    sleep 0.5

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

test_deescalate_word_fixed() {
    echo_test "De-escalate word: 'fixed'"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    test_deescalate_hook "That is fixed now."
    sleep 0.5

    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

# ============================================================================
# TEST SUITE: FALSE POSITIVE GUARDS
# ============================================================================

echo -e "\n${BLUE}=== FALSE POSITIVE GUARD TESTS ===${NC}"

test_guard_thanks_but() {
    echo_test "Guard: 'thanks but' should not trigger"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    local before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "Thanks but it still doesn't work."
    sleep 0.5
    local after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$before" = "$after" ]; then
        echo_pass "Guard: 'thanks but' correctly blocked"
    else
        echo_fail "Guard failed: model changed from $before to $after"
    fi

    teardown_test
}

test_guard_thanks_however() {
    echo_test "Guard: 'thanks however' should not trigger"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    local before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "Thanks, however we need more work."
    sleep 0.5
    local after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$before" = "$after" ]; then
        echo_pass "Guard: 'thanks however' correctly blocked"
    else
        echo_fail "Guard failed: model changed"
    fi

    teardown_test
}

test_guard_fixed_needed() {
    echo_test "Guard: 'need to fix' should not trigger"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5

    local before=$(jq -r '.model' "$SETTINGS_FILE")
    test_deescalate_hook "We need to fix this part."
    sleep 0.5
    local after=$(jq -r '.model' "$SETTINGS_FILE")

    if [ "$before" = "$after" ]; then
        echo_pass "Guard: 'need to fix' correctly blocked"
    else
        echo_fail "Guard failed"
    fi

    teardown_test
}

# ============================================================================
# TEST SUITE: CASCADE DE-ESCALATION
# ============================================================================

echo -e "\n${BLUE}=== CASCADE DE-ESCALATION TESTS ===${NC}"

test_cascade_opus_to_sonnet() {
    echo_test "Cascade: Opus → Sonnet"
    setup_test

    test_escalate_hook "/escalate to opus"
    sleep 0.5

    test_deescalate_hook "That works!"
    sleep 0.5

    assert_model "claude-sonnet-4-6"

    teardown_test
}

test_cascade_full_opus_sonnet_haiku() {
    echo_test "Cascade: Opus → Sonnet → Haiku"
    setup_test

    # Escalate to Opus
    test_escalate_hook "/escalate to opus"
    sleep 0.5
    assert_model "claude-opus-4-6"

    # De-escalate to Sonnet
    test_deescalate_hook "Good start!"
    sleep 0.5
    assert_model "claude-sonnet-4-6"

    # De-escalate to Haiku
    test_deescalate_hook "Perfect solution!"
    sleep 0.5
    assert_model "claude-haiku-4-5-20251001"

    teardown_test
}

# ============================================================================
# TEST SUITE: METADATA CONSISTENCY
# ============================================================================

echo -e "\n${BLUE}=== METADATA CONSISTENCY TESTS ===${NC}"

test_effort_consistency_opus() {
    echo_test "Opus + effort=high consistency"
    setup_test

    test_escalate_hook "/escalate to opus"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [[ "$model" == *"opus"* ]] && [ "$effort" = "high" ]; then
        echo_pass "Opus has matching effort=high"
    else
        echo_fail "Consistency check failed"
    fi

    teardown_test
}

test_effort_consistency_haiku() {
    echo_test "Haiku + effort=low consistency"
    setup_test

    test_escalate_hook "/escalate to haiku"

    local model=$(jq -r '.model' "$SETTINGS_FILE")
    local effort=$(jq -r '.effortLevel' "$SETTINGS_FILE")

    if [[ "$model" == *"haiku"* ]] && [ "$effort" = "low" ]; then
        echo_pass "Haiku has matching effort=low"
    else
        echo_fail "Consistency check failed"
    fi

    teardown_test
}

# ============================================================================
# TEST SUITE: JSON INTEGRITY
# ============================================================================

echo -e "\n${BLUE}=== JSON INTEGRITY TESTS ===${NC}"

test_json_valid_after_escalate() {
    echo_test "JSON valid after escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"

    if jq . "$SETTINGS_FILE" > /dev/null 2>&1; then
        echo_pass "settings.json is valid JSON"
    else
        echo_fail "JSON validation failed"
    fi

    teardown_test
}

test_json_valid_after_deescalate() {
    echo_test "JSON valid after de-escalation"
    setup_test

    test_escalate_hook "/escalate to sonnet"
    sleep 0.5
    test_deescalate_hook "Works!"
    sleep 0.5

    if jq . "$SETTINGS_FILE" > /dev/null 2>&1; then
        echo_pass "settings.json is valid JSON"
    else
        echo_fail "JSON validation failed"
    fi

    teardown_test
}

# ============================================================================
# MAIN TEST RUNNER
# ============================================================================

main() {
    echo -e "${BLUE}"
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║     ESCALATION HOOK TEST SUITE v2 - Improved Isolation        ║"
    echo "║     Comprehensive testing with proper test isolation          ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    # Run all tests
    test_escalate_default_to_sonnet
    test_escalate_explicit
    test_escalate_to_opus
    test_escalate_to_haiku_from_sonnet
    test_escalate_case_insensitive

    test_model_mapping_opus
    test_model_mapping_sonnet
    test_model_mapping_haiku

    test_deescalate_phrase_works_great
    test_deescalate_phrase_thanks_for
    test_deescalate_word_perfect
    test_deescalate_word_solved
    test_deescalate_word_fixed

    test_guard_thanks_but
    test_guard_thanks_however
    test_guard_fixed_needed

    test_cascade_opus_to_sonnet
    test_cascade_full_opus_sonnet_haiku

    test_effort_consistency_opus
    test_effort_consistency_haiku

    test_json_valid_after_escalate
    test_json_valid_after_deescalate

    # Summary
    echo -e "\n${BLUE}════════════════════════════════════════════════════════════════${NC}"
    echo -e "Tests Run:    $TESTS_RUN"
    echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "\n${GREEN}✅ ALL TESTS PASSED${NC}"
        cleanup_all
        exit 0
    else
        echo -e "\n${RED}❌ SOME TESTS FAILED${NC}"
        cleanup_all
        exit 1
    fi
}

trap cleanup_all EXIT
main "$@"
