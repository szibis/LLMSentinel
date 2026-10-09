package batch

import (
	"github.com/szibis/claude-escalate/internal/costs"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCacheManagerModelIsolationAndSimilarity(t *testing.T) {
	cm := NewCacheManager()
	if stats := cm.CacheStats(); stats.PromptCount != 0 || stats.TotalAccesses != 0 {
		t.Fatal("new cache not empty")
	}
	a := cm.CachePrompt("shared text", "sonnet", 100)
	b := cm.CachePrompt("shared text", "haiku", 100)
	if a == b {
		t.Error("different models share prompt cache identity")
	}
	if cm.FindSimilarPrompt("shared text", "sonnet", 1) != a || cm.FindSimilarPrompt("shared text", "haiku", 1) != b {
		t.Error("model prompt overwritten")
	}
	hash := cm.CachePrompt("abcdefghij", "opus", 20)
	if cm.FindSimilarPrompt("abcdefghijk", "opus", .9) != hash {
		t.Error("similar prompt missed")
	}
	if cm.FindSimilarPrompt("unrelated", "opus", .9) != "" || cm.FindSimilarPrompt("abcdefghij", "sonnet", 1) != "" {
		t.Error("dissimilar/model mismatch returned")
	}
	cm.CacheResponse("response", "sonnet", 200)
	stats := cm.CacheStats()
	if stats.PromptCount != 3 || stats.ResponseCount != 1 || stats.TotalSize <= 0 || stats.TotalAccesses != 4 || stats.AverageCacheAge < 0 {
		t.Fatalf("stats=%+v", stats)
	}
}
func TestCacheManagerExpirationAndEviction(t *testing.T) {
	cm := NewCacheManager()
	cm.maxCacheSize = 2
	first := cm.CacheResponse("first", "sonnet", 1)
	cm.responses[first].LastAccessedAt = time.Now().Add(-time.Hour)
	cm.CacheResponse("second", "sonnet", 1)
	cm.CacheResponse("third", "sonnet", 1)
	if stats := cm.CacheStats(); stats.ResponseCount != 2 {
		t.Errorf("response capacity not enforced: %+v", stats)
	}
	if _, ok := cm.responses[first]; ok {
		t.Error("oldest response not evicted")
	}
	cm = NewCacheManager()
	cm.maxCacheSize = 2
	p := cm.CachePrompt("prompt", "sonnet", 1)
	cm.prompts[p].LastAccessedAt = time.Now().Add(-time.Hour)
	cm.CacheResponse("r1", "sonnet", 1)
	cm.CacheResponse("r2", "sonnet", 1)
	if cm.FindSimilarPrompt("prompt", "sonnet", 1) != "" {
		t.Error("oldest prompt not evicted")
	}
	cm = NewCacheManager()
	p = cm.CachePrompt("prompt", "sonnet", 1)
	r := cm.CacheResponse("response", "sonnet", 1)
	cm.SetCacheTTL(time.Hour)
	cm.prompts[p].CreatedAt = time.Now().Add(-2 * time.Hour)
	cm.responses[r].CreatedAt = time.Now().Add(-2 * time.Hour)
	if cm.FindSimilarPrompt("prompt", "sonnet", .9) != "" {
		t.Error("expired prompt returned")
	}
	if got := cm.GetCacheOptimizations("prompt", 1, "sonnet"); got.CanUseCachedPrompt {
		t.Error("expired prompt optimized")
	}
	if cleared := cm.ClearExpiredEntries(); cleared != 2 {
		t.Errorf("cleared=%d", cleared)
	}
	if cleared := cm.ClearExpiredEntries(); cleared != 0 {
		t.Error("cleared live entries")
	}
}
func TestCacheOptimizationSavingsAndRationale(t *testing.T) {
	cm := NewCacheManager()
	prompt := "prompt text"
	cm.CachePrompt(prompt, "sonnet", 100)
	cached := cm.GetCacheOptimizations(prompt, 20, "sonnet")
	if !cached.CanUseCachedPrompt || !cached.RecommendBatching || cached.EstimatedSavings <= 0 || cached.SavingsPercent < 0 || cached.SavingsPercent > 100 || math.IsNaN(cached.SavingsPercent) {
		t.Errorf("cached optimization=%+v", cached)
	}
	if got := cm.GetCacheOptimizations("uncached", 20, "sonnet"); got.CanUseCachedPrompt || !got.RecommendBatching {
		t.Fatalf("uncached optimization=%+v", got)
	}
	cm.minSavingsPercent = 101
	if got := cm.GetCacheOptimizations(prompt, 20, "sonnet"); !got.CanUseCachedPrompt || got.RecommendBatching {
		t.Fatalf("cache-only optimization=%+v", got)
	}
	if got := cm.GetCacheOptimizations("uncached", 20, "sonnet"); got.Rationale != "no optimization opportunity" {
		t.Errorf("no optimization=%+v", got)
	}
}

func TestCacheOnlySavingsMatchChosenStrategy(t *testing.T) {
	cm := NewCacheManager()
	prompt := strings.Repeat("x", 1000)
	cm.CachePrompt(prompt, "sonnet", 1000)
	cm.minSavingsPercent = 101
	opt := cm.GetCacheOptimizations(prompt, 1000, "sonnet")
	normal, err := cm.calculator.CalculateCost("sonnet", costs.TokenCosts{InputTokens: len(prompt), OutputTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	want := opt.EstimatedSavings / normal.TotalCost * 100
	if opt.RecommendBatching || math.Abs(opt.SavingsPercent-want) > 1e-9 {
		t.Fatalf("cache-only percent=%f want=%f recommendation=%v", opt.SavingsPercent, want, opt.RecommendBatching)
	}
}
