# Phase 2: Load Testing to 1000 req/sec Sustained

**Status**: Implementation Ready  
**Objective**: Validate production-scale performance claims  
**Exit Criteria**: 1000 req/sec sustained, <500ms P99, <200MB memory, >90% cache  
**Effort**: 1-2 weeks (can run parallel to Phase 1)

---

## Overview

Phase 2 extends load testing infrastructure to validate that LLMSentinel meets production SLOs:

| SLO | Target | Test Scenario | Status |
|-----|--------|---|---|
| Sustained throughput | 1000 req/sec | constant load 1 hour | 🔴 |
| P99 latency | <500ms | constant load | 🔴 |
| Memory efficiency | <200MB peak | constant load | 🔴 |
| Cache hit rate | >90% sustained | mixed workload | 🔴 |
| Goroutine leaks | <5 per 1000 reqs | all scenarios | 🔴 |
| Burst recovery | <30s to baseline | failure recovery | 🔴 |

---

## Load Test Scenarios

### Scenario 1: Constant Load (1000 req/sec, 1 hour)

**Purpose**: Validate steady-state performance and stability  
**Workload**: 1000 req/sec with ramp-up (5 min) and ramp-down (5 min)  
**Duration**: 1 hour total (50 min sustained)  
**Workers**: 100 concurrent workers

**Expected Behavior**:
- Request latency: 100-200ms median
- P99 latency: <500ms
- Memory: Stable, <200MB after ramp-up
- Error rate: <0.1%
- Cache hit rate: >90%

**Command**:
```bash
./load-test -scenario=constant
```

**Success Criteria**:
✓ Completes full 1 hour without degradation  
✓ P99 latency remains <500ms throughout  
✓ Memory growth <1% per hour  
✓ No goroutine leaks detected  

---

### Scenario 2: Burst Load (5000 req/sec spike)

**Purpose**: Test backpressure handling and burst recovery  
**Workload**: Spike from 1000 to 5000 req/sec over 30 seconds  
**Duration**: 30 seconds total
**Workers**: 200 workers (more to simulate rapid growth)

**Expected Behavior**:
- During burst (5000 req/sec): P99 <1000ms
- During spike: error rate <5%
- After spike: recover to baseline in <10s
- Queue depth: visible spike then drain

**Command**:
```bash
./load-test -scenario=burst
```

**Success Criteria**:
✓ Handles 5x spike without complete failure  
✓ Error rate during spike <5%  
✓ Recovers to <500ms P99 within 10s  
✓ No connection pool exhaustion  

---

### Scenario 3: Connection Churn (Rapid connect/disconnect)

**Purpose**: Validate connection pool stability and resource cleanup  
**Workload**: 200 workers connecting/disconnecting rapidly  
**Duration**: 30 seconds
**Rate**: 500 req/sec baseline

**Expected Behavior**:
- Connection pool size stable
- No goroutine leaks
- Latency: 50-200ms mostly
- Occasional reconnects: 200ms

**Command**:
```bash
./load-test -scenario=churn
```

**Success Criteria**:
✓ No goroutine growth after test ends  
✓ Connection pool cleans up properly  
✓ No resource handles leaked  
✓ Success rate >98%  

---

### Scenario 4: Mixed Workload (60% cache, 30% batch, 10% fresh)

**Purpose**: Test realistic workload with diverse request types  
**Workload**:
- 60%: Cache hits (20-50ms)
- 30%: Batch jobs (500-1000ms)
- 10%: Fresh requests (200-500ms)

**Duration**: 2 minutes  
**Rate**: 1000 req/sec  
**Workers**: 100

**Expected Behavior**:
- Correct routing decisions (batch → batch, cache → cache)
- Cache hit rate: >90% on cache-eligible requests
- Batch API: correct submission and polling
- Fresh requests: correct model routing

**Command**:
```bash
./load-test -scenario=mixed
```

**Success Criteria**:
✓ >85% overall success rate  
✓ Correct request routing (no misroutes)  
✓ Cache hit rate >90% for cache-eligible requests  
✓ Batch jobs complete successfully  

---

### Scenario 5: Failure Recovery (Simulate API failures)

