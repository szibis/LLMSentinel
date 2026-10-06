package localgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HybridConfig opts into separately billed vendor APIs. Keys are read only from
// explicitly named environment variables; CLI subscription tokens are unused.
type HybridConfig struct {
	AnthropicBaseURL   string
	AnthropicModel     string
	AnthropicAPIKeyEnv string
	OpenAIBaseURL      string
	OpenAIModel        string
	OpenAIAPIKeyEnv    string
	AllowPaidAPI       bool
}

type hybridProvider struct {
	base          *url.URL
	model, keyEnv string
}
type hybridDecision struct{ Provider, Model, Reason, BillingClass string }
type hybridRouter struct {
	mu                                                                 sync.RWMutex
	policy                                                             string
	providers                                                          map[string]hybridProvider
	transport                                                          http.RoundTripper
	training                                                           *trainingRecorder
	maxRequestBytes, maxResponseBytes, maxStreamBytes, maxCaptureBytes int64
}

func newHybridRouter(cfg *HybridConfig, training *trainingRecorder) (*hybridRouter, error) {
	if cfg == nil {
		return nil, nil
	}
	configured := cfg.AnthropicBaseURL != "" || cfg.AnthropicModel != "" || cfg.AnthropicAPIKeyEnv != "" || cfg.OpenAIBaseURL != "" || cfg.OpenAIModel != "" || cfg.OpenAIAPIKeyEnv != ""
	if configured && !cfg.AllowPaidAPI {
		return nil, errors.New("configured commercial providers require explicit allow-paid-api opt-in; APIs do not use CLI subscriptions")
	}
	if !configured {
		return nil, nil
	}
	h := &hybridRouter{policy: "balanced", providers: map[string]hybridProvider{}, training: training, transport: &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 2 * time.Minute, IdleConnTimeout: 30 * time.Second}, maxRequestBytes: 2 * 1024 * 1024, maxResponseBytes: 8 * 1024 * 1024, maxStreamBytes: 64 * 1024 * 1024, maxCaptureBytes: 8 * 1024 * 1024}
	envName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, provider := range []struct{ name, base, model, keyEnv string }{{"anthropic", cfg.AnthropicBaseURL, cfg.AnthropicModel, cfg.AnthropicAPIKeyEnv}, {"openai", cfg.OpenAIBaseURL, cfg.OpenAIModel, cfg.OpenAIAPIKeyEnv}} {
		if provider.base == "" && provider.model == "" && provider.keyEnv == "" {
			continue
		}
		base, err := url.Parse(provider.base)
		if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.RawPath != "" {
			return nil, fmt.Errorf("%s hybrid base URL must be HTTPS without credentials, query or fragment", provider.name)
		}
		if strings.TrimSpace(provider.model) == "" || !envName.MatchString(provider.keyEnv) {
			return nil, fmt.Errorf("%s hybrid provider requires an explicit model and API-key environment variable name", provider.name)
		}
		h.providers[provider.name] = hybridProvider{base: base, model: provider.model, keyEnv: provider.keyEnv}
	}
	return h, nil
}

