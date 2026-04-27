package batch

import (
	"testing"
	"time"
)

func TestNewBatchQueue(t *testing.T) {
	q := NewBatchQueue()
	if q == nil {
		t.Error("expected non-nil queue")
	}
	if q.Size() != 0 {
		t.Errorf("expected empty queue, got size %d", q.Size())
	}
	if q.maxQueueSize != 100 {
		t.Errorf("expected maxQueueSize 100, got %d", q.maxQueueSize)
	}
}

func TestEnqueueDequeue(t *testing.T) {
	q := NewBatchQueue()
	req := &BatchRequest{
		ID:           "req_1",
		PromptLength: 1000,
		Model:        "haiku",
	}

	err := q.Enqueue(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}

	dequeued := q.Dequeue()
	if dequeued == nil || dequeued.ID != "req_1" {
		t.Error("expected to dequeue req_1")
	}

	if q.Size() != 0 {
		t.Errorf("expected empty queue after dequeue, got size %d", q.Size())
	}
}

func TestQueueFull(t *testing.T) {
	q := NewBatchQueue()
	q.SetMaxQueueSize(2)

	req1 := &BatchRequest{ID: "req_1", Model: "haiku"}
	req2 := &BatchRequest{ID: "req_2", Model: "haiku"}
	req3 := &BatchRequest{ID: "req_3", Model: "haiku"}

	q.Enqueue(req1)
	q.Enqueue(req2)

	if !q.IsFull() {
		t.Error("expected queue to be full")
	}

	err := q.Enqueue(req3)
	if err == nil {
		t.Error("expected error when enqueueing to full queue")
	}
}

func TestIsReady(t *testing.T) {
	q := NewBatchQueue()
	q.SetMinBatchSize(2)
	q.SetIdleTimeout(100 * time.Millisecond)

	// Empty queue should not be ready
	if q.IsReady() {
		t.Error("empty queue should not be ready")
	}

	// Add one request (less than minBatchSize)
	q.Enqueue(&BatchRequest{ID: "req_1", Model: "haiku"})
	if q.IsReady() {
		t.Error("queue with 1 request (min 2) should not be ready")
	}

	// Add second request (reaches minBatchSize)
	q.Enqueue(&BatchRequest{ID: "req_2", Model: "haiku"})
	if !q.IsReady() {
		t.Error("queue with 2 requests (min 2) should be ready")
	}
}

func TestQueueFlush(t *testing.T) {
	q := NewBatchQueue()
	q.Enqueue(&BatchRequest{ID: "req_1", Priority: 1})
	q.Enqueue(&BatchRequest{ID: "req_2", Priority: 2})
	q.Enqueue(&BatchRequest{ID: "req_3", Priority: 1})

	requests := q.Flush()
	if len(requests) != 3 {
		t.Errorf("expected 3 requests after flush, got %d", len(requests))
	}

	// Verify queue is now empty
	if q.Size() != 0 {
		t.Errorf("expected empty queue after flush, got size %d", q.Size())
	}

	// Verify priority sorting (higher priority first)
	if requests[0].Priority != 2 {
		t.Errorf("expected first request to have priority 2, got %d", requests[0].Priority)
	}
}

func TestQueuePriority(t *testing.T) {
	q := NewBatchQueue()
	q.Enqueue(&BatchRequest{ID: "low", Priority: 0})
	q.Enqueue(&BatchRequest{ID: "high", Priority: 2})
	q.Enqueue(&BatchRequest{ID: "medium", Priority: 1})

	requests := q.Flush()

	// Check order: high (2), medium (1), low (0)
	if requests[0].ID != "high" {
		t.Errorf("expected first request ID 'high', got %q", requests[0].ID)
	}
	if requests[1].ID != "medium" {
		t.Errorf("expected second request ID 'medium', got %q", requests[1].ID)
	}
	if requests[2].ID != "low" {
		t.Errorf("expected third request ID 'low', got %q", requests[2].ID)
	}
}

