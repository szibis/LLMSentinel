package discovery

import (
	"testing"
)

func TestGetKnownTools_ReturnsArray(t *testing.T) {
	tools := GetKnownTools()
	if len(tools) == 0 {
		t.Error("GetKnownTools returned empty array")
	}
}

func TestGetKnownTools_HasRequiredFields(t *testing.T) {
	tools := GetKnownTools()
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("Tool missing name field")
		}
		if tool.Type == "" {
			t.Error("Tool missing type field")
		}
	}
}

func TestGetAvailableToolTypes_ReturnsArray(t *testing.T) {
	types := GetAvailableToolTypes()
	if len(types) == 0 {
		t.Error("GetAvailableToolTypes returned empty array")
	}
}

func TestGetAvailableToolTypes_ContainsExpectedTypes(t *testing.T) {
	types := GetAvailableToolTypes()
	typeMap := make(map[string]bool)
	for _, typeInfo := range types {
		typeMap[typeInfo["type"]] = true
	}

	expectedTypes := []string{"cli", "mcp", "rest", "database", "binary"}
	for _, expected := range expectedTypes {
		if !typeMap[expected] {
			t.Errorf("Expected tool type not found: %s", expected)
		}
	}
}

func TestDetectTools_ReturnsDetectedToolsStruct(t *testing.T) {
	tools := DetectTools()
	if tools == nil {
		t.Error("DetectTools returned nil")
		return
	}
	if tools.LSPServers == nil {
		t.Error("DetectTools LSPServers is nil")
	}
}

func TestDetectInstalledLanguages_ReturnsArray(t *testing.T) {
	langs := DetectInstalledLanguages()
	if langs == nil {
		t.Error("DetectInstalledLanguages returned nil")
	}
}
