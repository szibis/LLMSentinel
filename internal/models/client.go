package models

import (
	"context"
	"fmt"

	"github.com/szibis/claude-escalate/internal/client"
	"github.com/szibis/claude-escalate/internal/provider"
)

// UnifiedClient provides a high-level interface for interacting with any LLM model
// It abstracts away the differences between Anthropic, local LLMs, and mock implementations
type UnifiedClient struct {
	registry      *ModelRegistry
	providers     map[string]provider.ClientProvider // provider.ProviderType -> provider.ClientProvider
	defaultModel  string
	lastUsedModel string
}

// NewUnifiedClient creates a unified client with a model registry
func NewUnifiedClient(registry *ModelRegistry) *UnifiedClient {
	return &UnifiedClient{
		registry:  registry,
		providers: make(map[string]provider.ClientProvider),
	}
}

// RegisterProvider adds a provider for a specific model
func (u *UnifiedClient) RegisterProvider(modelID string, prov provider.ClientProvider) error {
	_, exists := u.registry.GetModel(modelID)
	if !exists {
		return fmt.Errorf("model not found: %s", modelID)
	}

	u.providers[modelID] = prov
	return nil
}

// SetDefaultModel sets the model to use when no specific model is requested
func (u *UnifiedClient) SetDefaultModel(modelID string) error {
	_, exists := u.registry.GetModel(modelID)
	if !exists {
		return fmt.Errorf("model not found: %s", modelID)
	}
	u.defaultModel = modelID
	return nil
}

// CreateMessage sends a message using a specific model (or default if not specified)
func (u *UnifiedClient) CreateMessage(ctx context.Context, message string, modelID string) (*client.MessageResponse, error) {
	if modelID == "" {
		modelID = u.defaultModel
	}
	if modelID == "" {
		return nil, fmt.Errorf("no model specified and no default set")
	}

	prov, exists := u.providers[modelID]
	if !exists {
		return nil, fmt.Errorf("no provider registered for model: %s", modelID)
	}

	// Build request
	req := &client.MessageRequest{
		Model:     modelID,
		MaxTokens: 4096,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: message},
		},
	}

	u.lastUsedModel = modelID
	return prov.CreateMessage(ctx, req)
}

// CreateMessageWithContext sends a message with system context
func (u *UnifiedClient) CreateMessageWithContext(ctx context.Context, message, systemMessage string, modelID string) (*client.MessageResponse, error) {
	if modelID == "" {
		modelID = u.defaultModel
	}
	if modelID == "" {
		return nil, fmt.Errorf("no model specified and no default set")
	}

	prov, exists := u.providers[modelID]
	if !exists {
		return nil, fmt.Errorf("no provider registered for model: %s", modelID)
	}

	// Build request with system message
	req := &client.MessageRequest{
		Model:     modelID,
		MaxTokens: 4096,
		System:    systemMessage,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: message},
		},
	}

	u.lastUsedModel = modelID
	return prov.CreateMessage(ctx, req)
}

// RoutedCreateMessage uses the registry to select the best model for the task
func (u *UnifiedClient) RoutedCreateMessage(ctx context.Context, message string, routingStrategy RoutingStrategy) (*client.MessageResponse, error) {
	modelID, err := routingStrategy.SelectModel(u.registry)
	if err != nil {
		return nil, fmt.Errorf("model routing failed: %w", err)
	}

	return u.CreateMessage(ctx, message, modelID)
}

// LastUsedModel returns the ID of the last model used
func (u *UnifiedClient) LastUsedModel() string {
	return u.lastUsedModel
}

// RoutingStrategy defines how to select a model for a task
type RoutingStrategy interface {
	SelectModel(registry *ModelRegistry) (string, error)
}

// CapabilityRouter selects the cheapest model with a specific capability
type CapabilityRouter struct {
	Capability ModelCapability
	InputSize  int
	OutputSize int
}

func (r *CapabilityRouter) SelectModel(registry *ModelRegistry) (string, error) {
	model := registry.FindCheapestWithCapability(r.Capability, r.InputSize, r.OutputSize)
	if model == nil {
		return "", fmt.Errorf("no model found with capability: %s", r.Capability)
	}
	return model.ID, nil
}

// CostOptimizedRouter selects the cheapest available model
type CostOptimizedRouter struct {
	InputSize  int
	OutputSize int
}

func (r *CostOptimizedRouter) SelectModel(registry *ModelRegistry) (string, error) {
	model := registry.FindCheapest(r.InputSize, r.OutputSize)
	if model == nil {
		return "", fmt.Errorf("no model available")
	}
	return model.ID, nil
}

// PreferredModelRouter tries preferred models in order
type PreferredModelRouter struct {
	Preferences []string // Model IDs in preference order
}

func (r *PreferredModelRouter) SelectModel(registry *ModelRegistry) (string, error) {
	for _, modelID := range r.Preferences {
		model, exists := registry.GetModel(modelID)
		if exists && model.Enabled {
			return modelID, nil
		}
	}
	return "", fmt.Errorf("no preferred models available")
}

// CapabilityAndCostRouter combines capability and cost optimization
type CapabilityAndCostRouter struct {
	Capabilities []ModelCapability
	InputSize    int
	OutputSize   int
}

func (r *CapabilityAndCostRouter) SelectModel(registry *ModelRegistry) (string, error) {
	candidates := registry.FindByCapabilities(r.Capabilities...)
	if len(candidates) == 0 {
		return "", fmt.Errorf("no model found with required capabilities")
	}

	// Select cheapest among candidates
	var cheapest *ModelInfo
	minCost := float64(0)

	for _, model := range candidates {
		cost := float64(r.InputSize) * model.CostPer1KInput / 1000
		cost += float64(r.OutputSize) * model.CostPer1KOutput / 1000

		if cheapest == nil || cost < minCost {
			cheapest = model
			minCost = cost
		}
	}

	return cheapest.ID, nil
}
