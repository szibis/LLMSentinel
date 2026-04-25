# Pure Binary Validation - No Shell Scripts

**Design**: All logic in Go binary, HTTP communication, minimal post-hook wrapper.

---

## Architecture: Binary-Only

```
┌─────────────────────────────────────────────────────────┐
│ Claude Code Session                                     │
└────────────┬────────────────────────────────────────────┘
             │ (user prompt stdin)
             ▼
┌─────────────────────────────────────────────────────────┐
│ Hook (Minimal Wrapper - 3 lines)                       │
│ ~/.claude/hooks/http-hook.sh                           │
├─────────────────────────────────────────────────────────┤
│ #!/bin/bash                                            │
│ read -r PROMPT                                         │
│ curl -s -X POST http://localhost:9000/api/hook \       │
│   -d "{\"prompt\":\"$PROMPT\"}"                        │
└────────────┬────────────────────────────────────────────┘
             │ HTTP POST /api/hook
             ▼
┌─────────────────────────────────────────────────────────┐
│ Escalation Service (Go Binary)                         │
│ localhost:9000                                         │
├─────────────────────────────────────────────────────────┤
│ POST /api/hook                                         │
│ ├─ Analyzes prompt                                     │
│ ├─ Detects effort, /escalate, success                 │
│ ├─ Estimates input/output tokens                       │
│ ├─ Creates validation_metric (estimate)                │
│ ├─ Updates settings.json                               │
│ ├─ Logs to SQLite                                      │
│ └─ Returns: routing decision + validation_id           │
│                                                         │
│ [Claude processes prompt]                              │
│                                                         │
│ POST /api/validate                                     │
│ ├─ Receives actual token metrics                       │
│ ├─ Looks up validation_id                              │
│ ├─ Compares: estimate vs actual                        │
│ ├─ Calculates errors                                   │
│ ├─ Updates validation_metric (validated)               │
│ └─ Returns: success + validation_id                    │
│                                                         │
│ GET /api/validation/metrics                            │
│ └─ Returns all validation records                      │
│                                                         │
│ GET /api/validation/stats                              │
│ └─ Returns aggregated statistics                       │
└────────────┬────────────────────────────────────────────┘
             │
    ┌────────┼────────┐
    ▼        ▼        ▼
┌────────┐ ┌────────┐ ┌──────────┐
│Settins │ │SQLite  │ │Dashboard │
│.json   │ │Database│ │   UI     │
└────────┘ └────────┘ └──────────┘
```

---

## Component Breakdown

### 1. Hook (Minimal Wrapper)

**File**: `~/.claude/hooks/http-hook.sh`

```bash
#!/bin/bash
read -r PROMPT
curl -s -X POST http://localhost:9000/api/hook -d "{\"prompt\":\"$PROMPT\"}"
```

**That's it**. 3 lines. All logic in binary.

### 2. Service (All Logic in Go Binary)

**File**: `cmd/claude-escalate/main.go` → `escalation-manager service`

When called:
```bash
escalation-manager service --port 9000
```

Starts HTTP server with endpoints:

#### Endpoint: POST /api/hook
**What it does** (pre-response):
```go
func (s *Service) handleHook(w http.ResponseWriter, r *http.Request) {
  // 1. Parse prompt
  // 2. Detect effort level (in Go)
  // 3. Detect /escalate commands (in Go)
  // 4. Detect success signals (in Go)
  // 5. Estimate tokens (in Go)
  // 6. Create ValidationMetric with estimates
  // 7. Log to SQLite
  // 8. Update settings.json
  // 9. Return routing decision + validation_id
}
```

#### Endpoint: POST /api/validate
**What it does** (post-response):
```go
func (s *Service) handleValidate(w http.ResponseWriter, r *http.Request) {
  // 1. Receive actual token metrics
  // 2. Look up validation_id
  // 3. Compare estimate vs actual
  // 4. Calculate error percentages
  // 5. Update ValidationMetric (add actuals)
  // 6. Store results
  // 7. Return success
}
```

