#!/bin/bash
# =============================================================================
# Real-World Escalation Testing Harness
# Executes tests in actual Claude Code sessions and records stats
# =============================================================================

set -o pipefail

HARNESS_LOG="/tmp/escalation-real-world-tests.log"
STATS_FILE="/tmp/escalation-real-world-stats.json"
RESULTS_FILE="/tmp/escalation-real-world-results.md"
SESSION_ID="session-$(date +%s)"
START_TIME=$(date +%s)

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo_info() { echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$HARNESS_LOG"; }
echo_pass() { echo -e "${GREEN}[✓]${NC} $1" | tee -a "$HARNESS_LOG"; }
echo_fail() { echo -e "${RED}[✗]${NC} $1" | tee -a "$HARNESS_LOG"; }
echo_warn() { echo -e "${YELLOW}[⚠]${NC} $1" | tee -a "$HARNESS_LOG"; }

# Initialize log
cat > "$HARNESS_LOG" << EOF
═══════════════════════════════════════════════════════════════════════════════
                    Real-World Escalation Testing Harness
                              Session: $SESSION_ID
                              Start: $(date)
═══════════════════════════════════════════════════════════════════════════════

TEST EXECUTION LOG
─────────────────────────────────────────────────────────────────────────────

EOF

echo_info "Starting real-world escalation tests (session: $SESSION_ID)"

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

check_current_model() {
    if [ -f ~/.claude/settings.json ]; then
        jq -r '.model // "unknown"' ~/.claude/settings.json 2>/dev/null
    else
        echo "unknown"
    fi
}

check_current_effort() {
    if [ -f ~/.claude/settings.json ]; then
        jq -r '.effortLevel // "unknown"' ~/.claude/settings.json 2>/dev/null
    else
        echo "unknown"
    fi
}

check_session_exists() {
    [ -f "/tmp/.escalation_$(id -u)/escalation_session" ] && echo "yes" || echo "no"
}

record_test_result() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    local gap="$4"
    local severity="$5"

    cat >> "$RESULTS_FILE" << EOF

## Test: $test_name

**Expected**: $expected
**Actual**: $actual
**Gap**: $gap
**Severity**: $severity

EOF
}

# =============================================================================
# PRE-TEST DIAGNOSTICS
# =============================================================================

echo_info "Collecting pre-test diagnostics..."

INITIAL_MODEL=$(check_current_model)
INITIAL_EFFORT=$(check_current_effort)
INITIAL_SESSION=$(check_session_exists)

echo_pass "Current model: $INITIAL_MODEL"
echo_pass "Current effort: $INITIAL_EFFORT"
echo_pass "Session file exists: $INITIAL_SESSION"

# =============================================================================
# STATS COLLECTION SETUP
# =============================================================================

echo_info "Setting up stats collection..."

# Initialize stats JSON
cat > "$STATS_FILE" << EOF
{
  "session_id": "$SESSION_ID",
  "start_time": "$START_TIME",
  "initial_model": "$INITIAL_MODEL",
  "initial_effort": "$INITIAL_EFFORT",
  "tests_run": 0,
  "tests_passed": 0,
  "tests_failed": 0,
  "gaps_found": 0,
  "gaps": [],
  "test_results": []
}
EOF

echo_pass "Stats file initialized: $STATS_FILE"

# =============================================================================
# INITIALIZE RESULTS REPORT
# =============================================================================

cat > "$RESULTS_FILE" << EOF
# Real-World Escalation Testing Results

**Session ID**: $SESSION_ID
**Date**: $(date)
**Initial Model**: $INITIAL_MODEL
**Initial Effort**: $INITIAL_EFFORT

---

## Test Execution Plan

This document records results of real-world escalation hook testing in actual Claude Code sessions.

### Pre-Test State
- Current Model: $INITIAL_MODEL
- Current Effort: $INITIAL_EFFORT
- Session Exists: $INITIAL_SESSION

### Test Categories

EOF

# =============================================================================
# TEST 1: VERIFY HOOK EXECUTION
# =============================================================================

echo_info ""
echo_info "TEST GROUP 1: Hook Execution Verification"
echo_info "Testing: Can we detect hook execution in real sessions?"

# This test would require Claude Code integration to inject test prompts
# For now, create a checklist for manual verification

cat >> "$RESULTS_FILE" << 'EOF'

## Test Group 1: Hook Execution Verification

### 1.1 Escalate-model.sh Execution
- [ ] /escalate command in next session shows hookSpecificOutput
- [ ] Model actually changes in settings.json
- [ ] Effort level matches new model

### 1.2 De-escalate-model.sh Execution
- [ ] Success signal triggers de-escalation
- [ ] Model downgrades correctly
- [ ] Session cleanup happens

### 1.3 Barista Integration
- [ ] Statusline shows new model immediately
- [ ] Emoji updates correctly (🪶 Haiku, 🧠 Opus, etc.)
- [ ] Effort indicator updates (🎚 low/high)

