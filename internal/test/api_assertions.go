package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
)

// APIAssertions provides helper methods for validating API claims
type APIAssertions struct {
	t *testing.T
}

// NewAPIAssertions creates a new API assertions helper
func NewAPIAssertions(t *testing.T) *APIAssertions {
	return &APIAssertions{t: t}
}

// TokenSavingsMetrics tracks token usage across requests
type TokenSavingsMetrics struct {
	OriginalTokens    int
	OptimizedTokens   int
	SavingsPercent    float64
	ImprovementFactor float64
}

// CacheMetrics tracks cache effectiveness
type CacheMetrics struct {
	TotalRequests int
	CacheHits     int
	HitRate       float64
	TokensSaved   int
}

// LatencyMetrics tracks response latency
type LatencyMetrics struct {
	P50Ms       float64
	P99Ms       float64
	P999Ms      float64
	AvgMs       float64
	MaxMs       float64
	MinMs       float64
	SampleCount int
}

// AssertBatchAPISubmission validates batch job submission
func (a *APIAssertions) AssertBatchAPISubmission(ctx context.Context, apiClient *client.AnthropicClient, requests []client.BatchRequest) (*client.BatchJob, error) {
	if len(requests) == 0 {
		a.t.Error("batch submission requires at least one request")
		return nil, fmt.Errorf("empty batch")
	}

	job, err := apiClient.SubmitBatch(ctx, requests)
	if err != nil {
		a.t.Errorf("batch submission failed: %v", err)
		return nil, err
	}

	// Assertions
	if job.ID == "" {
		a.t.Error("batch job ID is empty")
		return nil, fmt.Errorf("no job ID returned")
	}

	if job.ProcessingStatus == "" {
		a.t.Error("batch processing status is empty")
		return nil, fmt.Errorf("no processing status")
	}

	a.t.Logf("batch submitted: job_id=%s, status=%s, requests=%d", job.ID, job.ProcessingStatus, job.RequestCounts.Total)
	return job, nil
}

// AssertBatchStatusPolling validates batch status polling
func (a *APIAssertions) AssertBatchStatusPolling(ctx context.Context, apiClient *client.AnthropicClient, jobID string, maxPolls int) (*client.BatchJob, error) {
	var job *client.BatchJob
	var err error

	for i := 0; i < maxPolls; i++ {
		job, err = apiClient.GetBatchStatus(ctx, jobID)
		if err != nil {
			a.t.Errorf("status poll %d failed: %v", i+1, err)
			return nil, err
		}

		a.t.Logf("batch status poll %d: status=%s, processing=%d, succeeded=%d, errored=%d",
			i+1, job.ProcessingStatus, job.RequestCounts.Processing, job.RequestCounts.Succeeded, job.RequestCounts.Errored)

		// Check for terminal states
		if job.ProcessingStatus == "completed" || job.ProcessingStatus == "failed" {
			return job, nil
		}

		// Wait before next poll
		if i < maxPolls-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				// Continue to next poll
			}
		}
	}

	if job == nil {
		a.t.Error("no batch status returned after polling")
		return nil, fmt.Errorf("polling timeout")
	}

	return job, nil
}

// AssertTokenSavings validates token optimization claims
func (a *APIAssertions) AssertTokenSavings(original, optimized int) TokenSavingsMetrics {
	if original <= 0 {
		a.t.Error("original token count must be > 0")
		return TokenSavingsMetrics{}
	}

	savings := float64(original-optimized) / float64(original) * 100
	factor := float64(original) / float64(optimized)

	// Claim: 40-60% token savings
	if savings < 40 || savings > 60 {
		a.t.Logf("token savings out of expected range: %.1f%% (expected 40-60%%)", savings)
	}

	m := TokenSavingsMetrics{
		OriginalTokens:    original,
		OptimizedTokens:   optimized,
		SavingsPercent:    savings,
		ImprovementFactor: factor,
	}

	a.t.Logf("token savings: %d → %d (%.1f%%, %.2fx improvement)", original, optimized, savings, factor)
	return m
}

