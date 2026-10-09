package jes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/szibis/claude-escalate/internal/taskquality"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type brokenIO struct{}

func (brokenIO) Read([]byte) (int, error)  { return 0, errors.New("read failed") }
func (brokenIO) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestQualityBoundariesAndUnknownPreservation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := knownSample()
	p := Policy{MinimumScore: 1, CaptureOptIn: true, AllowTraining: true, HumanApproved: true}
	raw, _ := json.Marshal(Request{Version: 1, Samples: []Sample{s}, Policy: p})
	for _, input := range []io.Reader{nil, brokenIO{}, bytes.NewReader(bytes.Repeat([]byte(" "), (1<<20)+1))} {
		if Run(nil, input, io.Discard, io.Discard) != 1 {
			t.Fatal("bad reader")
		}
	}
	if Run(nil, bytes.NewReader(raw), brokenIO{}, io.Discard) != 1 {
		t.Fatal("writer failure")
	}
	if Run([]string{"positional"}, bytes.NewReader(raw), io.Discard, io.Discard) != 2 {
		t.Fatal("extra argument")
	}
	if Run([]string{"--output", filepath.Join(root, "missing", "file")}, bytes.NewReader(raw), io.Discard, io.Discard) != 1 {
		t.Fatal("bad output")
	}
	if Run([]string{"--output", root}, bytes.NewReader(raw), io.Discard, io.Discard) != 1 {
		t.Fatal("directory output")
	}
	if Run(nil, bytes.NewReader([]byte(`{"version":1,"samples":[{"id":"x"}],"policy":{"minimum_score":0}}`)), io.Discard, io.Discard) != 1 {
		t.Fatal("invalid policy")
	}
	for _, raw := range []string{`{"version":1,"samples":[{"id":"x"},{"id":"x"}],"policy":{"minimum_score":1}}`, `{"version":1,"samples":[{"id":""}],"policy":{"minimum_score":1}}`, `{"version":1,"samples":[{"id":"x","unknown":true}],"policy":{"minimum_score":1}}`} {
		if _, err := decodeRequest([]byte(raw)); err == nil {
			t.Fatal("invalid samples")
		}
	}
	if report := Load(root); report != nil {
		t.Fatal("missing report")
	}
	path := filepath.Join(root, reportName)
	if code := Run([]string{"--output", path}, bytes.NewReader(raw), io.Discard, io.Discard); code != 0 {
		t.Fatal(code)
	}
	valid, _ := os.ReadFile(path)
	var report Report
	_ = json.Unmarshal(valid, &report)
	if report.TrainingEligible != 1 {
		t.Fatal(report)
	}
	for _, change := range []func(*Report){func(r *Report) { r.Outcomes[0].Version = 2 }, func(r *Report) { r.Outcomes[0].Scope = "semantic" }, func(r *Report) { r.Outcomes[0].Score = new(float64); *r.Outcomes[0].Score = 3 }, func(r *Report) { r.TrainingEligible = 99 }, func(r *Report) { r.Outcomes = nil }} {
		_ = json.Unmarshal(valid, &report)
		change(&report)
		bad, _ := json.Marshal(report)
		_ = os.WriteFile(path, bad, 0600)
		if Load(root) != nil {
			t.Fatal("invalid report exposed")
		}
	}
	_ = os.Remove(path)
	_ = os.Symlink(filepath.Join(root, "missing"), path)
	if Load(root) != nil {
		t.Fatal("symlink loaded")
	}
	if _, err := Evaluate(context.Background(), fixedJudge{err: errors.New("backend failed")}, s, p); err == nil {
		t.Fatal("backend error")
	}
	if _, err := Evaluate(context.Background(), fixedJudge{judgment: Judgment{Backend: "missing-scope"}}, s, p); err == nil {
		t.Fatal("missing scope")
	}
	one := 1.0
	zero := 0.0
	left := Outcome{SampleID: "same", CorpusVersion: 2, CorpusSHA256: "corpus", Score: &zero}
	right := left
	right.Score = &one
	if c := Compare(left, right); c.Recommendation != "prefer-right-on-this-fixture" {
		t.Fatal(c)
	}
	if c := Compare(left, left); c.Recommendation != "equal-on-this-fixture" {
		t.Fatal(c)
	}
}

