# Token Metrics Integration - Multiple Paths

**Flexibility**: Don't depend on barista alone. Support any integration point that can access Claude's metrics.

---

## Integration Points

Any of these can send actual token metrics to our service:

### ✅ Option A: Barista Module (Recommended - Built-in)
**How**: Custom barista module parses `.context_window.current_usage`  
**When**: Post-response (has access to actual tokens)  
**Setup**: Create `~/.claude/barista/modules/escalation-validation.sh`  
**Endpoint**: `POST /api/validate`

```bash
curl -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens": 742, "actual_cost": 0.0074}'
```

---

### ✅ Option B: Custom Post-Response Hook
**How**: User creates a hook that runs AFTER Claude responds  
**When**: Post-response (has access to actual tokens)  
**Setup**: Implement in settings.json `PostResponse` hook  
**Endpoint**: `POST /api/validate`

**Example Implementation**:
```bash
#!/bin/bash
# ~/.claude/hooks/post-response-validation.sh
# Runs after Claude generates response

# Get token data from Claude's response metadata (if available)
# or from environment variables set by Claude Code

curl -X POST http://localhost:9000/api/validate \
  -d "{
    \"actual_input_tokens\": $ACTUAL_INPUT,
    \"actual_output_tokens\": $ACTUAL_OUTPUT,
    \"actual_total_tokens\": $ACTUAL_TOTAL,
    \"actual_cost\": $ACTUAL_COST
  }"
```

---

### ✅ Option C: Direct CLI Integration
**How**: Command-line tool that user runs to report metrics  
**When**: Any time (manual post-response)  
**Setup**: Create shell script wrapper  
**Endpoint**: `POST /api/validate`

**Example**:
```bash
#!/bin/bash
# ~/bin/report-tokens
# User runs: report-tokens --actual-tokens 742 --actual-cost 0.0074

curl -X POST http://localhost:9000/api/validate \
  -d "{\"actual_total_tokens\": $TOKENS, \"actual_cost\": $COST}"
```

---

### ✅ Option D: Environment Variable Integration
**How**: Claude Code exports token metrics → script reads them  
**When**: Post-response (if Claude exposes vars)  
**Setup**: Create script that reads environment  
**Endpoint**: `POST /api/validate`

**Example**:
```bash
#!/bin/bash
# Triggered by Claude Code with token env vars set

if [ -n "$CLAUDE_TOKENS_USED" ]; then
  curl -X POST http://localhost:9000/api/validate \
    -d "{\"actual_total_tokens\": $CLAUDE_TOKENS_USED}"
fi
```

---

### ✅ Option E: Standalone Metrics Collector
**How**: Background process that monitors Claude Code output  
**When**: Real-time or periodic  
**Setup**: Create daemon that tails logs/cache  
**Endpoint**: `POST /api/validate`

**Example**:
```bash
#!/bin/bash
# ~/.local/bin/token-metrics-daemon
# Runs in background, monitors barista-cache for metrics

while true; do
  # Read recent barista cache or logs
  TOKENS=$(jq '.context_window.current_usage' ~/.claude/barista-cache/latest 2>/dev/null)
  
  if [ -n "$TOKENS" ]; then
    curl -X POST http://localhost:9000/api/validate \
      -d "{\"actual_total_tokens\": $TOKENS}"
  fi
  
  sleep 5
done
```

---

### ✅ Option F: IDE Extension / Plugin
**How**: VSCode/IDE extension reports metrics  
**When**: Post-response (IDE has visibility)  
**Setup**: Install plugin, configure endpoint  
**Endpoint**: `POST /api/validate`

**Example plugin snippet**:
```javascript
// VSCode extension
const response = await vscode.commands.executeCommand('claude.getLastResponse');
if (response && response.tokens) {
  await fetch('http://localhost:9000/api/validate', {
    method: 'POST',
    body: JSON.stringify({
      actual_total_tokens: response.tokens.total,
      actual_cost: response.tokens.cost
    })
  });
}
```

---

### ✅ Option G: Browser/Web Integration
**How**: Web dashboard or Claude.ai extension  
**When**: Post-response (web has token info)  
**Setup**: Install browser extension, configure endpoint  
**Endpoint**: `POST /api/validate`

---

### ✅ Option H: API Webhook
**How**: Claude Code calls webhook → we process  
**When**: Post-response (if webhook supported)  
**Setup**: Configure Claude Code to call our endpoint  
**Endpoint**: `POST /api/validate`

**Configuration in settings.json**:
```json
{
  "webhooks": {
    "onResponse": "http://localhost:9000/api/validate"
  }
}
```

---

## Hook Integration Paths

### The Three Reporting Points

