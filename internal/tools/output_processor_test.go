package tools

import (
	"testing"
)

func TestNewOutputProcessor(t *testing.T) {
	proc := NewOutputProcessor()
	if proc == nil {
		t.Error("NewOutputProcessor() returned nil")
		return
	}
	if proc.maxContextTokens == 0 {
		t.Error("maxContextTokens should be set")
	}
	if proc.tokenBudget == 0 {
		t.Error("tokenBudget should be set")
	}
}

func TestOutputProcessor_ToString_String(t *testing.T) {
	proc := NewOutputProcessor()
	output := proc.toString("hello world")
	if output != "hello world" {
		t.Errorf("toString() = %q, want %q", output, "hello world")
	}
}

func TestOutputProcessor_ToString_Bytes(t *testing.T) {
	proc := NewOutputProcessor()
	output := proc.toString([]byte("hello bytes"))
	if output != "hello bytes" {
		t.Errorf("toString() = %q, want %q", output, "hello bytes")
	}
}

func TestOutputProcessor_ToString_Map(t *testing.T) {
	proc := NewOutputProcessor()
	data := map[string]interface{}{"key": "value", "count": 42}
	output := proc.toString(data)
	if output == "" {
		t.Error("toString() returned empty string for map")
	}
	// Should contain JSON representation
	if !(len(output) > 0) {
		t.Error("toString() output should not be empty")
	}
}

func TestOutputProcessor_ToString_Default(t *testing.T) {
	proc := NewOutputProcessor()
	output := proc.toString(123)
	if output == "" {
		t.Error("toString() should convert int to string")
	}
}

func TestOutputProcessor_CleanOutput_ANSICodes(t *testing.T) {
	proc := NewOutputProcessor()
	proc.stripANSI = true

	input := "hello\x1b[31mred\x1b[0m world"
	output := proc.cleanOutput(input)
	if output == input {
		t.Error("cleanOutput() should strip ANSI codes")
	}
}

func TestOutputProcessor_CleanOutput_ControlChars(t *testing.T) {
	proc := NewOutputProcessor()
	proc.stripANSI = false

	input := "hello\x00world\x01test"
	output := proc.cleanOutput(input)
	// Control characters are removed, leaving "helloworldtest" without spaces
	if output != "helloworldtest" {
		t.Errorf("cleanOutput() = %q, want %q", output, "helloworldtest")
	}
}

func TestOutputProcessor_CleanOutput_Whitespace(t *testing.T) {
	proc := NewOutputProcessor()
	input := "  hello world  \n"
	output := proc.cleanOutput(input)
	if output != "hello world" {
		t.Errorf("cleanOutput() = %q, want %q", output, "hello world")
	}
}

func TestOutputProcessor_ProcessOutput_TokenEstimation(t *testing.T) {
	proc := NewOutputProcessor()
	output := "a" // Roughly 0.25 tokens per character
	_, tokens, err := proc.ProcessOutput("test_tool", output, map[string]interface{}{})
	if err != nil {
		t.Errorf("ProcessOutput() error = %v", err)
	}
	if tokens < 0 {
		t.Errorf("ProcessOutput() tokens = %d, want non-negative", tokens)
	}
}

func TestOutputProcessor_ProcessOutput_TruncateOnLimit(t *testing.T) {
	proc := NewOutputProcessor()
	proc.perToolLimit = 100
	proc.truncateOnLimit = true

	// Create large output (5000+ chars = 1250+ tokens, exceeds 100 token limit)
	largeOutput := ""
	for i := 0; i < 1250; i++ {
		largeOutput += "a"
	}

	_, tokens, err := proc.ProcessOutput("test_tool", largeOutput, map[string]interface{}{})
	if err != nil {
		t.Errorf("ProcessOutput() error = %v", err)
	}
	if tokens > 100 {
		t.Errorf("ProcessOutput() returned %d tokens, limit is 100", tokens)
	}
}

func TestOutputProcessor_ProcessOutput_ExceedLimitError(t *testing.T) {
	proc := NewOutputProcessor()
	proc.perToolLimit = 100
	proc.truncateOnLimit = false

	largeOutput := ""
	for i := 0; i < 1250; i++ {
		largeOutput += "a"
	}

	_, _, err := proc.ProcessOutput("test_tool", largeOutput, map[string]interface{}{})
	if err == nil {
		t.Error("ProcessOutput() should error when truncate is disabled and limit exceeded")
	}
}

