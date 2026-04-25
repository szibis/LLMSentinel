# System Overview

High-level architecture of Claude Escalate.

## Core Concept

Claude Escalate is a **sentiment-aware, budget-conscious model routing system** that:

1. **Detects user frustration** (repeated failures, explicit signals)
2. **Automatically escalates** to better models (Haiku → Sonnet → Opus)
3. **Learns patterns** (which models work best for which tasks)
4. **Protects budget** (daily/monthly/per-model spending limits)
5. **Provides analytics** (3-phase visibility into every request)

## Three-Phase Architecture

Every user prompt goes through three phases:

```
PHASE 1: PRE-RESPONSE (T=0-0.2s)
├─ User types prompt
├─ Hook analyzes: effort, sentiment, task type
├─ Service estimates: input/output tokens
├─ Decision: which model? within budget?
└─ Result: validation record created

PHASE 2: DURING-RESPONSE (T=0.5-2s)
├─ Claude generating response
├─ Real-time token tracking
├─ Sentiment sampling (user impatient? pausing?)
├─ Budget warning if approaching limit
└─ Result: partial validation record updated

PHASE 3: POST-RESPONSE (T=2.5-3s)
├─ Response complete
├─ Actual tokens extracted
├─ Sentiment fully assessed (success? confused? frustrated?)
├─ Decision: escalate, de-escalate, or continue?
├─ Learning stored: this pattern, this outcome
└─ Result: validation record finalized + database updated
```

## Component Architecture

```
┌─────────────────────────────────────────────────┐
│             Claude Code Session                 │
│  (user types prompts, reads responses)          │
└────────────────┬────────────────────────────────┘
                 │ UserPromptSubmit hook
                 ▼
┌─────────────────────────────────────────────────┐
│      HTTP Hook (3 lines bash)                   │
│  read -r PROMPT; curl -X POST /api/hook         │
└────────────────┬────────────────────────────────┘
                 │ HTTP POST
                 ▼
┌─────────────────────────────────────────────────┐
│    Escalation Service (Go Binary)               │
│  localhost:9000                                 │
├─────────────────────────────────────────────────┤
│ • POST /api/hook (Phase 1)                      │
│   ├─ Analyze prompt                             │
│   ├─ Estimate tokens                            │
│   ├─ Detect effort & signals                    │
│   ├─ Create validation record                   │
│   └─ Update settings.json                       │
│                                                  │
│ • POST /api/validate (Phase 3)                  │
│   ├─ Receive actual metrics                     │
│   ├─ Compare vs estimate                        │
│   ├─ Calculate accuracy                         │
│   ├─ Update validation record                   │
│   └─ Store learning                             │
│                                                  │
│ • GET /api/statusline                           │
│   └─ Real-time metrics (Phase 2)                │
│                                                  │
│ • GET /api/analytics/* (All phases)             │
│   └─ Full data for dashboards                   │
│                                                  │
│ • GET /api/validation/* (Dashboard)             │
│   └─ Historical records & stats                 │
└────────────────┬────────────────────────────────┘
                 │
      ┌──────────┼──────────┐
      ▼          ▼          ▼
   settings.json SQLite    Dashboard
                  db        http://localhost:9000
```

## Key Systems

### 1. Effort Detection
Analyzes prompt to determine complexity:
- **LOW**: "What is", "How do I", simple questions → Haiku
- **MEDIUM**: Code review, debugging → Sonnet
- **HIGH**: Architecture, complex design, debugging errors → Opus

### 2. Signal Detection
Looks for keywords indicating:
- **Frustration**: "still broken", "again", "not working"
- **Success**: "thanks", "perfect", "works great"
- **Confusion**: "why", "confused", "don't understand"
- **Commands**: "/escalate", "/escalate to opus", "/help"

### 3. Token Estimation
Predicts tokens for a given prompt/model:
- Input tokens: ~4 chars per token
- Output tokens: baseline + model overhead
- Cost estimation: tokens × model rate

### 4. Sentiment Detection
Infers user emotional state from:
- **Explicit signals**: Keywords in prompt
- **Implicit signals**: Timing (rapid follow-ups = impatient)
- **Response quality**: Did previous response help?

### 5. Budget Enforcement
Tracks spending against limits:
- Daily budget (resets midnight UTC)
- Monthly budget (resets 1st)
- Per-model limits (Opus $5/day, etc.)
- Per-task-type limits (concurrency 5k tokens, etc.)

### 6. Learning System
Stores outcomes to improve routing:
- (task_type, model, sentiment) → success_rate
- Example: "concurrency on Haiku has 40% success"
- Used for predictive routing next time

## Data Model

