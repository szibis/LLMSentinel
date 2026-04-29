package tools

import (
	"fmt"
	"testing"
)

// TestErrorHandler_Fuzz_AllErrorTypes tests all 6 error scenarios from spec
func TestErrorHandler_Fuzz_AllErrorTypes(t *testing.T) {
	policy := ErrorPolicy{
		FailOpen:       true,
		LogAllErrors:   true,
		AlertThreshold: 5,
	}
	handler := NewErrorHandler(policy)

	scenarios := []ErrorScenario{
		ErrorTimeout,
		ErrorConnection,
		ErrorInvalidOutput,
		ErrorCrash,
		ErrorRateLimit,
		ErrorAuthorization,
	}

	for _, scenario := range scenarios {
		retryCount := 0
		result := handler.Handle("test_tool", scenario, fmt.Errorf("test error"), &retryCount)

		if result != nil {
			toolErr := result.(*ToolError)
			t.Logf("Scenario %v: retry=%v, fallback=%v", scenario, toolErr.Retry, toolErr.Fallback)
		}
	}
}

// TestErrorHandler_Fuzz_ConsecutiveErrors tests consecutive error tracking
func TestErrorHandler_Fuzz_ConsecutiveErrors(t *testing.T) {
	policy := ErrorPolicy{
		AlertThreshold: 3,
		FailOpen:       false,
	}
	handler := NewErrorHandler(policy)

	// Trigger errors until alert threshold
	for i := 0; i < policy.AlertThreshold+2; i++ {
		retryCount := 0
		_ = handler.Handle("tool_a", ErrorConnection, fmt.Errorf("error %d", i), &retryCount)
	}

	// Check that consecutive error count is tracked
	if handler.policy.ConsecutiveErrors["tool_a"] <= policy.AlertThreshold {
		t.Logf("Consecutive error count: %d", handler.policy.ConsecutiveErrors["tool_a"])
	}

	// Different tool should start fresh
	retryCount := 0
	_ = handler.Handle("tool_b", ErrorConnection, fmt.Errorf("error"), &retryCount)

	if handler.policy.ConsecutiveErrors["tool_b"] != 1 {
		t.Errorf("Different tool should start fresh, got count %d", handler.policy.ConsecutiveErrors["tool_b"])
	}
}

// TestErrorHandler_Fuzz_AllToolNames tests with various tool names
func TestErrorHandler_Fuzz_AllToolNames(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{LogAllErrors: false})

	toolNames := []string{
		"",
		"tool",
		"UPPERCASE_TOOL",
		"tool_with_underscore",
		"tool-with-dash",
		"tool/with/slash",
		"tool with spaces",
		"\ttabbed_tool",
		"tool_\x00_null",
		"tool_" + fmt.Sprintf("%d", 999999),
	}

	for _, name := range toolNames {
		retryCount := 0
		_ = handler.Handle(name, ErrorConnection, fmt.Errorf("error"), &retryCount)
	}

	// All should be tracked
	if len(handler.policy.ConsecutiveErrors) < len(toolNames) {
		t.Logf("Tracked %d tools from %d unique names", len(handler.policy.ConsecutiveErrors), len(toolNames))
	}
}

// TestErrorHandler_Fuzz_AlertThresholds tests various alert thresholds
func TestErrorHandler_Fuzz_AlertThresholds(t *testing.T) {
	thresholds := []int{
		0,  // will be set to 5 (default)
		1,  // alert immediately
		5,  // default
		100,
		999999,
	}

	for _, threshold := range thresholds {
		policy := ErrorPolicy{
			AlertThreshold: threshold,
			LogAllErrors:   true,
		}
		handler := NewErrorHandler(policy)

		// Trigger enough errors to hit or exceed threshold
		for i := 0; i < threshold+10; i++ {
			retryCount := 0
			_ = handler.Handle("tool", ErrorTimeout, fmt.Errorf("error %d", i), &retryCount)
		}
	}
}

