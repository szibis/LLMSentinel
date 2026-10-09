package jes

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPersistedDecisionRetainsEvidenceAndPolicy(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sample := knownSample()
	policy := Policy{MinimumScore: 1, CaptureOptIn: true, HumanApproved: true, AllowTraining: true}
	raw, _ := json.Marshal(Request{Version: 1, Samples: []Sample{sample}, Policy: policy})
	if code := Run([]string{"--output", filepath.Join(root, reportName)}, bytes.NewReader(raw), io.Discard, io.Discard); code != 0 {
		t.Fatal(code)
	}
	data, err := os.ReadFile(filepath.Join(root, reportName))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Outcomes []struct {
			Sample Sample `json:"sample"`
			Policy Policy `json:"policy"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Outcomes) != 1 || !reflect.DeepEqual(saved.Outcomes[0].Sample, sample) || saved.Outcomes[0].Policy != policy {
		t.Fatalf("decision lost evidence or policy: %s", data)
	}
	if report := Load(root); report == nil || report.TrainingEligible != 1 {
		t.Fatalf("valid eligible evidence unavailable: %+v", report)
	}
	for _, change := range []func(map[string]any){
		func(o map[string]any) { o["sample"].(map[string]any)["synthetic"] = true },
		func(o map[string]any) { o["sample"].(map[string]any)["evidence_sha256"] = "" },
		func(o map[string]any) { o["policy"].(map[string]any)["human_approved"] = false },
		func(o map[string]any) { o["backend"] = "unsupported-judge" },
	} {
		var forged map[string]any
		if err := json.Unmarshal(data, &forged); err != nil {
			t.Fatal(err)
		}
		change(forged["outcomes"].([]any)[0].(map[string]any))
		raw, _ := json.Marshal(forged)
		if err := os.WriteFile(filepath.Join(root, reportName), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if Load(root) != nil {
			t.Fatal("edited evidence retained an unsupported eligibility claim")
		}
	}
}

func TestIncompletePersistedDecisionCannotClaimTrainingEligibility(t *testing.T) {
	root := t.TempDir()
	report := Report{Version: 1, Scope: "advisory-fixture-quality", FinishedAt: time.Now().UTC(), TrainingEligible: 1, Outcomes: []Outcome{{Version: 1, Scope: "fixture-only", TrainingEligible: true}}}
	raw, _ := json.Marshal(report)
	if err := os.WriteFile(filepath.Join(root, reportName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(root); got != nil {
		t.Fatalf("unsupported training claim accepted: %+v", got)
	}
}