func (h *hybridRouter) setPolicy(policy string) error {
	if h == nil {
		return errors.New("hybrid is not configured")
	}
	switch policy {
	case "local-only", "balanced", "quality":
	default:
		return errors.New("policy must be local-only, balanced or quality")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.policy = policy
	return nil
}

func (h *hybridRouter) policyName() string {
	if h == nil {
		return "local-only"
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.policy
}

func (h *hybridRouter) Close() {
	if h == nil {
		return
	}
	if transport, ok := h.transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

// decide is a deterministic role/effort policy, not a trained Jes decision.
// Missing providers and cheap requests continue through the local gateway.
func (h *hybridRouter) decide(path string, body []byte) (hybridDecision, error) {
	if h == nil {
		return hybridDecision{}, nil
	}
	policy := h.policyName()
	if policy == "local-only" {
		return hybridDecision{}, nil
	}
	provider := ""
	switch path {
	case "/v1/messages":
		provider = "anthropic"
	case "/v1/responses", "/v1/chat/completions":
		provider = "openai"
	default:
		return hybridDecision{}, nil
	}
	var request struct {
		Model     string `json:"model"`
		Reasoning struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' || json.Unmarshal(body, &request) != nil {
		return hybridDecision{}, errors.New("expected a JSON request object")
	}
	heavy := request.Model == "sentinel-opus" || request.Model == "opus"
	reason := "explicit Opus role"
	if path == "/v1/responses" && (request.Reasoning.Effort == "high" || request.Reasoning.Effort == "xhigh") {
		heavy = true
		reason = "explicit high reasoning effort"
	}
	if policy == "quality" {
		heavy = true
		reason = "explicit quality policy"
	}
	if !heavy {
		return hybridDecision{}, nil
	}
	selected, ok := h.providers[provider]
	if !ok {
		return hybridDecision{}, nil
	}
	return hybridDecision{Provider: provider, Model: selected.model, Reason: reason, BillingClass: "api_usage"}, nil
}

func hybridError(w http.ResponseWriter, path string, status int, message string) {
	if path == "/v1/messages" {
		claudeError(w, status, message)
	} else {
		responsesError(w, status, message)
	}
}

func (h *hybridRouter) serve(w http.ResponseWriter, r *http.Request) bool {
	if h == nil {
		return false
	}
	switch r.URL.Path {
	case "/v1/messages", "/v1/responses", "/v1/chat/completions":
	default:
		return false
	}
	if r.Method != http.MethodPost {
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxRequestBytes))
	_ = r.Body.Close()
	if err != nil {
		hybridError(w, r.URL.Path, 413, "Hybrid request exceeds configured body limit")
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	decision, err := h.decide(r.URL.Path, body)
	if err != nil {
		hybridError(w, r.URL.Path, 400, err.Error())
		return true
	}
	if decision.Provider == "" {
		return false
	}
	selected := h.providers[decision.Provider]
	key := os.Getenv(selected.keyEnv)
	if strings.TrimSpace(key) == "" {
		hybridError(w, r.URL.Path, 503, "Configured commercial API-key environment variable is unset; no paid or local fallback was attempted")
		return true
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		hybridError(w, r.URL.Path, 400, "Invalid request object")
		return true
	}
	payload["model"] = decision.Model
	endpoint := *selected.base
	basePath := strings.TrimRight(endpoint.Path, "/")
	if strings.HasSuffix(basePath, "/v1") {
		endpoint.Path = basePath + strings.TrimPrefix(r.URL.Path, "/v1")
	} else {
		endpoint.Path = basePath + r.URL.Path
	}
	endpoint.RawPath = ""
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(mustJSON(payload)))
	if err != nil {
		hybridError(w, r.URL.Path, 502, "Cannot construct commercial request")
		return true
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if decision.Provider == "anthropic" {
		request.Header.Set("x-api-key", key)
		version := r.Header.Get("anthropic-version")
		if version == "" {
			version = "2023-06-01"
		}
		request.Header.Set("anthropic-version", version)
		if beta := r.Header.Get("anthropic-beta"); beta != "" {
			request.Header.Set("anthropic-beta", beta)
		}
	} else {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Transport: h.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("commercial redirects disabled") }}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		h.record(r, decision, payload, "", nil, false, false, time.Since(started), "commercial API transport failed")
		hybridError(w, r.URL.Path, 502, "Commercial API request failed; no fallback was attempted")
		return true
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		h.record(r, decision, payload, "", nil, false, false, time.Since(started), "commercial redirect refused")
		hybridError(w, r.URL.Path, 502, "Commercial redirect refused")
		return true
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") && response.StatusCode == 200 {
		h.stream(w, r, response, decision, payload, started)
		return true
	}
	output, err := io.ReadAll(io.LimitReader(response.Body, h.maxResponseBytes+1))
	if err != nil || int64(len(output)) > h.maxResponseBytes {
		h.record(r, decision, payload, "", nil, false, false, time.Since(started), "commercial response exceeds limit or could not be read")
		hybridError(w, r.URL.Path, 502, "Commercial response exceeds limit or could not be read")
		return true
	}
	if !json.Valid(output) {
		h.record(r, decision, payload, "", nil, false, false, time.Since(started), "commercial response was not JSON")
		hybridError(w, r.URL.Path, 502, "Commercial response was not JSON")
		return true
	}
	usage, completed := hybridJSONUsage(r.URL.Path, output)
	accepted := response.StatusCode == 200 && completed
	h.record(r, decision, payload, string(output), usage, accepted, true, time.Since(started), "")
	copyHybridHeaders(w, response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(output)
	return true
}

func copyHybridHeaders(w http.ResponseWriter, headers http.Header) {
	// Response cookies and authentication headers never enter the local client.
	for _, name := range []string{"Content-Type", "Cache-Control", "Retry-After", "Request-Id", "X-Request-Id", "OpenAI-Processing-Ms", "Anthropic-Ratelimit-Requests-Remaining", "Anthropic-Ratelimit-Tokens-Remaining"} {
		if value := headers.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
}

func hybridJSONUsage(path string, body []byte) (map[string]any, bool) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil {
		return nil, false
	}
	usage, _ := object["usage"].(map[string]any)
	switch path {
	case "/v1/responses":
		return usage, object["status"] == "completed"
	case "/v1/messages":
		return usage, object["type"] == "message" && object["stop_reason"] != nil
	case "/v1/chat/completions":
		choices, _ := object["choices"].([]any)
		for _, raw := range choices {
			if choice, ok := raw.(map[string]any); ok && choice["finish_reason"] != nil {
				return usage, true
			}
		}
	}
	return usage, false
}

func (h *hybridRouter) stream(w http.ResponseWriter, r *http.Request, response *http.Response, decision hybridDecision, payload map[string]any, started time.Time) {
	copyHybridHeaders(w, response.Header)
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(response.StatusCode)
	var capture bytes.Buffer
	buffer := make([]byte, 32*1024)
	total := int64(0)
	captureComplete := true
	readComplete := false
	failure := ""
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > h.maxStreamBytes {
				failure = "commercial stream exceeds forwarding limit"
				break
			}
			if captureComplete {
				if int64(capture.Len()+n) <= h.maxCaptureBytes {
					capture.Write(buffer[:n])
				} else {
					captureComplete = false
					capture.Reset()
				}
			}
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				failure = "commercial client disconnected"
				break
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				readComplete = true
			} else {
				failure = "commercial stream read failed"
			}
			break
		}
	}
	var usage map[string]any
	completed := false
	if captureComplete {
		usage, completed = hybridSSEUsage(r.URL.Path, capture.String())
	} else {
		failure = "commercial stream exceeded training capture limit"
	}
	accepted := readComplete && completed && captureComplete
	output := capture.String()
	if !accepted {
		output = ""
		usage = nil
		if failure == "" {
			failure = "commercial stream ended without terminal completion"
		}
	}
	h.record(r, decision, payload, output, usage, accepted, accepted, time.Since(started), failure)
}

