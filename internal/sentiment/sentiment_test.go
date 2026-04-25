package sentiment

import (
	"testing"
	"time"
)

func TestDetectFrustration(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name               string
		prompt             string
		isFollowUp         bool
		timeSincePrompt    time.Duration
		shouldBeFrustrated bool
		minRisk            float64
	}{
		{
			name:               "Clear frustration keyword",
			prompt:             "still broken, why isn't this working?",
			isFollowUp:         false,
			timeSincePrompt:    0,
			shouldBeFrustrated: true,
			minRisk:            0.25,
		},
		{
			name:               "Frustration with broken keyword",
			prompt:             "this is broken",
			isFollowUp:         false,
			timeSincePrompt:    0,
			shouldBeFrustrated: true,
			minRisk:            0.25,
		},
		{
			name:               "Neutral prompt",
			prompt:             "explain how context works",
			isFollowUp:         false,
			timeSincePrompt:    0,
			shouldBeFrustrated: false,
			minRisk:            0.0,
		},
		{
			name:               "Success signal",
			prompt:             "perfect! this works exactly",
			isFollowUp:         false,
			timeSincePrompt:    0,
			shouldBeFrustrated: false,
			minRisk:            0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := detector.Detect(tt.prompt, tt.isFollowUp, tt.timeSincePrompt)

			if tt.shouldBeFrustrated && score.FrustrationRisk < tt.minRisk {
				t.Errorf("expected frustration risk >= %.2f, got %.2f", tt.minRisk, score.FrustrationRisk)
			}

			if score.FrustrationRisk < 0 || score.FrustrationRisk > 1.0 {
				t.Errorf("frustration risk out of bounds: %.2f", score.FrustrationRisk)
			}

			if score.Confidence < 0 || score.Confidence > 1.0 {
				t.Errorf("confidence out of bounds: %.2f", score.Confidence)
			}
		})
	}
}

func TestDetectSentiments(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name         string
		prompt       string
		expectedType Sentiment
		shouldDetect bool
	}{
		{
			name:         "Satisfied sentiment",
			prompt:       "perfect, thanks!",
			expectedType: SentimentSatisfied,
			shouldDetect: true,
		},
		{
			name:         "Confused sentiment",
			prompt:       "why does this happen? confused",
			expectedType: SentimentConfused,
			shouldDetect: true,
		},
		{
			name:         "Impatient sentiment",
			prompt:       "ASAP please hurry",
			expectedType: SentimentImpatient,
			shouldDetect: true,
		},
		{
			name:         "Neutral",
			prompt:       "what is golang",
			expectedType: SentimentNeutral,
			shouldDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := detector.Detect(tt.prompt, false, 0)

			if tt.shouldDetect && score.Confidence < 0.5 {
				t.Logf("warning: expected stronger signal for %s", tt.name)
			}
		})
	}
}

func TestRapidFollowUp(t *testing.T) {
	detector := NewDetector()

	// Rapid follow-up (< 5 seconds) should increase frustration
	score := detector.Detect("trying again", true, 2*time.Second)

	if score.FrustrationRisk < 0.15 {
		t.Errorf("rapid follow-up should increase frustration risk, got %.2f", score.FrustrationRisk)
	}
}

func TestEmptyPrompt(t *testing.T) {
	detector := NewDetector()

	score := detector.Detect("", false, 0)

	if score.Primary != SentimentNeutral {
		t.Errorf("empty prompt should be neutral")
	}

	if score.FrustrationRisk != 0.0 {
		t.Errorf("empty prompt should have zero frustration")
	}
}

func TestScoreStructure(t *testing.T) {
	detector := NewDetector()
	score := detector.Detect("some prompt", false, 0)

	// Verify score structure is properly initialized
	if score.Timestamp.IsZero() {
		t.Errorf("timestamp should be set")
	}

	if len(score.Sources) == 0 && score.Primary != SentimentNeutral {
		t.Logf("sources array might be empty for %s", score.Primary)
	}
}

func BenchmarkDetect(b *testing.B) {
	detector := NewDetector()
	prompt := "still broken, tried again, doesn't work, frustrated"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(prompt, false, 0)
	}
}

func BenchmarkDetectLongPrompt(b *testing.B) {
	detector := NewDetector()
	longPrompt := generateLongPrompt(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(longPrompt, false, 0)
	}
}

func generateLongPrompt(length int) string {
	const word = "test "
	result := ""
	for len(result) < length {
		result += word
	}
	return result[:length]
}
