# Tool Invocation & Escalation Integration

This document describes how the tool invocation system works during escalation workflows and how different task types present information to users.

## Overview

The tool invocation system provides intent-based tool selection and execution within the escalation workflow. Tools are discovered, registered, health-checked, and invoked based on the current escalation phase and user intent.

## Tool Lifecycle

### 1. Discovery Phase
Tools are discovered from multiple sources:
- **Built-in Tools**: RTK (Git), LSP (code navigation), Scrapling (web fetching)
- **MCP Servers**: Auto-detected from system sockets or manifest files
- **Custom Tools**: User-configured via dashboard or CLI
- **External APIs**: REST endpoints, database connections

Discovery happens at gateway startup and can be refreshed on-demand.

### 2. Registration Phase
When a tool is registered (via dashboard, CLI, or config file):
1. **Validation**: Path exists (CLI), socket format valid (MCP), endpoint reachable (REST)
2. **Health Check**: Initial connectivity test to ensure tool responds
3. **Capability Detection**: For MCP, discover available resources/methods
4. **Storage**: Persist to `config.yaml` for automatic load on restart
5. **Audit**: Log registration event with timestamp and operator

### 3. Monitoring Phase
Active health monitoring runs on configurable intervals (default: 5 minutes):
- **Health Status**: Per-tool status tracking (healthy/unhealthy)
- **Failure Detection**: Track consecutive failures, auto-disable after threshold
- **Recovery**: Retry unhealthy tools periodically (exponential backoff)
- **Audit Trail**: Log all health events for troubleshooting

### 4. Invocation Phase
During escalation, tools are invoked based on:
- **Escalation Phase**: Which phase of escalation permits tool use
- **Intent Matching**: User intent maps to available tools
- **Preference Weighting**: Custom tools (weight: 0.9) preferred over built-in (weight: 0.7)
- **Fallback Strategy**: If primary tool fails, try next in selection list

### 5. Output Processing Phase
Tool output is processed through a 5-step pipeline:
1. **Capture**: Get raw output from tool (subprocess stdout, socket response, HTTP body)
2. **Parse**: Detect format (JSON, YAML, plain text) and extract structure
3. **Truncate**: Enforce token budget (~2000 tokens per tool, ~3000 total)
4. **Cache**: Store results for reuse (if cache-safe and within duration)
5. **Inject**: Format as markdown and include in context window

### 6. Cleanup Phase
When tools are removed:
1. **Validation**: Check tool not in use by active escalations
2. **Backup**: Save current config before removal
3. **Removal**: Delete from runtime config and config.yaml
4. **Audit**: Log removal event with timestamp and reason

## Escalation Phases & Tool Integration

### Phase 1: Classification (No Tools)
**Purpose**: Determine user intent (quick_answer, code_search, security_check, etc.)

**Tool Availability**: DISABLED
- Reason: Intent detection must be fast; external tool calls would introduce latency
- Decision: Based on prompt keywords and history, not tool output

### Phase 2: Context Gathering (Tools Available)
**Purpose**: Gather relevant context efficiently

**Tool Availability**: ENABLED
- **Preferred Tools**: `rtk_git`, `lsp_lookup`, `rtk_grep`, `scrapling_web`
- **Timeout**: 30 seconds per tool call
- **Max Results**: 3 highest-relevance results
- **Token Budget**: 2000 tokens for all tool outputs combined

**Examples**:
- User asks about recent commits: invoke `rtk_git log`
- User asks about function usage: invoke `lsp_lookup`
- User asks about web content: invoke `scrapling_web`

### Phase 3: Model Selection (No Tools)
**Purpose**: Choose optimal model (Haiku, Sonnet, Opus)

**Tool Availability**: DISABLED
- Reason: Model selection based on intent complexity, not tool output
- Decision: Cached based on intent + escalation history

### Phase 4: Response Enhancement (Tools Available)
**Purpose**: Enhance response with additional context if needed

**Tool Availability**: ENABLED
- **Intent-Tool Mapping**:
  - `quick_answer`: Use fast tools only (Git, LSP)
  - `detailed_analysis`: Use comprehensive tools (all types)
  - `security_check`: Use security-focused tools only
  - `code_search`: Use code navigation tools (LSP, RTK)
  - `documentation`: Use content tools (Scrapling, REST)
- **Parallel Execution**: Up to 2 tools can run in parallel
- **Timeout**: 45 seconds per tool call
- **Max Results**: 2 results with complete output

### Phase 5: Response Generation (No Tools)
**Purpose**: Generate final response to user

**Tool Availability**: DISABLED
- Reason: Response generation should complete without waiting on external tools
- Decision: Use cached/buffered tool output from Phase 4

---

## Tool Selection Strategy

### Intent-Based Mapping
Each user intent maps to a set of preferred tools:

```yaml
quick_answer:
  - scrapling_web      # Fast web lookup
  - rtk_git           # Recent context
  
detailed_analysis:
  - lsp_lookup        # Code structure
  - rtk_grep          # Pattern matching
  - custom_analyzer   # User's custom tool

code_search:
  - lsp_lookup        # Symbol navigation
  - rtk_grep          # Content search
  
security_check:
  - security_scanner  # Dedicated scanner
  - lsp_lookup        # Code analysis
  
error_diagnosis:
  - rtk_git           # Recent changes
  - lsp_lookup        # Error context
```

### Weighted Selection
When multiple tools are available:
1. **Custom Tools**: Weight 0.9 (strongly preferred if healthy)
2. **Built-in Tools**: Weight 0.7 (fallback)
3. **Random Selection**: If weights equal, pick randomly
4. **Health-Based Filtering**: Exclude unhealthy tools from selection

### Fallback Strategy
If primary tool fails:
1. **Classify Failure**: Timeout, connection error, invalid output, crash, etc.
2. **Decide Action**: Retry, fallback to next tool, or skip tool invocation
3. **Log Event**: Audit failure with error details
4. **Update Status**: Mark tool unhealthy if persistent failures

---

## Error Handling & Recovery

### Error Scenarios

| Error | Trigger | Action | Retry | Fallback |
|-------|---------|--------|-------|----------|
| **Timeout** | Execution exceeds timeout_seconds | Kill subprocess | No | Yes |
| **Connection** | Can't connect to tool | Mark unhealthy | Yes (3x) | Yes |
| **Invalid Output** | Output malformed/unparseable | Discard | No | Yes |
| **Crash** | Exit code non-zero | Log error | No | Yes |
| **Rate Limit** | HTTP 429 response | Backoff exponentially | Yes (3x) | Yes |
| **Authorization** | HTTP 401/403 response | Log warning | No | No |

### Retry Strategy
- **Consecutive Failures**: Track per-tool, auto-disable after 5 failures
- **Recovery Delay**: 5 minutes before re-enabling unhealthy tool
- **Exponential Backoff**: Wait 5s, then 10s, then 20s between retries
- **Max Retries**: 3 attempts per tool per request

---

## User Transparency Modes

How the system presents tool invocation to users depends on task type:

### Quick Mode (Default)
**User Command**: `claude code "What's the latest Redis memory-safe mode?"`

**User Experience**: Black box
- Only final answer shown
- Tool invocation completely hidden
- No intermediate results
- Fastest response (tools run in background)

**Internal**: Tools invoked in Phase 2 & 4, results cached and inserted into context

---

### Research Mode (--research flag)
**User Command**: `claude code --research "Analyze concurrency patterns"`

**User Experience**: Full transparency
- Shows all 5 escalation phases with timing
- Lists tools invoked per phase
- Shows health status of each tool
- Displays intermediate findings
- Shows model escalation if triggered
- Final structured report with sources

**Example Output**:
```
=== ESCALATION: Concurrency Pattern Analysis ===

Phase 1: Classification [250ms]
→ Detected intent: detailed_analysis
→ Will require code analysis tools

Phase 2: Context Gathering [1.2s]
→ Tool: lsp_lookup [healthy]
  Found 12 concurrent patterns in codebase
→ Tool: rtk_grep [healthy]
  Matched 8 pattern variations

Phase 3: Model Selection [100ms]
→ Selected: Claude Sonnet (detailed analysis requires reasoning)

Phase 4: Response Enhancement [800ms]
→ Tool: security_scanner [healthy]
  Found 3 potential race conditions
→ Tool: custom_analyzer [healthy]
  Risk assessment: MEDIUM

Phase 5: Response Generation [2.1s]
→ Model: Sonnet (no escalation needed)
→ Response: [detailed analysis with findings]

Total Time: 4.45s
Tools Invoked: 4
Escalations: None
```

---

### Verbose Mode (--verbose --research)
**User Command**: `claude code --verbose --research "Audit SQL security"`

**User Experience**: Complete transparency including internals
- All of research mode info
- Plus internal decision trees
- Plus token usage per tool
- Plus full error logs if any
- Plus cache hit/miss details
- Plus adapter initialization details

---

### Background Mode (--background)
**User Command**: `claude code --background "Audit all SQL queries"`

**User Experience**: Asynchronous with stream to file
- Immediate job ID returned: `Job ID: audit_sql_20260429_091500`
- User can tail -f to watch progress in real-time
- Shows: phases, tool progress, findings, escalations
- Final report written to file when complete
- User can cancel with `claude code --cancel audit_sql_20260429_091500`

**File Output**:
```
$ tail -f ~/.claude/jobs/audit_sql_20260429_091500.log

[09:15:00] Starting SQL security audit...
[09:15:02] Phase 1: Classification → detected: security_check
[09:15:05] Phase 2: Tool: security_scanner [healthy] → analyzing SQL...
[09:15:12] Tool: security_scanner → Found 8 potential injections
[09:15:15] Phase 4: Tool: custom_validator [healthy] → validating patterns...
[09:15:22] Phase 5: Generating report...
[09:15:25] COMPLETE: Found 3 high-risk, 5 medium-risk issues
```

