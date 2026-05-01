package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
)

// MockAnthropicClient simulates Anthropic API responses for testing
type MockAnthropicClient struct {
	mu                sync.Mutex
	batches           map[string]*client.BatchJob
	batchResults      map[string][]client.BatchResult
	nextBatchID       int
	messageDelay      time.Duration
	batchDelay        time.Duration
	failureRate       float64
	responseGenerator ResponseGenerator
}

// ResponseGenerator generates deterministic responses for testing
type ResponseGenerator interface {
	GenerateMessage(req *client.MessageRequest) *client.MessageResponse
	GenerateTokens(text string) (int, int) // input, output tokens
}

// DefaultResponseGenerator provides standard test responses
type DefaultResponseGenerator struct{}

func (d *DefaultResponseGenerator) GenerateMessage(req *client.MessageRequest) *client.MessageResponse {
	inputTokens := EstimateTokens(req.System) + EstimateTokens(req.Messages[0].Content)
	outputText := fmt.Sprintf("Mock response for model %s with %d input tokens", req.Model, inputTokens)
	outputTokens := EstimateTokens(outputText)

	return &client.MessageResponse{
		ID:   fmt.Sprintf("msg_mock_%d", time.Now().UnixNano()),
		Type: "message",
		Role: "assistant",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: outputText},
		},
		Model:      req.Model,
		StopReason: "end_turn",
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}
}

func (d *DefaultResponseGenerator) GenerateTokens(text string) (int, int) {
	// Simple mock: split input/output 50/50
	total := EstimateTokens(text)
	return total / 2, total / 2
}

// EstimateTokens provides rough token estimates (4 chars per token)
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) / 4) + 1
}

// NewMockAnthropicClient creates a mock client for testing
func NewMockAnthropicClient() *MockAnthropicClient {
	return &MockAnthropicClient{
		batches:           make(map[string]*client.BatchJob),
		batchResults:      make(map[string][]client.BatchResult),
		nextBatchID:       1000,
		messageDelay:      10 * time.Millisecond,
		batchDelay:        50 * time.Millisecond,
		failureRate:       0.0,
		responseGenerator: &DefaultResponseGenerator{},
	}
}

// SetMessageDelay sets the simulated delay for message responses
func (m *MockAnthropicClient) SetMessageDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messageDelay = d
}

// SetBatchDelay sets the simulated delay for batch responses
func (m *MockAnthropicClient) SetBatchDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchDelay = d
}

// SetFailureRate sets the probability of simulated failures (0.0 to 1.0)
func (m *MockAnthropicClient) SetFailureRate(rate float64) {
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// SetResponseGenerator sets a custom response generator
func (m *MockAnthropicClient) SetResponseGenerator(gen ResponseGenerator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseGenerator = gen
}

// CreateMessage returns a mock response
func (m *MockAnthropicClient) CreateMessage(ctx context.Context, req *client.MessageRequest) (*client.MessageResponse, error) {
	// Simulate API delay
	select {
	case <-time.After(m.messageDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	m.mu.Lock()
	failureRate := m.failureRate
	generator := m.responseGenerator
	m.mu.Unlock()

	// Simulate failure
	if failureRate > 0 && time.Now().UnixNano()%100 < int64(failureRate*100) {
		return nil, fmt.Errorf("mock API error: simulated failure")
	}

	return generator.GenerateMessage(req), nil
}

// SubmitBatch submits a mock batch job
func (m *MockAnthropicClient) SubmitBatch(ctx context.Context, requests []client.BatchRequest) (*client.BatchJob, error) {
	select {
	case <-time.After(m.batchDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextBatchID++
	jobID := fmt.Sprintf("batch_mock_%d", m.nextBatchID)

	job := &client.BatchJob{
		ID:               jobID,
		Type:             "batch",
		ProcessingStatus: "processing",
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	job.RequestCounts.Total = len(requests)
	job.RequestCounts.Processing = len(requests)

	// Generate mock results
	results := make([]client.BatchResult, len(requests))
	for i, req := range requests {
		results[i] = client.BatchResult{
			CustomID: req.CustomID,
			Result:   *m.responseGenerator.GenerateMessage(&req.Params),
		}
	}

	m.batches[jobID] = job
	m.batchResults[jobID] = results

	return job, nil
}

// GetBatchStatus returns the status of a batch job
func (m *MockAnthropicClient) GetBatchStatus(ctx context.Context, jobID string) (*client.BatchJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, exists := m.batches[jobID]
	if !exists {
		return nil, fmt.Errorf("batch job not found: %s", jobID)
	}

	// Simulate job progression: processing -> completed
	if job.ProcessingStatus == "processing" {
		job.ProcessingStatus = "completed"
		job.RequestCounts.Succeeded = job.RequestCounts.Total
		job.RequestCounts.Processing = 0
		job.CompletedAt = time.Now()
		job.OutputFileID = jobID + "_output"
	}

	return job, nil
}

// GetBatchResults returns results from a batch job
func (m *MockAnthropicClient) GetBatchResults(ctx context.Context, jobID string) ([]client.BatchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results, exists := m.batchResults[jobID]
	if !exists {
		return nil, fmt.Errorf("batch results not found: %s", jobID)
	}

	return results, nil
}

// CancelBatch cancels a batch job
func (m *MockAnthropicClient) CancelBatch(ctx context.Context, jobID string) (*client.BatchJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, exists := m.batches[jobID]
	if !exists {
		return nil, fmt.Errorf("batch job not found: %s", jobID)
	}

	job.ProcessingStatus = "canceled"
	return job, nil
}

// ClearBatches clears all stored batches (useful for test cleanup)
func (m *MockAnthropicClient) ClearBatches() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches = make(map[string]*client.BatchJob)
	m.batchResults = make(map[string][]client.BatchResult)
	m.nextBatchID = 1000
}

// GetBatchCount returns the number of stored batches
func (m *MockAnthropicClient) GetBatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}
