# Real-World Escalation Testing Plan

**Purpose**: Validate escalation system in actual Claude Code sessions  
**Method**: Manual tests with observation and gap identification  
**Duration**: 30-40 minutes  

---

## Test Strategy

### Phase 1: Basic Commands (10 min)
- [ ] /escalate → verify model changes to Sonnet
- [ ] /escalate to opus → verify model changes to Opus
- [ ] /escalate to haiku → verify model changes back to Haiku
- [ ] Barista shows correct emoji/model name
- [ ] Auto-effort hook doesn't override manual /escalate

### Phase 2: De-escalation Signals (15 min)
- [ ] Say "works great" → verify auto-downgrade to Haiku
- [ ] Say "thanks for the help" → verify auto-downgrade
- [ ] Say "perfect solution" → verify auto-downgrade
- [ ] Check statusline shows model change
- [ ] Verify effort level updated

### Phase 3: False Positive Guards (10 min)
- [ ] Say "thanks but it didn't work" → verify NO downgrade
- [ ] Say "thanks however we need more" → verify NO downgrade
- [ ] Say "need to fix this" → verify NO downgrade
- [ ] Multiple scenarios with "but/however"

### Phase 4: Cascade Testing (15 min)
- [ ] /escalate to opus → Opus
- [ ] "Great start!" → Sonnet (cascade step 1)
- [ ] "Perfect!" → Haiku (cascade step 2)
- [ ] Verify statusline updated at each step
- [ ] Verify session cleaned up at final step

### Phase 5: Auto-effort Interaction (10 min)
- [ ] Complex task (auto-effort → opusplan)
- [ ] /escalate to sonnet → overrides auto-effort
- [ ] Simple task (auto-effort → Haiku)
- [ ] /escalate to opus → overrides again
- [ ] Verify /effort respects /escalate commands

### Phase 6: Edge Cases (10 min)
- [ ] Very long prompt with success signal
- [ ] Multiple success signals in one prompt
- [ ] Success signal in code block (should not trigger)
- [ ] Mixed case: "THANKS" or "Works GREAT"
- [ ] Punctuation: "perfect!" vs "perfect?" vs "perfect."

---

## Observation Template

For each test, record:

```
TEST: [test name]
COMMAND: /escalate to sonnet
STATUS: [what happened]
EXPECTED: Sonnet model, effort=high
ACTUAL: [what actually happened]
BARISTA: [what statusline showed]
GAP: [any difference between expected and actual?]
SEVERITY: [none / minor / major]
```

---

## Known Expected Behaviors to Verify

### ✅ Should Work
```
/escalate → Sonnet + high effort
/escalate to opus → Opus + high effort
/escalate to haiku → Haiku + low effort
"works great" → De-escalate to Haiku
"thanks for" → De-escalate to Haiku
"perfect" → De-escalate to Haiku
Opus→Sonnet→Haiku cascade
"thanks but X" → NO change
```

### ⚠️ Potential Gaps to Look For
```
De-escalation timing (delayed vs immediate)
Barista refresh lag (does it show new model immediately?)
Auto-effort override (does /escalate win?)
Session cleanup (is session file deleted?)
Model persistence (does it stay changed across prompts?)
Effort sync (always matches model?)
Error handling (what if settings corrupted?)
Performance (any noticeable lag?)
```

---

## Specific Test Cases

### Test 1: Basic Escalation
```
SETUP: Start in default state (check current model with barista)
ACTION: Type "/escalate"
OBSERVE: 
  - Does model change to Sonnet?
  - Does statusline update immediately or with lag?
  - Does effort change to "high"?
  - Does next response use Sonnet model?
GAPS TO CHECK:
  - Timing of statusline update
  - Any timeout/delay
  - Hook output visible to user
```

### Test 2: De-escalation Success Signal
```
SETUP: /escalate to sonnet (wait for response)
ACTION: Type "Thanks! That works great."
OBSERVE:
  - Does model change to Haiku?
  - What's the exact timing?
  - Does statusline show downgrade message?
  - Does next response use Haiku?
GAPS TO CHECK:
  - Does "works great" detect reliably?
  - Is cascade message shown?
  - Is there any perceptible lag?
  - Does cost info show in barista?
```

### Test 3: Cascade Behavior
```
SETUP: /escalate to opus
ACTION 1: Type something (wait for response)
ACTION 2: Type "Good progress!" (should go Opus→Sonnet)
ACTION 3: Type "Perfect solution!" (should go Sonnet→Haiku)
OBSERVE:
  - Does first de-escalation trigger?
  - Does cascade message show "continuing"?
  - Does second de-escalation trigger?
  - Does cascade message show "complete"?
  - Are session files cleaned up?
GAPS TO CHECK:
  - Timing between cascades
  - Message clarity
  - Session management
  - Whether "Good progress!" triggers (probably not in phrase list)
```

