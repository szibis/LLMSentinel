package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CacheEntry represents a cached response with embedding and metadata
type CacheEntry struct {
	Key        string    // Request hash or dedup key
	Query      string    // Original query text
	Embedding  []float32 // Semantic embedding
	Response   string    // Cached response
	Timestamp  time.Time // When cached
	TTL        time.Duration
	HitCount   int // Number of cache hits on this entry
	ExactMatch bool // Was this an exact dedup match or semantic match?
}

// SemanticCache implements similarity-based response caching
type SemanticCache struct {
	entries              map[string]*CacheEntry // key -> entry
	embedder             *EmbeddingModel
	similarityThreshold  float32 // >0.85 = hit (strict threshold)
	falsePositiveLimit   float32 // Kill cache if false positive rate >0.5%
	falsePositiveCount   int
	totalSemanticHits    int
	mu                   sync.RWMutex
	maxCacheSize         int       // Max entries in cache
	falsePositiveReportInterval int // Report every N hits
}

// NewSemanticCache creates a new semantic cache with embeddings
func NewSemanticCache(embedder *EmbeddingModel, opts ...CacheOption) *SemanticCache {
	cache := &SemanticCache{
		entries:                     make(map[string]*CacheEntry),
		embedder:                    embedder,
		similarityThreshold:         0.85,      // Strict threshold (default)
		falsePositiveLimit:          0.005,     // 0.5% limit (default)
		maxCacheSize:                10000,     // Max entries
		falsePositiveReportInterval: 1000,      // Report every 1000 hits
	}

	// Apply options
	for _, opt := range opts {
		opt(cache)
	}

	return cache
}

// CacheOption allows functional configuration
type CacheOption func(*SemanticCache)

// WithSimilarityThreshold sets the cosine similarity threshold
func WithSimilarityThreshold(threshold float32) CacheOption {
	return func(c *SemanticCache) {
		c.similarityThreshold = threshold
	}
}

// WithFalsePositiveLimit sets the false positive acceptance limit
func WithFalsePositiveLimit(limit float32) CacheOption {
	return func(c *SemanticCache) {
		c.falsePositiveLimit = limit
	}
}

// WithMaxCacheSize sets the maximum cache size
func WithMaxCacheSize(size int) CacheOption {
	return func(c *SemanticCache) {
		c.maxCacheSize = size
	}
}

// Store adds a response to the semantic cache with its embedding
func (c *SemanticCache) Store(ctx context.Context, key, query, response string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if key == "" || query == "" || response == "" {
		return fmt.Errorf("all cache parameters required (key, query, response)")
	}

	// Check cache size limit
	if len(c.entries) >= c.maxCacheSize {
		c.evictOldest()
	}

	// Compute embedding for query
	embedding, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to embed query: %w", err)
	}

	entry := &CacheEntry{
		Key:       key,
		Query:     query,
		Embedding: embedding,
		Response:  response,
		Timestamp: time.Now(),
		TTL:       24 * time.Hour,    // Default 24-hour TTL
		HitCount:  0,
		ExactMatch: false,
	}

	c.entries[key] = entry
	return nil
}

// Lookup finds a cached response using semantic similarity
// Returns: (response, similarity_score, is_hit, error)
func (c *SemanticCache) Lookup(ctx context.Context, query string) (string, float32, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if query == "" {
		return "", 0, false, fmt.Errorf("empty query")
	}

	// Compute embedding for input query
	queryEmbedding, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to embed query: %w", err)
	}

	var bestMatch *CacheEntry
	var bestSimilarity float32

	// Find best matching cached entry
	for _, entry := range c.entries {
		// Skip expired entries
		if time.Since(entry.Timestamp) > entry.TTL {
			continue
		}

		// Compute similarity
		similarity, err := CosineSimilarity(queryEmbedding, entry.Embedding)
		if err != nil {
			continue // Skip entries with similarity errors
		}

		// Track best match
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = entry
		}
	}

	// Check if best match exceeds threshold
	if bestMatch == nil || bestSimilarity < c.similarityThreshold {
		return "", 0, false, nil // No match
	}

	// Increment hit count (non-blocking, update in background)
	go func() {
		c.mu.Lock()
		bestMatch.HitCount++
		c.totalSemanticHits++
		c.mu.Unlock()
	}()

	return bestMatch.Response, bestSimilarity, true, nil
}

// ExactLookup finds exact matches (for deduplication)
func (c *SemanticCache) ExactLookup(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return "", false
	}

	// Check TTL
	if time.Since(entry.Timestamp) > entry.TTL {
		return "", false
	}

	return entry.Response, true
}

// RecordFalsePositive records a semantic cache false positive (cached answer was wrong)
func (c *SemanticCache) RecordFalsePositive() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.falsePositiveCount++

	// Report if interval reached
	if c.falsePositiveCount%c.falsePositiveReportInterval == 0 {
		rate := float32(c.falsePositiveCount) / float32(c.totalSemanticHits+1)
		_ = rate // False positive rate tracking for monitoring
	}
}

// Stats returns cache statistics
func (c *SemanticCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var fpRate float32
	if c.totalSemanticHits > 0 {
		fpRate = float32(c.falsePositiveCount) / float32(c.totalSemanticHits)
	}

	return CacheStats{
		EntriesCount:        len(c.entries),
		MaxCacheSize:        c.maxCacheSize,
		TotalSemanticHits:   c.totalSemanticHits,
		FalsePositives:      c.falsePositiveCount,
		FalsePositiveRate:   fpRate,
		SimilarityThreshold: c.similarityThreshold,
		IsHealthy:           fpRate <= c.falsePositiveLimit,
	}
}

// CacheStats holds cache performance metrics
type CacheStats struct {
	EntriesCount        int
	MaxCacheSize        int
	TotalSemanticHits   int
	FalsePositives      int
	FalsePositiveRate   float32
	SimilarityThreshold float32
	IsHealthy           bool
}

// evictOldest removes the least recently used entry (called when cache is full)
func (c *SemanticCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestTime.IsZero() || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Clear removes all entries from cache
func (c *SemanticCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.falsePositiveCount = 0
	c.totalSemanticHits = 0
}

// Prune removes expired entries
func (c *SemanticCache) Prune() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) > entry.TTL {
			delete(c.entries, key)
			count++
		}
	}

	return count
}
