package intent

import (
	"regexp"
	"strings"
)

// BypassDetectionResult contains the result of bypass pattern detection
type BypassDetectionResult struct {
	IsBypass   bool    // True if bypass pattern detected
	Confidence float64 // 0.0-1.0 confidence in bypass detection
	Layer      string  // "explicit", "contextual", "paste"
	Reason     string  // Explanation of detection
}

// BypassDetector detects cache bypass patterns in user queries
type BypassDetector struct {
	// Explicit layer patterns (highest confidence)
	explicitPrefixes []string
	explicitSuffixes []string

	// Contextual layer patterns (medium confidence)
	contextualPatterns []*regexp.Regexp

	// Paste detection (prevents accidental bypass)
	codeBlockIndicators []*regexp.Regexp
}

// NewBypassDetector creates a new bypass detector
func NewBypassDetector() *BypassDetector {
	return &BypassDetector{
		// Explicit patterns: these have space after them, indicating user intent
		explicitPrefixes: []string{
			"--no-cache ",
			"--fresh ",
			"! ",
			"--bypass ",
		},
		explicitSuffixes: []string{
			" (no cache)",
			" (fresh)",
			" (bypass)",
			" --no-cache",
			" --fresh",
		},

		// Contextual patterns: natural language indicating bypass request
		contextualPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)bypass\s+cache`),   // "bypass cache"
			regexp.MustCompile(`(?i)skip\s+cache`),     // "skip cache"
			regexp.MustCompile(`(?i)fresh\s+response`), // "fresh response"
			regexp.MustCompile(`(?i)no\s+cache`),       // "no cache"
			regexp.MustCompile(`(?i)don't\s+cache`),    // "don't cache"
			regexp.MustCompile(`(?i)do\s+not\s+cache`), // "do not cache"
		},

		// Code block indicators: patterns that suggest pasted code
		codeBlockIndicators: []*regexp.Regexp{
			regexp.MustCompile("```"),                         // Markdown code block
			regexp.MustCompile(`^\s*func\s+`),                 // Go function
			regexp.MustCompile(`^\s*def\s+`),                  // Python function
			regexp.MustCompile(`^\s*function\s+`),             // JavaScript function
			regexp.MustCompile(`^\s*class\s+`),                // Class definition
			regexp.MustCompile("SELECT|INSERT|UPDATE|DELETE"), // SQL
			regexp.MustCompile("^#!"),                         // Shebang
			regexp.MustCompile("=>"),                          // JavaScript arrow function
		},
	}
}

// Detect checks if query contains cache bypass pattern
func (bd *BypassDetector) Detect(query string) *BypassDetectionResult {
	trimmed := strings.TrimSpace(query)

	// Layer 1: Check for explicit patterns (highest confidence: 0.99)
	result := bd.checkExplicitPatterns(trimmed)
	if result.IsBypass {
		return result
	}

	// Layer 2: Check for contextual patterns (medium confidence: 0.70-0.90)
	result = bd.checkContextualPatterns(trimmed)
	if result.IsBypass {
		return result
	}

	// Layer 3: Check for paste detection (lowest confidence, should NOT bypass)
	result = bd.checkPasteDetection(trimmed)
	if result.IsBypass {
		// Paste detection should reduce confidence significantly
		return result
	}

	// No bypass pattern detected
	return &BypassDetectionResult{
		IsBypass:   false,
		Confidence: 0.0,
		Layer:      "none",
		Reason:     "No bypass pattern detected",
	}
}

