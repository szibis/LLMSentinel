package tools

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"gopkg.in/yaml.v3"
)

// ToolRegistry manages tool discovery, registration, and lifecycle
type ToolRegistry struct {
	mu                 sync.RWMutex
	tools              map[string]*ToolMetadata
	configPath         string
	monitoringInterval time.Duration
	monitoringEnabled  bool
	validationOnAdd    bool
	healthCheckOnAdd   bool
	cascadeCleanup     bool
	backupOnRemove     bool
	auditTrail         bool
	auditLog           []AuditEntry
}

// ToolMetadata tracks tool information and status
type ToolMetadata struct {
	Name              string                 `yaml:"name"`
	Type              string                 `yaml:"type"`
	Path              string                 `yaml:"path,omitempty"`
	Socket            string                 `yaml:"socket,omitempty"`
	Settings          map[string]interface{} `yaml:"settings,omitempty"`
	RegisteredAt      time.Time              `yaml:"registered_at"`
	LastHealthCheck   time.Time              `yaml:"last_health_check,omitempty"`
	HealthStatus      string                 `yaml:"health_status"` // healthy, unhealthy, unknown
	ConsecutiveErrors int                    `yaml:"consecutive_errors"`
	Enabled           bool                   `yaml:"enabled"`
}

// AuditEntry tracks tool lifecycle events
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"` // registered, updated, removed, health_check, error
	ToolName  string    `json:"tool_name"`
	Details   string    `json:"details,omitempty"`
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry(configPath string) *ToolRegistry {
	return &ToolRegistry{
		tools:              make(map[string]*ToolMetadata),
		configPath:         configPath,
		monitoringInterval: 5 * time.Minute,
		monitoringEnabled:  true,
		validationOnAdd:    true,
		healthCheckOnAdd:   true,
		cascadeCleanup:     true,
		backupOnRemove:     true,
		auditTrail:         true,
		auditLog:           make([]AuditEntry, 0),
	}
}

// RegisterTool adds a tool to the registry with validation
func (tr *ToolRegistry) RegisterTool(ctx context.Context, tool *ToolMetadata) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}

	if tool.Type == "" {
		return fmt.Errorf("tool type is required")
	}

	// Check if tool already exists
	if _, exists := tr.tools[tool.Name]; exists {
		return fmt.Errorf("tool already registered: %s", tool.Name)
	}

	// Validate tool configuration
	if tr.validationOnAdd {
		if err := tr.validateTool(tool); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Set initial values
	tool.RegisteredAt = time.Now()
	tool.HealthStatus = "unknown"
	tool.Enabled = true
	tool.ConsecutiveErrors = 0

	// Health check on registration
	if tr.healthCheckOnAdd {
		healthErr := tr.healthCheck(ctx, tool)
		if healthErr != nil {
			tool.HealthStatus = "unhealthy"
			tool.ConsecutiveErrors = 1
			// Continue anyway, tool will be marked unhealthy
		} else {
			tool.HealthStatus = "healthy"
		}
		tool.LastHealthCheck = time.Now()
	}

	// Add to registry
	tr.tools[tool.Name] = tool

	// Log audit
	tr.logAudit(AuditEntry{
		Timestamp: time.Now(),
		Action:    "registered",
		ToolName:  tool.Name,
		Details:   fmt.Sprintf("type=%s, status=%s", tool.Type, tool.HealthStatus),
	})

	return nil
}

// UpdateTool modifies an existing tool configuration
func (tr *ToolRegistry) UpdateTool(ctx context.Context, name string, updates *ToolMetadata) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tool, exists := tr.tools[name]
	if !exists {
		return fmt.Errorf("tool not found: %s", name)
	}

	// Update fields
	if updates.Path != "" {
		tool.Path = updates.Path
	}
	if updates.Socket != "" {
		tool.Socket = updates.Socket
	}
	if len(updates.Settings) > 0 {
		if tool.Settings == nil {
			tool.Settings = make(map[string]interface{})
		}
		for k, v := range updates.Settings {
			tool.Settings[k] = v
		}
	}

	// Validate updated config
	if tr.validationOnAdd {
		if err := tr.validateTool(tool); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Log audit
	tr.logAudit(AuditEntry{
		Timestamp: time.Now(),
		Action:    "updated",
		ToolName:  name,
		Details:   "configuration changed",
	})

	return nil
}

// RemoveTool removes a tool from the registry
func (tr *ToolRegistry) RemoveTool(name string) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[name]; !exists {
		return fmt.Errorf("tool not found: %s", name)
	}

	// Backup before removal if enabled
	if tr.backupOnRemove {
		tr.backupConfig()
	}

	// TODO: Cascade cleanup - remove from all selectors and configurations if enabled

	// Remove from registry
	delete(tr.tools, name)

	// Log audit
	tr.logAudit(AuditEntry{
		Timestamp: time.Now(),
		Action:    "removed",
		ToolName:  name,
		Details:   "tool unregistered",
	})

	return nil
}

