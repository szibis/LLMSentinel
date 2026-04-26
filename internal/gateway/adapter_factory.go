package gateway

import (
	"context"
	"fmt"
	"sync"

	"claude-escalate/internal/config"
)

// AdapterFactory creates and manages tool adapters
type AdapterFactory struct {
	adapters map[string]ToolAdapter
	mu       sync.RWMutex
}

// NewAdapterFactory creates a new adapter factory
func NewAdapterFactory() *AdapterFactory {
	return &AdapterFactory{
		adapters: make(map[string]ToolAdapter),
	}
}

// CreateFromConfig creates adapters based on configuration
func (af *AdapterFactory) CreateFromConfig(cfg *config.Config) error {
	af.mu.Lock()
	defer af.mu.Unlock()

	// Clear existing adapters
	af.adapters = make(map[string]ToolAdapter)

	// Create MCP adapters if enabled
	if cfg.Optimizations.MCP.Enabled {
		for _, tool := range cfg.Optimizations.MCP.Tools {
			switch tool.Type {
			case "web_scraping":
				adapter, err := NewMCPAdapter("scrapling", tool.Settings)
				if err == nil {
					af.adapters[tool.Name] = adapter
				}
			case "code_analysis":
				adapter, err := NewMCPAdapter("lsp", tool.Settings)
				if err == nil {
					af.adapters[tool.Name] = adapter
				}
			case "database":
				adapter, err := NewMCPAdapter("database", tool.Settings)
				if err == nil {
					af.adapters[tool.Name] = adapter
				}
			}
		}
	}

	// Create CLI adapter for shell commands
	cliAdapter := NewCLIAdapter()
	af.adapters["cli"] = cliAdapter

	// Create REST adapter for HTTP calls
	restAdapter := NewRESTAdapter()
	af.adapters["rest"] = restAdapter

	return nil
}

// GetAdapter returns an adapter by name
func (af *AdapterFactory) GetAdapter(name string) (ToolAdapter, error) {
	af.mu.RLock()
	defer af.mu.RUnlock()

	if adapter, exists := af.adapters[name]; exists {
		return adapter, nil
	}

	return nil, fmt.Errorf("adapter not found: %s", name)
}

// RegisterAdapter registers a custom adapter
func (af *AdapterFactory) RegisterAdapter(name string, adapter ToolAdapter) error {
	af.mu.Lock()
	defer af.mu.Unlock()

	if _, exists := af.adapters[name]; exists {
		return fmt.Errorf("adapter already registered: %s", name)
	}

	af.adapters[name] = adapter
	return nil
}

// UnregisterAdapter removes an adapter
func (af *AdapterFactory) UnregisterAdapter(name string) {
	af.mu.Lock()
	defer af.mu.Unlock()

	delete(af.adapters, name)
}

// GetAllAdapters returns all registered adapters
func (af *AdapterFactory) GetAllAdapters() map[string]ToolAdapter {
	af.mu.RLock()
	defer af.mu.RUnlock()

	result := make(map[string]ToolAdapter)
	for name, adapter := range af.adapters {
		result[name] = adapter
	}

	return result
}

// Close closes all adapters
func (af *AdapterFactory) Close() error {
	af.mu.Lock()
	defer af.mu.Unlock()

	for name, adapter := range af.adapters {
		if err := adapter.Close(); err != nil {
			return fmt.Errorf("error closing adapter %s: %w", name, err)
		}
	}

	af.adapters = make(map[string]ToolAdapter)
	return nil
}

// HealthCheck checks health of all adapters
func (af *AdapterFactory) HealthCheck() map[string]error {
	af.mu.RLock()
	defer af.mu.RUnlock()

	results := make(map[string]error)
	for name, adapter := range af.adapters {
		// Use background context for health check
		ctx := context.Background()
		results[name] = adapter.Health(ctx)
	}

	return results
}
