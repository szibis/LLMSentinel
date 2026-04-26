package optimization

import (
	"context"
	"fmt"
	"time"

	"github.com/szibis/claude-escalate/internal/cache"
	"github.com/szibis/claude-escalate/internal/graph"
	"github.com/szibis/claude-escalate/internal/metrics"
)

// PipelineRequest represents a request flowing through the optimization pipeline
type PipelineRequest struct {
	Query       string            // User query/prompt
	Intent      string            // Classified intent (quick_answer, detailed_analysis, etc)
	Tool        string            // Tool type (mcp, cli, rest, db, binary)
	Params      map[string]interface{}
	Context     context.Context
	Timestamp   time.Time
}

// PipelineResponse represents the response from the optimization pipeline
type PipelineResponse struct {
	// Response content
	Content string

	// Pipeline metadata
	Source           string              // Which layer returned this (cache, graph, claude)
	Layer            int                 // Layer number (1-7)
	Confidence       float32             // Confidence in response (0-1)
	TokensSaved      int64               // Tokens saved by optimization
	EstimatedTokens  int                 // Tokens used for this response
	Latency          time.Duration       // Time to get response
	Error            error               // Any errors during processing

	// Graph context (if applicable)
	GraphContext *GraphContext

	// Transparency footer for user
	TransparencyFooter string
}

// GraphContext contains information about graph-based responses
type GraphContext struct {
	GraphHit     bool                  // True if response came from graph
	Nodes        []graph.Node          // Related nodes
	RelationshipType string             // Type of relationship (calls, imports, etc)
	Depth        int                   // Graph traversal depth
	ConfidenceScore float32            // Graph query confidence (0-1)
}

// OptimizationPipeline implements the 7+ layer optimization pipeline
// Layer 1: Exact Dedup Cache (100% savings)
// Layer 1.5: Graph Lookup (99% savings) - NEW
// Layer 2: Batch API (50% savings)
// Layer 3: Semantic Cache (98% savings)
// Layer 4: Input Optimization (30-40% savings)
// Layer 5: Model Selection (varies)
// Layer 6: Claude API Call (direct)
// Layer 7: Output Optimization (30-50% savings)
type OptimizationPipeline struct {
	cacheGraphLayer *cache.CacheGraphLayer
	optimizer       *Optimizer
	metrics         *metrics.MetricsCollector
	config          *PipelineConfig
}

// PipelineConfig holds configuration for the optimization pipeline
type PipelineConfig struct {
	MaxLatency              time.Duration
	GraphLookupEnabled      bool
	SemanticCacheEnabled    bool
	InputOptimizationEnabled bool
	OutputOptimizationEnabled bool
}

// DefaultPipelineConfig returns sensible defaults
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		MaxLatency:                500 * time.Millisecond,
		GraphLookupEnabled:        true,
		SemanticCacheEnabled:      true,
		InputOptimizationEnabled:  true,
		OutputOptimizationEnabled: true,
	}
}

// NewOptimizationPipeline creates a new optimization pipeline
func NewOptimizationPipeline(
	cacheGraphLayer *cache.CacheGraphLayer,
	optimizer *Optimizer,
	metricsCollector *metrics.MetricsCollector,
	config *PipelineConfig,
) *OptimizationPipeline {
	if config == nil {
		config = DefaultPipelineConfig()
	}

	return &OptimizationPipeline{
		cacheGraphLayer: cacheGraphLayer,
		optimizer:       optimizer,
		metrics:         metricsCollector,
		config:          config,
	}
}

// Process runs a request through the complete optimization pipeline
func (p *OptimizationPipeline) Process(req *PipelineRequest) *PipelineResponse {
	if req == nil || req.Context == nil {
		return &PipelineResponse{
			Source: "error",
			Error:  fmt.Errorf("invalid request"),
		}
	}

	startTime := time.Now()

	// Ensure Context is not nil
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Step 1-2: Try cache + graph lookup (unified layer)
	if p.cacheGraphLayer != nil {
		cacheResult := p.cacheGraphLayer.Lookup(ctx, req.Query, req.Intent)
		if cacheResult.Source != "claude" {
			return p.buildResponse(req, cacheResult, 1, startTime)
		}
	}

	// Step 3: Use original optimizer for batch/model decisions
	decision := p.optimizer.Optimize(req.Query, "haiku", 500) // Default heuristic

	resp := &PipelineResponse{
		Source:          decision.Direction,
		Content:         decision.Rationale,
		EstimatedTokens: int(decision.TotalSavings),
		Latency:         time.Since(startTime),
		Confidence:      float32(decision.SavingsPercent / 100.0),
		Layer:           6, // Claude API layer
	}

	// Generate transparency footer
	resp.TransparencyFooter = p.generateTransparencyFooter(resp)

	// Store response in cache for future lookups
	if p.cacheGraphLayer != nil {
		_ = p.cacheGraphLayer.Store(ctx, req.Query, resp.Content)
	}

	// Record metrics
	if p.metrics != nil && decision.TotalSavings > 0 {
		p.metrics.RecordTokenSavings(int64(decision.TotalSavings))
	}

	return resp
}

