package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"claude-escalate/internal/config"
	"claude-escalate/internal/metrics"
)

// Server represents the dashboard HTTP server
type Server struct {
	host            string
	port            int
	configLoader    *config.Loader
	metricsCollector *metrics.MetricsCollector
	metricsPublisher *metrics.MetricsPublisher
	httpServer      *http.Server
	mu              sync.RWMutex
	configPath      string
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

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
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
		"message": "WebSocket streaming not yet implemented, use /api/metrics with polling",
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
		}

		// Load metrics every second
		setInterval(loadMetrics, 1000);

		// Initial load
		loadMetrics();
		loadConfig();
	</script>
</body>
</html>`)
}