EOF

# =============================================================================
# TEST 2: COMMAND VARIANTS
# =============================================================================

echo_info "TEST GROUP 2: Command Variants"

cat >> "$RESULTS_FILE" << 'EOF'

## Test Group 2: Command Variants

Testing all /escalate command formats in real sessions.

### 2.1 Basic /escalate
```
ACTION: /escalate
EXPECTED: Model → claude-sonnet-4-6, Effort → high
ACTUAL: [run in real session and record]
GAP: [note any differences]
```

### 2.2 Explicit variants
```
ACTION: /escalate to opus
EXPECTED: Model → claude-opus-4-6, Effort → high
ACTUAL: [test result]

ACTION: /escalate to haiku
EXPECTED: Model → claude-haiku-4-5-20251001, Effort → low
ACTUAL: [test result]
```

EOF

# =============================================================================
# TEST 3: DE-ESCALATION SIGNALS
# =============================================================================

echo_info "TEST GROUP 3: De-escalation Signals"

cat >> "$RESULTS_FILE" << 'EOF'

## Test Group 3: De-escalation Signals

Testing real success phrase detection in actual responses.

### 3.1 Phrase: "works great"
```
SETUP: /escalate to sonnet
ACTION: "Thanks! That works great."
EXPECTED: Model → claude-haiku-4-5-20251001
ACTUAL: [test in real session]
TIMING: [How long to de-escalate? Immediate or delayed?]
```

### 3.2 Phrase: "thanks for"
```
SETUP: /escalate to sonnet
ACTION: "Thanks for your help!"
EXPECTED: De-escalate to Haiku
ACTUAL: [test result]
BARISTA: [what does statusline show?]
```

### 3.3 Word: "perfect"
```
SETUP: /escalate to opus
ACTION: "That's perfect!"
EXPECTED: Cascade to Sonnet (Opus→Sonnet)
ACTUAL: [test result]
```

EOF

# =============================================================================
# TEST 4: FALSE POSITIVE GUARDS
# =============================================================================

echo_info "TEST GROUP 4: False Positive Guards"

cat >> "$RESULTS_FILE" << 'EOF'

## Test Group 4: False Positive Guards

Testing that guards work in real scenarios.

### 4.1 Guard: "thanks but"
```
SETUP: /escalate to sonnet
ACTION: "Thanks but it still doesn't work"
EXPECTED: Model stays as Sonnet (NO de-escalation)
ACTUAL: [test result]
```

### 4.2 Guard: "thanks however"
```
SETUP: /escalate to opus
ACTION: "Thanks, however we need more work"
EXPECTED: Model stays as Opus (NO de-escalation)
ACTUAL: [test result]
```

EOF

# =============================================================================
# TEST 5: STATS RECORDING
# =============================================================================

echo_info "TEST GROUP 5: Stats Recording & Dashboard"
echo_info "Checking if escalation stats are recorded for dashboard..."

# Check if stats tracking is enabled
STATS_HOOK="${HOME}/.claude/hooks/track-escalation-patterns.sh"
STATS_DATA_DIR="${HOME}/.claude/data/escalation"

if [ -f "$STATS_HOOK" ]; then
    echo_pass "Stats tracking hook found: $STATS_HOOK"
else
    echo_fail "Stats tracking hook NOT found: $STATS_HOOK"
fi

if [ -d "$STATS_DATA_DIR" ]; then
    echo_pass "Stats data directory exists: $STATS_DATA_DIR"
    ls -la "$STATS_DATA_DIR" | tee -a "$HARNESS_LOG"
else
    echo_warn "Stats data directory not found (should be created): $STATS_DATA_DIR"
fi

# Check existing stats files
if [ -f "$STATS_DATA_DIR/escalations.log" ]; then
    echo_pass "Escalations log exists"
    echo "Recent entries:" | tee -a "$HARNESS_LOG"
    tail -5 "$STATS_DATA_DIR/escalations.log" | tee -a "$HARNESS_LOG"
else
    echo_warn "Escalations log not found"
fi

cat >> "$RESULTS_FILE" << EOF

## Test Group 5: Stats Recording & Dashboard

### 5.1 Stats Hook Status
- Hook Found: $([ -f "$STATS_HOOK" ] && echo "YES" || echo "NO")
- Data Directory: $([ -d "$STATS_DATA_DIR" ] && echo "EXISTS" || echo "MISSING")
- Escalations Log: $([ -f "$STATS_DATA_DIR/escalations.log" ] && echo "EXISTS" || echo "MISSING")

### 5.2 Dashboard Integration
**Issue**: User reports no stats visible in dashboard

**Possible Causes**:
1. Stats hook not in hook chain
2. Stats files not being created
3. Dashboard looking in wrong location
4. Hook execution disabled

