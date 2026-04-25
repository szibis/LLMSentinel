package sentiment

import (
	"regexp"
	"strings"
	"time"
)

// Sentiment represents user emotional state.
type Sentiment string

const (
	SentimentSatisfied  Sentiment = "satisfied"   // Success + happy
	SentimentFrustrated Sentiment = "frustrated"  // Failure + angry
	SentimentConfused   Sentiment = "confused"    // Questions, clarity needed
	SentimentImpatient  Sentiment = "impatient"   // Time pressure, repeated requests
	SentimentCautious   Sentiment = "cautious"    // Careful, slow approach
	SentimentNeutral    Sentiment = "neutral"     // No emotion signal
)

// Score represents sentiment analysis result.
type Score struct {
	Primary          Sentiment
	Confidence       float64 // 0.0-1.0
	FrustrationRisk  float64 // 0.0-1.0 (separate dimension)
	Sources          []string
	Details          string
	Timestamp        time.Time
}

// Detector analyzes prompts for sentiment signals.
type Detector struct {
	patterns map[Sentiment][]*regexp.Regexp
}

// NewDetector creates a sentiment detector.
func NewDetector() *Detector {
	d := &Detector{
		patterns: make(map[Sentiment][]*regexp.Regexp),
	}

	// Satisfaction signals
	d.patterns[SentimentSatisfied] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(perfect|great|excellent|thanks|thank you|appreciate|works|working|solved|fixed|correct|exactly|done|complete)\b`),
		regexp.MustCompile(`(?i)\b(that's it|spot on|bang on|nail it|right on|you got it)\b`),
		regexp.MustCompile(`✓|✅|👍|🎉|😊`),
	}

	// Frustration signals
	d.patterns[SentimentFrustrated] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(still broken|still failing|not working|doesn't work|broken|error|failed|fail|can't|won't|doesn't)\b`),
		regexp.MustCompile(`(?i)\b(again|retry|one more time|try again|please fix|still issue|still problem)\b`),
		regexp.MustCompile(`(?i)\b(frustrated|angry|annoyed|irritated|mad|upset|dammit|damn)\b`),
		regexp.MustCompile(`✗|❌|😤|😠|😡|💢`),
	}

	// Confusion signals
	d.patterns[SentimentConfused] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(why|confused|don't understand|not clear|unclear|confusing|how does|what is|explain)\b`),
		regexp.MustCompile(`(?i)\b(don't know|not sure|unclear|what do you mean|can you clarify|can you explain)\b`),
		regexp.MustCompile(`\?{2,}|🤔|😕|😐`),
	}

	// Impatience signals
	d.patterns[SentimentImpatient] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(hurry|fast|quick|ASAP|quickly|now|urgent|time|slow|wait)\b`),
		regexp.MustCompile(`(?i)\b(again|retry|one more time|please hurry|can't wait|hurry up)\b`),
		regexp.MustCompile(`⏰|⏱|⌛|🏃|💨|⚡`),
	}

	// Caution signals
	d.patterns[SentimentCautious] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(careful|carefully|slow|slowly|don't break|be careful|be cautious|step by step)\b`),
		regexp.MustCompile(`(?i)\b(important|critical|must not|should not|careful not)\b`),
	}

	return d
}

// Detect analyzes prompt for sentiment signals.
func (d *Detector) Detect(prompt string, isFollowUp bool, timeSinceLastPrompt time.Duration) Score {
	score := Score{
		Primary:   SentimentNeutral,
		Confidence: 0.5,
		Timestamp: time.Now(),
	}

	promptLower := strings.ToLower(prompt)

	// Check each sentiment pattern
	sentimentScores := make(map[Sentiment]float64)

	for sentiment, patternList := range d.patterns {
		for _, pattern := range patternList {
			if pattern.MatchString(prompt) {
				sentimentScores[sentiment] += 0.5  // Simple weight
				if sentiment == SentimentFrustrated || sentiment == SentimentConfused {
					score.FrustrationRisk += 0.25
				}
				score.Sources = append(score.Sources, "explicit_"+string(sentiment))
			}
		}
	}

	// Determine primary sentiment from explicit signals
	if len(sentimentScores) > 0 {
		maxScore := 0.0
		for sentiment, s := range sentimentScores {
			if s > maxScore {
				maxScore = s
				score.Primary = sentiment
				score.Confidence = 0.8
			}
		}
	}

	// Implicit signals from interaction patterns
	// Rapid follow-up suggests confusion or impatience
	if isFollowUp && timeSinceLastPrompt < 5*time.Second {
		score.FrustrationRisk += 0.2
		if score.Primary == SentimentNeutral {
			score.Primary = SentimentConfused
			score.Sources = append(score.Sources, "implicit_rapid_followup")
		}
	}

	// Multiple escalation requests suggest frustration
	if strings.Contains(promptLower, "/escalate") {
		score.FrustrationRisk += 0.15
		if score.Primary == SentimentNeutral {
			score.Primary = SentimentFrustrated
			score.Sources = append(score.Sources, "implicit_escalate_command")
		}
	}

	// Clamp frustration risk to [0, 1]
	if score.FrustrationRisk > 1.0 {
		score.FrustrationRisk = 1.0
	}

	return score
}

// IsSuccess checks if prompt contains success signals.
func (d *Detector) IsSuccess(prompt string) bool {
	for _, pattern := range d.patterns[SentimentSatisfied] {
		if pattern.MatchString(prompt) {
			return true
		}
	}
	return false
}

// IsFailure checks if prompt contains failure signals.
func (d *Detector) IsFailure(prompt string) bool {
	for _, pattern := range d.patterns[SentimentFrustrated] {
		if pattern.MatchString(prompt) {
			return true
		}
	}
	return false
}

// IsEscalateCommand checks for /escalate command.
func (d *Detector) IsEscalateCommand(prompt string) (bool, string) {
	escalatePattern := regexp.MustCompile(`^/escalate(?:\s+to\s+(\w+))?`)
	matches := escalatePattern.FindStringSubmatch(strings.TrimSpace(prompt))
	if len(matches) > 0 {
		target := "sonnet"  // default
		if len(matches) > 1 && matches[1] != "" {
			target = matches[1]
		}
		return true, target
	}
	return false, ""
}