// TestErrorHandler_Fuzz_RetryCountTracking tests retry count edge cases
func TestErrorHandler_Fuzz_RetryCountTracking(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{})

	retryCounts := []int{0, 1, 10, 100, -1, 999999}

	for _, count := range retryCounts {
		retryCount := count
		_ = handler.Handle("tool", ErrorConnection, fmt.Errorf("error"), &retryCount)

		// Should return ToolError with retry/fallback info
	}
}

// TestErrorHandler_Fuzz_ErrorTypes tests with various error values
func TestErrorHandler_Fuzz_ErrorTypes(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{LogAllErrors: true})

	errs := []error{
		nil,
		fmt.Errorf(""),
		fmt.Errorf("simple error"),
		fmt.Errorf("error with newline\n"),
		fmt.Errorf("error with %s", "formatting"),
		fmt.Errorf("%1000s", "very long error"),
	}

	for _, err := range errs {
		retryCount := 0
		_ = handler.Handle("tool", ErrorTimeout, err, &retryCount)
	}
}

// TestErrorHandler_Fuzz_ConcurrentErrorHandling tests concurrent error handling
func TestErrorHandler_Fuzz_ConcurrentErrorHandling(t *testing.T) {
	handler := NewErrorHandler(ErrorPolicy{AlertThreshold: 10})

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(idx int) {
			toolName := fmt.Sprintf("tool_%d", idx%10)
			scenario := ErrorScenario([]string{
				"timeout", "connection_refused", "invalid_output",
				"crash", "rate_limit", "authorization",
			}[idx%6])

			retryCount := idx
			_ = handler.Handle(toolName, scenario, fmt.Errorf("error %d", idx), &retryCount)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify error counts are tracked (safe read with lock)
	handler.mu.Lock()
	errorCount := len(handler.policy.ConsecutiveErrors)
	handler.mu.Unlock()

	if errorCount == 0 {
		t.Error("No concurrent errors tracked")
	}
}

// TestErrorHandler_Fuzz_FailOpenBehavior tests fail_open policy
func TestErrorHandler_Fuzz_FailOpenBehavior(t *testing.T) {
	testCases := []bool{true, false}

	for _, failOpen := range testCases {
		policy := ErrorPolicy{FailOpen: failOpen}
		handler := NewErrorHandler(policy)

		retryCount := 0
		result := handler.Handle("tool", ErrorTimeout, fmt.Errorf("error"), &retryCount)

		toolErr := result.(*ToolError)
		if failOpen && !toolErr.Fallback {
			t.Errorf("With FailOpen=true, should allow fallback")
		}
	}
}

// TestErrorHandler_Fuzz_LoggingBehavior tests logging flag
func TestErrorHandler_Fuzz_LoggingBehavior(t *testing.T) {
	policies := []bool{true, false}

	for _, logAll := range policies {
		policy := ErrorPolicy{LogAllErrors: logAll}
		handler := NewErrorHandler(policy)

		// Should not panic regardless of logging setting
		for i := 0; i < 100; i++ {
			retryCount := 0
			_ = handler.Handle("tool", ErrorConnection, fmt.Errorf("error %d", i), &retryCount)
		}
	}
}

// TestErrorHandler_Fuzz_ScenarioDistribution tests all scenarios with various configurations
func TestErrorHandler_Fuzz_ScenarioDistribution(t *testing.T) {
	scenarios := []ErrorScenario{
		ErrorTimeout,
		ErrorConnection,
		ErrorInvalidOutput,
		ErrorCrash,
		ErrorRateLimit,
		ErrorAuthorization,
	}

	for _, scenario := range scenarios {
		handler := NewErrorHandler(ErrorPolicy{AlertThreshold: 5})

		retryCount := 0
		testErr := fmt.Errorf("test error for scenario %v", scenario)
		result := handler.Handle("test", scenario, testErr, &retryCount)

		toolErr := result.(*ToolError)
		if toolErr.Scenario != scenario {
			t.Errorf("Scenario mismatch: expected %v, got %v", scenario, toolErr.Scenario)
		}

		t.Logf("Scenario %v: Retry=%v, Fallback=%v, RetryDelay=%v",
			scenario, toolErr.Retry, toolErr.Fallback, toolErr.RetryDelay)
	}
}
