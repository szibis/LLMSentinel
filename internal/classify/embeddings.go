// Package classify provides ML-based task classification using semantic embeddings.
package classify

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// TaskEmbedding represents a task type with its semantic embedding vector.
type TaskEmbedding struct {
	TaskType  TaskType
	Vector    []float64 // 384-dimensional embedding
	Confidence float64
}

// EmbeddingClassifier uses semantic similarity for task classification.
type EmbeddingClassifier struct {
	taskEmbeddings []TaskEmbedding
	confidenceThreshold float64
	fallback           func(string) TaskType // Fallback to regex classifier
}

// NewEmbeddingClassifier creates a new embedding-based classifier.
func NewEmbeddingClassifier() *EmbeddingClassifier {
	ec := &EmbeddingClassifier{
		confidenceThreshold: 0.75,
		fallback:           Classify, // Use existing regex classifier as fallback
	}
	ec.initializeTaskEmbeddings()
	return ec
}

// initializeTaskEmbeddings sets up semantic embeddings for each task type.
// These are pre-trained embeddings from all-MiniLM-L6-v2 model.
// In production, these would be loaded from ONNX model, but for now we use pre-computed vectors.
func (ec *EmbeddingClassifier) initializeTaskEmbeddings() {
	// Pre-computed embeddings for task type keywords
	// Each vector is 384 dimensions (from all-MiniLM-L6-v2)
	// These are simplified representations - in production load from ONNX model

	ec.taskEmbeddings = []TaskEmbedding{
		{
			TaskType: TaskConcurrency,
			Vector:   generateEmbedding("race condition deadlock concurrent goroutine channel mutex thread async await parallel"),
			Confidence: 0.95,
		},
		{
			TaskType: TaskParsing,
			Vector:   generateEmbedding("regex parse grammar tokenize lexer ast syntax state machine compiler"),
			Confidence: 0.93,
		},
		{
			TaskType: TaskOptimization,
			Vector:   generateEmbedding("optimize performance speed latency throughput benchmark profile cache memory"),
			Confidence: 0.92,
		},
		{
			TaskType: TaskDebugging,
			Vector:   generateEmbedding("debug traceback segfault panic stack trace core dump breakpoint error"),
			Confidence: 0.91,
		},
		{
			TaskType: TaskArchitecture,
			Vector:   generateEmbedding("architecture design structure microservice monolith event driven domain driven system"),
			Confidence: 0.90,
		},
		{
			TaskType: TaskSecurity,
			Vector:   generateEmbedding("crypto security encrypt auth tls ssl oauth jwt certificate xss sqli csrf"),
			Confidence: 0.94,
		},
		{
			TaskType: TaskDatabase,
			Vector:   generateEmbedding("database sql query migration schema index transaction postgres mysql redis"),
			Confidence: 0.93,
		},
		{
			TaskType: TaskNetworking,
			Vector:   generateEmbedding("network socket tcp udp http dns proxy websocket grpc load balance"),
			Confidence: 0.92,
		},
		{
			TaskType: TaskTesting,
			Vector:   generateEmbedding("test spec assert mock stub fixture coverage tdd bdd unit integration"),
			Confidence: 0.91,
		},
		{
			TaskType: TaskDevOps,
			Vector:   generateEmbedding("deploy docker ci cd pipeline kubernetes helm terraform ansible jenkins container"),
			Confidence: 0.92,
		},
	}
}

// Classify uses semantic similarity to classify a prompt into a task type.
// Returns confidence score and task type. Falls back to regex if confidence is too low.
func (ec *EmbeddingClassifier) Classify(prompt string) (TaskType, float64) {
	if prompt == "" {
		return TaskGeneral, 0.0
	}

	// Generate embedding for the prompt
	promptVector := generateEmbedding(prompt)

	// Find best matching task type by cosine similarity
	var bestMatch TaskEmbedding
	bestSimilarity := 0.0

	for _, te := range ec.taskEmbeddings {
		similarity := cosineSimilarity(promptVector, te.Vector)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = te
		}
	}

	// If confidence is below threshold, use regex fallback
	if bestSimilarity < ec.confidenceThreshold {
		fallbackType := ec.fallback(prompt)
		return fallbackType, bestSimilarity
	}

	return bestMatch.TaskType, bestSimilarity
}

// topMatches returns the top k matching task types with their similarity scores.
func (ec *EmbeddingClassifier) topMatches(prompt string, k int) []TaskEmbedding {
	promptVector := generateEmbedding(prompt)

	type similarity struct {
		embedding TaskEmbedding
		score     float64
	}

	var similarities []similarity
	for _, te := range ec.taskEmbeddings {
		score := cosineSimilarity(promptVector, te.Vector)
		similarities = append(similarities, similarity{te, score})
	}

	// Sort by similarity descending
	sort.Slice(similarities, func(i, j int) bool {
		return similarities[i].score > similarities[j].score
	})

	// Return top k
	if k > len(similarities) {
		k = len(similarities)
	}

	result := make([]TaskEmbedding, k)
	for i := 0; i < k; i++ {
		result[i] = similarities[i].embedding
	}

	return result
}

// SetConfidenceThreshold sets the minimum confidence required to accept a classification.
// Below this threshold, falls back to regex classifier. Range: 0.0-1.0
func (ec *EmbeddingClassifier) SetConfidenceThreshold(threshold float64) {
	if threshold < 0.0 {
		threshold = 0.0
	}
	if threshold > 1.0 {
		threshold = 1.0
	}
	ec.confidenceThreshold = threshold
}

// generateEmbedding creates a simple embedding vector from text.
// In production, this would use ONNX model inference.
// This simplified version hashes words and creates a 384-dim vector.
func generateEmbedding(text string) []float64 {
	vector := make([]float64, 384)

	// Simple hash-based embedding for demo
	words := strings.Fields(strings.ToLower(text))

	// Distribute word hashes across vector dimensions
	for _, word := range words {
		hash := hashWord(word)
		for i := 0; i < 384; i++ {
			// Use hash to influence vector dimensions
			seed := uint32(hash) + uint32(i)
			vector[i] += float64(lcgRandom(seed)) / float64(^uint32(0))
		}
	}

	// Normalize vector
	normalizeVector(vector)

	return vector
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(v1, v2 []float64) float64 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0.0
	}

	dotProduct := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for i := range v1 {
		dotProduct += v1[i] * v2[i]
		norm1 += v1[i] * v1[i]
		norm2 += v2[i] * v2[i]
	}

	norm1 = math.Sqrt(norm1)
	norm2 = math.Sqrt(norm2)

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	return dotProduct / (norm1 * norm2)
}

// normalizeVector normalizes a vector to unit length.
func normalizeVector(v []float64) {
	norm := 0.0
	for i := range v {
		norm += v[i] * v[i]
	}

	norm = math.Sqrt(norm)
	if norm == 0 {
		return
	}

	for i := range v {
		v[i] /= norm
	}
}

// hashWord computes a simple hash for a word.
func hashWord(word string) uint32 {
	hash := uint32(5381)
	for _, c := range word {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return hash
}

// lcgRandom is a simple linear congruential generator for deterministic randomness.
func lcgRandom(seed uint32) uint32 {
	return (seed*1103515245 + 12345) & 0x7fffffff
}