**Current Status**:
- Stats hook present: $([ -f "$STATS_HOOK" ] && echo "YES" || echo "NO")
- Data directory: $([ -d "$STATS_DATA_DIR" ] && echo "EXISTS" || echo "MISSING")

### 5.3 Recommendations
- [ ] Verify stats hook in settings.json hooks chain
- [ ] Check if hook is being executed (add test escalation)
- [ ] Verify stats data directory has write permissions
- [ ] Confirm dashboard is reading from correct location

EOF

echo_warn "Stats collection issue detected - needs investigation"

# =============================================================================
# AUTO-EFFORT INTERACTION TEST
# =============================================================================

echo_info "TEST GROUP 6: Auto-Effort Interaction"

cat >> "$RESULTS_FILE" << 'EOF'

## Test Group 6: Auto-Effort Interaction

Testing interaction between /escalate and auto-effort routing.

### 6.1 Auto-effort precedence
```
SETUP: Type complex implementation task
OBSERVE: Auto-effort classification (check with /effort)
ACTION: /escalate to haiku
EXPECTED: Model changes to Haiku (NOT overridden by auto-effort)
ACTUAL: [test result]
QUESTION: Does auto-effort re-escalate on next prompt?
```

### 6.2 /effort vs /escalate
```
ACTION: /effort medium
EXPECTED: effort=medium
ACTION: /escalate to opus
EXPECTED: model=opus, effort=high (overrides /effort)
ACTUAL: [test result]
```

EOF

# =============================================================================
# GENERATE TEST CHECKLIST
# =============================================================================

cat >> "$RESULTS_FILE" << 'EOF'

---

## Test Execution Checklist

### Before Running Tests
- [ ] Fresh Claude Code session started
- [ ] Barista statusline visible
- [ ] settings.json backup created
- [ ] Test harness ready

### During Tests
- [ ] Note exact timing of model changes
- [ ] Screenshot barista state at each step
- [ ] Verify actual model used (not just settings)
- [ ] Monitor for any errors/warnings
- [ ] Record all actual vs expected outcomes

### After Tests
- [ ] Document all gaps found
- [ ] Categorize by severity
- [ ] Create reproduction steps
- [ ] Update this report with findings

---

## Key Metrics to Measure

1. **Latency**
   - Time from /escalate to model change: ___ms
   - Time from success signal to de-escalation: ___ms
   - Barista refresh delay: ___ms

2. **Reliability**
   - Escalation success rate: ___ / ___
   - De-escalation success rate: ___ / ___
   - Cascade success rate: ___ / ___
   - False positive rate: ___ / ___

3. **Integration**
   - Barista updates: Yes / No
   - Stats recorded: Yes / No
   - Auto-effort interaction: Working / Issues
   - Model persistence: Yes / No

---

## Gap Summary

(To be filled in after real-world testing)

### Critical Gaps
- [List any critical issues found]

### High Priority Gaps
- [List high-priority issues]

### Medium Priority Gaps
- [List medium-priority issues]

### Low Priority / Polish
- [List low-priority improvements]

---

## Recommendations

(To be filled in after testing)

### Immediate Fixes Needed
1. [If any critical gaps found]

### Phase 2 Improvements
1. [Based on gap analysis]

### Documentation Updates
1. [Any docs that need updating]

EOF

echo_info ""
echo_pass "Results file created: $RESULTS_FILE"

# =============================================================================
# FINAL REPORT
# =============================================================================

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

cat >> "$HARNESS_LOG" << EOF

═══════════════════════════════════════════════════════════════════════════════
                              Test Summary
═══════════════════════════════════════════════════════════════════════════════

Session ID:          $SESSION_ID
Duration:            ${DURATION}s
Start Time:          $(date -f '%s' "$START_TIME" 2>/dev/null || date)
End Time:            $(date)

Results File:        $RESULTS_FILE
Stats File:          $STATS_FILE
Log File:            $HARNESS_LOG

NEXT STEPS:
1. Review $RESULTS_FILE
2. Execute test cases in real Claude Code sessions
3. Record results in RESULTS_FILE
4. Analyze gaps and categorize by severity
5. Create follow-up tasks for fixes

KEY ISSUE DETECTED:
⚠️  Stats not visible in dashboard
   - Stats hook may not be in hook chain
   - Stats files may not be created
   - Dashboard may be looking in wrong location

═══════════════════════════════════════════════════════════════════════════════
EOF

echo_info ""
echo_pass "Test harness setup complete!"
echo_info ""
echo_info "NEXT STEPS:"
echo_info "1. Run real tests in Claude Code sessions"
echo_info "2. Record results in: $RESULTS_FILE"
echo_info "3. Check: $STATS_FILE"
echo_info ""
echo_warn "⚠️  IMPORTANT: Stats recording issue detected"
echo_warn "    Check if stats hook is in settings.json hook chain"
echo_warn ""
