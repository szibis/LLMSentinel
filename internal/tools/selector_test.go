package tools

import (
	"testing"
)

func TestNewSelector(t *testing.T) {
	sel := NewSelector()
	if sel == nil {
		t.Error("NewSelector() returned nil")
		return
	}
	if sel.intentToolMap == nil {
		t.Error("Selector.intentToolMap is nil")
		return
	}
	if len(sel.intentToolMap) == 0 {
		t.Error("Selector.intentToolMap should have default mappings")
	}
}

func TestSelector_DefaultIntentMappings(t *testing.T) {
	sel := NewSelector()
	tests := []string{
		"quick_answer",
		"detailed_analysis",
		"code_search",
		"security_check",
		"documentation",
		"error_diagnosis",
		"performance_optimization",
		"refactoring",
	}

	for _, intent := range tests {
		tools, err := sel.SelectForIntent(intent)
		if err != nil {
			t.Errorf("SelectForIntent(%s) error = %v", intent, err)
		}
		if len(tools) == 0 {
			t.Errorf("SelectForIntent(%s) returned no tools", intent)
		}
	}
}

func TestSelector_SelectForIntent_UnknownIntent(t *testing.T) {
	sel := NewSelector()
	tools, err := sel.SelectForIntent("unknown_intent_xyz")
	// With fallback enabled (default), unknown intents return fallback tools
	if sel.fallbackEnabled {
		if err != nil {
			t.Errorf("SelectForIntent() with fallback enabled should not error: %v", err)
		}
		if len(tools) == 0 {
			t.Error("SelectForIntent() with fallback should return fallback tools")
		}
	} else {
		if err == nil {
			t.Error("SelectForIntent() should error when fallback disabled")
		}
	}
}

func TestSelector_AddIntentMapping(t *testing.T) {
	sel := NewSelector()
	customTools := []string{"custom_tool_1", "custom_tool_2"}
	sel.AddIntentMapping("custom_intent", customTools)

	tools, err := sel.SelectForIntent("custom_intent")
	if err != nil {
		t.Errorf("SelectForIntent() error = %v", err)
	}
	if len(tools) != len(customTools) {
		t.Errorf("SelectForIntent() returned %d tools, want %d", len(tools), len(customTools))
	}
	for i, tool := range tools {
		if tool != customTools[i] {
			t.Errorf("Tool %d: got %s, want %s", i, tool, customTools[i])
		}
	}
}

func TestSelector_AddIntentMapping_Overwrite(t *testing.T) {
	sel := NewSelector()
	originalTools, _ := sel.SelectForIntent("quick_answer")

	newTools := []string{"overwritten_tool"}
	sel.AddIntentMapping("quick_answer", newTools)

	tools, _ := sel.SelectForIntent("quick_answer")
	if len(tools) != len(newTools) {
		t.Errorf("After overwrite, got %d tools, want %d", len(tools), len(newTools))
	}
	if tools[0] != "overwritten_tool" {
		t.Errorf("After overwrite, got %s, want overwritten_tool", tools[0])
	}
	if len(originalTools) == 0 {
		t.Error("Original tools should not be empty")
	}
}

func TestSelector_SetWeights_DefaultValues(t *testing.T) {
	sel := NewSelector()
	if sel.customToolWeight == 0 || sel.builtinWeight == 0 {
		t.Error("Default weights should be non-zero")
	}
}

func TestSelector_SetWeights_Custom(t *testing.T) {
	sel := NewSelector()
	customW := 0.5
	builtinW := 0.8
	sel.SetWeights(customW, builtinW)

	if sel.customToolWeight != customW {
		t.Errorf("customToolWeight = %f, want %f", sel.customToolWeight, customW)
	}
	if sel.builtinWeight != builtinW {
		t.Errorf("builtinWeight = %f, want %f", sel.builtinWeight, builtinW)
	}
}

func TestSelector_SetWeights_EdgeCases(t *testing.T) {
	sel := NewSelector()

	// Test with zero weights (should be allowed)
	sel.SetWeights(0, 0)
	if sel.customToolWeight != 0 || sel.builtinWeight != 0 {
		t.Error("SetWeights failed to set zero weights")
	}

	// Test with max weights (1.0 is max)
	sel.SetWeights(1.0, 1.0)
	if sel.customToolWeight != 1.0 || sel.builtinWeight != 1.0 {
		t.Error("SetWeights failed to set max weights")
	}

	// Test that out-of-range values are ignored
	sel.SetWeights(999.9, -1.0)
	if sel.customToolWeight == 999.9 {
		t.Error("SetWeights should ignore out-of-range custom weight")
	}
}

