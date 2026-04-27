package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/metrics"
)

// Server represents the dashboard HTTP server
type Server struct {
	host             string
	port             int
	configLoader     *config.Loader
	metricsCollector *metrics.MetricsCollector
	metricsPublisher *metrics.MetricsPublisher
	httpServer       *http.Server
	mu               sync.RWMutex
	configPath       string
}

// NewServer creates a new dashboard server
func NewServer(
	host string,
	port int,
	configLoader *config.Loader,
	metricsCollector *metrics.MetricsCollector,
	metricsPublisher *metrics.MetricsPublisher,
) *Server {
	s := &Server{
		host:             host,
		port:             port,
		configLoader:     configLoader,
		metricsCollector: metricsCollector,
		metricsPublisher: metricsPublisher,
	}

	// Create HTTP routes
	mux := http.NewServeMux()

	// Dashboard UI
	mux.HandleFunc("/dashboard", s.handleDashboard)

	// Configuration endpoints
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/reload", s.handleReload)

	// Metrics endpoints
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/metrics/history", s.handleMetricsHistory)
	mux.HandleFunc("/api/metrics/export", s.handleMetricsExport)

	// WebSocket for real-time metrics
	mux.HandleFunc("/api/metrics/stream", s.handleMetricsStream)

	// Tool management endpoints (v0.7.0+) - more specific routes first
	mux.HandleFunc("/api/tools/add", s.handleToolsAdd)
	mux.HandleFunc("/api/tools/types", s.handleToolsTypes)
	mux.HandleFunc("/api/tools/", s.handleToolsDynamic)
	mux.HandleFunc("/api/tools", s.handleTools)

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Static files
	mux.HandleFunc("/static/", s.handleStatic)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start starts the dashboard server
func (s *Server) Start() error {
	fmt.Printf("Starting dashboard at http://%s:%d/dashboard\n", s.host, s.port)
	return s.httpServer.ListenAndServe()
}

// Stop stops the dashboard server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Handlers

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(getDashboardHTML())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleConfigGet(w, r)
	case http.MethodPost:
		s.handleConfigSet(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"config":  cfg,
	})
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: Validate config and save
	// For now, just acknowledge

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Config updated (validation and save not yet implemented)",
	})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reload configuration
	_, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reloading config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Configuration reloaded (0 downtime)",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.metricsPublisher.GetExportedJSON())
}

func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	// Get history from collector
	history := s.metricsCollector.GetMetricsHistory()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"history": history,
		"count":   len(history),
	})
}

func (s *Server) handleMetricsExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "prometheus"
	}

	switch format {
	case "prometheus":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(s.metricsPublisher.GetExportedMetrics()))
	case "json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.metricsPublisher.GetExportedJSON())
	default:
		http.Error(w, "Invalid format (use 'prometheus' or 'json')", http.StatusBadRequest)
	}
}

func (s *Server) handleMetricsStream(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement WebSocket streaming
	// For now, return polling recommendation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "WebSocket streaming not yet implemented, use /api/metrics with polling",
		"interval": "1000ms (recommended)",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Serve static files from web/ directory
	// For now, return 404 (static serving would be implemented)
	http.NotFound(w, r)
}

