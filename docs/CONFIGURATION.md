# Claude Escalate Configuration Guide

Complete reference for all configuration parameters and their meanings. Use this guide to understand what each setting does and how it affects token optimization and model routing.

## Table of Contents

1. [Gateway Configuration](#gateway-configuration)
2. [Optimizations](#optimizations)
3. [Intent Detection](#intent-detection)
4. [Security](#security)
5. [Metrics](#metrics)
6. [Thresholds Configuration](#thresholds-configuration)
7. [Keywords Configuration](#keywords-configuration)
8. [Signals Configuration](#signals-configuration)
9. [Token Limits Configuration](#token-limits-configuration)
10. [Timeouts Configuration](#timeouts-configuration)
11. [Paths Configuration](#paths-configuration)
12. [Models Configuration](#models-configuration)
13. [Example Configuration](#example-configuration)

---

## Gateway Configuration

The `gateway` section configures the HTTP server and core system paths.

```yaml
gateway:
  port: 8080
  host: "0.0.0.0"
  security_layer: true
  shutdown_timeout_seconds: 30
  max_request_size_bytes: 10485760
  data_dir: "~/.claude-escalate/data"
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `port` | int | 8080 | Port the gateway listens on. Can override with `--port` flag or `ESCALATE_PORT` environment variable. |
| `host` | string | 0.0.0.0 | Bind address for the gateway. Set to `127.0.0.1` for localhost-only access (more secure in development). |
| `security_layer` | bool | true | **CRITICAL**: Enable input/output security validation (SQL injection, XSS, command injection detection). Cannot be disabled - this is always-on for safety. |
| `shutdown_timeout_seconds` | int | 30 | Max seconds to wait for graceful shutdown before forcing termination. Increase if requests take >30s. |
| `max_request_size_bytes` | int | 10485760 (10MB) | Maximum request body size. Prevent memory exhaustion from huge pastes. Adjust if you process files >10MB. |
| `data_dir` | string | ~/.claude-escalate/data | Path for storing escalation history, metrics, and cache data. Must be writable. Auto-created if missing. |

### Use Cases

- **Local development**: Set `host: 127.0.0.1`, `port: 8080` (avoid exposing to network)
- **Docker/K8s**: Set `host: 0.0.0.0`, `port: 8080` (listen on all interfaces)
- **Remote access**: Reverse proxy (nginx/Caddy) in front of localhost-only gateway for TLS + auth

---

## Optimizations

The `optimizations` section controls which token-saving techniques are applied. All are individually toggleable - disable any optimization if it causes issues.

### RTK Optimization

Rust Token Killer - external CLI proxy that reduces command output by 99.4%.

```yaml
optimizations:
  rtk:
    enabled: true
    command_proxy_savings: 99.4
    models:
      low_effort: "haiku"
      medium_effort: "sonnet"
      high_effort: "opus"
    cache_savings: true
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable RTK optimization if RTK is installed. Auto-detected at startup. |
| `command_proxy_savings` | float | 99.4 | Percentage token savings from RTK compression (realistic: 99.4% for command output). Used for metrics calculation, not a hard limit. |
| `models[low_effort]` | string | "haiku" | Route low-effort CLI calls through Haiku (8x cheaper). Can change to sonnet/opus if Haiku responses inadequate. |
| `models[medium_effort]` | string | "sonnet" | Route medium-effort CLI calls through Sonnet. Balanced cost/quality. |
| `models[high_effort]` | string | "opus" | Route high-effort CLI calls through Opus (best quality, most expensive). For complex analysis that needs deep reasoning. |
| `cache_savings` | bool | true | Cache RTK responses. Safe because RTK output is deterministic (same command = same output). |

### MCP Configuration

MCP (Model Context Protocol) tool adapters.

```yaml
optimizations:
  mcp:
    enabled: true
    tools:
      - type: "web_scraping"
        name: "scrapling"
        settings:
          css_selector: true
          markdown_only: true
          cache_responses: true
      
      - type: "code_analysis"
        name: "builtin_lsp"
        settings:
          use_lsp: true
          cache_symbols: true
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable MCP tools. Auto-detected at startup. |
| `tools[].type` | string | - | Tool category: "web_scraping", "code_analysis", "database", "binary". |
| `tools[].name` | string | - | Tool identifier. Must match available tool name. |
| `tools[].settings.css_selector` | bool | true | Extract only CSS selector matches instead of full page (60-80% token savings on web scraping). Recommended for large pages. |
| `tools[].settings.markdown_only` | bool | true | Convert HTML to Markdown (80-90% more token-efficient than raw HTML). Always recommended. |
| `tools[].settings.cache_responses` | bool | true | Cache web scraping results. Safe if pages don't change frequently. Disable for live-updating content. |
| `tools[].settings.use_lsp` | bool | true | Use Language Server Protocol for code analysis instead of grep. 10x more efficient (~200t vs 2000t+). Always recommended for supported languages. |
| `tools[].settings.cache_symbols` | bool | true | Cache LSP symbol lookups. Safe because code structure doesn't change between parses. |

### Semantic Cache

Semantic similarity-based response caching using embeddings.

```yaml
optimizations:
  semantic_cache:
    enabled: true
    embedding_model: "onnx-mini-l6"
    similarity_threshold: 0.85
    hit_rate_target: 60
    false_positive_limit: 0.5
    max_cache_size_mb: 500
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable semantic caching. Disable if false positive rate too high (see false_positive_limit). |
| `embedding_model` | string | "onnx-mini-l6" | Model for embedding computation. Options: "onnx-mini-l6" (384-dim, 22MB), "onnx-large" (768-dim, larger). Smaller is faster. |
| `similarity_threshold` | float | 0.85 | Cosine similarity threshold for cache hits (0-1). **Increase** to 0.90+ if false positives high. **Decrease** to 0.75 if hit rate too low. **CRITICAL**: >0.85 is recommended to avoid wrong answers. |
| `hit_rate_target` | float | 60 | Target cache hit percentage (for metrics). If actual <50%, reduce threshold or increase queries similarity. |
| `false_positive_limit` | float | 0.5 | Kill threshold: if false positive rate >0.5%, automatically disable semantic cache. Set monitoring alert at 0.3%. |
| `max_cache_size_mb` | int | 500 | Max cache memory (embeddings + responses). Increase if cache filling up (check metrics). Decrease if memory constrained. |

**Safety Note**: Semantic cache is guarded by strict thresholding and false positive monitoring. If even 1% of responses are wrong, cache is disabled. Monitor actual usage before enabling in production.

### Knowledge Graph

Graph-based query answering (Phase 2, currently disabled).

```yaml
optimizations:
  knowledge_graph:
    enabled: false
    index_local_code: false
    index_web_content: false
    cache_lookups: true
    db_path: "~/.claude-escalate/graph.db"
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | false | Enable knowledge graph (Phase 2 - experimental). Requires continuous indexing. |
| `index_local_code` | bool | false | Index local codebase for instant relationship lookups (99% token savings on graph queries). Enable for stable codebases. |
| `index_web_content` | bool | false | Index web pages for instant lookups. Requires active crawler. Experimental. |
| `cache_lookups` | bool | true | Cache graph query results. Safe because relationships don't change frequently. |
| `db_path` | string | ~/.claude-escalate/graph.db | SQLite database path for graph storage. Must have write permission. |

### Input Optimization

Optimize requests sent to Claude.

```yaml
optimizations:
  input_optimization:
    enabled: true
    strip_unused_tools: true
    compress_parameters: true
    dedup_exact_requests: true
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable input optimization. Safe - never affects response correctness. |
| `strip_unused_tools` | bool | true | Send only relevant tools instead of all 300+ tools. 20-30% input token savings. Minimal quality impact. |
| `compress_parameters` | bool | true | Compress verbose parameters to structured format. Reduces noise, improves response quality. |
| `dedup_exact_requests` | bool | true | Skip redundant identical requests in batches. 100% savings on exact duplicates. Always safe. |

### Output Optimization

Optimize responses from Claude.

```yaml
optimizations:
  output_optimization:
    enabled: true
    response_compression: true
    field_filtering: true
    delta_detection: true
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable output optimization. Safe for structured queries, selective for reasoning. |
| `response_compression` | bool | true | Compress prose responses to JSON. 30-50% token savings. Safe if response is structured (code, lists, data). Risky if response is reasoning - could lose conclusions. |
| `field_filtering` | bool | true | Return only requested fields (e.g., filename, line number). Safe if user asked for summary. Unsafe if user wants full context. |
| `delta_detection` | bool | true | Return only changes (vs previous response). 90% savings if nothing changed. Safe - if code truly unchanged, no changes needed in response. |

### Batch API

Async batch processing for non-interactive workflows.

```yaml
optimizations:
  batch_api:
    enabled: false
    min_batch_size: 10
    max_batch_size: 100
    auto_batch_similar: true
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | false | Enable Anthropic Batch API (50% cost discount, 5-24h latency). Use for overnight jobs, not interactive queries. |
| `min_batch_size` | int | 10 | Minimum requests before submitting batch (avoids overhead on small batches). |
| `max_batch_size` | int | 100 | Maximum requests per batch (API limit). Split larger workloads. |
| `auto_batch_similar` | bool | true | Group similar queries in batch (can share cached context, further savings). |

---

## Intent Detection

Intent classification and cache bypass detection.

```yaml
intent_detection:
  enabled: true
  cache_bypass_patterns:
    - "--no-cache"
    - "--fresh"
    - "!"
    - "(no cache)"
    - "(bypass)"
  personalization:
    learn_from_feedback: true
    adapt_per_user: true
    feedback_history_depth: 90
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Enable intent classifier. Always on - critical for safety (determines when caching is safe). |
| `cache_bypass_patterns[...]` | array | ["--no-cache", "--fresh", "!", "(no cache)", "(bypass)"] | Patterns that signal user wants fresh response (no caching). HIGHEST PRIORITY - overrides intent detection. Add custom patterns (e.g., "@fresh", "@bypass") to match your workflow. |
| `learn_from_feedback` | bool | true | Learn from user feedback (😊 / 😞 ratings) to improve intent detection and model routing. Enables per-user personalization. |
| `adapt_per_user` | bool | true | Learn per-user preferences (user A prefers detail, user B wants speed). Scales personalization beyond generic intent. |
| `feedback_history_depth` | int | 90 | Days of feedback history to retain for learning. Older feedback discarded. Increase for long-term pattern learning, decrease to forget old preferences. |

### Intent Classification

Classifier outputs 4 decisions simultaneously:

| Intent Type | Cache Safe? | Model | Max Tokens | Use Case |
|-------------|---------|-------|-----------|----------|
| QUICK_ANSWER | YES | Haiku | 256 | "Quick summary", "tl;dr", "briefly explain" |
| DETAILED_ANALYSIS | NO | Sonnet/Opus | Unlimited | "Explain why", "detailed analysis", "comprehensive" |
| ROUTINE | YES | Haiku | 256 | Identical query repeated (e.g., daily scan with same params) |
| LEARNING | NO | Sonnet | 512 | "What if...", "Try...", "Compare", "Explore" |
| FOLLOW_UP | NO | Sonnet | 512 | "More about X", "Also check Y", refinement on previous query |

---

## Security

Input/output validation (always on, cannot be disabled).

```yaml
security:
  enabled: true
  sql_injection_detection: true
  xss_prevention: true
  command_injection_detection: true
  rate_limiting:
    requests_per_minute: 1000
    per_ip: true
  audit_logging: true
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | true | Master security switch. **ALWAYS ON** - cannot be disabled for safety. |
| `sql_injection_detection` | bool | true | Detect SQL injection patterns (DROP, ' OR '1'='1, UNION, etc). 16 patterns. Blocks obvious attacks. |
| `xss_prevention` | bool | true | Detect XSS payloads (<script>, javascript:, onerror=, etc). 25 patterns. Prevents client-side attacks. |
| `command_injection_detection` | bool | true | Detect shell injection ($(...), backticks, pipes, etc). 12 patterns. Prevents shell command execution. |
| `rate_limiting.requests_per_minute` | int | 1000 | Max requests per minute (global). Prevent DoS/abuse. Lower for strict limits, raise if hitting limits legitimately. |
| `rate_limiting.per_ip` | bool | true | Enforce rate limit per IP address (not per user). Prevent one user from exhausting quota. |
| `audit_logging` | bool | true | Log all security events (injection attempts, rate limits, blocked requests). Always on for compliance. |

### Security Posture

- **Defense-in-depth**: Multiple detection layers catch >95% of attacks
- **No bypass**: Security layer cannot be disabled (it's foundational)
- **False positive rate**: <0.1% on legitimate input (permissive when safe)
- **Response**: Blocked requests return 403 + logged event

---

## Metrics

Observability and monitoring.

```yaml
metrics:
  enabled: true
  publish_to:
    prometheus:
      enabled: true
      port: 9090
      path: "/metrics"
    grafana:
      enabled: false
    cloudwatch:
      enabled: false
    debug_logs:
      enabled: true
      dir: "~/.claude-escalate/metrics"
  track:
    cache_hit_rate:
      enabled: true
      interval: 60s
    cache_false_positive_rate:
      enabled: true
      interval: 60s
      alert_if_above: 0.5
    token_savings_percent:
      enabled: true
      interval: 60s
    latency_by_layer:
      enabled: true
    per_optimization_savings:
      enabled: true
    security_events:
      enabled: true
    cost_tracking:
      enabled: true
```

### Parameters

| Section | Parameter | Type | Default | Description |
|---------|-----------|------|---------|-------------|
| Prometheus | `enabled` | bool | true | Export metrics in Prometheus format (scrape via `http://localhost:9090/metrics`). |
| Prometheus | `port` | int | 9090 | Prometheus metrics port. |
| Prometheus | `path` | string | "/metrics" | Prometheus endpoint path. |
| Grafana | `enabled` | bool | false | Send metrics to Grafana for dashboarding (requires Grafana server). |
| CloudWatch | `enabled` | bool | false | Send metrics to AWS CloudWatch (requires AWS credentials). |
| Debug Logs | `enabled` | bool | true | Write metrics to local JSON files for debugging. Always enable in dev. |
| Debug Logs | `dir` | string | ~/.claude-escalate/metrics | Directory for local metric files. |
| cache_hit_rate | `enabled` | bool | true | Track percentage of cache hits vs total requests. Target: >50%. |
| cache_hit_rate | `interval` | duration | 60s | How often to update metric (default: every 60 seconds). |
| false_positive_rate | `enabled` | bool | true | Track % of cache hits that were wrong answers. **CRITICAL**: Alert if >0.5%. |
| false_positive_rate | `alert_if_above` | float | 0.5 | Trigger alert/disable cache if false positive rate exceeds this %. |
| token_savings | `enabled` | bool | true | Track overall token savings %. Target: 60-75%. |
| latency_by_layer | `enabled` | bool | true | Breakdown latency: cache lookup (10ms), security (20ms), Claude API (2000ms+). Identify bottlenecks. |
| per_optimization_savings | `enabled` | bool | true | Per-optimization breakdown (RTK savings %, semantic cache savings %, etc). Validate each optimization's ROI. |
| security_events | `enabled` | bool | true | Count injection attempts blocked, rate limit triggers. Baseline for security posture. |
| cost_tracking | `enabled` | bool | true | Tokens to Claude, estimated cost, savings. Business KPI. |

### Monitoring Strategy

1. **Real-time**: Dashboard shows live metrics (WebSocket streaming, updated every second)
2. **Alerting**: Thresholds trigger on:
   - False positive rate >0.5% (cache disabled)
   - Token savings <60% (optimization ineffective)
   - Latency >300ms p99 (performance degradation)
   - Injection attempts >5/min (attack pattern)
3. **Trending**: Weekly reports on:
   - Model routing (% Haiku vs Sonnet vs Opus)
   - Cache effectiveness over time
   - Cost savings realized
   - Top optimization sources

---

## Thresholds Configuration

Numerical thresholds for decision-making and confidence scoring.

### `thresholds.cache_similarity`
- **Default**: `0.85` (cosine similarity, 0.0-1.0)
- **Impact**: **CRITICAL QUALITY**. Controls semantic cache false positive rate
  - `0.90-1.00`: Very conservative, <0.05% false positives (strict quality)
  - `0.85`: Balanced (default), <0.1% false positives (recommended)
  - `0.70`: Loose, ~5% false positives (NOT recommended)
- **When to adjust**: Increase to 0.90 if observing wrong cached answers, never decrease below 0.85

### `thresholds.confidence_scores`
- **high** (default `0.95`): Very confident intent classification
- **medium** (default `0.85`): Moderately confident intent classification
- **low** (default `0.70`): Some uncertainty in intent classification
- **Impact**: Used for escalating models when confidence is low. Keep defaults (well-calibrated)

### `thresholds.model_accuracy`
- **Default**: `0.85`
- **Impact**: Threshold for auto-escalating to better model if accuracy drops
- **When to adjust**: Keep default (well-calibrated threshold)

---

## Keywords Configuration

Keywords for intent detection and security pattern matching.

### `keywords.detail`
- **Default**: `["detailed", "comprehensive", "explain", "why", "how", "deep", "thorough", "analysis"]`
- **Purpose**: Detect requests for detailed analysis (should bypass cache, use premium model)
- **When to adjust**: Add domain-specific detail keywords (e.g., "architectural", "design rationale")

### `keywords.quick`
- **Default**: `["quick", "brief", "summary", "tl;dr", "just", "simply", "shortly"]`
- **Purpose**: Detect quick/summary requests (safe to cache, use cheaper model)
- **When to adjust**: Add domain-specific quick keywords (e.g., "gist", "one-liner")

### `keywords.follow_up`
- **Default**: `["more", "additional", "also", "what about", "furthermore", "moreover"]`
- **Purpose**: Detect follow-up refinements (need fresh analysis on new angle)
- **When to adjust**: Add domain-specific refinement keywords (e.g., "now for", "in addition")

### `keywords.learning`
- **Default**: `["what if", "try", "experiment", "compare", "explore", "alternative"]`
- **Purpose**: Detect exploratory/learning queries (different scenarios need fresh responses)
- **When to adjust**: Add domain-specific exploration keywords (e.g., "contrast", "evaluate")

### `keywords.suspicious`
- **Default**: `["DROP TABLE", "DELETE FROM", "INSERT INTO", "UPDATE", "TRUNCATE", "UNION SELECT"]`
- **Purpose**: Detect potential SQL injection in input
- **When to adjust**: Keep defaults (standard SQL injection keywords)

---

## Signals Configuration

User feedback signals for learning system (success, failure, frustration indicators).

### `signals.success_signals`
- **Map format**: `{signal: confidence}` (e.g., `"perfect": 0.95`)
- **Default signals**: "thanks" (0.85), "solved" (0.90), "works" (0.85), "got_it" (0.85)
- **Purpose**: Learn when optimizations are working (reinforce decisions)
- **When to adjust**: Add domain-specific success indicators (e.g., "chef's kiss" → 0.95)

### `signals.failure_signals`
- **Map format**: `{signal: confidence}`
- **Default signals**: "error" (0.90), "broken" (0.90), "failed" (0.90), "doesnt_work" (0.85)
- **Purpose**: Learn when optimizations fail (adjust decisions)
- **When to adjust**: Add domain-specific failure indicators (e.g., "bust" → 0.85)

### `signals.frustration_keywords`
- **Map format**: `{keyword: confidence}`
- **Default signals**: "still_broken" (0.90), "going_in_circles" (0.85), "stuck" (0.85)
- **Purpose**: Detect frustration (triggers model escalation, e.g., Haiku → Sonnet)
- **When to adjust**: Keep defaults (well-validated frustration indicators)

---

## Token Limits Configuration

Token budgets per intent type and model.

| Setting | Default | Purpose |
|---------|---------|---------|
| `quick_answer` | 256 | Max tokens for summary responses |
| `detailed_analysis` | 2000 | Max tokens for detailed analysis (essentially unlimited) |
| `routine` | 256 | Max tokens for routine/repeated queries |
| `learning` | 1024 | Max tokens for exploratory queries (discuss trade-offs) |
| `follow_up` | 512 | Max tokens for follow-up refinements |
| `cache_bypass` | 2000 | Max tokens for cache-bypassed fresh queries (no limit) |
| `max_cache_size_per_model.haiku` | 5000 | Max embedding cache size for Haiku responses |
| `max_cache_size_per_model.sonnet` | 10000 | Max embedding cache size for Sonnet responses |
| `max_cache_size_per_model.opus` | 50000 | Max embedding cache size for Opus responses |

### When to adjust:
- **Increase `quick_answer` to 512** if users want more detail in summaries
- **Increase `learning` to 2000** if users want very detailed exploration of alternatives
- **Increase model cache sizes** if you have memory and want higher cache hit rate

---

## Timeouts Configuration

Operation timeouts in milliseconds.

| Setting | Default | Purpose |
|---------|---------|---------|
| `individual_tool_check_ms` | 500 | Timeout per tool during discovery |
| `total_discovery_ms` | 5000 | Total timeout for all tool discovery (startup) |
| `escalation_ms` | 2000 | Timeout for model escalation decision |
| `intent_detection_ms` | 50 | Timeout for intent classification |
| `security_validation_ms` | 20 | Timeout for security checks |
| `cache_lookup_ms` | 10 | Timeout for semantic cache search |

### When to adjust:
- **Increase discovery timeouts** if tools are on slow storage
- **Decrease intent/security timeouts** if you need ultra-fast responses
- Keep **cache_lookup_ms** <10ms (critical for interactive performance)

---

## Paths Configuration

Directory paths for data storage and configuration files.

| Setting | Default | Purpose |
|---------|---------|---------|
| `config_dir` | ~/.claude-escalate | Configuration directory |
| `data_dir` | ~/.claude-escalate/data | Runtime cache and state |
| `graph_db_path` | ~/.claude-escalate/graph.db | Knowledge graph SQLite DB (Phase 2) |
| `metrics_dir` | ~/.claude-escalate/metrics | Metrics log files |
| `log_dir` | ~/.claude-escalate/logs | Application logs |
| `cache_dir` | ~/.claude-escalate/cache | Cache storage |
| `claude_home` | ~/.claude | Claude configuration directory |

### When to adjust:
- **Change to SSD** for `data_dir`, `cache_dir` (faster cache performance)
- **Change `metrics_dir`** to central logging server in production
- Keep others at defaults (system standard locations)

---

## Models Configuration

Model identifiers and pricing for Haiku, Sonnet, Opus.

### Format per model:
```yaml
MODEL:
  id: claude-MODEL-VERSION              # API model ID
  cost_per_1k_input: 0.0008             # Cost per 1,000 input tokens
  cost_per_1k_output: 0.0004            # Cost per 1,000 output tokens
  context_window: 200000                # Max context in tokens
```

### Current Models:

| Model | ID | Input Cost | Output Cost | Context |
|-------|----|----|--------|---------|
| Haiku | claude-haiku-4-5-20251001 | $0.0008/1K | $0.0004/1K | 200K |
| Sonnet | claude-sonnet-4-6 | $0.003/1K | $0.015/1K | 200K |
| Opus | claude-opus-4-6 | $0.015/1K | $0.075/1K | 200K |

### When to adjust:
- **Update IDs** when Anthropic releases new model versions
- **Update pricing** if pricing changes
- **Update context_window** if API increases context size

---

## Example Configuration

Complete, production-ready configuration with all parameters and sensible defaults:

```yaml
# Claude Escalate v4.1.0 Configuration
# Complete reference with all parameters explained

# HTTP Gateway Configuration
gateway:
  # Port the gateway listens on
  port: 8080
  
  # Bind address: 0.0.0.0 (all interfaces) or 127.0.0.1 (localhost only)
  host: "127.0.0.1"
  
  # Security validation layer (SQL injection, XSS, command injection detection)
  # CRITICAL: Always on, cannot be disabled
  security_layer: true
  
  # Graceful shutdown timeout in seconds
  shutdown_timeout_seconds: 30
  
  # Maximum request body size (bytes, 10MB default)
  max_request_size_bytes: 10485760
  
  # Data directory for escalation history, metrics, and cache
  # Auto-created if missing, must be writable
  data_dir: "~/.claude-escalate/data"

# Token Optimization Layers
optimizations:
  
  # RTK (Rust Token Killer) - Command output compression (99.4% savings)
  rtk:
    enabled: true
    command_proxy_savings: 99.4
    models:
      low_effort: "haiku"        # 8x cheaper
      medium_effort: "sonnet"    # Balanced
      high_effort: "opus"        # Best quality
    cache_savings: true
  
  # MCP (Model Context Protocol) Tools
  mcp:
    enabled: true
    tools:
      # Web Scraping (Scrapling)
      - type: "web_scraping"
        name: "scrapling"
        settings:
          # Extract only CSS selector matches (60-80% savings on big pages)
          css_selector: true
          # Convert HTML to Markdown (80-90% more efficient)
          markdown_only: true
          # Cache scraping results (safe if pages stable)
          cache_responses: true
      
      # Code Analysis (LSP)
      - type: "code_analysis"
        name: "builtin_lsp"
        settings:
          # Use LSP instead of grep (10x more efficient)
          use_lsp: true
          # Cache symbol lookups
          cache_symbols: true
  
  # Semantic Cache - Similarity-based response caching
  semantic_cache:
    enabled: true
    
    # Embedding model: "onnx-mini-l6" (384-dim, fast, 22MB)
    embedding_model: "onnx-mini-l6"
    
    # Cosine similarity threshold for cache hits (0-1)
    # Higher = stricter matching, lower false positives
    # Recommended: 0.85 (conservative, <0.1% wrong answers)
    similarity_threshold: 0.85
    
    # Target cache hit rate (%)
    hit_rate_target: 60
    
    # Kill threshold: disable cache if false positive rate exceeds this
    false_positive_limit: 0.5
    
    # Max cache memory (embeddings + responses, MB)
    max_cache_size_mb: 500
  
  # Knowledge Graph - Relationship-based query answering (Phase 2)
  knowledge_graph:
    enabled: false  # Disabled in Phase 1, requires indexing overhead
    
    # Index local codebase for instant graph lookups (99% savings)
    index_local_code: false
    
    # Index web content (experimental, requires crawler)
    index_web_content: false
    
    # Cache graph query results
    cache_lookups: true
    
    # SQLite database path
    db_path: "~/.claude-escalate/graph.db"
  
  # Input Optimization - Optimize requests to Claude
  input_optimization:
    enabled: true
    
    # Strip unused tools (300 tools → 5 relevant, 20-30% savings)
    strip_unused_tools: true
    
    # Compress parameters to structured format
    compress_parameters: true
    
    # Skip redundant identical requests in batches (100% savings on exact dupes)
    dedup_exact_requests: true
  
  # Output Optimization - Optimize responses from Claude
  output_optimization:
    enabled: true
    
    # Compress prose to JSON (30-50% savings, safe for structured queries)
    response_compression: true
    
    # Return only requested fields
    field_filtering: true
    
    # Return only changes vs previous response (90% savings if nothing changed)
    delta_detection: true
  
  # Batch API - Async batch processing (50% cost discount, 5-24h latency)
  batch_api:
    enabled: false  # For non-interactive overnight jobs only
    
    # Min requests before submitting batch
    min_batch_size: 10
    
    # Max requests per batch (API limit)
    max_batch_size: 100
    
    # Group similar queries in batch (shared context = more savings)
    auto_batch_similar: true

# Intent Detection & Cache Bypass
intent_detection:
  enabled: true
  
  # Patterns that signal user wants fresh response (cache bypass, HIGHEST PRIORITY)
  cache_bypass_patterns:
    - "--no-cache"     # Prefix: "--no-cache Find functions"
    - "--fresh"        # Prefix: "--fresh Analyze code"
    - "!"              # Prefix: "! Get all files"
    - "(no cache)"     # Suffix: "Analyze code (no cache)"
    - "(bypass)"       # Suffix: "Find X (bypass)"
  
  # User Preference Learning
  personalization:
    # Learn from feedback (😊 helpful, 😞 wrong) to improve routing
    learn_from_feedback: true
    
    # Learn per-user preferences (user A likes detail, user B wants speed)
    adapt_per_user: true
    
    # Days of feedback history to retain (older feedback discarded)
    feedback_history_depth: 90

# Security Validation (Always On, Cannot Be Disabled)
security:
  enabled: true
  
  # SQL Injection Detection (DROP, ' OR '1'='1, UNION, etc)
  sql_injection_detection: true
  
  # XSS Prevention (<script>, javascript:, onerror=, etc)
  xss_prevention: true
  
  # Command Injection Detection ($(...), backticks, pipes, etc)
  command_injection_detection: true
  
  # Rate Limiting
  rate_limiting:
    # Requests per minute (global limit)
    requests_per_minute: 1000
    
    # Enforce limit per IP address (not per user)
    per_ip: true
  
  # Audit Logging (all security events)
  audit_logging: true

# Metrics & Monitoring
metrics:
  enabled: true
  
  # Publishing targets
  publish_to:
    # Prometheus format (scrape via http://localhost:9090/metrics)
    prometheus:
      enabled: true
      port: 9090
      path: "/metrics"
    
    # Grafana integration (optional, requires Grafana server)
    grafana:
      enabled: false
    
    # AWS CloudWatch (optional, requires AWS credentials)
    cloudwatch:
      enabled: false
    
    # Local JSON files (always useful for debugging)
    debug_logs:
      enabled: true
      dir: "~/.claude-escalate/metrics"
  
  # Metrics to track
  track:
    # Cache hit rate tracking
    cache_hit_rate:
      enabled: true
      interval: 60s
    
    # False positive rate (if >0.5%, cache disabled automatically)
    cache_false_positive_rate:
      enabled: true
      interval: 60s
      alert_if_above: 0.5
    
    # Overall token savings %
    token_savings_percent:
      enabled: true
      interval: 60s
    
    # Latency breakdown by layer (cache vs security vs Claude API)
    latency_by_layer:
      enabled: true
    
    # Savings from each optimization (RTK%, semantic cache%, etc)
    per_optimization_savings:
      enabled: true
    
    # Security events (injection attempts, rate limits)
    security_events:
      enabled: true
    
    # Cost tracking (tokens, estimated cost, savings)
    cost_tracking:
      enabled: true
```

---

## Configuration Tips & Best Practices

### Development Setup
```yaml
gateway:
  host: "127.0.0.1"        # Localhost only
  port: 8080

semantic_cache:
  similarity_threshold: 0.95  # Conservative (avoid false positives while learning)

metrics:
  debug_logs:
    enabled: true          # Always on in dev for debugging
```

### Production Setup
```yaml
gateway:
  host: "0.0.0.0"          # All interfaces (reverse proxy in front)
  port: 8080
  security_layer: true     # Always on

semantic_cache:
  similarity_threshold: 0.85  # Balanced (proven <0.1% false positive rate)
  false_positive_limit: 0.3   # Alert if >0.3%, disable at 0.5%

metrics:
  publish_to:
    prometheus:
      enabled: true        # Monitoring dashboard
```

### Testing/Validation
```yaml
semantic_cache:
  enabled: true
  similarity_threshold: 0.85

metrics:
  track:
    cache_false_positive_rate:
      alert_if_above: 0.1  # Strict during validation phase
```

---

## Troubleshooting Configuration

| Issue | Cause | Solution |
|-------|-------|----------|
| Cache hit rate <40% | Threshold too strict | Lower `similarity_threshold` to 0.80 (monitor for false positives) |
| False positives >0.5% | Threshold too loose | Raise `similarity_threshold` to 0.90 (stricter matching) |
| High latency | Claude API slow | Check `latency_by_layer` metrics; not a gateway issue |
| Injections not detected | Pattern incomplete | Add custom patterns to `cache_bypass_patterns` or `security` rules |
| Rate limits triggering | Traffic too high | Increase `requests_per_minute`, or implement request queuing |

---

## Environment Variable Overrides

All config values can be overridden via environment variables:

```bash
# Gateway
ESCALATE_PORT=9000
ESCALATE_HOST=127.0.0.1
ESCALATE_DATA_DIR=/var/lib/escalate/data

# Semantic Cache
ESCALATE_SIMILARITY_THRESHOLD=0.80
ESCALATE_FP_LIMIT=0.3

# Metrics
ESCALATE_PROMETHEUS_ENABLED=true
ESCALATE_PROMETHEUS_PORT=9090
```

Load config from file:
```bash
./claude-escalate service --config /etc/escalate/config.yaml
```

---

## Live Configuration Reload

Change `config.yaml`, save, and hot-reload without restart:

```bash
curl -X POST http://localhost:8080/api/reload
# Returns: {"success": true, "message": "Config reloaded"}
```

**In-flight requests**: Continue with old config
**New requests**: Use new config
**Downtime**: 0 seconds

See [Dashboard](DASHBOARD.md) for UI-based config editing.
