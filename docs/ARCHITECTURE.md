# Architecture & Technical Design

## System Overview

The escalation system is a unified state machine that manages Claude model selection based on task type and outcomes.

```
User Input (Prompt)
        ↓
┌─────────────────────────────────────┐
│   Escalation Manager (Main Binary)  │
│   Single source of truth            │
└─────────────────────────────────────┘
        ↓
    Dispatches to:
        ├─ /escalate command handler
        ├─ De-escalation (success detection)
        ├─ Auto-effort (task classification)
        └─ Stats tracking (logging)
        ↓
┌─────────────────────────────────────┐
│   Update settings.json              │
│   (atomic writes via jq)            │
└─────────────────────────────────────┘
        ↓
    Claude Code reads new model/effort
    and routes next response accordingly
```

## Hook Integration

The system runs as **Claude Code hooks** (UserPromptSubmit):

### Hook Types
1. **on-prompt** (UserPromptSubmit) — main hook on every user input
2. **pre-tool** (PreToolUse) — before tool execution
3. **post-tool** (PostToolUse) — after tool execution

Currently implemented: **on-prompt** (UserPromptSubmit)

### Hook Dispatch

```bash
# User types something → UserPromptSubmit hook fires
HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager < hook_input.json

# Input: {"prompt": "your text here"}
# Output: {"continue": true, "suppressOutput": true, "hookSpecificOutput": {...}}
```

## Core Components

### 1. /escalate Command Handler

**Purpose**: User-initiated model escalation

**Flow**:
```
Input: "/escalate to opus"
  ↓
Parse target model (opus|sonnet|haiku)
  ↓
Update settings.json with new model
  ↓
Create escalation_session (timestamp)
  ↓
Log to escalations.log
  ↓
Output: "🚀 Escalated: Opus"
```

**Files Modified**:
- `~/.claude/settings.json` → model, effortLevel
- `/tmp/.escalation_$(id -u)/escalation_session` → timestamp
- `~/.claude/data/escalation/escalations.log` → append

### 2. De-escalation (Success Detection)

**Purpose**: Cascade down models when problems are solved

**Flow**:
```
Input: "Perfect! That works great."
  ↓
Detect success signal
  (24+ phrases matched with context guards)
  ↓
Check escalation context
  (must have active escalation_session)
  ↓
Check cascade timeout
  (5-min minimum between cascades)
  ↓
Step down model: Opus → Sonnet → Haiku
  ↓
Update settings.json
  ↓
Clear session on final step (Haiku)
  ↓
Output: "⬇️ Auto-downgrade: Sonnet"
```

**Success Phrases** (24+ detected):
```
Multi-word: "works great", "that fixed it", "works perfectly",
            "got it working", "thank you", "that's exactly right"

Single-word: "perfect", "solved", "thanks" (with context guards)

With context: "no longer broken", "ship it", "all good"
```

**Context Guards**:
- ❌ "thanks but X" → ignored (negation)
- ✅ "thanks, it works" → triggers cascade
- Prevents false positives from mixed messages

### 3. Auto-Effort Routing

**Purpose**: Automatically select model based on task type

**Task Types Detected**:
```
Debugging        → High effort → Opus
Planning         → High effort → Opus
Code generation  → High effort → Opus
Documentation    → High effort → Opus
Search/lookup    → Low effort  → Haiku
Optimization     → Medium      → Sonnet
Review           → Medium      → Sonnet
[Fallback]       → Score-based → varies
```

**Scoring Algorithm**:
```
Score = base score (0)
  + length bonus
  + complexity keywords
  - simplicity indicators

Score < 0  → Haiku
Score 0-2  → Haiku or Sonnet
Score 2-6  → Sonnet
Score 6+   → Opus
```

**Flow**:
```
Input: "Implement a REST API endpoint"
  ↓
Classify task type: "code_gen"
  ↓
Get task-based model: "opus"
  ↓
Check if changed from current
  ↓
Update settings.json if needed
  ↓
Set deescalation_just_ran flag (prevents override)
  ↓
Output: "🔥 Auto-effort: high → Opus"
```

