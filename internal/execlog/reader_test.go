package execlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderDatasetMetricsAndRecommendations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("broken record\n")
	entries := []Entry{
		{SessionID: "old", CommandNormalized: "slow", DurationMS: 3000, Status: "success", OperationType: "cli", TokensEstimate: 10},
		{SessionID: "new", CommandNormalized: "slow", DurationMS: 4000, Status: "failed", OperationType: "cli", TokensEstimate: 20},
		{SessionID: "new", CommandNormalized: "slow", DurationMS: 5000, Status: "success", OperationType: "cli", TokensEstimate: 30},
		{SessionID: "new", CommandNormalized: "fast", DurationMS: 100, Status: "success", OperationType: "rest", TokensEstimate: 40},
		{SessionID: "new", CommandNormalized: "fast", DurationMS: 200, Status: "success", OperationType: "rest", TokensEstimate: 50},
		{SessionID: "new", CommandNormalized: "quick", DurationMS: 50, Status: "success", OperationType: "cli", TokensEstimate: 60},
		{SessionID: "old", CommandNormalized: "slower", DurationMS: 8000, Status: "success"},
	}
	for _, e := range entries {
		if err := json.NewEncoder(f).Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Count() != 7 || len(r.AllEntries()) != 7 {
		t.Fatal(r.Count())
	}
	m := r.SessionMetrics("new")
	if m.TotalOperations != 5 || m.TotalDurationMS != 9350 || m.AvgDurationMS != 1870 || m.SuccessRate != .8 || m.EstimatedTokens != 200 || m.OperationsByType["rest"] != 2 {
		t.Fatalf("metrics: %+v", m)
	}
	if r.SessionMetrics("").SessionID != "old" {
		t.Fatal("latest session")
	}
	if r.SessionMetrics("missing").TotalOperations != 0 {
		t.Fatal("missing session")
	}
	slow := r.SlowestOperations(1)
	if len(slow) != 1 || slow[0].Operation != "slower" {
		t.Fatal(slow)
	}
	fast := r.FastestOperations(1)
	if len(fast) != 1 || fast[0].Operation != "quick" {
		t.Fatal(fast)
	}
	c := r.CachingOpportunities()
	if len(c) != 1 || c[0].Repetitions != 3 || c[0].PotentialSavings != 9000 || c[0].AvgDurationMS != 4000 {
		t.Fatal(c)
	}
	for _, s := range r.aggregateByCommand() {
		if s.Operation == "slow" && (s.CachingPotential != "high" || s.FailureCount != 1 || s.MinDurationMS != 3000 || s.MaxDurationMS != 5000) {
			t.Fatal(s)
		}
		if s.Operation == "fast" && s.CachingPotential != "medium" {
			t.Fatal(s)
		}
	}
}

func TestReaderEmptyMissingOversizedAndTopFive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if _, err := NewReader(path); err == nil {
		t.Fatal("missing accepted")
	}
	os.WriteFile(path, nil, 0600)
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.SessionMetrics("").TotalOperations != 0 || len(r.FastestOperations(5)) != 0 || len(r.SlowestOperations(5)) != 0 {
		t.Fatal("empty metrics")
	}
	os.WriteFile(path, []byte(strings.Repeat("x", 100000)), 0600)
	if _, err := NewReader(path); err == nil {
		t.Fatal("oversized record accepted")
	}
	f, _ := os.Create(path)
	for i := 0; i < 6; i++ {
		for j := 0; j < 3; j++ {
			json.NewEncoder(f).Encode(Entry{CommandNormalized: string(rune('a' + i)), DurationMS: int64((i + 1) * 100)})
		}
	}
	f.Close()
	r, err = NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	c := r.CachingOpportunities()
	if len(c) != 5 || c[0].Operation != "f" || c[4].Operation != "b" {
		t.Fatal(c)
	}
}

func FuzzReaderJSONLines(f *testing.F) {
	f.Add(`{"session_id":"s","duration_ms":100,"status":"success"}`+"\ninvalid\n", int8(3))
	f.Add("", int8(0))
	f.Add(`{"duration_ms":3000}`, int8(-1))
	f.Fuzz(func(t *testing.T, body string, limit int8) {
		path := filepath.Join(t.TempDir(), "log.jsonl")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		r, err := NewReader(path)
		if err != nil {
			return
		}
		if r.Count() != len(r.AllEntries()) {
			t.Fatal("count mismatch")
		}
		for _, stats := range [][]OperationStats{r.FastestOperations(int(limit)), r.SlowestOperations(int(limit))} {
			if (limit <= 0 && len(stats) != 0) || (limit > 0 && len(stats) > int(limit)) {
				t.Fatal("limit exceeded")
			}
			for _, s := range stats {
				if s.ExecutionCount != s.SuccessCount+s.FailureCount {
					t.Fatal("count mismatch")
				}
			}
		}
		if len(r.CachingOpportunities()) > 5 {
			t.Fatal("too many suggestions")
		}
	})
}
