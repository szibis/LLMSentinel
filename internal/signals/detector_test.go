package signals

import "testing"

func TestSignalPriorityAndFallback(t *testing.T) {
	d := NewDetector()
	for _, tc := range []struct {
		text    string
		typ     SignalType
		pattern string
	}{
		{" perfect /escalate to opus ", SignalEscalation, "explicit_escalate"},
		{"thanks, perfect", SignalSuccess, "perfect"}, {"still broken", SignalFailure, "still_broken"},
		{"can you explain", SignalClarification, "explain"}, {"complex but simple", SignalEffortHigh, "complex"},
		{"just a quick task", SignalEffortLow, "quick"}, {"unrelated", SignalNone, ""},
	} {
		s := d.DetectSignal(tc.text)
		if s.Type != tc.typ || s.Pattern != tc.pattern {
			t.Fatalf("%q: %+v", tc.text, s)
		}
		if s.Confidence < 0 || s.Confidence > 1 {
			t.Fatal(s)
		}
	}
	if d.AnalyzePromptAndResponse("perfect", "incorrect").Type != SignalFailure {
		t.Fatal("response must take priority")
	}
	if d.AnalyzePromptAndResponse("perfect", "unrelated").Type != SignalSuccess {
		t.Fatal("prompt fallback lost")
	}
	if d.patternToSignalType(Pattern{name: "unknown"}) != SignalNone {
		t.Fatal("unknown pattern")
	}
}

func FuzzDetectSignalDeterministic(f *testing.F) {
	for _, s := range []string{"", "perfect /escalate", "still broken", "\x00é"} {
		f.Add(s)
	}
	d := NewDetector()
	f.Fuzz(func(t *testing.T, s string) {
		a, b := d.DetectSignal(s), d.DetectSignal(s)
		if a != b || a.Confidence < 0 || a.Confidence > 1 {
			t.Fatalf("invalid signal: %+v %+v", a, b)
		}
	})
}
