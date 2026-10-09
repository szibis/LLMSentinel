package jes

import (
	"context"
	"math"
	"testing"
)

func knownSample() Sample {
	return Sample{ID: "case", CorpusVersion: 2, CorpusSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ModelRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceRevision: "cccccccccccccccccccccccccccccccccccccccc", EvidenceSHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", ProtocolValid: true, Grounded: true, AssertionPassed: true, Model: "local-large", Role: "sonnet", ChecksComplete: true}
}

func TestGateRequiresMeasuredJudgmentAndExplicitTrainingApproval(t *testing.T) {
	sample := knownSample()
	policy := Policy{MinimumScore: 1, CaptureOptIn: true, HumanApproved: true, AllowTraining: true}
	outcome, err := Evaluate(context.Background(), EvidenceJudge{}, sample, policy)
	if err != nil || outcome.Score == nil || *outcome.Score != 1 || !outcome.TrainingEligible || outcome.Recommendation != "retain-current-route" {
		t.Fatalf("valid gate: %+v %v", outcome, err)
	}
	for _, kind := range []string{"unapproved", "capture-off", "disabled", "unknown-provenance", "unknown-score", "failed-protocol", "synthetic", "unknown-model", "failed-assertion"} {
		s, p := sample, policy
		switch kind {
		case "unapproved":
			p.HumanApproved = false
		case "capture-off":
			p.CaptureOptIn = false
		case "disabled":
			p.AllowTraining = false
		case "unknown-provenance":
			s.EvidenceSHA256 = ""
		case "unknown-score":
			s.ChecksComplete = false
		case "failed-protocol":
			s.ProtocolValid = false
		case "synthetic":
			s.Synthetic = true
		case "unknown-model":
			s.ModelRevision = ""
		case "failed-assertion":
			s.AssertionPassed = false
		}
		result, err := Evaluate(context.Background(), EvidenceJudge{}, s, p)
		if err != nil || result.TrainingEligible || len(result.Reasons) == 0 {
			t.Fatalf("%s unexpectedly admitted: %+v %v", kind, result, err)
		}
	}
}

type fixedJudge struct {
	judgment Judgment
	err      error
}

func (f fixedJudge) Judge(context.Context, Sample) (Judgment, error) { return f.judgment, f.err }
func TestMalformedJudgmentsAndPoliciesFailClosed(t *testing.T) {
	sample := knownSample()
	for _, score := range []float64{-1, 1.01, math.NaN(), math.Inf(1)} {
		_, err := Evaluate(context.Background(), fixedJudge{judgment: Judgment{Score: &score, Backend: "fake", Scope: "fixture-only"}}, sample, Policy{MinimumScore: 1})
		if err == nil {
			t.Fatal("invalid score", score)
		}
	}
	for _, minimum := range []float64{-1, 0, 1.01, math.NaN(), math.Inf(1)} {
		if _, err := Evaluate(context.Background(), EvidenceJudge{}, sample, Policy{MinimumScore: minimum}); err == nil {
			t.Fatal("invalid policy", minimum)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Evaluate(ctx, EvidenceJudge{}, sample, Policy{MinimumScore: 1}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestComparisonsPreserveUnknownAndCorpusBoundaries(t *testing.T) {
	one, zero := 1.0, 0.0
	left := Outcome{SampleID: "a", CorpusVersion: 2, CorpusSHA256: "same", Score: &one}
	right := left
	right.Score = &zero
	if c := Compare(left, right); c.Delta == nil || *c.Delta != 1 || c.Recommendation != "prefer-left-on-this-fixture" {
		t.Fatal(c)
	}
	for _, kind := range []string{"unknown", "different-corpus", "different-version"} {
		r := right
		switch kind {
		case "unknown":
			r.Score = nil
		case "different-corpus":
			r.CorpusSHA256 = "different"
		case "different-version":
			r.CorpusVersion = 3
		}
		if c := Compare(left, r); c.Delta != nil || c.Recommendation != "insufficient-evidence" {
			t.Fatal(kind, c)
		}
	}
}
