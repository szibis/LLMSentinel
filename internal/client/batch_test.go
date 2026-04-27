package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewBatchQueue(t *testing.T) {
	queue := NewBatchQueue(10, 100, 5*time.Second)
	if queue.Size() != 0 {
		t.Errorf("expected size 0, got %d", queue.Size())
	}
	if queue.minSize != 10 {
		t.Errorf("expected minSize 10, got %d", queue.minSize)
	}
	if queue.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", queue.maxSize)
	}
}

func TestBatchQueueEnqueue(t *testing.T) {
	queue := NewBatchQueue(5, 10, 5*time.Second)

	req := BatchRequest{
		CustomID: "req_1",
		Params: MessageRequest{
			Model:     "claude-3-sonnet",
			MaxTokens: 1000,
		},
	}

	success := queue.Enqueue(req)
	if !success {
		t.Error("expected enqueue to succeed")
	}
	if queue.Size() != 1 {
		t.Errorf("expected size 1, got %d", queue.Size())
	}

	// Fill to max
	for i := 1; i < 10; i++ {
		req.CustomID = "req_" + string(rune(i+1))
		queue.Enqueue(req)
	}

	// Try to enqueue beyond max
	success = queue.Enqueue(req)
	if success {
		t.Error("expected enqueue to fail when queue is full")
	}
	if queue.Size() != 10 {
		t.Errorf("expected size 10, got %d", queue.Size())
	}
}

func TestBatchQueueIsReady(t *testing.T) {
	// Test ready when max size reached
	queue := NewBatchQueue(5, 10, 5*time.Second)
	if queue.IsReady() {
		t.Error("queue should not be ready when empty")
	}

	req := BatchRequest{CustomID: "req_1"}
	for i := 0; i < 10; i++ {
		queue.Enqueue(req)
	}

	if !queue.IsReady() {
		t.Error("queue should be ready when max size reached")
	}

	// Test ready when min size + timeout reached
	queue = NewBatchQueue(5, 100, 100*time.Millisecond)
	for i := 0; i < 5; i++ {
		queue.Enqueue(req)
	}

	if queue.IsReady() {
		t.Error("queue should not be ready immediately")
	}

	time.Sleep(150 * time.Millisecond)
	if !queue.IsReady() {
		t.Error("queue should be ready after timeout with min size")
	}
}

func TestBatchQueueClear(t *testing.T) {
	queue := NewBatchQueue(5, 100, 5*time.Second)
	req := BatchRequest{CustomID: "req_1"}
	queue.Enqueue(req)
	queue.Enqueue(req)

	if queue.Size() != 2 {
		t.Errorf("expected size 2, got %d", queue.Size())
	}

	queue.Clear()
	if queue.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", queue.Size())
	}
}

func TestBatchQueueRequests(t *testing.T) {
	queue := NewBatchQueue(5, 100, 5*time.Second)
	req1 := BatchRequest{CustomID: "req_1"}
	req2 := BatchRequest{CustomID: "req_2"}

	queue.Enqueue(req1)
	queue.Enqueue(req2)

	requests := queue.Requests()
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].CustomID != "req_1" {
		t.Errorf("expected first request to be 'req_1', got %s", requests[0].CustomID)
	}
}

func TestBatchPoller(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		job := BatchJob{
			ID:   "batch_123",
			Type: "batch",
		}

		if callCount == 1 {
			job.ProcessingStatus = "queued"
		} else if callCount == 2 {
			job.ProcessingStatus = "in_progress"
		} else {
			job.ProcessingStatus = "succeeded"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	poller := NewBatchPoller(client, 50*time.Millisecond, 5*time.Second)
	job, err := poller.Poll(context.Background(), "batch_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.ProcessingStatus != "succeeded" {
		t.Errorf("expected status 'succeeded', got %s", job.ProcessingStatus)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestBatchPollerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job := BatchJob{
			ID:               "batch_123",
			Type:             "batch",
			ProcessingStatus: "in_progress",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	poller := NewBatchPoller(client, 50*time.Millisecond, 100*time.Millisecond)
	_, err := poller.Poll(context.Background(), "batch_123")
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestBatchCostTracker(t *testing.T) {
	calc := &CostCalculator{
		InputCostPer1KTokens:  0.001,
		OutputCostPer1KTokens: 0.002,
		BatchDiscount:         0.5,
	}

	tracker := NewBatchCostTracker(calc)
	costs := tracker.CompareCosts(1000, 500, 2)

	if costs["requests"] != 2 {
		t.Errorf("expected 2 requests, got %v", costs["requests"])
	}

	regularCost, ok := costs["regular_api_cost"].(float64)
	if !ok {
		t.Error("expected regular_api_cost to be float64")
	}

	batchCost, ok := costs["batch_api_cost"].(float64)
	if !ok {
		t.Error("expected batch_api_cost to be float64")
	}

	if batchCost >= regularCost {
		t.Errorf("expected batch cost to be lower than regular cost")
	}

	savingsPercent, ok := costs["savings_percent"].(float64)
	if !ok {
		t.Error("expected savings_percent to be float64")
	}

	if savingsPercent <= 0 {
		t.Errorf("expected positive savings percent, got %v", savingsPercent)
	}
}

func TestNewBatchResultProcessor(t *testing.T) {
	client := NewAnthropicClient("test-api-key")
	processor := NewBatchResultProcessor(client)
	if processor.client != client {
		t.Error("processor client should be set correctly")
	}
}

func TestBatchResultProcessor(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// First call: return job status with output file
		if callCount == 1 {
			job := BatchJob{
				ID:               "batch_123",
				Type:             "batch",
				ProcessingStatus: "succeeded",
				OutputFileID:     "file_123",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(job)
			return
		}

		// Second call: return file contents (JSONL)
		result1 := BatchResult{
			CustomID: "req_1",
			Result: MessageResponse{
				ID:    "msg_1",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-3-sonnet",
			},
		}
		result1.Result.Usage.InputTokens = 100
		result1.Result.Usage.OutputTokens = 50

		result2 := BatchResult{
			CustomID: "req_2",
			Result: MessageResponse{
				ID:    "msg_2",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-3-sonnet",
			},
		}
		result2.Result.Usage.InputTokens = 100
		result2.Result.Usage.OutputTokens = 50

		w.Header().Set("Content-Type", "application/x-ndjson")
		json.NewEncoder(w).Encode(result1)
		json.NewEncoder(w).Encode(result2)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	processor := NewBatchResultProcessor(client)
	results, err := processor.ProcessResults(context.Background(), "batch_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if _, ok := results["req_1"]; !ok {
		t.Error("expected req_1 in results")
	}
	if _, ok := results["req_2"]; !ok {
		t.Error("expected req_2 in results")
	}
}
