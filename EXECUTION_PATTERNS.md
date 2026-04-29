# Execution Patterns & Optimization Guide

*This file is auto-generated from execution logs. Run `claude-generate-patterns` to regenerate with latest data.*

## 🚀 First Run

This is your first execution patterns guide. As operations are logged, this file will be automatically updated with:

- **Fast operations** (< 500ms) — Safe to call frequently
- **Slow operations** (> 2s) — Candidates for caching or optimization
- **Caching opportunities** — Commands repeated 3+ times (potential token savings)
- **Token savings tips** — Proven optimizations for this project
- **Best practices** — Project-specific patterns learned from execution history

## How It Works

1. **Operations are logged**: Every bash command, Python script, web fetch, and binary run is logged to `.execution-log.jsonl` with timing and metadata
2. **Patterns are generated**: After 50+ operations, Claude auto-generates this guide from real execution data
3. **Claude reads patterns**: At session start, Claude reads this file and adapts behavior based on learned patterns
4. **Optimization applies**: Claude uses faster approaches, caches results, and avoids expensive operations

## Current Status

⏳ No execution data yet. Start using the project and patterns will be generated automatically.

**To manually generate patterns**:
```bash
claude-generate-patterns --log-file .execution-log.jsonl --output EXECUTION_PATTERNS.md
```

## What Claude Will Learn

Once operations are logged, this guide will show:

### Fast Operations (✅ < 500ms)
Examples of what will appear:
- `git status` — instant, safe to call freely
- `wc -l <file>` — 1-5ms, use before Read tool (saves 90% tokens)
- `grep -n pattern src/` — quick pattern search before detailed reading

### Slow Operations (⚠️ > 2s)
Examples of what will appear:
- `go test ./...` — 3.5s avg, cache between commits if code unchanged
- `golangci-lint ./...` — 8.2s avg, batch fixes to avoid iteration
- `scrapling full-page fetch` — 2.5s avg, use css_selector instead

### Caching Opportunities (💾)
Examples of what will appear:
- Commands repeated 3+ times (potential to cache results)
- URLs fetched multiple times (cache 30min intervals)
- Operations with high token cost but stable output

### Token Savings Tips
Examples of what will appear:
- "Use `wc -l` before Read on large files (saves ~90% tokens)"
- "Use `grep -n` to locate, then Read with offset/limit (saves 80%+)"
- "Use LSP for symbol search instead of grep (structured output, 10x cheaper)"
- "Use css_selector on web fetches to target sections (saves 85-95%)"
- "Batch similar operations to reduce context switching"

### Decision Pattern Analysis
Examples of what will appear:
- Which decision contexts lead to efficient execution (fewest operations)
- Which decision contexts lead to wasted operations (repeated failures)
- Project-specific best practices based on actual execution history

## Next Steps

1. **Use the project normally** — Claude will log all operations
2. **Run tests, build, fetch documentation** — These operations get logged
3. **After ~50 operations**, Claude auto-generates this guide with real data
4. **Read the updated guide** — Claude will use patterns to optimize future sessions
5. **Monitor the dashboard** — See execution metrics at `http://localhost:9000/analytics`

## Viewing Analytics

**Dashboard**: `http://localhost:9000/analytics`
- Real-time operation metrics
- Performance trends by operation type
- Slowest operations this session
- Optimization opportunities and recommendations

**CLI Analytics**:
```bash
claude-analytics --summary              # Session overview
claude-analytics --slowest 10           # Slowest operations
claude-analytics --duplicates 3         # Caching opportunities
claude-analytics --recommendations      # Optimization tips
claude-analytics --all                  # Complete analysis
```

## Privacy & Security

All logging respects privacy:
- ✅ Auto-redacts API keys, tokens, passwords from commands
- ✅ Normalizes file paths (shows `<path>` instead of full paths)
- ✅ Logs are gitignored (`.execution-log.jsonl` not committed)
- ✅ This patterns file is checked in (safe, contains only aggregate statistics)

## How Claude Uses These Patterns

### Pattern 1: Slow Operations → Cache or Skip
```
Pattern: "go test ./... takes 3.5s; run 7 times per session"
Claude's behavior:
  1. Check git status: has code changed in /internal/tools?
  2. If no: skip test (result cached from last run)
  3. If yes: run test, cache result with commit hash
  Result: Save 20+ seconds per session
```

### Pattern 2: Token Savings → Apply Proven Optimizations
```
Pattern: "wc -l before Read saves 90% tokens; 47 uses detected"
Claude's behavior:
  1. For large files: always run `wc -l` first
  2. If > 1000 lines: use grep -n to find location
  3. Then Read with offset/limit
  Result: Consistent 90% token savings on file reads
```

### Pattern 3: Web Patterns → Use CSS Selectors
```
Pattern: "css_selector on docs.python.org accurate; works in 12/12 uses"
Claude's behavior:
  1. When fetching docs.python.org: use css_selector=".documentation"
  2. Reduces output from 5000+ tokens to 100-200 tokens
  3. Still gets accurate, focused information
  Result: 96% reduction in web fetch tokens
```

### Pattern 4: Decision Context Efficiency
```
Pattern: "explore_codebase leads to 8+ operations avg; use Agent instead"
Claude's behavior:
  1. If decision context = "explore codebase"
  2. Use Explore agent instead of sequential commands
  3. Agent works in parallel, returns structured results
  Result: 60% fewer operations, faster completion
```

---

*For implementation details, see CLAUDE.md section "Execution Feedback Loop: Patterns & Optimization"*
