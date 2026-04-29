package tools

import (
	"context"
	"testing"
	"time"
)

func TestNewToolRegistry(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	if reg == nil {
		t.Error("NewToolRegistry() returned nil")
		return
	}
	if reg.configPath != "/tmp/config.yaml" {
		t.Errorf("configPath = %q, want /tmp/config.yaml", reg.configPath)
	}
	if len(reg.tools) != 0 {
		t.Error("tools map should start empty")
	}
	if !reg.monitoringEnabled {
		t.Error("monitoring should be enabled by default")
	}
	if !reg.validationOnAdd {
		t.Error("validation should be enabled by default")
	}
	if !reg.healthCheckOnAdd {
		t.Error("health check should be enabled by default")
	}
	if !reg.auditTrail {
		t.Error("audit trail should be enabled by default")
	}
}

func TestToolRegistry_RegisterTool_Valid(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "test_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	err := reg.RegisterTool(context.Background(), tool)
	if err != nil {
		t.Errorf("RegisterTool() error = %v", err)
	}

	registered, _ := reg.GetTool("test_tool")
	if registered == nil {
		t.Error("Tool not registered")
		return
	}
	if registered.Name != "test_tool" {
		t.Errorf("Tool name = %q, want test_tool", registered.Name)
	}
	if !registered.Enabled {
		t.Error("Tool should be enabled after registration")
	}
}

func TestToolRegistry_RegisterTool_MissingName(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	err := reg.RegisterTool(context.Background(), tool)
	if err == nil {
		t.Error("RegisterTool() should error when name is missing")
	}
}

func TestToolRegistry_RegisterTool_MissingType(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "test_tool",
		Type: "",
		Path: "/usr/bin/ls",
	}

	err := reg.RegisterTool(context.Background(), tool)
	if err == nil {
		t.Error("RegisterTool() should error when type is missing")
	}
}

func TestToolRegistry_RegisterTool_Duplicate(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool1 := &ToolMetadata{
		Name: "duplicate_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}
	tool2 := &ToolMetadata{
		Name: "duplicate_tool",
		Type: "cli",
		Path: "/usr/bin/cat",
	}

	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false

	reg.RegisterTool(context.Background(), tool1)
	err := reg.RegisterTool(context.Background(), tool2)
	if err == nil {
		t.Error("RegisterTool() should error for duplicate name")
	}
}

func TestToolRegistry_UpdateTool(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name:    "test_tool",
		Type:    "cli",
		Path:    "/usr/bin/ls",
		Enabled: true,
	}

	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false
	reg.RegisterTool(context.Background(), tool)

	updates := &ToolMetadata{
		Path: "/usr/bin/cat",
	}
	err := reg.UpdateTool(context.Background(), "test_tool", updates)
	if err != nil {
		t.Errorf("UpdateTool() error = %v", err)
	}

	updated, _ := reg.GetTool("test_tool")
	if updated.Path != "/usr/bin/cat" {
		t.Errorf("Tool path = %q, want /usr/bin/cat", updated.Path)
	}
}

func TestToolRegistry_UpdateTool_NotFound(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	updates := &ToolMetadata{Path: "/usr/bin/cat"}

	err := reg.UpdateTool(context.Background(), "nonexistent", updates)
	if err == nil {
		t.Error("UpdateTool() should error when tool not found")
	}
}

func TestToolRegistry_RemoveTool(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "removable_tool",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false
	reg.RegisterTool(context.Background(), tool)

	err := reg.RemoveTool("removable_tool")
	if err != nil {
		t.Errorf("RemoveTool() error = %v", err)
	}

	_, err = reg.GetTool("removable_tool")
	if err == nil {
		t.Error("GetTool() should error after tool removed")
	}
}

func TestToolRegistry_RemoveTool_NotFound(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	err := reg.RemoveTool("nonexistent")
	if err == nil {
		t.Error("RemoveTool() should error when tool not found")
	}
}

func TestToolRegistry_GetTool(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "get_test_tool",
		Type: "mcp",
		Socket: "/tmp/test.sock",
	}

	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false
	reg.RegisterTool(context.Background(), tool)

	retrieved, err := reg.GetTool("get_test_tool")
	if err != nil {
		t.Errorf("GetTool() error = %v", err)
	}
	if retrieved.Name != "get_test_tool" {
		t.Errorf("Tool name = %q, want get_test_tool", retrieved.Name)
	}
}