// GetTool retrieves tool metadata by name
func (tr *ToolRegistry) GetTool(name string) (*ToolMetadata, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, exists := tr.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

// ListTools returns all registered tools
func (tr *ToolRegistry) ListTools() map[string]*ToolMetadata {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	result := make(map[string]*ToolMetadata)
	for name, tool := range tr.tools {
		result[name] = tool
	}
	return result
}

// StartMonitoring begins periodic health checks
func (tr *ToolRegistry) StartMonitoring(ctx context.Context) {
	if !tr.monitoringEnabled {
		return
	}

	ticker := time.NewTicker(tr.monitoringInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tr.performHealthChecks(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// performHealthChecks checks health of all tools
func (tr *ToolRegistry) performHealthChecks(ctx context.Context) {
	tr.mu.Lock()
	toolsCopy := make([]*ToolMetadata, 0, len(tr.tools))
	for _, tool := range tr.tools {
		toolsCopy = append(toolsCopy, tool)
	}
	tr.mu.Unlock()

	for _, tool := range toolsCopy {
		err := tr.healthCheck(ctx, tool)

		tr.mu.Lock()
		if err != nil {
			tool.HealthStatus = "unhealthy"
			tool.ConsecutiveErrors++

			// Alert if threshold exceeded
			if tool.ConsecutiveErrors >= 5 {
				fmt.Printf("⚠️ Alert: Tool %s unhealthy for %d checks\n", tool.Name, tool.ConsecutiveErrors)
			}

			tr.logAudit(AuditEntry{
				Timestamp: time.Now(),
				Action:    "error",
				ToolName:  tool.Name,
				Details:   fmt.Sprintf("health check failed: %v", err),
			})
		} else {
			tool.HealthStatus = "healthy"
			tool.ConsecutiveErrors = 0

			tr.logAudit(AuditEntry{
				Timestamp: time.Now(),
				Action:    "health_check",
				ToolName:  tool.Name,
				Details:   "healthy",
			})
		}
		tool.LastHealthCheck = time.Now()
		tr.mu.Unlock()
	}
}

// healthCheck tests if a tool is healthy
func (tr *ToolRegistry) healthCheck(_ context.Context, tool *ToolMetadata) error {
	// TODO: Implement per-type health checking using adapters
	// For now, basic path validation
	switch tool.Type {
	case "cli":
		if tool.Path == "" {
			return fmt.Errorf("CLI tool missing path")
		}
		if _, err := os.Stat(tool.Path); err != nil {
			return fmt.Errorf("CLI tool path not found: %v", err)
		}
	case "mcp":
		if tool.Socket == "" {
			return fmt.Errorf("MCP tool missing socket")
		}
		// TODO: Implement MCP socket health check
	case "rest":
		if tool.Path == "" {
			return fmt.Errorf("REST tool missing base URL")
		}
		// TODO: Implement REST health check
	default:
		return fmt.Errorf("unsupported tool type: %s", tool.Type)
	}

	return nil
}

// validateTool validates tool configuration
func (tr *ToolRegistry) validateTool(tool *ToolMetadata) error {
	// Validate tool name
	if !isValidToolName(tool.Name) {
		return fmt.Errorf("invalid tool name: must be alphanumeric with underscores")
	}

	// Validate tool type
	validTypes := []string{"cli", "mcp", "rest", "database", "binary"}
	typeValid := false
	for _, t := range validTypes {
		if tool.Type == t {
			typeValid = true
			break
		}
	}
	if !typeValid {
		return fmt.Errorf("invalid tool type: %s", tool.Type)
	}

	// Type-specific validation
	switch tool.Type {
	case "cli":
		if tool.Path == "" {
			return fmt.Errorf("CLI tool requires path")
		}
	case "mcp":
		if tool.Socket == "" {
			return fmt.Errorf("MCP tool requires socket")
		}
	case "rest":
		if tool.Path == "" {
			return fmt.Errorf("REST tool requires base_url")
		}
	}

	return nil
}

// SaveToConfig persists tools to config file
func (tr *ToolRegistry) SaveToConfig(cfg *config.Config) error {
	tr.mu.RLock()
	tools := make([]config.MCPTool, 0, len(tr.tools))
	for _, tool := range tr.tools {
		if !tool.Enabled {
			continue
		}
		tools = append(tools, config.MCPTool{
			Name:     tool.Name,
			Type:     tool.Type,
			Settings: tool.Settings,
		})
	}
	tr.mu.RUnlock()

	cfg.Tools = tools

	// Write to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Backup existing config if exists
	if _, err := os.Stat(tr.configPath); err == nil {
		backupPath := tr.configPath + ".backup"
		if err := os.Rename(tr.configPath, backupPath); err != nil {
			fmt.Printf("Warning: failed to backup config: %v\n", err)
		}
	}

	if err := os.WriteFile(tr.configPath, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// LoadFromConfig loads tools from config file
func (tr *ToolRegistry) LoadFromConfig(cfg *config.Config) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	for _, tool := range cfg.Tools {
		tr.tools[tool.Name] = &ToolMetadata{
			Name:     tool.Name,
			Type:     tool.Type,
			Settings: tool.Settings,
			Enabled:  true,
		}
	}

	return nil
}

// backupConfig creates a backup of the config file
func (tr *ToolRegistry) backupConfig() {
	if _, err := os.Stat(tr.configPath); err == nil {
		backupPath := tr.configPath + ".backup." + time.Now().Format("20060102150405")
		_ = os.Rename(tr.configPath, backupPath)
	}
}

// logAudit records an audit entry
func (tr *ToolRegistry) logAudit(entry AuditEntry) {
	tr.auditLog = append(tr.auditLog, entry)

	// Keep last 1000 entries
	if len(tr.auditLog) > 1000 {
		tr.auditLog = tr.auditLog[len(tr.auditLog)-1000:]
	}

	if tr.auditTrail {
		fmt.Printf("[AUDIT] %s: %s - %s (%s)\n",
			entry.Timestamp.Format("15:04:05"),
			entry.Action,
			entry.ToolName,
			entry.Details)
	}
}

// GetAuditLog returns recent audit entries
func (tr *ToolRegistry) GetAuditLog(limit int) []AuditEntry {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if limit > len(tr.auditLog) {
		limit = len(tr.auditLog)
	}

	result := make([]AuditEntry, limit)
	copy(result, tr.auditLog[len(tr.auditLog)-limit:])
	return result
}

// isValidToolName checks if tool name is valid
func isValidToolName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}
