// Package jes defines an advisory quality policy and the seam for a future
// trained judge. Its bootstrap judge scores independent fixture assertions;
// it does not claim general semantic correctness or commercial-model parity.
package jes

import (
	"context"
	"errors"
	"math"
	"regexp"
)

type Sample struct {
	ID              string `json:"id"`
	CorpusVersion   int    `json:"corpus_version"`
	CorpusSHA256    string `json:"corpus_sha256"`
	SourceRevision  string `json:"source_revision"`
	ModelRevision   string `json:"model_revision"`
	EvidenceSHA256  string `json:"evidence_sha256"`
	Model           string `json:"model"`
	Role            string `json:"role"`
	ProtocolValid   bool   `json:"protocol_valid"`
	Grounded        bool   `json:"grounded"`
	AssertionPassed bool   `json:"assertion_passed"`
	ChecksComplete  bool   `json:"checks_complete"`
	Synthetic       bool   `json:"synthetic"`
}
type Policy struct {
	MinimumScore  float64 `json:"minimum_score"`
	CaptureOptIn  bool    `json:"capture_opt_in"`
	HumanApproved bool    `json:"human_approved"`
	AllowTraining bool    `json:"allow_training"`
}
type Judgment struct {
	Score   *float64 `json:"score"`
	Backend string   `json:"backend"`
	Scope   string   `json:"scope"`
}
type Judge interface {
	Judge(context.Context, Sample) (Judgment, error)
}
type EvidenceJudge struct{}

func (EvidenceJudge) Judge(ctx context.Context, s Sample) (Judgment, error) {
	if err := ctx.Err(); err != nil {
		return Judgment{}, err
	}
	judgment := Judgment{Backend: "independent-fixture-assertions-v1", Scope: "fixture-only"}
	if s.ChecksComplete {
		score := 0.0
		if s.ProtocolValid && s.Grounded && s.AssertionPassed {
			score = 1
		}
		judgment.Score = &score
	}
	return judgment, nil
}

type Outcome struct {
	Sample           *Sample  `json:"sample"`
	Policy           *Policy  `json:"policy"`
	Version          int      `json:"version"`
	SampleID         string   `json:"sample_id"`
	CorpusVersion    int      `json:"corpus_version"`
	CorpusSHA256     string   `json:"corpus_sha256"`
	Model            string   `json:"model"`
	Role             string   `json:"role"`
	Score            *float64 `json:"score"`
	Backend          string   `json:"backend"`
	Scope            string   `json:"scope"`
	Recommendation   string   `json:"recommendation"`
	TrainingEligible bool     `json:"training_eligible"`
	Reasons          []string `json:"reasons"`
}

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

func validScore(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func Evaluate(ctx context.Context, judge Judge, s Sample, p Policy) (Outcome, error) {
	out := Outcome{Sample: &s, Policy: &p, Version: 1, SampleID: s.ID, CorpusVersion: s.CorpusVersion, CorpusSHA256: s.CorpusSHA256, Model: s.Model, Role: s.Role, Recommendation: "insufficient-evidence", Reasons: []string{}}
	if !validScore(p.MinimumScore) || p.MinimumScore == 0 || judge == nil {
		return out, errors.New("invalid quality policy or judge")
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	j, err := judge.Judge(ctx, s)
	if err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if j.Backend == "" || j.Scope != "fixture-only" || (j.Score != nil && !validScore(*j.Score)) {
		return out, errors.New("malformed or unsupported judgment")
	}
	if j.Score != nil {
		score := *j.Score
		out.Score = &score
	}
	out.Backend = j.Backend
	out.Scope = j.Scope
	reason := func(ok bool, message string) {
		if !ok {
			out.Reasons = append(out.Reasons, message)
		}
	}
	reason(out.Score != nil, "score unknown")
	reason(s.ChecksComplete, "independent checks incomplete")
	reason(s.ProtocolValid, "protocol rejected")
	reason(s.Grounded, "executed evidence missing")
	reason(s.AssertionPassed, "fixture assertion failed")
	measured := out.Score != nil && *out.Score >= p.MinimumScore && s.ChecksComplete && s.ProtocolValid && s.Grounded && s.AssertionPassed
	if out.Score != nil && *out.Score < p.MinimumScore {
		out.Reasons = append(out.Reasons, "score below threshold")
	}
	if measured {
		out.Recommendation = "retain-current-route"
	} else if out.Score != nil {
		out.Recommendation = "review-or-escalate"
	}
	reason(s.ID != "" && s.CorpusVersion > 0 && sha256Pattern.MatchString(s.CorpusSHA256) && sha256Pattern.MatchString(s.EvidenceSHA256) && revisionPattern.MatchString(s.SourceRevision), "evidence provenance incomplete")
	reason(s.Model != "" && revisionPattern.MatchString(s.ModelRevision), "model provenance incomplete")
	reason(!s.Synthetic, "synthetic CI evidence is not training admission")
	reason(p.CaptureOptIn, "capture not opted in")
	reason(p.AllowTraining, "training admission disabled")
	reason(p.HumanApproved, "human approval missing")
	out.TrainingEligible = measured && len(out.Reasons) == 0
	return out, nil
}

type Comparison struct {
	Delta          *float64 `json:"delta"`
	Recommendation string   `json:"recommendation"`
}

func Compare(left, right Outcome) Comparison {
	result := Comparison{Recommendation: "insufficient-evidence"}
	if left.Score == nil || right.Score == nil || !validScore(*left.Score) || !validScore(*right.Score) || left.SampleID != right.SampleID || left.CorpusVersion != right.CorpusVersion || left.CorpusSHA256 == "" || left.CorpusSHA256 != right.CorpusSHA256 {
		return result
	}
	delta := *left.Score - *right.Score
	result.Delta = &delta
	result.Recommendation = "equal-on-this-fixture"
	if delta > 0 {
		result.Recommendation = "prefer-left-on-this-fixture"
	} else if delta < 0 {
		result.Recommendation = "prefer-right-on-this-fixture"
	}
	return result
}
