package cache

import (
	"context"
	"fmt"

	"github.com/szibis/claude-escalate/internal/graph"
	"github.com/szibis/claude-escalate/internal/metrics"
)

// CacheGraphLayer integrates semantic cache with knowledge graph lookups
// Lookup order: Exact Cache → Semantic Cache → Graph Lookup → Claude (fallback)
type CacheGraphLayer struct {
	cache    *Layer
	graphDB  *graph.GraphDB
	metrics  *metrics.MetricsCollector
	enabled  bool
}

// GraphQueryResult represents the result of a unified cache+graph lookup
type GraphQueryResult struct {
	Source            string                  // "exact_cache", "semantic_cache", "graph", "claude"
	Content           string                  // Response content
	Confidence        float32                 // 0.0-1.0 confidence in answer
	IsGraphMatch      bool                    // True if answer came from graph
	RelatedNodes      []graph.Node            // Related nodes from graph (if applicable)
	TokenSavings      int64                   // Estimated tokens saved
	Error             error                   // Any errors during lookup
}

// NewCacheGraphLayer creates a unified cache+graph lookup layer
func NewCacheGraphLayer(cacheLayer *Layer, graphDB *graph.GraphDB, metrics *metrics.MetricsCollector) *CacheGraphLayer {
	return &CacheGraphLayer{
		cache:   cacheLayer,
		graphDB: graphDB,
		metrics: metrics,
		enabled: true,
	}
}

// Lookup performs unified cache+graph lookup in order:
// 1. Exact cache (Layer 1) - 100% savings, 100% confidence
// 2. Semantic cache (Layer 5) - 98% savings, 99.2% confidence
// 3. Graph lookup (Layer 1.5) - 99% savings, depends on graph quality
// 4. Returns result, caller should fallback to Claude API if needed
func (cgl *CacheGraphLayer) Lookup(ctx context.Context, query string, intent string) *GraphQueryResult {
	if !cgl.enabled {
		return &GraphQueryResult{
			Source:     "disabled",
			Confidence: 0,
		}
	}

	// Step 1: Try exact cache (Layer 1)
	req := &Request{Content: query}
	exactResp, found, _ := cgl.cache.LookupExact(req)
	if found {
		return &GraphQueryResult{
			Source:       "exact_cache",
			Content:      exactResp,
			Confidence:   1.0,
			TokenSavings: 2500, // Typical Claude response
		}
	}

	// Step 2: Try semantic cache (Layer 5) only if safe
	if cgl.shouldUseCacheForIntent(intent) {
		semResp, similarity, found, _ := cgl.cache.LookupSemantic(ctx, req)
		if found {
			return &GraphQueryResult{
				Source:       "semantic_cache",
				Content:      semResp,
				Confidence:   similarity, // Cosine similarity score
				TokenSavings: int64(float32(2500) * 0.98), // 98% savings minus embedding cost
			}
		}
	}

	// Step 3: Try graph lookup (Layer 1.5) for relationship queries
	if cgl.isGraphQuery(query) && cgl.graphDB != nil {
		graphResult := cgl.lookupGraph(ctx, query)
		if graphResult.Found && graphResult.Error == "" {
			return &GraphQueryResult{
				Source:       "graph",
				Content:      graphResult.NodeContent,
				Confidence:   graphResult.ConfidenceScore,
				IsGraphMatch: true,
				RelatedNodes: graphResult.RelatedNodes,
				TokenSavings: int64(float32(2500) * 0.99), // 99% savings
			}
		}
	}

	// No cache hit - need fresh response from Claude
	return &GraphQueryResult{
		Source:     "claude",
		Confidence: 0, // Needs fresh answer
	}
}

// Store caches a response for future lookups
func (cgl *CacheGraphLayer) Store(ctx context.Context, query string, response string) error {
	if !cgl.enabled {
		return nil
	}

	req := &Request{Content: query}
	resp := &Response{Content: response}

	return cgl.cache.Store(ctx, req, resp)
}

// shouldUseCacheForIntent determines if cache is safe for the given intent
func (cgl *CacheGraphLayer) shouldUseCacheForIntent(intent string) bool {
	// Cache is safe for these intent types
	cacheSafeIntents := map[string]bool{
		"quick_answer": true,
		"routine":      true,
		"lookup":       true,
	}

	return cacheSafeIntents[intent]
}

// isGraphQuery checks if the query is likely answerable by the knowledge graph
func (cgl *CacheGraphLayer) isGraphQuery(query string) bool {
	if query == "" {
		return false
	}

	// Keywords that indicate graph-answerable questions
	graphKeywords := []string{
		"find all",
		"functions that",
		"calls to",
		"callers of",
		"who calls",
		"imports",
		"references",
		"uses",
		"defined in",
		"defined at",
		"dependencies",
		"related",
		"hierarchy",
	}

	for _, keyword := range graphKeywords {
		// Simple substring matching for now
		// Could be enhanced with NLP/intent detection
		if len(query) >= len(keyword) && query[:len(keyword)] == keyword {
			return true
		}
	}

	return false
}

