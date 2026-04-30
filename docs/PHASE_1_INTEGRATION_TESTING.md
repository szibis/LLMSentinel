# Phase 1: Integration Testing with Real Anthropic API

**Status**: Planning → Implementation  
**Objective**: Validate core claims against real Anthropic API  
**Exit Criteria**: 100+ integration tests, 95%+ passing, claims validated  
**Effort**: 1-2 weeks

---

## Overview

Phase 1 validates that LLMSentinel's core claims work correctly against the real Anthropic API:

| Claim | Current Verification | Phase 1 Goal |
|-------|---|---|
| Batch API integration works at scale | Unit tests only | Real API batch submission + polling |
| Semantic cache achieves 98% hit rate | Never tested | Real API: measure >90% hit rate |
| Input optimization saves 40-60% tokens | Model-based (untested) | Real API: measure actual savings |
| Knowledge graph queries <10ms | Local tests only | Real-world scale testing |

---

## Setup

### Prerequisites

1. **Anthropic API Account**
   - Create account at https://console.anthropic.com/
   - Get API key from API settings
   - (Recommended) Create a separate test/sandbox account for CI/CD

2. **Environment Variables**
   ```bash
   export ANTHROPIC_API_KEY=sk-ant-your-key-here
   ```

3. **Go 1.26+**
   ```bash
   go version
   ```

### Installation

```bash
# Clone repository
git clone https://github.com/szibis/LLMSentinel
cd LLMSentinel

# Download dependencies
go mod download

# Copy environment template
cp .env.example .env
# Edit .env with your API key
```

---

## Test Structure

### Test Categories

**Internal Tests** (exist, no API key required):
- Intent classification
- Cache decision logic  
- Security validation
- Metrics tracking

**Integration Tests** (Phase 1, requires API key):
- Real API connectivity
- Batch job submission
- Batch status polling
- Message creation
- Token usage measurement
- Cache effectiveness

**Load Tests** (Phase 2):
- Sustained 1000 req/sec
- Memory efficiency
- Goroutine management

---

## Running Phase 1 Tests

### Run All Tests (Skip Integration Tests)
```bash
go test ./... -v
```

### Run Only Integration Tests
```bash
# Must set ANTHROPIC_API_KEY
export ANTHROPIC_API_KEY=sk-ant-...
go test ./internal/test -v -run Integration
```

### Run Specific Integration Tests
```bash
# Test batch API submission
go test ./internal/test -v -run TestBatchAPISubmission

# Test semantic cache
go test ./internal/test -v -run TestSemanticCacheHitRate

# Test token savings
go test ./internal/test -v -run TestTokenSavingsValidation
```

### Run Short Tests Only (No Long-Running Tests)
```bash
go test ./... -short
```

---

## Test Implementation Plan

### Test 1: Real API Sanity Check
**What**: Verify basic API connectivity  
**How**: Send simple message to real API, verify response structure  
**Expected**: Response includes ID, content, token counts  
**Assertion**: Non-empty response with valid JSON structure  

**Code Location**: `internal/test/integration_test.go::TestRealAPISanity`

```go
// Validates:
// - API key authentication
// - Request/response marshaling
// - Basic latency baseline
```

### Test 2: Batch API Submission
**What**: Submit batch job to real API  
**How**:
1. Create 10 diverse requests
2. Submit batch job
3. Verify job ID returned
4. Verify status transitions (pending → processing)

**Expected**: Job accepted, ID returned, status updates observed  
**Assertion**: Job ID non-empty, status reflects processing  

**Code Location**: `internal/test/integration_test.go::TestBatchAPISubmissionIntegration`

**Metrics to Collect**:
- Submission latency
- Job ID format
- Status transition timing
- Request counts reflected correctly

### Test 3: Semantic Cache Effectiveness
**What**: Measure real cache hit rate  
**How**:
1. Submit 30 semantically similar queries
2. Track cache hits vs misses
3. Measure token savings from hits

**Expected**: 90%+ hit rate on semantically similar queries  
**Assertion**: Hit rate >= 0.90, token savings proportional  

**Code Location**: `internal/test/integration_test.go::TestSemanticCacheHitRate`

**Metrics to Collect**:
- Hit rate (target: >90%)
- Tokens saved per hit
- Cache retrieval latency (<5ms)
- Semantic similarity scores

### Test 4: Token Savings Validation
**What**: Verify input optimization claims  
**How**:
1. Send unoptimized query (full text)
2. Send optimized query (compressed)
3. Compare input token counts
4. Verify output quality unchanged

**Expected**: 40-60% reduction in input tokens  
**Assertion**: Savings within expected range, outputs equivalent  

**Code Location**: `internal/test/integration_test.go::TestTokenSavingsValidation`

**Metrics to Collect**:
- Original token count
- Optimized token count
- Savings percentage (target: 40-60%)
- Compression algorithm performance
- Output quality (token count in response)

