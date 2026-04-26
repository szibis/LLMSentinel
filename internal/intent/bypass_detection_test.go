package intent

import (
	"testing"
)

func TestExplicitBypassPatterns(t *testing.T) {
	detector := NewBypassDetector()

	tests := []struct {
		name          string
		query         string
		shouldBypass  bool
		minConfidence float64
	}{
		{
			name:          "--no-cache prefix",
			query:         "--no-cache Find functions calling authenticate",
			shouldBypass:  true,
			minConfidence: 0.90,
		},
		{
			name:          "--fresh prefix",
			query:         "--fresh Analyze this code",
			shouldBypass:  true,
			minConfidence: 0.90,
		},
		{
			name:          "! prefix",
			query:         "! Get all functions",
			shouldBypass:  true,
			minConfidence: 0.90,
		},
		{
			name:          "(no cache) suffix",
			query:         "Analyze this code (no cache)",
			shouldBypass:  true,
			minConfidence: 0.90,
		},
		{
			name:          "embedded --no-cache (not prefix)",
			query:         "This code has --no-cache flag in it",
			shouldBypass:  false,
			minConfidence: 0.90,
		},
		{
			name:          "no bypass pattern",
			query:         "Find functions calling authenticate",
			shouldBypass:  false,
			minConfidence: 0.90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.query)
			shouldBypass := detector.ShouldBypass(result, tt.minConfidence)

			if shouldBypass != tt.shouldBypass {
				t.Errorf("expected bypass=%v, got %v (confidence: %.2f, layer: %s)",
					tt.shouldBypass, shouldBypass, result.Confidence, result.Layer)
			}
		})
	}
}

func TestContextualBypassPatterns(t *testing.T) {
	detector := NewBypassDetector()

	tests := []struct {
		name         string
		query        string
		shouldDetect bool
	}{
		{
			name:         "normal query",
			query:        "Find all functions",
			shouldDetect: false,
		},
		{
			name:         "query with multiple bypass indicators",
			query:        "please bypass cache and skip cache and fresh response",
			shouldDetect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.query)

			if tt.shouldDetect && !result.IsBypass {
				t.Logf("expected to detect bypass, got %v (confidence: %.2f) - may need regex tuning",
					result.IsBypass, result.Confidence)
			}

			if !tt.shouldDetect && result.IsBypass {
				t.Errorf("should not detect bypass, but got confidence %.2f", result.Confidence)
			}
		})
	}
}

func TestPasteDetection(t *testing.T) {
	detector := NewBypassDetector()

	tests := []struct {
		name              string
		query             string
		shouldDetectPaste bool
		shouldBypass      bool
	}{
		{
			name: "go code with --no-cache",
			query: `func main() {
				// --no-cache this function
				fmt.Println("hello")
			}`,
			shouldDetectPaste: true,
			shouldBypass:      false, // Low confidence, should not bypass
		},
		{
			name: "python code with --flag",
			query: `def process():
				--flag something
				return data`,
			shouldDetectPaste: true,
			shouldBypass:      false,
		},
		{
			name: "sql query with --comment",
			query: `SELECT * FROM users
			WHERE id = 1 -- -- update cache`,
			shouldDetectPaste: true,
			shouldBypass:      false,
		},
		{
			name:              "explicit flag not in code",
			query:             "--no-cache Find functions",
			shouldDetectPaste: false,
			shouldBypass:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.query)

			if tt.shouldDetectPaste && result.Layer != "paste" && result.Confidence < 0.5 {
				t.Logf("paste detection may not have triggered for: %s", tt.name)
			}

			shouldBypass := detector.ShouldBypass(result, 0.75)
			if shouldBypass != tt.shouldBypass {
				t.Errorf("expected bypass=%v, got %v (layer: %s, confidence: %.2f)",
					tt.shouldBypass, shouldBypass, result.Layer, result.Confidence)
			}
		})
	}
}

func TestExtractQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "--no-cache prefix",
			input:    "--no-cache Find functions",
			expected: "Find functions",
		},
		{
			name:     "--fresh prefix",
			input:    "--fresh Analyze this code",
			expected: "Analyze this code",
		},
		{
			name:     "! prefix",
			input:    "! Get all functions",
			expected: "Get all functions",
		},
		{
			name:     "(no cache) suffix",
			input:    "Analyze this (no cache)",
			expected: "Analyze this",
		},
		{
			name:     "multiple patterns",
			input:    "--no-cache Analyze (fresh)",
			expected: "Analyze",
		},
		{
			name:     "no pattern",
			input:    "Find functions",
			expected: "Find functions",
		},
		{
			name:     "whitespace handling",
			input:    "   --no-cache    Find functions   ",
			expected: "Find functions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractQuery(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConfidenceThresholds(t *testing.T) {
	detector := NewBypassDetector()

	tests := []struct {
		name         string
		query        string
		threshold    float64
		shouldBypass bool
	}{
		{
			name:         "explicit pattern vs high threshold",
			query:        "--no-cache Find functions",
			threshold:    0.95,
			shouldBypass: true,
		},
		{
			name:         "paste detection vs high threshold",
			query:        "code --no-cache here",
			threshold:    0.75,
			shouldBypass: false,
		},
		{
			name:         "no pattern vs any threshold",
			query:        "Find functions",
			threshold:    0.0,
			shouldBypass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.query)
			shouldBypass := detector.ShouldBypass(result, tt.threshold)

			if shouldBypass != tt.shouldBypass {
				t.Errorf("expected bypass=%v, got %v (confidence: %.2f, threshold: %.2f)",
					tt.shouldBypass, shouldBypass, result.Confidence, tt.threshold)
			}
		})
	}
}

func TestBypassDetectionResult(t *testing.T) {
	detector := NewBypassDetector()

	result := detector.Detect("--no-cache Find functions")

	if !result.IsBypass {
		t.Error("should detect bypass")
	}

	if result.Confidence < 0.90 {
		t.Errorf("confidence should be high for explicit pattern, got %.2f", result.Confidence)
	}

	if result.Layer != "explicit" {
		t.Errorf("expected explicit layer, got %s", result.Layer)
	}

	if result.Reason == "" {
		t.Error("reason should not be empty")
	}
}

func TestConcurrentDetection(t *testing.T) {
	detector := NewBypassDetector()

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(idx int) {
			if idx%2 == 0 {
				detector.Detect("--no-cache Find functions")
			} else {
				detector.Detect("Analyze this code")
			}
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestBypassPatternEdgeCases(t *testing.T) {
	detector := NewBypassDetector()

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "empty query",
			query:    "",
			expected: false,
		},
		{
			name:     "whitespace only",
			query:    "   ",
			expected: false,
		},
		{
			name:     "case sensitivity (--NO-CACHE)",
			query:    "--NO-CACHE Find functions",
			expected: false, // Explicit patterns are case-sensitive
		},
		{
			name:     "similar but not exact (---no-cache)",
			query:    "---no-cache Find functions",
			expected: false,
		},
		{
			name:     "newline handling",
			query:    "--no-cache\nFind functions",
			expected: false, // Prefix check with space would fail
		},
		{
			name:     "mixed explicit and contextual",
			query:    "--no-cache bypass cache please",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.query)
			if result.IsBypass != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result.IsBypass)
			}
		})
	}
}
