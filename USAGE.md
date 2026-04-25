# Usage Guide

## How Escalation Works

The escalation system manages which Claude model you're using based on task complexity and outcomes.

### Model Hierarchy

```
Low Cost ────────────────→ High Cost
  Haiku (1x)  →  Sonnet (8x)  →  Opus (30x)
Budget      Balanced      Premium
 Cheap      Capable       Powerful
```

## Automatic Routing (Auto-Effort)

The system automatically detects task type and routes to the appropriate model:

### Low Effort → Haiku (8x cheaper)
- Simple questions: "What is 2+2?"
- Lookups: "Show me the syntax for..."
- Clarifications: "Explain this code"
- Quick one-liners

**Example:**
```
You: "What is the capital of France?"
System: ⚡ Auto-effort: low → Haiku (score-based)
Claude: "Paris" (using budget-optimized Haiku)
```

### Medium Effort → Sonnet (baseline)
- Testing, modifications, deployments
- Reviews, integrations
- Configuration changes

**Example:**
```
You: "Write a unit test for this function"
System: ⚙️ Auto-effort: medium → opusplan
Claude: [comprehensive test suite using Sonnet]
```

### High Effort → Opus (most capable)
- Debugging complex issues
- Architecture design
- Code generation from scratch
- Full system implementation

**Example:**
```
You: "Design a microservices architecture for..."
System: 🔥 Auto-effort: high → Opus (high capability)
Claude: [in-depth architecture plan using Opus]
```

## Manual Escalation

Override auto-effort anytime with slash commands:

### `/escalate` or `/escalate to sonnet`
Escalate to Sonnet (4-5x more capable than Haiku)
```
You: /escalate
System: 🚀 Escalated: Sonnet (precision code & balanced reasoning)
Claude: [uses Sonnet for next response]
```

### `/escalate to opus`
Escalate to Opus (most capable model)
```
You: /escalate to opus
System: 🚀 Escalated: Opus (deep reasoning & complex logic)
Claude: [uses Opus for next response]
```

### `/escalate to haiku`
Manually downgrade to Haiku (cost-optimized)
```
You: /escalate to haiku
System: 🚀 Escalated: Haiku (cost-optimized)
Claude: [uses Haiku for next response]
```

## Automatic De-escalation

When you confirm a problem is solved, the system automatically downgrades to cheaper models:

### Success Signals (say any of these)
```
✅ "works"              "perfect"          "thanks"
✅ "got it"             "solved"           "fixed it"
✅ "that works great"   "exactly right"    "appreciate it"
✅ "ship it"            "all good"         "no more errors"
```

### De-escalation Chain
```
Opus + "Perfect!"
  ↓ (cascade)
Sonnet + "Thanks for the fix"
  ↓ (cascade complete)
Haiku (cost-optimized for next task)
```

**Example Workflow:**
```
You: [escalate to opus for complex debugging]
System: 🚀 Escalated: Opus
Claude: [deep debugging analysis]
You: "Perfect! That fixed it."
System: ⬇️ Auto-downgrade: Sonnet (cascade continues)
Claude: [ready for next task]
You: "Thanks, works great now"
System: ⬇️ Auto-downgrade: Haiku (cascade complete)
Claude: [cost-optimized for next question]
```

## Override Auto-Effort

Use `/effort` or `/model` to force specific settings:

### `/effort low|medium|high`
```
You: /effort high
System: Effort level set to HIGH
Claude: [uses appropriate high-capability model]
```

### `/model haiku|sonnet|opus`
```
You: /model opus
System: Model set to claude-opus-4-6
Claude: [uses Opus for next response]
```

## Cascade Timeout

The system prevents over-optimization with a **5-minute timeout** between cascades:

- First success signal → cascades down (e.g., Opus → Sonnet)
- Second success signal within 5 min → **blocked** (prevents loop)
- After 5 minutes → can cascade again if appropriate

This prevents:
- ❌ Multiple success signals triggering rapid cascades
- ❌ Thrashing between models
- ✅ Preserves cost savings while maintaining stability

## Dashboard Monitoring

Real-time metrics available at `http://localhost:8077`:

```
📊 Current Model        → Opus / Sonnet / Haiku
🎯 Effort Level         → LOW / MEDIUM / HIGH
📈 Total Escalations    → Number of times you escalated
⬇️ Total De-escalations → Number of cascade steps
📉 Cascade Rate         → Success signal frequency
✅ Success Rate         → Problem resolution rate
🔄 Avg Cascade Depth    → Steps per escalation
💰 Token Cost           → Estimated tokens used
```

## Command Reference

### Binary Commands
```bash
escalation-manager stats     # Output JSON stats
escalation-manager version   # Show version
escalation-manager help      # Show help
```

### Slash Commands (in Claude Code)
```
/escalate              # Escalate to Sonnet
/escalate to opus      # Escalate to Opus
/escalate to haiku     # Downgrade to Haiku
/effort low|med|high   # Set effort level
/model opus|sonnet|... # Set specific model
```

## Cost Optimization Tips

1. **Let auto-effort work** — it routes to optimal model automatically
2. **Confirm success** — say "works!" to cascade down and save costs
3. **Use /effort override sparingly** — trust the classification
4. **Monitor dashboard** — see cost savings and patterns
5. **Check stats** — `escalation-manager stats` shows real numbers

## Troubleshooting Usage

### Model not changing
- Check: `jq '.model' ~/.claude/settings.json`
- Try explicit: `/model opus`
- Check hooks running: See TROUBLESHOOTING.md

### De-escalation not triggering
- Use exact phrases: "works", "perfect", "thanks"
- Avoid negation: ❌ "thanks but it broke"  ✅ "thanks, it works"
- Check 5-min timeout between cascades

### Dashboard shows old data
- Refresh browser (or F5)
- Check: `escalation-manager stats`
- Verify binary: `~/.claude/bin/escalation-manager`

## Examples

### Example 1: Simple Question
```
User: "What's the Python syntax for list comprehension?"
System: ⚡ Auto-effort: low → Haiku
Haiku: "[concise explanation]"
Cost: Minimal (Haiku is cheapest)
```

### Example 2: Debugging Complex Issue
```
User: [describes race condition in concurrent code]
System: 🔥 Auto-effort: high → Opus
Opus: [deep analysis with multiple solutions]
User: "Perfect! That explains it."
System: ⬇️ Auto-downgrade: Sonnet
Result: Problem solved, cost-optimized for next task
```

### Example 3: Manual Escalation
```
User: "I'm stuck, can you help debug this?"
System: ⚙️ Auto-effort: medium → opusplan
Sonnet: [analysis, but user still confused]
User: "/escalate to opus"
System: 🚀 Escalated: Opus
Opus: [deeper analysis, problem solved]
```

## Advanced: Understanding Auto-Effort Scoring

The system scores tasks on multiple factors:

**High Complexity Keywords** (+3 points each):
- implement, build system, debug complex, multi-file, scalable, architecture

**Medium Complexity Keywords** (+1 point each):
- test, deploy, configure, integrate, database, API

**Low Complexity Keywords** (-2 points each):
- what is, how do I, show me, quick, simple, typo

**Score → Model Mapping:**
- < 0: Haiku (budget)
- 0-2: Haiku or Sonnet (mixed)
- 2-6: Sonnet (medium)
- 6+: Opus (high)

Override anytime with `/effort` or `/model`.

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) — System design
- [DASHBOARD.md](DASHBOARD.md) — Dashboard API and features
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Common issues
- [SETUP.md](SETUP.md) — Installation guide

