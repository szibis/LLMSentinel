package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAnthropicClient(t *testing.T) {
	client := NewAnthropicClient("test-api-key")
	if client.apiKey != "test-api-key" {
		t.Errorf("expected api key 'test-api-key', got %s", client.apiKey)
	}
	if client.baseURL != "https://api.anthropic.com/v1" {
		t.Errorf("expected base URL 'https://api.anthropic.com/v1', got %s", client.baseURL)
	}
}

func TestCreateMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Error("expected x-api-key header")
		}

		response := MessageResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-sonnet",
			StopReason: "end_turn",
		}
		response.Content = make([]struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}, 1)
		response.Content[0].Type = "text"
		response.Content[0].Text = "Hello, world!"
		response.Usage.InputTokens = 100
		response.Usage.OutputTokens = 50

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	req := &MessageRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 1000,
	}

	resp, err := client.CreateMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "msg_123" {
		t.Errorf("expected ID 'msg_123', got %s", resp.ID)
	}
	if resp.Usage.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", resp.Usage.InputTokens)
	}
}

func TestSubmitBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		job := BatchJob{
			ID:               "batch_123",
			Type:             "batch",
			ProcessingStatus: "queued",
			CreatedAt:        time.Now(),
		}
		job.RequestCounts.Total = 2

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	requests := []BatchRequest{
		{
			CustomID: "req_1",
			Params: MessageRequest{
				Model:     "claude-3-sonnet",
				MaxTokens: 1000,
			},
		},
		{
			CustomID: "req_2",
			Params: MessageRequest{
				Model:     "claude-3-sonnet",
				MaxTokens: 1000,
			},
		},
	}

	job, err := client.SubmitBatch(context.Background(), requests)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.ID != "batch_123" {
		t.Errorf("expected ID 'batch_123', got %s", job.ID)
	}
	if job.ProcessingStatus != "queued" {
		t.Errorf("expected status 'queued', got %s", job.ProcessingStatus)
	}
}

func TestGetBatchStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job := BatchJob{
			ID:               "batch_123",
			Type:             "batch",
			ProcessingStatus: "in_progress",
			CreatedAt:        time.Now(),
		}
		job.RequestCounts.Total = 2
		job.RequestCounts.Processing = 1
		job.RequestCounts.Succeeded = 1

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	job, err := client.GetBatchStatus(context.Background(), "batch_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.ProcessingStatus != "in_progress" {
		t.Errorf("expected status 'in_progress', got %s", job.ProcessingStatus)
	}
	if job.RequestCounts.Processing != 1 {
		t.Errorf("expected 1 processing, got %d", job.RequestCounts.Processing)
	}
}

func TestCancelBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		job := BatchJob{
			ID:               "batch_123",
			Type:             "batch",
			ProcessingStatus: "canceled",
			CreatedAt:        time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key")
	client.baseURL = server.URL

	job, err := client.CancelBatch(context.Background(), "batch_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.ProcessingStatus != "canceled" {
		t.Errorf("expected status 'canceled', got %s", job.ProcessingStatus)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"hello", 2},
		{"hello world", 3},
		{"hello world this is a test", 7},
		{"", 1},
	}

	for _, test := range tests {
		result := EstimateTokens(test.text)
		if result != test.expected {
			t.Errorf("EstimateTokens(%q) = %d, expected %d", test.text, result, test.expected)
		}
	}
}

func TestCostCalculation(t *testing.T) {
	calc := &CostCalculator{
		InputCostPer1KTokens:  0.001,
		OutputCostPer1KTokens: 0.002,
		BatchDiscount:         0.5,
	}

	// Test regular cost
	cost := calc.CalculateCost(1000, 500, false)
	expected := (1.0 * 0.001) + (0.5 * 0.002)
	if cost != expected {
		t.Errorf("CalculateCost(1000, 500, false) = %v, expected %v", cost, expected)
	}

	// Test batch discounted cost
	batchCost := calc.CalculateCost(1000, 500, true)
	expectedBatch := expected * 0.5
	if batchCost != expectedBatch {
		t.Errorf("CalculateCost(1000, 500, true) = %v, expected %v", batchCost, expectedBatch)
	}
}
