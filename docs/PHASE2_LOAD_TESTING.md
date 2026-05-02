# Phase 2: Load Testing - 1000 req/sec Validation

Complete end-to-end load testing at production scale with zero API costs. Validate that the unified gateway sustains 1000 req/sec with <500ms P99 latency and <200MB memory.

## Quick Start

### Run Mixed Workload (2 min smoke test)
```bash
./cmd/load-test/load-test -scenario=mixed
```

### Run Constant Load (1 hour sustained)
```bash
./cmd/load-test/load-test -scenario=constant
```

### List All Scenarios
```bash
./cmd/load-test/load-test -list-scenarios
```

## Phase 2 Scenarios (8 Total)

### 1. Constant Load (1 hour)
**Purpose**: Verify steady-state performance, memory stability, cache consistency

```bash
./cmd/load-test/load-test -scenario=constant
```

**Configuration**:
- Duration: 60 minutes
- Target: 1000 req/sec sustained
- Workers: 100
- Ramp-up: 5 min, Ramp-down: 5 min

**Success Criteria**:
- P99 latency: <500ms (held throughout)
- Memory: <200MB
- Cache hit rate: >90%
- Error rate: <0.1%

### 2. Burst Load (30 sec)
**Purpose**: Test backpressure handling, queue overflow, burst recovery

```bash
./cmd/load-test/load-test -scenario=burst
```

**Configuration**:
- Duration: 30 seconds
- Peak load: 5000 req/sec (10s spike)
- Workers: 200

**Success Criteria**:
- P99 latency during burst: <1000ms
- Recovery latency: <500ms
- Error rate during spike: <5%

### 3. Ramp-Up (40 min)
**Purpose**: Verify smooth load increase, adaptive scaling, resource allocation

```bash
./cmd/load-test/load-test -scenario=rampup
```

**Configuration**:
- Duration: 40 minutes
- Stages: 100→250→500→750→1000 req/sec (8 min each)
- Workers: 100

**Success Criteria**:
- Latency increases proportional to load
- No sudden failures
- Smooth scaling behavior

### 4. Mixed Workload (2 min)
**Purpose**: Validate routing correctness under realistic load mix

```bash
./cmd/load-test/load-test -scenario=mixed
```

**Configuration**:
- Duration: 2 minutes
- Load: 1000 req/sec
- Workload mix:
  - 60% cache hits (20ms latency)
  - 30% batch jobs (500ms latency)
  - 10% fresh requests (200-300ms latency)

**Success Criteria**:
- Overall success rate: >85%
- Cache hits routed correctly
- Batch jobs processed
- Fresh requests fall back appropriately

### 5. Failure Recovery (2 min)
**Purpose**: Test error handling, retry logic, graceful degradation

```bash
./cmd/load-test/load-test -scenario=recovery
```

**Configuration**:
- Duration: 2 minutes
- Baseline: 500 req/sec
- Failure injection:
  - 0-20s: Normal operation (1% errors)
  - 20-30s: API timeout (50% errors)
  - 30-40s: Partial recovery (20% errors)
  - 40-60s: Back to normal
  - 60-70s: Database lag (5% errors)
  - 70+: Normal again

**Success Criteria**:
- Recovery time: <30 seconds
- Graceful degradation under failure
- Proper error handling
- No cascading failures

### 6. Sustained Load (30 min)
**Purpose**: Validate long-running stability, memory growth, cache consistency

```bash
./cmd/load-test/load-test -scenario=sustained
```

**Configuration**:
- Duration: 30 minutes
- Target: 1000 req/sec sustained
- Workers: 100

**Success Criteria**:
- P99 latency: <200ms (maintained throughout)
- Memory growth: <5% across full duration
- No goroutine leaks
- Cache effectiveness stable

### 7. Spike (90 sec)
**Purpose**: Test adaptive throttling, queue management, recovery

```bash
./cmd/load-test/load-test -scenario=spike
```

**Configuration**:
- Duration: 90 seconds
- Phases:
  - 0-20s: Baseline (100 req/sec)
  - 20-25s: Spike (2000 req/sec)
  - 25-90s: Recovery to baseline

**Success Criteria**:
- Recovery within 30 seconds
- Error rate during spike: <5%
- Queue drains properly
- No request loss

### 8. Degradation (2 min)
**Purpose**: Identify system limits under extreme progressive load

```bash
./cmd/load-test/load-test -scenario=degradation
```

**Configuration**:
- Duration: 2 minutes
- Progressive stages (30s each):
  - Stage 1: 200 req/sec (baseline)
  - Stage 2: 500 req/sec
  - Stage 3: 1000 req/sec
  - Stage 4: 2000 req/sec
  - Stage 5: 5000 req/sec (extreme)

**Success Criteria**:
- Identify breaking point
- Error rate <20% at max load
- Graceful degradation (no crashes)
- Clear performance degradation pattern

## Running Phase 2 Complete Suite

