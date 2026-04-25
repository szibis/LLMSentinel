# Token Validation Quick Start (15 minutes)

**Goal**: Set up automatic cost validation and start comparing estimated vs actual token usage.

---

## Step 1: Create Barista Module (5 minutes)

Create the validation module that captures Claude's actual token metrics:

```bash
cat > ~/.claude/barista/modules/escalation-validation.sh << 'EOF'
#!/bin/bash
# Escalation Validation Module - captures token metrics for cost validation

module_escalation_validation() {
    local input="$1"
    
    # Extract token counts from Claude's statusline JSON
    local input_tokens=$(echo "$input" | jq -r '.context_window.current_usage.input_tokens // 0' 2>/dev/null)
    local cache_creation=$(echo "$input" | jq -r '.context_window.current_usage.cache_creation_input_tokens // 0' 2>/dev/null)
    local cache_read=$(echo "$input" | jq -r '.context_window.current_usage.cache_read_input_tokens // 0' 2>/dev/null)
    local output_tokens=$(echo "$input" | jq -r '.context_window.current_usage.output_tokens // 0' 2>/dev/null)
    
    # Calculate total
    local total_tokens=$((input_tokens + cache_creation + cache_read + output_tokens))
    
    # Send to validation endpoint (async, don't block)
    if [ "$total_tokens" -gt 0 ]; then
        curl -s -X POST http://localhost:9000/api/validate \
            -H "Content-Type: application/json" \
            -d "{
                \"actual_input_tokens\": $input_tokens,
                \"actual_cache_creation_tokens\": $cache_creation,
                \"actual_cache_read_tokens\": $cache_read,
                \"actual_output_tokens\": $output_tokens,
                \"actual_total_tokens\": $total_tokens,
                \"actual_cost\": $(echo "scale=6; ($input_tokens * 15 + $cache_creation * 15 + $cache_read * 15 + $output_tokens * 60) / 1000000" | bc 2>/dev/null || echo "0")
            }" >/dev/null 2>&1 &
    fi
}
EOF

chmod +x ~/.claude/barista/modules/escalation-validation.sh
```

## Step 2: Enable Module in Barista (2 minutes)

Add the module to barista configuration:

```bash
# Add to barista.conf
echo 'MODULE_ESCALATION_VALIDATION="true"' >> ~/.claude/barista/barista.conf

# Update module order to include escalation-validation
# Edit ~/.claude/barista/barista.conf and find the MODULE_ORDER line
# Add escalation-validation to the list, e.g.:
# MODULE_ORDER="...,escalation-validation,..."
```

## Step 3: Rebuild Service (3 minutes)

Update to the latest binary with validation support:

```bash
cd /tmp/claude-escalate
go build -o claude-escalate ./cmd/claude-escalate
cp claude-escalate ~/.local/bin/escalation-manager
```

## Step 4: Restart Service (2 minutes)

Restart the escalation service to activate validation endpoints:

```bash
# Stop old service
pkill -f "escalation-manager service"

# Start new service (on port 9000 or your preferred port)
escalation-manager service --port 9000 &

# Verify it's running
curl http://localhost:9000/api/health
```

## Step 5: Verify Setup (3 minutes)

Check that everything is working:

```bash
# Check barista module exists
ls -la ~/.claude/barista/modules/escalation-validation.sh

# Check service is running
curl http://localhost:9000/api/health | jq .

# Check validation endpoints exist
curl http://localhost:9000/api/validation/stats | jq .
```

**Expected output**: Should show validation endpoints responding with JSON.

---

## That's It! 🎉

The validation system is now active. Every time you use the escalation system:

1. **Hook captures**: Estimated tokens (on prompt submission)
2. **Barista captures**: Actual tokens (after response generation)
3. **Service compares**: Calculates accuracy metrics
4. **Dashboard shows**: Real-time validation results

---

## View Validation Data

### In Dashboard

Go to http://localhost:9000/ and look for the **Validation** section showing:
- Total validation records
- Average token error %
- Average cost error %
- Detailed metrics table

### Via API

```bash
# Get recent validation metrics
curl http://localhost:9000/api/validation/metrics | jq '.metrics | length'

# Get validation statistics
curl http://localhost:9000/api/validation/stats | jq '.'
```

---

## Troubleshooting

### Barista module not capturing data

**Check 1**: Module file exists
```bash
ls -la ~/.claude/barista/modules/escalation-validation.sh
# Should show the file with execute permission
```

**Check 2**: Module is enabled
```bash
grep "MODULE_ESCALATION_VALIDATION" ~/.claude/barista/barista.conf
# Should show: MODULE_ESCALATION_VALIDATION="true"
```

**Check 3**: Service is receiving data
```bash
# Use a fresh session and then check:
curl http://localhost:9000/api/validation/stats | jq '.total_metrics'
# Should show number > 0
```

### Service not responding

**Check 1**: Service is running
```bash
curl http://localhost:9000/api/health
# Should return {"status":"healthy",...}
```

**Check 2**: Port is available
```bash
lsof -i :9000
# If blocked, use different port: escalation-manager service --port 9001
```

**Check 3**: Check logs
```bash
tail -50 ~/.claude/data/escalation/escalation.log
# Or run in foreground: escalation-manager service --port 9000
```

---

## What Gets Tracked

For each interaction, the system now tracks:

| Metric | Source | Value |
|--------|--------|-------|
| Prompt text | Hook | User's input |
| Effort detected | Hook | low/medium/high |
| Routed model | Hook | haiku/sonnet/opus |
| Est. input tokens | Hook estimate | ~4 chars per token |
| Est. output tokens | Hook estimate | prompt_tokens / 4 |
| Act. input tokens | Barista → Claude | Actual measured |
| Act. output tokens | Barista → Claude | Actual measured |
| Cache tokens | Barista → Claude | If any |
| Total tokens | Both | Sum of all |
| Error % | Service calc | (est - actual) / actual |

---

## Expected Results

### Within 1 Hour
- ✅ 5-10 validation records collected
- ✅ Dashboard showing validation metrics
- ✅ First accuracy calculations visible

### Within 1 Day
- ✅ 50+ validation records
- ✅ Pattern analysis begins
- ✅ Accuracy trends visible

### Within 1 Week
- ✅ 300+ validation records
- ✅ Statistical significance achieved
- ✅ Confidence intervals established
- ✅ Cost savings validated

---

## Next Steps

After setup is complete, see **VALIDATION_INTEGRATION.md** for:
- Detailed API reference
- Advanced configuration options
- Dashboard customization
- Data analysis techniques
- Reporting and metrics

Or check **VALIDATION_FINDINGS.md** for:
- Research summary
- Technical architecture
- Known limitations
- Future enhancements

---

## Success Indicators

✅ Service starts without errors  
✅ Barista module file exists  
✅ Validation stats show records > 0  
✅ Dashboard displays validation data  
✅ Metrics update in real-time  

If all checks pass, validation is working! 🚀

Use the system normally and watch the validation data accumulate.

