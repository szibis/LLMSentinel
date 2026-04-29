package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ErrorScenario represents different tool failure scenarios
type ErrorScenario string

const (
	ErrorTimeout       ErrorScenario = "timeout"
	ErrorConnection    ErrorScenario = "connection_refused"
	ErrorInvalidOutput ErrorScenario = "invalid_output"
	ErrorCrash         ErrorScenario = "crash"
	ErrorRateLimit     ErrorScenario = "rate_limit"
	ErrorAuthorization ErrorScenario = "authorization"
)

// ErrorPolicy defines how different errors are handled
type ErrorPolicy struct {
	FailOpen            bool
	LogAllErrors        bool
	AlertThreshold      int
	ConsecutiveErrors   map[string]int
}

// ErrorHandler manages tool execution errors
type ErrorHandler struct {
	mu     sync.Mutex
	policy ErrorPolicy
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(policy ErrorPolicy) *ErrorHandler {
	if policy.AlertThreshold == 0 {
		policy.AlertThreshold = 5
	}
	if policy.ConsecutiveErrors == nil {
		policy.ConsecutiveErrors = make(map[string]int)
	}
	return &ErrorHandler{policy: policy}
}

// Handle processes a tool error and determines recovery strategy
func (eh *ErrorHandler) Handle(toolName string, scenario ErrorScenario, err error, retryCount *int) error {
	eh.mu.Lock()
	eh.policy.ConsecutiveErrors[toolName]++
	consecutiveCount := eh.policy.ConsecutiveErrors[toolName]
	logAllErrors := eh.policy.LogAllErrors
	alertThreshold := eh.policy.AlertThreshold
	eh.mu.Unlock()

	// Log error if enabled
	if logAllErrors {
		fmt.Printf("Tool error [%s]: %s - %v (consecutive: %d)\n",
			toolName, scenario, err, consecutiveCount)
	}

	// Check alert threshold
	if consecutiveCount >= alertThreshold {
		fmt.Printf("⚠️ Alert: Tool %s has %d consecutive errors\n",
			toolName, consecutiveCount)
	}

	// Return strategy per scenario
	switch scenario {
	case ErrorTimeout:
		// Don't retry on timeout, try next tool
		return &ToolError{
			Scenario: scenario,
			Message:  "Tool execution timeout",
			Retry:    false,
			Fallback: true,
		}

	case ErrorConnection:
		// Retry with backoff
		return &ToolError{
			Scenario: scenario,
			Message:  fmt.Sprintf("Connection refused: %v", err),
			Retry:    true,
			Fallback: true,
			RetryDelay: 5 * time.Second,
			MaxRetries: 3,
		}

	case ErrorInvalidOutput:
		// Don't retry, try next tool
		return &ToolError{
			Scenario: scenario,
			Message:  "Tool output malformed",
			Retry:    false,
			Fallback: true,
		}

	case ErrorCrash:
		// Recover after delay
		return &ToolError{
			Scenario: scenario,
			Message:  "Tool crashed",
			Retry:    false,
			Fallback: true,
			RecoveryDelay: 5 * time.Minute,
		}

	case ErrorRateLimit:
		// Backoff exponentially
		baseDelay := 1 * time.Second
		if *retryCount > 0 {
			baseDelay = time.Duration(1<<uint(*retryCount)) * time.Second
		}
		return &ToolError{
			Scenario: scenario,
			Message:  "Rate limit exceeded",
			Retry:    true,
			Fallback: true,
			RetryDelay: baseDelay,
			MaxRetries: 3,
		}

	case ErrorAuthorization:
		// Don't auto-retry
		return &ToolError{
			Scenario: scenario,
			Message:  "Authorization failed",
			Retry:    false,
			Fallback: true,
		}

	default:
		return fmt.Errorf("unknown error scenario: %s", scenario)
	}
}

// ResetConsecutiveCount resets error counter for a tool
func (eh *ErrorHandler) ResetConsecutiveCount(toolName string) {
	eh.mu.Lock()
	eh.policy.ConsecutiveErrors[toolName] = 0
	eh.mu.Unlock()
}

// ToolError represents a tool execution error with recovery strategy
type ToolError struct {
	Scenario      ErrorScenario
	Message       string
	Retry         bool
	Fallback      bool
	RetryDelay    time.Duration
	MaxRetries    int
	RecoveryDelay time.Duration
}

// Error implements the error interface
func (te *ToolError) Error() string {
	return fmt.Sprintf("[%s] %s", te.Scenario, te.Message)
}

// ShouldRetry determines if the error should be retried
func (te *ToolError) ShouldRetry() bool {
	return te.Retry
}

// ShouldFallback determines if next tool should be tried
func (te *ToolError) ShouldFallback() bool {
	return te.Fallback
}

// InvokeWithErrorHandling executes a function with error recovery
func InvokeWithErrorHandling(ctx context.Context, fn func(context.Context) error, eh *ErrorHandler, toolName string) error {
	retries := 0
	maxRetries := 3

	for {
		err := fn(ctx)
		if err == nil {
			eh.ResetConsecutiveCount(toolName)
			return nil
		}

		toolErr, ok := err.(*ToolError)
		if !ok {
			return err
		}

		if !toolErr.ShouldRetry() || retries >= maxRetries {
			return toolErr
		}

		retries++
		select {
		case <-time.After(toolErr.RetryDelay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
