# Critical Gap: Barista Always Shows Haiku

**Issue**: "In barista I still see only haiku used here all the time"  
**Severity**: CRITICAL  
**Impact**: Auto-escalation system appears broken in real-world usage  
**Status**: Requires immediate investigation  

---

## Issue Description

User reports that despite /escalate commands and escalation improvements, the barista statusline always shows Haiku model, even after:
- Running /escalate commands
- Expecting auto-effort to route to expensive models
- Running complex tasks

---

## Root Cause Analysis

### Hypothesis 1: De-escalation Triggering Too Aggressively
**Theory**: De-escalation might be triggering immediately on success phrases that appear naturally in responses

**Evidence to check**:
- [ ] Are Claude responses containing success phrases?
- [ ] Is de-escalation happening immediately after each successful response?
- [ ] Does session persist or get cleared?

**Test**:
```bash
# Monitor real-time escalation
tail -f ~/.claude/data/escalation/escalations.log

# Check de-escalation frequency
tail -f ~/.claude/data/escalation/deescalations.log
```

### Hypothesis 2: Auto-effort Not Routing to Expensive Models
**Theory**: Auto-effort classification might be failing or routing everything to Haiku

**Evidence to check**:
- [ ] What does /effort show after complex tasks?
- [ ] Is auto-effort hook executing?
- [ ] Is complexity classification working?

**Test**:
```bash
# Check auto-effort classification
grep "auto-effort" ~/.claude/hooks/auto-effort.sh | head -5

# Test complex task
[Ask Claude a complex question]
/effort
# What model is shown?
```

### Hypothesis 3: /escalate Commands Not Persisting
**Theory**: /escalate might change model, but it reverts to Haiku quickly

**Evidence to check**:
- [ ] Does model change immediately after /escalate?
- [ ] Does it stay changed for the response?
- [ ] Does it revert afterward?

**Test**:
```bash
# Check initial model
/effort
# Should show current model

# Escalate
/escalate to opus
# Check again
/effort
# Should show Opus

# Wait for response
[Get response]

# Check again after response
/effort
# Should still be Opus
```

### Hypothesis 4: Barista Display Bug
**Theory**: Model is actually changing, but barista display not updating

**Evidence to check**:
- [ ] Check actual model in settings.json
- [ ] Does barista emoji match settings.json?
- [ ] Is there a refresh lag?

**Test**:
```bash
# Check actual model
jq '.model' ~/.claude/settings.json
# vs
# What barista shows

# Check effort
jq '.effortLevel' ~/.claude/settings.json
# vs
# What barista shows
```

---

## Investigation Script

```bash
#!/bin/bash

echo "=== BARISTA MODEL DISPLAY ISSUE INVESTIGATION ==="
echo ""

echo "1. Current Model & Effort"
echo "   settings.json: model=$(jq -r '.model' ~/.claude/settings.json)"
echo "   settings.json: effort=$(jq -r '.effortLevel' ~/.claude/settings.json)"
echo "   Barista shows: [manually check]"
echo ""

echo "2. Escalation History (last 10)"
echo "   Escalations:"
tail -10 ~/.claude/data/escalation/escalations.log 2>/dev/null || echo "   [No escalations logged]"
echo ""

echo "3. De-escalation History (last 10)"
echo "   De-escalations:"
tail -10 ~/.claude/data/escalation/deescalations.log 2>/dev/null || echo "   [No de-escalations logged]"
echo ""

echo "4. Session Status"
SESSION_DIR="/tmp/.escalation_$(id -u)"
if [ -f "$SESSION_DIR/escalation_session" ]; then
    SESSION_TIME=$(cat "$SESSION_DIR/escalation_session")
    ELAPSED=$(( $(date +%s) - SESSION_TIME ))
    echo "   Session active: YES (${ELAPSED}s ago)"
else
    echo "   Session active: NO"
fi
echo ""

echo "5. Auto-effort Turn Log (last 3 turns)"
tail -3 ~/.claude/data/escalation/turns 2>/dev/null || echo "   [No turn log found]"
echo ""

echo "6. Hook Execution Check"
grep "track-escalation" ~/.claude/settings.json > /dev/null && \
    echo "   Stats hook: REGISTERED" || \
    echo "   Stats hook: NOT REGISTERED"
echo ""

echo "7. Response Phrases Check"
echo "   Common success phrases found in recent responses:"
grep -i "works\|perfect\|thanks\|solved\|fixed" ~/.claude/data/escalation/turns 2>/dev/null | head -5 || \
    echo "   [No phrase logs found]"

```

---

## Diagnostic Steps

