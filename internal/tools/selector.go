package tools

import (
	"fmt"
	"math/rand/v2"
)

// Selector implements intent-based tool selection with weights and fallbacks
type Selector struct {
	// Intent-to-tools mapping
	intentToolMap map[string][]string
	// Tool preferences
	customToolWeight float64
	builtinWeight    float64
	fallbackEnabled  bool
}

// NewSelector creates a new tool selector
func NewSelector() *Selector {
	return &Selector{
		intentToolMap:    defaultIntentToolMap(),
		customToolWeight: 0.9,
		builtinWeight:    0.7,
		fallbackEnabled:  true,
	}
}

// SelectForIntent returns tools selected for a given intent
func (s *Selector) SelectForIntent(intent string) ([]string, error) {
	if intent == "" {
		return []string{}, fmt.Errorf("empty intent")
	}

	tools, exists := s.intentToolMap[intent]
	if !exists || len(tools) == 0 {
		// Return default/fallback tools if available
		if s.fallbackEnabled {
			return []string{"rtk_git", "lsp_lookup"}, nil
		}
		return []string{}, fmt.Errorf("no tools mapped for intent: %s", intent)
	}

	return tools, nil
}

// AddIntentMapping adds or updates an intent-to-tools mapping
func (s *Selector) AddIntentMapping(intent string, tools []string) {
	s.intentToolMap[intent] = tools
}

// SetWeights updates tool preference weights
func (s *Selector) SetWeights(customWeight, builtinWeight float64) {
	if customWeight >= 0 && customWeight <= 1.0 {
		s.customToolWeight = customWeight
	}
	if builtinWeight >= 0 && builtinWeight <= 1.0 {
		s.builtinWeight = builtinWeight
	}
}

// SetFallback enables/disables fallback tools when intent has no mapping
func (s *Selector) SetFallback(enabled bool) {
	s.fallbackEnabled = enabled
}

// defaultIntentToolMap returns the default intent-to-tools mapping from spec
func defaultIntentToolMap() map[string][]string {
	return map[string][]string{
		// Phase 2: Context Gathering intents
		"quick_answer": {
			"scrapling_web",
			"rtk_git",
		},
		"code_search": {
			"lsp_lookup",
			"rtk_grep",
		},
		"find_definition": {
			"lsp_lookup",
		},
		"find_references": {
			"lsp_lookup",
		},

		// Phase 4: Response Enhancement intents
		"detailed_analysis": {
			"custom_analyzer",
		},
		"security_check": {
			"security_scanner",
		},
		"performance_analysis": {
			"profiler_tool",
		},
		"documentation": {
			"doc_generator",
		},

		// Fallback for unmapped intents
		"general": {
			"rtk_git",
			"lsp_lookup",
			"scrapling_web",
		},
		"unknown": {
			"rtk_git",
		},
	}
}

// SelectByWeight implements weighted tool selection (currently not used but available for future)
func (s *Selector) SelectByWeight(tools []string, weights map[string]float64) string {
	if len(tools) == 0 {
		return ""
	}

	// Calculate total weight
	totalWeight := 0.0
	for _, tool := range tools {
		if weight, ok := weights[tool]; ok {
			totalWeight += weight
		}
	}

	if len(tools) == 0 {
		return ""
	}

	if totalWeight == 0 {
		// Fallback to random selection
		// nolint:gosec // G404 suppressed: non-cryptographic use case
		return tools[rand.IntN(len(tools))]
	}

	// Weighted random selection
	//nolint:gosec
	r := rand.Float64() * totalWeight
	current := 0.0
	for _, tool := range tools {
		weight := weights[tool]
		current += weight
		if r <= current {
			return tool
		}
	}

	return tools[len(tools)-1]
}
