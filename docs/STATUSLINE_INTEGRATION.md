# Statusline Plugin Integration

**Endpoint**: `GET /api/statusline`

Any statusline plugin (barista, custom plugins, or status tools) can query this endpoint to get live escalation and validation metrics for display.

---

## API Response Format

```json
{
  "model": "haiku",
  "effort": "medium",
  "timestamp": "2026-04-25T15:55:01Z",
  
  "escalations": 5,
  "de_escalations": 3,
  "turns": 42,
  
  "validations": 42,
  "accuracy": 96.5,
  "avg_token_error": -3.5,
  
  "estimated_tokens": 12340,
  "actual_tokens": 11920,
  "tokens_saved": 420,
  "savings_percent": 3.4,
  
  "estimated_cost": 0.1234,
  "actual_cost": 0.1192,
  "cost_saved": 0.0042,
  
  "service": "escalation-manager",
  "version": "2.0"
}
```

---

## For Barista Users

### Option A: Use Existing Barista Integration

If you already have barista configured with escalation-validation module:

```bash
# Barista can query this endpoint in its module
jq_query=$(curl -s http://localhost:9000/api/statusline)
model=$(echo $jq_query | jq -r '.model')
accuracy=$(echo $jq_query | jq -r '.accuracy')

echo "Model: $model | Accuracy: ${accuracy}%"
```

### Option B: Create Custom Barista Module

**File**: `~/.claude/barista/modules/escalation-metrics.sh`

```bash
#!/bin/bash
module_escalation_metrics() {
  # Query service for live metrics
  metrics=$(curl -s http://localhost:9000/api/statusline)
  
  if [ $? -ne 0 ]; then
    return 1
  fi
  
  local model=$(echo "$metrics" | jq -r '.model // "unknown"')
  local effort=$(echo "$metrics" | jq -r '.effort // "unknown"')
  local accuracy=$(echo "$metrics" | jq -r '.accuracy // 0')
  local saved=$(echo "$metrics" | jq -r '.savings_percent // 0')
  
  # Format for display
  local result="🔀 $model ($(printf "%.0f" "$accuracy")%)"
  
  if [ "$effort" != "unknown" ]; then
    case "$effort" in
      low)    result="$result ⚡L" ;;
      medium) result="$result ⚙M" ;;
      high)   result="$result 🔥H" ;;
    esac
  fi
  
  if [ $(echo "$saved > 0" | bc) -eq 1 ]; then
    result="$result 💰$(printf "%.1f" "$saved")%"
  fi
  
  echo "$result"
}
```

---

## For Custom Statusline Plugins

### Creating a Plugin That Uses This Endpoint

**Example**: Custom status tool that displays escalation metrics

```bash
#!/bin/bash
# ~/.local/bin/show-escalation-status

SERVICE_URL="${ESCALATION_SERVICE_URL:-http://localhost:9000}"

# Query the statusline endpoint
metrics=$(curl -s "$SERVICE_URL/api/statusline")

# Extract and display
model=$(echo "$metrics" | jq -r '.model')
accuracy=$(echo "$metrics" | jq -r '.accuracy')
saved=$(echo "$metrics" | jq -r '.savings_percent')

echo "Escalation Status:"
echo "  Model:    $model"
echo "  Accuracy: ${accuracy}%"
echo "  Savings:  ${saved}%"
```

### VSCode Extension Example

```javascript
// Get escalation metrics for display in VSCode statusline
const response = await fetch('http://localhost:9000/api/statusline');
const metrics = await response.json();

const statusItem = vscode.window.createStatusBarItem();
statusItem.text = `$(zap) ${metrics.model} ${metrics.accuracy.toFixed(0)}%`;
statusItem.show();
```

---

## What Each Field Means

| Field | Meaning | Example |
|-------|---------|---------|
| `model` | Current active model | `"haiku"`, `"sonnet"`, `"opus"` |
| `effort` | Detected task difficulty | `"low"`, `"medium"`, `"high"` |
| `escalations` | Times escalated up | `5` |
| `de_escalations` | Times downgraded | `3` |
| `validations` | Validation records collected | `42` |
| `accuracy` | Estimate accuracy % | `96.5` (higher is better) |
| `avg_token_error` | Average estimation error | `-3.5%` (negative = under-estimate) |
| `estimated_tokens` | What we predicted total | `12340` |
| `actual_tokens` | What Claude actually used | `11920` |
| `tokens_saved` | Difference (if positive) | `420` |
| `savings_percent` | % saved vs estimate | `3.4%` |
| `estimated_cost` | Predicted cost | `$0.1234` |
| `actual_cost` | Real cost | `$0.1192` |
| `cost_saved` | Cost reduction | `$0.0042` |

---

## Plugin Display Ideas

