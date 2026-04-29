# Multi-CLI Implementation Roadmap

**Status**: Ready for Development  
**Total Effort**: 8-13 weeks (phased, parallelizable)  
**Team Capacity**: 1 developer (you) can complete this full-time in 2-3 months

---

## Executive Decision Matrix

| Decision | Recommendation | Rationale |
|----------|---|---|
| **Proceed with multi-CLI?** | ✅ YES | Core execution feedback loop is universally feasible. Feature coverage 85%+ is acceptable. |
| **Start with Phase 1?** | ✅ YES | Low risk, high value. Unlocks analytics for all 4 CLIs immediately. |
| **Include Copilot?** | ⚠️ YES with reduced scope | Proprietary API limits features, but execution logging still valuable. 45% feature coverage acceptable as "v1". |
| **Parallel development?** | ✅ YES | Providers are independent. Build Claude + Gemini in parallel, OpenAI + Copilot sequential. |
| **Full feature parity?** | ❌ NO | Accept 60-85% parity per provider. Sentiment & escalation are "nice to have", not critical. |

---

## Phase Breakdown & Timeline

### ✅ Phase 1: Provider Abstraction + Logging (3 weeks)
**Effort**: Medium  
**Risk**: Low  
**Value**: High (100% feature coverage on execution loop)

**Week 1: Foundation**
- [ ] Design provider interface (`internal/providers/provider.go`)
- [ ] Implement ClaudeProvider (reuse existing code)
- [ ] Implement GeminiProvider (new, medium complexity)
- [ ] Unit tests: 50 tests, 100% coverage

**Week 2: OpenAI + Copilot**
- [ ] Implement OpenAIProvider (reuse existing code)
- [ ] Implement CopilotProvider (new, high complexity — proprietary API)
- [ ] Provider factory pattern
- [ ] Authentication validation for all 4

**Week 3: Multi-Provider Logging**
- [ ] Extend execution log types with provider field
- [ ] Implement MultiProviderLogger
- [ ] Backwards compatibility: old .execution-log.jsonl still readable
- [ ] Integration tests: log from all 4 providers
- [ ] Update CLAUDE.md with new provider configuration

**Deliverables**:
- All 4 providers integrated and tested
- Unified execution logging working
- `.execution-log.jsonl` includes provider field
- Backwards compatible with old logs

**Exit Criteria**:
- ✅ All tests pass (including -race)
- ✅ Can log operations from all 4 providers
- ✅ Cost calculation works per provider
- ✅ Config schema accepts all 4 providers

---

### ✅ Phase 2: Multi-Provider Analytics (2 weeks)
**Effort**: Low-Medium  
**Risk**: Low  
**Value**: High (unified dashboard, cost comparison)

**Week 1: Analytics Engine**
- [ ] Implement MultiProviderAnalytics
- [ ] Add cost breakdown by provider
- [ ] Add per-provider operation stats
- [ ] Provider comparison metrics

**Week 2: Dashboard Integration**
- [ ] Add "Providers" tab to dashboard
- [ ] Per-provider cost charts
- [ ] Cross-provider comparison table
- [ ] "Cheapest provider for this query" recommendation
- [ ] Pattern generation across providers

**Deliverables**:
- Dashboard shows multi-provider breakdown
- Real-time cost comparison
- "Switch to cheaper provider" recommendations

**Exit Criteria**:
- ✅ Dashboard updated with provider breakdown
- ✅ Accurate cost calculation per provider
- ✅ Cross-provider metrics working

---

### ⚠️ Phase 3: Sentiment Detection (2 weeks)
**Effort**: Medium  
**Risk**: Medium  
**Value**: Medium (feature parity on 3/4 providers)

**Note**: Skip Copilot (impossible without API). Implement Claude, Gemini, OpenAI.

**Week 1: Per-Provider Implementation**
- [ ] SentimentDetectorFactory with provider selection
- [ ] ClaudeSentimentDetector (reuse existing code)
- [ ] GeminiSentimentDetector (new, reuses GeminiProvider)
- [ ] OpenAISentimentDetector (new, reuses OpenAIProvider)

**Week 2: Integration + Testing**
- [ ] Sentiment escalation per provider
- [ ] Cross-provider sentiment trends
- [ ] Fallback if primary provider unavailable
- [ ] Cost analysis: sentiment detection overhead per provider

**Deliverables**:
- Sentiment detection on Claude, Gemini, OpenAI
- Provider-aware escalation triggering
- Sentiment trends analytics

**Exit Criteria**:
- ✅ Sentiment detection works on 3 providers
- ✅ Escalation triggered per provider
- ✅ Analytics show sentiment trends

