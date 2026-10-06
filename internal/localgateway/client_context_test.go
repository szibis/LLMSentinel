package localgateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeClientSystemMessagesBecomeBackendContext(t *testing.T) {
	req := claudeRequest{Model: "sentinel-sonnet", MaxTokens: 128, JSONTools: true,
		System: json.RawMessage(`"Client instructions"`),
		Tools:  []claudeTool{{Name: "Read", Schema: map[string]any{"type": "object"}}},
		Messages: []claudeMessage{
			{Role: "user", Content: json.RawMessage(`"Read the fixture"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"Primary working directory: /synthetic/workspace"}]`)},
		},
	}
	messages, err := prepareClaude(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0]["role"] != "system" || messages[1]["role"] != "user" {
		t.Fatalf("client context leaked into conversation roles: %v", messages)
	}
	context := messages[0]["content"]
	for _, expected := range []string{"Client instructions", "Primary working directory: /synthetic/workspace", "Reply ONLY with one JSON object"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("missing context %q", expected)
		}
	}
	if strings.Index(context, "Primary working directory") > strings.Index(context, "Reply ONLY") {
		t.Fatal("output contract must follow client context")
	}
	if !strings.HasPrefix(messages[1]["content"], "Read the fixture") {
		t.Fatal("user request changed")
	}
}

func TestClaudeClientSystemMessageCannotContainToolBlocks(t *testing.T) {
	req := claudeRequest{Model: "local", MaxTokens: 128, Messages: []claudeMessage{
		{Role: "system", Content: json.RawMessage(`[{"type":"tool_use","id":"x","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`"hello"`)},
	}}
	if _, err := prepareClaude(req); err == nil {
		t.Fatal("tool block in client system context accepted")
	}
}

func TestClaudeRequiresConversationBeyondClientContext(t *testing.T) {
	req := claudeRequest{Model: "local", MaxTokens: 128, Messages: []claudeMessage{{Role: "system", Content: json.RawMessage(`"Environment only"`)}}}
	if _, err := prepareClaude(req); err == nil {
		t.Fatal("context-only request accepted")
	}
}