**Purpose**: Test graceful degradation and recovery  
**Workload**:
- 0-20s: Normal (0.01% failure)
- 20-30s: High failure (50% failure, simulating API timeout)
- 30-40s: Partial recovery (20% failure)
- 40-60s: Back to normal
- 60-70s: Database lag (5% failure, 2s latency)
- 70s+: Back to normal

**Duration**: 2 minutes  
**Rate**: 500 req/sec baseline  
**Workers**: 100

**Expected Behavior**:
- During API timeout: fail gracefully
- Retry logic: exponential backoff
- Recovery: gradual traffic increase
- No cascading failures

**Command**:
```bash
./load-test -scenario=recovery
```

**Success Criteria**:
✓ Recovers to 95%+ success within 10s of failure end  
✓ No cascading failures or queue overflow  
✓ Error messages logged appropriately  
✓ Retry logic doesn't amplify failures  

---

## Running Load Tests

### Prerequisites

```bash
# 1. Build the load test binary
go build -o load-test ./cmd/load-test

# 2. Start the service in another terminal
./llm-sentinel service --port 9000

# 3. Verify service is running
curl http://localhost:9000/health
```

### Run Individual Scenarios

```bash
# List all scenarios
./load-test -list-scenarios

# Run constant load (1 hour, 1000 req/sec)
./load-test -scenario=constant

# Run burst load (5000 req/sec spike)
./load-test -scenario=burst

# Run connection churn (rapid connect/disconnect)
./load-test -scenario=churn

# Run mixed workload (60/30/10 split)
./load-test -scenario=mixed

# Run failure recovery simulation
./load-test -scenario=recovery
```

### Run Custom Configuration

```bash
# Custom parameters (not using a pre-defined scenario)
./load-test \
  -duration=10m \
  -rate=1500 \
  -workers=150 \
  -ramp-up=2m \
  -ramp-down=1m \
  -report=30s
```

### Run All Scenarios (Full Test Suite)

```bash
#!/bin/bash
# run-all-load-tests.sh

echo "Running Phase 2 Load Tests..."
echo ""

scenarios=("constant" "burst" "churn" "mixed" "recovery")
for scenario in "${scenarios[@]}"; do
    echo "Starting scenario: $scenario"
    ./load-test -scenario=$scenario
    echo ""
    sleep 30  # Cool down between scenarios
done

echo "All load tests completed!"
```

---

## Metrics Collection

### Real-time Metrics (During Test)

The load test prints interim reports every 10-30 seconds:

```
[   30.0s] Requests: 30000 | Rate: 1000.0 req/s | Latency: min=85ms avg=142ms max=512ms | Success: 29997 | Errors: 3
[   60.0s] Requests: 60000 | Rate: 1000.0 req/s | Latency: min=82ms avg=145ms max=623ms | Success: 59993 | Errors: 7
[   90.0s] Requests: 90000 | Rate: 999.9 req/s | Latency: min=80ms avg=148ms max=701ms | Success: 89984 | Errors: 16
```

### Final Report (End of Test)

```
================================================================================
LOAD TEST RESULTS
================================================================================

Test Duration: 1h0m15s
Total Requests: 3606182
Successful: 3601984 (99.88%)
Failed: 4198
Requests/sec: 1000.15

Latency Metrics:
  Min: 78ms
  Avg: 145ms
  Max: 2104ms
  P50: 122ms
  P95: 267ms
  P99: 481ms
  P99.9: 892ms

Target Validation:
  ✓ P99 Latency Target (<500ms): PASS (actual: 481ms)
  ✓ Throughput Target (1000 req/s): PASS (actual: 1000.15 req/s)
  ✓ Success Rate Target (>99.5%): PASS (actual: 99.88%)

================================================================================
```

### Memory Profiling

Enable memory profiling during load tests:

```bash
# Run with memory profiling enabled
GOMAXPROCS=4 ./load-test -scenario=constant -cpuprofile=cpu.prof -memprofile=mem.prof

# Analyze results
go tool pprof -http=:6060 cpu.prof
go tool pprof -http=:6061 mem.prof
```

### Goroutine Leak Detection

Check for goroutine leaks before and after test:

```bash
# Start in one terminal
./llm-sentinel service --port 9000

# In another terminal, get baseline
curl http://localhost:9000/metrics | grep goroutines

# Run load test
./load-test -scenario=churn

# Check again (should be similar to baseline)
curl http://localhost:9000/metrics | grep goroutines
```

---

## Expected Baselines (From Existing Tests)