func hybridSSEUsage(path, body string) (map[string]any, bool) {
	var usage map[string]any
	completed, chatFinished := false, false
	body = strings.ReplaceAll(body, "\r\n", "\n")
	frames := strings.Split(body, "\n\n")
	// An EOF fragment without a blank-line delimiter was never dispatched as
	// an SSE event by the client. It cannot prove protocol completion.
	for _, frame := range frames[:len(frames)-1] {
		var data []string
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		raw := strings.Join(data, "\n")
		if raw == "" {
			continue
		}
		if raw == "[DONE]" && path == "/v1/chat/completions" {
			completed = chatFinished
			continue
		}
		if !json.Valid([]byte(raw)) {
			continue
		}
		var event map[string]any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&event) != nil {
			continue
		}
		if value, ok := event["usage"].(map[string]any); ok {
			if usage == nil {
				usage = map[string]any{}
			}
			for key, item := range value {
				usage[key] = item
			}
		}
		switch path {
		case "/v1/responses":
			if event["type"] == "response.completed" {
				if response, ok := event["response"].(map[string]any); ok && response["status"] == "completed" {
					usage, _ = response["usage"].(map[string]any)
					completed = true
				}
			}
		case "/v1/messages":
			if event["type"] == "message_start" {
				if message, ok := event["message"].(map[string]any); ok {
					usage, _ = message["usage"].(map[string]any)
				}
			}
			if event["type"] == "message_stop" {
				completed = true
			}
		case "/v1/chat/completions":
			choices, _ := event["choices"].([]any)
			for _, rawChoice := range choices {
				if choice, ok := rawChoice.(map[string]any); ok && choice["finish_reason"] != nil {
					chatFinished = true
				}
			}
		}
	}
	return usage, completed
}

func (h *hybridRouter) record(r *http.Request, decision hybridDecision, input map[string]any, output string, usage map[string]any, accepted, captureComplete bool, latency time.Duration, failure string) {
	flat := map[string]int{}
	for key, value := range usage {
		if number, ok := value.(json.Number); ok {
			if integer, err := strconv.ParseInt(number.String(), 10, 64); err == nil && integer >= 0 && int64(int(integer)) == integer {
				flat[key] = int(integer)
			}
		}
	}
	quality := map[string]any{"billing_class": decision.BillingClass, "capture_complete": captureComplete, "usage_known": usage != nil}
	h.training.record(r.Context(), trainingEvent{Provider: decision.Provider, Model: decision.Model, Role: "commercial", Reason: decision.Reason, AttemptID: newID("attempt_"), Input: input, Output: output, Usage: flat, LatencyMS: latency.Milliseconds(), Accepted: accepted, Error: failure, Quality: quality, Reference: map[string]any{"protocol": r.URL.Path, "vendor_usage": usage, "billing_class": decision.BillingClass}})
}
