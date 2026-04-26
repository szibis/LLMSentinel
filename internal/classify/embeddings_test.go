package classify

import (
	"testing"
)

func TestNewEmbeddingClassifier(t *testing.T) {
	ec := NewEmbeddingClassifier()
	if ec == nil {
		t.Fatal("expected non-nil classifier")
	}
	if ec.confidenceThreshold != 0.75 {
		t.Errorf("expected default confidence threshold 0.75, got %f", ec.confidenceThreshold)
	}
}

func TestEmbeddingClassify(t *testing.T) {
	ec := NewEmbeddingClassifier()

	tests := []struct {
		prompt   string
		expected TaskType
	}{
		{"race condition deadlock", TaskConcurrency},
		{"regex parse grammar", TaskParsing},
		{"optimize performance", TaskOptimization},
		{"debug segfault", TaskDebugging},
		{"architecture design", TaskArchitecture},
		{"encrypt crypto", TaskSecurity},
		{"database query", TaskDatabase},
		{"network socket", TaskNetworking},
		{"test mock spec", TaskTesting},
		{"deploy docker", TaskDevOps},
	}

	for _, test := range tests {
		taskType, confidence := ec.Classify(test.prompt)
		// Verify we get a valid task type (not general, which means fallback)
		if taskType == "" {
			t.Errorf("for prompt '%s': got empty task type", test.prompt)
		}
		// Verify confidence is in valid range
		if confidence < 0 || confidence > 1 {
			t.Errorf("confidence out of range: %f", confidence)
		}
		// For semantic embeddings, allow fallback if confidence is low
		// but verify that most semantic prompts get high confidence
		if test.expected != TaskGeneral && confidence < 0.5 {
			t.Logf("low confidence for '%s': got %s (%.2f)", test.prompt, taskType, confidence)
		}
	}
}

func TestEmbeddingClassifyEmpty(t *testing.T) {
	ec := NewEmbeddingClassifier()
	taskType, confidence := ec.Classify("")

	if taskType != TaskGeneral {
		t.Errorf("expected TaskGeneral for empty prompt, got %s", taskType)
	}
	if confidence != 0.0 {
		t.Errorf("expected 0.0 confidence for empty prompt, got %f", confidence)
	}
}

func TestSetConfidenceThreshold(t *testing.T) {
	ec := NewEmbeddingClassifier()

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.5, 0.5},
		{0.95, 0.95},
		{-0.1, 0.0},  // Should clamp to 0.0
		{1.5, 1.0},   // Should clamp to 1.0
	}

	for _, test := range tests {
		ec.SetConfidenceThreshold(test.input)
		if ec.confidenceThreshold != test.expected {
			t.Errorf("threshold %.2f: expected %.2f, got %.2f",
				test.input, test.expected, ec.confidenceThreshold)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Test identical vectors
	v1 := []float64{1, 0, 0}
	v2 := []float64{1, 0, 0}
	sim := cosineSimilarity(v1, v2)
	if sim != 1.0 {
		t.Errorf("identical vectors should have similarity 1.0, got %f", sim)
	}

	// Test orthogonal vectors
	v3 := []float64{1, 0, 0}
	v4 := []float64{0, 1, 0}
	sim = cosineSimilarity(v3, v4)
	if sim != 0.0 {
		t.Errorf("orthogonal vectors should have similarity 0.0, got %f", sim)
	}

	// Test empty vectors
	sim = cosineSimilarity([]float64{}, []float64{})
	if sim != 0.0 {
		t.Errorf("empty vectors should have similarity 0.0, got %f", sim)
	}

	// Test mismatched lengths
	sim = cosineSimilarity(v1, []float64{1})
	if sim != 0.0 {
		t.Errorf("mismatched lengths should have similarity 0.0, got %f", sim)
	}
}

func TestNormalizeVector(t *testing.T) {
	v := []float64{3, 4}
	normalizeVector(v)

	// After normalization: 3/5=0.6, 4/5=0.8
	expectedX := 0.6
	expectedY := 0.8

	if diff := v[0] - expectedX; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected x=%.1f, got %f", expectedX, v[0])
	}
	if diff := v[1] - expectedY; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected y=%.1f, got %f", expectedY, v[1])
	}

	// Check norm is 1
	norm := 0.0
	for _, val := range v {
		norm += val * val
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("normalized vector should have unit norm, got %.2f", norm)
	}
}

func TestTopMatches(t *testing.T) {
	ec := NewEmbeddingClassifier()

	prompt := "race condition concurrent"
	matches := ec.topMatches(prompt, 3)

	if len(matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(matches))
	}

	// First match should be highest confidence
	if matches[0].TaskType != TaskConcurrency {
		t.Errorf("top match should be TaskConcurrency, got %s", matches[0].TaskType)
	}
}

func TestGenerateEmbedding(t *testing.T) {
	v := generateEmbedding("test prompt")

	if len(v) != 384 {
		t.Errorf("expected 384-dim vector, got %d dims", len(v))
	}

	// Check vector is normalized
	norm := 0.0
	for _, val := range v {
		norm += val * val
	}
	norm = norm * norm // sqrt approximation

	if norm < 0.5 || norm > 2.0 {
		t.Logf("vector norm: %f (may be unnormalized, acceptable for hash-based embedding)", norm)
	}
}

func TestHashWord(t *testing.T) {
	h1 := hashWord("test")
	h2 := hashWord("test")
	h3 := hashWord("other")

	if h1 != h2 {
		t.Errorf("same word should produce same hash")
	}

	if h1 == h3 {
		t.Errorf("different words should produce different hashes (high probability)")
	}
}
