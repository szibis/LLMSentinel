package models

import (
	"fmt"
	"sync"
)

// ModelCapability represents what a model can do
type ModelCapability string

const (
	CapabilityChat          ModelCapability = "chat"
	CapabilityBatch         ModelCapability = "batch"
	CapabilityEmbeddings    ModelCapability = "embeddings"
	CapabilityReasoning     ModelCapability = "reasoning"
	CapabilityCodeAnalysis  ModelCapability = "code_analysis"
	CapabilityFunctionCall  ModelCapability = "function_call"
	CapabilityStreaming     ModelCapability = "streaming"
)

// ModelInfo contains metadata about an available model
type ModelInfo struct {
	ID              string
	Provider        string // "anthropic", "local", "mock"
	Name            string
	DisplayName     string
	Capabilities    []ModelCapability
	MaxTokens       int
	CostPer1KInput  float64 // in cents
	CostPer1KOutput float64 // in cents
	ContextWindow   int
	Enabled         bool
	Description     string
}

// ModelRegistry maintains a registry of available models
type ModelRegistry struct {
	mu     sync.RWMutex
	models map[string]*ModelInfo
	order  []string // maintains insertion order
}

// NewModelRegistry creates a new model registry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: make(map[string]*ModelInfo),
		order:  []string{},
	}
}

// RegisterModel adds or updates a model in the registry
func (r *ModelRegistry) RegisterModel(model *ModelInfo) error {
	if model.ID == "" {
		return fmt.Errorf("model ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Track new models
	if _, exists := r.models[model.ID]; !exists {
		r.order = append(r.order, model.ID)
	}

	r.models[model.ID] = model
	return nil
}

// GetModel retrieves a model by ID
func (r *ModelRegistry) GetModel(id string) (*ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	model, exists := r.models[id]
	return model, exists
}

// GetAllModels returns all registered models
func (r *ModelRegistry) GetAllModels() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelInfo, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.models[id])
	}
	return result
}

// GetEnabledModels returns only enabled models
func (r *ModelRegistry) GetEnabledModels() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
	for _, id := range r.order {
		model := r.models[id]
		if model.Enabled {
			result = append(result, model)
		}
	}
	return result
}

// FindByCapability returns models that have a specific capability
func (r *ModelRegistry) FindByCapability(cap ModelCapability) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
	for _, id := range r.order {
		model := r.models[id]
		if !model.Enabled {
			continue
		}

		for _, c := range model.Capabilities {
			if c == cap {
				result = append(result, model)
				break
			}
		}
	}
	return result
}

// FindByCapabilities returns models that have all specified capabilities
func (r *ModelRegistry) FindByCapabilities(caps ...ModelCapability) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
nextModel:
	for _, id := range r.order {
		model := r.models[id]
		if !model.Enabled {
			continue
		}

		// Check if model has all required capabilities
		for _, requiredCap := range caps {
			found := false
			for _, modelCap := range model.Capabilities {
				if modelCap == requiredCap {
					found = true
					break
				}
			}
			if !found {
				continue nextModel
			}
		}

		result = append(result, model)
	}
	return result
}

// FindCheapest returns the cheapest model for input/output work
func (r *ModelRegistry) FindCheapest(inputSize, outputSize int) *ModelInfo {
	candidates := r.GetEnabledModels()
	if len(candidates) == 0 {
		return nil
	}

	var cheapest *ModelInfo
	minCost := float64(0)

	for _, model := range candidates {
		cost := float64(inputSize) * model.CostPer1KInput / 1000
		cost += float64(outputSize) * model.CostPer1KOutput / 1000

		if cheapest == nil || cost < minCost {
			cheapest = model
			minCost = cost
		}
	}

	return cheapest
}

// FindCheapestWithCapability returns the cheapest model with a capability
func (r *ModelRegistry) FindCheapestWithCapability(cap ModelCapability, inputSize, outputSize int) *ModelInfo {
	candidates := r.FindByCapability(cap)
	if len(candidates) == 0 {
		return nil
	}

	var cheapest *ModelInfo
	minCost := float64(0)

	for _, model := range candidates {
		cost := float64(inputSize) * model.CostPer1KInput / 1000
		cost += float64(outputSize) * model.CostPer1KOutput / 1000

		if cheapest == nil || cost < minCost {
			cheapest = model
			minCost = cost
		}
	}

	return cheapest
}