### Test 4: False Positive Guard
```
SETUP: /escalate to sonnet
ACTION: Type "Thanks but it's still broken"
OBSERVE:
  - Does model STAY as Sonnet?
  - Is there any attempt to de-escalate?
  - How does statusline react?
GAPS TO CHECK:
  - Does "thanks but" reliably block?
  - What if no space after "but"? ("thanksbut")
  - What if "but" comes much later?
  - Multiple guards in one sentence?
```

### Test 5: Auto-effort Interaction
```
SETUP: Type a complex implementation task
OBSERVE: Auto-effort picks model (check with /effort)
ACTION: /escalate to haiku
OBSERVE: 
  - Does model actually change to Haiku?
  - Next response: does it use Haiku?
  - Or does auto-effort override?
GAPS TO CHECK:
  - Does /escalate win over auto-effort?
  - Can auto-effort re-escalate after /escalate to haiku?
  - Is there a conflict between systems?
```

### Test 6: Persistence Across Prompts
```
SETUP: /escalate to opus
ACTION 1: Send a prompt → check model used
ACTION 2: Send another prompt → check model still Opus
ACTION 3: Wait 30+ min → check if model reverts
GAPS TO CHECK:
  - Is model persistent?
  - Are settings saved?
  - Is timeout working?
  - What happens after 30 min window?
```

---

## Gap Analysis Framework

### Severity Levels

**CRITICAL** (Breaks functionality):
- De-escalation doesn't trigger when it should
- De-escalation triggers when it shouldn't
- Model doesn't actually change
- Session validation fails

**HIGH** (Affects reliability):
- Timing issues (>5s delay)
- Cascade doesn't work properly
- Barista doesn't update
- False positives/negatives in phrase detection

**MEDIUM** (Affects UX):
- Messages unclear or missing
- Statusline doesn't update promptly
- Performance degradation (<5s)
- Minor phrase misses

**LOW** (Polish):
- Message wording
- Emoji clarity
- Documentation gaps
- Edge case handling

---

## Expected Outputs

After real-world testing, create:

1. **Real-World Test Report**
   - What actually works vs. expected
   - Any timing issues observed
   - Barista refresh behavior
   - Auto-effort interaction results

2. **Gap List**
   - [ ] Gap 1: [description] [Severity]
   - [ ] Gap 2: [description] [Severity]
   - [ ] etc.

3. **Recommendations**
   - Quick fixes for high-severity gaps
   - Phase 2 improvements for lower-severity gaps
   - Documentation updates needed

---

## Test Execution Checklist

### Before Testing
- [ ] Fresh Claude Code session
- [ ] Barista statusline visible
- [ ] settings.json backup created
- [ ] Test suite ready to validate

### During Testing
- [ ] Take notes on each observation
- [ ] Screenshot barista state changes
- [ ] Check hook output in console
- [ ] Monitor response latency
- [ ] Verify actual model used (check response quality)

### After Testing
- [ ] Document all gaps found
- [ ] Categorize by severity
- [ ] Create reproduction steps
- [ ] Suggest fixes
- [ ] Update test suite if needed

---

## Session Recording Template

```
DATE: 2026-04-25
SESSION_ID: [auto-generated]
INITIAL_MODEL: [check with /effort]
INITIAL_EFFORT: [check with /effort]

TEST_1: Basic /escalate
  Time: HH:MM:SS
  Command: /escalate
  Expected: Sonnet, high
  Actual: [result]
  Barista: [showed emoji/model]
  Gap: [Y/N - describe if yes]
  Severity: [none/low/medium/high/critical]

TEST_2: [continue for each test...]
```

---

## Common Real-World Issues to Watch For

1. **Timing**
   - Hook execution delay
   - Barista refresh lag
   - De-escalation lag (when should it trigger - immediately or after response?)

2. **State Management**
   - Does model stick across multiple prompts?
   - Does effort sync stay correct?
   - Are sessions cleaned up properly?

3. **Integration**
   - Does /escalate override auto-effort?
   - Does /model command work alongside?
   - Does /effort command work alongside?

4. **Edge Cases**
   - Very long prompts
   - Special characters in phrases
   - Multiple success signals
   - Rapid escalation/de-escalation

5. **User Experience**
   - Are messages clear?
   - Is statusline helpful?
   - Any surprising behavior?
   - Is cost savings visible?

---

## Success Criteria

Testing is successful if:

- [ ] ✅ All basic /escalate variants work
- [ ] ✅ De-escalation triggers correctly
- [ ] ✅ False positive guards work
- [ ] ✅ Cascade behavior proven
- [ ] ✅ No critical gaps found
- [ ] ✅ Auto-effort interaction verified
- [ ] ✅ Persistence works as expected
- [ ] ✅ Performance acceptable
- [ ] ✅ All gaps documented
- [ ] ✅ Recommendations provided

---

## Next Steps After Testing

1. **Document gaps** in ~/escalation-real-world-gaps.md
2. **Categorize by severity** for prioritization
3. **Create follow-up tasks** for fixes
4. **Update test suite** if needed
5. **Feed findings** into Phase 3+ improvements
