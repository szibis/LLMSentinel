package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MCPAdapter implements ToolAdapter for MCP (Model Context Protocol) tools
type MCPAdapter struct {
	name      string
	protocol  string
	settings  map[string]interface{}
	connected bool
	signature *ToolSignature
}

// NewMCPAdapter creates a new MCP adapter
func NewMCPAdapter(protocol string, settings map[string]interface{}) (*MCPAdapter, error) {
	adapter := &MCPAdapter{
		name:     fmt.Sprintf("mcp-%s", protocol),
		protocol: protocol,
		settings: settings,
	}

	// Initialize signature based on protocol
	adapter.initializeSignature()

	return adapter, nil
}

// Name returns the adapter name
func (a *MCPAdapter) Name() string {
	return a.name
}

// Type returns the tool type
func (a *MCPAdapter) Type() ToolType {
	return ToolTypeMCP
}

// Execute executes the MCP tool
func (a *MCPAdapter) Execute(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
	if !a.connected {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   "MCP adapter not connected",
		}, nil
	}

	start := time.Now()

	// Route to appropriate protocol handler
	var data interface{}
	var execErr error

	switch a.protocol {
	case "scrapling":
		data, execErr = a.executeScraping(ctx, req.Params)
	case "lsp":
		data, execErr = a.executeLSP(ctx, req.Params)
	case "database":
		data, execErr = a.executeDatabase(ctx, req.Params)
	default:
		execErr = fmt.Errorf("unknown protocol: %s", a.protocol)
	}

	if execErr != nil {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   execErr.Error(),
			Timing: ResponseTiming{
				TotalMs: time.Since(start).Milliseconds(),
			},
		}, nil
	}

	return &ToolResponse{
		ID:      req.ID,
		Success: true,
		Data:    data,
		Timing: ResponseTiming{
			TotalMs:    time.Since(start).Milliseconds(),
			ToolExecMs: time.Since(start).Milliseconds(),
		},
	}, nil
}

// GetSignature returns the tool signature
func (a *MCPAdapter) GetSignature() *ToolSignature {
	return a.signature
}

// Health checks if the MCP tool is accessible
func (a *MCPAdapter) Health(ctx context.Context) error {
	// For now, just check if settings exist
	if a.settings == nil {
		return fmt.Errorf("MCP adapter %s: no settings configured", a.name)
	}
	a.connected = true
	return nil
}

// Close closes the MCP connection
func (a *MCPAdapter) Close() error {
	a.connected = false
	return nil
}

// executeScraping handles web scraping via scrapling
func (a *MCPAdapter) executeScraping(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Extract parameters
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter required for scraping")
	}

	// Extract optional CSS selector
	cssSelector := ""
	if selector, ok := params["css_selector"].(string); ok {
		cssSelector = selector
	}

	// In production, this would call the actual scrapling MCP
	// For now, return mock response with metadata
	result := map[string]interface{}{
		"url":          url,
		"title":        "Mock Scraped Content",
		"content":      "This is mock content from " + url,
		"css_selector": cssSelector,
		"tokens_saved": 85, // Approximate token savings
	}

	return result, nil
}

// executeLSP handles code analysis via LSP
func (a *MCPAdapter) executeLSP(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Extract parameters
	file, ok := params["file"].(string)
	if !ok {
		return nil, fmt.Errorf("file parameter required for LSP")
	}

	action, ok := params["action"].(string)
	if !ok {
		action = "symbols"
	}

	// In production, this would call the actual LSP server
	// For now, return mock response
	result := map[string]interface{}{
		"file":   file,
		"action": action,
		"symbols": []map[string]interface{}{
			{
				"name": "exampleFunction",
				"type": "function",
				"line": 42,
			},
		},
		"tokens_saved": 1200, // LSP is much more efficient than grep
	}

	return result, nil
}

// executeDatabase handles database queries
func (a *MCPAdapter) executeDatabase(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Extract parameters
	query, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter required for database")
	}

	// In production, this would execute actual database queries
	// For now, return mock response
	result := map[string]interface{}{
		"query":  query,
		"rows":   0,
		"status": "success",
	}

	return result, nil
}

// initializeSignature sets up the tool signature
func (a *MCPAdapter) initializeSignature() {
	switch a.protocol {
	case "scrapling":
		a.signature = &ToolSignature{
			Name:        "scrapling",
			Type:        ToolTypeMCP,
			Description: "Web scraping tool with CSS selector support",
			Parameters: map[string]*ParamSchema{
				"url": {
					Type:        "string",
					Description: "URL to scrape",
					Required:    true,
				},
				"css_selector": {
					Type:        "string",
					Description: "CSS selector to extract specific content",
					Required:    false,
				},
				"markdown_only": {
					Type:        "boolean",
					Description: "Extract as markdown only (token efficient)",
					Required:    false,
					Default:     true,
				},
			},
			Returns: &ParamSchema{
				Type:        "object",
				Description: "Scraped content with metadata",
			},
		}

	case "lsp":
		a.signature = &ToolSignature{
			Name:        "lsp",
			Type:        ToolTypeMCP,
			Description: "Language Server Protocol for code analysis",
			Parameters: map[string]*ParamSchema{
				"file": {
					Type:        "string",
					Description: "File path to analyze",
					Required:    true,
				},
				"action": {
					Type:        "string",
					Description: "Action to perform (symbols, hover, definition, etc)",
					Required:    false,
					Default:     "symbols",
				},
			},
			Returns: &ParamSchema{
				Type:        "object",
				Description: "Code analysis results",
			},
		}

	case "database":
		a.signature = &ToolSignature{
			Name:        "database",
			Type:        ToolTypeMCP,
			Description: "Database query executor",
			Parameters: map[string]*ParamSchema{
				"query": {
					Type:        "string",
					Description: "SQL query to execute",
					Required:    true,
				},
				"prepared": {
					Type:        "boolean",
					Description: "Use prepared statement (recommended for security)",
					Required:    false,
					Default:     true,
				},
			},
			Returns: &ParamSchema{
				Type:        "object",
				Description: "Query results",
			},
		}
	}
}

// MarshalJSON marshals the adapter to JSON
func (a *MCPAdapter) MarshalJSON() ([]byte, error) {
	data := map[string]interface{}{
		"name":      a.Name(),
		"type":      a.Type(),
		"protocol":  a.protocol,
		"signature": a.signature,
		"connected": a.connected,
	}
	return json.Marshal(data)
}