---

## Configuration Examples

### Minimal Config (Built-in Tools Only)
```yaml
tools: []  # No custom tools, use built-ins only
tool_selection:
  intent_tool_mapping:
    quick_answer: ["rtk_git", "lsp_lookup"]
    code_search: ["lsp_lookup", "rtk_grep"]
```

### Production Config (Custom + Built-in)
```yaml
tools:
  - name: security_scanner
    type: rest
    path: "http://localhost:9000"
    settings:
      timeout_override: 45
      auth_type: bearer
      
  - name: custom_analyzer
    type: cli
    path: "/usr/local/bin/analyze"
    settings:
      args: ["--json"]

tool_selection:
  intent_tool_mapping:
    security_check: ["security_scanner", "lsp_lookup"]
    detailed_analysis: ["custom_analyzer", "rtk_grep"]
    
tool_invocation:
  selection_strategy: intent_matching
  output_handling:
    max_context_tokens: 2000
    truncate_on_limit: true
```

---

## Monitoring & Debugging

### Check Tool Status
```bash
# Dashboard: http://localhost:9000/dashboard → Tools tab
# CLI: escalate-tools status
# API: GET /api/tools
```

### View Health Events
```bash
# Audit trail
escalate-tools tools audit-log

# Real-time monitoring
escalate-tools tools monitor --interval 10s
```

### Test Tool Connectivity
```bash
# Dashboard: Click "Test Connection" button
# CLI: escalate-tools tools test security_scanner
# API: POST /api/tools/security_scanner/test
```

### Debug Tool Invocation
```bash
# Verbose logs
escalate-tools --verbose run "some query"

# Profile tool performance
escalate-tools tools profile --intent security_check
```

---

## Performance Characteristics

### Tool Invocation Timing
| Phase | Typical Duration | Tool Count |
|-------|------------------|-----------|
| Phase 2 (Context Gathering) | 500-1500ms | 2-3 tools |
| Phase 4 (Response Enhancement) | 300-1200ms | 1-2 tools |
| **Total Overhead** | **800-2700ms** | **3-5 total** |

### Token Impact
| Operation | Token Cost | Budget |
|-----------|-----------|--------|
| Tool discovery | 0 (cached) | - |
| Health check | ~50 | - |
| Tool invocation | ~200-300 | 2000/tool |
| Output processing | ~100-200 | 3000/request |
| **Total Overhead** | **~500-700/request** | **~5% of 16k context** |

### Cache Effectiveness
- **Hit Rate**: 70-85% for repeated intents
- **Reuse Savings**: 2000-3000 tokens per cache hit
- **Cache Duration**: 30 minutes (configurable)

---

## Security Considerations

### Tool Sandboxing
- **CLI Tools**: Run in subprocess with timeout, restricted env vars
- **MCP Tools**: Connect via socket, no host shell access
- **REST Tools**: HTTP only (no direct execute), auth via bearer token
- **Database Tools**: Connection string validated, no shell injection possible

### Access Control
- Tools run with **user privileges only** (no privilege escalation)
- Environment variables filtered (no secrets exposed)
- Subprocess stdout/stderr captured (no side effects)
- Tool output sanitized (ANSI codes stripped, control chars removed)

### Audit Trail
- All tool invocations logged: timestamp, tool name, result, duration
- All failures logged: error type, retry count, fallback action
- All changes logged: registration, removal, configuration update

---

## Troubleshooting

### Tool Shows Unhealthy
**Diagnosis**:
```bash
escalate-tools tools health custom_tool
# Shows: UNHEALTHY - Connection timeout
```

**Fix**:
1. Check tool is running: `ps aux | grep tool_name`
2. Test connectivity: `escalate-tools tools test custom_tool`
3. Check credentials: Verify auth token in config
4. Check network: Verify firewall/proxy settings
5. Retry: `escalate-tools tools retry custom_tool`

### Tool Invocation Too Slow
**Diagnosis**: Phase 2 or 4 takes >2 seconds

**Fix**:
1. Reduce timeout: Lower `timeout_seconds` in config
2. Disable slow tool: Remove from intent mapping
3. Check health: Run health monitor to identify degradation
4. Check network: Latency to tool endpoint

### Output Gets Truncated
**Diagnosis**: Tool output shows `[truncated to X tokens]`

**Fix**:
1. Increase budget: Raise `max_context_tokens` in config
2. Reduce verbosity: Configure tool to output JSON only
3. Use CSS selector: For Scrapling, target specific sections only

---

## What's Next

See [Tool Configuration](./TOOL_CONFIG.md) for how to add custom tools via the dashboard.

See [CONFIG_SPEC.yaml](./CONFIG_SPEC.yaml) for complete specification of all tool system configuration options.
