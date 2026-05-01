package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
	"github.com/szibis/claude-escalate/internal/llm"
	"github.com/szibis/claude-escalate/internal/mock"
)

// ProviderType identifies which backend to use
type ProviderType string

const (
	ProviderTypeReal  ProviderType = "real"      // Real Anthropic API
	ProviderTypeMock  ProviderType = "mock"      // Mock API for testing
	ProviderTypeLocal ProviderType = "local"     // Local LLM (LM Studio, Ollama)
)

// ClientProvider interface allows switching between different API backends
// All backends must implement the same interface as AnthropicClient
type ClientProvider interface {
	CreateMessage(ctx context.Context, req *client.MessageRequest) (*client.MessageResponse, error)
	SubmitBatch(ctx context.Context, requests []client.BatchRequest) (*client.BatchJob, error)
	GetBatchStatus(ctx context.Context, jobID string) (*client.BatchJob, error)
	GetBatchResults(ctx context.Context, jobID string) ([]client.BatchResult, error)
	CancelBatch(ctx context.Context, jobID string) (*client.BatchJob, error)
}

// MockConfig holds mock-specific configuration
type MockConfig struct {
	MessageDelay  int     // milliseconds
	BatchDelay    int     // milliseconds
	FailureRate   float64 // 0.0 to 1.0
	ResponseFunc  func(req *client.MessageRequest) *client.MessageResponse
}

// ProviderConfig holds configuration for provider selection
type ProviderConfig struct {
	Type       ProviderType
	APIKey     string     // For real Anthropic API
	LocalURL   string     // For local LLM backend
	LocalModel string     // For local LLM backend
	MockConfig *MockConfig // For mock API configuration
}

// Factory creates API clients based on provider configuration
type Factory struct {
	config ProviderConfig
}

// NewFactory creates a new provider factory
func NewFactory(config ProviderConfig) *Factory {
	return &Factory{config: config}
}

// CreateClient creates an API client based on the configured provider type
func (f *Factory) CreateClient() (ClientProvider, error) {
	switch f.config.Type {
	case ProviderTypeReal:
		if f.config.APIKey == "" {
			return nil, fmt.Errorf("API key required for real Anthropic provider")
		}
		return client.NewAnthropicClient(f.config.APIKey), nil

	case ProviderTypeMock:
		mockClient := mock.NewMockAnthropicClient()

		// Apply mock-specific configuration
		if f.config.MockConfig != nil {
			if f.config.MockConfig.MessageDelay > 0 {
				mockClient.SetMessageDelay(convertMilliseconds(f.config.MockConfig.MessageDelay))
			}
			if f.config.MockConfig.BatchDelay > 0 {
				mockClient.SetBatchDelay(convertMilliseconds(f.config.MockConfig.BatchDelay))
			}
			if f.config.MockConfig.FailureRate > 0 {
				mockClient.SetFailureRate(f.config.MockConfig.FailureRate)
			}
			if f.config.MockConfig.ResponseFunc != nil {
				mockClient.SetResponseGenerator(&CustomResponseGenerator{
					fn: f.config.MockConfig.ResponseFunc,
				})
			}
		}

		return mockClient, nil

	case ProviderTypeLocal:
		if f.config.LocalURL == "" {
			return nil, fmt.Errorf("local URL required for local LLM provider")
		}
		if f.config.LocalModel == "" {
			return nil, fmt.Errorf("model name required for local LLM provider")
		}

		// Verify connectivity
		ctx, cancel := context.WithTimeout(context.Background(), 5000*1000*1000) // 5 seconds in nanoseconds
		defer cancel()

		if err := llm.LocalLLMHealthCheck(ctx, f.config.LocalURL); err != nil {
			return nil, fmt.Errorf("local LLM health check failed: %w", err)
		}

		return llm.NewLocalLLMAdapter(f.config.LocalURL, f.config.LocalModel), nil

	default:
		return nil, fmt.Errorf("unknown provider type: %s", f.config.Type)
	}
}

// CustomResponseGenerator wraps a custom response function
type CustomResponseGenerator struct {
	fn func(req *client.MessageRequest) *client.MessageResponse
}

func (c *CustomResponseGenerator) GenerateMessage(req *client.MessageRequest) *client.MessageResponse {
	return c.fn(req)
}

func (c *CustomResponseGenerator) GenerateTokens(text string) (int, int) {
	total := (len(text) / 4) + 1
	return total / 2, total / 2
}

// Helper to convert milliseconds to time.Duration
func convertMilliseconds(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// ProviderFromEnv creates a factory from environment variables
// Supports:
// - LLM_PROVIDER: "real", "mock", or "local"
// - ANTHROPIC_API_KEY: API key for real provider
// - LOCAL_LLM_URL: Base URL for local LLM (e.g., "http://localhost:8000")
// - LOCAL_LLM_MODEL: Model name for local LLM (e.g., "llama-2-7b")
func ProviderFromEnv() (ClientProvider, error) {
	var providerType ProviderType
	providerStr := os.Getenv("LLM_PROVIDER")
	if providerStr == "" {
		providerStr = "real" // Default to real API
	}

	switch providerStr {
	case "real":
		providerType = ProviderTypeReal
	case "mock":
		providerType = ProviderTypeMock
	case "local":
		providerType = ProviderTypeLocal
	default:
		return nil, fmt.Errorf("invalid LLM_PROVIDER: %s", providerStr)
	}

	config := ProviderConfig{
		Type:       providerType,
		APIKey:     os.Getenv("ANTHROPIC_API_KEY"),
		LocalURL:   os.Getenv("LOCAL_LLM_URL"),
		LocalModel: os.Getenv("LOCAL_LLM_MODEL"),
	}

	factory := NewFactory(config)
	return factory.CreateClient()
}
