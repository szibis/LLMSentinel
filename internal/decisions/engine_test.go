package decisions

import (
	"github.com/szibis/claude-escalate/internal/signals"
	"github.com/szibis/claude-escalate/internal/store"
	"testing"
)

func TestDecisionPrioritiesAndBoundaries(t *testing.T) {
	e := NewEngine()
	cases := []struct {
		model                string
		signal               signals.SignalType
		confidence, error    float64
		validated            bool
		action, next, effort string
	}{
		{"haiku", signals.SignalEscalation, .1, 0, false, "escalate", "opus", "low"},
		{"opus", signals.SignalSuccess, .8, 100, true, "cascade", "sonnet", "low"},
		{"sonnet", signals.SignalSuccess, .9, 0, false, "cascade", "haiku", "low"},
		{"haiku", signals.SignalSuccess, 1, 0, false, "stay", "haiku", "low"},
		{"haiku", signals.SignalFailure, .8, -100, true, "escalate", "sonnet", "medium"},
		{"sonnet", signals.SignalFailure, 1, 0, false, "escalate", "opus", "high"},
		{"opus", signals.SignalFailure, 1, 0, false, "stay", "opus", "low"},
		{"haiku", signals.SignalNone, 0, 15.1, true, "escalate", "sonnet", "medium"},
		{"sonnet", signals.SignalNone, 0, 16, true, "escalate", "opus", "high"},
		{"opus", signals.SignalNone, 0, -16, true, "cascade", "sonnet", "low"},
		{"sonnet", signals.SignalNone, 0, -16, true, "cascade", "haiku", "low"},
		{"haiku", signals.SignalNone, 0, 15, true, "stay", "haiku", "low"},
		{"sonnet", signals.SignalNone, 0, -15, true, "stay", "sonnet", "low"},
		{"haiku", signals.SignalSuccess, .79, 0, false, "", "haiku", "low"},
		{"haiku", signals.SignalEffortHigh, .7, 0, false, "adjust_effort", "sonnet", "high"},
		{"sonnet", signals.SignalEffortHigh, .7, 0, false, "adjust_effort", "opus", "high"},
		{"opus", signals.SignalEffortHigh, .7, 0, false, "adjust_effort", "opus", "high"},
		{"opus", signals.SignalEffortLow, .7, 0, false, "adjust_effort", "sonnet", "low"},
		{"sonnet", signals.SignalEffortLow, .7, 0, false, "adjust_effort", "haiku", "low"},
		{"haiku", signals.SignalEffortLow, .7, 0, false, "adjust_effort", "haiku", "low"},
	}
	for _, c := range cases {
		d := e.MakeDecision(store.ValidationMetric{RoutedModel: c.model, DetectedEffort: "low", TokenError: c.error, Validated: c.validated}, signals.Signal{Type: c.signal, Confidence: c.confidence})
		if d.Action != c.action || d.NextModel != c.next || d.NextEffort != c.effort {
			t.Errorf("%+v => %+v", c, d)
		}
		if d.CascadeAvailable != (d.Action == "cascade") || d.EscalateAvailable != (d.Action == "escalate") {
			t.Errorf("availability: %+v", d)
		}
	}
}

func TestLearningFromMixedDataset(t *testing.T) {
	e := NewEngine()
	data := []store.ValidationMetric{
		{DetectedEffort: "low", RoutedModel: "haiku", Validated: true, TokenError: 10},
		{DetectedEffort: "low", RoutedModel: "haiku", Validated: true, TokenError: -10},
		{DetectedEffort: "low", RoutedModel: "sonnet", Validated: true, TokenError: 30},
		{DetectedEffort: "high", RoutedModel: "opus", Validated: true, TokenError: 0},
	}
	l := e.CalculateLearning(data)
	low := l["low_effort"].(map[string]interface{})
	if low["count"] != 3 || low["avg_token_error"] != float64(10) || low["success_rate"] != 66.6 || low["best_model"] != "haiku" {
		t.Fatalf("low: %+v", low)
	}
	if l["medium_effort"].(map[string]interface{})["samples"] != "insufficient" {
		t.Fatal(l)
	}
	if l["high_effort"].(map[string]interface{})["success_rate"] != float64(100) {
		t.Fatal(l)
	}
}

func FuzzDecisionAvailability(f *testing.F) {
	f.Add("haiku", "success", .9, 20.0, true)
	f.Add("opus", "failure", 1.0, -20.0, false)
	f.Fuzz(func(t *testing.T, model, kind string, confidence, tokenError float64, validated bool) {
		d := NewEngine().MakeDecision(store.ValidationMetric{RoutedModel: model, DetectedEffort: "medium", TokenError: tokenError, Validated: validated}, signals.Signal{Type: signals.SignalType(kind), Confidence: confidence})
		if d.CascadeAvailable && d.EscalateAvailable {
			t.Fatalf("conflicting availability: %+v", d)
		}
		if d.CascadeAvailable && d.Action != "cascade" || d.EscalateAvailable && d.Action != "escalate" {
			t.Fatalf("inconsistent decision: %+v", d)
		}
		if d.Action == "escalate" && d.NextModel != "sonnet" && d.NextModel != "opus" {
			t.Fatalf("unsupported escalation: %+v", d)
		}
		if d.Action == "cascade" && d.NextModel != "haiku" && d.NextModel != "sonnet" {
			t.Fatalf("unsupported cascade: %+v", d)
		}
	})
}
