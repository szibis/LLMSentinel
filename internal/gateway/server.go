package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/models"
)

// Embed the web UI static files
//go:embed static
var webUIFiles embed.FS

// Debug function to list embedded files
func listEmbeddedFiles() {
	fmt.Println("DEBUG: Checking embedded files in webUIFiles...")
	walkEmbedFS(webUIFiles, ".", 0)
}

func walkEmbedFS(fsys embed.FS, dir string, depth int) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		fmt.Printf("%*sError reading %s: %v\n", depth*2, "", dir, err)
		return
	}

	if len(entries) == 0 {
		fmt.Printf("%*s(empty)\n", depth*2, "")
		return
	}

	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if dir == "." {
			path = entry.Name()
		}

		if entry.IsDir() {
			fmt.Printf("%*s[DIR] %s\n", depth*2, "", path)
			walkEmbedFS(fsys, path, depth+1)
		} else {
			fmt.Printf("%*s[FILE] %s\n", depth*2, "", path)
		}
	}
}

// RequestMetrics tracks API usage for observability
type RequestMetrics struct {
	mu              sync.RWMutex
	TotalRequests   int64
	TotalTokens     int64
	TotalCost       float64
	ByModel         map[string]*ModelMetrics
	ByProvider      map[string]*ProviderMetrics
}

// ModelMetrics tracks usage by model
type ModelMetrics struct {
	Requests      int64
	InputTokens   int64
	OutputTokens  int64
	Cost          float64
	LastUsed      time.Time
	AvgLatency    float64
}

// ProviderMetrics tracks usage by provider
type ProviderMetrics struct {
	Requests     int64
	Successful   int64
	Failed       int64
	LastUsed     time.Time
	LastError    string
}

// Server is the HTTP gateway for the unified LLM client
type Server struct {
	unifiedClient *models.UnifiedClient
	registry      *models.ModelRegistry
	metrics       *RequestMetrics
	listenAddr    string
	apiKey        string
	wsMetrics     *WebSocketMetrics
}

// NewServer creates a new gateway server
func NewServer(unifiedClient *models.UnifiedClient, registry *models.ModelRegistry, listenAddr string) *Server {
	return &Server{
		unifiedClient: unifiedClient,
		registry:      registry,
		listenAddr:    listenAddr,
		metrics: &RequestMetrics{
			ByModel:    make(map[string]*ModelMetrics),
			ByProvider: make(map[string]*ProviderMetrics),
		},
		wsMetrics: NewWebSocketMetrics(),
	}
}

// SetAPIKey sets the required API key for authentication
func (s *Server) SetAPIKey(key string) {
	s.apiKey = key
}