// checkExplicitPatterns checks for explicit bypass patterns
func (bd *BypassDetector) checkExplicitPatterns(query string) *BypassDetectionResult {
	// Check prefixes
	for _, prefix := range bd.explicitPrefixes {
		if strings.HasPrefix(query, prefix) {
			return &BypassDetectionResult{
				IsBypass:   true,
				Confidence: 0.99,
				Layer:      "explicit",
				Reason:     "Explicit bypass pattern detected at query start: " + prefix,
			}
		}
	}

	// Check suffixes
	for _, suffix := range bd.explicitSuffixes {
		if strings.HasSuffix(query, suffix) {
			return &BypassDetectionResult{
				IsBypass:   true,
				Confidence: 0.99,
				Layer:      "explicit",
				Reason:     "Explicit bypass pattern detected at query end: " + suffix,
			}
		}
	}

	return &BypassDetectionResult{
		IsBypass:   false,
		Confidence: 0.0,
	}
}

// checkContextualPatterns checks for contextual bypass indicators
func (bd *BypassDetector) checkContextualPatterns(query string) *BypassDetectionResult {
	matches := 0
	for _, pattern := range bd.contextualPatterns {
		if pattern.MatchString(query) {
			matches++
		}
	}

	if matches > 0 {
		// Require clear indication for contextual bypass
		confidence := float64(matches) * 0.20 // Each match contributes 20% confidence
		if confidence > 0.90 {
			confidence = 0.90 // Cap at 90% for contextual
		}

		// Check if query is primarily asking for bypass (not just mentioning it)
		if confidence >= 0.40 {
			return &BypassDetectionResult{
				IsBypass:   true,
				Confidence: confidence,
				Layer:      "contextual",
				Reason:     "Contextual cache bypass indicators detected",
			}
		}
	}

	return &BypassDetectionResult{
		IsBypass:   false,
		Confidence: 0.0,
	}
}

// checkPasteDetection checks if bypass pattern is embedded in pasted code
func (bd *BypassDetector) checkPasteDetection(query string) *BypassDetectionResult {
	// Check if query contains code block indicators
	codeIndicators := 0
	for _, pattern := range bd.codeBlockIndicators {
		if pattern.MatchString(query) {
			codeIndicators++
		}
	}

	// If code indicators found and query contains actual code-style flags
	if codeIndicators > 0 && strings.Contains(query, "--no-cache") {
		// This is likely pasted code with flags, not user intent to bypass cache
		return &BypassDetectionResult{
			IsBypass:   true,
			Confidence: 0.1, // Very low confidence - likely false positive
			Layer:      "paste",
			Reason:     "Cache bypass pattern embedded in code block (likely false positive)",
		}
	}

	return &BypassDetectionResult{
		IsBypass:   false,
		Confidence: 0.0,
	}
}

// ShouldBypass determines if cache should be bypassed based on confidence threshold
func (bd *BypassDetector) ShouldBypass(result *BypassDetectionResult, minConfidence float64) bool {
	if !result.IsBypass {
		return false
	}

	// Paste detection results should never trigger bypass
	if result.Layer == "paste" && result.Confidence < 0.5 {
		return false
	}

	return result.Confidence >= minConfidence
}

// ExtractQuery removes bypass patterns from query for actual processing
func ExtractQuery(query string) string {
	trimmed := strings.TrimSpace(query)

	// Remove explicit prefixes (repeatedly until all are gone)
	prefixes := []string{
		"--no-cache ",
		"--fresh ",
		"! ",
		"--bypass ",
	}

	changed := true
	for changed {
		changed = false
		for _, prefix := range prefixes {
			if strings.HasPrefix(trimmed, prefix) {
				trimmed = strings.TrimPrefix(trimmed, prefix)
				changed = true
				break
			}
		}
	}

	// Remove explicit suffixes (repeatedly until all are gone)
	suffixes := []string{
		" (no cache)",
		" (fresh)",
		" (bypass)",
		" --no-cache",
		" --fresh",
	}

	changed = true
	for changed {
		changed = false
		for _, suffix := range suffixes {
			if strings.HasSuffix(trimmed, suffix) {
				trimmed = strings.TrimSuffix(trimmed, suffix)
				changed = true
				break
			}
		}
	}

	return strings.TrimSpace(trimmed)
}