**Bypass Flags**:
1. `/escalate` command (user override)
2. `/effort` command (user override)
3. `/model` command (user override)
4. `deescalation_just_ran` marker (prevent conflict with cascade)

### 4. Stats Tracking

**Purpose**: Log all escalation/de-escalation events for analytics

**Tracked**:
- Task type classification (for phase 4 learning)
- Escalation events with timestamp and task
- De-escalation events with cascade status
- Log rotation (keep latest 200 entries)

**Output**: `escalation-manager stats` command
```json
{
  "currentState": {
    "model": "Opus",
    "fullModel": "claude-opus-4-6",
    "modelColor": "#FF6B6B",
    "effort": "HIGH",
    "lastTaskType": "debugging"
  },
  "stats": {
    "escalations": 6,
    "deescalations": 51,
    "cascadeRate": 850,
    "successRate": 85,
    "avgCascadeDepth": 8,
    "tokensCost": 10200
  },
  "metrics": {
    "totalSessions": 6,
    "problemsResolved": 51,
    "costSavings": "~150 tokens"
  }
}
```

## State Management

### Settings File

**Path**: `~/.claude/settings.json`

**Keys Used**:
```json
{
  "model": "claude-opus-4-6",    // Current model
  "effortLevel": "high"           // Current effort level
}
```

**Update Method**: Atomic (using jq + temp file)
```bash
# Ensures no corruption from concurrent updates
atomic_json_update() {
  jq 'modification' "$file" > "$tmpfile" && \
  mv "$tmpfile" "$file"
}
```

### Session State

**Path**: `/tmp/.escalation_$(id -u)/`

**Files**:
| File | Purpose | Lifetime |
|------|---------|----------|
| `escalation_session` | Current session timestamp | 30 min or end of cascade |
| `last_cascade_time` | Last cascade timestamp | 5 min (timeout) |
| `deescalation_just_ran` | Flag to prevent auto-effort override | 3 seconds |
| `last_task_context` | Most recent task type | Until next classification |

### Data Logs

**Path**: `~/.claude/data/escalation/`

**Files**:
| File | Format | Retention |
|------|--------|-----------|
| `escalations.log` | Timestamped escalation events | Latest 200 |
| `deescalations.log` | Timestamped cascade events | Latest 200 |
| `last_task_context` | Task type string | Latest only |

## Cascade Mechanism

The system cascades down through models when success is detected:

### Session Creation (on /escalate)
```
User: /escalate to opus
↓
escalation_session = $(date +%s)  # Session started
↓
Can de-escalate for 30 minutes
```

### Cascade Step 1 (Opus → Sonnet)
```
User: "Perfect!"
↓
detect_success_signal() → true
↓
has_escalation_context() → true
↓
has_cascade_timeout() → false
↓
step_down_model() → "claude-sonnet-4-6"
↓
escalation_session = $(date +%s)  # Refresh for next cascade
↓
last_cascade_time = $(date +%s)   # Start 5-min timeout
↓
Output: "⬇️ Auto-downgrade: Sonnet (continuing cascade)"
```

### Cascade Step 2 (Sonnet → Haiku) - FINAL
```
User: "Thanks!"
↓ [6 seconds later, past timeout check]
↓
detect_success_signal() → true
↓
has_escalation_context() → true
↓
has_cascade_timeout() → false  # 6 sec elapsed, timeout expired
↓
step_down_model() → "claude-haiku-4-5-20251001"
↓
rm -f escalation_session  # CLEAR SESSION - cascade complete
↓
last_cascade_time = $(date +%s)  # Track final cascade
↓
Output: "⬇️ Auto-downgrade: Haiku (cascade complete)"
```

### Cascade Timeout

Prevents over-optimization with repeated success signals:

