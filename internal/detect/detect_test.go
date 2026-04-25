package detect

import "testing"

func TestDetectFrustration(t *testing.T) {
	tests := []struct {
		prompt   string
		expected bool
	}{
		{"that didn't work, same error as before", true},
		{"still broken, going in circles", true},
		{"try again with a different approach", true},
		{"keeps failing on the same step", true},
		{"How do I parse JSON?", false},
		{"implement the feature please", false},
		{"I think it needs more optimization", false},
	}
	for _, tt := range tests {
		got := DetectFrustration(tt.prompt)
		if got != tt.expected {
			t.Errorf("DetectFrustration(%q) = %v, want %v", tt.prompt, got, tt.expected)
		}
	}
}

func TestDetectSuccess(t *testing.T) {
	tests := []struct {
		prompt   string
		expected bool
	}{
		{"Perfect! That solution works great.", true},
		{"Thanks, that fixed it!", true},
		{"That works perfectly, appreciate the help.", true},
		{"Got it working now!", true},
		{"The error still happens sometimes", false},
		{"How does this function works?", false}, // false positive guard
		{"Thanks but it still doesn't compile", false},
	}
	for _, tt := range tests {
		got := DetectSuccess(tt.prompt)
		if got != tt.expected {
			t.Errorf("DetectSuccess(%q) = %v, want %v", tt.prompt, got, tt.expected)
		}
	}
}

func TestIsEscalateCommand(t *testing.T) {
	tests := []struct {
		prompt     string
		isEscalate bool
		target     string
	}{
		{"/escalate to sonnet", true, "sonnet"},
		{"/escalate to opus", true, "opus"},
		{"/escalate to haiku", true, "haiku"},
		{"/escalate", true, "sonnet"},
		{"fix this bug", false, ""},
	}
	for _, tt := range tests {
		isEsc, target := IsEscalateCommand(tt.prompt)
		if isEsc != tt.isEscalate || target != tt.target {
			t.Errorf("IsEscalateCommand(%q) = (%v, %q), want (%v, %q)",
				tt.prompt, isEsc, target, tt.isEscalate, tt.target)
		}
	}
}

func TestExtractConcepts(t *testing.T) {
	concepts := ExtractConcepts("Fix the race condition in concurrent thread-safe code")
	if len(concepts) == 0 {
		t.Fatal("expected concepts, got none")
	}
	found := false
	for _, c := range concepts {
		if c == "race" || c == "concurrent" || c == "thread" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected concurrency concepts, got %v", concepts)
	}
}

func TestDetectCircularPattern(t *testing.T) {
	// 4 turns with "error" and "debug" repeating
	turns := [][]string{
		{"error", "debug", "crash"},
		{"error", "fix", "debug"},
		{"error", "stack", "debug"},
		{"error", "debug", "trace"},
	}
	if !DetectCircularPattern(turns, 4) {
		t.Error("expected circular pattern detected")
	}

	// Not enough turns
	if DetectCircularPattern(turns[:2], 4) {
		t.Error("should not detect pattern with < 4 turns")
	}

	// No repetition
	diverse := [][]string{
		{"error", "crash"},
		{"network", "socket"},
		{"database", "query"},
		{"deploy", "docker"},
	}
	if DetectCircularPattern(diverse, 4) {
		t.Error("should not detect pattern with diverse concepts")
	}
}
