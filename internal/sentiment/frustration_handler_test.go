package sentiment

import (
	"strings"
	"testing"
)

func TestFrustrationAndRecoveryRouting(t *testing.T) {
	h := NewFrustrationHandler()
	for _, tc := range []struct {
		score        Score
		model        string
		attempt      int
		action, next string
	}{
		{Score{FrustrationRisk: .8}, "haiku", 0, "escalate", "sonnet"}, {Score{FrustrationRisk: .8}, "sonnet", 1, "escalate", "opus"}, {Score{FrustrationRisk: .8}, "haiku", 2, "escalate", "opus"},
		{Score{FrustrationRisk: .8}, "opus", 2, "", ""}, {Score{Primary: SentimentImpatient}, "sonnet", 0, "adjust_effort", "haiku"}, {Score{Primary: SentimentConfused}, "haiku", 0, "escalate", "sonnet"}, {Score{}, "sonnet", 0, "", ""},
	} {
		d := h.HandleFrustration(tc.score, tc.model, tc.attempt, "debug")
		if tc.action == "" {
			if d != nil {
				t.Fatal(d)
			}
			continue
		}
		if d == nil || d.Action != tc.action || d.NextModel != tc.next || d.Rationale == "" {
			t.Fatal(d, tc)
		}
		if !strings.Contains(d.String(), tc.next) {
			t.Fatal(d.String())
		}
		if tc.action == "adjust_effort" && !d.ShouldShowWarning() {
			t.Fatal("warning missing")
		}
	}
	for _, tc := range []struct {
		success bool
		score   Score
		model   string
		budget  float64
		next    string
	}{{false, Score{Primary: SentimentSatisfied}, "opus", .1, ""}, {true, Score{}, "opus", .1, ""}, {true, Score{Primary: SentimentSatisfied}, "haiku", .1, ""}, {true, Score{Primary: SentimentSatisfied}, "opus", .1, "sonnet"}, {true, Score{Primary: SentimentSatisfied}, "sonnet", .9, "haiku"}} {
		d := h.ShouldDeEscalate(tc.score, tc.success, tc.model, tc.budget)
		if tc.next == "" {
			if d != nil {
				t.Fatal(d)
			}
		} else if d == nil || d.NextModel != tc.next || d.Action != "de-escalate" || d.ShouldShowWarning() {
			t.Fatal(d)
		}
	}
	for _, m := range []string{"haiku", "sonnet", "opus", "unknown"} {
		if effortForModel(m) == "" || escalateByOne(m) == "" || deEscalateByOne(m) == "" {
			t.Fatal(m)
		}
	}
	var d *Decision
	if d.ShouldShowWarning() || d.String() != "No action needed" {
		t.Fatal("nil decision")
	}
	detector := NewDetector()
	if !detector.IsSuccess("perfect") || detector.IsSuccess("nothing") || !detector.IsFailure("still broken") || detector.IsFailure("nothing") {
		t.Fatal("success/failure classification")
	}
	for _, tc := range []struct {
		text, target string
		ok           bool
	}{{"/escalate to opus", "opus", true}, {"/escalate to sonnet", "sonnet", true}, {"/escalate to haiku", "haiku", true}, {"/escalate", "sonnet", true}, {"plain", "", false}} {
		ok, target := detector.IsEscalateCommand(tc.text)
		if ok != tc.ok || target != tc.target {
			t.Fatal(ok, target, tc)
		}
	}
}
