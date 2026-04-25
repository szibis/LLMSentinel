# Troubleshooting Guide

## Common Issues

### Hook Not Running

**Symptom**: Model doesn't change when using `/escalate`, auto-effort not triggering

**Diagnosis**:
```bash
# Check if hook is registered
jq '.hooks.UserPromptSubmit' ~/.claude/settings.json

# Expected: Shows escalation-manager command in hook chain

# Check binary exists
ls -la ~/.claude/bin/escalation-manager
# Should show: -rwxr-xr-x ... escalation-manager

# Test binary directly
echo '{"prompt": "/escalate to opus"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager
# Should output JSON response
```

**Solutions**:

1. **Hook not in settings.json**:
```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "~/.claude/bin/escalation-manager",
        "timeout": 5,
        "continueOnFailure": true
      }
    ]
  }
}
```

2. **Binary not executable**:
```bash
chmod +x ~/.claude/bin/escalation-manager
```

3. **Claude Code needs restart**:
```bash
# Some versions require restart after settings change
# Completely close Claude Code and reopen
```

---

### Model Not Changing

**Symptom**: `/escalate` command accepted, but model stays the same

**Diagnosis**:
```bash
# Check current model in settings
jq '.model' ~/.claude/settings.json

# Test escalation directly
echo '{"prompt": "/escalate to opus"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager

# Check output for error messages
```

**Solutions**:

1. **Check settings.json is writable**:
```bash
ls -la ~/.claude/settings.json
# Should be user-readable and writable
chmod 644 ~/.claude/settings.json
```

2. **Verify jq is installed**:
```bash
which jq
# Should show path to jq
# If not: brew install jq (macOS) or apt-get install jq (Linux)
```

3. **Check for JSON syntax errors**:
```bash
jq . ~/.claude/settings.json
# If error, file is corrupted. Restore from backup or recreate.
```

4. **Test with explicit path**:
```bash
HOME=/Users/yourname ~/.claude/bin/escalation-manager stats | jq .
```

---

### De-escalation Not Triggering

**Symptom**: Say "works!" but model doesn't cascade down to cheaper option

**Diagnosis**:
```bash
# Check if escalation session exists
ls -la /tmp/.escalation_$(id -u)/
# Should show: escalation_session

# Check if success phrase is detected
echo '{"prompt": "Perfect!"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager

# Check if cascade timeout is active
cat /tmp/.escalation_$(id -u)/last_cascade_time 2>/dev/null
# If exists: too recent, need to wait 5 min between cascades
```

**Solutions**:

1. **Use correct success phrases**:
```
✅ Works: "works", "perfect", "thanks", "solved", "fixed it"
❌ Doesn't work: "thanks but X", "almost works", "kinda works"
```

2. **Wait for cascade timeout**:
```bash
# Cascade timeout is 5 minutes between cascades
# If you just cascaded, you must wait
# Check: cat /tmp/.escalation_$(id -u)/last_cascade_time

# Or force downgrade:
echo '{"prompt": "/escalate to haiku"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager
```

3. **Verify escalation context exists**:
```bash
# Session must be active (created by /escalate)
[ -f /tmp/.escalation_$(id -u)/escalation_session ] && \
  echo "Session exists" || echo "No active session"

# If missing, must /escalate first
```

4. **Check phrase detection logic**:
```bash
# Manually test phrase matching
phrase="Perfect! This works great."
echo "$phrase" | grep -qiE "works great|that fixed it|that works" && \
  echo "Phrase detected" || echo "Phrase not detected"
```

---

### Dashboard Not Showing Data

**Symptom**: Dashboard loads but shows 0 for all metrics

**Diagnosis**:
```bash
# Check if stats command works
~/.claude/bin/escalation-manager stats | jq .

# Check log files exist
ls -la ~/.claude/data/escalation/
# Should show: escalations.log, deescalations.log

# Check log file permissions
file ~/.claude/data/escalation/escalations.log
```

**Solutions**:

