package localgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func localUpstream(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("invalid upstream URL")
	}
	if u.Hostname() == "localhost" {
		port := u.Port()
		u.Host = "127.0.0.1"
		if port != "" {
			u.Host = net.JoinHostPort("127.0.0.1", port)
		}
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimRight(u.Path, "/") != "/v1" || u.RawPath != "" {
		return nil, errors.New("strict-local upstream must be http://127.0.0.1:PORT/v1 or another literal loopback address, without credentials/query")
	}
	u.Path = "/v1"
	return u, nil
}

func (g *Gateway) routeUpstream(route RouteDecision) (*url.URL, error) {
	if route.Role == "" {
		if route.Model != "local" {
			return nil, errors.New("router selected an unconfigured model")
		}
		return g.upstream, nil
	}
	u := g.roles[route.Role]
	if u == nil {
		return nil, fmt.Errorf("local %s role has no configured runtime", route.Role)
	}
	return u, nil
}

// Qwen thinking can start in the generation prompt, so output may contain only
// the closing marker. Accept only a completed section for an Opus generation.
func finalRoleText(text string, thinking bool, format ...string) (string, error) {
	marker := "</think>"
	if strings.Contains(text, "<|channel>") || strings.Contains(text, "<channel|>") {
		marker = "<channel|>"
	}
	if len(format) > 0 && thinking {
		switch format[0] {
		case "gemma":
			marker = "<channel|>"
		case "think":
			marker = "</think>"
		}
	}
	before, after, complete := strings.Cut(text, marker)
	if complete {
		if marker == "<channel|>" && strings.Contains(before, "<|channel>") && !strings.HasPrefix(strings.TrimLeft(before, " \t\r\n"), "<|channel>thought\n") {
			return "", invalidOutput("unsupported local reasoning channel")
		}
		// Some native continuations repeat the empty thought delimiter. Strip
		// only complete, empty leading sections; never discard a second thought.
		if marker == "<channel|>" {
			for i := 0; i < 8; i++ {
				candidate := strings.TrimSpace(after)
				if !strings.HasPrefix(candidate, "<|channel>thought\n<channel|>") {
					break
				}
				after = strings.TrimPrefix(candidate, "<|channel>thought\n<channel|>")
			}
		}
		if strings.Contains(after, "<think") || strings.Contains(after, "</think") || strings.Contains(after, "<|channel") || strings.Contains(after, "<channel|") || strings.TrimSpace(after) == "" {
			return "", invalidOutput("local reasoning did not complete; no partial answer or tool input accepted")
		}
		return strings.TrimSpace(after), nil
	}
	if thinking || strings.Contains(text, "<think") || strings.Contains(text, "</think") || strings.Contains(text, "<|channel") || strings.Contains(text, "<channel|") {
		return "", invalidOutput("local reasoning did not complete; no partial answer or tool input accepted")
	}
	return text, nil
}

type modelCapabilities struct {
	Family          string `json:"model_family"`
	ThinkingControl bool   `json:"thinking_control"`
	ReasoningFormat string `json:"reasoning_format"`
}
type modelCapabilitiesKey struct{}

// Resolve once per incoming role turn and pin through format recovery. This
// avoids stale family controls if an operator replaces a runtime at its port.
func (g *Gateway) modelCapabilities(ctx context.Context, endpoint *url.URL) (modelCapabilities, error) {
	legacy := modelCapabilities{Family: "qwen", ThinkingControl: true, ReasoningFormat: "think"}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	u := *endpoint
	u.Path = "/health"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	transport := http.RoundTripper(g.transport)
	if g.claudeTransport != nil {
		transport = g.claudeTransport
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("runtime redirects are disabled") }}
	res, err := client.Do(req)
	if err != nil {
		return modelCapabilities{}, errors.New("runtime family health unavailable; no inference dispatched")
	}
	defer res.Body.Close()
	var health struct {
		Capabilities modelCapabilities `json:"capabilities"`
	}
	body, readErr := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if res.StatusCode != 200 || readErr != nil || len(body) > 64*1024 || json.Unmarshal(body, &health) != nil {
		return modelCapabilities{}, errors.New("invalid runtime family health; no inference dispatched")
	}
	if health.Capabilities.Family == "" {
		// Older native Qwen runtimes advertise only the template option.
		var old struct {
			Capabilities struct {
				Options []string `json:"chat_template_kwargs"`
			} `json:"capabilities"`
		}
		if json.Unmarshal(body, &old) == nil {
			for _, option := range old.Capabilities.Options {
				if option == "enable_thinking" {
					return legacy, nil
				}
			}
		}
		return modelCapabilities{}, errors.New("runtime lacks model-family capabilities; update MLX-Flash")
	}
	caps := health.Capabilities
	valid := false
	switch caps.Family {
	case "lfm2_moe":
		valid = !caps.ThinkingControl && caps.ReasoningFormat == "think"
	case "gemma4":
		valid = caps.ThinkingControl && caps.ReasoningFormat == "gemma"
	case "qwen", "qwen3", "qwen3_moe", "qwen3_5", "qwen3_5_moe", "qwen3_next":
		valid = caps.ThinkingControl && caps.ReasoningFormat == "think"
	}
	if !valid {
		return modelCapabilities{}, errors.New("unsupported or inconsistent runtime family capabilities; no inference dispatched")
	}
	return caps, nil
}

type upstreamContextKey struct{}

func (g *Gateway) roleProfiles() map[string]any {
	profiles := map[string]any{}
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		if g.roles[role] == nil {
			continue
		}
		decision, _ := (RoleRouter{}).Select(context.Background(), RouteTask{Model: role})
		decision.MaxTokens = g.roleBudget(role, decision.MaxTokens)
		profiles[role] = map[string]any{"model": "sentinel-" + role, "max_tokens": decision.MaxTokens, "thinking": decision.Thinking}
	}
	return profiles
}

func (g *Gateway) rolesStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	client := &http.Client{Transport: g.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("runtime redirects are disabled") }}
	runtimes := map[string]any{}
	cache := map[string]any{}
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		endpoint := g.roles[role]
		if endpoint == nil {
			continue
		}
		if health, ok := cache[endpoint.String()]; ok {
			runtimes[role] = health
			continue
		}
		u := *endpoint
		u.Path = "/health"
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		res, err := client.Do(req)
		var health any
		if err != nil {
			health = map[string]string{"error": "runtime health unavailable"}
		} else {
			body, readErr := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
			res.Body.Close()
			if res.StatusCode != 200 || readErr != nil || len(body) > 64*1024 || !json.Valid(body) {
				health = map[string]string{"error": "invalid runtime health"}
			} else {
				health = json.RawMessage(body)
			}
		}
		cache[endpoint.String()] = health
		runtimes[role] = health
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"scope": "runtimes", "roles": g.roleProfiles(), "runtime_health": runtimes})
}
