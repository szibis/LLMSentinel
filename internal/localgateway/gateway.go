// Package localgateway implements local transport and an experimental Claude adapter.
// It deliberately does not use upstream placeholder inference or tool adapters.
package localgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	Upstream                 string
	Timeout                  time.Duration
	MaxRequestBytes          int64
	ClaudeAdapter            bool
	ClaudeMaxTokens          int
	ClaudeBufferedValidation bool
	Router                   DecisionRouter
	RoleUpstreams            map[string]string
	Training                 *TrainingConfig
	LearningOnly             bool
	Hybrid                   *HybridConfig
}

type Gateway struct {
	qualityRejections atomic.Uint64
	qualityRecoveries atomic.Uint64
	cfg               Config
	upstream          *url.URL
	proxy             *httputil.ReverseProxy
	transport         *http.Transport
	inference         chan struct{}
	claudeTransport   http.RoundTripper // Optional in-process transport for protocol tests.
	roles             map[string]*url.URL
	training          *trainingRecorder
	hybrid            *hybridRouter
	roleBudgets       map[string]*atomic.Int64
	activityState     activityState
}

func New(cfg Config) (*Gateway, error) {
	training, err := newTrainingRecorder(cfg.Training)
	if err != nil {
		return nil, err
	}
	if cfg.LearningOnly && training == nil {
		return nil, errors.New("learning mode requires training storage")
	}
	hybrid, err := newHybridRouter(cfg.Hybrid, training)
	if err != nil {
		return nil, err
	}
	u, err := localUpstream(cfg.Upstream)
	if err != nil {
		return nil, errors.New("invalid upstream URL")
	}
	roles := map[string]*url.URL{}
	for role, value := range cfg.RoleUpstreams {
		if role != "haiku" && role != "sonnet" && role != "opus" {
			return nil, errors.New("unknown configured local role")
		}
		endpoint, err := localUpstream(value)
		if err != nil {
			return nil, fmt.Errorf("%s endpoint: %w", role, err)
		}
		roles[role] = endpoint
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 10*time.Minute || cfg.MaxRequestBytes < 1 || cfg.MaxRequestBytes > 16*1024*1024 {
		return nil, errors.New("timeout must be positive and at most 10m; request limit must be 1..16 MiB")
	}
	if cfg.ClaudeMaxTokens == 0 {
		cfg.ClaudeMaxTokens = 768
	}
	if cfg.ClaudeMaxTokens < 1 || cfg.ClaudeMaxTokens > 32768 {
		return nil, errors.New("claude output budget must be 1..32768 tokens")
	}
	u.Path = "/v1"
	if cfg.Router == nil {
		if len(roles) > 0 {
			cfg.Router = RoleRouter{}
		} else {
			cfg.Router = LocalRouter{}
		}
	}
	g := &Gateway{cfg: cfg, upstream: u, roles: roles, training: training, hybrid: hybrid, inference: make(chan struct{}, 1), transport: &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		IdleConnTimeout: 30 * time.Second, MaxIdleConnsPerHost: 2,
	}}
	g.roleBudgets = map[string]*atomic.Int64{"haiku": new(atomic.Int64), "sonnet": new(atomic.Int64), "opus": new(atomic.Int64)}
	g.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			target := u
			if selected, ok := pr.In.Context().Value(upstreamContextKey{}).(*url.URL); ok {
				target = selected
			}
			pr.SetURL(target)
			pr.Out.URL.Path = "/v1" + strings.TrimPrefix(pr.In.URL.Path, "/v1")
			pr.Out.URL.RawPath = ""
		},
		Transport:     g.transport,
		FlushInterval: -1,
		ModifyResponse: func(res *http.Response) error {
			if res.StatusCode >= 300 && res.StatusCode < 400 {
				return errors.New("upstream redirects are disabled by strict-local policy")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			apiError(w, 502, "local_backend_unavailable", "MLX-Flash request failed; inspect /sentinel/status and runtime logs: "+err.Error())
		},
	}
	return g, nil
}

