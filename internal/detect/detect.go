// Package detect implements frustration signal detection and circular reasoning analysis.
package detect

import (
	"regexp"
	"strings"
)

// Frustration patterns indicating the user is stuck.
var frustrationPatterns = []string{
	"that didn't work", "that doesn't work", "that didn't help",
	"still broken", "still doesn't", "still failing", "still getting",
	"try again", "try a different", "try something",
	"same error", "same problem", "same issue", "same result",
	"no progress", "stuck on", "going in circles",
	"not working", "doesn't work", "isn't working",
	"didn't fix", "didn't solve", "didn't help",
	"we already tried", "already tried that",
	"wrong approach", "different approach",
	"keeps happening", "keeps failing", "keeps breaking",
}

// Success patterns indicating the problem is solved.
var successPhrases = []string{
	"works great", "works perfectly", "working now", "got it working",
	"that fixed it", "that works", "that solved", "issue resolved",
	"problem solved", "thank you", "thanks for", "thanks a lot",
	"that's it", "that's exactly", "no longer broken", "no longer failing",
	"all good", "looks good", "ship it",
}

var successWords = []struct {
	pattern *regexp.Regexp
}{
	{regexp.MustCompile(`(?i)\bperfect\b`)},
	{regexp.MustCompile(`(?i)\bsolved\b`)},
	{regexp.MustCompile(`(?i)(that |it |is |got )fixed`)},
	{regexp.MustCompile(`(?i)(it|that|this) works`)},
}

var thanksButPattern = regexp.MustCompile(`(?i)thanks.*(but|however|although|yet|still)`)

// DetectFrustration checks if the prompt contains frustration signals.
func DetectFrustration(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, p := range frustrationPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// DetectSuccess checks if the prompt contains success signals.
func DetectSuccess(prompt string) bool {
	lower := strings.ToLower(prompt)

	// Check multi-word phrases first (most specific)
	for _, phrase := range successPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	// Check word patterns
	for _, sw := range successWords {
		if sw.pattern.MatchString(lower) {
			return true
		}
	}

	// Check "thanks" with guard against "thanks but..."
	if strings.Contains(lower, "thanks") && !thanksButPattern.MatchString(lower) {
		return true
	}

	return false
}

// IsMetaCommand checks if the prompt is a slash command that should bypass processing.
func IsMetaCommand(prompt string) bool {
	return strings.HasPrefix(prompt, "/escalate") ||
		strings.HasPrefix(prompt, "/effort") ||
		strings.HasPrefix(prompt, "/model")
}

// IsEscalateCommand checks if the prompt is an /escalate command and returns the target.
func IsEscalateCommand(prompt string) (bool, string) {
	if !strings.HasPrefix(strings.ToLower(prompt), "/escalate") {
		return false, ""
	}

	lower := strings.ToLower(prompt)
	targets := []string{"opus", "sonnet", "haiku"}
	for _, t := range targets {
		if strings.Contains(lower, t) {
			return true, t
		}
	}

	return true, "sonnet" // default
}

// ConceptKeywords for circular reasoning detection — domain-specific terms
// that indicate what problem domain the user is working in.
var conceptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(race|concurrent|thread|deadlock|atomic|mutex|lock|semaphore|goroutine|async|await|parallel|synchroniz)\b`),
	regexp.MustCompile(`(?i)\b(regex|parse|grammar|tokenize|lexer|ast|syntax)\b`),
	regexp.MustCompile(`(?i)\b(optimi[zs]|perform|speed|cache|memory|latency|throughput|profil)\b`),
	regexp.MustCompile(`(?i)\b(debug|error|bug|crash|exception|traceback|segfault|panic|undefined)\b`),
	regexp.MustCompile(`(?i)\b(architec|design|structur|pattern|microservice|monolith)\b`),
	regexp.MustCompile(`(?i)\b(crypto|security|encrypt|auth|token|tls|ssl|oauth|jwt)\b`),
	regexp.MustCompile(`(?i)\b(database|query|sql|index|migration|transaction|schema)\b`),
	regexp.MustCompile(`(?i)\b(network|socket|tcp|udp|http|dns|proxy|websocket)\b`),
	regexp.MustCompile(`(?i)\b(fail|broke|not.work|issue|problem|still|same|again|stuck)\b`),
}

// ExtractConcepts pulls domain keywords from a prompt for turn-level comparison.
func ExtractConcepts(prompt string) []string {
	seen := make(map[string]bool)
	var concepts []string

	for _, pat := range conceptPatterns {
		matches := pat.FindAllString(strings.ToLower(prompt), -1)
		for _, m := range matches {
			m = strings.ToLower(m)
			if !seen[m] {
				seen[m] = true
				concepts = append(concepts, m)
			}
			if len(concepts) >= 10 {
				return concepts
			}
		}
	}
	return concepts
}

// DetectCircularPattern checks if concepts repeat across recent turns.
// Returns true if 2+ concepts appear in 3+ of the recent turns.
func DetectCircularPattern(recentConcepts [][]string, minTurns int) bool {
	if len(recentConcepts) < minTurns {
		return false
	}

	// Count how many turns each concept appears in
	conceptTurnCount := make(map[string]int)
	for _, turnConcepts := range recentConcepts {
		seen := make(map[string]bool)
		for _, c := range turnConcepts {
			if !seen[c] {
				conceptTurnCount[c]++
				seen[c] = true
			}
		}
	}

	// Count concepts that appear in 3+ turns
	repeatedConcepts := 0
	for _, count := range conceptTurnCount {
		if count >= 3 {
			repeatedConcepts++
		}
	}

	return repeatedConcepts >= 2
}