### Test 5: Knowledge Graph Query Latency
**What**: Measure real graph query performance  
**How**:
1. Insert 1000+ nodes into knowledge graph
2. Query by semantic embedding
3. Measure response latency
4. Verify result accuracy

**Expected**: P99 latency < 10ms  
**Assertion**: All queries respond in <10ms (P99)  

**Code Location**: `internal/test/integration_test.go::TestKnowledgeGraphQueryLatency`

**Metrics to Collect**:
- Query latency distribution (P50, P99, P999)
- Result accuracy (embeddings match)
- Database read performance
- Index effectiveness

### Test 6: End-to-End Optimization Flow
**What**: Validate complete pipeline  
**How**:
1. User input → Security validation
2. Intent classification
3. Cache lookup (semantic)
4. Batch API routing decision
5. Input optimization
6. API submission
7. Result collection & formatting

**Expected**: All stages complete successfully, metrics published  
**Assertion**: Each stage produces expected output  

**Code Location**: `internal/test/integration_test.go::TestEndToEndOptimizationFlow`

**Metrics to Collect**:
- Total pipeline latency
- Per-stage latency breakdown
- Cache hit/miss decision correctness
- Batch vs immediate routing decision
- Final token count vs original

---

## Test Fixtures & Data

### Sample Payloads

Generated by `test.GenerateTestPayloads()`:

```go
{
  CustomID: "quick_summary_1",
  Params: {
    Model: "claude-3-5-sonnet-20241022",
    MaxTokens: 200,
    Messages: [{Role: "user", Content: "Summarize..."}]
  }
}
```

### Expected Baselines

| Metric | Expected Value | Actual | Status |
|--------|---|---|---|
| Batch submission latency | <1000ms | TBD | 🔴 |
| Semantic cache hit rate | >90% | TBD | 🔴 |
| Token savings (optimized) | 40-60% | TBD | 🔴 |
| Graph query P99 latency | <10ms | TBD | 🔴 |
| Batch job completion time | <1min | TBD | 🔴 |

---

## API Assertions Helper

The `APIAssertions` type in `internal/test/api_assertions.go` provides:

```go
// Create assertions helper
assert := test.NewAPIAssertions(t)

// Validate token savings
metrics := assert.AssertTokenSavings(1000, 400) // 60% savings

// Validate cache effectiveness
cache := assert.AssertCacheHitRate(100, 95) // 95% hit rate

// Validate latency
latency := assert.AssertLatency([]float64{...}, 500.0) // P99 < 500ms

// Validate batch submission
job, _ := assert.AssertBatchAPISubmission(ctx, client, requests)
```

---

## CI/CD Integration

### GitHub Actions Workflow

Phase 1 tests should run in CI/CD but only when:
1. API key is available (skip in PRs by default)
2. Test is not marked as `short`
3. Environment is ready

**Example workflow** (see `.github/workflows/integration-tests.yml`):

```yaml
name: Integration Tests

on:
  push:
    branches: [main]
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM UTC

jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      
      - name: Run Integration Tests
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          go test ./internal/test -v -timeout 30m
```

---

## Success Metrics

### Must Pass (Phase 1 Exit Criteria)

✅ Batch API submission: Job accepted, ID returned, status tracked  
✅ Cache effectiveness: 90%+ hit rate measured  
✅ Token savings: 40-60% reduction validated  
✅ Graph queries: <10ms latency confirmed  
✅ End-to-end: Full pipeline executes without errors  

### Test Coverage

✅ 100+ test cases (combinations of payloads, scenarios, assertions)  
✅ 95%+ tests passing against real API  
✅ All failure modes documented  
✅ Regression tests for each validated claim  

### Documentation

✅ README for running tests  
✅ API assertion helpers documented  
✅ Baseline metrics published  
✅ CI/CD integration complete  

---

## Troubleshooting

### "ANTHROPIC_API_KEY not set"
```bash
export ANTHROPIC_API_KEY=sk-ant-your-key
go test ./internal/test -v -run Integration
```

### "API error: 401 Unauthorized"
- Check API key is valid
- Verify key has sufficient quota
- Check key hasn't been rotated

### "Batch job stuck in processing"
- Normal behavior for actual batch jobs (can take minutes)
- Use poll interval of 10-30 seconds in production
- Check job output file when complete

### "Token counts don't match claims"
- May indicate optimization not applied
- Verify compression algorithm active
- Check system prompt length
- Log full request/response for analysis

---

## Next Steps

1. Set up API credentials
2. Run Phase 1 tests: `go test ./internal/test -v -run Integration`
3. Document actual baseline metrics
4. Fix any failing assertions
5. Publish results to PHASE_1_RESULTS.md
6. Proceed to Phase 2 (Load Testing)

---

## Related Documentation

- [Production Roadmap](../DEVELOPMENT_ROADMAP.md) — Full 7-phase plan to v1.0.0
- [Production Status](../PRODUCTION_STATUS.md) — Current production readiness
- [Load Testing Guide](./PHASE_2_LOAD_TESTING.md) — Phase 2 procedures
- [API Documentation](./API.md) — Anthropic API integration details