#### Endpoint: POST /api/metrics/hook
**What it does** (explicit estimate reporting):
```go
func (s *Service) handleHookMetrics(w http.ResponseWriter, r *http.Request) {
  // 1. Receive estimated metrics from hook analysis
  // 2. Create ValidationMetric with estimates
  // 3. Log to SQLite
  // 4. Return validation_id
}
```

---

## Phase 1: Pre-Response (Hook → Service)

### Workflow
```
User: "What is machine learning?"
    ↓
Hook wrapper reads prompt
    ↓
curl POST http://localhost:9000/api/hook
    {"prompt": "What is machine learning?"}
    ↓
Service analyzes IN BINARY (Go):
  • Parse: "What is machine learning?"
  • Detect: low effort (keyword "what is")
  • Estimate input tokens: 27 chars / 4 = 7
  • Estimate output tokens: 500 (base for low effort)
  • Total estimate: 507 tokens
  • Model routing: haiku
    ↓
Service CREATES validation record:
  ValidationMetric {
    ID: 42,
    Prompt: "What is machine learning?",
    DetectedEffort: "low",
    EstimatedInputTokens: 7,
    EstimatedOutputTokens: 500,
    EstimatedTotalTokens: 507,
    RoutedModel: "haiku",
    Validated: false  ← waiting for Phase 2
  }
    ↓
Service STORES to SQLite
    ↓
Service UPDATES settings.json
  {"model": "claude-haiku-4-5-20251001"}
    ↓
Service RETURNS to hook:
  {
    "continue": true,
    "suppressOutput": true,
    "currentModel": "haiku",
    "validationId": 42
  }
    ↓
Hook returns response to Claude Code
```

---

## Phase 2: Post-Response (Integration → Service)

### Option A: Monitoring Daemon (Binary-based)

Instead of barista module, run a lightweight Go daemon:

```bash
escalation-manager monitor --port 9000
```

This daemon:
1. Monitors Claude's process output
2. Extracts token metrics (if available)
3. POSTs to /api/validate
4. Runs continuously in background

**Advantages**:
- ✅ Pure binary (no shell scripts)
- ✅ Can integrate with Claude's metrics directly
- ✅ Runs as background service
- ✅ Configurable via CLI flags

**Implementation**:
```go
// cmd/claude-escalate/monitor.go
func runMonitor(port string) {
  // Watch for token metrics from Claude Code
  // Extract actual tokens
  // POST to /api/validate
  // Loop continuously
}
```

### Option B: Post-Response Hook (Minimal HTTP Call)

If user prefers minimal setup, just a 1-line post-response hook:

```bash
curl -s -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens": 507}'
```

Hook runs after Claude generates response, calls service endpoint.

### Option C: Environment Variable Integration

Claude Code exports metrics → Binary reads them:

```bash
# If Claude sets: CLAUDE_TOKENS_USED, CLAUDE_TOKENS_INPUT, etc.
escalation-manager report-metrics
```

Binary reads environment, POSTs to service.

---

## Complete Workflow (Pure Binary + HTTP)

```
┌──────────────────────────────────────────────┐
│ Startup                                      │
├──────────────────────────────────────────────┤
│ escalation-manager service --port 9000       │
│                                              │
│ [Service runs on localhost:9000]             │
│ [Ready to receive hook and validation calls] │
└──────────────────────────────────────────────┘
        │
        ├──→ (optional) escalation-manager monitor --port 9000
        │                [Runs monitoring daemon]
        │
        └──→ (optional) escalation-manager report-metrics
                        [Periodic metric reporting]

┌──────────────────────────────────────────────┐
│ Per User Interaction                         │
├──────────────────────────────────────────────┤
│ User: "What is ML?"                          │
│    ↓                                         │
│ Hook (bash): read prompt, curl POST          │
│    ↓ HTTP                                    │
│ Service: /api/hook (Go)                      │
│    • Analyze prompt                          │
│    • Estimate tokens                         │
│    • Create validation record (estimate)     │
│    • Return routing                          │
│    ↓                                         │
│ Claude: Process and generate response        │
│    ↓                                         │
│ Monitor/Hook: Extract actual tokens          │
│    ↓ HTTP                                    │
│ Service: /api/validate (Go)                  │
│    • Receive actual metrics                  │
│    • Match with estimate                     │
│    • Calculate errors                        │
│    • Update record (validated)               │
│    ↓                                         │
│ Dashboard: Query via /api/validation/*       │
│    • Display both sides                      │
│    • Show accuracy                           │
│    • Accumulate stats                        │
└──────────────────────────────────────────────┘
```

