package labbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchmarkSavesImmutableRunHistory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	first := Report{Version: 1, Scope: scope, Timestamp: time.Unix(100, 0).UTC(), Samples: []Sample{minimalSample()}}
	if err := save(path, first); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(path)
	second := first
	second.Timestamp = time.Unix(200, 0).UTC()
	second.Samples = []Sample{minimalSample()}
	second.Samples[0].TotalMS = 25
	if err := save(path, second); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "benchmark-history"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("prior benchmark lost: entries=%v err=%v", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmark-history", entries[0].Name()))
	if err != nil || string(raw) != string(firstBytes) {
		t.Fatalf("first run changed or disappeared: %s %v", raw, err)
	}
	var identity map[string]any
	if err := json.Unmarshal(raw, &identity); err != nil {
		t.Fatal(err)
	}
	if identity["run_id"] == nil || identity["run_id"] == "" {
		t.Fatal("history lacks run identity")
	}
}

func TestBenchmarkHistoryRetentionIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 25; i++ {
		r := Report{Version: 1, Scope: scope, Timestamp: time.Unix(int64(100+i), 0).UTC(), Samples: []Sample{minimalSample()}}
		if err := save(filepath.Join(root, "benchmark-latest.json"), r); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "benchmark-history"))
	if err != nil || len(entries) != 20 {
		t.Fatalf("unbounded history: %d %v", len(entries), err)
	}
	runs, err := LoadHistory(root)
	if err != nil || len(runs) != 20 || runs[0].Timestamp.Unix() != 124 || runs[19].Timestamp.Unix() != 105 {
		t.Fatalf("retention kept the wrong runs: %+v %v", runs, err)
	}
}

func TestBenchmarkRunIdentityCannotBeReusedForChangedEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	r := Report{Version: 1, Scope: scope, Timestamp: time.Unix(100, 0).UTC(), Samples: []Sample{minimalSample()}}
	if err := save(path, r); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(before, &r); err != nil {
		t.Fatal(err)
	}
	r.Timestamp = time.Unix(200, 0).UTC()
	r.Samples[0].TotalMS = 99
	if err := save(path, r); err == nil {
		t.Fatal("one run ID replaced with different evidence")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("latest evidence changed after rejected duplicate run")
	}
}

func TestBenchmarkHistoryRejectsSymlinksAndForgedIdentity(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "benchmark-history")
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	r := Report{Version: 1, Scope: scope, Timestamp: time.Unix(100, 0).UTC(), Samples: []Sample{minimalSample()}}
	if err := save(filepath.Join(root, "benchmark-latest.json"), r); err == nil {
		t.Fatal("archive followed a directory symlink")
	}
	files, _ := os.ReadDir(outside)
	if len(files) != 0 {
		t.Fatal("wrote outside archive root")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := save(filepath.Join(root, "benchmark-latest.json"), r); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	path := filepath.Join(dir, entries[0].Name())
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	r.RunID = "ffffffffffffffffffffffffffffffff"
	raw, _ = json.Marshal(r)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHistory(root); err == nil {
		t.Fatal("archive filename and evidence identity disagree")
	}
}
