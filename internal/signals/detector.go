package signals

import (
	"regexp"
	"strings"
)

// SignalType categorizes user signals
type SignalType string

const (
	SignalSuccess       SignalType = "success"
	SignalFailure       SignalType = "failure"
	SignalEscalation    SignalType = "escalation"
	SignalClarification SignalType = "clarification"
	SignalEffortLow     SignalType = "effort_low"
	SignalEffortHigh    SignalType = "effort_high"
	SignalNone          SignalType = "none"
)

// Signal represents a detected user signal with confidence
type Signal struct {
	Type       SignalType
	Text       string
	Confidence float64 // 0.0-1.0
	Timing     float64 // seconds after response
	Pattern    string  // which pattern matched
}

// Detector analyzes prompts and responses for user signals
type Detector struct {
	successPatterns       []Pattern
	failurePatterns       []Pattern
	escalationPatterns    []Pattern
	clarificationPatterns []Pattern
	effortLowPatterns     []Pattern
	effortHighPatterns    []Pattern
}

// Pattern represents a detectable signal pattern
type Pattern struct {
	regex      *regexp.Regexp
	confidence float64
	name       string
}

// NewDetector creates a new signal detector
func NewDetector() *Detector {
	d := &Detector{
		successPatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\bperfect\b`), confidence: 0.95, name: "perfect"},
			{regex: regexp.MustCompile(`(?i)\bworks\s+(great|well|perfectly)\b`), confidence: 0.93, name: "works_great"},
			{regex: regexp.MustCompile(`(?i)\bthank\s+you\b`), confidence: 0.85, name: "thank_you"},
			{regex: regexp.MustCompile(`(?i)\bthanks\b`), confidence: 0.85, name: "thanks"},
			{regex: regexp.MustCompile(`(?i)\bthat\s+fixed\s+it\b`), confidence: 0.94, name: "fixed_it"},
			{regex: regexp.MustCompile(`(?i)\bsolve[d]?\b`), confidence: 0.90, name: "solved"},
			{regex: regexp.MustCompile(`(?i)\bexactly\b`), confidence: 0.88, name: "exactly"},
			{regex: regexp.MustCompile(`(?i)\bexcellent\b`), confidence: 0.92, name: "excellent"},
			{regex: regexp.MustCompile(`(?i)\bgreat\b`), confidence: 0.80, name: "great"},
			{regex: regexp.MustCompile(`(?i)\bappreciate\b`), confidence: 0.87, name: "appreciate"},
			{regex: regexp.MustCompile(`(?i)\bgot\s+it\b`), confidence: 0.82, name: "got_it"},
		},
		failurePatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\bdidn't\s+work\b`), confidence: 0.92, name: "didnt_work"},
			{regex: regexp.MustCompile(`(?i)\bstill\s+(broken|wrong|failing)\b`), confidence: 0.91, name: "still_broken"},
			{regex: regexp.MustCompile(`(?i)\bthat's\s+wrong\b`), confidence: 0.93, name: "thats_wrong"},
			{regex: regexp.MustCompile(`(?i)\bincorrect\b`), confidence: 0.94, name: "incorrect"},
			{regex: regexp.MustCompile(`(?i)\berror\b.*\berror\b`), confidence: 0.85, name: "double_error"},
			{regex: regexp.MustCompile(`(?i)\bgoing\s+in\s+circles\b`), confidence: 0.88, name: "circles"},
			{regex: regexp.MustCompile(`(?i)\btry\s+again\b`), confidence: 0.82, name: "try_again"},
			{regex: regexp.MustCompile(`(?i)\bdoesn't\s+(work|make|help)\b`), confidence: 0.87, name: "doesnt_work"},
			{regex: regexp.MustCompile(`(?i)\bdoesn't\s+solve\b`), confidence: 0.89, name: "doesnt_solve"},
			{regex: regexp.MustCompile(`(?i)\bincomplete\b`), confidence: 0.88, name: "incomplete"},
			{regex: regexp.MustCompile(`(?i)\bmissing\b`), confidence: 0.81, name: "missing"},
		},
		escalationPatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\/escalate\s+to\s+(opus|sonnet)\b`), confidence: 0.99, name: "explicit_escalate"},
			{regex: regexp.MustCompile(`(?i)\/escalate\b`), confidence: 0.98, name: "escalate_default"},
		},
		clarificationPatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\bcan\s+you\s+explain\b`), confidence: 0.90, name: "explain"},
			{regex: regexp.MustCompile(`(?i)\bwhy\s+did\b`), confidence: 0.88, name: "why"},
			{regex: regexp.MustCompile(`(?i)\bhow\s+does\b`), confidence: 0.87, name: "how"},
			{regex: regexp.MustCompile(`(?i)\bwhat\s+about\b`), confidence: 0.85, name: "what_about"},
		},
		effortLowPatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\bsimple\b`), confidence: 0.80, name: "simple"},
			{regex: regexp.MustCompile(`(?i)\bjust\s+(a\s+)?quick\b`), confidence: 0.85, name: "quick"},
			{regex: regexp.MustCompile(`(?i)\beasy\b`), confidence: 0.78, name: "easy"},
		},
		effortHighPatterns: []Pattern{
			{regex: regexp.MustCompile(`(?i)\bcomplex\b`), confidence: 0.88, name: "complex"},
			{regex: regexp.MustCompile(`(?i)\bdifficult\b`), confidence: 0.86, name: "difficult"},
			{regex: regexp.MustCompile(`(?i)\bmultiple\s+steps\b`), confidence: 0.84, name: "multiple_steps"},
			{regex: regexp.MustCompile(`(?i)\barchitecture\b`), confidence: 0.85, name: "architecture"},
		},
	}
	return d
}

// DetectSignal analyzes text for user signals
func (d *Detector) DetectSignal(text string) Signal {
	text = strings.TrimSpace(text)

	// Check escalation first (explicit command has highest priority)
	if sig := d.matchPatterns(text, d.escalationPatterns); sig.Type != SignalNone {
		return sig
	}

	// Check success (high confidence signals)
	if sig := d.matchPatterns(text, d.successPatterns); sig.Type != SignalNone {
		return sig
	}

	// Check failure
	if sig := d.matchPatterns(text, d.failurePatterns); sig.Type != SignalNone {
		return sig
	}

	// Check clarification
	if sig := d.matchPatterns(text, d.clarificationPatterns); sig.Type != SignalNone {
		return sig
	}

	// Check effort signals
	if sig := d.matchPatterns(text, d.effortHighPatterns); sig.Type != SignalNone {
		return sig
	}

	if sig := d.matchPatterns(text, d.effortLowPatterns); sig.Type != SignalNone {
		return sig
	}

	// No signal detected
	return Signal{
		Type:       SignalNone,
		Text:       text,
		Confidence: 0,
	}
}

// matchPatterns checks text against a slice of patterns
func (d *Detector) matchPatterns(text string, patterns []Pattern) Signal {
	// Find highest confidence match
	var bestSignal Signal
	bestSignal.Type = SignalNone
	bestSignal.Text = text
	bestSignal.Confidence = 0

	for _, p := range patterns {
		if p.regex.MatchString(text) {
			if p.confidence > bestSignal.Confidence {
				bestSignal.Confidence = p.confidence
				bestSignal.Pattern = p.name
				// Determine signal type from pattern group
				bestSignal.Type = d.patternToSignalType(patterns, p)
			}
		}
	}

	return bestSignal
}

// patternToSignalType maps pattern group to signal type
func (d *Detector) patternToSignalType(patterns []Pattern, p Pattern) SignalType {
	// Check which pattern group this pattern belongs to
	for _, sp := range d.successPatterns {
		if sp.name == p.name {
			return SignalSuccess
		}
	}
	for _, fp := range d.failurePatterns {
		if fp.name == p.name {
			return SignalFailure
		}
	}
	for _, ep := range d.escalationPatterns {
		if ep.name == p.name {
			return SignalEscalation
		}
	}
	for _, cp := range d.clarificationPatterns {
		if cp.name == p.name {
			return SignalClarification
		}
	}
	for _, eh := range d.effortHighPatterns {
		if eh.name == p.name {
			return SignalEffortHigh
		}
	}
	for _, el := range d.effortLowPatterns {
		if el.name == p.name {
			return SignalEffortLow
		}
	}
	return SignalNone
}

// AnalyzePromptAndResponse analyzes both prompt and response for signals
func (d *Detector) AnalyzePromptAndResponse(prompt, response string) Signal {
	// First check response (most recent user signal)
	if sig := d.DetectSignal(response); sig.Type != SignalNone {
		return sig
	}

	// Fall back to prompt analysis if no response signal
	return d.DetectSignal(prompt)
}
