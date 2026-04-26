package batch

import (
	// #nosec G501 - MD5 is used only for non-cryptographic hash of cache keys, not for security-sensitive hashing
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/costs"
)

// CacheEntry represents a cached prompt or response
type CacheEntry struct {
	Hash              string // MD5 hash of content
	Content           string // Original content (for matching)
	ContentLength     int
	Model             string
	CreatedAt         time.Time
	LastAccessedAt    time.Time
	AccessCount       int
	EstimatedTokens   int
	CacheFillPercent  float64 // 0.0-1.0
}

// CacheOptimization suggests cache-based optimizations
type CacheOptimization struct {
	CanUseCachedPrompt   bool
	CanUseCachedResponse bool
	CachedPromptHash     string
	CachedResponseHash   string
	EstimatedSavings     float64
	SavingsPercent       float64
	CacheAge             time.Duration
	RecommendBatching    bool
	Rationale            string
}

// CacheManager manages prompt/response caching for optimization
type CacheManager struct {
	prompts           map[string]*CacheEntry
	responses         map[string]*CacheEntry
	maxCacheSize      int
	cacheTTL          time.Duration
	calculator        *costs.Calculator
	minSavingsPercent float64
	mu                sync.RWMutex
}

// NewCacheManager creates a cache manager with default settings
func NewCacheManager() *CacheManager {
	return &CacheManager{
		prompts:           make(map[string]*CacheEntry),
		responses:         make(map[string]*CacheEntry),
		maxCacheSize:      1000,
		cacheTTL:          24 * time.Hour,
		calculator:        costs.NewCalculator(),
		minSavingsPercent: 5.0,
	}
}

// CachePrompt stores a prompt in the cache
func (cm *CacheManager) CachePrompt(content string, model string, estimatedTokens int) string {
	hash := cm.hashContent(content)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	entry := &CacheEntry{
		Hash:            hash,
		Content:         content,
		ContentLength:   len(content),
		Model:           model,
		CreatedAt:       now,
		LastAccessedAt:  now,
		AccessCount:     1,
		EstimatedTokens: estimatedTokens,
	}

	cm.prompts[hash] = entry
	cm.evictIfNeeded()

	return hash
}

// CacheResponse stores a response in the cache
func (cm *CacheManager) CacheResponse(content string, model string, estimatedTokens int) string {
	hash := cm.hashContent(content)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	entry := &CacheEntry{
		Hash:            hash,
		Content:         content,
		ContentLength:   len(content),
		Model:           model,
		CreatedAt:       now,
		LastAccessedAt:  now,
		AccessCount:     1,
		EstimatedTokens: estimatedTokens,
	}

	cm.responses[hash] = entry
	cm.evictIfNeeded()

	return hash
}

// FindSimilarPrompt finds a cached prompt similar to the given content
// Returns the hash if found, empty string if not
func (cm *CacheManager) FindSimilarPrompt(content string, model string, similarityThreshold float64) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	contentHash := cm.hashContent(content)

	// First try exact match
	if entry, exists := cm.prompts[contentHash]; exists {
		if entry.Model == model && time.Since(entry.CreatedAt) < cm.cacheTTL {
			return entry.Hash
		}
	}

	// Then try fuzzy match (Jaro-Winkler similarity or simple substring)
	// For now, simple substring match with 80%+ similarity
	for hash, entry := range cm.prompts {
		if entry.Model == model && time.Since(entry.CreatedAt) < cm.cacheTTL {
			similarity := cm.calculateSimilarity(entry.Content, content)
			if similarity >= similarityThreshold {
				return hash
			}
		}
	}

	return ""
}

