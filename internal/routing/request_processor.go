package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/szibis/claude-escalate/internal/client"
	"github.com/szibis/claude-escalate/internal/models"
)

// RequestProcessor handles intelligent request routing and optimization
type RequestProcessor struct {
	router        *IntelligentRouter
	unifiedClient *models.UnifiedClient
	cache         map[string]CachedResponse // simple response cache
}

// ProcessedRequest contains routing and optimization decisions
type ProcessedRequest struct {
	OriginalText      string
	OptimizedText     string
	SelectedModel     string
	TaskType          TaskType
	Sensitivity       DataSensitivity
	Complexity        int
	OptimizationApplied bool
	Strategy          TokenOptimizationStrategy
	ModelChars        ModelCharacteristics
}

// CachedResponse stores a cached response
type CachedResponse struct {
	Model    string
	Response *client.MessageResponse
}

// NewRequestProcessor creates a new request processor
func NewRequestProcessor(router *IntelligentRouter, client *models.UnifiedClient) *RequestProcessor {
	return &RequestProcessor{
		router:        router,
		unifiedClient: client,
		cache:         make(map[string]CachedResponse),
	}
}

// ProcessRequest analyzes and routes a request
func (rp *RequestProcessor) ProcessRequest(text string) (*ProcessedRequest, error) {
	// Route to appropriate model
	selectedModel, taskType, sensitivity, err := rp.router.RouteRequest(text)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}

	// Get optimization strategy for this task type
	strategy := rp.router.GetTokenOptimizationStrategy(taskType)

	// Get model characteristics
	modelChars := rp.router.GetModelCharacteristics(selectedModel)

	// Apply optimizations
	optimizedText := text
	optimizationApplied := false

	if strategy.Aggressive || strategy.RemoveExamples || strategy.Summarize {
		optimizedText, optimizationApplied = rp.optimizeText(text, strategy)
	}

	// Calculate complexity
	_, _, complexity := rp.router.detector.DetectTask(text)

	return &ProcessedRequest{
		OriginalText:       text,
		OptimizedText:      optimizedText,
		SelectedModel:      selectedModel,
		TaskType:           taskType,
		Sensitivity:        sensitivity,
		Complexity:         complexity,
		OptimizationApplied: optimizationApplied,
		Strategy:           strategy,
		ModelChars:         modelChars,
	}, nil
}

// ExecuteRequest processes and executes a request with automatic model selection
func (rp *RequestProcessor) ExecuteRequest(ctx context.Context, text string) (*client.MessageResponse, *ProcessedRequest, error) {
	// Process and route request
	processed, err := rp.ProcessRequest(text)
	if err != nil {
		return nil, nil, err
	}

	// Check cache
	cacheKey := rp.getCacheKey(processed.SelectedModel, processed.OptimizedText)
	if cached, exists := rp.cache[cacheKey]; exists {
		return cached.Response, processed, nil
	}

	// Execute with selected model
	resp, err := rp.unifiedClient.CreateMessage(ctx, processed.OptimizedText, processed.SelectedModel)
	if err != nil {
		return nil, processed, err
	}

	// Cache response
	rp.cache[cacheKey] = CachedResponse{
		Model:    processed.SelectedModel,
		Response: resp,
	}

	return resp, processed, nil
}

// optimizeText applies token optimization strategies to text
func (rp *RequestProcessor) optimizeText(text string, strategy TokenOptimizationStrategy) (string, bool) {
	optimized := text

	// Remove extra whitespace
	optimized = strings.Join(strings.Fields(optimized), " ")

	// Remove examples if configured
	if strategy.RemoveExamples {
		optimized = rp.removeExamples(optimized)
	}

	// Summarize if needed
	if strategy.Summarize {
		optimized = rp.summarizeText(optimized)
	}

	// Add security context if needed
	if strategy.AddSecurityContext {
		optimized = rp.addSecurityContext(optimized)
	}

	return optimized, optimized != text
}

// removeExamples strips example text from prompts
func (rp *RequestProcessor) removeExamples(text string) string {
	patterns := []string{
		"Example:", "Example 1:", "Example 2:",
		"For example:", "e.g.", "i.e.",
		"Here's an example:", "Try this:",
	}

	result := text
	for _, pattern := range patterns {
		// Simple removal - in production, use more sophisticated parsing
		idx := strings.Index(strings.ToLower(result), strings.ToLower(pattern))
		if idx != -1 {
			result = result[:idx]
		}
	}

	return strings.TrimSpace(result)
}

// summarizeText extracts key information from text
func (rp *RequestProcessor) summarizeText(text string) string {
	// For now, just limit to first 300 words
	words := strings.Fields(text)
	if len(words) > 300 {
		words = words[:300]
		return strings.Join(words, " ") + "..."
	}
	return text
}

// addSecurityContext reminds model to handle sensitive data carefully
func (rp *RequestProcessor) addSecurityContext(text string) string {
	prefix := "[SENSITIVE: Handle with care, don't log or store this request]\n\n"
	return prefix + text
}

