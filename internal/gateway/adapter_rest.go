package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RESTAdapter implements ToolAdapter for HTTP REST calls
type RESTAdapter struct {
	client    *http.Client
	signature *ToolSignature
}

// NewRESTAdapter creates a new REST adapter
func NewRESTAdapter() *RESTAdapter {
	adapter := &RESTAdapter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	adapter.initializeSignature()
	return adapter
}

// Name returns the adapter name
func (a *RESTAdapter) Name() string {
	return "rest"
}

// Type returns the tool type
func (a *RESTAdapter) Type() ToolType {
	return ToolTypeREST
}

// Execute executes an HTTP REST call
func (a *RESTAdapter) Execute(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
	url, ok := req.Params["url"].(string)
	if !ok {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   "url parameter required",
		}, nil
	}

	method := "GET"
	if m, ok := req.Params["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	start := time.Now()

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to create request: %v", err),
		}, nil
	}

	// Add headers if provided
	if headers, ok := req.Params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strVal, ok := value.(string); ok {
				httpReq.Header.Set(key, strVal)
			}
		}
	}

	// Execute request
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   fmt.Sprintf("request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to read response: %v", err),
		}, nil
	}

	return &ToolResponse{
		ID:      req.ID,
		Success: resp.StatusCode >= 200 && resp.StatusCode < 300,
		Data: map[string]interface{}{
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
			"body":        string(body),
		},
		Timing: ResponseTiming{
			TotalMs:    time.Since(start).Milliseconds(),
			ToolExecMs: time.Since(start).Milliseconds(),
		},
	}, nil
}

// GetSignature returns the tool signature
func (a *RESTAdapter) GetSignature() *ToolSignature {
	return a.signature
}

// Health checks if the REST adapter is functional
func (a *RESTAdapter) Health(ctx context.Context) error {
	// REST adapter is always functional
	return nil
}

// Close closes the REST adapter
func (a *RESTAdapter) Close() error {
	a.client.CloseIdleConnections()
	return nil
}

// initializeSignature sets up the tool signature
func (a *RESTAdapter) initializeSignature() {
	a.signature = &ToolSignature{
		Name:        "rest",
		Type:        ToolTypeREST,
		Description: "Make HTTP REST API calls",
		Parameters: map[string]*ParamSchema{
			"url": {
				Type:        "string",
				Description: "URL to call",
				Required:    true,
			},
			"method": {
				Type:        "string",
				Description: "HTTP method (GET, POST, PUT, DELETE, etc)",
				Required:    false,
				Default:     "GET",
			},
			"headers": {
				Type:        "object",
				Description: "HTTP headers",
				Required:    false,
			},
		},
		Returns: &ParamSchema{
			Type:        "object",
			Description: "HTTP response with status code and body",
		},
	}
}