// lookupGraph queries the knowledge graph for relationship data
func (cgl *CacheGraphLayer) lookupGraph(ctx context.Context, query string) *graph.GraphLookupResult {
	if cgl.graphDB == nil {
		return &graph.GraphLookupResult{
			Found: false,
			Error: "graph database not available",
		}
	}

	// Extract the function/entity name from the query
	// Simple heuristic: look for quoted strings or capitalized identifiers
	entityName := cgl.extractEntityName(query)
	if entityName == "" {
		return &graph.GraphLookupResult{
			Found: false,
			Error: "could not extract entity name from query",
		}
	}

	// Perform graph lookup based on query type
	if cgl.isFindCallersQuery(query) {
		// Find all functions calling entityName
		nodes, err := cgl.graphDB.FindCallers(ctx, entityName, 10)
		if err != nil {
			return &graph.GraphLookupResult{
				Found: false,
				Error: fmt.Sprintf("graph lookup failed: %v", err),
			}
		}

		if len(nodes) == 0 {
			return &graph.GraphLookupResult{
				Found: false,
				Error: fmt.Sprintf("no callers found for %q", entityName),
			}
		}

		// Format response
		responseContent := cgl.formatCallersResponse(entityName, nodes)
		relatedNodes := make([]graph.Node, len(nodes))
		for i, n := range nodes {
			if n != nil {
				relatedNodes[i] = *n
			}
		}
		return &graph.GraphLookupResult{
			Found:           true,
			NodeID:          nodes[0].ID,
			NodeContent:     responseContent,
			RelatedNodes:    relatedNodes,
			ConfidenceScore: 0.99, // High confidence for graph lookups
		}
	}

	// Get single node info
	node, err := cgl.graphDB.GetNode(ctx, entityName)
	if err != nil {
		return &graph.GraphLookupResult{
			Found: false,
			Error: fmt.Sprintf("graph lookup failed: %v", err),
		}
	}

	if node == nil {
		return &graph.GraphLookupResult{
			Found: false,
			Error: fmt.Sprintf("%q not found in graph", entityName),
		}
	}

	return &graph.GraphLookupResult{
		Found:           true,
		NodeID:          node.ID,
		NodeContent:     node.Content,
		FilePath:        node.FilePath,
		LineNumber:      node.LineNumber,
		ConfidenceScore: 0.95,
	}
}

// extractEntityName tries to extract the entity name from the query
func (cgl *CacheGraphLayer) extractEntityName(query string) string {
	// Look for patterns: "calling <name>", "of <name>", "calls <name>", etc
	patterns := []string{
		" calling ", " of ", " call ", " callers of ", " calls ", " imports ", " uses ", "who calls ",
	}

	for _, pattern := range patterns {
		idx := len(query)
		for i := 0; i <= len(query)-len(pattern); i++ {
			if query[i:i+len(pattern)] == pattern {
				idx = i + len(pattern)
				break
			}
		}

		if idx < len(query) {
			// Extract the word after the pattern
			end := idx
			for end < len(query) && query[end] != ' ' && query[end] != '.' && query[end] != '?' {
				end++
			}

			if end > idx {
				return query[idx:end]
			}
		}
	}

	return ""
}

// isFindCallersQuery checks if query is asking for callers
func (cgl *CacheGraphLayer) isFindCallersQuery(query string) bool {
	callersPatterns := []string{
		"functions that call",
		"callers of",
		"who calls",
		"calls to",
		"calling ",
	}

	// Check for specific caller-related patterns
	for _, pattern := range callersPatterns {
		for i := 0; i <= len(query)-len(pattern); i++ {
			if query[i:i+len(pattern)] == pattern {
				return true
			}
		}
	}

	// Special case: "find all functions calling"
	if len(query) > 30 && query[:30] == "find all functions calling " {
		return true
	}

	return false
}

// formatCallersResponse formats a list of callers as a response
func (cgl *CacheGraphLayer) formatCallersResponse(targetName string, callers []*graph.Node) string {
	if len(callers) == 0 {
		return fmt.Sprintf("No functions found calling %q.", targetName)
	}

	response := fmt.Sprintf("Functions calling %q:\n", targetName)
	for _, caller := range callers {
		response += fmt.Sprintf("- %s (%s) at %s:%d\n",
			caller.Name, caller.Type, caller.FilePath, caller.LineNumber)
	}

	return response
}

// GetStats returns combined cache and graph statistics
func (cgl *CacheGraphLayer) GetStats(ctx context.Context) map[string]interface{} {
	stats := map[string]interface{}{
		"cache": cgl.cache.GetStats(),
	}

	if cgl.graphDB != nil {
		graphStats, err := cgl.graphDB.GetStats(ctx)
		if err == nil {
			stats["graph"] = graphStats
		}
	}

	return stats
}

// Clear removes all cached entries and optionally clears graph
func (cgl *CacheGraphLayer) Clear(ctx context.Context, clearGraph bool) error {
	cgl.cache.Clear()

	if clearGraph && cgl.graphDB != nil {
		return cgl.graphDB.Clear(ctx)
	}

	return nil
}