### Quick Validation (10 min total)
```bash
# Test basic functionality
./cmd/load-test/load-test -scenario=mixed
./cmd/load-test/load-test -scenario=spike
./cmd/load-test/load-test -scenario=degradation
```

### Standard Validation (2.5 hour total)
```bash
# Full Phase 2 validation
./cmd/load-test/load-test -scenario=rampup      # 40 min
./cmd/load-test/load-test -scenario=sustained   # 30 min
./cmd/load-test/load-test -scenario=burst       # 30 sec
./cmd/load-test/load-test -scenario=mixed       # 2 min
./cmd/load-test/load-test -scenario=recovery    # 2 min
./cmd/load-test/load-test -scenario=spike       # 90 sec
```

### Production Validation (1+ hours)
```bash
# Longest running scenarios for production readiness
./cmd/load-test/load-test -scenario=constant    # 1 hour
```

## Interpreting Results

### Latency Metrics
- **P50**: 50th percentile latency (median)
- **P95**: 95% of requests faster than this
- **P99**: 99% of requests faster than this (SLA target)
- **P99.9**: 99.9% of requests faster than this (tail behavior)

### Target Latencies by Scenario
| Scenario | P50 | P95 | P99 | P99.9 |
|----------|-----|-----|-----|-------|
| constant | <100ms | <200ms | <500ms | <1000ms |
| sustained | <100ms | <150ms | <200ms | <400ms |
| burst | <200ms | <500ms | <1000ms | <2000ms |
| spike | <150ms | <300ms | <500ms | <1000ms |
| mixed | <150ms | <400ms | <800ms | <1500ms |
| recovery | varies | varies | varies | varies |
| rampup | scales | scales | scales | scales |
| degradation | increasing | increasing | increasing | increasing |

### Success Criteria

✅ **Phase 2 PASS if**:
- Constant scenario: P99 <500ms, error rate <0.1%
- Sustained scenario: P99 <200ms held for 30 min
- Spike scenario: Recovery <30s, errors <5%
- Mixed scenario: Success rate >85%
- Degradation scenario: Clear performance pattern, graceful

❌ **Phase 2 FAIL if**:
- Any scenario shows goroutine leaks
- Memory grows >10% per hour under load
- P99 latency exceeds scenario target by >20%
- Unexpected crashes or hangs
- Request loss during any test

## Performance Monitoring During Tests

### Real-Time Metrics
Each load test scenario prints interim reports every 10-30 seconds:

```
[  30.0s] Requests: 15000 | Rate: 500.0 req/s | Latency: min=10ms avg=45ms max=520ms | Success: 15000 | Errors: 0
[ 60.0s] Requests: 30000 | Rate: 500.0 req/s | Latency: min=10ms avg=46ms max=810ms | Success: 29997 | Errors: 3
```

### Final Report
At test completion, detailed report shows:

```
================================================================================
LOAD TEST RESULTS
================================================================================

Test Duration: 2m0s
Total Requests: 120000
Successful: 119997 (99.997%)
Failed: 3
Requests/sec: 1000.0

Latency Metrics:
  Min: 10ms
  Avg: 45ms
  Max: 1234ms
  P50: 32ms
  P95: 98ms
  P99: 234ms
  P99.9: 567ms

Target Validation:
  ✓ P99 Latency Target (<200ms): PASS (actual: 234ms)
  ✓ Throughput Target (1000 req/s): PASS (actual: 1000.0 req/s)
  ✓ Success Rate Target (>99.5%): PASS (actual: 99.997%)
```

## Custom Load Test

Run arbitrary load configuration:

```bash
./cmd/load-test/load-test \
  -duration 5m \
  -rate 500 \
  -workers 50 \
  -ramp-up 30s \
  -ramp-down 30s
```

## Phase 2 Exit Criteria

✅ **Phase 2 COMPLETE** when:
- [ ] constant scenario: P99 <500ms sustained, memory <200MB
- [ ] sustained scenario: P99 <200ms for full 30 minutes
- [ ] spike scenario: Recovery <30s with <5% errors
- [ ] mixed scenario: >85% success rate with correct routing
- [ ] recovery scenario: Graceful degradation with <30s recovery
- [ ] rampup scenario: Smooth scaling, no sudden failures
- [ ] degradation scenario: Clear performance pattern identified
- [ ] burst scenario: Queue management validated
- [ ] No goroutine leaks detected
- [ ] Memory growth <5% per hour
- [ ] All target latencies met

## Next: Phase 3 (Security Audit)

Once Phase 2 validation passes:

1. Run security audit: `./cmd/security-audit/audit -full`
2. Document findings and remediation
3. Publish security report
4. Move to Phase 4 (Database Finalization)

## Related Files

- `cmd/load-test/main.go` — Load test engine and metrics collection
- `cmd/load-test/scenarios.go` — 8 production scenarios
- `internal/test/load_test.go` — Benchmark tests
- `internal/batch/router.go` — Routing under load
- `internal/cache/semantic.go` — Cache behavior under load
- `docs/PHASE1_INTEGRATION_TESTING.md` — Phase 1 reference
