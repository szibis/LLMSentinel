package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicClient wraps the Anthropic API for messages and batch operations
type AnthropicClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryMax   int
	retryDelay time.Duration
}

// MessageRequest represents a single message request
type MessageRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	System string `json:"system,omitempty"`
}

// MessageResponse represents the response from the messages API
type MessageResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// BatchRequest represents a single request in a batch
type BatchRequest struct {
	CustomID string         `json:"custom_id"`
	Params   MessageRequest `json:"params"`
}

// BatchJob represents a submitted batch job
type BatchJob struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	ProcessingStatus string `json:"processing_status"`
	RequestCounts    struct {
		Processing int `json:"processing"`
		Succeeded  int `json:"succeeded"`
		Errored    int `json:"errored"`
		Canceled   int `json:"canceled"`
		Total      int `json:"total"`
	} `json:"request_counts"`
	OutputFileID string    `json:"output_file_id,omitempty"`
	ErrorFileID  string    `json:"error_file_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
}

// BatchResult represents a result from a batch job
type BatchResult struct {
	CustomID string                 `json:"custom_id"`
	Result   MessageResponse        `json:"result"`
	Error    map[string]interface{} `json:"error,omitempty"`
}

// NewAnthropicClient creates a new client for the Anthropic API
func NewAnthropicClient(apiKey string) *AnthropicClient {
	return &AnthropicClient{
		apiKey:     apiKey,
		baseURL:    "https://api.anthropic.com/v1",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retryMax:   3,
		retryDelay: 1 * time.Second,
	}
}

// CreateMessage sends a single message request and returns the response
func (c *AnthropicClient) CreateMessage(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var msgResp MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &msgResp, nil
}

// SubmitBatch submits a batch of requests to the Anthropic Batch API
func (c *AnthropicClient) SubmitBatch(ctx context.Context, requests []BatchRequest) (*BatchJob, error) {
	// Convert requests to JSONL format
	var buf bytes.Buffer
	for _, req := range requests {
		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		buf.Write(data)
		buf.WriteString("\n")
	}

	// For simplicity, we'll send the JSONL as raw body
	// In production, use multipart/form-data with file upload
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/batches", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var job BatchJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &job, nil
}

// GetBatchStatus retrieves the status of a submitted batch job
func (c *AnthropicClient) GetBatchStatus(ctx context.Context, jobID string) (*BatchJob, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/batches/%s", c.baseURL, jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var job BatchJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &job, nil
}

// GetBatchResults retrieves the results from a completed batch job
func (c *AnthropicClient) GetBatchResults(ctx context.Context, jobID string) ([]BatchResult, error) {
	job, err := c.GetBatchStatus(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.OutputFileID == "" {
		return nil, fmt.Errorf("batch job not complete or no output file")
	}

	// Retrieve the output file
	httpReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/files/%s/content", c.baseURL, job.OutputFileID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse JSONL output
	var results []BatchResult
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var result BatchResult
		if err := decoder.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode result: %w", err)
		}
		results = append(results, result)
	}

	return results, nil
}

// CancelBatch cancels a batch job
func (c *AnthropicClient) CancelBatch(ctx context.Context, jobID string) (*BatchJob, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/batches/%s/cancel", c.baseURL, jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq)

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var job BatchJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &job, nil
}

// Helper: set common headers for all requests
func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "claude-escalate/0.6.0")
}

// Helper: do request with retry logic on transient failures
func (c *AnthropicClient) doWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.retryMax; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.retryMax {
				// exponential backoff: cap shift at 30 to prevent overflow
				shift := attempt
				if shift > 30 {
					shift = 30
				}
				// #nosec G115: shift is bounded to 30, safe to convert to uint
				time.Sleep(c.retryDelay * time.Duration(1<<uint(shift)))
				continue
			}
			return nil, fmt.Errorf("HTTP error: %w", err)
		}

		// Retry on 429 (rate limit) and 5xx errors
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < c.retryMax {
				// exponential backoff: cap shift at 30 to prevent overflow
				shift := attempt
				if shift > 30 {
					shift = 30
				}
				// #nosec G115: shift is bounded to 30, safe to convert to uint
				time.Sleep(c.retryDelay * time.Duration(1<<uint(shift)))
				continue
			}
			return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// TokenCounter estimates token usage for requests
type TokenCounter struct {
	InputTokens  int64
	OutputTokens int64
}

// EstimateTokens provides rough token estimates before sending to API
func EstimateTokens(text string) int {
	// Rough estimate: 4 characters per token (OpenAI's estimate)
	return (len(text) / 4) + 1
}

// CostCalculator estimates API costs with batch discount
type CostCalculator struct {
	InputCostPer1KTokens  float64
	OutputCostPer1KTokens float64
	BatchDiscount         float64 // e.g., 0.5 for 50% discount
}

// CalculateCost returns the cost for given token counts
func (cc *CostCalculator) CalculateCost(inputTokens, outputTokens int64, useBatchDiscount bool) float64 {
	inputCost := float64(inputTokens) / 1000 * cc.InputCostPer1KTokens
	outputCost := float64(outputTokens) / 1000 * cc.OutputCostPer1KTokens
	totalCost := inputCost + outputCost

	if useBatchDiscount {
		totalCost *= (1 - cc.BatchDiscount)
	}

	return totalCost
}