func (g *Gateway) Close() {
	g.transport.CloseIdleConnections()
	if g.hybrid != nil {
		g.hybrid.Close()
	}
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "sentinel_error", "code": code, "message": message}})
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := withTrainingRequestID(r.Context(), newID("req_"))
	client := map[string]string{"/v1/messages": "anthropic_messages", "/v1/responses": "openai_responses", "/v1/chat/completions": "openai_chat_completions"}[r.URL.Path]
	r = r.WithContext(withTrainingClient(ctx, client))
	if g.cfg.LearningOnly && r.URL.Path != "/health" && r.URL.Path != "/sentinel/training/events" && r.URL.Path != "/sentinel/control" && r.URL.Path != "/sentinel/activity" {
		apiError(w, 403, "learning_only", "Learning mode only accepts copied events; inference remains direct with the original provider")
		return
	}
	if !g.cfg.LearningOnly && g.hybrid != nil {
		ctx, cancel := context.WithTimeout(r.Context(), g.cfg.Timeout)
		defer cancel()
		r = r.WithContext(ctx)
		if g.hybrid.serve(w, r) {
			return
		}
	}
	switch r.URL.Path {
	case "/sentinel/activity":
		g.activity(w, r)
		return
	case "/sentinel/control":
		g.control(w, r)
		return
	case "/sentinel/training/events":
		g.trainingIngest(w, r)
		return
	case "/health":
		if r.Method != http.MethodGet {
			apiError(w, 405, "method_not_allowed", "Use GET")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		streaming := "buffered-model-with-heartbeats"
		if g.cfg.ClaudeBufferedValidation {
			streaming = "validated-buffered-model"
		}
		mode := "serving"
		if g.cfg.Hybrid != nil {
			mode = "hybrid"
		}
		if g.cfg.LearningOnly {
			mode = "learning"
		}
		adapter := g.cfg.ClaudeAdapter && !g.cfg.LearningOnly
		policy := "strict-local"
		if g.hybrid != nil {
			policy = "hybrid-" + g.hybrid.policyName()
		}
		if g.cfg.LearningOnly {
			policy = "copies-only"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "scope": "gateway", "mode": mode, "training": g.training.captureEnabled(), "policy": policy, "controls": g.controlStatus(), "routing": g.cfg.Router.Name(), "roles": g.roleProfiles(), "claude_max_tokens": g.cfg.ClaudeMaxTokens, "tool_mode": "validated-qwen-and-json", "capabilities": map[string]any{"plain_text": !g.cfg.LearningOnly, "tools": adapter, "responses": adapter, "messages": adapter, "claude_roles": len(g.roles) == 3 && !g.cfg.LearningOnly, "streaming": streaming}})
		return
	case "/sentinel/status":
		if r.Method != http.MethodGet {
			apiError(w, 405, "method_not_allowed", "Use GET")
			return
		}
		g.status(w, r)
		return
	case "/v1/messages":
		if g.cfg.ClaudeAdapter {
			g.claude(w, r)
			return
		}
		apiError(w, 501, "protocol_not_ready", "Claude adapter is disabled")
		return
	case "/api/hello":
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		apiError(w, 405, "method_not_allowed", "Use HEAD")
		return
	case "/v1/messages/count_tokens":
		apiError(w, 501, "token_count_unavailable", "Exact token counting is unavailable; Claude Code can use its context estimate")
		return
	case "/v1/responses":
		if g.cfg.ClaudeAdapter {
			g.responses(w, r)
			return
		}
		apiError(w, 501, "protocol_not_ready", "Codex Responses adapter is pending; no request was sent to inference.")
		return
	case "/v1/models":
		if r.Method != http.MethodGet {
			apiError(w, 405, "method_not_allowed", "Use GET")
			return
		}
		if len(g.roles) > 0 {
			models := []map[string]string{}
			for _, role := range []string{"haiku", "sonnet", "opus"} {
				if g.roles[role] != nil {
					models = append(models, map[string]string{"id": "sentinel-" + role, "object": "model", "owned_by": "sentinel-local"})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
			return
		}
	case "/v1/chat/completions":
		if g.cfg.ClaudeAdapter {
			g.chatCompletions(w, r)
			return
		}
		if r.Method != http.MethodPost {
			apiError(w, 405, "method_not_allowed", "Use POST")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, g.cfg.MaxRequestBytes))
		_ = r.Body.Close()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				apiError(w, 413, "request_too_large", "Request exceeds configured body limit")
			} else {
				apiError(w, 400, "invalid_request", "Cannot read request body")
			}
			return
		}
		var payload struct {
			Model        string            `json:"model"`
			Tools        []json.RawMessage `json:"tools"`
			Functions    []json.RawMessage `json:"functions"`
			ToolChoice   json.RawMessage   `json:"tool_choice"`
			FunctionCall json.RawMessage   `json:"function_call"`
			Messages     []struct {
				Role         string            `json:"role"`
				ToolCalls    []json.RawMessage `json:"tool_calls"`
				FunctionCall json.RawMessage   `json:"function_call"`
			} `json:"messages"`
		}
		if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' || json.Unmarshal(body, &payload) != nil {
			apiError(w, 400, "invalid_json", "Expected a JSON request object")
			return
		}
		needsTools := len(payload.Tools) > 0 || len(payload.Functions) > 0 || requestedCall(payload.ToolChoice) || requestedCall(payload.FunctionCall)
		for _, m := range payload.Messages {
			needsTools = needsTools || m.Role == "tool" || m.Role == "function" || len(m.ToolCalls) > 0 || requestedCall(m.FunctionCall)
		}
		if needsTools {
			apiError(w, 501, "tools_not_ready", "The audited MLX-Flash path lacks a verified tool-call contract. No inference request was sent; local coding-agent compatibility is not ready.")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		if len(g.roles) > 0 {
			route, err := g.cfg.Router.Select(r.Context(), RouteTask{Model: payload.Model, Messages: len(payload.Messages)})
			if err != nil {
				apiError(w, 400, "unknown_model", err.Error())
				return
			}
			target, err := g.routeUpstream(route)
			if err != nil {
				apiError(w, 503, "role_unavailable", err.Error())
				return
			}
			var routed map[string]any
			if json.Unmarshal(body, &routed) != nil {
				apiError(w, 400, "invalid_json", "Invalid request")
				return
			}
			budget, ok := routed["max_tokens"].(float64)
			if !ok {
				budget = float64(route.MaxTokens)
			}
			if budget < 1 || budget != float64(int(budget)) {
				apiError(w, 400, "invalid_request", "max_tokens must be a positive integer")
				return
			}
			routed["model"] = "local"
			routed["max_tokens"] = min(int(budget), route.MaxTokens)
			routed["chat_template_kwargs"] = map[string]bool{"enable_thinking": route.Thinking}
			body = mustJSON(routed)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			r = r.WithContext(context.WithValue(r.Context(), upstreamContextKey{}, target))
			w.Header().Set("X-Sentinel-Role", route.Role)
		}
	default:
		apiError(w, 404, "unknown_endpoint", "Available: /health, /sentinel/status, /v1/models, /v1/chat/completions")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.Timeout)
	defer cancel()
	g.proxy.ServeHTTP(w, r.WithContext(ctx))
}

func requestedCall(value json.RawMessage) bool {
	v := strings.TrimSpace(string(value))
	return v != "" && v != "null" && v != `"none"`
}

func (g *Gateway) status(w http.ResponseWriter, r *http.Request) {
	if len(g.roles) > 0 {
		g.rolesStatus(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	u := *g.upstream
	u.Path = "/health"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	client := &http.Client{Transport: g.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("runtime redirects are disabled") }}
	res, err := client.Do(req)
	if err != nil {
		apiError(w, 502, "runtime_status_unavailable", "Runtime health unavailable; this does not prove whether a request is loading or generating")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		apiError(w, 502, "runtime_status_unavailable", fmt.Sprintf("Runtime health returned HTTP %d", res.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 || !json.Valid(body) {
		apiError(w, 502, "invalid_runtime_status", "Invalid or oversized runtime health response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"scope": "runtime", "upstream": g.upstream.String(), "health": json.RawMessage(body)})
}