---

### ⚠️ Phase 4: Model Escalation (3-4 weeks)
**Effort**: Medium-High  
**Risk**: Medium (different escalation chains per provider)  
**Value**: Medium (useful but not critical)

**Week 1: Per-Provider Escalation Logic**
- [ ] ClaudeEscalationStrategy (Haiku → Sonnet → Opus)
- [ ] GeminiEscalationStrategy (Flash → Pro)
- [ ] OpenAIEscalationStrategy (GPT-3.5 → GPT-4o)
- [ ] CopilotEscalationStrategy (stub - no control)

**Week 2: Cross-Provider Orchestration**
- [ ] ExecutionEngine: implement FallbackPolicy
- [ ] When Claude budget hit → escalate to Gemini
- [ ] When Gemini budget hit → escalate to OpenAI
- [ ] Max depth to prevent infinite loops

**Week 3: Cost-Aware Escalation**
- [ ] Recommendation: "Escalate to Gemini Flash instead of Claude Opus" (7x cheaper)
- [ ] Automatic provider switching based on cost/capability tradeoff
- [ ] Cross-provider model selection logic

**Week 4: Testing + Analytics**
- [ ] Integration tests: fallback chains
- [ ] Analytics: track fallback frequency + cost savings
- [ ] Dashboard: "Fallback recommendations"

**Deliverables**:
- Per-provider model escalation
- Cross-provider fallback orchestration
- Cost-aware escalation decisions
- Analytics on fallback patterns

**Exit Criteria**:
- ✅ Model escalation works per provider
- ✅ Cross-provider fallback prevents budget overages
- ✅ Max depth prevents loops
- ✅ Analytics show fallback patterns

---

### ✅ Phase 5: Budget Enforcement (2 weeks)
**Effort**: Low-Medium  
**Risk**: Low  
**Value**: High (spend visibility, cost control)

**Week 1: Per-Provider Budgets**
- [ ] Extend BudgetTracker for multi-provider
- [ ] Per-provider daily/monthly limits
- [ ] Hard vs soft limits per provider

**Week 2: Alerts + Analytics**
- [ ] Budget alerts: 50%, 80%, 100% spent
- [ ] Per-provider budget reports
- [ ] Cross-provider spending trends
- [ ] Dashboard: "Budget status" per provider

**Deliverables**:
- Per-provider budget enforcement
- Spending visibility across providers
- Alerts and limits

**Exit Criteria**:
- ✅ Budget tracking per provider works
- ✅ Hard limits enforced
- ✅ Dashboard shows budget status

---

## Parallel Development Strategy

**Week 1**: You implement Phase 1 sequentially (foundation, then providers)
**Weeks 2-13**: Parallelizable phases

```
Week 1: Phase 1 Foundation (sequential)
│
├─ Weeks 2-3: Phase 1 Completion (sequential)
│  └─ Provider implementations + logging
│
├─ Weeks 4-5: Phase 2 (Analytics) + Phase 3 (Sentiment Detection)
│  ├─ Day 1-3: Phase 2 dashboard updates
│  ├─ Day 4-7: Phase 3 sentiment detectors (parallel development)
│  └─ End of week: both complete, integrate
│
├─ Weeks 6-9: Phase 4 (Escalation)
│  ├─ Week 6: Per-provider strategies
│  ├─ Week 7: Cross-provider orchestration
│  ├─ Week 8: Cost-aware escalation
│  └─ Week 9: Testing + analytics
│
└─ Weeks 10-11: Phase 5 (Budget)
   ├─ Week 10: Per-provider budgets
   └─ Week 11: Alerts + analytics

Total: 11 weeks (3 weeks faster than sequential 8-13 week estimate)
```

---

## Critical Path Dependencies

```
Phase 1 ──→ Phase 2 (must have logging before analytics)
Phase 1 ──→ Phase 3 (must have providers before sentiment)
Phase 3 ──→ Phase 4 (sentiment enables escalation logic)
Phase 1 ──→ Phase 5 (must have logging before budget tracking)

Independent (can start anytime after Phase 1):
- Phase 2 ← Phase 3 (parallel)
- Phase 4 ← Phase 5 (parallel)
```

---

## Risk Mitigation Strategies

### Risk 1: Copilot API Changes (Severity: High)
**Mitigation**:
- [ ] Implement CopilotProvider as thin wrapper (easy to update)
- [ ] Document current Copilot API behavior
- [ ] Monitor GitHub Copilot repo for API changes
- [ ] Fallback to CLI output parsing if API breaks

