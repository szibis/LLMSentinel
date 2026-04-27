package batch

import (
	"testing"
	"time"
)

func TestRouterWithDetectorEnabled(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(true)
	router.SetDetectorConfidence(0.6)

	// Create a non-interactive batch request
	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    5000,
		EstimatedOutput: 2000,
		Model:           "sonnet",
		Priority:        1,
		MaxWaitTime:     10 * time.Minute,
		CreatedAt:       time.Now(),
		UserContext: map[string]interface{}{
			"intent": "batch_analysis",
			"query":  "analyze all files in repository",
		},
	}

	decision, err := router.MakeRoutingDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.UsesBatchAPI {
		t.Error("batch analysis request should use batch API")
	}
	if decision.EstimatedSavings <= 0 {
		t.Error("expected positive savings estimate")
	}
	if decision.ROIScore == 0 {
		t.Error("expected positive ROI score")
	}
}

func TestRouterWithDetectorDisabled(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(false)

	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    1000,
		EstimatedOutput: 500,
		Model:           "sonnet",
		Priority:        1,
		MaxWaitTime:     1 * time.Second, // Very short wait time
		CreatedAt:       time.Now(),
		UserContext: map[string]interface{}{
			"intent": "batch_analysis",
		},
	}

	decision, err := router.MakeRoutingDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without detector and without high savings, should not batch
	if decision.UsesBatchAPI && decision.EstimatedSavings < 0.001 {
		t.Error("low-savings request should not batch")
	}
}

func TestRouterQueueing(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(true)

	req1 := BatchRequest{
		ID:              "req_1",
		PromptLength:    5000,
		EstimatedOutput: 2000,
		Model:           "sonnet",
		MaxWaitTime:     5 * time.Minute,
		UserContext: map[string]interface{}{
			"intent": "batch_analysis",
		},
	}

	req2 := BatchRequest{
		ID:              "req_2",
		PromptLength:    5000,
		EstimatedOutput: 2000,
		Model:           "sonnet",
		MaxWaitTime:     5 * time.Minute,
		UserContext: map[string]interface{}{
			"intent": "bulk_processing",
		},
	}

	// First request
	decision1, _ := router.MakeRoutingDecision(req1)
	if !decision1.UsesBatchAPI {
		t.Error("batch request should use batch API")
	}

	// Check queue size
	if router.QueueSize() != 1 {
		t.Errorf("expected queue size 1, got %d", router.QueueSize())
	}

	// Second request
	decision2, _ := router.MakeRoutingDecision(req2)
	if !decision2.UsesBatchAPI {
		t.Error("bulk processing request should use batch API")
	}

	if router.QueueSize() != 2 {
		t.Errorf("expected queue size 2, got %d", router.QueueSize())
	}
}

func TestRouterDetectorConfidence(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(true)
	router.SetDetectorConfidence(0.9) // Very high threshold

	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    1000,
		EstimatedOutput: 500,
		Model:           "sonnet",
		MaxWaitTime:     5 * time.Minute,
		UserContext: map[string]interface{}{
			"intent": "analysis", // Neutral intent, low confidence
		},
	}

	decision, _ := router.MakeRoutingDecision(req)

	// With high confidence threshold and neutral intent, should not batch based on detection
	// (Cost savings alone might not be sufficient)
	if decision.UsesBatchAPI && decision.EstimatedSavings < 0.001 {
		t.Error("low-confidence detection with low savings should not batch")
	}
}

func TestRouterAlwaysStrategy(t *testing.T) {
	router := NewRouter(StrategyAlways)

	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    100,
		EstimatedOutput: 50,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Second,
	}

	decision, _ := router.MakeRoutingDecision(req)
	if !decision.UsesBatchAPI {
		t.Error("StrategyAlways should always batch")
	}
}

func TestRouterNeverStrategy(t *testing.T) {
	router := NewRouter(StrategyNever)

	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    5000,
		EstimatedOutput: 2000,
		Model:           "sonnet",
		MaxWaitTime:     10 * time.Minute,
		UserContext: map[string]interface{}{
			"intent": "batch_analysis",
		},
	}

	decision, _ := router.MakeRoutingDecision(req)
	if decision.UsesBatchAPI {
		t.Error("StrategyNever should never batch")
	}
}

func TestRouterUserChoiceStrategy(t *testing.T) {
	router := NewRouter(StrategyUserChoice)

	req := BatchRequest{
		ID:              "req_1",
		PromptLength:    5000,
		EstimatedOutput: 2000,
		Model:           "sonnet",
		MaxWaitTime:     5 * time.Minute,
	}

	decision, _ := router.MakeRoutingDecision(req)
	if decision.UsesBatchAPI {
		t.Error("StrategyUserChoice should default to direct API")
	}
	if !decision.UserCanOverride {
		t.Error("StrategyUserChoice should allow user override")
	}
}

func TestRouterQueueStats(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(true)

	// Add multiple requests
	for i := 0; i < 5; i++ {
		req := BatchRequest{
			ID:              "req_" + string(rune('1'+i)),
			PromptLength:    5000,
			EstimatedOutput: 2000,
			Model:           "sonnet",
			MaxWaitTime:     5 * time.Minute,
			UserContext: map[string]interface{}{
				"intent": "batch_analysis",
			},
		}
		router.MakeRoutingDecision(req)
	}

	stats := router.QueueStats()
	if stats.Size != 5 {
		t.Errorf("expected queue size 5, got %d", stats.Size)
	}
	if stats.MaxSize <= stats.Size {
		t.Errorf("max size should be > current size")
	}
}

func TestRouterFlushQueue(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.EnableDetector(true)

	// Add requests
	for i := 0; i < 3; i++ {
		req := BatchRequest{
			ID:              "req_" + string(rune('1'+i)),
			PromptLength:    5000,
			EstimatedOutput: 2000,
			Model:           "sonnet",
			MaxWaitTime:     5 * time.Minute,
			UserContext: map[string]interface{}{
				"intent": "batch_analysis",
			},
		}
		router.MakeRoutingDecision(req)
	}

	if router.QueueSize() != 3 {
		t.Errorf("expected queue size 3, got %d", router.QueueSize())
	}

	// Flush queue
	requests, err := router.FlushQueue()
	if err != nil {
		t.Fatalf("unexpected error flushing queue: %v", err)
	}
	if len(requests) != 3 {
		t.Errorf("expected 3 requests in flush, got %d", len(requests))
	}

	if router.QueueSize() != 0 {
		t.Errorf("expected empty queue after flush, got size %d", router.QueueSize())
	}
}