```
Opus → Sonnet:       Cascades ✓
Sonnet → Haiku:      Cascades ✓
Haiku → (blocked):   Timeout until 5 min elapsed ✗
```

This prevents:
```
Bad scenario:
User: "Works great!"     → Cascades Opus→Sonnet
User: "Perfect!"         → Cascades Sonnet→Haiku
User: "Excellent!"       → Would cascade Haiku→? (blocked)
                            Timeout for 5 minutes

Good behavior:
Cascade happens once per significant interaction
Prevents thrashing between models
```

## Command Handler Priority

When user input arrives, handlers execute in order:

```
1. /escalate → STOP (user override)
   ↓ (if no match)
2. De-escalation check → STOP (if success detected)
   ↓ (if no match)
3. Auto-effort check → STOP (if needed)
   ↓ (if no match)
4. Stats tracking → Continue
   ↓
5. Pass through (continue: true)
```

## Dashboard Architecture

```
HTTP Request (port 8077)
        ↓
Python HTTP Server
        ↓
    GET /api/dashboard
        ↓
Try: escalation-manager stats
        ↓
Extract JSON fields
        ↓
Return formatted response
        ↓
        ↓ Fallback (if binary fails):
        ↓ Read log files directly
        ↓
Frontend (HTML/CSS/JS)
        ↓
Auto-refresh every 2 seconds
        ↓
Display metrics + charts
```

## Bash Implementation Details

### Key Technologies Used

| Component | Technology | Why |
|-----------|-----------|-----|
| Script | Bash 4.0+ | Lightweight, no deps |
| JSON | jq | Atomic updates, no corruption |
| Regex | Bash native + grep | Pattern matching for success/task detection |
| HTTP | Python 3 (dashboard) | Cross-platform, easy HTTP server |
| State | Temp files + atomic writes | No database dependency |
| Logging | Plain text files | Human-readable, easy to monitor |

### Performance Characteristics

- **Startup**: ~50-100ms (shell + jq startup)
- **Execution**: ~20-50ms (pattern matching + json update)
- **Memory**: ~5-10MB per invocation
- **Disk I/O**: One atomic write per escalation/cascade

### Why Not Go/Rust?

Current bash version prioritizes:
- **Simplicity** → No build toolchain needed
- **Portability** → Works on any Unix system
- **Maintainability** → Human-readable code
- **Modularity** → Clean separation of concerns

Future: Go version available for compiled distribution.

## Integration Points

### Claude Code Settings

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

### Barista Status Line (Optional)

Can display in statusline:
- Current model (Opus/Sonnet/Haiku)
- Effort level (🔥/⚙️/⚡)
- Cost indicator (1x/8x/30x)

### Future Hooks

- **pre-tool**: Block tools based on model
- **post-tool**: Track tool usage patterns
- **response**: Analyze response quality

## Design Patterns

### Atomic Updates
All settings updates use atomic operations to prevent corruption:
```bash
atomic_json_update "$file" \
  --arg key "value" \
  '.field = $key'
```

### Fallback Design
Each handler has graceful fallbacks:
- Binary fails → read from files
- Settings missing → use defaults
- Session files missing → treat as no session

### Idempotent Operations
Can re-run same operation safely:
- Escalating to current model → no-op
- De-escalating with no session → no-op
- Same success signal twice → blocked by timeout

## Future Enhancements (Phase 3+)

1. **Predictive Routing**
   - Learn which task types need which models
   - Pre-escalate known-hard tasks
   - Reduce frustration loops

2. **Cost Analytics**
   - Detailed token cost per task
   - ROI of escalations
   - Cost trends over time

3. **Performance Metrics**
   - Time to solution by model
   - Quality scoring
   - Learning effectiveness

4. **Go Compilation**
   - Single binary distribution
   - Faster startup (~5ms)
   - No shell dependency

## See Also

- [SETUP.md](SETUP.md) — Installation
- [USAGE.md](USAGE.md) — User guide
- [DASHBOARD.md](DASHBOARD.md) — Dashboard API
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Common issues