func TestSelector_Sequential_Access(t *testing.T) {
	sel := NewSelector()

	// Sequential reads and writes should work fine
	for i := 0; i < 5; i++ {
		tools, _ := sel.SelectForIntent("quick_answer")
		if len(tools) == 0 {
			t.Error("SelectForIntent should return tools")
		}

		toolName := []string{"sequential_tool"}
		sel.AddIntentMapping("sequential_intent", toolName)
	}

	// Verify final state
	tools, _ := sel.SelectForIntent("quick_answer")
	if len(tools) == 0 {
		t.Error("Default mapping should still exist")
	}
}

func TestSelector_EmptyIntentMapping(t *testing.T) {
	sel := NewSelector()
	sel.fallbackEnabled = false // Disable fallback to test empty mapping
	sel.AddIntentMapping("empty_intent", []string{})

	tools, err := sel.SelectForIntent("empty_intent")
	if err == nil {
		t.Error("SelectForIntent() should error for empty tool list with fallback disabled")
	}
	if len(tools) != 0 {
		t.Error("SelectForIntent() should return no tools for empty mapping with fallback disabled")
	}
}

func TestSelector_IntentToolMapping_Persistence(t *testing.T) {
	sel := NewSelector()
	customTools := []string{"persistent_tool_1", "persistent_tool_2", "persistent_tool_3"}
	sel.AddIntentMapping("persistent_intent", customTools)

	// Query multiple times
	for i := 0; i < 5; i++ {
		tools, err := sel.SelectForIntent("persistent_intent")
		if err != nil {
			t.Errorf("Iteration %d: SelectForIntent() error = %v", i, err)
		}
		if len(tools) != len(customTools) {
			t.Errorf("Iteration %d: got %d tools, want %d", i, len(tools), len(customTools))
		}
	}
}

func TestSelector_SetFallback_Enabled(t *testing.T) {
	sel := NewSelector()
	sel.SetFallback(true)

	if !sel.fallbackEnabled {
		t.Error("SetFallback(true) should enable fallback")
	}
}

func TestSelector_SetFallback_Disabled(t *testing.T) {
	sel := NewSelector()
	sel.SetFallback(false)

	if sel.fallbackEnabled {
		t.Error("SetFallback(false) should disable fallback")
	}
}

func TestSelector_SelectByWeight_Empty(t *testing.T) {
	sel := NewSelector()
	result := sel.SelectByWeight([]string{}, map[string]float64{})

	if result != "" {
		t.Errorf("SelectByWeight() with empty tools = %q, want empty", result)
	}
}

func TestSelector_SelectByWeight_Single(t *testing.T) {
	sel := NewSelector()
	tools := []string{"single_tool"}
	weights := map[string]float64{"single_tool": 1.0}

	result := sel.SelectByWeight(tools, weights)

	if result != "single_tool" {
		t.Errorf("SelectByWeight() = %q, want single_tool", result)
	}
}

func TestSelector_SelectByWeight_Multiple(t *testing.T) {
	sel := NewSelector()
	tools := []string{"tool_a", "tool_b", "tool_c"}
	weights := map[string]float64{
		"tool_a": 0.5,
		"tool_b": 0.3,
		"tool_c": 0.2,
	}

	// Run selection multiple times to test weighted randomness
	results := make(map[string]int)
	for i := 0; i < 100; i++ {
		result := sel.SelectByWeight(tools, weights)
		results[result]++
	}

	// All tools should be selected at least once
	for _, tool := range tools {
		if count, ok := results[tool]; !ok || count == 0 {
			t.Errorf("Tool %q was never selected", tool)
		}
	}
}

func TestSelector_SelectByWeight_NoWeights(t *testing.T) {
	sel := NewSelector()
	tools := []string{"tool_a", "tool_b", "tool_c"}
	weights := map[string]float64{} // No weights

	result := sel.SelectByWeight(tools, weights)

	// Should return a random tool from the list
	if result == "" {
		t.Error("SelectByWeight() with no weights should return a tool")
	}
	found := false
	for _, tool := range tools {
		if result == tool {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SelectByWeight() returned %q, not in tools list", result)
	}
}
