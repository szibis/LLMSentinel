package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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

// validateWebhookURL ensures the webhook URL is safe to call.
// Prevents SSRF attacks by blocking local, private, reserved, and metadata addresses.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Enforce HTTPS only
	if u.Scheme != "https" {
		return fmt.Errorf("webhook must use https, got %s", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("webhook URL missing hostname")
	}

	// Validate against literal IP
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isRestrictedIP(ip) {
			return fmt.Errorf("webhook cannot target reserved/private address: %s", hostname)
		}
		return nil // Valid public IP
	}

	// Resolve hostname and validate all returned IPs (prevent DNS rebinding)
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve webhook hostname: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("webhook hostname resolved to no addresses: %s", hostname)
	}

	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return fmt.Errorf("webhook hostname resolves to restricted address: %s -> %s", hostname, ip.String())
		}
	}

	return nil
}

// isRestrictedIP checks if an IP is in a restricted range (loopback, private, link-local, metadata, etc.)
func isRestrictedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.Equal(net.IPv4bcast) ||
		ip.Equal(net.IPv4allsys) ||
		ip.Equal(net.IPv4allrouter) ||
		ip.Equal(net.IPv4zero)
}

// NewWebhookSource creates a webhook source.
func NewWebhookSource(url, authToken string) *WebhookSource {
	enabled := false
	if url != "" {
		if err := validateWebhookURL(url); err == nil {
			enabled = true
		}
	}

	return &WebhookSource{
		url:       url,
		authToken: authToken,
		enabled:   enabled,
		client: &http.Client{
			Timeout: 5 * time.Second,
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
		InputTokens         *int    `json:"input_tokens"`
		OutputTokens        *int    `json:"output_tokens"`
		CacheHitTokens      *int    `json:"cache_hit_tokens"`
		CacheCreationTokens *int    `json:"cache_creation_tokens"`
		ContextUsage        *int    `json:"context_usage_percent"`
		Model               *string `json:"model"`
		IsCaching           *bool   `json:"is_caching"`
		CachePercent        *float64 `json:"cache_fill_percent"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&webhookMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse webhook response: %w", err)
	}

	// Validate required fields are present
	if webhookMetrics.InputTokens == nil || webhookMetrics.OutputTokens == nil {
		return StatuslineData{}, fmt.Errorf("webhook response missing required token fields")
	}

	// Validate token ranges to prevent integer overflow/underflow
	if *webhookMetrics.InputTokens < 0 || *webhookMetrics.OutputTokens < 0 {
		return StatuslineData{}, fmt.Errorf("webhook response contains negative token counts")
	}

	const maxTokens = 1000000
	if *webhookMetrics.InputTokens > maxTokens || *webhookMetrics.OutputTokens > maxTokens {
		return StatuslineData{}, fmt.Errorf("webhook token counts exceed maximum allowed: %d", maxTokens)
	}

	// Default missing optional fields
	cacheHit := 0
	if webhookMetrics.CacheHitTokens != nil {
		cacheHit = *webhookMetrics.CacheHitTokens
	}

	cacheCreate := 0
	if webhookMetrics.CacheCreationTokens != nil {
		cacheCreate = *webhookMetrics.CacheCreationTokens
	}

	contextUsage := 0
	if webhookMetrics.ContextUsage != nil && *webhookMetrics.ContextUsage >= 0 && *webhookMetrics.ContextUsage <= 100 {
		contextUsage = *webhookMetrics.ContextUsage
	}

	model := ""
	if webhookMetrics.Model != nil {
		model = *webhookMetrics.Model
	}

	caching := false
	if webhookMetrics.IsCaching != nil {
		caching = *webhookMetrics.IsCaching
	}

	cachePercent := 0.0
	if webhookMetrics.CachePercent != nil && *webhookMetrics.CachePercent >= 0.0 && *webhookMetrics.CachePercent <= 1.0 {
		cachePercent = *webhookMetrics.CachePercent
	}

	return StatuslineData{
		Source:              ws.Name(),
		Timestamp:           time.Now(),
		InputTokens:         *webhookMetrics.InputTokens,
		OutputTokens:        *webhookMetrics.OutputTokens,
		CacheHitTokens:      cacheHit,
		CacheCreationTokens: cacheCreate,
		ContextWindowUsage:  contextUsage,
		Model:               model,
		IsCaching:           caching,
		CacheFillPercentage: cachePercent,
	}, nil
}
