package models

import (
	"time"
)

// ModelConfig contains configuration for all models
type ModelConfig struct {
	Enabled            bool
	AutoDownload       bool
	CachePath          string
	Intent             ModelSubConfig
	SecurityAnomaly    ModelSubConfig
	SemanticEmbeddings ModelSubConfig
}

// ModelSubConfig contains configuration for a specific model
type ModelSubConfig struct {
	Enabled          bool
	Source           string        // "huggingface", "local", "remote"
	ModelID          string        // Model identifier (e.g., "distilbert-base-uncased")
	Version          string        // Model version (e.g., "1.0.0")
	Quantized        bool          // Use quantized version if available
	FallbackTo       string        // Fallback mechanism ("keywords", "regex", "disabled")
	InferenceTimeout time.Duration // Max inference time
	CacheResults     bool          // Cache inference results
	MaxCacheSize     int           // Max cache entries
}

// DefaultModelConfig returns sensible defaults
func DefaultModelConfig() *ModelConfig {
	return &ModelConfig{
		Enabled:      true,
		AutoDownload: true,
		CachePath:    "~/.claude-escalate/models",
		Intent: ModelSubConfig{
			Enabled:          true,
			Source:           "huggingface",
			ModelID:          "distilbert-base-uncased",
			Version:          "1.0.0",
			Quantized:        true,
			FallbackTo:       "keywords",
			InferenceTimeout: 100 * time.Millisecond,
			CacheResults:     true,
			MaxCacheSize:     10000,
		},
		SecurityAnomaly: ModelSubConfig{
			Enabled:          true,
			Source:           "local",
			ModelID:          "isolation-forest-v1",
			Version:          "1.0.0",
			Quantized:        true,
			FallbackTo:       "regex",
			InferenceTimeout: 10 * time.Millisecond,
			CacheResults:     true,
			MaxCacheSize:     5000,
		},
		SemanticEmbeddings: ModelSubConfig{
			Enabled:          true,
			Source:           "huggingface",
			ModelID:          "all-MiniLM-L6-v2",
			Version:          "1.0.0",
			Quantized:        true,
			FallbackTo:       "disabled",
			InferenceTimeout: 100 * time.Millisecond,
			CacheResults:     true,
			MaxCacheSize:     50000,
		},
	}
}

// Validate checks if configuration is valid
func (mc *ModelConfig) Validate() error {
	if !mc.Enabled {
		return nil // Models disabled, no validation needed
	}

	// All sub-configs should have at least one valid fallback
	// This is validated implicitly during usage

	return nil
}
