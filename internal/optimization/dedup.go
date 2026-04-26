package optimization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// RequestDeduplicator detects duplicate requests beyond exact cache matches
type RequestDeduplicator struct {
	// Map of request hash to cached response
	cache map[string]string
	mu    sync.RWMutex
	// Statistics
	stats DedupStats
}

// DedupStats tracks deduplication performance
type DedupStats struct {
	TotalLookups int
	CacheHits    int
	CacheMisses  int
}

// NewRequestDeduplicator creates a new request deduplicator
func NewRequestDeduplicator() *RequestDeduplicator {
	return &RequestDeduplicator{
		cache: make(map[string]string),
	}
}

// Hash computes a SHA256 hash of the request for deduplication
func (rd *RequestDeduplicator) Hash(req *PipelineRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("request cannot be nil")
	}

	// Create normalized request structure for hashing
	// Include: query, intent, tool, params
	hashData := map[string]interface{}{
		"query":  req.Query,
		"intent": req.Intent,
		"tool":   req.Tool,
		"params": normalizeParams(req.Params),
	}

	jsonData, err := json.Marshal(hashData)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:]), nil
}

// Lookup retrieves a cached response by hash
func (rd *RequestDeduplicator) Lookup(ctx context.Context, hash string) (string, bool) {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	rd.stats.TotalLookups++

	response, found := rd.cache[hash]
	if found {
		rd.stats.CacheHits++
	} else {
		rd.stats.CacheMisses++
	}

	return response, found
}

// Store caches a response by hash
func (rd *RequestDeduplicator) Store(ctx context.Context, hash string, response string) error {
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.cache[hash] = response
	return nil
}

// Clear removes all cached responses
func (rd *RequestDeduplicator) Clear(ctx context.Context) error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.cache = make(map[string]string)
	rd.stats = DedupStats{}
	return nil
}

// GetStats returns deduplication statistics
func (rd *RequestDeduplicator) GetStats() map[string]interface{} {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	hitRate := 0.0
	if rd.stats.TotalLookups > 0 {
		hitRate = float64(rd.stats.CacheHits) / float64(rd.stats.TotalLookups) * 100
	}

	return map[string]interface{}{
		"total_lookups": rd.stats.TotalLookups,
		"cache_hits":    rd.stats.CacheHits,
		"cache_misses":  rd.stats.CacheMisses,
		"hit_rate":      hitRate,
		"cached_items":  len(rd.cache),
	}
}

// normalizeParams creates a consistent representation of params for hashing
func normalizeParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return make(map[string]interface{})
	}

	// Create a copy and normalize values
	normalized := make(map[string]interface{})
	for k, v := range params {
		// Convert slices to sorted lists for consistent hashing
		if slice, ok := v.([]interface{}); ok {
			// Simple normalization: stringify for hashing
			normalized[k] = fmt.Sprintf("%v", slice)
		} else {
			normalized[k] = v
		}
	}

	return normalized
}
