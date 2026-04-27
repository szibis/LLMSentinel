# Batch API Integration Guide

## Overview

Claude Escalate v0.7.0 integrates Anthropic's Batch API for **50% cost reduction** on non-interactive workloads. Batch requests are processed in the background (5 minutes to 24 hours) and cost 50% less than regular API calls.

**Cost Comparison**:
- Regular API: 100 × 2500 tokens = 250,000 tokens = $0.75 (Haiku)
- Batch API: 100 × 2500 × 0.5 = 125,000 tokens = $0.375
- **Savings: $0.375 (50%)**

---

## When to Use Batch API

### ✅ Good Use Cases
- **Bulk analysis**: "Analyze all 100 files in repo for security"
- **Overnight jobs**: Scheduled security scans, code review batches
- **Bulk documentation**: Generate docs for all functions at once
- **Batch processing**: Process large datasets with Claude in background
- **Report generation**: Bulk analytics that can be generated asynchronously

**Characteristics**:
- Non-urgent (can wait 5-24 hours for results)
- Multiple similar requests (batch efficiency)
- Cost-sensitive (50% savings justify delay)
- Batch size: 10-100 requests per batch (configurable)

### ❌ Poor Use Cases
- **Interactive queries**: User is waiting for response
- **Real-time code review**: User needs immediate feedback
- **Quick lookups**: Single query that's fast anyway
- **Production debugging**: Need answer now, not tomorrow

**Why not**:
- User experience: 5min-24h delay unacceptable
- No batching opportunity (single request)
- Total cost already low (<$0.01)

---

## Configuration

### Enable Batch API

```yaml
# config.yaml
batch_api:
  enabled: true
  min_batch_size: 10         # Minimum requests to batch
  max_batch_size: 100        # Maximum per batch
  auto_batch_similar: true   # Group similar queries
  timeout_minutes: 30        # Auto-flush after 30 min
```

### Auto-Detection Thresholds

Batch API automatically routes non-interactive workloads:

```yaml
detector:
  enabled: true
  confidence_threshold: 0.6  # Confidence needed to batch
  min_request_count: 5       # At least 5 req/30s = bulk
  max_response_time_ms: 5000 # Timeout: can batch if >5s acceptable
```

---

## Usage Examples

### Example 1: Bulk File Analysis

```bash
# Regular API (interactive)
$ claude-escalate --api regular "Analyze this file for security"
# Cost: ~$0.003, latency: <2s

# Batch API (non-interactive)
$ claude-escalate --api batch "Analyze all 50 files in repo for security"
# Cost: ~$0.0015 (50% savings), latency: 5-24h
# Job ID returned immediately
```

### Example 2: Scheduled Analysis

```go
// Go code using SDK
ctx := context.Background()

// Submit batch of requests
batch := &escalate.BatchRequest{
    Requests: []*escalate.Request{
        {Query: "Analyze file1.go for security"},
        {Query: "Analyze file2.go for security"},
        // ... 50 files total
    },
}

jobID, err := client.SubmitBatch(ctx, batch)
// jobID returned immediately (e.g., "batch_abc123")

// Poll for results (in background job)
for {
    status, err := client.GetBatchStatus(ctx, jobID)
    if status.Completed {
        results, err := client.GetBatchResults(ctx, jobID)
        // Process results
        break
    }
    time.Sleep(30 * time.Second) // Poll every 30s
}
```

### Example 3: Cost Comparison

```
Scenario: Analyze 100 files for security issues

Without Batch API:
  - 100 requests × 2500 tokens each = 250,000 tokens
  - Cost: $0.75 (Haiku pricing)
  - Latency: 200s (2s per request)

With Batch API:
  - 100 requests × 2500 × 0.5 = 125,000 tokens
  - Cost: $0.375 (50% discount)
  - Latency: 5-24 hours (background processing)

Combined with Semantic Cache:
  - Batch API: 50% savings
  - Similar requests cached: 98% savings
  - If 20 requests are similar (repeat queries):
    - Batch: 80 × 0.5 = 40 units
    - Cached: 20 × 0.02 = 0.4 units (98% saved)
    - Total: 40.4 units vs 100 baseline = 60% savings!
```

---

## API Reference

### SubmitBatch

Submit a batch of requests for processing:

```go
type BatchRequest struct {
    Requests []*Request
}

type Request struct {
    Query     string
    Context   string
    Model     string // Optional, default: Sonnet
    MaxTokens int    // Optional
}

jobID, err := client.SubmitBatch(ctx, batch)
// Returns job ID for polling (e.g., "batch_abc123")
// Error if batch invalid, network error, or quota exceeded
```

### GetBatchStatus

Check status of submitted batch:

```go
status, err := client.GetBatchStatus(ctx, jobID)

type BatchStatus struct {
    JobID       string
    Status      string // "queued", "processing", "completed", "failed"
    SubmittedAt time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    RequestCount int
    CompletedCount int
    FailedCount  int
}
```

### GetBatchResults

Retrieve results from completed batch:

```go
results, err := client.GetBatchResults(ctx, jobID)

type BatchResult struct {
    Requests  []*Request
    Responses []*Response
    Errors    map[int]error // Map of request index to error
}
```

### CancelBatch

Cancel a submitted batch (if not yet processing):

```go
err := client.CancelBatch(ctx, jobID)
```

---

## Monitoring

### Cost Tracking

Monitor batch vs regular API costs:

```bash
# View cost breakdown
$ claude-escalate metrics --cost
Batch API cost today: $1.23
Regular API cost today: $3.45
Batch savings: $2.22 (39%)
```

### Batch Job Tracking

```bash
# List active batch jobs
$ claude-escalate batch list
batch_abc123  processing  45/100  submitted 2h ago
batch_xyz789  completed   50/50   submitted 6h ago

# Check specific job
$ claude-escalate batch status batch_abc123
Status: processing
Progress: 45 of 100 requests completed
Estimated completion: 2h 15m
Cost so far: $0.56 (50% discount applied)
```

---

## Best Practices

### 1. Batch Similar Requests

Good:
```
[Request 1] Analyze file1.go
[Request 2] Analyze file2.go
[Request 3] Analyze file3.go
```

Bad (different models/contexts):
```
[Request 1] Analyze file1.go (context: security)
[Request 2] Summarize file2.go (context: documentation)
[Request 3] Find bugs in file3.go (context: code review)
```

Grouping similar requests in same batch = better cost optimization.

### 2. Use Appropriate Batch Size

- **Too small** (5-10): Overhead of batch API not worth it, use regular API
- **Optimal** (10-100): Good balance of cost savings and processing time
- **Too large** (>100): May hit quotas, breaks up into multiple batches

Default: Auto-batching at 10-100 requests per batch.

### 3. Plan for Latency

Batch API latency: 5 minutes to 24 hours.
- Don't use for real-time interactions
- Schedule batch jobs for off-peak hours
- Plan results delivery asynchronously (webhooks, polling, email)

### 4. Error Handling

```go
// Some requests in batch may fail
results, err := client.GetBatchResults(ctx, jobID)
for i, respErr := range results.Errors {
    if respErr != nil {
        log.Warnf("Request %d failed: %v", i, respErr)
        // Retry that request with regular API
        req := results.Requests[i]
        resp, err := client.CreateMessage(ctx, req)
        // Handle response
    }
}
```

---

## Troubleshooting

### "Batch quota exceeded"
- Too many concurrent batch requests
- Wait for some batches to complete before submitting more
- Check quota with: `claude-escalate batch quota`

### "Batch timed out after 24h"
- Anthropic processes batches within 24 hours
- After 24h, batch is abandoned and no results returned
- Resubmit batch if needed

### "Batch not found"
- Job ID is invalid or expired (older than 29 days)
- Check job ID format: should be `batch_...`
- Use: `claude-escalate batch list` to see active jobs

### "Result rate limited"
- Too many requests to fetch results
- Wait 30-60 seconds before polling again
- Use: Exponential backoff (start at 30s, increase by 1.5x each retry)

---

## Migration from v0.6.0

Batch API is **fully backward compatible**. No code changes required.

**To enable**:
1. Add `batch_api.enabled: true` to config.yaml
2. Restart gateway
3. Batch API auto-detects non-interactive workloads

**To use explicitly**:
```bash
# Before
$ claude-escalate "analyze all files"  # Uses regular API

# After
$ claude-escalate --batch "analyze all files"  # Uses Batch API
# or
$ claude-escalate batch submit "analyze all files"  # Explicit batch
```

---

## See Also

- [CHANGELOG.md](../CHANGELOG.md) — v0.7.0 release notes
- [API.md](API.md) — Full gateway API reference
- [ARCHITECTURE.md](ARCHITECTURE.md) — How optimization layers work together
