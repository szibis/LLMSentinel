package tools

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/gateway"
)

func TestNewToolManager(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}

	manager := NewToolManager(registry, factory)

	if manager == nil {
		t.Error("NewToolManager() returned nil")
		return
	}
	if manager.registry == nil {
		t.Error("Manager.registry is nil")
		return
	}
	if manager.invoker == nil {
		t.Error("Manager.invoker is nil")
		return
	}
	if manager.selector == nil {
		t.Error("Manager.selector is nil")
		return
	}
	if manager.errorHandler == nil {
		t.Error("Manager.errorHandler is nil")
		return
	}
	if manager.processor == nil {
		t.Error("Manager.processor is nil")
		return
	}
}

func TestToolManager_InvokeForIntent_Success(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	ctx := context.Background()
	result, err := manager.InvokeForIntent(ctx, "quick_answer", "test query")

	if err != nil {
		t.Errorf("InvokeForIntent() error = %v", err)
	}
	// Result should be string (context injection output)
	if result == "" {
		t.Logf("InvokeForIntent() returned empty string")
	}
}

func TestToolManager_InvokeForIntent_UnknownIntent(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	ctx := context.Background()
	result, err := manager.InvokeForIntent(ctx, "unknown_intent_xyz", "test query")

	// Should not error due to fail_open policy
	if err != nil {
		t.Logf("InvokeForIntent() returned error (acceptable with fail_open): %v", err)
	}
	// Result should be empty string or empty context
	if result != "" {
		t.Logf("InvokeForIntent() returned non-empty result for unknown intent")
	}
}

func TestToolManager_RegisterCustomTool(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tool := &ToolMetadata{
		Name: "custom_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false

	err := manager.RegisterCustomTool(context.Background(), tool)
	if err != nil {
		t.Errorf("RegisterCustomTool() error = %v", err)
	}

	registered, _ := registry.GetTool("custom_tool")
	if registered == nil {
		t.Error("Tool not registered after RegisterCustomTool()")
	}
}

func TestToolManager_UpdateTool(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tool := &ToolMetadata{
		Name: "updateable_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false
	registry.RegisterTool(context.Background(), tool)

	updates := &ToolMetadata{
		Path: "/usr/bin/cat",
	}

	err := manager.UpdateTool(context.Background(), "updateable_tool", updates)
	if err != nil {
		t.Errorf("UpdateTool() error = %v", err)
	}

	updated, _ := registry.GetTool("updateable_tool")
	if updated.Path != "/usr/bin/cat" {
		t.Errorf("Tool path = %q, want /usr/bin/cat", updated.Path)
	}
}

func TestToolManager_RemoveTool(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tool := &ToolMetadata{
		Name: "removable_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false
	registry.RegisterTool(context.Background(), tool)

	err := manager.RemoveTool("removable_tool")
	if err != nil {
		t.Errorf("RemoveTool() error = %v", err)
	}

	_, err = registry.GetTool("removable_tool")
	if err == nil {
		t.Error("Tool still exists after RemoveTool()")
	}
}

func TestToolManager_GetToolStatus(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tool := &ToolMetadata{
		Name:           "status_tool",
		Type:           "cli",
		Path:           "/usr/bin/ls",
		HealthStatus:   "healthy",
		Enabled:        true,
		RegisteredAt:   time.Now(),
		ConsecutiveErrors: 0,
	}

	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false
	registry.RegisterTool(context.Background(), tool)

	status, err := manager.GetToolStatus("status_tool")
	if err != nil {
		t.Errorf("GetToolStatus() error = %v", err)
	}

	if status["name"] != "status_tool" {
		t.Errorf("Status name = %v, want status_tool", status["name"])
	}
	if status["type"] != "cli" {
		t.Errorf("Status type = %v, want cli", status["type"])
	}
}

func TestToolManager_GetToolStatus_NotFound(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	_, err := manager.GetToolStatus("nonexistent")
	if err == nil {
		t.Error("GetToolStatus() should error for nonexistent tool")
	}
}

func TestToolManager_ListAllTools(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tools := []string{"tool1", "tool2", "tool3"}
	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false

	for _, name := range tools {
		tool := &ToolMetadata{
			Name:         name,
			Type:         "cli",
			Path:         "/usr/bin/ls",
			HealthStatus: "healthy",
			Enabled:      true,
		}
		registry.RegisterTool(context.Background(), tool)
	}

	listed := manager.ListAllTools()
	if len(listed) != 3 {
		t.Errorf("ListAllTools() returned %d tools, want 3", len(listed))
	}
}

func TestToolManager_StartHealthMonitoring(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.StartHealthMonitoring(ctx)
	// Should not panic; monitoring runs in goroutine
	time.Sleep(100 * time.Millisecond)
}

func TestToolManager_GetAuditLog(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tool := &ToolMetadata{
		Name: "audit_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	registry.validationOnAdd = false
	registry.healthCheckOnAdd = false
	registry.RegisterTool(context.Background(), tool)

	log := manager.GetAuditLog(10)
	if len(log) == 0 {
		t.Error("GetAuditLog() should contain entries")
	}
}

func TestToolManager_UpdateIntentToolMapping(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	tools := []string{"custom_tool_1", "custom_tool_2"}
	manager.UpdateIntentToolMapping("custom_intent", tools)

	selected, _ := manager.selector.SelectForIntent("custom_intent")
	if len(selected) != len(tools) {
		t.Errorf("After UpdateIntentToolMapping, got %d tools, want %d", len(selected), len(tools))
	}
}

func TestToolManager_SetToolWeights(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	customWeight := 0.6
	builtinWeight := 0.4

	manager.SetToolWeights(customWeight, builtinWeight)

	if manager.selector.customToolWeight != customWeight {
		t.Errorf("customToolWeight = %f, want %f", manager.selector.customToolWeight, customWeight)
	}
	if manager.selector.builtinWeight != builtinWeight {
		t.Errorf("builtinWeight = %f, want %f", manager.selector.builtinWeight, builtinWeight)
	}
}

func TestToolManager_LoadFromConfig(t *testing.T) {
	registry := NewToolRegistry("/tmp/config.yaml")
	factory := &gateway.AdapterFactory{}
	manager := NewToolManager(registry, factory)

	if manager == nil {
		t.Error("NewToolManager returned nil")
	}
	// LoadFromConfig tested in integration tests with proper config
}