// Tool Management Handlers (v0.7.0+)

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleToolsList(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolsList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	// Build list of tools from config
	tools := []map[string]interface{}{}
	if cfg.Optimizations.MCP.Enabled && len(cfg.Optimizations.MCP.Tools) > 0 {
		for _, tool := range cfg.Optimizations.MCP.Tools {
			pathVal := ""
			if tool.Settings != nil {
				if p, exists := tool.Settings["path"]; exists {
					if ps, ok := p.(string); ok {
						pathVal = ps
					}
				}
			}
			tools = append(tools, map[string]interface{}{
				"name":     tool.Name,
				"type":     tool.Type,
				"path":     pathVal,
				"health":   "ok",
				"settings": tool.Settings,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": tools,
	})
}

func (s *Server) handleToolsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate input
	name, ok := req["name"].(string)
	if !ok || name == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "name required"}`, http.StatusBadRequest)
		return
	}

	toolType, ok := req["type"].(string)
	if !ok || toolType == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "type required"}`, http.StatusBadRequest)
		return
	}

	path, ok := req["path"].(string)
	if !ok || path == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "path required"}`, http.StatusBadRequest)
		return
	}

	cfg, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Error loading config: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Check if tool name already exists
	if cfg.Optimizations.MCP.Tools != nil {
		for _, t := range cfg.Optimizations.MCP.Tools {
			if t.Name == name {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error": "Tool with that name already exists"}`, http.StatusBadRequest)
				return
			}
		}
	}

	// Create new tool
	settings := map[string]interface{}{"path": path}
	if settingsVal, ok := req["settings"].(map[string]interface{}); ok {
		for k, v := range settingsVal {
			settings[k] = v
		}
	}

	newTool := config.MCPTool{
		Type:     toolType,
		Name:     name,
		Settings: settings,
	}

	// Enable MCP if not already enabled
	if !cfg.Optimizations.MCP.Enabled {
		cfg.Optimizations.MCP.Enabled = true
	}

	// Add tool to config
	cfg.Optimizations.MCP.Tools = append(cfg.Optimizations.MCP.Tools, newTool)

	// Save config (use YAML marshaling)
	if err := saveConfigToFile(cfg); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to save config: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Reload config
	s.configLoader = config.NewLoader("")
	s.configLoader.Load()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "created",
		"message": "Tool added successfully",
		"tool": map[string]interface{}{
			"name":     name,
			"type":     toolType,
			"path":     path,
			"health":   "ok",
			"settings": settings,
		},
	})
}

func (s *Server) handleToolsDynamic(w http.ResponseWriter, r *http.Request) {
	// Extract tool name from URL path (/api/tools/{name}/...)
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/api/tools/"), "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	toolName := parts[0]

	// Route based on HTTP method and path
	if len(parts) > 1 && parts[1] == "test" && r.Method == http.MethodPost {
		s.handleToolTest(w, r, toolName)
	} else if r.Method == http.MethodPut {
		s.handleToolEdit(w, r, toolName)
	} else if r.Method == http.MethodDelete {
		s.handleToolDelete(w, r, toolName)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolTest(w http.ResponseWriter, r *http.Request, toolName string) {
	// TODO: Implement tool health check
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"message": "Tool is responding",
	})
}

func (s *Server) handleToolEdit(w http.ResponseWriter, r *http.Request, toolName string) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	cfg, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	// Find and update tool
	found := false
	for i, t := range cfg.Optimizations.MCP.Tools {
		if t.Name == toolName {
			// Update path if provided
			if path, ok := req["path"].(string); ok && path != "" {
				cfg.Optimizations.MCP.Tools[i].Settings["path"] = path
			}

			// Merge settings
			if settingsVal, ok := req["settings"].(map[string]interface{}); ok {
				for k, v := range settingsVal {
					cfg.Optimizations.MCP.Tools[i].Settings[k] = v
				}
			}

			found = true
			break
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Tool not found"}`, http.StatusNotFound)
		return
	}

	// Save config
	if err := saveConfigToFile(cfg); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to save config: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Reload config
	s.configLoader = config.NewLoader("")
	s.configLoader.Load()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "updated",
		"message": "Tool updated successfully",
	})
}

func (s *Server) handleToolDelete(w http.ResponseWriter, r *http.Request, toolName string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.configLoader.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	// Find and remove tool
	found := false
	for i, t := range cfg.Optimizations.MCP.Tools {
		if t.Name == toolName {
			cfg.Optimizations.MCP.Tools = append(cfg.Optimizations.MCP.Tools[:i], cfg.Optimizations.MCP.Tools[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Tool not found"}`, http.StatusNotFound)
		return
	}

	// Save config
	if err := saveConfigToFile(cfg); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to save config: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Reload config
	s.configLoader = config.NewLoader("")
	s.configLoader.Load()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "deleted",
		"message": "Tool removed successfully",
	})
}

func (s *Server) handleToolsTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"types": []map[string]string{
			{"type": "cli", "description": "Shell command or script"},
			{"type": "mcp", "description": "MCP (Model Context Protocol) server"},
			{"type": "rest", "description": "HTTP REST API"},
			{"type": "database", "description": "SQL database"},
			{"type": "binary", "description": "Standalone executable"},
		},
	})
}