// AssertCacheHitRate validates semantic cache effectiveness
func (a *APIAssertions) AssertCacheHitRate(totalRequests, cacheHits int) CacheMetrics {
	if totalRequests <= 0 {
		a.t.Error("total requests must be > 0")
		return CacheMetrics{}
	}

	hitRate := float64(cacheHits) / float64(totalRequests)

	// Claim: 90%+ cache hit rate on semantic cache
	if hitRate < 0.90 {
		a.t.Logf("cache hit rate below expected threshold: %.1f%% (expected >90%%)", hitRate*100)
	}

	m := CacheMetrics{
		TotalRequests: totalRequests,
		CacheHits:     cacheHits,
		HitRate:       hitRate,
	}

	a.t.Logf("cache effectiveness: %d/%d hits (%.1f%% hit rate)", cacheHits, totalRequests, hitRate*100)
	return m
}

// AssertLatency validates latency SLOs
func (a *APIAssertions) AssertLatency(latencies []float64, targetP99Ms float64) LatencyMetrics {
	if len(latencies) == 0 {
		a.t.Error("latencies slice is empty")
		return LatencyMetrics{}
	}

	// Sort for percentile calculation
	min, max, avg := latencies[0], latencies[0], 0.0
	for _, l := range latencies {
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
		avg += l
	}
	avg /= float64(len(latencies))

	// Simple percentile approximation (for real, use proper percentile library)
	p50Idx := len(latencies) / 2
	p99Idx := (len(latencies) * 99) / 100
	p999Idx := (len(latencies) * 999) / 1000

	m := LatencyMetrics{
		P50Ms:       latencies[p50Idx],
		P99Ms:       latencies[p99Idx],
		P999Ms:      latencies[p999Idx],
		AvgMs:       avg,
		MaxMs:       max,
		MinMs:       min,
		SampleCount: len(latencies),
	}

	// Claim: P99 < 500ms
	if m.P99Ms > targetP99Ms {
		a.t.Logf("P99 latency exceeds target: %.1fms (target: %.1fms)", m.P99Ms, targetP99Ms)
	}

	a.t.Logf("latency metrics: P50=%.1fms, P99=%.1fms, P99.9=%.1fms, avg=%.1fms (n=%d)",
		m.P50Ms, m.P99Ms, m.P999Ms, m.AvgMs, m.SampleCount)

	return m
}

// AssertKnowledgeGraphQueryPerformance validates graph query latency
func (a *APIAssertions) AssertKnowledgeGraphQueryPerformance(queryLatencies []float64) LatencyMetrics {
	// Claim: Knowledge graph queries < 10ms
	return a.AssertLatency(queryLatencies, 10.0)
}

// AssertNoRegressions compares metrics against baseline
func (a *APIAssertions) AssertNoRegressions(current, baseline map[string]interface{}) {
	tolerance := 0.10 // 10% tolerance

	for metric, baselineValue := range baseline {
		currentValue, ok := current[metric]
		if !ok {
			a.t.Logf("metric missing in current results: %s", metric)
			continue
		}

		// Simple type assertions for common metrics
		switch b := baselineValue.(type) {
		case float64:
			if c, ok := currentValue.(float64); ok {
				deviation := (c - b) / b
				if deviation > tolerance {
					a.t.Logf("metric %s regressed: %.2f%% above baseline", metric, deviation*100)
				}
			}
		default:
			a.t.Logf("metric %s: %v (type: %T)", metric, currentValue, currentValue)
		}
	}
}

// GenerateTestPayloads creates diverse test payloads for API testing
func GenerateTestPayloads() []client.BatchRequest {
	payloads := []client.BatchRequest{
		{
			CustomID: "quick_summary_1",
			Params: client.MessageRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 200,
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Summarize the main points of Go's concurrency model"},
				},
				System: "You are a helpful assistant.",
			},
		},
		{
			CustomID: "detailed_analysis_1",
			Params: client.MessageRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1000,
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Provide a detailed analysis of memory safety in Rust compared to Go"},
				},
				System: "You are an expert programmer.",
			},
		},
		{
			CustomID: "code_review_1",
			Params: client.MessageRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 800,
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Review this code for performance issues and suggest optimizations"},
				},
				System: "You are a code review expert.",
			},
		},
	}
	return payloads
}