// getCacheKey creates a cache key for a request
func (rp *RequestProcessor) getCacheKey(model, text string) string {
	// Simple hash-based cache key
	// In production, use proper hash function
	return fmt.Sprintf("%s:%s", model, hashString(text))
}

// hashString creates a simple hash of a string
func hashString(s string) string {
	// Very simple hash for demonstration
	// In production, use crypto/sha256
	hash := uint32(5381)
	for _, c := range s {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return fmt.Sprintf("%d", hash)
}

// ClearCache clears the response cache
func (rp *RequestProcessor) ClearCache() {
	rp.cache = make(map[string]CachedResponse)
}

// GetCacheStats returns cache statistics
func (rp *RequestProcessor) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cached_responses": len(rp.cache),
	}
}

// HybridExecutor manages hybrid local/cloud execution
type HybridExecutor struct {
	processor     *RequestProcessor
	preferences   HybridPreferences
}

// HybridPreferences defines hybrid execution preferences
type HybridPreferences struct {
	PreferLocal      bool   // prefer local models when possible
	MaxLocalTokens   int    // max tokens to use local model
	OfflineFirstFor  []TaskType // task types to execute locally first
	FallbackToCloud  bool   // fall back to cloud if local fails
}

// NewHybridExecutor creates a hybrid executor
func NewHybridExecutor(processor *RequestProcessor, prefs HybridPreferences) *HybridExecutor {
	return &HybridExecutor{
		processor:   processor,
		preferences: prefs,
	}
}

// Execute handles hybrid execution with fallback
func (he *HybridExecutor) Execute(ctx context.Context, text string) (*client.MessageResponse, *ExecutionInfo, error) {
	processed, err := he.processor.ProcessRequest(text)
	if err != nil {
		return nil, nil, err
	}

	execInfo := &ExecutionInfo{
		RequestText:  text,
		TaskType:     processed.TaskType,
		Sensitivity:  processed.Sensitivity,
		Attempts:     []AttemptInfo{},
	}

	// Decide execution strategy
	shouldUseLocal := he.shouldUseLocal(processed)

	if shouldUseLocal {
		// Try local first
		localModel := "local-llm"
		resp, err := he.processor.unifiedClient.CreateMessage(ctx, processed.OptimizedText, localModel)
		if err == nil {
			execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
				Model:   localModel,
				Success: true,
				Reason:  "Local execution successful",
			})
			execInfo.ExecutedLocally = true
			return resp, execInfo, nil
		}

		// Log failed attempt
		execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
			Model:   localModel,
			Success: false,
			Reason:  fmt.Sprintf("Local execution failed: %v", err),
		})

		// Fallback to cloud if configured
		if he.preferences.FallbackToCloud {
			cloudModel := processed.SelectedModel
			if cloudModel == localModel {
				cloudModel = "claude-sonnet" // default fallback
			}

			resp, err := he.processor.unifiedClient.CreateMessage(ctx, processed.OptimizedText, cloudModel)
			if err == nil {
				execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
					Model:   cloudModel,
					Success: true,
					Reason:  "Cloud fallback successful",
				})
				execInfo.ExecutedLocally = false
				return resp, execInfo, nil
			}

			execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
				Model:   cloudModel,
				Success: false,
				Reason:  fmt.Sprintf("Cloud fallback failed: %v", err),
			})
		}

		return nil, execInfo, fmt.Errorf("all execution attempts failed")
	}

	// Execute on cloud model
	resp, err := he.processor.unifiedClient.CreateMessage(ctx, processed.OptimizedText, processed.SelectedModel)
	if err != nil {
		execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
			Model:   processed.SelectedModel,
			Success: false,
			Reason:  fmt.Sprintf("Execution failed: %v", err),
		})
		return nil, execInfo, err
	}

	execInfo.Attempts = append(execInfo.Attempts, AttemptInfo{
		Model:   processed.SelectedModel,
		Success: true,
		Reason:  "Cloud execution successful",
	})
	execInfo.ExecutedLocally = false

	return resp, execInfo, nil
}

// shouldUseLocal determines if request should use local model
func (he *HybridExecutor) shouldUseLocal(processed *ProcessedRequest) bool {
	// Always local for secrets/sensitive data
	if processed.Sensitivity == SensitivitySecrets || processed.Sensitivity == SensitivityConfidential {
		return true
	}

	// Check if task type should run locally
	for _, tType := range he.preferences.OfflineFirstFor {
		if tType == processed.TaskType {
			return true
		}
	}

	// Check preferences
	if he.preferences.PreferLocal && processed.Complexity <= 3 {
		return true
	}

	return processed.SelectedModel == "local-llm"
}

// ExecutionInfo tracks execution details
type ExecutionInfo struct {
	RequestText     string
	TaskType        TaskType
	Sensitivity     DataSensitivity
	ExecutedLocally bool
	Attempts        []AttemptInfo
}

// AttemptInfo tracks individual execution attempts
type AttemptInfo struct {
	Model   string
	Success bool
	Reason  string
}