func FuzzTrainingAdmissionFailsClosed(f *testing.F) {
	f.Add(false, false, false, false, 1.0)
	f.Add(true, true, true, true, 0.5)
	f.Fuzz(func(t *testing.T, capture, approval, allow, complete bool, minimum float64) {
		s := knownSample()
		s.ChecksComplete = complete
		result, err := Evaluate(context.Background(), EvidenceJudge{}, s, Policy{MinimumScore: minimum, CaptureOptIn: capture, HumanApproved: approval, AllowTraining: allow})
		if err == nil && result.TrainingEligible && (!capture || !approval || !allow || !complete || result.Score == nil || *result.Score < minimum) {
			t.Fatal("unapproved admission")
		}
	})
}

func TestTaskReportScoresOnlyKnownChecksAndNeverAdmitsSynthetic(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "task.json")
	source := strings.Repeat("a", 40)
	report := taskquality.Report{Version: 1, FixtureVersion: 2, CorpusSHA256: strings.Repeat("b", 64), Scope: "real-cli-task-probes", FinishedAt: time.Now().UTC(), Results: []taskquality.Result{
		{Task: "literal-markers", Role: "sonnet", Protocol: "codex-cli", Client: "codex", Checks: map[string]bool{"fixture_read": true, "protocol_valid": true, "final_assertion": false}},
		{Task: "coding-fix", Role: "opus", Protocol: "claude-cli", Client: "claude", Passed: true, Checks: map[string]bool{"fixture_read": true, "protocol_valid": true, "final_assertion": true, "independent_tests": true, "edit_and_tests_preserved": true}},
		{Task: "unknown", Role: "haiku", Protocol: "messages"},
	}}
	raw, _ := json.Marshal(report)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, reportName)
	if code := Run([]string{"--task-report", path, "--source-revision", source, "--output", output}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatal(code)
	}
	scored := Load(root)
	if scored == nil || scored.TrainingEligible != 0 || *scored.Outcomes[0].Score != 0 || *scored.Outcomes[1].Score != 1 || scored.Outcomes[2].Score != nil {
		t.Fatal(scored)
	}
	for _, outcome := range scored.Outcomes {
		if outcome.TrainingEligible {
			t.Fatal("synthetic admission", outcome)
		}
	}
	if Run([]string{"--task-report", filepath.Join(root, "missing")}, nil, io.Discard, io.Discard) != 1 {
		t.Fatal("missing report")
	}
	for _, bad := range []string{"{}", "not json", strings.Repeat(" ", (1<<20)+1)} {
		_ = os.WriteFile(path, []byte(bad), 0600)
		if Run([]string{"--task-report", path}, nil, io.Discard, io.Discard) != 1 {
			t.Fatal("invalid task report")
		}
	}
	report.Scope = "bounded-api-task-probes"
	report.Results[1].Client = ""
	raw, _ = json.Marshal(report)
	if _, err := requestFromTaskReport(raw, "", ""); err != nil {
		t.Fatal("API scope", err)
	}
}

func TestCommandExplicitPolicyAndReportPersistence(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "jes-quality-latest.json")
	input, _ := json.Marshal(Request{Version: 1, Samples: []Sample{knownSample()}, Policy: Policy{MinimumScore: 1}})
	var out, diagnostic bytes.Buffer
	if code := Run([]string{"--output", path}, bytes.NewReader(input), &out, &diagnostic); code != 0 {
		t.Fatal(code, diagnostic.String())
	}
	report := Load(root)
	if report == nil || len(report.Outcomes) != 1 || report.Outcomes[0].TrainingEligible || report.Scope != "advisory-fixture-quality" {
		t.Fatal(report)
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("private report permissions", stat.Mode())
	}
	for _, raw := range [][]byte{nil, []byte(`{"version":1,"version":1}`), []byte(`{"version":99}`), []byte(`{"version":1,"samples":[],"policy":{"minimum_score":1}}`), []byte(`{} {}`)} {
		if Run(nil, bytes.NewReader(raw), &bytes.Buffer{}, &bytes.Buffer{}) != 1 {
			t.Fatal("invalid request accepted", string(raw))
		}
	}
	if Run([]string{"--unknown"}, bytes.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{}) != 2 {
		t.Fatal("unknown flag")
	}
	_ = os.WriteFile(path, []byte(`{"version":1,"scope":"other"}`), 0600)
	if Load(root) != nil {
		t.Fatal("unknown scope")
	}
}
