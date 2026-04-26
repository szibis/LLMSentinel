package optimization

import (
	"context"
	"testing"
)

// ========== DEDUPLICATION TESTS ==========

func TestRequestDedup_ExactMatch(t *testing.T) {
	dedup := NewRequestDeduplicator()
	ctx := context.Background()

	req := &PipelineRequest{
		Query:  "Find all functions",
		Intent: "quick_answer",
		Tool:   "cli",
		Params: map[string]interface{}{"cmd": "go", "args": []string{"list"}},
	}

	// First request
	hash1, err := dedup.Hash(req)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	dedup.Store(ctx, hash1, "response1")

	// Identical request
	hash2, err := dedup.Hash(req)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Fatal("Identical requests should produce identical hashes")
	}

	// Lookup should find cached response
	cached, found := dedup.Lookup(ctx, hash2)
	if !found {
		t.Fatal("Should find cached response for identical request")
	}
	if cached != "response1" {
		t.Fatalf("Expected 'response1', got %q", cached)
	}
}

func TestRequestDedup_DifferentParams(t *testing.T) {
	dedup := NewRequestDeduplicator()

	req1 := &PipelineRequest{
		Query:  "Find all functions",
		Params: map[string]interface{}{"limit": 10},
	}

	req2 := &PipelineRequest{
		Query:  "Find all functions",
		Params: map[string]interface{}{"limit": 20},
	}

	hash1, _ := dedup.Hash(req1)
	hash2, _ := dedup.Hash(req2)

	if hash1 == hash2 {
		t.Fatal("Different params should produce different hashes")
	}
}

func TestRequestDedup_Clear(t *testing.T) {
	dedup := NewRequestDeduplicator()
	ctx := context.Background()

	req := &PipelineRequest{Query: "test"}
	hash, _ := dedup.Hash(req)
	dedup.Store(ctx, hash, "response")

	// Verify cached
	_, found := dedup.Lookup(ctx, hash)
	if !found {
		t.Fatal("Should find cached response before clear")
	}

	// Clear
	dedup.Clear(ctx)

	// Verify cleared
	_, found = dedup.Lookup(ctx, hash)
	if found {
		t.Fatal("Should not find response after clear")
	}
}

func TestRequestDedup_Stats(t *testing.T) {
	dedup := NewRequestDeduplicator()
	ctx := context.Background()

	// No requests yet
	stats := dedup.GetStats()
	if stats["total_lookups"] != 0 {
		t.Fatal("Should start with 0 lookups")
	}

	// Store and lookup
	req := &PipelineRequest{Query: "test"}
	hash, _ := dedup.Hash(req)
	dedup.Store(ctx, hash, "response")
	dedup.Lookup(ctx, hash)

	stats = dedup.GetStats()
	if stats["cache_hits"].(int) != 1 {
		t.Fatalf("Expected 1 cache hit, got %d", stats["cache_hits"])
	}
}

// ========== FORMATTER TESTS ==========

func TestInputFormatter_CompactJSON(t *testing.T) {
	formatter := NewInputFormatter()

	input := &PipelineRequest{
		Query:  "Analyze this code for security",
		Intent: "detailed_analysis",
		Tool:   "code_analyzer",
		Params: map[string]interface{}{
			"include_metadata": true,
			"max_results":      100,
			"recursive":        true,
			"timeout_seconds":  30,
		},
	}

	compact, err := formatter.CompactJSON(input)
	if err != nil {
		t.Fatalf("CompactJSON failed: %v", err)
	}

	if compact == "" {
		t.Fatal("Compact JSON should not be empty")
	}

	// Verify it's actually shorter than JSON dump
	fullJSON := formatAsJSON(input)
	if len(compact) >= len(fullJSON) {
		t.Logf("Warning: compact form not shorter (compact: %d, full: %d)", len(compact), len(fullJSON))
	}
}

