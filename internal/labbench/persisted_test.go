package labbench

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func persistedReport(t *testing.T, sample Sample) string {
	t.Helper()
	root := t.TempDir()
	r := Report{Version: 1, Scope: scope, Timestamp: time.Now().UTC(), Samples: []Sample{sample}}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "benchmark-latest.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	return root
}
func minimalSample() Sample {
	return Sample{Pair: 1, Phase: "first", Role: "haiku", Protocol: "openai_chat_completions", Status: "success", TotalMS: 5}
}
func ptr(n float64) *float64 { return &n }
func TestPersistedReportCannotInventNativeAttribution(t *testing.T) {
	s := minimalSample()
	s.TTFT = ptr(999)
	s.NativeDecode = ptr(500)
	s.Cache = CacheEvidence{Status: "unknown", Provenance: "unavailable", CachedTokens: ptr(100)}
	r, e := Load(persistedReport(t, s))
	if e != nil {
		t.Fatal(e)
	}
	got := r.Samples[0]
	if got.TTFT != nil || got.NativeDecode != nil || got.Cache.CachedTokens != nil || got.Cache.Status != "unknown" {
		t.Fatal("unsupported metrics retained", got)
	}
}
func TestReportSavePreservesPreviousFileOnWriteFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	old := Report{Version: 1, Scope: scope, Timestamp: time.Now().UTC(), Samples: []Sample{minimalSample()}}
	if e := save(path, old); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	original := writeReport
	writeReport = func(path string, raw []byte, mode os.FileMode) error {
		if e := os.WriteFile(path, raw[:len(raw)/2], mode); e != nil {
			return e
		}
		return errors.New("fixture partial write")
	}
	defer func() { writeReport = original }()
	if e := save(path, Report{Version: 2}); e == nil {
		t.Fatal("failure ignored")
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(after, before) {
		t.Fatal("previous report truncated")
	}
	files, _ := os.ReadDir(root)
	if len(files) != 2 {
		t.Fatal("temporary file leaked", files)
	}
}

func attributedSample() Sample {
	s := minimalSample()
	s.StartedAt = time.Now().UTC()
	s.ObservedRole = s.Role
	s.Before = Snapshot{Source: "http://127.0.0.1:19090/sentinel/status", Model: "native-haiku", Requests: ptr(2), Uptime: ptr(10), Reused: ptr(20), Processed: ptr(30)}
	s.After = Snapshot{Source: s.Before.Source, Model: s.Before.Model, Requests: ptr(3), Uptime: ptr(11), Reused: ptr(30), Processed: ptr(40), Native: true, Cached: ptr(10), TTFT: ptr(12), Decode: ptr(40)}
	s.TTFT = ptr(12)
	s.NativeDecode = ptr(40)
	s.Cache = CacheEvidence{Status: "cached_tokens_observed", Provenance: "isolated_runtime_counter_window", ReusedDelta: ptr(10), ProcessedDelta: ptr(10), CachedTokens: ptr(10)}
	return s
}
func TestPersistedNativeEvidenceRequiresMatchingWindow(t *testing.T) {
	for name, alter := range map[string]func(*Sample){"valid": func(*Sample) {}, "native absent": func(s *Sample) { s.After.Native = false }, "role mismatch": func(s *Sample) { s.ObservedRole = "opus" }, "fractional counters": func(s *Sample) { s.Before.Requests = ptr(2.5); s.After.Requests = ptr(3.5) }, "counter reset": func(s *Sample) { s.After.Requests = ptr(1) }, "multiple requests": func(s *Sample) { s.After.Requests = ptr(4) }, "uptime reset": func(s *Sample) { s.After.Uptime = ptr(1) }, "model changed": func(s *Sample) { s.After.Model = "foreign" }, "source changed": func(s *Sample) { s.After.Source = "foreign" }, "cache reset": func(s *Sample) { s.After.Reused = ptr(1) }, "no provenance": func(s *Sample) { s.Cache.Provenance = "unavailable" }, "wrong delta": func(s *Sample) { s.Cache.ProcessedDelta = ptr(99) }, "wrong cached tokens": func(s *Sample) { s.Cache.CachedTokens = ptr(99) }, "wrong cache status": func(s *Sample) { s.Cache.Status = "warm" }, "missing native cache": func(s *Sample) { s.After.Cached = nil }, "wrong native measurements": func(s *Sample) { s.TTFT = ptr(999); s.NativeDecode = ptr(999) }, "zero cache": func(s *Sample) {
		s.After.Cached = ptr(0)
		s.Cache.CachedTokens = ptr(0)
		s.Cache.Status = "zero_cached_tokens_observed"
	}} {
		t.Run(name, func(t *testing.T) {
			s := attributedSample()
			alter(&s)
			r, e := Load(persistedReport(t, s))
			if e != nil {
				t.Fatal(e)
			}
			got := r.Samples[0]
			if name == "valid" || name == "zero cache" {
				if got.TTFT == nil || *got.TTFT != 12 || got.NativeDecode == nil || *got.NativeDecode != 40 || got.Cache.CachedTokens == nil {
					t.Fatal("valid evidence lost", got)
				}
			} else if got.TTFT != nil || got.NativeDecode != nil {
				t.Fatal("unsupported evidence retained", got)
			}
		})
	}
}
func TestAtomicReplacementIsPrivate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	// #nosec G306 -- Fixed temporary fixture starts world-readable to verify replacement tightens it to 0600.
	os.WriteFile(path, []byte("previous"), 0600)
	r := Report{Version: 1, Scope: scope, Timestamp: time.Now().UTC(), Samples: []Sample{attributedSample()}}
	if e := save(path, r); e != nil {
		t.Fatal(e)
	}
	loaded, e := Load(root)
	if e != nil || loaded.Samples[0].TTFT == nil {
		t.Fatal(loaded, e)
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, e)
	}
	files, _ := os.ReadDir(root)
	if len(files) != 2 {
		t.Fatal("temporary file leaked")
	}
}
func FuzzPersistedAttribution(f *testing.F) {
	f.Add(uint8(0), int64(3), "haiku", "isolated_runtime_counter_window")
	f.Add(uint8(1), int64(999), "opus", "unknown")
	f.Fuzz(func(t *testing.T, flags uint8, requests int64, role, provenance string) {
		s := attributedSample()
		s.After.Requests = ptr(float64(requests))
		s.ObservedRole = role
		s.Cache.Provenance = provenance
		if flags&1 != 0 {
			s.After.Native = false
		}
		if flags&2 != 0 {
			s.After.Cached = nil
		}
		if flags&4 != 0 {
			s.TTFT = ptr(999)
		}
		if flags&8 != 0 {
			s.Before.Reused = nil
		}
		normalizePersistedAttribution(&s)
		if s.TTFT != nil || s.NativeDecode != nil {
			if !s.After.Native || s.After.Cached == nil || s.ObservedRole != s.Role || s.After.Requests == nil || *s.After.Requests-*s.Before.Requests != 1 || s.Cache.Provenance != "isolated_runtime_counter_window" {
				t.Fatal("unsupported attribution escaped")
			}
		}
		copy := s
		normalizePersistedAttribution(&copy)
		if !reflect.DeepEqual(s, copy) {
			t.Fatal("normalization not idempotent")
		}
	})
}
