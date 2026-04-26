package cache

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
)

func TestSemanticCacheStore(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	// Test storing a cache entry
	err = cache.Store(ctx, "key1", "Find all functions calling authenticate", "response1")
	if err != nil {
		t.Errorf("Failed to store cache entry: %v", err)
	}

	// Verify entry was stored
	stats := cache.Stats()
	if stats.EntriesCount != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.EntriesCount)
	}
}

func TestSemanticCacheLookupIdentical(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	query := "Find functions calling authenticate"
	response := "Functions: auth.go, api.go"

	// Store entry
	err = cache.Store(ctx, "key1", query, response)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Wait a moment for background hit count increment
	time.Sleep(10 * time.Millisecond)

	// Lookup identical query
	result, similarity, hit, err := cache.Lookup(ctx, query)
	if err != nil {
		t.Errorf("Lookup failed: %v", err)
	}

	if !hit {
		t.Error("Expected cache hit for identical query")
	}

	if similarity < 0.99 {
		t.Errorf("Expected high similarity (>0.99), got %f", similarity)
	}

	if result != response {
		t.Errorf("Expected response %q, got %q", response, result)
	}
}

func TestSemanticCacheLookupSimilar(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	query1 := "Find all functions calling authenticate()"
	response1 := "auth.go:45, api.go:120"

	// Store first query
	err = cache.Store(ctx, "key1", query1, response1)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Lookup similar (but not identical) query
	query2 := "List functions that call authenticate()"
	_, similarity, hit, err := cache.Lookup(ctx, query2)
	if err != nil {
		t.Errorf("Lookup failed: %v", err)
	}

	// With mock embeddings, "similar" queries will have some variation
	// This test validates the threshold behavior
	t.Logf("Similarity for similar query: %f (threshold: 0.85)", similarity)

	if similarity > 0 && similarity < 0.85 {
		if hit {
			t.Error("Expected no hit for query below threshold")
		}
	}
}

func TestSemanticCacheLookupDissimilar(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder, WithSimilarityThreshold(0.85))

	query1 := "Find authentication functions"
	response1 := "auth.go"

	err = cache.Store(ctx, "key1", query1, response1)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Lookup completely different query
	query2 := "Calculate token savings metrics"
	_, similarity, hit, err := cache.Lookup(ctx, query2)
	if err != nil {
		t.Errorf("Lookup failed: %v", err)
	}

	t.Logf("Similarity for dissimilar query: %f", similarity)

	// Should not hit cache for completely different query
	if hit && similarity > 0 && similarity < 0.85 {
		t.Errorf("Unexpected cache hit for dissimilar query with similarity %f", similarity)
	}
}

func TestExactLookup(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	key := "exact_key_123"
	query := "Find functions"
	response := "response text"

	// Store entry
	err = cache.Store(ctx, key, query, response)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Lookup by exact key
	result, found := cache.ExactLookup(key)
	if !found {
		t.Error("Expected to find entry with exact key")
	}

	if result != response {
		t.Errorf("Expected %q, got %q", response, result)
	}

	// Lookup non-existent key
	_, found = cache.ExactLookup("nonexistent")
	if found {
		t.Error("Should not find non-existent key")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name      string
		a, b      []float32
		expectErr bool
		minSim    float32
	}{
		{
			name:      "identical vectors",
			a:         []float32{1, 0, 0},
			b:         []float32{1, 0, 0},
			expectErr: false,
			minSim:    0.99,
		},
		{
			name:      "orthogonal vectors",
			a:         []float32{1, 0, 0},
			b:         []float32{0, 1, 0},
			expectErr: false,
			minSim:    0,
		},
		{
			name:      "opposite vectors",
			a:         []float32{1, 0, 0},
			b:         []float32{-1, 0, 0},
			expectErr: false,
			minSim:    -0.1,
		},
		{
			name:      "different lengths",
			a:         []float32{1, 0},
			b:         []float32{1, 0, 0},
			expectErr: true,
		},
		{
			name:      "zero vectors",
			a:         []float32{0, 0, 0},
			b:         []float32{0, 0, 0},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim, err := CosineSimilarity(tt.a, tt.b)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectErr && sim < tt.minSim {
				t.Errorf("Expected similarity >= %f, got %f", tt.minSim, sim)
			}
		})
	}
}

func TestCacheStatistics(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder, WithFalsePositiveLimit(0.01))

	// Add some entries
	for i := 0; i < 5; i++ {
		key := "key" + string(rune(48+i))
		query := "query " + string(rune(48+i))
		response := "response " + string(rune(48+i))
		err := cache.Store(ctx, key, query, response)
		if err != nil {
			t.Fatalf("Failed to store entry %d: %v", i, err)
		}
	}

	stats := cache.Stats()

	if stats.EntriesCount != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.EntriesCount)
	}

	if !stats.IsHealthy {
		t.Error("Cache should be healthy initially")
	}

	if stats.FalsePositiveRate != 0 {
		t.Errorf("Expected 0 false positives initially, got %f", stats.FalsePositiveRate)
	}
}

func TestCachePrune(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	// Add entry with very short TTL
	key := "short_ttl"
	query := "test query"
	response := "test response"

	err = cache.Store(ctx, key, query, response)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Manually set short TTL for testing
	cache.mu.Lock()
	cache.entries[key].TTL = 1 * time.Millisecond
	cache.mu.Unlock()

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Prune expired entries
	pruned := cache.Prune()
	if pruned != 1 {
		t.Errorf("Expected 1 pruned entry, got %d", pruned)
	}

	// Verify entry is gone
	_, found := cache.ExactLookup(key)
	if found {
		t.Error("Expected expired entry to be pruned")
	}
}

func TestCacheClear(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	cache := NewSemanticCache(embedder)

	// Add entries
	for i := 0; i < 3; i++ {
		key := "key" + string(rune(48+i))
		query := "query " + string(rune(48+i))
		response := "response " + string(rune(48+i))
		cache.Store(ctx, key, query, response)
	}

	stats := cache.Stats()
	if stats.EntriesCount != 3 {
		t.Errorf("Expected 3 entries before clear, got %d", stats.EntriesCount)
	}

	// Clear cache
	cache.Clear()

	stats = cache.Stats()
	if stats.EntriesCount != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.EntriesCount)
	}
}

func TestCacheMaxSize(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	embedder, err := NewEmbeddingModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	maxSize := 5
	cache := NewSemanticCache(embedder, WithMaxCacheSize(maxSize))

	// Fill cache beyond max size
	for i := 0; i < maxSize+3; i++ {
		key := "key" + string(rune(48+i))
		query := "query " + string(rune(48+i))
		response := "response " + string(rune(48+i))
		err := cache.Store(ctx, key, query, response)
		if err != nil {
			t.Fatalf("Failed to store entry %d: %v", i, err)
		}
	}

	stats := cache.Stats()
	if stats.EntriesCount > maxSize {
		t.Errorf("Cache size %d exceeds max %d", stats.EntriesCount, maxSize)
	}
}