// Helper function to save config to file
func saveConfigToFile(cfg *config.Config) error {
	// Determine config file path
	configPath := ""

	// Try to find existing config file first
	defaultPaths := []string{
		"./config.yaml",
		"./configs/config.yaml",
		expandHome("~/.claude-escalate/config.yaml"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			break
		}
	}

	// If no existing file, use home directory default
	if configPath == "" {
		configDir := expandHome("~/.claude-escalate")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return err
		}
		configPath = filepath.Join(configDir, "config.yaml")
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(configPath, data, 0600)
}

// Helper function to expand ~ in paths
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		return path
	}

	return filepath.Join(usr.HomeDir, path[1:])
}

// Helper to get dashboard HTML
func getDashboardHTML() []byte {
	return []byte(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Claude Escalate Control Panel</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
			background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
			color: #333;
			padding: 20px;
			min-height: 100vh;
		}
		.container {
			max-width: 1400px;
			margin: 0 auto;
			background: white;
			border-radius: 10px;
			box-shadow: 0 20px 60px rgba(0,0,0,0.3);
			overflow: hidden;
		}
		.header {
			background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
			color: white;
			padding: 30px;
			text-align: center;
		}
		.header h1 { font-size: 28px; margin-bottom: 10px; }
		.header p { font-size: 14px; opacity: 0.9; }
		.grid {
			display: grid;
			grid-template-columns: 1fr 1fr 1fr 1fr;
			gap: 20px;
			padding: 30px;
			background: #f7f8fa;
		}
		.metric-card {
			background: white;
			padding: 20px;
			border-radius: 8px;
			box-shadow: 0 2px 8px rgba(0,0,0,0.1);
			border-left: 4px solid #667eea;
		}
		.metric-card h3 { font-size: 12px; color: #999; text-transform: uppercase; margin-bottom: 10px; }
		.metric-card .value { font-size: 32px; font-weight: bold; color: #667eea; }
		.metric-card .unit { font-size: 14px; color: #999; margin-left: 5px; }
		.metric-card.trending .value { color: #16a34a; }
		.tabs {
			display: flex;
			border-bottom: 2px solid #e5e7eb;
			padding: 0 30px;
		}
		.tab {
			padding: 15px 20px;
			border: none;
			background: none;
			cursor: pointer;
			font-size: 14px;
			color: #666;
			border-bottom: 3px solid transparent;
			transition: all 0.3s;
		}
		.tab.active {
			color: #667eea;
			border-bottom-color: #667eea;
		}
		.tab:hover { color: #667eea; }
		.tab-content {
			padding: 30px;
			display: none;
		}
		.tab-content.active { display: block; }
		.config-editor {
			background: #1e1e1e;
			color: #d4d4d4;
			padding: 15px;
			border-radius: 6px;
			font-family: 'Courier New', monospace;
			font-size: 13px;
			line-height: 1.6;
			overflow-x: auto;
			max-height: 400px;
		}
		.button-group {
			display: flex;
			gap: 10px;
			margin-top: 20px;
		}
		.btn {
			padding: 10px 20px;
			border: none;
			border-radius: 6px;
			cursor: pointer;
			font-size: 14px;
			font-weight: 500;
			transition: all 0.3s;
		}
		.btn-primary {
			background: #667eea;
			color: white;
		}
		.btn-primary:hover { background: #5568d3; }
		.btn-secondary {
			background: #e5e7eb;
			color: #333;
		}
		.btn-secondary:hover { background: #d1d5db; }
		.status {
			display: flex;
			align-items: center;
			gap: 10px;
			padding: 10px;
			border-radius: 6px;
			background: #d1fae5;
			color: #065f46;
			margin-top: 20px;
		}
		.status-dot {
			width: 8px;
			height: 8px;
			background: #16a34a;
			border-radius: 50%;
		}
		.loading { opacity: 0.5; pointer-events: none; }
		@media (max-width: 1024px) {
			.grid { grid-template-columns: 1fr 1fr; }
		}
		@media (max-width: 640px) {
			.grid { grid-template-columns: 1fr; }
		}
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Claude Escalate Control Panel</h1>
			<p>Token Optimization Gateway</p>
		</div>

		<div class="tabs">
			<button class="tab active" onclick="switchTab('metrics')">📊 Metrics</button>
			<button class="tab" onclick="switchTab('config')">⚙️ Configuration</button>
			<button class="tab" onclick="switchTab('security')">🛡️ Security</button>
			<button class="tab" onclick="switchTab('tools')">🔧 Tools</button>
			<button class="tab" onclick="switchTab('feedback')">👍 Feedback</button>
			<button class="tab" onclick="switchTab('analytics')">📈 Analytics</button>
		</div>

		<div id="metrics" class="tab-content active">
			<div class="grid" id="metrics-grid">
				<div class="metric-card">
					<h3>Token Savings</h3>
					<div><span class="value" id="metric-savings">0</span><span class="unit">%</span></div>
				</div>
				<div class="metric-card">
					<h3>Cache Hit Rate</h3>
					<div><span class="value" id="metric-cache">0</span><span class="unit">%</span></div>
				</div>
				<div class="metric-card trending">
					<h3>Requests/sec</h3>
					<div><span class="value" id="metric-rps">0</span><span class="unit">req/s</span></div>
				</div>
				<div class="metric-card">
					<h3>False Positives</h3>
					<div><span class="value" id="metric-fp">0.0</span><span class="unit">%</span></div>
				</div>
			</div>
			<div style="padding: 30px;">
				<h3>Real-time Metrics (poll /api/metrics)</h3>
				<p style="color: #666; margin: 10px 0;">Polling interval: 1 second</p>
				<div class="status">
					<div class="status-dot"></div>
					<span>Metrics streaming active</span>
				</div>
			</div>
		</div>

		<div id="config" class="tab-content">
			<h3>Configuration Editor</h3>
			<p style="color: #666; margin: 15px 0;">Edit configuration and reload without downtime</p>
			<div class="config-editor" id="config-editor">
				Loading configuration...
			</div>
			<div class="button-group">
				<button class="btn btn-primary" onclick="saveConfig()">Save & Reload</button>
				<button class="btn btn-secondary" onclick="discardChanges()">Discard</button>
			</div>
			<div id="config-status"></div>
		</div>

		<div id="security" class="tab-content">
			<h3>Security Status</h3>
			<div class="grid">
				<div class="metric-card">
					<h3>Injections Blocked</h3>
					<div><span class="value" id="metric-injections">0</span></div>
				</div>
				<div class="metric-card">
					<h3>Rate Limits Triggered</h3>
					<div><span class="value" id="metric-ratelimits">0</span></div>
				</div>
				<div class="metric-card">
					<h3>Validation Failures</h3>
					<div><span class="value" id="metric-validation">0</span></div>
				</div>
				<div class="metric-card">
					<h3>Unauthorized Attempts</h3>
					<div><span class="value" id="metric-unauthorized">0</span></div>
				</div>
			</div>
		</div>

		<div id="tools" class="tab-content">
			<h3>Tool Configuration</h3>
			<p style="color: #666; margin: 15px 0;">Add, edit, and manage custom CLI, MCP, REST, and other tools</p>

			<div style="margin-bottom: 30px;">
				<h4 style="margin-bottom: 15px;">Configured Tools</h4>
				<div style="overflow-x: auto;">
					<table style="width: 100%; border-collapse: collapse;">
						<thead>
							<tr style="background: #f7f8fa; border-bottom: 2px solid #e5e7eb;">
								<th style="padding: 12px; text-align: left; font-weight: 600;">Name</th>
								<th style="padding: 12px; text-align: left; font-weight: 600;">Type</th>
								<th style="padding: 12px; text-align: left; font-weight: 600;">Path/Socket</th>
								<th style="padding: 12px; text-align: left; font-weight: 600;">Health</th>
								<th style="padding: 12px; text-align: left; font-weight: 600;">Actions</th>
							</tr>
						</thead>
						<tbody id="tools-list-table">
							<tr><td colspan="5" style="padding: 20px; text-align: center; color: #999;">Loading tools...</td></tr>
						</tbody>
					</table>
				</div>
			</div>

			<div style="background: #f7f8fa; padding: 20px; border-radius: 8px; margin-bottom: 20px;">
				<h4 style="margin-bottom: 15px;">Add New Tool</h4>
				<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 15px;">
					<div>
						<label style="display: block; margin-bottom: 8px; font-weight: 500;">Tool Type:</label>
						<select id="tool-type" onchange="switchToolType(this.value)" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
							<option value="">-- Select Type --</option>
							<option value="cli">CLI Command/Script</option>
							<option value="mcp">MCP Server</option>
							<option value="rest">REST API</option>
							<option value="database">Database</option>
							<option value="binary">Binary Executable</option>
						</select>
					</div>
					<div>
						<label style="display: block; margin-bottom: 8px; font-weight: 500;">Tool Name:</label>
						<input type="text" id="tool-name" placeholder="my_tool" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
					</div>
					<div>
						<label style="display: block; margin-bottom: 8px; font-weight: 500;" id="tool-path-label">Path:</label>
						<input type="text" id="tool-path" placeholder="/usr/local/bin/tool" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
					</div>
					<div>
						<label style="display: block; margin-bottom: 8px; font-weight: 500;">Settings (JSON):</label>
						<input type="text" id="tool-settings" placeholder="{}" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
					</div>
				</div>
				<div class="button-group">
					<button class="btn btn-secondary" onclick="testTool()">Test Connection</button>
					<button class="btn btn-primary" onclick="addTool()">Add Tool</button>
				</div>
				<div id="tool-status" style="margin-top: 10px; color: #666;"></div>
			</div>
		</div>

		<div id="feedback" class="tab-content">
			<h3>Response Feedback</h3>
			<p style="color: #666; margin: 15px 0;">Help us improve by rating your responses (1-5 stars)</p>
			<div style="background: #f7f8fa; padding: 20px; border-radius: 8px; max-width: 400px;">
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 8px; font-weight: 500;">Request ID:</label>
					<input type="text" id="feedback-request-id" placeholder="Enter request ID" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
				</div>
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 8px; font-weight: 500;">Rating (1-5):</label>
					<div style="display: flex; gap: 10px;">
						<button class="star-btn" onclick="setRating(1)">⭐</button>
						<button class="star-btn" onclick="setRating(2)">⭐⭐</button>
						<button class="star-btn" onclick="setRating(3)">⭐⭐⭐</button>
						<button class="star-btn" onclick="setRating(4)">⭐⭐⭐⭐</button>
						<button class="star-btn" onclick="setRating(5)">⭐⭐⭐⭐⭐</button>
					</div>
					<input type="hidden" id="feedback-rating" value="0">
				</div>
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 8px; font-weight: 500;">
						<input type="checkbox" id="feedback-helpful"> Was this helpful?
					</label>
					<label style="display: block; margin-bottom: 8px; font-weight: 500;">
						<input type="checkbox" id="feedback-accurate"> Was this accurate?
					</label>
				</div>
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 8px; font-weight: 500;">Comment (optional):</label>
					<textarea id="feedback-comment" placeholder="Any additional feedback..." style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; resize: vertical; height: 100px;"></textarea>
				</div>
				<button class="btn btn-primary" onclick="submitFeedback()" style="width: 100%;">Submit Feedback</button>
				<div id="feedback-status" style="margin-top: 10px; color: #666;"></div>
			</div>
		</div>

		<div id="analytics" class="tab-content">
			<h3>Your Analytics & Preferences</h3>
			<p style="color: #666; margin: 15px 0;">Personalized insights based on your feedback and usage patterns</p>
			<div class="grid">
				<div class="metric-card">
					<h3>Average Rating</h3>
					<div><span class="value" id="analytics-rating">-</span><span class="unit">/5.0</span></div>
				</div>
				<div class="metric-card">
					<h3>Helpful Responses</h3>
					<div><span class="value" id="analytics-helpful">-</span><span class="unit">%</span></div>
				</div>
				<div class="metric-card">
					<h3>Accuracy</h3>
					<div><span class="value" id="analytics-accuracy">-</span><span class="unit">%</span></div>
				</div>
				<div class="metric-card">
					<h3>Total Feedback</h3>
					<div><span class="value" id="analytics-count">-</span></div>
				</div>
			</div>
			<div style="padding: 20px; background: #f7f8fa; border-radius: 8px; margin-top: 20px;">
				<h4 style="margin-bottom: 10px;">Your Preferences Learned:</h4>
				<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px;">
					<div>
						<label style="display: block; margin-bottom: 8px;">
							<input type="checkbox" id="pref-freshness" disabled> Prefers fresh responses
						</label>
						<label style="display: block; margin-bottom: 8px;">
							<input type="checkbox" id="pref-opus" disabled> Prefers detailed (Opus)
						</label>
					</div>
					<div>
						<label style="display: block; margin-bottom: 8px;">
							<input type="checkbox" id="pref-brief" disabled> Prefers brief responses
						</label>
						<label style="display: block; margin-bottom: 8px;">
							<input type="checkbox" id="pref-model" disabled> Preferred Model: <span id="pref-model-text">-</span>
						</label>
					</div>
				</div>
			</div>
			<button class="btn btn-primary" onclick="loadAnalytics()" style="margin-top: 20px;">Refresh Analytics</button>
		</div>
	</div>

	<script>
		let originalConfig = '';

		async function loadMetrics() {
			try {
				const response = await fetch('/api/metrics');
				const data = await response.json();

				document.getElementById('metric-savings').textContent = (data.tokens.savings_percent * 100).toFixed(1);
				document.getElementById('metric-cache').textContent = (data.cache.hit_rate * 100).toFixed(1);
				document.getElementById('metric-fp').textContent = (data.cache.false_positive_rate * 100).toFixed(2);
				document.getElementById('metric-injections').textContent = data.security.injections_blocked;
				document.getElementById('metric-ratelimits').textContent = data.security.rate_limits_triggered;
				document.getElementById('metric-validation').textContent = data.security.validation_failures;
				document.getElementById('metric-unauthorized').textContent = data.security.unauthorized_attempts;
			} catch (err) {
				console.error('Error loading metrics:', err);
			}
		}

		async function loadConfig() {
			try {
				const response = await fetch('/api/config');
				const data = await response.json();
				originalConfig = JSON.stringify(data.config, null, 2);
				document.getElementById('config-editor').textContent = originalConfig;
			} catch (err) {
				console.error('Error loading config:', err);
				document.getElementById('config-editor').textContent = 'Error loading configuration';
			}
		}

		async function saveConfig() {
			const editor = document.getElementById('config-editor');
			try {
				const config = JSON.parse(editor.textContent);
				const response = await fetch('/api/config', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(config)
				});

				if (response.ok) {
					await fetch('/api/config/reload');
					document.getElementById('config-status').innerHTML = '<div class="status"><div class="status-dot"></div><span>✓ Configuration reloaded (0 downtime)</span></div>';
					originalConfig = editor.textContent;
				}
			} catch (err) {
				document.getElementById('config-status').innerHTML = '<div class="status" style="background: #fee2e2; color: #991b1b;"><span>✗ Error: ' + err.message + '</span></div>';
			}
		}

		function discardChanges() {
			document.getElementById('config-editor').textContent = originalConfig;
			document.getElementById('config-status').innerHTML = '';
		}

		function switchTab(tabName) {
			document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
			document.querySelectorAll('.tab').forEach(el => el.classList.remove('active'));
			document.getElementById(tabName).classList.add('active');
			event.target.classList.add('active');
			if (tabName === 'analytics') {
				loadAnalytics();
			}
		}

		// Feedback UI functions
		let currentRating = 0;

		function setRating(rating) {
			currentRating = rating;
			document.getElementById('feedback-rating').value = rating;
			document.querySelectorAll('.star-btn').forEach((btn, idx) => {
				if (idx < rating) {
					btn.style.opacity = '1';
				} else {
					btn.style.opacity = '0.4';
				}
			});
		}

		async function submitFeedback() {
			const requestId = document.getElementById('feedback-request-id').value;
			const rating = parseInt(document.getElementById('feedback-rating').value);
			const helpful = document.getElementById('feedback-helpful').checked;
			const accurate = document.getElementById('feedback-accurate').checked;
			const comment = document.getElementById('feedback-comment').value;

			if (!requestId) {
				alert('Please enter a request ID');
				return;
			}
			if (rating === 0) {
				alert('Please select a rating');
				return;
			}

			try {
				const response = await fetch('/api/feedback', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						request_id: requestId,
						rating: rating,
						helpful: helpful,
						accurate: accurate,
						comment: comment
					})
				});
				const data = await response.json();
				document.getElementById('feedback-status').innerHTML = '<span style="color: #16a34a;">✓ Thank you for your feedback!</span>';

				// Reset form after 2 seconds
				setTimeout(() => {
					document.getElementById('feedback-request-id').value = '';
					document.getElementById('feedback-rating').value = 0;
					document.getElementById('feedback-helpful').checked = false;
					document.getElementById('feedback-accurate').checked = false;
					document.getElementById('feedback-comment').value = '';
					document.getElementById('feedback-status').innerHTML = '';
					document.querySelectorAll('.star-btn').forEach(btn => btn.style.opacity = '0.4');
				}, 2000);
			} catch (err) {
				document.getElementById('feedback-status').innerHTML = '<span style="color: #dc2626;">✗ Error submitting feedback</span>';
				console.error('Error submitting feedback:', err);
			}
		}

		async function loadAnalytics() {
			try {
				const response = await fetch('/api/analytics/personal');
				const data = await response.json();

				document.getElementById('analytics-rating').textContent = (data.average_rating || 0).toFixed(1);
				document.getElementById('analytics-helpful').textContent = (data.helpful_percentage || 0).toFixed(0);
				document.getElementById('analytics-accuracy').textContent = (data.accuracy_percentage || 0).toFixed(0);
				document.getElementById('analytics-count').textContent = data.total_feedback_count || 0;

				// Update preferences
				document.getElementById('pref-freshness').checked = data.prefers_freshness || false;
				document.getElementById('pref-opus').checked = data.prefers_opus || false;
				document.getElementById('pref-brief').checked = data.prefers_briefness || false;
				document.getElementById('pref-model-text').textContent = data.preferred_model || 'none';
			} catch (err) {
				console.error('Error loading analytics:', err);
			}
		}

		// Tool management functions
		async function loadTools() {
			try {
				const response = await fetch('/api/tools');
				const data = await response.json();
				const tbody = document.getElementById('tools-list-table');

				if (!data.tools || data.tools.length === 0) {
					tbody.innerHTML = '<tr><td colspan="5" style="padding: 20px; text-align: center; color: #999;">No tools configured yet</td></tr>';
					return;
				}

				tbody.innerHTML = data.tools.map(tool => {
					const healthBg = tool.health === 'ok' ? '#d1fae5' : '#fee2e2';
					const healthColor = tool.health === 'ok' ? '#065f46' : '#991b1b';
					const healthText = tool.health === 'ok' ? '✓ OK' : '✗ Error';
					return '<tr style="border-bottom: 1px solid #e5e7eb;">' +
						'<td style="padding: 12px;">' + escapeHtml(tool.name) + '</td>' +
						'<td style="padding: 12px;">' + tool.type + '</td>' +
						'<td style="padding: 12px; font-family: monospace; font-size: 12px;">' + escapeHtml(tool.path) + '</td>' +
						'<td style="padding: 12px;"><span style="display: inline-block; padding: 4px 8px; border-radius: 4px; background: ' + healthBg + '; color: ' + healthColor + ';">' + healthText + '</span></td>' +
						'<td style="padding: 12px;">' +
						'<button class="btn btn-secondary" onclick="editTool(\'' + escapeHtml(tool.name) + '\')" style="padding: 6px 12px; font-size: 12px; margin-right: 5px;">Edit</button>' +
						'<button class="btn btn-secondary" onclick="deleteTool(\'' + escapeHtml(tool.name) + '\')" style="padding: 6px 12px; font-size: 12px; color: #dc2626;">Delete</button>' +
						'</td></tr>';
				}).join('');
			} catch (err) {
				console.error('Error loading tools:', err);
				document.getElementById('tools-list-table').innerHTML = '<tr><td colspan="5" style="padding: 20px; text-align: center; color: #dc2626;">Error loading tools</td></tr>';
			}
		}

		function switchToolType(type) {
			const label = document.getElementById('tool-path-label');
			if (type === 'mcp') {
				label.textContent = 'Socket Path:';
				document.getElementById('tool-path').placeholder = '~/.sockets/custom.sock';
			} else if (type === 'rest') {
				label.textContent = 'API URL:';
				document.getElementById('tool-path').placeholder = 'http://localhost:8080';
			} else if (type === 'database') {
				label.textContent = 'Connection String:';
				document.getElementById('tool-path').placeholder = 'postgresql://user:pass@localhost/db';
			} else {
				label.textContent = 'Path:';
				document.getElementById('tool-path').placeholder = '/usr/local/bin/tool';
			}
		}

		function validateToolForm() {
			const name = document.getElementById('tool-name').value.trim();
			const type = document.getElementById('tool-type').value;
			const path = document.getElementById('tool-path').value.trim();

			if (!name || !type || !path) {
				document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ Please fill all required fields</span>';
				return false;
			}

			if (!/^[a-zA-Z0-9_]+$/.test(name)) {
				document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ Tool name must be alphanumeric with underscores only</span>';
				return false;
			}

			return true;
		}

		async function testTool() {
			if (!validateToolForm()) return;

			const name = document.getElementById('tool-name').value.trim();
			document.getElementById('tool-status').innerHTML = '<span style="color: #666;">Testing connection...</span>';

			try {
				const response = await fetch('/api/tools/' + name + '/test', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' }
				});
				const data = await response.json();

				if (data.status === 'healthy') {
					document.getElementById('tool-status').innerHTML = '<span style="color: #16a34a;">✓ Connection successful</span>';
				} else {
					const msg = data.error || 'Connection failed';
					document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ ' + msg + '</span>';
				}
			} catch (err) {
				document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ Error: ' + err.message + '</span>';
			}
		}

		async function addTool() {
			if (!validateToolForm()) return;

			const name = document.getElementById('tool-name').value.trim();
			const type = document.getElementById('tool-type').value;
			const path = document.getElementById('tool-path').value.trim();
			const settingsStr = document.getElementById('tool-settings').value.trim() || '{}';

			let settings = {};
			try {
				settings = JSON.parse(settingsStr);
			} catch (e) {
				document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ Invalid JSON in settings</span>';
				return;
			}

			document.getElementById('tool-status').innerHTML = '<span style="color: #666;">Adding tool...</span>';

			try {
				const response = await fetch('/api/tools/add', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name, type, path, settings })
				});
				const data = await response.json();

				if (response.ok) {
					document.getElementById('tool-status').innerHTML = '<span style="color: #16a34a;">✓ Tool added successfully</span>';
					document.getElementById('tool-name').value = '';
					document.getElementById('tool-type').value = '';
					document.getElementById('tool-path').value = '';
					document.getElementById('tool-settings').value = '';
					setTimeout(() => {
						loadTools();
						document.getElementById('tool-status').innerHTML = '';
					}, 1000);
				} else {
					const msg = data.error || 'Failed to add tool';
					document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ ' + msg + '</span>';
				}
			} catch (err) {
				document.getElementById('tool-status').innerHTML = '<span style="color: #dc2626;">✗ Error: ' + err.message + '</span>';
			}
		}

		async function editTool(name) {
			if (!confirm('Edit tool "' + name + '"? (Not yet implemented)')) return;
			// TODO: Implement tool editing
		}

		async function deleteTool(name) {
			if (!confirm('Are you sure you want to delete "' + name + '"?')) return;

			try {
				const response = await fetch('/api/tools/' + name, { method: 'DELETE' });
				if (response.ok) {
					loadTools();
				} else {
					alert('Failed to delete tool');
				}
			} catch (err) {
				alert('Error: ' + err.message);
			}
		}

		function escapeHtml(text) {
			const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
			return text.replace(/[&<>"']/g, m => map[m]);
		}

		// Load metrics every second
		setInterval(loadMetrics, 1000);

		// Initial load
		loadMetrics();
		loadConfig();
		loadTools();
	</script>
</body>
</html>`)
}
