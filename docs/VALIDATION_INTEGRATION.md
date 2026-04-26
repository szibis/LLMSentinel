# Cost Validation & Token Metrics Integration

**Objective**: Compare our hook's estimated token usage against Claude's actual token consumption to validate cost savings claims.

## Discovery: Where Claude Tracks Token Metrics

### Token Data Available in Claude Code

Claude Code **exposes token metrics to barista** through the statusline input JSON:

```json
{
  "context_window": {
    "context_window_size": 200000,
    "current_usage": {
      "input_tokens": 2500,
      "cache_creation_input_tokens": 300,
      "cache_read_input_tokens": 800,
      "output_tokens": 450
    }
  }
}
```

**Key insight**: This data is available AFTER Claude generates responses, but hooks run BEFORE generation.

### Solution: Two-Phase Validation

1. **Phase 1 (Hook/Pre-response)**: Record estimated tokens
2. **Phase 2 (Post-response)**: Capture actual tokens via custom metrics integration

---

## Implementation: Token Metrics Capture

### Option A: Custom Barista Module (Recommended)

Create a barista module that captures token metrics and sends them to our service.

**File**: `~/.claude/barista/modules/escalation-validation.sh`

```bash
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
    
    # Send to validation endpoint
    if [ "$total_tokens" -gt 0 ]; then
        curl -s -X POST http://localhost:9000/api/validate \
            -H "Content-Type: application/json" \
            -d "{
                \"actual_input_tokens\": $input_tokens,
                \"actual_cache_creation_tokens\": $cache_creation,
                \"actual_cache_read_tokens\": $cache_read,
                \"actual_output_tokens\": $output_tokens,
                \"actual_total_tokens\": $total_tokens,
                \"actual_cost\": $(echo "scale=6; $total_tokens * 0.00001" | bc)
            }" &
    fi
}
```

**Installation**:
```bash
# 1. Create module file
cp escalation-validation.sh ~/.claude/barista/modules/

# 2. Add to barista.conf
echo 'MODULE_ESCALATION_VALIDATION="true"' >> ~/.claude/barista/barista.conf

# 3. Add to module order
sed -i 's/MODULE_ORDER=.*/MODULE_ORDER="...,escalation-validation,..."/' ~/.claude/barista/barista.conf
```

### Option B: Hook Enhancement (Fallback)

If barista integration is unavailable, enhance the hook to estimate output tokens:

**File**: `hooks/http-hook.sh` (enhanced)

```bash
#!/bin/bash
set -o pipefail

SERVICE_URL="${ESCALATION_SERVICE_URL:-http://localhost:9000}"
TIMEOUT=5

read -r PROMPT

# Estimate output tokens based on prompt complexity
# Rule: output tokens ≈ (input_tokens / 4) + base
PROMPT_TOKENS=$(echo "$PROMPT" | wc -w)
ESTIMATED_INPUT=$((PROMPT_TOKENS * 4))  # ~4 chars per token
ESTIMATED_OUTPUT=$((ESTIMATED_INPUT / 4 + 100))  # heuristic

# Send to hook endpoint with estimates
RESPONSE=$(curl -s -m $TIMEOUT -X POST \
  -H "Content-Type: application/json" \
  -d "{
    \"prompt\": \"$PROMPT\",
    \"estimated_input_tokens\": $ESTIMATED_INPUT,
    \"estimated_output_tokens\": $ESTIMATED_OUTPUT
  }" \
  "$SERVICE_URL/api/hook" 2>/dev/null)

echo "$RESPONSE"
```

---

## API Integration

### New Endpoint: POST /api/validate

Accepts actual token metrics from barista or other sources.

**Request** (from barista module or post-response tracking):
```json
{
  "actual_input_tokens": 2500,
  "actual_cache_creation_tokens": 300,
  "actual_cache_read_tokens": 800,
  "actual_output_tokens": 450,
  "actual_total_tokens": 4050,
  "actual_cost": 0.0405
}
```

**Response**:
```json
{
  "success": true,
  "matched": true,
  "token_error_percent": -3.2,
  "cost_error_percent": -2.8,
  "validation_id": 42
}
```

### Updated Hook Endpoint: POST /api/hook

Now accepts optional token estimates.

**Request**:
```json
{
  "prompt": "/escalate to opus",
  "estimated_input_tokens": 450,
  "estimated_output_tokens": 320
}
```

**Response**:
```json
{
  "continue": true,
  "suppressOutput": true,
  "action": "escalate",
  "currentModel": "opus",
  "validation_id": 42
}
```

---

## Dashboard Integration

### New Validation Tab

Add to dashboard:

```html
<section id="validation">
  <h2>📊 Token Cost Validation</h2>
  
  <div class="validation-summary">
    <metric>
      <label>Total Validations</label>
      <value id="total-validations">0</value>
    </metric>
    <metric>
      <label>Avg Token Error</label>
      <value id="avg-token-error">0.0%</value>
    </metric>
    <metric>
      <label>Avg Cost Error</label>
      <value id="avg-cost-error">0.0%</value>
    </metric>
  </div>
  
  <table id="validation-table">
    <thead>
      <tr>
        <th>Timestamp</th>
        <th>Task Type</th>
        <th>Estimated</th>
        <th>Actual</th>
        <th>Error %</th>
        <th>Status</th>
      </tr>
    </thead>
    <tbody></tbody>
  </table>
</section>
```

### Chart: Estimated vs Actual Tokens

```javascript
// Fetch validation data
fetch('/api/validation/metrics')
  .then(r => r.json())
  .then(data => {
    // Plot estimated vs actual on scatter chart
    // x-axis: estimated tokens
    // y-axis: actual tokens
    // diagonal line: perfect match
    // points above: over-estimated
    // points below: under-estimated
  })
```

---

## Data Collection Workflow

### Per-Session Flow

```
User types prompt
    ↓
Hook receives prompt
    ├─ Detects /escalate, success, effort
    ├─ Estimates input/output tokens based on prompt length
    ├─ Records to DB as "validation_metric" (estimated)
    ├─ Returns model change to Claude
    └─ Sends validation_id back
    ↓
Claude processes prompt
    ↓
Claude generates response
    ↓
Barista module runs (post-response)
    ├─ Reads .context_window.current_usage from statusline
    ├─ Extracts actual token counts
    ├─ Sends to /api/validate with validation_id
    └─ Service matches and calculates errors
    ↓
Dashboard refreshes
    └─ Shows estimated vs actual side-by-side
```

---

## Success Criteria for Validation

### Phase 1: Data Collection (Week 1)
- [ ] Barista module successfully captures token data
- [ ] Hook records estimation data
- [ ] Service receives actual token metrics
- [ ] Database stores 100+ validation records
- [ ] Validation tab appears in dashboard

### Phase 2: Accuracy Validation (Week 2)
- [ ] Task classification accuracy: 85%+
  ```
  (correct_model_matches / total_predictions) >= 0.85
  ```
- [ ] Token estimation error: ±15%
  ```
  |estimated - actual| / actual <= 0.15
  ```
- [ ] Cost error: ±10%
  ```
  |estimated_cost - actual_cost| / actual_cost <= 0.10
  ```

### Phase 3: Savings Validation (Week 3)
- [ ] Cascade effectiveness: 40%+ token savings
  ```
  (baseline_tokens - cascade_tokens) / baseline_tokens >= 0.40
  ```
- [ ] Model distribution: Haiku 40%+, Sonnet 35%+, Opus 25%-
- [ ] De-escalation success: 60%+ cascade success rate

---

## Troubleshooting

### Validation data not appearing

**Check 1: Barista module installed**
```bash
ls -la ~/.claude/barista/modules/escalation-validation.sh
```

**Check 2: Service running**
```bash
curl http://localhost:9000/api/health
```

**Check 3: Validation endpoint working**
```bash
curl -X POST http://localhost:9000/api/validate \
  -H "Content-Type: application/json" \
  -d '{"actual_total_tokens": 1000, "actual_cost": 0.01}'
```

**Check 4: Database has records**
```bash
# Check database size (should be > 50KB after 100 validations)
ls -lh ~/.claude/data/escalation/escalation.db
```

### Token numbers look wrong

**Validate Claude's token accounting**:
```bash
# Read a recent session's token metrics
jq '.context_window.current_usage' ~/.claude/sessions/*.json
```

**Check cost calculation**:
- Haiku: $0.80/M input, $2.40/M output
- Sonnet: $3/M input, $15/M output  
- Opus: $15/M input, $60/M output

---

## Next Actions

1. **Install validation module** (10 min)
   ```bash
   cp escalation-validation.sh ~/.claude/barista/modules/
   echo 'MODULE_ESCALATION_VALIDATION="true"' >> ~/.claude/barista/barista.conf
   ```

2. **Deploy updated service** (5 min)
   ```bash
   go build -o escalation-manager ./cmd/claude-escalate
   cp escalation-manager ~/.local/bin/
   ```

3. **Start data collection** (ongoing)
   - Use escalation system normally
   - Validation metrics collect automatically
   - Dashboard updates in real-time

4. **Review weekly reports** (10 min/week)
   - Check accuracy metrics
   - Identify patterns
   - Adjust estimates if needed

---

## Expected Findings

### If validation succeeds ✅
- Estimated tokens match actual within ±15%
- Task classification accuracy: 85%+
- Cascade savings verified: 40-60% reduction
- Dashboard shows cost benefits real

### If validation reveals gaps ⚠️
- Adjust effort detection heuristics
- Update token estimation formulas
- Refine model routing logic
- Document findings in COST_VALIDATION.md

---

## Future Enhancements

- [ ] ML-based effort classification (instead of keyword heuristics)
- [ ] Per-model token cost predictions
- [ ] Real-time savings calculator
- [ ] Anomaly detection for unusual patterns
- [ ] Integration with Claude's official metrics API (if available)