func TestQueuePeek(t *testing.T) {
	q := NewBatchQueue()
	req := &BatchRequest{ID: "req_1"}

	q.Enqueue(req)
	peeked := q.Peek()

	if peeked == nil || peeked.ID != "req_1" {
		t.Error("expected to peek req_1")
	}

	// Verify queue still has the request (peek doesn't remove)
	if q.Size() != 1 {
		t.Error("peek should not remove request from queue")
	}
}

func TestEstimatedWaitTime(t *testing.T) {
	q := NewBatchQueue()

	// Empty queue
	if q.EstimatedWaitTime() != 0 {
		t.Error("empty queue should have 0 wait time")
	}

	// Add requests
	for i := 0; i < 3; i++ {
		q.Enqueue(&BatchRequest{ID: "req_1"})
	}

	waitTime := q.EstimatedWaitTime()
	if waitTime == 0 {
		t.Error("queue with 3 requests should have non-zero wait time")
	}

	// Wait time should be roughly 3 * 10 seconds = 30 seconds
	expected := 30 * time.Second
	if waitTime != expected {
		t.Errorf("expected wait time ~%v, got %v", expected, waitTime)
	}
}

func TestQueueStats(t *testing.T) {
	q := NewBatchQueue()
	q.Enqueue(&BatchRequest{ID: "req_1", CreatedAt: time.Now()})
	q.Enqueue(&BatchRequest{ID: "req_2", CreatedAt: time.Now().Add(-5 * time.Second)})

	stats := q.QueueStats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Errorf("expected max size 100, got %d", stats.MaxSize)
	}

	// Oldest request should be ~5 seconds old
	if stats.OldestRequestAge < 4*time.Second || stats.OldestRequestAge > 6*time.Second {
		t.Errorf("expected oldest request age ~5s, got %v", stats.OldestRequestAge)
	}
}

func TestQueueClear(t *testing.T) {
	q := NewBatchQueue()
	q.Enqueue(&BatchRequest{ID: "req_1"})
	q.Enqueue(&BatchRequest{ID: "req_2"})

	if q.Size() != 2 {
		t.Errorf("expected size 2 before clear, got %d", q.Size())
	}

	q.Clear()

	if q.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", q.Size())
	}
}

func TestQueueIdleTimeout(t *testing.T) {
	q := NewBatchQueue()
	q.SetMinBatchSize(5) // High threshold
	q.SetIdleTimeout(50 * time.Millisecond)

	q.Enqueue(&BatchRequest{ID: "req_1"})

	// Initially not ready (less than minBatchSize)
	if q.IsReady() {
		t.Error("queue should not be ready yet")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should be ready now due to idle timeout
	if !q.IsReady() {
		t.Error("queue should be ready after idle timeout")
	}
}

func TestRecordSavings(t *testing.T) {
	q := NewBatchQueue()
	stats := q.QueueStats()

	if stats.EstimatedSaved != 0 {
		t.Errorf("expected 0 savings, got %f", stats.EstimatedSaved)
	}

	q.RecordSavings(10.5)
	stats = q.QueueStats()

	if stats.EstimatedSaved != 10.5 {
		t.Errorf("expected savings 10.5, got %f", stats.EstimatedSaved)
	}
}

func TestConfigurationMethods(t *testing.T) {
	q := NewBatchQueue()

	q.SetMinBatchSize(5)
	q.SetMaxQueueSize(200)
	q.SetIdleTimeout(2 * time.Minute)
	q.SetMaxBatchWaitTime(10 * time.Minute)

	stats := q.QueueStats()
	if stats.MinSize != 5 {
		t.Errorf("expected min size 5, got %d", stats.MinSize)
	}
	if stats.MaxSize != 200 {
		t.Errorf("expected max size 200, got %d", stats.MaxSize)
	}
}
