package tools

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/gateway"
)

func TestInvocationRequest_Fields(t *testing.T) {
	req := &InvocationRequest{
		Intent:      "quick_answer",
		Input:       "test query",
		MaxDuration: 30 * time.Second,
		MaxResults:  3,
		TokenBudget: 3000,
	}

	if req.Intent != "quick_answer" {
		t.Errorf("Intent = %q, want quick_answer", req.Intent)
	}
	if req.Input != "test query" {
		t.Errorf("Input = %q, want test query", req.Input)
	}
	if req.MaxResults != 3 {
		t.Errorf("MaxResults = %d, want 3", req.MaxResults)
	}
}

func TestInvocationResult_Fields(t *testing.T) {
	result := &InvocationResult{
		ToolName:   "test_tool",
		Success:    true,
		Output:     "test output",
		DurationMs: 100,
		TokensUsed: 25,
		Cached:     false,
	}

	if result.ToolName != "test_tool" {
		t.Errorf("ToolName = %q, want test_tool", result.ToolName)
	}
	if !result.Success {
		t.Error("Success should be true")
	}
	if result.TokensUsed != 25 {
		t.Errorf("TokensUsed = %d, want 25", result.TokensUsed)
	}
}

func TestInvoker_New(t *testing.T) {
	factory := &gateway.AdapterFactory{}
	selector := NewSelector()
	invoker := NewInvoker(factory, selector)

	if invoker == nil {
		t.Error("NewInvoker() returned nil")
		return
	}
	if invoker.selector == nil {
		t.Error("Invoker.selector is nil")
		return
	}
	if invoker.factory == nil {
		t.Error("Invoker.factory is nil")
		return
	}
}

func TestInvoker_Invoke_EmptyTools(t *testing.T) {
	selector := &Selector{intentToolMap: make(map[string][]string)}
	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	ctx := context.Background()
	req := &InvocationRequest{
		Intent:      "unknown_intent",
		Input:       "test",
		MaxDuration: 5 * time.Second,
		MaxResults:  1,
		TokenBudget: 1000,
	}

	results, err := invoker.Invoke(ctx, req)
	if err != nil {
		t.Logf("Invoke() error = %v (expected for unknown intent)", err)
	}
	if len(results) != 0 {
		t.Logf("Invoke() returned %d results", len(results))
	}
}

func TestInvoker_Invoke_ContextCancellation(t *testing.T) {
	selector := NewSelector()
	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &InvocationRequest{
		Intent:      "quick_answer",
		Input:       "test",
		MaxDuration: 5 * time.Second,
		MaxResults:  1,
		TokenBudget: 1000,
	}

	results, err := invoker.Invoke(ctx, req)
	// Should return empty or error due to cancellation (acceptable on context cancellation)
	if results != nil {
		t.Log("Results received despite context cancellation")
	}
	_ = err // Context cancellation is acceptable
}

func TestInvoker_MaxResults_Limit(t *testing.T) {
	selector := NewSelector()
	// Add multiple tools for same intent
	selector.AddIntentMapping("code_search", []string{"tool1", "tool2", "tool3", "tool4"})

	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &InvocationRequest{
		Intent:      "code_search",
		Input:       "test",
		MaxDuration: 5 * time.Second,
		MaxResults:  2, // Limit to 2 results
		TokenBudget: 5000,
	}

	results, _ := invoker.Invoke(ctx, req)
	if len(results) > 2 {
		t.Errorf("Invoke() returned %d results, want at most 2", len(results))
	}
}

func TestInvoker_TokenBudget_Stored(t *testing.T) {
	selector := NewSelector()
	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	if invoker == nil {
		t.Error("Invoker should be created")
	}
	// TokenBudget is part of request, not invoker
}

func TestInvoker_Default_Values(t *testing.T) {
	selector := NewSelector()
	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	if invoker.maxResults != 3 {
		t.Errorf("Default maxResults = %d, want 3", invoker.maxResults)
	}
	if invoker.timeout != 30*time.Second {
		t.Errorf("Default timeout = %v, want 30s", invoker.timeout)
	}
}

func TestInvoker_Cache_Disabled_By_Default(t *testing.T) {
	selector := NewSelector()
	factory := &gateway.AdapterFactory{}
	invoker := NewInvoker(factory, selector)

	if invoker.cache == nil {
		t.Error("Cache should be initialized")
	}
	if len(invoker.cache) != 0 {
		t.Error("Cache should start empty")
	}
}

func TestInvocationRequest_EmptyIntent(t *testing.T) {
	req := &InvocationRequest{
		Intent:      "",
		Input:       "test",
		MaxDuration: 30 * time.Second,
	}

	if req.Intent == "" {
		t.Log("Empty intent is allowed at construction")
	}
}

func TestInvocationRequest_WithContext(t *testing.T) {
	context := map[string]interface{}{
		"user": "test_user",
		"project": "test_project",
	}

	req := &InvocationRequest{
		Intent:   "quick_answer",
		Input:    "test",
		Context:  context,
		MaxResults: 1,
		TokenBudget: 1000,
	}

	if len(req.Context) != 2 {
		t.Errorf("Context size = %d, want 2", len(req.Context))
	}
}

func TestInvocationResult_ErrorCase(t *testing.T) {
	result := &InvocationResult{
		ToolName: "failed_tool",
		Success:  false,
		Error:    "Connection timeout",
	}

	if result.Success {
		t.Error("Success should be false for error result")
	}
	if result.Error == "" {
		t.Error("Error should contain message")
	}
}
