# Your First Model Escalation

A step-by-step walkthrough of how model escalation works.

## Setup
Before starting, make sure you've completed the [5-Minute Setup](5-minute-setup.md).

## The Scenario

You're going to ask Claude something complex, watch it fail, then escalate to a better model.

## Step 1: Start Simple (Haiku)

Service automatically starts with **Haiku** (cheapest) for simple tasks. Ask an easy question:

```
You: "What is machine learning?"
```

→ System detects: LOW effort (keyword "what is")  
→ System routes to: **Haiku** (fast & cheap)  
→ Result: ✅ Works great, saves tokens

## Step 2: Try Something Harder (Still Haiku)

Now ask something more complex that Haiku struggles with:

```
You: "Design a thread-safe concurrent cache in Go with exponential backoff retry logic"
```

→ System detects: HIGH effort (keywords: threading, concurrency, design)  
→ System still routes to: **Haiku** (because nothing failed yet)  
→ Haiku gives answer but it's incomplete/buggy
→ Result: ❌ Not quite right

## Step 3: Trigger Escalation

Now ask a follow-up showing frustration:

```
You: "That's still broken. The concurrency issue is still there. Can you fix it?"
```

→ System detects: **FRUSTRATION** (keywords: "still broken", "still there")  
→ System detects: 2 failed attempts in short time  
→ System recommends: `/escalate to sonnet`  
→ In statusline you see: 🚀 Escalation suggestion

## Step 4: Execute Escalation

You can either:

**Option A: Manual escalation** (you decide)
```
You: "/escalate to sonnet"
```

→ System switches model: Haiku → **Sonnet**  
→ System updates settings.json  
→ Next prompt uses Sonnet

**Option B: Auto-escalation** (system decides after 2+ attempts)
```
You: "Can you try a different approach? The threading is still failing."
```

→ System auto-detects: 3rd attempt + frustration  
→ System escalates automatically to **Sonnet**  
→ No `/escalate` command needed

## Step 5: Watch It Work

With Sonnet:
```
Sonnet provides better concurrent design, handles edge cases, includes proper locking
```

→ Result: ✅ Works perfectly  
→ System detects: **SUCCESS** (you say "thanks" or "perfect")

## Step 6: Auto-Downgrade

After success, you continue:

```
You: "Great! Now how do I add rate limiting?"
```

→ System detects: SUCCESS on previous task  
→ System calculates: Sonnet is over-provisioned for this simple follow-up  
→ System auto-downgrades: Sonnet → **Haiku**  
→ Result: ✅ Saves tokens while still working

## Summary of Escalation

```
Simple question (Haiku) ✅
  ↓
Complex question (Haiku) ❌ Didn't work
  ↓ After 2 failures + frustration
Escalate to Sonnet ✅ Works!
  ↓
Simple follow-up
  ↓ After success
Auto-downgrade to Haiku ✅ Saves tokens
```

## What Happened Behind the Scenes

### Phase 1: Hook Analysis
```
Prompt received → Analyze effort → Estimate tokens → Create validation record
```

### Phase 2: Real-Time Monitoring  
```
Claude generating response → Track actual tokens → Update progress
```

### Phase 3: Validation
```
Response complete → Compare estimate vs actual → Learn pattern
```

## Signals System Recognizes

The system automatically detects:

**Frustration Signals** (triggers escalation):
- "still broken", "still failing", "not working"
- "again", "retry", "one more time"
- "/escalate" command

**Success Signals** (enables de-escalation):
- "thanks", "perfect", "that works"
- "exactly", "great", "got it"
- Longer time before follow-up question

**Confusion Signals** (escalates for clarity):
- "why", "confused", "don't understand"
- Multiple questions about same issue
- Rapid follow-ups

## Viewing Your History

Open the dashboard to see what happened:
```bash
escalation-manager dashboard
```

Or check stats:
```bash
escalation-manager validation stats
escalation-manager validation metrics --recent-hours 1
```

## Next Steps

- Learn about [Sentiment Detection](../architecture/sentiment-detection.md)
- Set up [Token Budgets](budgets-setup.md)
- Read the [System Overview](../architecture/overview.md)

## Pro Tips

1. **Escalation is reversible** — You can `/escalate down` to go back
2. **System learns** — Patterns of which models work for which tasks get stored
3. **You're in control** — You can always manual escalate with `/escalate to opus`
4. **No wasted tokens** — System de-escalates after solving to save money

Happy escalating! 🚀
