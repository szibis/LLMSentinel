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
func finalRoleText(text string, thinking bool) (string, error) {
	if !thinking {
		return text, nil
	}
	before, after, ok := strings.Cut(text, "</think>")
	if !ok || strings.Contains(before, "</think>") || strings.Contains(after, "<think>") || strings.Contains(after, "</think>") || strings.TrimSpace(after) == "" {
		return "", invalidOutput("local Opus reasoning did not complete; no partial answer or tool input accepted")
	}
	return strings.TrimSpace(after), nil
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
