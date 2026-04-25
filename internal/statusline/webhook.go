package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookSource fetches metrics from a custom HTTP webhook.
// Expects endpoint to return metrics as JSON.
type WebhookSource struct {
	url       string
	authToken string
	enabled   bool
	client    *http.Client
}

// NewWebhookSource creates a webhook source.
func NewWebhookSource(url, authToken string) *WebhookSource {
	enabled := url != ""

	return &WebhookSource{
		url:       url,
		authToken: authToken,
		enabled:   enabled,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// Name returns the source name.
func (ws *WebhookSource) Name() string {
	return "webhook"
}

// IsAvailable checks if webhook is configured.
func (ws *WebhookSource) IsAvailable() bool {
	return ws.enabled && ws.url != ""
}

// Priority returns webhook priority (3).
func (ws *WebhookSource) Priority() int {
	return 3
}

// Poll fetches metrics from webhook endpoint.
func (ws *WebhookSource) Poll() (StatuslineData, error) {
	if !ws.IsAvailable() {
		return StatuslineData{}, fmt.Errorf("webhook source not configured")
	}

	req, err := http.NewRequest("GET", ws.url, nil)
	if err != nil {
		return StatuslineData{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth token if provided
	if ws.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ws.authToken))
	}

	req.Header.Set("Accept", "application/json")

	resp, err := ws.client.Do(req)
	if err != nil {
		return StatuslineData{}, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return StatuslineData{}, fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(body))
	}

	var webhookMetrics struct {
		InputTokens         int     `json:"input_tokens"`
		OutputTokens        int     `json:"output_tokens"`
		CacheHitTokens      int     `json:"cache_hit_tokens"`
		CacheCreationTokens int     `json:"cache_creation_tokens"`
		ContextUsage        int     `json:"context_usage_percent"`
		Model               string  `json:"model"`
		IsCaching           bool    `json:"is_caching"`
		CachePercent        float64 `json:"cache_fill_percent"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&webhookMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse webhook response: %w", err)
	}

	return StatuslineData{
		Source:              ws.Name(),
		Timestamp:           time.Now(),
		InputTokens:         webhookMetrics.InputTokens,
		OutputTokens:        webhookMetrics.OutputTokens,
		CacheHitTokens:      webhookMetrics.CacheHitTokens,
		CacheCreationTokens: webhookMetrics.CacheCreationTokens,
		ContextWindowUsage:  webhookMetrics.ContextUsage,
		Model:               webhookMetrics.Model,
		IsCaching:           webhookMetrics.IsCaching,
		CacheFillPercentage: webhookMetrics.CachePercent,
	}, nil
}