### Risk 2: Authentication Token Leaks (Severity: High)
**Mitigation**:
- [ ] Never log API keys or auth tokens
- [ ] Sanitize all logged operations
- [ ] Use environment variables for all secrets
- [ ] Add token redaction in multi_provider_logger.go

### Risk 3: Token Counting Inconsistencies (Severity: Medium)
**Mitigation**:
- [ ] Store actual token counts from API (not estimated)
- [ ] Document token counting methodology per provider
- [ ] Add manual override: `escalate config --set-token-count=provider:model:120`
- [ ] Alert user if token counts deviate >10% from expected

### Risk 4: Cost Calculation Errors (Severity: Medium)
**Mitigation**:
- [ ] Unit test cost calculation for all providers
- [ ] Verify against actual pricing pages
- [ ] Add manual cost override in config
- [ ] Dashboard shows "estimated" where uncertain

### Risk 5: Cross-Provider Fallback Loops (Severity: High)
**Mitigation**:
- [ ] Max fallback depth = 2 (Claude → Gemini → done, no OpenAI)
- [ ] Log all fallback chains for audit
- [ ] Alert user after >3 fallbacks in 1 hour
- [ ] Require manual confirmation for cross-provider

---

## Testing Strategy

### Unit Tests (by phase)
- **Phase 1**: Provider interface + each implementation (50 tests)
- **Phase 2**: Analytics calculations (30 tests)
- **Phase 3**: Sentiment detection per provider (40 tests)
- **Phase 4**: Escalation logic + fallback (60 tests)
- **Phase 5**: Budget tracking + alerts (40 tests)

**Total**: ~220 unit tests

### Integration Tests
- [ ] Log from all 4 providers simultaneously
- [ ] Cost calculation accuracy per provider
- [ ] Sentiment escalation across providers
- [ ] Cross-provider fallback chains
- [ ] Budget enforcement per provider
- [ ] Dashboard updates with multi-provider data

**Total**: ~30 integration tests

### Backwards Compatibility Tests
- [ ] Old .execution-log.jsonl files still readable
- [ ] Claude-only config still works (no new fields required)
- [ ] Dashboard works with mixed old/new logs
- [ ] Migration path for users

### Performance Tests
- [ ] Logging overhead < 50ms per operation (all providers)
- [ ] Analytics dashboard load < 2s (with 10K logs)
- [ ] Cross-provider Analytics queries < 1s
- [ ] No memory leaks (goroutine monitoring)

---

## Configuration Migration Guide

### For Claude-Only Users (Current)
```yaml
# Old config (Phase 1: no changes needed)
models:
  haiku: claude-3-5-haiku-20241022
  sonnet: claude-3-5-sonnet-20241022
  opus: claude-opus-4-1-20250805

# Continues to work after multi-CLI implementation
# Automatically becomes "providers.claude"
```

### For Multi-CLI Users (Phase 2+)
```yaml
# New config
providers:
  claude:
    enabled: true
    models:
      cheap: claude-3-5-haiku-20241022
    budgets:
      daily_usd: 5.00
  
  gemini:
    enabled: true
    models:
      cheap: gemini-1.5-flash
    budgets:
      daily_usd: 10.00
```

**Migration tool**: `escalate config --migrate-to-multi-cli`
- Automatically converts old config to new format
- Preserves all settings
- Creates provider entries from existing models

---

## Success Metrics (Go-No-Go Criteria)

### Phase 1: Foundation
- [ ] ✅ All 4 providers implemented
- [ ] ✅ Execution logging works on all providers
- [ ] ✅ No data races (go test -race ./...)
- [ ] ✅ 100+ unit tests passing
- [ ] ✅ Config schema accepts all 4 providers

### Phase 2: Analytics
- [ ] ✅ Dashboard shows multi-provider breakdown
- [ ] ✅ Cost comparison accurate (verified manually)
- [ ] ✅ Cross-provider trends visible
- [ ] ✅ 30+ new tests passing

### Phase 3: Sentiment Detection
- [ ] ✅ Sentiment detection on 3/4 providers
- [ ] ✅ Escalation triggers correctly
- [ ] ✅ 40+ tests passing
- [ ] ⚠️ Accept Copilot unavailable (documented)

### Phase 4: Escalation
- [ ] ✅ Per-provider escalation working
- [ ] ✅ Cross-provider fallback prevents overages
- [ ] ✅ Max depth prevents loops
- [ ] ✅ 60+ tests passing

### Phase 5: Budget
- [ ] ✅ Per-provider budgets enforced
- [ ] ✅ Alerts work correctly
- [ ] ✅ Analytics show budget status
- [ ] ✅ 40+ tests passing