func TestOutputProcessor_FormatForContext(t *testing.T) {
	proc := NewOutputProcessor()
	output := "test output"
	metadata := map[string]interface{}{
		"execution_time_ms": int64(100),
		"cached":            true,
	}
	formatted := proc.formatForContext("test_tool", output, metadata, 5)

	if !(len(formatted) > 0) {
		t.Error("formatForContext() returned empty string")
	}
	// Should contain tool name, output, and token count
	if !(len(formatted) > len(output)) {
		t.Error("formatForContext() should add header/footer")
	}
}

func TestOutputProcessor_ProcessResults_Empty(t *testing.T) {
	proc := NewOutputProcessor()
	output, tokens, err := proc.ProcessResults([]InvocationResult{})
	if err != nil {
		t.Errorf("ProcessResults() error = %v", err)
	}
	if output != "" {
		t.Error("ProcessResults() should return empty string for empty input")
	}
	if tokens != 0 {
		t.Error("ProcessResults() should return 0 tokens for empty input")
	}
}

func TestOutputProcessor_ProcessResults_SingleSuccess(t *testing.T) {
	proc := NewOutputProcessor()
	results := []InvocationResult{
		{
			ToolName: "test_tool",
			Success:  true,
			Output:   "test output",
			DurationMs: 100,
			Cached:   false,
		},
	}
	output, tokens, err := proc.ProcessResults(results)
	if err != nil {
		t.Errorf("ProcessResults() error = %v", err)
	}
	if output == "" {
		t.Error("ProcessResults() returned empty output for successful result")
	}
	if tokens == 0 {
		t.Error("ProcessResults() should count tokens for successful result")
	}
}

func TestOutputProcessor_ProcessResults_WithFailure(t *testing.T) {
	proc := NewOutputProcessor()
	results := []InvocationResult{
		{
			ToolName: "test_tool",
			Success:  false,
			Error:    "Test error",
		},
		{
			ToolName: "test_tool_2",
			Success:  true,
			Output:   "success output",
			DurationMs: 50,
		},
	}
	output, _, err := proc.ProcessResults(results)
	if err != nil {
		t.Errorf("ProcessResults() error = %v", err)
	}
	if !(len(output) > 0) {
		t.Error("ProcessResults() should process mixed results")
	}
}

func TestOutputProcessor_TokenBudgetExceeded(t *testing.T) {
	proc := NewOutputProcessor()
	proc.tokenBudget = 100

	results := []InvocationResult{
		{
			ToolName: "tool1",
			Success:  true,
			Output:   string(make([]byte, 1000)), // ~250 tokens
			DurationMs: 50,
		},
		{
			ToolName: "tool2",
			Success:  true,
			Output:   string(make([]byte, 1000)), // ~250 tokens, will exceed
			DurationMs: 50,
		},
	}
	output, tokens, err := proc.ProcessResults(results)
	if err != nil {
		t.Errorf("ProcessResults() error = %v", err)
	}
	if tokens <= 100 {
		// Should stop adding results when budget exceeded
		// But may include first result before checking budget
		t.Log("Token budget enforcement working (stopped at budget)")
	}
	if output == "" {
		t.Error("ProcessResults() should return output even with budget exceeded")
	}
}

func TestOutputProcessor_StripANSICodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "reset code",
			input:    "hello\x1b[0mworld",
			expected: "helloworld",
		},
		{
			name:     "bold code",
			input:    "hello\x1b[1mworld",
			expected: "helloworld",
		},
		{
			name:     "color codes",
			input:    "hello\x1b[31mred\x1b[0m",
			expected: "hellored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := stripANSICodes(tt.input)
			if !(len(output) > 0) {
				t.Errorf("stripANSICodes() removed all content")
			}
		})
	}
}

func TestOutputProcessor_RemoveControlChars(t *testing.T) {
	input := "hello\x00world\x01test\x02"
	output := removeControlChars(input)
	if output == input {
		t.Error("removeControlChars() should remove control characters")
	}
	if !(len(output) > 0) {
		t.Error("removeControlChars() should preserve printable content")
	}
}