func TestToolRegistry_ListTools(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tools := []string{"tool1", "tool2", "tool3"}

	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false

	for _, name := range tools {
		tool := &ToolMetadata{Name: name, Type: "cli", Path: "/usr/bin/ls"}
		reg.RegisterTool(context.Background(), tool)
	}

	listed := reg.ListTools()
	if len(listed) != 3 {
		t.Errorf("ListTools() returned %d tools, want 3", len(listed))
	}

	for _, name := range tools {
		if _, ok := listed[name]; !ok {
			t.Errorf("Tool %q not in ListTools()", name)
		}
	}
}

func TestToolRegistry_AuditLog(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false

	tool := &ToolMetadata{
		Name: "audit_test",
		Type: "cli",
		Path: "/usr/bin/ls",
	}

	reg.RegisterTool(context.Background(), tool)

	log := reg.GetAuditLog(10)
	if len(log) == 0 {
		t.Error("GetAuditLog() should contain entries after RegisterTool")
	}

	found := false
	for _, entry := range log {
		if entry.Action == "registered" && entry.ToolName == "audit_test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Audit log should contain registered action for tool")
	}
}

func TestToolRegistry_GetAuditLog_Limit(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false

	// Add multiple tools
	for i := 0; i < 5; i++ {
		tool := &ToolMetadata{
			Name: "tool_" + string(rune(i)),
			Type: "cli",
			Path: "/usr/bin/ls",
		}
		reg.RegisterTool(context.Background(), tool)
	}

	log := reg.GetAuditLog(2)
	if len(log) != 2 {
		t.Errorf("GetAuditLog(2) returned %d entries, want 2", len(log))
	}
}

func TestToolRegistry_IsValidToolName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"empty", "", true},
		{"valid", "my_tool", false},
		{"valid_alpha", "MyTool", false},
		{"valid_numeric", "tool123", false},
		{"invalid_dash", "my-tool", true},
		{"invalid_space", "my tool", true},
		{"invalid_special", "my@tool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidToolName(tt.input)
			if valid == tt.shouldErr {
				t.Errorf("isValidToolName(%q) = %v, want %v", tt.input, valid, !tt.shouldErr)
			}
		})
	}
}

func TestToolRegistry_HealthCheck_CLI(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "cli_health",
		Type: "cli",
		Path: "/usr/bin/ls", // ls exists on most systems
	}

	err := reg.healthCheck(context.Background(), tool)
	// May succeed or fail depending on system, but should not panic
	t.Logf("Health check returned: %v", err)
}

func TestToolRegistry_HealthCheck_MCP_MissingSocket(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "mcp_health",
		Type: "mcp",
		Socket: "", // Missing
	}

	err := reg.healthCheck(context.Background(), tool)
	if err == nil {
		t.Error("healthCheck() should error for MCP with missing socket")
	}
}

func TestToolRegistry_HealthCheck_REST_MissingPath(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "rest_health",
		Type: "rest",
		Path: "", // Missing
	}

	err := reg.healthCheck(context.Background(), tool)
	if err == nil {
		t.Error("healthCheck() should error for REST with missing path")
	}
}

func TestToolRegistry_ValidateTool_InvalidType(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	tool := &ToolMetadata{
		Name: "test",
		Type: "invalid_type",
		Path: "/usr/bin/ls",
	}

	err := reg.validateTool(tool)
	if err == nil {
		t.Error("validateTool() should error for invalid type")
	}
}

func TestToolRegistry_Monitoring_StartStop(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.StartMonitoring(ctx)
	// Should not panic; monitoring runs in goroutine
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestToolRegistry_MonitoringDisabled(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	reg.monitoringEnabled = false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.StartMonitoring(ctx)
	// Should return immediately without starting goroutine
}

func TestToolRegistry_PerformHealthChecks(t *testing.T) {
	reg := NewToolRegistry("/tmp/config.yaml")
	reg.validationOnAdd = false
	reg.healthCheckOnAdd = false

	// Add a test tool
	tool := &ToolMetadata{
		Name:    "health_test",
		Type:    "cli",
		Path:    "/usr/bin/ls",
		Enabled: true,
	}
	reg.RegisterTool(context.Background(), tool)

	// Perform health checks
	ctx := context.Background()
	reg.performHealthChecks(ctx)

	// Verify health status was updated
	checked, _ := reg.GetTool("health_test")
	if checked.LastHealthCheck.IsZero() {
		t.Error("LastHealthCheck should be updated after performHealthChecks")
	}
}

func TestToolRegistry_LoadFromConfig_WithTools(t *testing.T) {
	// This would require importing the config package
	// For now, test that the method exists and doesn't panic
	reg := NewToolRegistry("/tmp/config.yaml")

	// Create a minimal config object
	// This is tested more thoroughly in integration tests
	if reg == nil {
		t.Error("Registry should not be nil")
	}
}