```go
type ValidationMetric struct {
  // Identification
  ID        string
  Timestamp time.Time
  
  // User Input
  Prompt    string
  TaskType  string
  Sentiment string  // detected user sentiment
  
  // Phase 1: Estimation
  DetectedEffort      string  // low, medium, high
  RoutedModel         string  // haiku, sonnet, opus
  EstimatedInputTokens   int
  EstimatedOutputTokens  int
  EstimatedTotalTokens   int
  EstimatedCost       float64
  
  // Phase 2: Progress
  RealTimeTokens    int  // tokens flowing during generation
  RealTimeProgress  float64  // percentage complete
  SentimentSignal   string  // user's current mood
  
  // Phase 3: Validation
  ActualInputTokens   int
  ActualOutputTokens  int
  ActualTotalTokens   int
  ActualCost       float64
  
  // Results
  TokenError       float64  // (actual - estimated) / estimated
  CostError        float64
  Success          bool
  ValidationTime   time.Time
}
```

## API Endpoints

### Phase 1: Estimation
```
POST /api/hook
├─ Input:  { "prompt": "..." }
└─ Output: { "continue": true, "currentModel": "haiku", "validationId": 42 }
```

### Phase 2: Real-time
```
GET /api/statusline
├─ Output: Current token metrics, model, sentiment
└─ Polled every 500ms during generation
```

### Phase 3: Validation
```
POST /api/validate
├─ Input:  { "validation_id": 42, "actual_total_tokens": 493 }
└─ Output: { "success": true, "error": -1.4% }
```

### Analytics
```
GET /api/analytics/phase-1/{validation_id}  → Estimation data
GET /api/analytics/phase-2/{validation_id}  → Progress data
GET /api/analytics/phase-3/{validation_id}  → Results + learning

GET /api/analytics/sentiment-trends         → User emotion patterns
GET /api/analytics/budget-status            → Spending overview
GET /api/analytics/model-satisfaction       → (task, model) → success rate
```

## Database Schema

```sql
-- Validation records (created in Phase 1, updated in Phase 3)
CREATE TABLE validation_metrics (
  id TEXT PRIMARY KEY,
  timestamp DATETIME,
  prompt TEXT,
  task_type TEXT,
  detected_effort TEXT,
  routed_model TEXT,
  estimated_input_tokens INT,
  estimated_output_tokens INT,
  actual_input_tokens INT,
  actual_output_tokens INT,
  token_error FLOAT,
  sentiment TEXT,
  success BOOLEAN,
  validated BOOLEAN
);

-- Learning outcomes (for predictive routing)
CREATE TABLE sentiment_outcomes (
  task_type TEXT,
  model TEXT,
  sentiment TEXT,
  success BOOLEAN,
  tokens INT,
  duration_seconds FLOAT,
  timestamp DATETIME
);

-- Budget tracking
CREATE TABLE budget_history (
  date TEXT,
  model TEXT,
  tokens INT,
  cost_usd FLOAT,
  timestamp DATETIME
);

-- Escalation events
CREATE TABLE escalations (
  from_model TEXT,
  to_model TEXT,
  reason TEXT,
  success BOOLEAN,
  timestamp DATETIME
);
```

## Deployment

### Minimal Setup
```bash
# 1. Install binary
cp escalation-manager ~/.local/bin/

# 2. Create 3-line hook
cat > ~/.claude/hooks/http-hook.sh << 'EOF'
#!/bin/bash
read -r PROMPT
curl -s -X POST http://localhost:9000/api/hook -d "{\"prompt\":\"$PROMPT\"}"
EOF

# 3. Start service
escalation-manager service --port 9000 &

# 4. Add to settings.json
# (see Quick Start guide)
```

### Production Setup
```bash
# 1-3. Same as above, plus:

# 4. Start monitor daemon
escalation-manager monitor --port 9000 &

# 5. Enable sentiment detection
escalation-manager config set sentiment.enabled true

# 6. Set budget limits
escalation-manager set-budget --daily 10.00 --monthly 100.00

# 7. Configure statusline (Barista, webhook, etc.)
escalation-manager config set statusline.sources.barista.enabled true
```

## Performance Characteristics

| Phase | Duration | Critical? | Async? |
|-------|----------|-----------|--------|
| Phase 1 (Hook) | ~100ms | YES | No - blocks prompt |
| Phase 2 (Monitor) | ~500ms poll | NO | Yes - background |
| Phase 3 (Validate) | ~50ms | NO | Yes - post-response |

**Hook timeout**: Default 5 seconds (configurable)  
**Database**: SQLite (local), auto-vacuumed quarterly  
**Storage**: ~/.claude/data/escalation/ (~10KB per 1000 validations)

## Next: Read Detailed Guides

- [3-Phase Flow](3-phase-flow.md) — Detailed walkthrough
- [Signal Detection](signal-detection.md) — Frustration/success detection
- [Sentiment Detection](sentiment-detection.md) — Emotion analysis
- [Token Validation](token-validation.md) — Accuracy tracking