### Minimal (Single Line)
```
🔀 Opus 94% 💰2.1%
```

### Detailed (Multiple Stats)
```
Model: Haiku | Accuracy: 96% | Saved: 3.4% | Cost: $0.0042
```

### Dashboard-Style
```
┌─ Escalation Status ─────────────┐
│ Model:     Haiku                │
│ Accuracy:  96.5%                │
│ Savings:   3.4% (420 tokens)    │
│ Cost Saved: $0.0042             │
└─────────────────────────────────┘
```

---

## Integration with Barista

### Current Barista Users

If you're already running barista, you can:

1. **Keep existing module** (if you have escalation-validation.sh)
2. **Add new module** that queries statusline endpoint
3. **Both work together** — No conflict

### Add to barista.conf

```bash
MODULE_ESCALATION_STATUS="true"   # New module that uses /api/statusline
MODULE_ESCALATION_VALIDATION="true"  # Old module that reports metrics (optional)
```

---

## Zero-Dependency Integration

The statusline endpoint requires NO special software:

```bash
# Any tool can use it
curl http://localhost:9000/api/statusline | jq '.model'

# Any plugin can query it
fetch('http://localhost:9000/api/statusline').then(r => r.json())

# Any shell script can consume it
metrics=$(curl -s http://localhost:9000/api/statusline)
```

---

## Health Checks

### Verify Service is Running

```bash
curl http://localhost:9000/api/statusline
```

**Success**: Returns JSON with all metrics  
**Failure**: Connection refused (service not running)

### Verify Endpoint Accessibility

```bash
# From any directory/environment
curl -s http://localhost:9000/api/statusline | jq '.service'
# Output: "escalation-manager"
```

---

## Example Integrations

### 1. Barista Module (Recommended)

```bash
# ~/.claude/barista/modules/escalation-status.sh
module_escalation_status() {
  local metrics=$(curl -s http://localhost:9000/api/statusline 2>/dev/null)
  [ -z "$metrics" ] && return 1
  
  local model=$(echo "$metrics" | jq -r '.model')
  local acc=$(echo "$metrics" | jq -r '.accuracy')
  echo "🔀 $model (${acc%.*}%)"
}
```

### 2. Shell Alias

```bash
alias escalate-status='curl -s http://localhost:9000/api/statusline | jq .'
```

### 3. Zsh Prompt Function

```bash
# In ~/.zshrc
escalation_status() {
  local metrics=$(curl -s http://localhost:9000/api/statusline 2>/dev/null)
  [ -z "$metrics" ] && return
  
  local model=$(echo "$metrics" | jq -r '.model')
  local acc=$(echo "$metrics" | jq -r '.accuracy')
  echo "%F{cyan}[esc:$model:${acc%.*}%]%f"
}

PROMPT='$(escalation_status) $ '
```

### 4. CLI Tool

```bash
#!/bin/bash
# ~/.local/bin/esc

case "$1" in
  status)
    curl -s http://localhost:9000/api/statusline | jq '.'
    ;;
  model)
    curl -s http://localhost:9000/api/statusline | jq -r '.model'
    ;;
  savings)
    curl -s http://localhost:9000/api/statusline | jq '.savings_percent'
    ;;
  accuracy)
    curl -s http://localhost:9000/api/statusline | jq '.accuracy'
    ;;
  *)
    echo "Usage: esc {status|model|savings|accuracy}"
    ;;
esac
```

---

## Performance Notes

- **Response time**: < 50ms (local HTTP call)
- **Data freshness**: Real-time (queries live database)
- **No caching**: Fresh metrics on every call
- **Load**: Negligible (simple query)

Safe to call frequently (every second) if desired.

---

## Security

- **Localhost only**: Service runs on 127.0.0.1:9000
- **No authentication**: Local machine access only
- **No sensitive data**: Just metrics and statistics
- **No external requests**: All data local

---

## Troubleshooting

### Endpoint returns empty/null values

Check that service is running:
```bash
curl http://localhost:9000/api/health
```

### Plugin can't reach endpoint

Verify service port:
```bash
lsof -i :9000
# Should show escalation-manager listening
```

### Metrics seem old

Ensure validation data is being collected:
```bash
curl http://localhost:9000/api/validation/stats | jq '.total_metrics'
# Should be > 0
```

---

## Summary

**Statusline endpoint** provides real-time escalation and validation metrics in a simple, consumable JSON format.

- ✅ Any plugin can query it
- ✅ No dependencies or special setup
- ✅ Works with barista, custom plugins, CLI tools
- ✅ Fast and lightweight
- ✅ Live data from database

**Use it for**:
- Displaying current model in statusline
- Showing validation accuracy
- Tracking cost savings
- Monitoring escalation activity
- Building custom dashboards