### Internal Benchmarks (Not Real API)

From `internal/test/load_test.go`:

```
TestLoadStability:
  - P99: 180ms (target: <300ms for mock)
  - Success rate: 99%+
  - Throughput: Variable (mock routing only)

TestMemoryStability:
  - Heap growth: <20% over 30s at 2000 req/sec
  - GC cycles: Normal

TestGoroutineLeakDetection:
  - Goroutine growth: <5 after load test
  - Connection cleanup: Proper
```

---

## CI/CD Integration

### GitHub Actions Workflow

Phase 2 tests run on:
- Main branch pushes
- Nightly schedule (2 AM UTC)
- Manual trigger (via workflow_dispatch)

**Workflow** (`.github/workflows/load-tests.yml`):

```yaml
name: Load Tests

on:
  push:
    branches: [main]
  schedule:
    - cron: '0 2 * * *'  # Nightly
  workflow_dispatch

jobs:
  load-test:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      
      - name: Build Binary
        run: go build -o load-test ./cmd/load-test
      
      - name: Build Service
        run: go build -o llm-sentinel ./cmd/llm-sentinel
      
      - name: Start Service
        run: ./llm-sentinel service --port 9000 &
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
      
      - name: Run Constant Load Test
        run: timeout 3700 ./load-test -scenario=constant 2>&1 | tee constant.log
      
      - name: Run Burst Load Test
        run: ./load-test -scenario=burst 2>&1 | tee burst.log
      
      - name: Run Mixed Workload Test
        run: ./load-test -scenario=mixed 2>&1 | tee mixed.log
      
      - name: Publish Results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: load-test-results
          path: |
            constant.log
            burst.log
            mixed.log
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check port is free
lsof -i :9000

# If in use, kill the process
kill -9 $(lsof -t -i :9000)

# Try again
./llm-sentinel service --port 9000
```

### Load Test Hangs

```bash
# Check service is responsive
curl http://localhost:9000/health

# If no response, service crashed:
# 1. Check logs for errors
# 2. Reduce load (lower -rate or -workers)
# 3. Run shorter test first (-duration=1m)
```

### High Error Rate During Load

**Possible causes**:
- Service out of memory (check with `top`)
- Connection pool exhausted (check net stats)
- Database locked (check WAL mode enabled)
- API rate limit hit (check API quota)

**Solutions**:
- Increase worker concurrency
- Enable connection pooling
- Reduce batch job sizes
- Increase database cache size

### Memory Doesn't Stabilize

```bash
# Force GC between test runs
export GOGC=25  # More aggressive GC

# Or monitor GC directly
GODEBUG=gctrace=1 ./llm-sentinel service --port 9000
```

---

## Success Metrics for Phase 2

✅ **Constant Load** (1000 req/sec, 1 hour):
- P99 latency <500ms throughout
- Memory <200MB after ramp-up
- No degradation over time
- Error rate <0.1%

✅ **Burst Load** (5000 req/sec spike):
- Handles 5x spike without collapse
- Error rate during spike <5%
- Recovers within 10 seconds

✅ **Connection Churn**:
- No goroutine leaks
- Connection pool size stable
- Resource cleanup complete

✅ **Mixed Workload**:
- Cache hit rate >90%
- Correct routing (no misroutes)
- Batch jobs complete

✅ **Failure Recovery**:
- Graceful degradation during failures
- Recovery within 30 seconds
- No cascading failures

---

## Next Steps

1. Build load test binary: `go build -o load-test ./cmd/load-test`
2. Start service: `./llm-sentinel service`
3. Run scenario: `./load-test -scenario=constant`
4. Monitor metrics in real-time
5. After completion, review final report against SLOs
6. If any SLO failed, diagnose and fix root cause
7. Re-run the failed scenario to validate fix
8. Proceed to Phase 3 (Security Audit) when all scenarios PASS

---

## Related Documentation

- [Production Roadmap](../DEVELOPMENT_ROADMAP.md) — Full 7-phase plan
- [Phase 1: Integration Testing](./PHASE_1_INTEGRATION_TESTING.md) — Real API validation
- [Phase 3: Security Audit](./PHASE_3_SECURITY_AUDIT.md) — Security review procedures
- [Performance Tuning](./PERFORMANCE_TUNING.md) — Tips for optimizing under load
