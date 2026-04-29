package tools

import (
	"context"
	"fmt"
	"testing"
)

func TestNewErrorHandler(t *testing.T) {
	policy := ErrorPolicy{
		FailOpen:       true,
		LogAllErrors:   true,
		AlertThreshold: 5,
	}
	handler := NewErrorHandler(policy)

	if handler == nil {
		t.Error("NewErrorHandler() returned nil")
		return
	}
	if handler.policy.FailOpen != policy.FailOpen {
		t.Error("ErrorPolicy.FailOpen not set correctly")
	}
}

func TestErrorHandler_TimeoutScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorTimeout, nil, &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if tErr.ShouldRetry() {
		t.Error("Timeout should not trigger retry")
	}
	if tErr.Scenario != ErrorTimeout {
		t.Errorf("Scenario = %v, want %v", tErr.Scenario, ErrorTimeout)
	}
}

func TestErrorHandler_ConnectionRefusedScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorConnection, fmt.Errorf("connection refused"), &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if !tErr.ShouldRetry() {
		t.Error("Connection error should trigger retry")
	}
	if tErr.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", tErr.MaxRetries)
	}
}

func TestErrorHandler_RateLimitScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorRateLimit, fmt.Errorf("rate limited"), &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if !tErr.ShouldRetry() {
		t.Error("Rate limit error should trigger retry")
	}
	if tErr.RetryDelay == 0 {
		t.Error("Retry delay should be set for rate limit")
	}
}

func TestErrorHandler_AuthorizationScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorAuthorization, fmt.Errorf("unauthorized"), &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if tErr.ShouldRetry() {
		t.Error("Authorization error should not trigger auto-retry")
	}
	if !tErr.ShouldFallback() {
		t.Error("Authorization error should allow fallback")
	}
}

func TestErrorHandler_CrashScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorCrash, fmt.Errorf("crashed"), &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if tErr.ShouldRetry() {
		t.Error("Crash should not trigger immediate retry")
	}
	if tErr.RecoveryDelay == 0 {
		t.Error("Recovery delay should be set for crash")
	}
}

func TestErrorHandler_InvalidOutputScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	toolErr := handler.Handle("test_tool", ErrorInvalidOutput, fmt.Errorf("invalid output"), &retryCount)
	if toolErr == nil {
		t.Error("Handle() should return ToolError")
	}

	tErr := toolErr.(*ToolError)
	if tErr.ShouldRetry() {
		t.Error("Invalid output should not trigger retry")
	}
	if !tErr.ShouldFallback() {
		t.Error("Invalid output should allow fallback to next tool")
	}
}

func TestErrorHandler_AlertThreshold(t *testing.T) {
	policy := ErrorPolicy{
		FailOpen:       true,
		LogAllErrors:   true,
		AlertThreshold: 3,
	}
	handler := NewErrorHandler(policy)

	if handler.policy.AlertThreshold != 3 {
		t.Errorf("AlertThreshold = %d, want 3", handler.policy.AlertThreshold)
	}
}

func TestErrorHandler_FailOpenPolicy(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})

	if !handler.policy.FailOpen {
		t.Error("FailOpen policy should be true")
	}
}

func TestErrorHandler_LoggingPolicy(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})

	if !handler.policy.LogAllErrors {
		t.Error("LogAllErrors policy should be true")
	}
}

func TestErrorHandler_UnknownScenario(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	// Create an unknown error scenario (not a defined ErrorScenario constant)
	toolErr := handler.Handle("test_tool", ErrorScenario("unknown"), fmt.Errorf("unknown"), &retryCount)

	if toolErr == nil {
		t.Error("Handle() should return error for unknown scenario")
	}
}

func TestErrorHandler_ConsecutiveErrorTracking(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 3})
	retryCount := 0

	// Simulate 3 consecutive errors
	for i := 0; i < 3; i++ {
		handler.Handle("test_tool", ErrorTimeout, nil, &retryCount)
	}

	if handler.policy.ConsecutiveErrors["test_tool"] != 3 {
		t.Errorf("ConsecutiveErrors = %d, want 3", handler.policy.ConsecutiveErrors["test_tool"])
	}
}

func TestErrorHandler_ResetConsecutiveCount(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})
	retryCount := 0

	// Add some errors
	handler.Handle("test_tool", ErrorTimeout, nil, &retryCount)
	handler.Handle("test_tool", ErrorTimeout, nil, &retryCount)

	// Reset
	handler.ResetConsecutiveCount("test_tool")

	if handler.policy.ConsecutiveErrors["test_tool"] != 0 {
		t.Errorf("After reset, ConsecutiveErrors = %d, want 0", handler.policy.ConsecutiveErrors["test_tool"])
	}
}

func TestToolError_Error_Method(t *testing.T) {
	toolErr := &ToolError{
		Scenario: ErrorTimeout,
		Message:  "Tool execution timed out",
	}

	errStr := toolErr.Error()
	if errStr == "" {
		t.Error("Error() should return non-empty string")
	}
	if !(len(errStr) > 0) {
		t.Error("Error() should contain error information")
	}
}

func TestToolError_ShouldRetry(t *testing.T) {
	toolErr := &ToolError{
		Scenario: ErrorTimeout,
		Retry:    false,
	}

	if toolErr.ShouldRetry() {
		t.Error("ShouldRetry() should return false when Retry is false")
	}

	toolErr.Retry = true
	if !toolErr.ShouldRetry() {
		t.Error("ShouldRetry() should return true when Retry is true")
	}
}

func TestToolError_ShouldFallback(t *testing.T) {
	toolErr := &ToolError{
		Scenario: ErrorTimeout,
		Fallback: true,
	}

	if !toolErr.ShouldFallback() {
		t.Error("ShouldFallback() should return true when Fallback is true")
	}

	toolErr.Fallback = false
	if toolErr.ShouldFallback() {
		t.Error("ShouldFallback() should return false when Fallback is false")
	}
}

func TestInvokeWithErrorHandling_Success(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{FailOpen: true, LogAllErrors: true, AlertThreshold: 5})

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return nil // Success
	}

	ctx := context.Background()
	err := InvokeWithErrorHandling(ctx, fn, handler, "test_tool")

	if err != nil {
		t.Errorf("InvokeWithErrorHandling() error = %v, want nil", err)
	}
	if callCount != 1 {
		t.Errorf("Function called %d times, want 1", callCount)
	}
}
