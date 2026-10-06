package localgateway

import (
	"context"
	"fmt"
)

// DecisionRouter is the seam for a trained Jes decision service. A decision
// selects a configured local model, never a provider credential or remote URL.
type DecisionRouter interface {
	Name() string
	Select(context.Context, RouteTask) (RouteDecision, error)
}
type RouteTask struct {
	Model     string
	HasTools  bool
	Messages  int
	Client    string
	MaxTokens int
}
type RouteDecision struct {
	Model     string
	Reason    string
	Role      string
	MaxTokens int
	Thinking  bool
}

// RoleRouter maps client roles, not task keywords, to configured local profiles.
// Sonnet and Opus share an artifact; Opus changes inference effort.
type RoleRouter struct{}

func (RoleRouter) Name() string { return "claude-roles; two local model artifacts; Jes not trained" }
func (RoleRouter) Select(_ context.Context, task RouteTask) (RouteDecision, error) {
	role := task.Model
	switch role {
	case "sentinel-haiku", "haiku":
		role = "haiku"
	case "sentinel-sonnet", "sonnet", "local":
		role = "sonnet"
	case "sentinel-opus", "opus":
		role = "opus"
	default:
		return RouteDecision{}, fmt.Errorf("unknown local model role %q; use sentinel-haiku, sentinel-sonnet or sentinel-opus", task.Model)
	}
	route := RouteDecision{Model: "local", Role: role, Reason: "explicit Claude role: " + role}
	switch role {
	case "haiku":
		route.MaxTokens = 1024
	case "sonnet":
		route.MaxTokens = 4096
	case "opus":
		route.MaxTokens = 8192
		route.Thinking = true
	}
	return route, nil
}

// LocalRouter is an explicit bootstrap policy, not Jes model inference.
type LocalRouter struct{}

func (LocalRouter) Name() string { return "bootstrap-single-local-model; Jes not trained/connected" }
func (LocalRouter) Select(_ context.Context, _ RouteTask) (RouteDecision, error) {
	return RouteDecision{Model: "local", Reason: "single configured MLX-Flash backend"}, nil
}
