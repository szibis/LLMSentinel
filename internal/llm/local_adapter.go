package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
)

// LocalLLMAdapter adapts local LLM backends (LM Studio, Ollama, etc.) to Anthropic API interface
type LocalLLMAdapter struct {
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
}

// LocalLLMRequest represents a request to a local LLM API
type LocalLLMRequest struct {
	Model       string                    `json:"model"`
	Messages    []LocalLLMMessage         `json:"messages"`
	Temperature float32                   `json:"temperature,omitempty"`
	MaxTokens   int                       `json:"max_tokens,omitempty"`
	Stream      bool                      `json:"stream,omitempty"`
	Stop        []string                  `json:"stop,omitempty"`
}

// LocalLLMMessage represents a message in local LLM format
type LocalLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LocalLLMResponse represents a response from a local LLM API
type LocalLLMResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens,omitempty"`
		CompletionTokens int `json:"completion_tokens,omitempty"`
	} `json:"usage,omitempty"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices,omitempty"`
}

// NewLocalLLMAdapter creates a new adapter for a local LLM backend
// baseURL should be the API endpoint (e.g., "http://localhost:8000" for LM Studio)
// model should be the model identifier (e.g., "llama-2-7b")
func NewLocalLLMAdapter(baseURL, model string) *LocalLLMAdapter {
	return &LocalLLMAdapter{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 300 * time.Second},
		timeout:    300 * time.Second,
	}
}

// SetTimeout sets the request timeout
func (a *LocalLLMAdapter) SetTimeout(d time.Duration) {
	a.timeout = d
	a.httpClient.Timeout = d
}

// CreateMessage sends a message to the local LLM and returns an Anthropic-compatible response
func (a *LocalLLMAdapter) CreateMessage(ctx context.Context, req *client.MessageRequest) (*client.MessageResponse, error) {
	// Convert Anthropic format to local LLM format
	messages := make([]LocalLLMMessage, 0)

	// Add system message if provided
	if req.System != "" {
		messages = append(messages, LocalLLMMessage{
			Role:    "system",
			Content: req.System,
		})
	}

	// Add user messages
	for _, msg := range req.Messages {
		messages = append(messages, LocalLLMMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create local LLM request
	localReq := LocalLLMRequest{
		Model:     a.model,
		Messages:  messages,
		MaxTokens: req.MaxTokens,
		Stream:    false,
	}

	body, err := json.Marshal(localReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Send request to local LLM
	endpoint := a.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("local LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local LLM error: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var localResp LocalLLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&localResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Extract message content and usage
	content := ""
	if len(localResp.Choices) > 0 {
		content = localResp.Choices[0].Message.Content
	} else if localResp.Message.Content != "" {
		content = localResp.Message.Content
	}

	inputTokens := localResp.Usage.PromptTokens
	outputTokens := localResp.Usage.CompletionTokens

	// Estimate tokens if not provided
	if inputTokens == 0 {
		inputTokens = estimateTokens(req.System) + estimateTokens(req.Messages[0].Content)
	}
	if outputTokens == 0 {
		outputTokens = estimateTokens(content)
	}

	// Convert to Anthropic format
	return &client.MessageResponse{
		ID:   fmt.Sprintf("msg_local_%d", time.Now().UnixNano()),
		Type: "message",
		Role: "assistant",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: content},
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
	}, nil
}

// SubmitBatch is not implemented for local LLMs (they don't support batching)
func (a *LocalLLMAdapter) SubmitBatch(ctx context.Context, requests []client.BatchRequest) (*client.BatchJob, error) {
	return nil, fmt.Errorf("batch API not supported by local LLM adapter")
}

// GetBatchStatus is not implemented for local LLMs
func (a *LocalLLMAdapter) GetBatchStatus(ctx context.Context, jobID string) (*client.BatchJob, error) {
	return nil, fmt.Errorf("batch API not supported by local LLM adapter")
}

// GetBatchResults is not implemented for local LLMs
func (a *LocalLLMAdapter) GetBatchResults(ctx context.Context, jobID string) ([]client.BatchResult, error) {
	return nil, fmt.Errorf("batch API not supported by local LLM adapter")
}

// CancelBatch is not implemented for local LLMs
func (a *LocalLLMAdapter) CancelBatch(ctx context.Context, jobID string) (*client.BatchJob, error) {
	return nil, fmt.Errorf("batch API not supported by local LLM adapter")
}

// estimateTokens provides a rough token count (4 characters per token)
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) / 4) + 1
}

// LocalLLMHealthCheck verifies connectivity to a local LLM backend
func LocalLLMHealthCheck(ctx context.Context, baseURL string) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("local LLM not available at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local LLM health check failed: status %d", resp.StatusCode)
	}

	return nil
}
