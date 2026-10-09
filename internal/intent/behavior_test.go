package intent

import (
	"context"
	"fmt"
	"github.com/szibis/claude-escalate/internal/models"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSentimentToneUrgencyAndComplexity(t *testing.T) {
	sa := NewSentimentAnalyzer()
	for _, tc := range []struct {
		query, tone string
		urgent      bool
	}{{"ordinary text", "neutral", false}, {"PLEASE help", "polite", false}, {"hey btw lol", "casual", false}, {"URGENT asap critical!!!????", "urgent", true}, {"frustrated and stuck", "frustrated", true}, {"can't proceed", "frustrated", false}, {"", "neutral", false}, {strings.Repeat("A!", 1000), "neutral", false}} {
		got := sa.AnalyzeSentiment(tc.query)
		if got.Tone != tc.tone || got.Urgent != tc.urgent || math.IsNaN(got.ComplexityScore) || got.ComplexityScore < 0 || got.ComplexityScore > 1 || got.UrgencyScore < 0 || got.UrgencyScore > 1 || got.Timestamp.IsZero() {
			t.Errorf("sentiment %q=%+v", tc.query, got)
		}
	}
	if got := sa.AnalyzeSentiment("urgent asap critical!!!????"); got.UrgencyScore != 1 {
		t.Fatalf("urgency cap=%v", got.UrgencyScore)
	}
}
func TestFeedbackFreshnessModulatesCachedSummary(t *testing.T) {
	c := newOfflineClassifier(t)
	ctx := context.Background()
	first := c.Classify(ctx, "quick summary", "user", nil)
	if !first.CacheSafe {
		t.Fatal("initial summary should be cache safe")
	}
	c.RecordFeedback("user", first, "poor")
	next := c.Classify(ctx, "summary", "user", nil)
	if next.CacheSafe || next.Intent != IntentDetailedAnalysis || !strings.Contains(next.Explanation, "user history") {
		t.Errorf("poor cached-summary feedback not respected: %+v", next)
	}
	for _, rating := range []string{"excellent", "perfect", "good", "okay", "unknown"} {
		c.RecordFeedback("user", next, rating)
	}
	feedback := c.userFeedback["user"]
	if feedback.PositiveFeedbackCount != 3 || feedback.NegativeFeedbackCount != 1 || feedback.LastFeedbackTime.IsZero() {
		t.Fatalf("feedback=%+v", feedback)
	}
}
func TestClassifierOfflineModelOutputContract(t *testing.T) {
	c := newOfflineClassifier(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.onnx"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &models.ModelConfig{CachePath: dir, Intent: models.ModelSubConfig{Enabled: true, Source: "local", ModelID: "fixture"}}
	manager, err := models.NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.LoadModel(context.Background(), models.ModelTypeIntent)
	if err != nil {
		t.Fatal(err)
	}
	c.modelManager = manager
	cases := []struct {
		result interface{}
		want   IntentType
	}{{map[string]interface{}{"intent": "routine"}, IntentRoutine}, {map[string]interface{}{"intent": "learning"}, IntentLearning}, {map[string]interface{}{"intent": "followup"}, IntentFollowUp}, {map[string]interface{}{"intent": "summary"}, IntentQuickAnswer}, {map[string]interface{}{"intent": "explain"}, IntentDetailedAnalysis}, {map[string]interface{}{"intent": "unrecognized"}, IntentQuickAnswer}, {map[string]interface{}{"intent": 3}, IntentQuickAnswer}, {map[string]interface{}{}, IntentQuickAnswer}, {"bad shape", IntentQuickAnswer}, {nil, IntentQuickAnswer}}
	for i, tc := range cases {
		c.modelManager = fixtureIntentInference(func(context.Context, interface{}) (interface{}, error) { return tc.result, nil })
		got := c.Classify(context.Background(), fmt.Sprint("plain request", i), "user", nil)
		if got.Intent != tc.want || got.MaxTokens <= 0 {
			t.Errorf("result=%v decision=%+v", tc.result, got)
		}
	}
	c.modelManager = fixtureIntentInference(func(context.Context, interface{}) (interface{}, error) {
		return nil, fmt.Errorf("inference unavailable")
	})
	if got := c.Classify(context.Background(), "quick unavailable", "user", nil); got.Intent != IntentQuickAnswer {
		t.Fatal("ML fallback failed")
	}
	c.userFeedback["expert"] = &UserFeedbackPattern{RecentAccuracy: 1, PrefersOpus: true}
	c.modelManager = fixtureIntentInference(func(context.Context, interface{}) (interface{}, error) {
		return map[string]interface{}{"intent": "learning"}, nil
	})
	if got := c.Classify(context.Background(), "experiment", "expert", nil); got.RecommendedModel != ModelOpus {
		t.Fatal("expert model preference lost")
	}
}

type fixtureIntentInference func(context.Context, interface{}) (interface{}, error)

func (f fixtureIntentInference) Infer(ctx context.Context, _ models.ModelType, input interface{}) (interface{}, error) {
	return f(ctx, input)
}
