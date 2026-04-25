package statusline

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvVarSource reads metrics from environment variables.
// Expected variables:
// - CLAUDE_TOKENS_INPUT
// - CLAUDE_TOKENS_OUTPUT
// - CLAUDE_CACHE_HIT_TOKENS
// - CLAUDE_CACHE_CREATION_TOKENS
// - CLAUDE_MODEL
// - CLAUDE_CONTEXT_USAGE
type EnvVarSource struct {
	enabled bool
}

// NewEnvVarSource creates an environment variable source.
func NewEnvVarSource() *EnvVarSource {
	// Always enabled if any relevant env var is set
	_, enabled := os.LookupEnv("CLAUDE_TOKENS_ACTUAL")
	return &EnvVarSource{
		enabled: enabled,
	}
}

// Name returns the source name.
func (evs *EnvVarSource) Name() string {
	return "envvar"
}

// IsAvailable checks if env vars are set.
func (evs *EnvVarSource) IsAvailable() bool {
	// Check if at least one token env var is set
	_, hasInput := os.LookupEnv("CLAUDE_TOKENS_INPUT")
	_, hasOutput := os.LookupEnv("CLAUDE_TOKENS_OUTPUT")
	_, hasActual := os.LookupEnv("CLAUDE_TOKENS_ACTUAL")
	return hasInput || hasOutput || hasActual
}

// Priority returns EnvVar priority (lowest: 5).
func (evs *EnvVarSource) Priority() int {
	return 5
}

// Poll reads metrics from environment variables.
func (evs *EnvVarSource) Poll() (StatuslineData, error) {
	if !evs.IsAvailable() {
		return StatuslineData{}, fmt.Errorf("no token env vars available")
	}

	data := StatuslineData{
		Source:    evs.Name(),
		Timestamp: time.Now(),
	}

	// Try CLAUDE_TOKENS_ACTUAL first (convenience var)
	if actual := os.Getenv("CLAUDE_TOKENS_ACTUAL"); actual != "" {
		if val, err := strconv.Atoi(actual); err == nil {
			// Split between input/output as 25/75
			data.InputTokens = val / 4
			data.OutputTokens = val * 3 / 4
		}
	}

	// Override with specific vars if set
	if input := os.Getenv("CLAUDE_TOKENS_INPUT"); input != "" {
		if val, err := strconv.Atoi(input); err == nil {
			data.InputTokens = val
		}
	}

	if output := os.Getenv("CLAUDE_TOKENS_OUTPUT"); output != "" {
		if val, err := strconv.Atoi(output); err == nil {
			data.OutputTokens = val
		}
	}

	if cacheHit := os.Getenv("CLAUDE_CACHE_HIT_TOKENS"); cacheHit != "" {
		if val, err := strconv.Atoi(cacheHit); err == nil {
			data.CacheHitTokens = val
		}
	}

	if cacheCreate := os.Getenv("CLAUDE_CACHE_CREATION_TOKENS"); cacheCreate != "" {
		if val, err := strconv.Atoi(cacheCreate); err == nil {
			data.CacheCreationTokens = val
		}
	}

	if model := os.Getenv("CLAUDE_MODEL"); model != "" {
		data.Model = model
	}

	if contextUsage := os.Getenv("CLAUDE_CONTEXT_USAGE"); contextUsage != "" {
		if val, err := strconv.Atoi(contextUsage); err == nil {
			data.ContextWindowUsage = val
		}
	}

	return data, nil
}
