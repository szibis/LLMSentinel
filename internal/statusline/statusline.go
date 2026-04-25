package statusline

import (
	"fmt"
	"time"
)

// StatuslineSource is the interface for any statusline data provider.
type StatuslineSource interface {
	// Name returns identifier for this source (e.g., "barista", "webhook", "envvar").
	Name() string

	// Poll fetches current metrics from this source.
	Poll() (StatuslineData, error)

	// IsAvailable checks if this source is currently active/configured.
	IsAvailable() bool

	// Priority returns fetch priority (0=highest, 10=lowest).
	// Barista: 1 (primary)
	// Claude native: 2 (fallback to native metrics)
	// Webhook: 3 (custom integration)
	// File: 4 (polling)
	// EnvVar: 5 (last resort)
	Priority() int
}

// StatuslineData is the metrics returned by a source.
type StatuslineData struct {
	Source                  string        // Which source provided this
	Timestamp               time.Time     // When these metrics were captured
	InputTokens             int           // Input tokens used so far
	OutputTokens            int           // Output tokens used so far
	CacheHitTokens          int           // Tokens from cache hits
	CacheCreationTokens     int           // Tokens for cache creation
	ContextWindowUsage      int           // Percent of context window used
	Model                   string        // Current model
	IsCaching               bool          // Is prompt caching active?
	CacheFillPercentage     float64       // Percent of cache filled (0.0-1.0)
	EstimatedCompletionTime time.Duration // Estimated time to completion
}

// Registry manages multiple statusline sources with fallback.
type Registry struct {
	sources []StatuslineSource
	timeout time.Duration
}

// NewRegistry creates a registry with optional timeout.
func NewRegistry(timeout time.Duration) *Registry {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return &Registry{
		sources: []StatuslineSource{},
		timeout: timeout,
	}
}

// Register adds a source to the registry.
func (r *Registry) Register(source StatuslineSource) {
	if source != nil && source.IsAvailable() {
		r.sources = append(r.sources, source)
	}
}

// GetBest returns the best available source (highest priority).
func (r *Registry) GetBest() StatuslineSource {
	if len(r.sources) == 0 {
		return nil
	}

	// Sort by priority (lower number = higher priority)
	best := r.sources[0]
	bestPriority := best.Priority()

	for _, source := range r.sources[1:] {
		if source.IsAvailable() && source.Priority() < bestPriority {
			best = source
			bestPriority = source.Priority()
		}
	}

	if !best.IsAvailable() {
		return nil
	}

	return best
}

// Poll fetches metrics from best available source.
func (r *Registry) Poll() (StatuslineData, error) {
	source := r.GetBest()
	if source == nil {
		return StatuslineData{}, fmt.Errorf("no available statusline sources")
	}

	// Create channel for result with timeout
	done := make(chan StatuslineData, 1)
	errChan := make(chan error, 1)

	go func() {
		data, err := source.Poll()
		if err != nil {
			errChan <- err
		} else {
			done <- data
		}
	}()

	select {
	case data := <-done:
		return data, nil
	case err := <-errChan:
		return StatuslineData{}, err
	case <-time.After(r.timeout):
		return StatuslineData{}, fmt.Errorf("statusline poll timeout (%s) from source: %s", r.timeout, source.Name())
	}
}

// GetSources returns all registered sources.
func (r *Registry) GetSources() []StatuslineSource {
	return r.sources
}

// Health checks which sources are available.
func (r *Registry) Health() map[string]bool {
	health := make(map[string]bool)
	for _, source := range r.sources {
		health[source.Name()] = source.IsAvailable()
	}
	return health
}