1. **Binary not outputting stats**:
```bash
# Test binary stats output
~/.claude/bin/escalation-manager stats

# If error, check bash syntax:
bash -n ~/.claude/bin/escalation-manager
```

2. **Data directory missing**:
```bash
mkdir -p ~/.claude/data/escalation
# Logs will be created on next escalation
```

3. **No escalations recorded yet**:
```bash
# Try escalation to generate data
echo '{"prompt": "/escalate to opus"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager

# Check logs
cat ~/.claude/data/escalation/escalations.log
```

4. **Dashboard can't read binary output**:
```bash
# Check dashboard permissions
ls -la ~/.claude/bin/escalation-dashboard*

# Make executable if needed
chmod +x ~/.claude/bin/escalation-dashboard*
```

---

### Dashboard Port Already in Use

**Symptom**: "Address already in use" error when starting dashboard

**Diagnosis**:
```bash
# Check what's on port 8077
lsof -i :8077

# Or use netstat
netstat -tlnp 2>/dev/null | grep 8077
```

**Solutions**:

1. **Kill existing process**:
```bash
# Find process ID
pid=$(lsof -ti :8077)
# Kill it
kill -9 $pid
# Or just pkill
pkill -f "dashboard"

# Wait a moment
sleep 2

# Restart
~/.claude/bin/escalation-dashboard 8077 &
```

2. **Use different port**:
```bash
~/.claude/bin/escalation-dashboard 8888
# Access: http://localhost:8888
```

3. **Check if zombie process**:
```bash
ps aux | grep dashboard | grep -v grep
# If shows but not responding, force kill:
pkill -9 -f "dashboard"
```

---

### Auto-Effort Not Routing Correctly

**Symptom**: Simple tasks route to Opus, complex tasks to Haiku

**Diagnosis**:
```bash
# Check auto-effort classification
prompt="design a microservices architecture"
echo "$prompt" | grep -qE "\b(plan|design|architect)" && \
  echo "Detected as high-complexity task"

# Check scoring
# Long prompt: +points
# Complexity keywords: +points
# Simple keywords: -points
```

**Solutions**:

1. **Override with /effort**:
```bash
/effort low   # Force Haiku
/effort high  # Force Opus/Sonnet
```

2. **Override with /model**:
```bash
/model haiku
/model sonnet
/model opus
```

3. **Check task classification**:
```bash
# Last task type is stored in:
cat ~/.claude/data/escalation/last_task_context

# If wrong, check keywords in your prompt
# Add explicit indicators:
# - "design architecture" → detected as planning (high)
# - "what is X" → detected as lookup (low)
```

---

### Settings.json Corrupted

**Symptom**: Jq error when trying to view settings, or settings keep reverting

**Diagnosis**:
```bash
# Check JSON validity
jq . ~/.claude/settings.json 2>&1

# If error shown, file is corrupted

# Check file size
wc -l ~/.claude/settings.json
```

**Solutions**:

1. **Restore from backup**:
```bash
# If you have a backup
cp ~/.claude/settings.json.backup ~/.claude/settings.json
```

2. **Recreate file**:
```bash
# Create minimal valid settings
cat > ~/.claude/settings.json << 'EOF'
{
  "model": "claude-sonnet-4-6",
  "effortLevel": "medium"
}
EOF
```

3. **Merge with existing settings carefully**:
```bash
# If file has other important keys:
# 1. Back up original
cp ~/.claude/settings.json ~/.claude/settings.json.broken

# 2. Create new file with model/effort preserved
jq '{model: .model, effortLevel: .effortLevel}' \
  ~/.claude/settings.json.broken > /tmp/new_settings.json

# 3. Review and replace
cat /tmp/new_settings.json > ~/.claude/settings.json
```

---

### Permission Denied Errors

**Symptom**: "Permission denied" when running escalation-manager

**Diagnosis**:
```bash
# Check file permissions
ls -la ~/.claude/bin/escalation-manager
# Should show: -rwxr-xr-x

# Check owner
ls -l ~/.claude/bin/escalation-manager | awk '{print $3, $4}'
# Should be your username
```

**Solutions**:

1. **Make executable**:
```bash
chmod +x ~/.claude/bin/escalation-manager
```

2. **Fix ownership** (if running as different user):
```bash
chown $(whoami) ~/.claude/bin/escalation-manager
```

3. **Check data directory permissions**:
```bash
chmod 755 ~/.claude/data/escalation
chmod 644 ~/.claude/data/escalation/*.log
```

---

### High Cascade Rate

**Symptom**: Dashboard shows cascade rate > 500%

**Diagnosis**:
```bash
# Check escalations vs de-escalations
escalations=$(wc -l < ~/.claude/data/escalation/escalations.log)
deescalations=$(wc -l < ~/.claude/data/escalation/deescalations.log)
echo "Escalations: $escalations, De-escalations: $deescalations"
echo "Rate: $(( deescalations * 100 / max(escalations, 1) ))%"

# Each escalation should have 1-3 cascade steps
# If > 10 per escalation, something is wrong
```

**Solutions**:

1. **Cascade timeout working?**
```bash
# Check if timeout file is being created
ls -la /tmp/.escalation_$(id -u)/last_cascade_time

# Should be updated after each cascade
# If not, timeout mechanism may be broken
```

2. **Reset logs if needed**:
```bash
# Start fresh counting
rm -f ~/.claude/data/escalation/escalations.log
rm -f ~/.claude/data/escalation/deescalations.log

# Next escalation will create new logs
```

---

### Success Phrases Not Detected

**Symptom**: Say "it works!" but no de-escalation happens

**Diagnosis**:
```bash
# Test phrase detection
phrase="it works!"
echo "$phrase" | grep -qE "works|perfect|thanks" && \
  echo "Would trigger" || echo "Not detected"

# Check for negation guards
phrase="thanks but still broken"
echo "$phrase" | grep -qE "thanks.*(but|however|still)" && \
  echo "Negation detected - blocked" || echo "Would trigger"
```

**Solutions**:

1. **Use exact phrases from list**:
```
✅ "works"        "perfect"     "thanks"
✅ "got it"       "solved it"   "that fixed it"
✅ "works great"  "ship it"     "all good"
```

2. **Avoid negation**:
```
❌ "thanks but X"
❌ "works but Y"
✅ "thanks, it works"
✅ "thanks anyway"
```

3. **Use /escalate to manually downgrade**:
```bash
/escalate to haiku   # Manual cascade to Haiku
```

---

## Getting Help

### Collect Debug Information

Before reporting issue, gather:
```bash
# System info
uname -a
bash --version
jq --version

# Configuration
jq . ~/.claude/settings.json

# Session state
ls -la /tmp/.escalation_$(id -u)/

# Recent logs (last 20 lines)
tail -20 ~/.claude/data/escalation/escalations.log
tail -20 ~/.claude/data/escalation/deescalations.log

# Test binary
~/.claude/bin/escalation-manager stats | jq .
~/.claude/bin/escalation-manager version
```

### Check Logs

Most detailed info is in log files:
```bash
# Escalation events
cat ~/.claude/data/escalation/escalations.log

# De-escalation events
cat ~/.claude/data/escalation/deescalations.log

# Current task context
cat ~/.claude/data/escalation/last_task_context
```

### Test Binary Directly

```bash
# Test escalation command
echo '{"prompt": "/escalate to opus"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager | jq .

# Test success detection
echo '{"prompt": "Perfect! Works great."}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager | jq .

# Test auto-effort
echo '{"prompt": "Implement a REST API"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager | jq .
```

### Report Issues

When opening a GitHub issue include:
1. **Symptom**: What's not working
2. **Reproduction**: Steps to reproduce
3. **Environment**: OS, bash version, Claude Code version
4. **Debug output**: Results of diagnostic commands above
5. **Logs**: Last 20 lines of escalation logs

## See Also

- [SETUP.md](SETUP.md) — Installation
- [USAGE.md](USAGE.md) — How to use
- [ARCHITECTURE.md](ARCHITECTURE.md) — System design
- [DASHBOARD.md](DASHBOARD.md) — Dashboard guide

