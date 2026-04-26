package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/metrics"
)

// Layer represents the cache layer in the 7-layer optimization pipeline
type Layer struct {
	exactCache      map[string]string        // Exact dedup cache (Layer 1)
	semanticCache   *SemanticCache           // Semantic cache (Layer 5)
	metrics         *metrics.MetricsCollector // Metrics tracking
	config          *config.Config
	enabled         bool
	semanticEnabled bool
}

// NewLayer creates a new cache layer for the optimization pipeline
func NewLayer(cfg *config.Config, collector *metrics.MetricsCollector) (*Layer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config required for cache layer")
	}

	// Create embedding model for semantic cache
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding model: %w", err)
	}

	// Create semantic cache with configured thresholds
	semCache := NewSemanticCache(
		embedder,
		WithSimilarityThreshold(float32(cfg.Optimizations.SemanticCache.SimilarityThreshold)),
		WithFalsePositiveLimit(float32(cfg.Optimizations.SemanticCache.FalsePositiveLimit)),
		WithMaxCacheSize(cfg.Optimizations.SemanticCache.MaxCacheSize),
	)

	layer := &Layer{
		exactCache:      make(map[string]string),
		semanticCache:   semCache,
		metrics:         collector,
		config:          cfg,
		enabled:         true, // Enable both exact and semantic cache by default
		semanticEnabled: cfg.Optimizations.SemanticCache.Enabled,
	}

	return layer, nil
}

// Request represents a request to be cached/looked-up
type Request struct {
	Content string // Request content/query
	Tool    string // Tool type
	Params  map[string]interface{}
}

// Response represents a response to be cached
type Response struct {
	Content string        // Response content
	Error   error
	Tokens  int           // Tokens used
	Latency time.Duration
}

// LookupExact performs exact deduplication (Layer 1)
// Returns: (response, found, error)
func (l *Layer) LookupExact(req *Request) (string, bool, error) {
	if !l.enabled || req == nil {
		return "", false, nil
	}

	key := l.hashRequest(req)

	response, found := l.exactCache[key]
	if found && l.metrics != nil {
		l.metrics.RecordCacheHit()
	}

	return response, found, nil
}

// LookupSemantic performs semantic cache lookup (Layer 5)
// Returns: (response, similarity_score, found, error)
func (l *Layer) LookupSemantic(ctx context.Context, req *Request) (string, float32, bool, error) {
	if !l.semanticEnabled || req == nil {
		return "", 0, false, nil
	}

	response, similarity, found, err := l.semanticCache.Lookup(ctx, req.Content)
	if err != nil {
		return "", 0, false, err
	}

	if found && l.metrics != nil {
		l.metrics.RecordCacheHit()
		// Estimate 98% token savings for semantic cache hit
		if tokens, ok := req.Params["estimated_tokens"].(int); ok {
			estimatedSavings := int64(float32(tokens) * 0.98)
			l.metrics.RecordTokenSavings(estimatedSavings)
		}
	}

	return response, similarity, found, nil
}

// Store caches a response for both exact and semantic matching
func (l *Layer) Store(ctx context.Context, req *Request, resp *Response) error {
	if !l.enabled || req == nil || resp == nil || resp.Error != nil {
		return nil // Don't cache errors
	}

	key := l.hashRequest(req)

	// Store in exact dedup cache
	l.exactCache[key] = resp.Content

	// Store in semantic cache
	err := l.semanticCache.Store(ctx, key, req.Content, resp.Content)
	if err != nil {
		// Log but don't fail - semantic cache is optional optimization
		// In production, could emit a metric or log error here
		_ = err // Error stored but not critical for functionality
	}

	return nil
}

// RecordFalsePositive records when a cached response was wrong
func (l *Layer) RecordFalsePositive() {
	if l.semanticCache != nil {
		l.semanticCache.RecordFalsePositive()
	}
}

// GetStats returns cache statistics for monitoring
func (l *Layer) GetStats() LayerStats {
	stats := l.semanticCache.Stats()

	return LayerStats{
		ExactCacheSize:      len(l.exactCache),
		SemanticCacheSize:   stats.EntriesCount,
		SemanticHits:        stats.TotalSemanticHits,
		FalsePositives:      stats.FalsePositives,
		FalsePositiveRate:   stats.FalsePositiveRate,
		SimilarityThreshold: stats.SimilarityThreshold,
		CacheHealthy:        stats.IsHealthy,
	}
}

// LayerStats holds cache layer statistics
type LayerStats struct {
	ExactCacheSize      int
	SemanticCacheSize   int
	SemanticHits        int
	FalsePositives      int
	FalsePositiveRate   float32
	SimilarityThreshold float32
	CacheHealthy        bool
}

// Prune removes expired entries from both caches
func (l *Layer) Prune() {
	if l.semanticCache != nil {
		l.semanticCache.Prune()
	}
}

// Clear removes all cached entries
func (l *Layer) Clear() {
	l.exactCache = make(map[string]string)
	if l.semanticCache != nil {
		l.semanticCache.Clear()
	}
}

// hashRequest creates a deterministic hash of the request for exact matching
func (l *Layer) hashRequest(req *Request) string {
	hash := sha256.Sum256([]byte(req.Content + req.Tool))
	return fmt.Sprintf("%x", hash)
}

// CacheSafetyDecision determines if caching is safe for a query based on intent
type CacheSafetyDecision struct {
	Safe      bool
	Reason    string
	Confidence float32
}

// EvaluateCacheSafety checks if a request can be safely cached (used by intent layer)
func (l *Layer) EvaluateCacheSafety(intent string) CacheSafetyDecision {
	switch intent {
	case "quick_answer", "routine":
		return CacheSafetyDecision{
			Safe:       true,
			Reason:     "Intent is quick answer or routine - caching safe",
			Confidence: 0.95,
		}
	case "detailed_analysis", "learning", "follow_up":
		return CacheSafetyDecision{
			Safe:       false,
			Reason:     "Intent requires fresh response - caching unsafe",
			Confidence: 0.90,
		}
	case "cache_bypass":
		return CacheSafetyDecision{
			Safe:       false,
			Reason:     "User explicitly requested cache bypass",
			Confidence: 1.0,
		}
	default:
		return CacheSafetyDecision{
			Safe:       false,
			Reason:     "Unknown intent - default to fresh response",
			Confidence: 0.5,
		}
	}
}