// buildResponse constructs a PipelineResponse with transparency footer
func (p *OptimizationPipeline) buildResponse(
	_ *PipelineRequest,
	cacheResult *cache.GraphQueryResult,
	layer int,
	startTime time.Time,
) *PipelineResponse {
	latency := time.Since(startTime)

	resp := &PipelineResponse{
		Source:          cacheResult.Source,
		Content:         cacheResult.Content,
		Layer:           layer,
		Confidence:      cacheResult.Confidence,
		TokensSaved:     cacheResult.TokenSavings,
		EstimatedTokens: int(2500 - cacheResult.TokenSavings), // Rough estimate
		Latency:         latency,
	}

	// Add graph context if applicable
	if cacheResult.IsGraphMatch && len(cacheResult.RelatedNodes) > 0 {
		nodes := make([]graph.Node, len(cacheResult.RelatedNodes))
		copy(nodes, cacheResult.RelatedNodes)
		resp.GraphContext = &GraphContext{
			GraphHit:    true,
			Nodes:       nodes,
			ConfidenceScore: cacheResult.Confidence,
		}
	}

	// Generate transparency footer
	resp.TransparencyFooter = p.generateTransparencyFooter(resp)

	// Record metrics
	if p.metrics != nil {
		p.metrics.RecordTokenSavings(cacheResult.TokenSavings)
	}

	return resp
}

// generateTransparencyFooter creates a user-visible footer showing what happened
func (p *OptimizationPipeline) generateTransparencyFooter(resp *PipelineResponse) string {
	switch resp.Source {
	case "exact_cache":
		return fmt.Sprintf(
			"⚡ Cached response (100%% token savings)\n"+
				"Identical query found in cache\n"+
				"Tokens saved: %d | Latency: %v",
			resp.TokensSaved, resp.Latency,
		)
	case "semantic_cache":
		return fmt.Sprintf(
			"⚡ Cached response (98%% token savings, semantic match)\n"+
				"Similar query found in cache (%.1f%% match)\n"+
				"Tokens saved: %d | Latency: %v",
			resp.Confidence*100, resp.TokensSaved, resp.Latency,
		)
	case "graph":
		graphInfo := ""
		if resp.GraphContext != nil && len(resp.GraphContext.Nodes) > 0 {
			graphInfo = fmt.Sprintf("\nRelated items: %d | Depth: %d",
				len(resp.GraphContext.Nodes), resp.GraphContext.Depth)
		}
		return fmt.Sprintf(
			"🔍 Graph-based response (99%% token savings)\n"+
				"Answered from indexed relationships%s\n"+
				"Tokens saved: %d | Latency: %v",
			graphInfo, resp.TokensSaved, resp.Latency,
		)
	default:
		return fmt.Sprintf(
			"✓ Fresh response from Claude (%d tokens used)\n"+
				"No optimization applied\n"+
				"Latency: %v",
			resp.EstimatedTokens, resp.Latency,
		)
	}
}

// GetStats returns combined pipeline statistics
func (p *OptimizationPipeline) GetStats(ctx context.Context) map[string]interface{} {
	stats := map[string]interface{}{
		"optimizer": p.optimizer.GetMetrics(),
	}

	if p.cacheGraphLayer != nil {
		stats["cache_graph"] = p.cacheGraphLayer.GetStats(ctx)
	}

	return stats
}

// Clear clears all caches
func (p *OptimizationPipeline) Clear(ctx context.Context) error {
	if p.cacheGraphLayer != nil {
		return p.cacheGraphLayer.Clear(ctx, false) // Clear cache only, keep graph
	}
	return nil
}

// ClearGraphData clears graph data (use with caution)
func (p *OptimizationPipeline) ClearGraphData(ctx context.Context) error {
	if p.cacheGraphLayer != nil {
		return p.cacheGraphLayer.Clear(ctx, true) // Clear both cache and graph
	}
	return nil
}
