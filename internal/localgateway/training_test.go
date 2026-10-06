package localgateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTrainingDisabledAndOptInPrivateCapture(t *testing.T) {
	disabled, err := newTrainingRecorder(nil)
	if err != nil || disabled != nil {
		t.Fatalf("disabled recorder: %v %v", disabled, err)
	}
	disabled.record(context.Background(), trainingEvent{})
	dir := filepath.Join(t.TempDir(), "capture")
	r, err := newTrainingRecorder(&TrainingConfig{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx := withTrainingRequestID(withTrainingClient(context.Background(), "claude"), "request-one")
	r.record(ctx, trainingEvent{AttemptID: "attempt-one", Model: "sentinel-sonnet", Input: map[string]any{"messages": []any{map[string]any{"content": "inspect a file"}}, "api_key": "private"}, Output: "Bearer hidden-value sk-abcdefgh123456789", Accepted: true, Usage: map[string]int{"input_tokens": 10}})
	data, err := os.ReadFile(filepath.Join(dir, "training.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hidden-value") || strings.Contains(string(data), "sk-abcdefgh") || strings.Contains(string(data), "private") {
		t.Fatalf("credentials escaped redaction: %s", data)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event["client"] != "claude" || event["request_id"] != "request-one" || event["provider"] != "local" || event["output"] == "" {
		t.Fatalf("missing capture metadata: %v", event)
	}
	quality := event["quality"].(map[string]any)
	if quality["status"] != "unscored" || quality["training_eligible"] != false {
		t.Fatalf("unsafe quality default: %v", quality)
	}
	for path, mode := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, "training.jsonl"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("private mode %s: %v %v", path, info, err)
		}
	}
}

func TestTrainingRotationConcurrentAndOversizeBound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	r, err := newTrainingRecorder(&TrainingConfig{Directory: dir, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 30; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			r.record(context.Background(), trainingEvent{Output: "candidate", Accepted: false, Error: "invalid format"})
		}()
	}
	group.Wait()
	for _, name := range []string{"training.jsonl", "training.jsonl.1"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 1024 {
			t.Fatalf("unbounded file %s: %d", name, len(data))
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("interleaved JSON: %v", err)
			}
		}
	}
	before, _ := os.ReadFile(filepath.Join(dir, "training.jsonl"))
	r.record(context.Background(), trainingEvent{Output: strings.Repeat("x", 2048)})
	after, _ := os.ReadFile(filepath.Join(dir, "training.jsonl"))
	if string(before) != string(after) {
		t.Fatal("oversized event should be dropped")
	}
}

func TestTrainingRejectsSymlinksAndPublicFiles(t *testing.T) {
	for _, kind := range []string{"directory-symlink", "file-symlink", "public-file"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "capture")
			target := filepath.Join(base, "target")
			if kind == "directory-symlink" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, dir); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "file-symlink" {
					if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, filepath.Join(dir, "training.jsonl")); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.WriteFile(filepath.Join(dir, "training.jsonl"), nil, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := newTrainingRecorder(&TrainingConfig{Directory: dir}); err == nil {
				t.Fatal("unsafe storage accepted")
			}
		})
	}
}

func TestTrainingWriteFailureDoesNotPanicOrExposeRawData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	r, err := newTrainingRecorder(&TrainingConfig{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	r.record(context.Background(), trainingEvent{Output: "private conversation"})
}

func TestTrainingRejectsNonregularAndPreviouslyOversizedStorage(t *testing.T) {
	for _, kind := range []string{"directory-file", "oversized-current", "oversized-previous"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "capture")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "training.jsonl")
			if kind == "directory-file" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if kind == "oversized-previous" {
					path += ".1"
				}
				if err := os.WriteFile(path, []byte(strings.Repeat("x", 2048)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := newTrainingRecorder(&TrainingConfig{Directory: dir, MaxBytes: 1024}); err == nil {
				t.Fatal("unsafe/unbounded existing storage accepted")
			}
		})
	}
}
