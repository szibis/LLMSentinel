package cache

import (
	"context"
	"fmt"
	"sync"

	"github.com/szibis/claude-escalate/internal/config"
)

// EmbeddingModel wraps ONNX sentence-transformers for semantic embeddings
type EmbeddingModel struct {
	modelID   string
	dimension int
	mu        sync.RWMutex
}

// NewEmbeddingModel creates a new embedding model (ONNX-based)
func NewEmbeddingModel(cfg *config.Config) (*EmbeddingModel, error) {
	// Validate config exists
	if cfg == nil {
		return nil, fmt.Errorf("config required for embedding model")
	}

	model := &EmbeddingModel{
		modelID:   "all-MiniLM-L6-v2", // 384-dimensional embeddings
		dimension: 384,
	}

	return model, nil
}

// Embed computes embedding vector for text query (returns fixed 384-dim vector)
func (e *EmbeddingModel) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}

	// Placeholder: In production, use ONNX runtime to load all-MiniLM-L6-v2
	// For now, return deterministic mock embedding for testing
	// Real implementation loads actual ONNX model (~450MB)

	embedding := make([]float32, e.dimension)

	// Generate deterministic mock embedding (same text → same vector)
	hash := fnvHash(text)
	for i := 0; i < e.dimension; i++ {
		// gosec: uint64 conversion is safe here since i is always >= 0
		// #nosec G115
		seed := uint64(hash) ^ (uint64(i) << 8) ^ (uint64(i) * 7) // #nosec G115
		seed = seed*6364136223846793005 + 1442695040888963407
		embedding[i] = float32((seed >> 32) % 10000) / 10000.0
	}

	return embedding, nil
}

// EmbedBatch computes embeddings for multiple texts efficiently
func (e *EmbeddingModel) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		emb, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	return embeddings, nil
}

// CosineSimilarity computes cosine distance between two embedding vectors
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("embedding dimensions mismatch: %d vs %d", len(a), len(b))
	}

	if len(a) == 0 {
		return 0, fmt.Errorf("empty embeddings")
	}

	var dotProduct, normA, normB float32

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("zero norm vectors")
	}

	similarity := dotProduct / (sqrtF32(normA) * sqrtF32(normB))

	// Clamp to [0, 1] range
	if similarity < 0 {
		similarity = 0
	} else if similarity > 1 {
		similarity = 1
	}

	return similarity, nil
}

// fnvHash implements FNV-1a hash for deterministic mock embeddings
func fnvHash(text string) uint32 {
	hash := uint32(2166136261) // FNV offset basis
	for i := 0; i < len(text); i++ {
		hash ^= uint32(text[i])
		hash *= 16777619 // FNV prime
	}
	return hash
}

// sqrtF32 computes square root of float32
func sqrtF32(x float32) float32 {
	if x < 0 {
		return 0
	}
	// Newton-Raphson approximation
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// EmbeddingDimension returns the dimension of embeddings from this model
func (e *EmbeddingModel) EmbeddingDimension() int {
	return e.dimension
}