// OpenAI-compatible request/response structures
type ChatCompletionRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	Temperature    float32 `json:"temperature,omitempty"`
	TopP           float32 `json:"top_p,omitempty"`
	Stream         bool    `json:"stream,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type ModelListResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID       string `json:"id"`
		Object   string `json:"object"`
		Owned_by string `json:"owned_by"`
	} `json:"data"`
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Debug: list embedded files
	listEmbeddedFiles()

	mux := http.NewServeMux()

	// OpenAI-compatible endpoints
	mux.HandleFunc("/v1/chat/completions", s.authMiddleware(s.handleChatCompletions))
	mux.HandleFunc("/v1/models", s.authMiddleware(s.handleListModels))

	// Admin/observability endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.authMiddleware(s.handleMetrics))
	mux.HandleFunc("/admin/usage", s.authMiddleware(s.handleUsage))

	// Web UI API endpoints (new)
	mux.HandleFunc("/api/config", s.handleAPIConfig)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/optimizations", s.handleAPIOptimizations)
	mux.HandleFunc("/api/optimizations/{name}/toggle", s.handleAPIOptimizationToggle)
	mux.HandleFunc("/api/cache/{action}", s.handleAPICacheControl)
	mux.HandleFunc("/api/metrics", s.handleAPIMetrics)
	mux.HandleFunc("/api/metrics/stream", s.wsMetrics.HandleWebSocket)

	// Serve web UI (embedded files) - catch-all routes after API routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip if trying to access known API endpoints
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/v1/") ||
			r.URL.Path == "/health" ||
			r.URL.Path == "/metrics" ||
			r.URL.Path == "/admin/usage" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the requested file from embedded FS
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "dashboard" {
			path = "index.html"
		}

		// Embedded files are under "static/" prefix
		fullPath := "static/" + path

		file, err := webUIFiles.Open(fullPath)
		if err == nil {
			defer file.Close()

			// Determine content type and serve
			contentType := "application/octet-stream"
			switch {
			case strings.HasSuffix(path, ".html"):
				contentType = "text/html; charset=utf-8"
			case strings.HasSuffix(path, ".css"):
				contentType = "text/css"
			case strings.HasSuffix(path, ".js"):
				contentType = "application/javascript"
			case strings.HasSuffix(path, ".json"):
				contentType = "application/json"
			case strings.HasSuffix(path, ".svg"):
				contentType = "image/svg+xml"
			case strings.HasSuffix(path, ".png"):
				contentType = "image/png"
			case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
				contentType = "image/jpeg"
			}

			w.Header().Set("Content-Type", contentType)
			if strings.HasSuffix(path, ".html") {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			io.Copy(w, file)
			return
		}

		// File not found, serve index.html for SPA routing
		indexFile, err := webUIFiles.Open("static/index.html")
		if err == nil {
			defer indexFile.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			io.Copy(w, indexFile)
			return
		}

		http.NotFound(w, r)
	})

	// Wrap mux with security headers middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		mux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:           s.listenAddr,
		Handler:        handler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	fmt.Printf("Gateway listening on %s\n", s.listenAddr)
	return server.ListenAndServe()
}

// authMiddleware enforces API key authentication if set
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			key := r.Header.Get("Authorization")
			if key == "" {
				key = r.Header.Get("x-api-key")
			}
			if key != "Bearer "+s.apiKey && key != s.apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// handleChatCompletions processes chat completion requests (OpenAI-compatible)
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		http.Error(w, "Messages required", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		http.Error(w, "Model required", http.StatusBadRequest)
		return
	}

	// Verify model exists
	modelInfo, exists := s.registry.GetModel(req.Model)
	if !exists {
		http.Error(w, fmt.Sprintf("Model not found: %s", req.Model), http.StatusBadRequest)
		return
	}

	if !modelInfo.Enabled {
		http.Error(w, fmt.Sprintf("Model not available: %s", req.Model), http.StatusBadRequest)
		return
	}

	// Record metrics start time
	startTime := time.Now()

	// Send request to unified client
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.unifiedClient.CreateMessage(ctx, req.Messages[0].Content, req.Model)
	if err != nil {
		s.recordFailure(modelInfo.Provider)
		http.Error(w, fmt.Sprintf("Gateway error: %v", err), http.StatusInternalServerError)
		return
	}

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	s.recordSuccess(req.Model, modelInfo.Provider, resp.Usage.InputTokens, resp.Usage.OutputTokens, latency)

	// Convert to OpenAI format
	response := ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
	}
	response.Choices = make([]struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}, 1)
	response.Choices[0].Index = 0
	response.Choices[0].Message.Role = "assistant"
	if len(resp.Content) > 0 {
		response.Choices[0].Message.Content = resp.Content[0].Text
	}
	response.Choices[0].FinishReason = resp.StopReason
	response.Usage.PromptTokens = resp.Usage.InputTokens
	response.Usage.CompletionTokens = resp.Usage.OutputTokens
	response.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListModels returns available models (OpenAI-compatible)
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models := s.registry.GetEnabledModels()
	response := ModelListResponse{
		Object: "list",
		Data:   make([]struct {
			ID       string `json:"id"`
			Object   string `json:"object"`
			Owned_by string `json:"owned_by"`
		}, len(models)),
	}

	for i, model := range models {
		response.Data[i].ID = model.ID
		response.Data[i].Object = "model"
		response.Data[i].Owned_by = model.Provider
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"models": s.registry.EnabledCount(),
	})
}

// handleMetrics returns usage metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_requests": s.metrics.TotalRequests,
		"total_tokens":   s.metrics.TotalTokens,
		"total_cost":     s.metrics.TotalCost,
		"by_model":       s.metrics.ByModel,
		"by_provider":    s.metrics.ByProvider,
	})
}

// handleUsage returns detailed usage information (admin only)
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics":  s.metrics,
		"timestamp": time.Now(),
	})
}

// recordSuccess records a successful request in metrics
func (s *Server) recordSuccess(modelID, provider string, inputTokens, outputTokens int, latencyMs int64) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	s.metrics.TotalRequests++
	s.metrics.TotalTokens += int64(inputTokens + outputTokens)

	// Update by-model metrics
	if _, exists := s.metrics.ByModel[modelID]; !exists {
		s.metrics.ByModel[modelID] = &ModelMetrics{}
	}
	modelMetrics := s.metrics.ByModel[modelID]
	modelMetrics.Requests++
	modelMetrics.InputTokens += int64(inputTokens)
	modelMetrics.OutputTokens += int64(outputTokens)
	modelMetrics.LastUsed = time.Now()
	// Update average latency
	modelMetrics.AvgLatency = (modelMetrics.AvgLatency*(float64(modelMetrics.Requests-1)) + float64(latencyMs)) / float64(modelMetrics.Requests)

	// Update by-provider metrics
	if _, exists := s.metrics.ByProvider[provider]; !exists {
		s.metrics.ByProvider[provider] = &ProviderMetrics{}
	}
	providerMetrics := s.metrics.ByProvider[provider]
	providerMetrics.Requests++
	providerMetrics.Successful++
	providerMetrics.LastUsed = time.Now()
}

// recordFailure records a failed request in metrics
func (s *Server) recordFailure(provider string) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	if _, exists := s.metrics.ByProvider[provider]; !exists {
		s.metrics.ByProvider[provider] = &ProviderMetrics{}
	}
	providerMetrics := s.metrics.ByProvider[provider]
	providerMetrics.Requests++
	providerMetrics.Failed++
	providerMetrics.LastError = time.Now().Format(time.RFC3339)
}