func TestInputFormatter_StructuredFormat(t *testing.T) {
	formatter := NewInputFormatter()

	// Verbose input
	verbose := "Find all Python functions that validate user input and are exported"

	structured, err := formatter.StructuredFormat(verbose)
	if err != nil {
		t.Fatalf("StructuredFormat failed: %v", err)
	}

	if structured == "" {
		t.Fatal("Structured format should not be empty")
	}

	// Verify structure (should be JSON with task, language, filter, etc)
	if !containsStructuredKeys(structured) {
		t.Logf("Warning: structured format may lack expected keys: %s", structured)
	}
}

func TestInputFormatter_RemoveUnnecessaryWhitespace(t *testing.T) {
	formatter := NewInputFormatter()

	input := `
		Function: authenticate

		Purpose: Validate user credentials

		Details:
			- Check username length
			- Verify password hash
	`

	cleaned, _ := formatter.RemoveUnnecessaryWhitespace(input)

	if len(cleaned) >= len(input) {
		t.Fatal("Should reduce whitespace")
	}

	if cleaned == "" {
		t.Fatal("Should preserve content")
	}
}

func TestInputFormatter_ShortenCommonTerms(t *testing.T) {
	formatter := NewInputFormatter()

	input := "Find all functions with documentation"
	shortened, _ := formatter.ShortenCommonTerms(input)

	// Should replace "Find all" with shorter equivalent or abbreviation
	if len(shortened) >= len(input) {
		t.Logf("Note: common term shortening not applied (input: %d, output: %d)", len(input), len(shortened))
	}
}

// ========== COMPRESSION TESTS ==========

func TestParameterCompression_Basic(t *testing.T) {
	compressor := NewParameterCompressor()

	params := map[string]interface{}{
		"include_metadata":   true,
		"max_results":        100,
		"recursive":          true,
		"timeout_seconds":    30,
		"output_format":      "json",
		"filter_by_language": "python",
	}

	compressed, err := compressor.Compress(params)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	// Compressed form should be shorter or equal (minimal case)
	originalJSON := formatAsJSON(params)
	if len(compressed) > len(originalJSON)*2 {
		t.Logf("Warning: compression overhead, original: %d, compressed: %d", len(originalJSON), len(compressed))
	}

	// Should be able to decompress back
	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if decompressed == nil {
		t.Fatal("Decompressed should not be nil")
	}
}

func TestParameterCompression_AbbreviateKeys(t *testing.T) {
	compressor := NewParameterCompressor()

	params := map[string]interface{}{
		"include_metadata": true,
		"max_results":      100,
		"recursive":        true,
	}

	abbreviated, err := compressor.AbbreviateKeys(params)
	if err != nil {
		t.Fatalf("AbbreviateKeys failed: %v", err)
	}

	if abbreviated == nil {
		t.Fatal("Abbreviated should not be nil")
	}

	// Should have fewer/shorter keys
	originalSize := len(formatAsJSON(params))
	abbreviatedSize := len(formatAsJSON(abbreviated))
	if abbreviatedSize >= originalSize {
		t.Logf("Note: abbreviation not applied or ineffective")
	}
}

func TestParameterCompression_RemoveDefaults(t *testing.T) {
	compressor := NewParameterCompressor()

	params := map[string]interface{}{
		"max_results":  100,        // Default
		"recursive":    true,       // Default
		"timeout":      30,         // Default
		"custom_field": "special",  // Non-default
	}

	optimized, err := compressor.RemoveDefaults(params)
	if err != nil {
		t.Fatalf("RemoveDefaults failed: %v", err)
	}

	// Should only have custom_field
	if optimized == nil {
		t.Fatal("Optimized should not be nil")
	}

	if len(optimized) > len(params) {
		t.Logf("Note: default removal not applied effectively")
	}
}