// ListByProvider returns all models from a specific provider
func (r *ModelRegistry) ListByProvider(provider string) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
	for _, id := range r.order {
		model := r.models[id]
		if model.Provider == provider && model.Enabled {
			result = append(result, model)
		}
	}
	return result
}

// DefaultRegistry provides a singleton instance with common models
var defaultRegistry *ModelRegistry
var registryOnce sync.Once

// DefaultModelRegistry returns the global model registry with common models pre-registered
func DefaultModelRegistry() *ModelRegistry {
	registryOnce.Do(func() {
		defaultRegistry = NewModelRegistry()
		registerDefaultModels(defaultRegistry)
	})
	return defaultRegistry
}

// registerDefaultModels registers commonly used models
func registerDefaultModels(r *ModelRegistry) {
	// Anthropic models
	r.RegisterModel(&ModelInfo{
		ID:          "claude-opus",
		Provider:    "anthropic",
		Name:        "claude-opus",
		DisplayName: "Claude Opus",
		Capabilities: []ModelCapability{
			CapabilityChat,
			CapabilityBatch,
			CapabilityReasoning,
			CapabilityCodeAnalysis,
			CapabilityFunctionCall,
		},
		MaxTokens:       200000,
		ContextWindow:   200000,
		CostPer1KInput:  3.0,  // $0.03 per 1K input tokens
		CostPer1KOutput: 15.0, // $0.15 per 1K output tokens
		Enabled:         true,
		Description:     "Most capable model for complex tasks",
	})

	r.RegisterModel(&ModelInfo{
		ID:          "claude-sonnet",
		Provider:    "anthropic",
		Name:        "claude-sonnet",
		DisplayName: "Claude Sonnet",
		Capabilities: []ModelCapability{
			CapabilityChat,
			CapabilityBatch,
			CapabilityCodeAnalysis,
			CapabilityFunctionCall,
		},
		MaxTokens:       200000,
		ContextWindow:   200000,
		CostPer1KInput:  3.0,
		CostPer1KOutput: 15.0,
		Enabled:         true,
		Description:     "Balanced model for most tasks",
	})

	r.RegisterModel(&ModelInfo{
		ID:          "claude-haiku",
		Provider:    "anthropic",
		Name:        "claude-haiku",
		DisplayName: "Claude Haiku",
		Capabilities: []ModelCapability{
			CapabilityChat,
			CapabilityBatch,
		},
		MaxTokens:       200000,
		ContextWindow:   200000,
		CostPer1KInput:  0.8,
		CostPer1KOutput: 4.0,
		Enabled:         true,
		Description:     "Fast, cost-effective model for simple tasks",
	})

	// Mock models (for testing)
	r.RegisterModel(&ModelInfo{
		ID:          "mock-model",
		Provider:    "mock",
		Name:        "mock-model",
		DisplayName: "Mock Model (Testing)",
		Capabilities: []ModelCapability{
			CapabilityChat,
			CapabilityBatch,
		},
		MaxTokens:       10000,
		ContextWindow:   10000,
		CostPer1KInput:  0.0,
		CostPer1KOutput: 0.0,
		Enabled:         true,
		Description:     "Mock model for testing and development",
	})

	// Local LLM models
	r.RegisterModel(&ModelInfo{
		ID:          "local-llm",
		Provider:    "local",
		Name:        "local-llm",
		DisplayName: "Local LLM",
		Capabilities: []ModelCapability{
			CapabilityChat,
		},
		MaxTokens:       4096,
		ContextWindow:   4096,
		CostPer1KInput:  0.0,
		CostPer1KOutput: 0.0,
		Enabled:         false, // Disabled by default until configured
		Description:     "Local language model (LM Studio, Ollama, etc.)",
	})
}

// UpdateModelStatus enables or disables a model
func (r *ModelRegistry) UpdateModelStatus(id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	model, exists := r.models[id]
	if !exists {
		return fmt.Errorf("model not found: %s", id)
	}

	model.Enabled = enabled
	return nil
}

// Count returns the number of models in the registry
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.models)
}

// EnabledCount returns the number of enabled models
func (r *ModelRegistry) EnabledCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, id := range r.order {
		if r.models[id].Enabled {
			count++
		}
	}
	return count
}
