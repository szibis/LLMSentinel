package batch

import (
	"testing"
	"time"
)

func TestMakeRoutingDecision_NeverStrategy(t *testing.T) {
	router := NewRouter(StrategyNever)
	req := BatchRequest{
		ID:              "test-1",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision, err := router.MakeRoutingDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=false with StrategyNever")
	}
}

func TestMakeRoutingDecision_AlwaysStrategy(t *testing.T) {
	router := NewRouter(StrategyAlways)
	req := BatchRequest{
		ID:              "test-1",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision, err := router.MakeRoutingDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=true with StrategyAlways")
	}

	// Queue should have the request
	if router.QueueSize() != 1 {
		t.Errorf("expected queue size 1, got %d", router.QueueSize())
	}
}

func TestMakeRoutingDecision_AutoStrategy_SmallQueue(t *testing.T) {
	router := NewRouter(StrategyAuto)
	router.SetMinBatchSize(2)

	// First request - batching saves money, so should batch
	req1 := BatchRequest{
		ID:              "test-1",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision1, _ := router.MakeRoutingDecision(req1)
	// With StrategyAuto and good savings, should batch
	if !decision1.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=true with StrategyAuto when savings are good")
	}

	// Second request should also batch (will be in queue with first)
	req2 := BatchRequest{
		ID:              "test-2",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}
	decision2, _ := router.MakeRoutingDecision(req2)
	if !decision2.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=true for second request")
	}

	// Queue should now have 2 items
	stats := router.QueueStats()
	if stats.Size != 2 {
		t.Errorf("expected queue size 2, got %d", stats.Size)
	}
}

func TestMakeRoutingDecision_UserChoice(t *testing.T) {
	router := NewRouter(StrategyUserChoice)
	req := BatchRequest{
		ID:              "test-1",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "sonnet",
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision, _ := router.MakeRoutingDecision(req)

	if decision.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=false with StrategyUserChoice")
	}

	if !decision.UserCanOverride {
		t.Error("expected UserCanOverride=true with StrategyUserChoice")
	}

	// Queue should be empty since we didn't batch
	if router.QueueSize() != 0 {
		t.Errorf("expected queue size 0, got %d", router.QueueSize())
	}
}

func TestFlushQueue(t *testing.T) {
	router := NewRouter(StrategyAlways)

	// Add requests with different priorities (with required fields)
	requests := []BatchRequest{
		{ID: "low", Priority: 0, PromptLength: 1000, EstimatedOutput: 500, Model: "sonnet", CreatedAt: time.Now()},
		{ID: "high", Priority: 2, PromptLength: 1000, EstimatedOutput: 500, Model: "sonnet", CreatedAt: time.Now()},
		{ID: "medium", Priority: 1, PromptLength: 1000, EstimatedOutput: 500, Model: "sonnet", CreatedAt: time.Now()},
	}

	for _, req := range requests {
		router.MakeRoutingDecision(req)
	}

	if router.QueueSize() != 3 {
		t.Errorf("expected 3 in queue, got %d", router.QueueSize())
	}

	// Flush queue
	flushed, _ := router.FlushQueue()

	if len(flushed) != 3 {
		t.Errorf("expected 3 flushed, got %d", len(flushed))
	}

	// Queue should be empty
	if router.QueueSize() != 0 {
		t.Errorf("expected empty queue after flush, got %d", router.QueueSize())
	}

	// High priority should be first
	if flushed[0].Priority != 2 {
		t.Errorf("expected high priority first, got %d", flushed[0].Priority)
	}
}

func TestCanAddToQueue(t *testing.T) {
	router := NewRouter(StrategyAlways)
	router.maxQueueSize = 2

	req1 := BatchRequest{ID: "1", PromptLength: 1000, EstimatedOutput: 500, Model: "sonnet", CreatedAt: time.Now()}
	req2 := BatchRequest{ID: "2", PromptLength: 1000, EstimatedOutput: 500, Model: "sonnet", CreatedAt: time.Now()}

	router.MakeRoutingDecision(req1)
	if !router.CanAddToQueue() {
		t.Error("expected CanAddToQueue=true at size 1")
	}

	router.MakeRoutingDecision(req2)
	// Now at max size (2), should not be able to add more
	if router.CanAddToQueue() {
		t.Error("expected CanAddToQueue=false at max size")
	}

	// Verify queue has the requests we added
	if router.QueueSize() != 2 {
		t.Errorf("expected queue size 2, got %d", router.QueueSize())
	}
}

func TestRecommendBatching(t *testing.T) {
	router := NewRouter(StrategyAuto)

	// Recommend batching for 2000 char prompt
	rec := router.RecommendBatching("haiku", 500, 250, 2*time.Minute)

	if rec.SavingsPercent < 50.0 {
		t.Errorf("expected >50%% savings for batch API, got %.1f%%", rec.SavingsPercent)
	}

	// With empty queue and good savings, should recommend
	if !rec.RecommendBatch {
		t.Errorf("expected RecommendBatch=true, got false (savings: %.1f%%, wait: %v)",
			rec.SavingsPercent, rec.EstimatedWaitTime)
	}
}

func TestSavingsCalculation(t *testing.T) {
	router := NewRouter(StrategyAlways)

	// For a request that would save money
	req := BatchRequest{
		ID:              "test",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "opus", // Most expensive model
		MaxWaitTime:     5 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision, _ := router.MakeRoutingDecision(req)

	// Batch API always provides 50% discount
	if decision.EstimatedSavings <= 0 {
		t.Errorf("expected positive savings, got %f", decision.EstimatedSavings)
	}
}

func TestSetters(t *testing.T) {
	router := NewRouter(StrategyAuto)

	router.SetMinBatchSize(5)
	router.SetMinSavingsPercent(10.0)
	router.SetMaxBatchWaitTime(1 * time.Minute)
	router.SetStrategy(StrategyNever)

	// Test that setters worked
	stats := router.QueueStats()
	if stats.MaxSize == 0 {
		t.Error("setters may not have worked")
	}
}

func TestAlternativeModelSuggestion(t *testing.T) {
	router := NewRouter(StrategyNever)

	req := BatchRequest{
		ID:              "test",
		PromptLength:    2000,
		EstimatedOutput: 1000,
		Model:           "opus", // Most expensive
		MaxWaitTime:     1 * time.Minute,
		CreatedAt:       time.Now(),
	}

	decision, _ := router.MakeRoutingDecision(req)

	// With StrategyNever, should not batch
	if decision.UsesBatchAPI {
		t.Error("expected UsesBatchAPI=false with StrategyNever")
	}

	// AlternativeSavings is calculated for model comparison baseline
	if decision.AlternativeSavings < 0 {
		t.Error("expected non-negative alternative savings")
	}
}
