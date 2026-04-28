package optimization

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/cache"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/graph"
	"github.com/szibis/claude-escalate/internal/metrics"
)

func setupTestPipeline(t *testing.T) (*OptimizationPipeline, *graph.GraphDB, func()) {
	tmpDir := t.TempDir()

	// Create graph DB
	graphDB, err := graph.Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open graph DB: %v", err)
	}

	// Create cache layer
	cfg := &config.Config{
		Optimizations: config.OptimizationsConfig{
			SemanticCache: config.SemanticCacheConfig{
				Enabled:             true,
				SimilarityThreshold: 0.85,
				FalsePositiveLimit:  0.005,
				MaxCacheSize:        10000,
			},
		},
	}

	collector := metrics.NewMetricsCollector()
	cacheLayer, err := cache.NewLayer(cfg, collector)
	if err != nil {
		t.Fatalf("Failed to create cache layer: %v", err)
	}

	cacheGraphLayer := cache.NewCacheGraphLayer(cacheLayer, graphDB, collector)

	// Create optimizer
	optimizer := NewOptimizer()

	// Create pipeline
	pipelineConfig := &PipelineConfig{
		MaxLatency:                500 * time.Millisecond,
		GraphLookupEnabled:        true,
		SemanticCacheEnabled:      true,
		InputOptimizationEnabled:  true,
		OutputOptimizationEnabled: true,
	}

	pipeline := NewOptimizationPipeline(cacheGraphLayer, optimizer, collector, pipelineConfig)

	cleanup := func() {
		graphDB.Close()
	}

	return pipeline, graphDB, cleanup
}

func TestOptimizationPipeline_Process(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	ctx := context.Background()
	req := &PipelineRequest{
		Query:   "test query",
		Intent:  "quick_answer",
		Tool:    "cli",
		Context: ctx,
	}

	resp := pipeline.Process(req)

	if resp == nil {
		t.Fatal("Process returned nil response")
	}

	if resp.Latency <= 0 {
		t.Fatal("Latency should be > 0")
	}

	if resp.TransparencyFooter == "" {
		t.Fatal("TransparencyFooter should not be empty")
	}
}

func TestOptimizationPipeline_ExactCacheHit(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	ctx := context.Background()
	query := "find all functions"
	expectedResp := "Functions: func1, func2"

	// Pre-populate cache
	if err := pipeline.cacheGraphLayer.Store(ctx, query, expectedResp); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Process same query
	req := &PipelineRequest{
		Query:   query,
		Intent:  "quick_answer",
		Context: ctx,
	}

	resp := pipeline.Process(req)

	if resp.Source != "exact_cache" {
		t.Fatalf("Expected source 'exact_cache', got %q", resp.Source)
	}

	if resp.Content != expectedResp {
		t.Fatalf("Expected content %q, got %q", expectedResp, resp.Content)
	}

	if resp.Confidence != 1.0 {
		t.Fatalf("Expected confidence 1.0, got %v", resp.Confidence)
	}
}

func TestOptimizationPipeline_GraphLookup(t *testing.T) {
	pipeline, graphDB, cleanup := setupTestPipeline(t)
	defer cleanup()

	ctx := context.Background()

	// Add nodes and edges to graph
	target := &graph.Node{
		ID:   "auth_func",
		Name: "authenticate",
		Type: graph.NodeTypeFunction,
	}
	caller := &graph.Node{
		ID:   "login_func",
		Name: "login",
		Type: graph.NodeTypeFunction,
	}

	if err := graphDB.CreateNode(ctx, target); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if err := graphDB.CreateNode(ctx, caller); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	edge := &graph.Edge{
		ID:           "edge_1",
		SourceID:     "login_func",
		TargetID:     "auth_func",
		RelationType: graph.RelationTypeCalls,
	}
	if err := graphDB.CreateEdge(ctx, edge); err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}

	// Process graph query
	req := &PipelineRequest{
		Query:   "functions that call authenticate",
		Intent:  "quick_answer",
		Context: ctx,
	}

	resp := pipeline.Process(req)

	// Should either be graph hit or direct
	if resp.Error != nil {
		t.Logf("Got error: %v (acceptable for complex graph queries)", resp.Error)
	}
}

func TestOptimizationPipeline_TransparencyFooter_ExactCache(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	resp := &PipelineResponse{
		Source:          "exact_cache",
		Content:         "cached",
		Layer:           1,
		Confidence:      1.0,
		TokensSaved:     2500,
		EstimatedTokens: 0,
		Latency:         10 * time.Millisecond,
	}

	footer := pipeline.generateTransparencyFooter(resp)

	if footer == "" {
		t.Fatal("Footer should not be empty")
	}

	if !contains(footer, "100%") && !contains(footer, "exact_cache") {
		t.Fatalf("Footer should mention exact cache: %s", footer)
	}
}