### Step 1: Verify Model Actually Changes
```bash
# Run /escalate command
/escalate to opus

# Check settings immediately
jq '.model' ~/.claude/settings.json
# Should show: "claude-opus-4-6"

# Check effort
jq '.effortLevel' ~/.claude/settings.json
# Should show: "high"

# Check what barista displays
# [Note emoji and model name shown in statusline]
```

### Step 2: Monitor De-escalation Behavior
```bash
# Tail logs while working
tail -f ~/.claude/data/escalation/escalations.log &
tail -f ~/.claude/data/escalation/deescalations.log &

# Do normal work
# Note when de-escalations occur
# Do they happen after every response?
# Or only when you say success phrases?
```

### Step 3: Test Auto-effort Classification
```bash
# Simple task
[Ask: "What is 2+2?"]
/effort
# Should be low (Haiku)

# Complex task
[Ask: complex multi-step implementation question]
/effort
# Should be high (Opus) after classification

# Check if auto-effort is working
grep "complexity" ~/.claude/hooks/auto-effort.sh | head -5
```

### Step 4: Check Hook Chain
```bash
# Verify all hooks in chain
jq '.hooks.UserPromptSubmit[0].hooks[].command' ~/.claude/settings.json

# Should show:
# - claude-escalate hook
# - auto-effort.sh
# - track-escalation-patterns.sh
# - de-escalate-model.sh

# Check if de-escalate hook is there
jq '.hooks.UserPromptSubmit' ~/.claude/settings.json | grep -i "deescalate\|de-escalate"
```

---

## Possible Fixes (Priority Order)

### Fix 1: De-escalation Too Aggressive
**If**: De-escalations log shows multiple per response

**Solution**: Tighten success signal detection
```bash
# Make phrase matching stricter
# Require multiple phrases per prompt
# Add context guards to single words
# Require explicit user intent
```

### Fix 2: Auto-effort Not Working
**If**: /effort always shows low/haiku

**Solution**: Debug auto-effort hook
```bash
# Check complexity classifier
# Verify hook is being called
# Check if settings update happens
# Add logging to auto-effort.sh
```

### Fix 3: /Escalate Not Persisting
**If**: Model changes then reverts

**Solution**: Check hook order and de-escalation session
```bash
# Verify /escalate runs first
# Ensure session file created
# Check if de-escalate is clearing it prematurely
```

### Fix 4: Barista Display Bug
**If**: Actual model != displayed model

**Solution**: Force barista refresh
```bash
# Restart Claude Code
# Clear barista cache
# Force model update display
```

---

## Questions for User

Before investigating further, please verify:

1. **Are you explicitly using /escalate commands?**
   - Or relying on auto-effort?
   - Both?

2. **What kind of tasks are you running?**
   - Simple lookups?
   - Complex implementations?
   - Mixed?

3. **How long between escalation and response?**
   - Does model show immediately?
   - Does it change after response comes back?

4. **Can you provide logs?**
   ```bash
   # Provide output of:
   jq '.model, .effortLevel' ~/.claude/settings.json
   tail -20 ~/.claude/data/escalation/escalations.log
   tail -20 ~/.claude/data/escalation/deescalations.log
   ```

---

## Recommended Actions

### Immediate (Before GitHub Push)
- [ ] Run diagnostic script
- [ ] Identify which hypothesis is correct
- [ ] Document actual behavior
- [ ] Note as known issue in README

### In This PR
- [ ] Add diagnostic tools
- [ ] Update documentation with troubleshooting
- [ ] Note the issue as "Known Issue - Under Investigation"

### In Phase 2
- [ ] Fix root cause based on investigation
- [ ] Add more aggressive testing for this scenario
- [ ] Improve barista display updates
- [ ] Add real-time monitoring

---

## Testing Instructions for User

**To help diagnose the issue, please run:**

```bash
# 1. Check current state
echo "=== Current State ==="
jq '.model, .effortLevel' ~/.claude/settings.json

# 2. Manually escalate
echo ""
echo "=== Escalating to Opus ==="
/escalate to opus

# Check immediately
jq '.model' ~/.claude/settings.json
# Note what barista shows

# 3. Send a test prompt
echo ""
echo "=== Sending test prompt ==="
# [Send a simple prompt]

# 4. Check after response
echo ""
echo "=== After response ==="
jq '.model, .effortLevel' ~/.claude/settings.json
# Note what barista shows

# 5. Check logs
echo ""
echo "=== Recent logs ==="
tail -5 ~/.claude/data/escalation/escalations.log
tail -5 ~/.claude/data/escalation/deescalations.log
```

**Then send output to help diagnose.**

---

## Summary

**Critical Issue Identified**: Model selection not working as expected in real usage

**Status**: Requires investigation before/after deployment

**Recommendation**: 
1. Document as known issue in GitHub PR
2. Add diagnostic tools to repository
3. Plan Phase 2 investigation
4. Deploy with this issue noted and monitoring enabled