---

## What's in the Binary

### Service Mode
```bash
escalation-manager service [--port 9000]
```
**Provides**:
- HTTP server on localhost:9000
- All endpoints: /api/hook, /api/validate, /api/validation/*
- SQLite database management
- Settings.json updates
- Dashboard UI

### Monitor Mode (NEW)
```bash
escalation-manager monitor [--port 9000] [--method {env|process|file}]
```
**Provides**:
- Continuous monitoring for token metrics
- Reads from: environment variables, process output, or files
- POSTs actual metrics to service
- Handles retries and error recovery

### Report Mode (NEW)
```bash
escalation-manager report-metrics [--tokens 507] [--cost 0.005]
```
**Provides**:
- One-time metric reporting
- Called from post-response hooks
- Simple JSON response

### Query Mode
```bash
escalation-manager validation stats
escalation-manager validation metrics [--limit 100]
escalation-manager validation compare --session-id 42
```
**Provides**:
- CLI access to validation data
- Reports generated locally
- No external tools needed

---

## File Structure (No Shell Dependencies)

```
~/.local/bin/
└── escalation-manager          ← Single binary, handles everything

~/.claude/hooks/
└── http-hook.sh               ← 3-line wrapper (only shell script)

~/.claude/settings.json         ← Managed by binary

~/.claude/data/escalation/
└── escalation.db              ← Managed by binary

http://localhost:9000/          ← Dashboard served by binary
```

---

## Implementation Checklist

### Phase 1: Service Endpoints (Done ✅)
- [x] POST /api/hook — analyzes prompt, estimates tokens
- [x] POST /api/validate — receives actual metrics
- [x] POST /api/metrics/hook — explicit estimate reporting
- [x] GET /api/validation/metrics — retrieve records
- [x] GET /api/validation/stats — summary statistics

### Phase 2: Monitor Mode (IMPLEMENTED)
- [x] Add `monitor` subcommand to main.go
- [x] Implement token metric extraction from environment, files, process
- [x] Add background loop for continuous monitoring
- [x] Implement error handling and retries (exponential backoff)
- [x] Add logging for monitoring events to ~/.claude/data/escalation/escalation.log

**Implementation**:
```go
// cmd/claude-escalate/monitor.go
func runMonitor() {
  // 1. Watch for token metrics from multiple sources:
  //    a) Environment variables: CLAUDE_TOKENS_ACTUAL, CLAUDE_TOKENS_INPUT, CLAUDE_TOKENS_OUTPUT
  //    b) Process output: Parse stdout/stderr for token counts
  //    c) Files: Watch ~/.claude/data/escalation/status.json for updates
  //
  // 2. Extract actual metrics:
  //    - Input tokens
  //    - Output tokens
  //    - Cache hit/creation tokens (if available)
  //    - Timestamp
  //
  // 3. POST to /api/validate:
  //    POST http://localhost:9000/api/validate
  //    {
  //      "validation_id": "uuid-from-phase-1",
  //      "actual_input_tokens": 268,
  //      "actual_output_tokens": 474,
  //      "actual_cache_hit_tokens": 0,
  //      "actual_cache_creation_tokens": 0
  //    }
  //
  // 4. Error handling:
  //    - Retry failed requests (exponential backoff: 100ms, 200ms, 400ms, max 10s)
  //    - Continue monitoring even if one request fails
  //    - Log all events to escalation.log
  //
  // 5. Loop continuously:
  //    - Poll every 500ms for new metrics
  //    - Stop on signal (Ctrl+C)
}
```

**Usage**:
```bash
escalation-manager monitor --port 9000 --method env
escalation-manager monitor --port 9000 --method file
escalation-manager monitor --port 9000 --method process
```

### Phase 3: Report Mode (IMPLEMENTED)
- [x] Add `report-metrics` subcommand to main.go
- [x] Accept CLI flags for token counts (--input-tokens, --output-tokens, --validation-id)
- [x] POST to /api/validate with actual metrics
- [x] Return success/failure with JSON response

**Implementation**:
```go
// cmd/claude-escalate/report.go
func runReportMetrics() {
  // 1. Parse CLI flags:
  //    --validation-id UUID          (required)
  //    --input-tokens INT            (required)
  //    --output-tokens INT           (required)
  //    --cache-hit-tokens INT        (optional, default 0)
  //    --cache-creation-tokens INT   (optional, default 0)
  //    --port 9000                   (optional, default 9000)
  //
  // 2. Validate inputs:
  //    - validation_id must be UUID format
  //    - token counts must be non-negative integers
  //
  // 3. POST to service /api/validate:
  //    POST http://localhost:9000/api/validate
  //    {
  //      "validation_id": "uuid-abc123",
  //      "actual_input_tokens": 268,
  //      "actual_output_tokens": 474,
  //      "actual_cache_hit_tokens": 0,
  //      "actual_cache_creation_tokens": 0
  //    }
  //
  // 4. Return JSON response:
  //    {
  //      "success": true,
  //      "validation_id": "uuid-abc123",
  //      "message": "Metrics recorded successfully"
  //    }
}
```

**Usage**:
```bash
escalation-manager report-metrics \
  --validation-id 42 \
  --input-tokens 268 \
  --output-tokens 474

escalation-manager report-metrics \
  --validation-id abc123 \
  --input-tokens 350 \
  --output-tokens 340 \
  --cache-hit-tokens 50 \
  --cache-creation-tokens 10 \
  --port 9000
```

### Phase 4: Query Mode (IMPLEMENTED)
- [x] Add `validation` subcommand to main.go
- [x] Implement subcommands: stats, metrics, compare with filters
- [x] Pretty-print results with formatting and colors
- [x] Support filtering/limits (--limit, --task-type, --model, --recent-hours)

**Implementation**:
```go
// cmd/claude-escalate/query.go
func runValidation(subcommand string) {
  // Available subcommands:
  
  // 1. validation stats
  //    - Aggregated statistics across all validations
  //    - Shows: total records, estimated vs actual, accuracy metrics
  //    - Pretty-print as table with headers
  //    Example output:
  //    Total Validations: 42
  //    Estimated Total Tokens: 12,340
  //    Actual Total Tokens: 11,920
  //    Tokens Saved: 420 (3.4%)
  //    Average Token Error: -3.2%
  //    Cost Accuracy: 96.8%
  //
  // 2. validation metrics [--limit 100] [--task-type concurrency] [--model opus]
  //    - List individual validation records
  //    - Filterable by task type, model, recency
  //    - Pretty-print as table with columns:
  //      | ID | Prompt | Estimated | Actual | Error | Model |
  //      | 42 | "What..." | 507 | 493 | -1.4% | haiku |
  //
  // 3. validation compare --validation-id 42
  //    - Show estimate vs actual side-by-side for single record
  //    - Pretty-print as comparison view:
  //      VALIDATION #42
  //      Prompt: "What is machine learning?"
  //      
  //      ESTIMATE (Phase 1)
  //      Input:  7 tokens
  //      Output: 500 tokens
  //      Total:  507 tokens
  //      Cost:   $0.005
  //      
  //      ACTUAL (Phase 3)
  //      Input:  6 tokens
  //      Output: 487 tokens
  //      Total:  493 tokens
  //      Cost:   $0.0049
  //      
  //      ERROR: -1.4% (excellent accuracy)
  //
  // 4. Query options:
  //    --limit N              (default 100, max 10000)
  //    --task-type STRING     (filter by detected task type)
  //    --model STRING         (filter by routed model)
  //    --recent-hours INT     (show last N hours of data, default all)
  //    --sort FIELD           (sort by field: date, error, tokens, etc.)
  //    --format TEXT|JSON     (output format)
}
```

**Usage**:
```bash
# Get overall statistics
escalation-manager validation stats

# List all validation records (last 100)
escalation-manager validation metrics --limit 100

# Filter by task type
escalation-manager validation metrics --task-type concurrency

# Filter by model
escalation-manager validation metrics --model opus

# Show only recent data
escalation-manager validation metrics --recent-hours 24

# Compare estimate vs actual for one validation
escalation-manager validation compare --validation-id 42

# JSON output for programmatic use
escalation-manager validation stats --format json

# Complex query with sorting
escalation-manager validation metrics \
  --task-type concurrency \
  --model sonnet \
  --recent-hours 48 \
  --limit 50 \
  --sort error
```

---

## Deployment (No Shell Scripts)

### Setup (5 minutes)

1. **Rebuild binary** (includes all endpoints)
   ```bash
   cd /tmp/claude-escalate
   go build -o claude-escalate ./cmd/claude-escalate
   cp claude-escalate ~/.local/bin/escalation-manager
   ```

2. **Create minimal hook** (3 lines only)
   ```bash
   cat > ~/.claude/hooks/http-hook.sh << 'EOF'
   #!/bin/bash
   read -r PROMPT
   curl -s -X POST http://localhost:9000/api/hook -d "{\"prompt\":\"$PROMPT\"}"
   EOF
   chmod +x ~/.claude/hooks/http-hook.sh
   ```

3. **Start service** (no config needed)
   ```bash
   escalation-manager service --port 9000 &
   ```

4. **Optional: Start monitor daemon**
   ```bash
   escalation-manager monitor --port 9000 &
   ```

5. **Configure hook in settings.json**
   ```json
   {
     "hooks": {
       "UserPromptSubmit": [{
         "type": "command",
         "command": "~/.claude/hooks/http-hook.sh",
         "timeout": 10
       }]
     }
   }
   ```

Done! Everything runs in binary + HTTP, no shell dependencies.

---

## Benefits of Pure Binary Approach

✅ **No bash scripts** — All logic in Go  
✅ **No external tools** — Single binary does everything  
✅ **HTTP communication** — Clean internal APIs  
✅ **Minimal hook** — Just calls curl to service  
✅ **Easy to extend** — Add new modes in Go code  
✅ **Better performance** — No shell startup overhead  
✅ **Easier testing** — Unit tests in Go  
✅ **Type-safe** — Go compiler checks everything  
✅ **Better error handling** — Structured error responses  
✅ **Portability** — Binary works on any system  

---

## Data Flow (No Shell Logic)

```
All analysis in binary:
├─ Effort detection
├─ Command parsing
├─ Token estimation
├─ Cost calculation
├─ Model routing
└─ Error calculation

All storage in binary:
├─ SQLite database
├─ JSON file updates
├─ Validation records
└─ Statistics aggregation

All communication over HTTP:
├─ Hook → Service POST /api/hook
├─ Monitor → Service POST /api/validate
├─ Dashboard ← Service GET /api/validation/*
└─ CLI → Service (internal database)
```

---

## Next Steps

1. **Rebuild binary** — Includes all new endpoints
   ```bash
   go build -o claude-escalate ./cmd/claude-escalate
   ```

2. **Create minimal hook** — Just 3 lines
   ```bash
   cat > ~/.claude/hooks/http-hook.sh << 'EOF'
   #!/bin/bash
   read -r PROMPT
   curl -s -X POST http://localhost:9000/api/hook -d "{\"prompt\":\"$PROMPT\"}"
   EOF
   ```

3. **Start service**
   ```bash
   escalation-manager service --port 9000 &
   ```

4. **Use normally** — Validation happens automatically

No shell scripts, no barista, no external dependencies. Just binary + HTTP.

