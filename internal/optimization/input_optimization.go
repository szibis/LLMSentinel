package optimization

import (
	"context"
	"encoding/json"
	"fmt"
)

// InputOptimizer coordinates all input optimization techniques
type InputOptimizer struct {
	dedup      *RequestDeduplicator
	formatter  *InputFormatter
	compressor *ParameterCompressor
}

// TokenSavings represents token reduction statistics
type TokenSavings struct {
	TotalTokens int
	Percent     float64
	ByLayer     map[string]int
}

// NewInputOptimizer creates a new input optimizer
func NewInputOptimizer() *InputOptimizer {
	return &InputOptimizer{
		dedup:      NewRequestDeduplicator(),
		formatter:  NewInputFormatter(),
		compressor: NewParameterCompressor(),
	}
}

// OptimizeInput applies all input optimization techniques to a request
func (io *InputOptimizer) OptimizeInput(ctx context.Context, req *PipelineRequest) (*PipelineRequest, *TokenSavings, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("request cannot be nil")
	}

	optimized := &PipelineRequest{
		Query:     req.Query,
		Intent:    req.Intent,
		Tool:      req.Tool,
		Params:    req.Params,
		Context:   req.Context,
		Timestamp: req.Timestamp,
	}

	savings := &TokenSavings{
		TotalTokens: 0,
		Percent:     0.0,
		ByLayer:     make(map[string]int),
	}

	// Step 1: Format query to structured form (reduces query ambiguity)
	if formatted, err := io.formatter.StructuredFormat(req.Query); err == nil && formatted != "" {
		// Estimate token savings from formatting
		queryTokens := len(optimized.Query) / 4 // Rough estimate: 4 chars per token
		formattedTokens := len(formatted) / 4
		querySavings := queryTokens - formattedTokens
		if querySavings > 0 {
			optimized.Query = formatted
			savings.ByLayer["formatting"] = querySavings
			savings.TotalTokens += querySavings
		}
	}

	// Step 2: Clean up query whitespace
	if cleaned, err := io.formatter.RemoveUnnecessaryWhitespace(optimized.Query); err == nil && cleaned != "" {
		cleanTokens := len(cleaned) / 4
		origTokens := len(optimized.Query) / 4
		cleanSavings := origTokens - cleanTokens
		if cleanSavings > 0 {
			optimized.Query = cleaned
			savings.ByLayer["whitespace"] = cleanSavings
			savings.TotalTokens += cleanSavings
		}
	}

	// Step 3: Shorten common verbose terms
	if shortened, err := io.formatter.ShortenCommonTerms(optimized.Query); err == nil && shortened != "" {
		shortTokens := len(shortened) / 4
		origTokens := len(optimized.Query) / 4
		shortSavings := origTokens - shortTokens
		if shortSavings > 0 {
			optimized.Query = shortened
			savings.ByLayer["term_shortening"] = shortSavings
			savings.TotalTokens += shortSavings
		}
	}

	// Step 4: Compress parameters (biggest win typically)
	if len(optimized.Params) > 0 {
		originalParams, _ := json.Marshal(optimized.Params)
		originalParamsSize := len(originalParams)

		// Remove defaults
		reduced, _ := io.compressor.RemoveDefaults(optimized.Params)
		reducedJSON, _ := json.Marshal(reduced)
		reducedSize := len(reducedJSON)
		reduceSavings := originalParamsSize - reducedSize

		// Abbreviate keys
		abbreviated, _ := io.compressor.AbbreviateKeys(reduced)
		abbrevJSON, _ := json.Marshal(abbreviated)
		abbrevSize := len(abbrevJSON)
		abbrevSavings := reducedSize - abbrevSize

		optimized.Params = abbreviated

		paramSavings := originalParamsSize - abbrevSize
		if paramSavings > 0 {
			paramTokens := paramSavings / 4
			savings.ByLayer["parameters"] = paramTokens
			savings.TotalTokens += paramTokens
		}

		if reduceSavings > 0 {
			savings.ByLayer["parameter_defaults"] = reduceSavings / 4
		}
		if abbrevSavings > 0 {
			savings.ByLayer["parameter_abbreviation"] = abbrevSavings / 4
		}
	}

	// Calculate percentage savings
	// Estimate input size: query + params
	querySize := len(optimized.Query) / 4
	paramsSize := 0
	if optimized.Params != nil {
		paramsJSON, _ := json.Marshal(optimized.Params)
		paramsSize = len(paramsJSON) / 4
	}
	totalSize := querySize + paramsSize

	if totalSize > 0 {
		savings.Percent = (float64(savings.TotalTokens) / float64(totalSize+savings.TotalTokens)) * 100
	}

	return optimized, savings, nil
}

// OptimizeInputBatch optimizes a batch of requests
func (io *InputOptimizer) OptimizeInputBatch(ctx context.Context, requests []*PipelineRequest) ([]*PipelineRequest, *TokenSavings, error) {
	if len(requests) == 0 {
		return nil, nil, fmt.Errorf("empty request batch")
	}

	optimized := make([]*PipelineRequest, 0, len(requests))
	totalSavings := &TokenSavings{
		ByLayer: make(map[string]int),
	}

	for _, req := range requests {
		optReq, savings, err := io.OptimizeInput(ctx, req)
		if err != nil {
			return nil, nil, err
		}

		optimized = append(optimized, optReq)

		// Accumulate savings
		totalSavings.TotalTokens += savings.TotalTokens
		for layer, tokens := range savings.ByLayer {
			totalSavings.ByLayer[layer] += tokens
		}
	}

	// Average percentage
	if len(requests) > 0 {
		totalSavings.Percent /= float64(len(requests))
	}

	return optimized, totalSavings, nil
}

// GetOptimizationMetrics returns current optimization statistics
func (io *InputOptimizer) GetOptimizationMetrics() map[string]interface{} {
	return map[string]interface{}{
		"dedup": io.dedup.GetStats(),
	}
}