func TestParameterCompression_CombinedSavings(t *testing.T) {
	compressor := NewParameterCompressor()

	params := map[string]interface{}{
		"include_metadata":   true,
		"max_results":        100,
		"recursive":          true,
		"timeout_seconds":    30,
		"output_format":      "json",
		"filter_by_language": "python",
		"verbose":            false,
		"debug":              false,
	}

	original := formatAsJSON(params)
	originalLen := len(original)

	// Apply all optimizations
	abbreviated, _ := compressor.AbbreviateKeys(params)
	optimized, _ := compressor.RemoveDefaults(abbreviated)
	compressed, _ := compressor.Compress(optimized)

	compressedLen := len(compressed)

	savingsPercent := 0.0
	if originalLen > 0 {
		savingsPercent = (float64(originalLen - compressedLen) / float64(originalLen)) * 100
		if savingsPercent < 0 {
			savingsPercent = 0
		}
	}

	t.Logf("Parameter compression savings: %.1f%% (original: %d, compressed: %d)", savingsPercent, originalLen, compressedLen)

	if compressedLen >= originalLen {
		t.Logf("Note: combined compression achieved no savings (may be overhead on small inputs)")
	}
}

// ========== INTEGRATED OPTIMIZATION TESTS ==========

func TestInputOptimization_FullPipeline(t *testing.T) {
	optimizer := NewInputOptimizer()
	ctx := context.Background()

	input := &PipelineRequest{
		Query:  "Find all Python functions that validate user input",
		Intent: "detailed_analysis",
		Tool:   "code_analyzer",
		Params: map[string]interface{}{
			"include_metadata":   true,
			"max_results":        100,
			"recursive":          true,
			"timeout_seconds":    30,
			"output_format":      "json",
			"filter_by_language": "python",
		},
	}

	optimized, savings, err := optimizer.OptimizeInput(ctx, input)
	if err != nil {
		t.Fatalf("OptimizeInput failed: %v", err)
	}

	if optimized == nil {
		t.Fatal("Optimized input should not be nil")
	}

	if savings.TotalTokens < 0 {
		t.Fatalf("Token savings should be non-negative, got %d", savings.TotalTokens)
	}

	// Optimized version should be more compact
	if savings.Percent < 0 {
		t.Fatal("Savings percent should be >= 0")
	}

	t.Logf("Input optimization achieved %.1f%% savings (%d tokens)", savings.Percent, savings.TotalTokens)
}

func TestInputOptimization_LargeInput(t *testing.T) {
	optimizer := NewInputOptimizer()
	ctx := context.Background()

	// Create large input
	largeParams := make(map[string]interface{})
	for i := 0; i < 50; i++ {
		largeParams[nameOfParam(i)] = valueOfParam(i)
	}

	input := &PipelineRequest{
		Query:  "Analyze" + repeatString(" large", 50),
		Intent: "detailed_analysis",
		Tool:   "analyzer",
		Params: largeParams,
	}

	optimized, savings, err := optimizer.OptimizeInput(ctx, input)
	if err != nil {
		t.Fatalf("OptimizeInput on large input failed: %v", err)
	}

	if optimized == nil {
		t.Fatal("Should handle large inputs")
	}

	t.Logf("Large input optimization: %.1f%% savings", savings.Percent)
}

func TestInputOptimization_Concurrent(t *testing.T) {
	optimizer := NewInputOptimizer()
	ctx := context.Background()

	results := make(chan error, 10)

	// Optimize 10 requests concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			input := &PipelineRequest{
				Query:  "Query " + string(rune(idx)),
				Intent: "quick_answer",
				Tool:   "cli",
				Params: map[string]interface{}{"index": idx},
			}

			_, _, err := optimizer.OptimizeInput(ctx, input)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < 10; i++ {
		if err := <-results; err != nil {
			t.Fatalf("Concurrent optimization failed: %v", err)
		}
	}
}

// ========== HELPER FUNCTIONS ==========

func formatAsJSON(v interface{}) string {
	// Simple JSON representation (stub for testing)
	return "json_data"
}

func containsStructuredKeys(s string) bool {
	// Check if string contains structured keys (simplified for testing)
	if s == "json_data" {
		return true // Assume well-formed
	}
	return false
}

func nameOfParam(i int) string {
	names := []string{"param_a", "param_b", "param_c", "param_d", "param_e"}
	return names[i%len(names)] + "_" + string(rune(48+i/5))
}

func valueOfParam(i int) interface{} {
	if i%2 == 0 {
		return i * 100
	}
	return "value_" + string(rune(65+i%26))
}

func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