// GetCacheOptimizations suggests cache and batch optimizations for a request
func (cm *CacheManager) GetCacheOptimizations(prompt string, estimatedOutput int, model string) CacheOptimization {
	opt := CacheOptimization{
		CanUseCachedPrompt:   false,
		CanUseCachedResponse: false,
		EstimatedSavings:     0,
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	now := time.Now()

	// Check for cached prompt (90% similarity threshold)
	if cachedPromptHash := cm.findSimilarPromptLocked(prompt, model, 0.90); cachedPromptHash != "" {
		if entry, exists := cm.prompts[cachedPromptHash]; exists && time.Since(entry.CreatedAt) < cm.cacheTTL {
			opt.CanUseCachedPrompt = true
			opt.CachedPromptHash = cachedPromptHash
			opt.CacheAge = now.Sub(entry.CreatedAt)

			// Calculate savings from cache
			// Cached prompts cost 10% of normal input cost
			tokens := costs.TokenCosts{
				InputTokens:      len(prompt),
				OutputTokens:     estimatedOutput,
				CacheReadTokens:  len(prompt),
				IsCached:         true,
			}
			cachedBreakdown, _ := cm.calculator.CalculateCost(model, tokens)
			normalBreakdown, _ := cm.calculator.CalculateCost(model, costs.TokenCosts{
				InputTokens:  len(prompt),
				OutputTokens: estimatedOutput,
			})

			opt.EstimatedSavings += normalBreakdown.TotalCost - cachedBreakdown.TotalCost
		}
	}

	// Estimate additional batch savings
	batchTokens := costs.TokenCosts{
		InputTokens:  len(prompt),
		OutputTokens: estimatedOutput,
		IsBatchAPI:   true,
	}
	batchBreakdown, _ := cm.calculator.CalculateCost(model, batchTokens)
	normalBreakdown, _ := cm.calculator.CalculateCost(model, costs.TokenCosts{
		InputTokens:  len(prompt),
		OutputTokens: estimatedOutput,
	})

	batchSavings := normalBreakdown.TotalCost - batchBreakdown.TotalCost
	totalSavings := opt.EstimatedSavings + batchSavings

	if normalBreakdown.TotalCost > 0 {
		opt.SavingsPercent = (totalSavings / normalBreakdown.TotalCost) * 100
	}

	// Recommend batching if cache + batch saves significant amount
	opt.RecommendBatching = opt.SavingsPercent >= cm.minSavingsPercent

	if opt.CanUseCachedPrompt && opt.RecommendBatching {
		opt.Rationale = fmt.Sprintf("use cached prompt + batch API: save %.1f%% ($%.4f)",
			opt.SavingsPercent, opt.EstimatedSavings+batchSavings)
	} else if opt.CanUseCachedPrompt {
		opt.Rationale = fmt.Sprintf("use cached prompt: save %.1f%% ($%.4f)",
			opt.SavingsPercent, opt.EstimatedSavings)
	} else if opt.RecommendBatching {
		opt.Rationale = fmt.Sprintf("use batch API: save %.1f%% ($%.4f)",
			opt.SavingsPercent, batchSavings)
	} else {
		opt.Rationale = "no optimization opportunity"
	}

	return opt
}

// ClearExpiredEntries removes entries older than cacheTTL
func (cm *CacheManager) ClearExpiredEntries() (cleared int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()

	// Clear expired prompts
	for hash, entry := range cm.prompts {
		if now.Sub(entry.CreatedAt) > cm.cacheTTL {
			delete(cm.prompts, hash)
			cleared++
		}
	}

	// Clear expired responses
	for hash, entry := range cm.responses {
		if now.Sub(entry.CreatedAt) > cm.cacheTTL {
			delete(cm.responses, hash)
			cleared++
		}
	}

	return
}

// CacheStats returns cache statistics
func (cm *CacheManager) CacheStats() CacheStatistics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := CacheStatistics{
		PromptCount:     len(cm.prompts),
		ResponseCount:   len(cm.responses),
		TotalSize:       0,
		TotalAccesses:   0,
		AverageCacheAge: 0,
	}

	if stats.PromptCount == 0 && stats.ResponseCount == 0 {
		return stats
	}

	now := time.Now()
	totalAge := time.Duration(0)
	entryCount := 0

	for _, entry := range cm.prompts {
		stats.TotalSize += entry.ContentLength
		stats.TotalAccesses += entry.AccessCount
		totalAge += now.Sub(entry.CreatedAt)
		entryCount++
	}

	for _, entry := range cm.responses {
		stats.TotalSize += entry.ContentLength
		stats.TotalAccesses += entry.AccessCount
		totalAge += now.Sub(entry.CreatedAt)
		entryCount++
	}

	if entryCount > 0 {
		stats.AverageCacheAge = totalAge / time.Duration(entryCount)
	}

	return stats
}

// CacheStatistics contains cache metrics
type CacheStatistics struct {
	PromptCount     int
	ResponseCount   int
	TotalSize       int // bytes
	TotalAccesses   int
	AverageCacheAge time.Duration
}

// SetCacheTTL sets time-to-live for cached entries
func (cm *CacheManager) SetCacheTTL(ttl time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cacheTTL = ttl
}

// Private helper methods

func (cm *CacheManager) hashContent(content string) string {
	// #nosec G401 - MD5 used only for cache key hashing, not security-sensitive
	return fmt.Sprintf("%x", md5.Sum([]byte(content)))
}

func (cm *CacheManager) calculateSimilarity(s1, s2 string) float64 {
	// Simple implementation: Jaro-Winkler-like scoring
	// For now, use character overlap percentage
	if len(s1) == 0 && len(s2) == 0 {
		return 1.0
	}

	longer := s1
	shorter := s2

	if len(s2) > len(s1) {
		longer = s2
		shorter = s1
	}

	if len(longer) == 0 {
		return 0.0
	}

	// Count character matches in order
	matches := 0
	j := 0
	for i := 0; i < len(shorter) && j < len(longer); i++ {
		if shorter[i] == longer[j] {
			matches++
		}
		j++
	}

	return float64(matches) / float64(len(longer))
}

func (cm *CacheManager) findSimilarPromptLocked(content string, model string, threshold float64) string {
	contentHash := cm.hashContent(content)

	// Exact match first
	if entry, exists := cm.prompts[contentHash]; exists && entry.Model == model {
		return entry.Hash
	}

	// Fuzzy match
	for hash, entry := range cm.prompts {
		if entry.Model == model {
			sim := cm.calculateSimilarity(entry.Content, content)
			if sim >= threshold {
				return hash
			}
		}
	}

	return ""
}

func (cm *CacheManager) evictIfNeeded() {
	// Simple eviction: remove oldest entry if cache is full
	totalEntries := len(cm.prompts) + len(cm.responses)

	if totalEntries > cm.maxCacheSize {
		// Find and remove oldest entry
		var oldestHash string
		var oldestTime time.Time

		for hash, entry := range cm.prompts {
			if oldestTime.IsZero() || entry.LastAccessedAt.Before(oldestTime) {
				oldestHash = hash
				oldestTime = entry.LastAccessedAt
			}
		}

		for hash, entry := range cm.responses {
			if entry.LastAccessedAt.Before(oldestTime) {
				oldestHash = hash
				oldestTime = entry.LastAccessedAt
			}
		}

		if oldestHash != "" {
			delete(cm.prompts, oldestHash)
			delete(cm.responses, oldestHash)
		}
	}
}