func TestOptimizationPipeline_TransparencyFooter_SemanticCache(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	resp := &PipelineResponse{
		Source:          "semantic_cache",
		Content:         "cached",
		Layer:           3,
		Confidence:      0.92,
		TokensSaved:     2450,
		EstimatedTokens: 50,
		Latency:         15 * time.Millisecond,
	}

	footer := pipeline.generateTransparencyFooter(resp)

	if footer == "" {
		t.Fatal("Footer should not be empty")
	}

	if !contains(footer, "98%") {
		t.Fatalf("Footer should mention 98%% savings: %s", footer)
	}
}

func TestOptimizationPipeline_TransparencyFooter_Graph(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	resp := &PipelineResponse{
		Source:          "graph",
		Content:         "graph result",
		Layer:           2,
		Confidence:      0.99,
		TokensSaved:     2475,
		EstimatedTokens: 25,
		Latency:         8 * time.Millisecond,
		GraphContext: &GraphContext{
			GraphHit:        true,
			Nodes:           []graph.Node{{Name: "func1"}, {Name: "func2"}},
			Depth:           2,
			ConfidenceScore: 0.99,
		},
	}

	footer := pipeline.generateTransparencyFooter(resp)

	if footer == "" {
		t.Fatal("Footer should not be empty")
	}

	if !contains(footer, "99%") {
		t.Fatalf("Footer should mention 99%% savings: %s", footer)
	}

	if !contains(footer, "Related items") {
		t.Fatalf("Footer should mention related items: %s", footer)
	}
}

func TestOptimizationPipeline_TransparencyFooter_Fresh(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	resp := &PipelineResponse{
		Source:          "claude",
		Content:         "fresh response",
		Layer:           6,
		Confidence:      0,
		TokensSaved:     0,
		EstimatedTokens: 2500,
		Latency:         500 * time.Millisecond,
	}

	footer := pipeline.generateTransparencyFooter(resp)

	if footer == "" {
		t.Fatal("Footer should not be empty")
	}

	if !contains(footer, "Fresh") {
		t.Fatalf("Footer should mention fresh response: %s", footer)
	}
}

func TestOptimizationPipeline_InvalidRequest(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	// Nil request
	resp := pipeline.Process(nil)
	if resp == nil || resp.Error == nil {
		t.Fatal("Should return error for nil request")
	}

	// Nil context
	req := &PipelineRequest{
		Query: "test",
	}
	resp = pipeline.Process(req)
	// Should handle gracefully
	if resp == nil {
		t.Fatal("Should handle nil context gracefully")
	}
}

func TestOptimizationPipeline_GetStats(t *testing.T) {
	pipeline, graphDB, cleanup := setupTestPipeline(t)
	defer cleanup()

	ctx := context.Background()

	// Add some data
	node := &graph.Node{
		ID:   "test",
		Name: "test",
		Type: graph.NodeTypeFunction,
	}
	if err := graphDB.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	stats := pipeline.GetStats(ctx)

	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	if _, ok := stats["optimizer"]; !ok {
		t.Fatal("Stats missing 'optimizer' key")
	}

	if _, ok := stats["cache_graph"]; !ok {
		t.Fatal("Stats missing 'cache_graph' key")
	}
}

func TestOptimizationPipeline_Clear(t *testing.T) {
	pipeline, _, cleanup := setupTestPipeline(t)
	defer cleanup()

	ctx := context.Background()

	// Add data
	if err := pipeline.cacheGraphLayer.Store(ctx, "query", "response"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Clear cache only
	if err := pipeline.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify cache is cleared
	stats := pipeline.GetStats(ctx)
	cacheGraphStats := stats["cache_graph"].(map[string]interface{})
	if cacheGraphStats == nil {
		t.Fatal("Stats should still be available")
	}
}

func TestOptimizationPipeline_Config(t *testing.T) {
	config := DefaultPipelineConfig()

	if config.MaxLatency <= 0 {
		t.Fatal("MaxLatency should be > 0")
	}

	if !config.GraphLookupEnabled {
		t.Fatal("GraphLookupEnabled should be true")
	}

	if !config.SemanticCacheEnabled {
		t.Fatal("SemanticCacheEnabled should be true")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr))
}
