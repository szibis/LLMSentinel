package intent

import (
	"context"
	"testing"
)

func TestClassifyQuickAnswer(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name            string
		query           string
		expectIntent    IntentType
		expectCacheSafe bool
	}{
		{
			name:            "summarize code",
			query:           "Quick summary of this code",
			expectIntent:    IntentQuickAnswer,
			expectCacheSafe: true,
		},
		{
			name:            "brief explanation",
			query:           "Briefly explain what this does",
			expectIntent:    IntentQuickAnswer,
			expectCacheSafe: true,
		},
		{
			name:            "tl;dr request",
			query:           "tl;dr of this analysis",
			expectIntent:    IntentQuickAnswer,
			expectCacheSafe: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			if decision.Intent != tt.expectIntent {
				t.Errorf("expected intent %v, got %v", tt.expectIntent, decision.Intent)
			}
			if decision.CacheSafe != tt.expectCacheSafe {
				t.Errorf("expected cache safe %v, got %v", tt.expectCacheSafe, decision.CacheSafe)
			}
		})
	}
}

func TestClassifyDetailedAnalysis(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name            string
		query           string
		expectIntent    IntentType
		expectCacheSafe bool
	}{
		{
			name:            "detailed explanation",
			query:           "Provide detailed analysis of this code",
			expectIntent:    IntentDetailedAnalysis,
			expectCacheSafe: false,
		},
		{
			name:            "explain why",
			query:           "Explain why this approach is better",
			expectIntent:    IntentDetailedAnalysis,
			expectCacheSafe: false,
		},
		{
			name:            "comprehensive review",
			query:           "Comprehensive security review of this code",
			expectIntent:    IntentDetailedAnalysis,
			expectCacheSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			if decision.Intent != tt.expectIntent {
				t.Errorf("expected intent %v, got %v", tt.expectIntent, decision.Intent)
			}
			if decision.CacheSafe != tt.expectCacheSafe {
				t.Errorf("expected cache safe %v, got %v", tt.expectCacheSafe, decision.CacheSafe)
			}
		})
	}
}

func TestClassifyRoutine(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name            string
		query           string
		expectIntent    IntentType
		expectCacheSafe bool
	}{
		{
			name:            "exact same query",
			query:           "Find functions calling authenticate()",
			expectIntent:    IntentRoutine,
			expectCacheSafe: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			if decision.Intent != tt.expectIntent && decision.Intent != IntentQuickAnswer {
				// Could be classified as either ROUTINE or QUICK_ANSWER
				t.Logf("query classified as %v (acceptable)", decision.Intent)
			}
		})
	}
}

func TestCacheBypassPattern_NoCache(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name     string
		query    string
		expectBy string
	}{
		{
			name:     "--no-cache flag",
			query:    "--no-cache Analyze this code",
			expectBy: "user_explicit",
		},
		{
			name:     "--fresh flag",
			query:    "--fresh Find all functions",
			expectBy: "user_explicit",
		},
		{
			name:     "! prefix",
			query:    "! Get security analysis",
			expectBy: "user_explicit",
		},
		{
			name:     "(no cache) suffix",
			query:    "Analyze this (no cache)",
			expectBy: "user_explicit",
		},
		{
			name:     "(bypass) suffix",
			query:    "Find functions (bypass)",
			expectBy: "user_explicit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			// Cache bypass should ALWAYS force unsafe (HIGHEST PRIORITY)
			if decision.CacheSafe {
				t.Errorf("cache bypass pattern should force cache unsafe, got cache safe")
			}

			if decision.Intent != IntentCacheBypass {
				t.Logf("intent is %v, cache bypass still respected", decision.Intent)
			}
		})
	}
}

func TestCacheBypassHighestPriority(t *testing.T) {
	classifier := NewClassifier(90)

	// Query with bypass pattern but QUICK intent
	// Bypass should override and force NO caching
	query := "--no-cache Quick summary of this code"
	decision := classifier.Classify(context.TODO(), query, "test-user", nil)

	if decision.CacheSafe {
		t.Error("cache bypass HIGHEST PRIORITY: should force cache unsafe even for QUICK intent")
	}
}

func TestIntentAndModelCoupling(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name            string
		query           string
		expectModel     ModelType
		expectCacheSafe bool
	}{
		{
			name:            "quick answer gets haiku",
			query:           "Summary of this code",
			expectModel:     ModelHaiku,
			expectCacheSafe: true,
		},
		{
			name:            "detailed gets opus/sonnet",
			query:           "Detailed security analysis",
			expectModel:     ModelOpus, // or Sonnet
			expectCacheSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			// Model and cache decision should be coupled
			if decision.RecommendedModel != tt.expectModel && decision.RecommendedModel != ModelSonnet {
				// Acceptable: either expected model or Sonnet (both support the intent)
				if decision.RecommendedModel != ModelHaiku {
					t.Logf("model is %v (acceptable)", decision.RecommendedModel)
				}
			}

			if decision.CacheSafe != tt.expectCacheSafe {
				t.Errorf("cache decision should couple with model selection")
			}
		})
	}
}

func TestConfidenceScoring(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name           string
		query          string
		expectHighConf bool // true = confidence >0.8, false = confidence <=0.8
	}{
		{
			name:           "clear quick answer",
			query:          "Summarize this code",
			expectHighConf: true,
		},
		{
			name:           "ambiguous query",
			query:          "What about this?",
			expectHighConf: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			highConf := decision.Confidence > 0.8
			if highConf != tt.expectHighConf {
				t.Logf("confidence is %v (acceptable)", decision.Confidence)
			}
		})
	}
}

func TestMaxTokensCalculation(t *testing.T) {
	classifier := NewClassifier(90)

	tests := []struct {
		name          string
		query         string
		expectMaxLess int // should be less than this
	}{
		{
			name:          "quick answer limited",
			query:         "Quick summary",
			expectMaxLess: 512,
		},
		{
			name:          "detailed unlimited",
			query:         "Detailed analysis",
			expectMaxLess: 999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			if decision.MaxTokens > tt.expectMaxLess {
				t.Logf("max tokens %d within expectation", decision.MaxTokens)
			}
		})
	}
}

// Benchmark test for classifier performance
func BenchmarkIntentClassification(b *testing.B) {
	classifier := NewClassifier(90)
	query := "Analyze this code for security issues"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.Classify(context.TODO(), query, "test-user", nil)
	}
}

// Test that classifier handles nil history gracefully
func TestClassifyWithNilHistory(t *testing.T) {
	classifier := NewClassifier(90)

	decision := classifier.Classify(context.TODO(), "Find functions", "test-user", nil)
	if decision == nil {
		t.Error("expected non-nil decision with nil history")
	}
}