### Overall
- [ ] ✅ 250+ unit tests passing
- [ ] ✅ All integration tests passing
- [ ] ✅ Zero data races
- [ ] ✅ Backwards compatible
- [ ] ✅ Documentation complete

---

## Effort Estimation Breakdown

| Phase | Component | Effort | Risk | Owner |
|-------|-----------|--------|------|-------|
| 1 | Provider interface | 2 days | Low | You |
| 1 | Claude provider | 1 day | Low | You |
| 1 | Gemini provider | 3 days | Medium | You |
| 1 | OpenAI provider | 2 days | Low | You |
| 1 | Copilot provider | 4 days | High | You |
| 1 | Multi-provider logger | 2 days | Low | You |
| 1 | Config schema | 1 day | Low | You |
| 1 | Testing | 3 days | Low | You |
| **Phase 1 Total** | | **3 weeks** | | |
| 2 | Analytics engine | 3 days | Low | You |
| 2 | Dashboard updates | 2 days | Low | You |
| 2 | Testing | 2 days | Low | You |
| **Phase 2 Total** | | **2 weeks** | | |
| 3 | Sentiment detectors | 4 days | Medium | You |
| 3 | Integration | 1 day | Low | You |
| 3 | Testing | 2 days | Medium | You |
| **Phase 3 Total** | | **2 weeks** | | |
| 4 | Per-provider escalation | 3 days | Medium | You |
| 4 | Cross-provider fallback | 3 days | High | You |
| 4 | Cost-aware escalation | 2 days | Medium | You |
| 4 | Testing | 4 days | High | You |
| **Phase 4 Total** | | **3 weeks** | | |
| 5 | Per-provider budgets | 2 days | Low | You |
| 5 | Alerts + analytics | 2 days | Low | You |
| 5 | Testing | 2 days | Low | You |
| **Phase 5 Total** | | **2 weeks** | | |
| | **TOTAL** | **12 weeks** | | |

**Time with parallelization**: 8-10 weeks (can do Phases 2-5 partially in parallel)

---

## Decision Checkpoints

### ✅ Go/No-Go: Phase 1
**When**: End of week 3  
**Criteria**:
- All 4 providers working
- Execution logging complete
- No data races

**Decision**: Proceed to Phase 2 (unless Copilot issues discovered)

### ⚠️ Conditional Go: Phase 2-3
**When**: End of week 5  
**Decision Criteria**:
- If Copilot API stable: proceed with Phase 4 (escalation)
- If Copilot API unstable: defer Phase 4, focus on Phase 5 (budget)
- If sentiment detection < 60% coverage: defer Phase 3, focus on Phase 4-5

### ✅ Full Go: Phase 4-5
**When**: End of week 11  
**Decision**: Release multi-CLI version 1.0 with 85% feature coverage

---

## Next Steps (Immediate)

1. ✅ **You review this analysis** (now)
2. ⏭️ **Decision**: Proceed with Phase 1? (Week of May 6)
3. ⏭️ **Create Phase 1 implementation plan** (detailed task breakdown)
4. ⏭️ **Start provider interface design** (internal/providers/provider.go)
5. ⏭️ **Implement ClaudeProvider first** (build on existing code)

---

## FAQ

**Q: Will this break existing Claude-only users?**  
A: No. Backwards compatible. Old config continues to work.

**Q: How much does multi-CLI cost?**  
A: Depends on provider. Gemini Flash = 6-7x cheaper than Claude Haiku. OpenAI = comparable to Claude. See pricing section.

**Q: Can I disable multi-CLI and stay Claude-only?**  
A: Yes. Config: `providers: { gemini: { enabled: false }, openai: { enabled: false }, copilot: { enabled: false } }`

**Q: What if authentication fails for a provider?**  
A: Falls back to next provider (if enabled). If all fail, returns error.

**Q: Can I have separate budgets per provider?**  
A: Yes. Each provider has independent daily/monthly limits.

**Q: Will sentiment detection work for Copilot?**  
A: No, not possible with proprietary API. Copilot gets best-effort logging only (45% feature coverage).

**Q: How do I migrate from Claude-only to multi-CLI?**  
A: Run `escalate config --migrate-to-multi-cli`. No action required.

---

## Related Documents

- [MULTI_CLI_FEASIBILITY_ANALYSIS.md](./MULTI_CLI_FEASIBILITY_ANALYSIS.md) — Detailed feasibility analysis
- [MULTI_CLI_ARCHITECTURE.md](./MULTI_CLI_ARCHITECTURE.md) — Technical architecture + code
- [CLAUDE.md](../CLAUDE.md) — User instructions (updated with multi-CLI guidance)