```
┌─────────────────────────────────────────┐
│ PRE-RESPONSE (Hook Time)                │
├─────────────────────────────────────────┤
│ Hook computes estimates                 │
│ POST /api/metrics/hook (optional)       │
│ Returns: validation_id, routing         │
└─────────────────────────────────────────┘
           ↓ Claude generates
┌─────────────────────────────────────────┐
│ POST-RESPONSE (Multiple Options)        │
├─────────────────────────────────────────┤
│ PICK ONE (or combine):                  │
│                                         │
│ • Barista module reads metrics          │
│ • Post-response hook captures metrics   │
│ • Daemon monitors and reports           │
│ • IDE extension sends metrics           │
│ • Manual CLI command reports            │
│ • Environment variables parsed          │
│ • Webhook called by Claude              │
│                                         │
│ All POST /api/validate (same endpoint)  │
└─────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────┐
│ SERVICE (Unifies All Sources)           │
├─────────────────────────────────────────┤
│ Receives metrics from ANY source        │
│ Matches with estimate from hook         │
│ Calculates error percentage             │
│ Stores complete validation record       │
│ Returns: success, validation_id         │
└─────────────────────────────────────────┘
```

---

## Server-Side Agnostic Design

Our service endpoint `/api/validate` accepts metrics from **any source**:

```json
// ANY of these can POST here
POST /api/validate
{
  "actual_input_tokens": 268,
  "actual_cache_creation_tokens": 0,
  "actual_cache_read_tokens": 0,
  "actual_output_tokens": 474,
  "actual_total_tokens": 742,
  "actual_cost": 0.0074
}
```

**Service doesn't care**:
- ✅ What tool sent it
- ✅ How metrics were obtained
- ✅ When it was sent
- ✅ How many sources report

---

## Recommended Priority

1. **Try Option A (Barista)** — Easiest, built-in, most reliable
2. **Fallback to Option D (Env vars)** — If Claude exposes token vars
3. **Fall back to Option E (Daemon)** — Lightweight, independent
4. **For IDE users → Option F** — Native integration
5. **For web users → Option G** — Browser extension

---

## Setup Strategy: Multiple Sources

**Best approach**: Support MULTIPLE sources simultaneously

```bash
# Install primary (barista)
~/.claude/barista/modules/escalation-validation.sh

# Install fallback (daemon)
~/.local/bin/token-metrics-daemon &

# Install CLI (manual reporting)
~/.local/bin/report-tokens

# All send to same endpoint
# Service deduplicates and aggregates
```

---

## Testing Each Integration

### Test Barista Module
```bash
# Simulate barista sending metrics
curl -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens": 742}'
```

### Test Post-Hook
```bash
# Simulate post-response hook
curl -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens": 742}'
```

### Test Daemon
```bash
# Simulate daemon reporting
curl -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens": 742}'
```

### Verify Service Received All
```bash
# Check validation stats (should show multiple records)
curl http://localhost:9000/api/validation/stats | jq '.total_metrics'
```

---

## Data Format (Flexible)

Service accepts **partial data** and fills in defaults:

```javascript
// Full data (ideal)
{
  "actual_input_tokens": 268,
  "actual_cache_creation_tokens": 0,
  "actual_cache_read_tokens": 0,
  "actual_output_tokens": 474,
  "actual_total_tokens": 742,
  "actual_cost": 0.0074
}

// Minimal data (works too)
{
  "actual_total_tokens": 742
}

// Service calculates missing values
```

---

## Recommended Architecture

### For Maximum Resilience:

```
Hook (estimates)
  ↓ POST /api/metrics/hook
Service (stores estimate)
  ↓
[Pick multiple post-response sources]
  • Barista module (primary)
  • Post-hook (backup)
  • Daemon (safety net)
  ↓ All POST /api/validate
Service (matches with estimate)
  ↓
Dashboard (shows both sides)
```

**Benefit**: If barista fails, daemon still reports. If daemon fails, post-hook still works.

---

## Not Barista-Only

✅ Barista is ONE option (easiest)  
✅ But service accepts metrics from ANY source  
✅ User can choose their preferred integration  
✅ Can combine multiple sources  
✅ No vendor lock-in to barista  

---

## Summary

| Option | Ease | Reliability | Setup | Fallback |
|--------|------|-------------|-------|----------|
| **Barista** | ⭐⭐⭐ | High | 5m | No |
| **Post-Hook** | ⭐⭐ | High | 10m | Yes (CLI) |
| **Daemon** | ⭐⭐ | Medium | 10m | Yes (Manual) |
| **CLI** | ⭐⭐⭐ | Manual | 5m | Always works |
| **IDE Ext** | ⭐ | High | 15m | Fallback needed |
| **Env Vars** | ⭐⭐ | Depends on Claude | 5m | Yes |

**Recommendation**: Start with Barista (easiest), add Daemon as fallback.

