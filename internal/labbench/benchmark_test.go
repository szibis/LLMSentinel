package labbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type benchFixture struct {
	mu       sync.Mutex
	counts   map[string]int
	last     map[string]any
	prompts  []string
	sessions []string
	mode     string
}

func fixtureServer(t *testing.T, mode string) (*httptest.Server, *benchFixture) {
	t.Helper()
	state := &benchFixture{counts: map[string]int{}, mode: mode}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.URL.Path {
		case "/health":
			if state.mode == "hybrid" {
				io.WriteString(w, `{"mode":"hybrid","policy":"hybrid-paid"}`)
			} else {
				io.WriteString(w, `{"mode":"serving","policy":"strict-local"}`)
			}
		case "/sentinel/activity":
			json.NewEncoder(w).Encode(map[string]any{"queued": 0, "active": []any{}, "last_completed": state.last})
		case "/sentinel/status", "/native":
			if state.mode == "stats-http-error" {
				w.WriteHeader(503)
				return
			}
			if state.mode == "stats-null" {
				io.WriteString(w, "null")
				return
			}
			runtimes := map[string]any{}
			for _, role := range []string{"haiku", "sonnet", "opus"} {
				count := state.counts[role]
				cached := 0
				if count%2 == 0 && count > 0 && state.mode != "zero-cache" {
					cached = 100
				}
				requests := count
				if state.mode == "foreign" && count > 0 {
					requests = count * 2
				}
				runtimes[role] = map[string]any{"model": "native-" + role, "stats": map[string]any{"requests": requests, "uptime_s": 100 + count, "prompt_cache": map[string]any{"reused_tokens": (count / 2) * 100, "processed_tokens": count * 20}, "last_generation": map[string]any{"native_generation_metadata": true, "cached_prompt_tokens": cached, "ttft_ms": 12, "generation_tps": 40}}}
			}
			if r.URL.Path == "/native" {
				json.NewEncoder(w).Encode(runtimes["haiku"])
			} else {
				json.NewEncoder(w).Encode(map[string]any{"runtime_health": runtimes})
			}
		case "/v1/chat/completions":
			var request struct {
				Model    string `json:"model"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
				Stream    bool `json:"stream"`
				MaxTokens int  `json:"max_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Stream || request.MaxTokens != 32 || len(request.Messages) != 1 {
				t.Errorf("invalid fixed request: %+v", request)
			}
			state.prompts = append(state.prompts, request.Messages[0].Content)
			state.sessions = append(state.sessions, r.Header.Get("X-Session-ID"))
			state.counts[request.Model]++
			role := request.Model
			if state.mode == "different-role" {
				role = "opus"
			}
			now := time.Now().UTC()
			state.last = map[string]any{"attempt_id": fmt.Sprintf("attempt-%d", len(state.prompts)), "role": role, "started_at": now, "finished_at": now.Add(time.Microsecond), "accepted": true}
			switch state.mode {
			case "request-http-error":
				w.WriteHeader(422)
				io.WriteString(w, `{"error":"bad"}`)
			case "malformed":
				io.WriteString(w, "{")
			case "null":
				io.WriteString(w, "null")
			case "no-choice":
				io.WriteString(w, `{"choices":[]}`)
			case "no-text":
				io.WriteString(w, `{"choices":[{"message":{"content":null}}]}`)
			case "disconnect":
				h, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("hijack unavailable")
				}
				conn, _, err := h.Hijack()
				if err != nil {
					t.Error(err)
				} else {
					conn.Close()
				}
			case "missing-usage":
				io.WriteString(w, `{"model":"haiku","choices":[{"message":{"content":"blue"}}],"usage":null}`)
			case "negative-usage":
				io.WriteString(w, `{"choices":[{"message":{"content":"blue"}}],"usage":{"prompt_tokens":-1,"completion_tokens":1}}`)
			case "response-error":
				io.WriteString(w, `{"error":{"message":"failed"},"choices":[{"message":{"content":"blue"}}]}`)
			case "oversized":
				io.WriteString(w, strings.Repeat("x", maxBody+1))
			default:
				json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "choices": []any{map[string]any{"message": map[string]string{"content": "blue"}}}, "usage": map[string]int{"prompt_tokens": 500, "completion_tokens": 1}})
			}
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return server, state
}
func configFor(server *httptest.Server) Config {
	return Config{Endpoint: server.URL, Roles: []string{"haiku"}, Pairs: 1, Timeout: time.Second}
}

func TestPairedPrefixHTTPMeasurement(t *testing.T) {
	server, state := fixtureServer(t, "")
	cfg := configFor(server)
	cfg.Pairs = 2
	cfg.Roles = []string{"haiku", "sonnet"}
	report, err := Benchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.PromptSet != "paired-prefix-v2" || report.SessionProvenance != "isolated_random_run_role_pair_session" || report.Version != 1 || report.Scope != scope || report.Summary.Successful != 8 || report.Summary.P50 == nil || report.Summary.P95 == nil || len(report.Samples) != 8 {
		t.Fatalf("report: %+v", report)
	}
	for i, s := range report.Samples {
		if s.Status != "success" || s.HTTPStatus != 200 || s.Output != "blue" || s.TotalMS < 0 || s.InputTokens == nil || *s.InputTokens != 500 || s.OutputTokens == nil || *s.OutputTokens != 1 || s.UsageSource != "completion_response" || s.TTFT == nil || *s.TTFT != 12 || s.NativeDecode == nil || *s.NativeDecode != 40 || s.ObservedRole != s.Role {
			t.Fatalf("sample: %+v", s)
		}
		if s.Cache.ReusedDelta == nil || s.Cache.ProcessedDelta == nil || *s.Cache.ProcessedDelta != 20 {
			t.Fatal(s.Cache)
		}
		want := "zero_cached_tokens_observed"
		if i%2 == 1 {
			want = "cached_tokens_observed"
		}
		if s.Cache.Status != want {
			t.Fatal(s.Cache)
		}
	}
	for i := 0; i < len(state.prompts); i += 2 {
		if state.prompts[i] != state.prompts[i+1] {
			t.Fatal("paired prefixes differ")
		}
	}
	if state.prompts[0] == state.prompts[2] {
		t.Fatal("different pair identifier missing")
	}
	cfg.Roles = []string{"haiku"}
	cfg.StatsEndpoint = server.URL + "/native"
	if _, err := Benchmark(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAndForeignStatsRemainUnknown(t *testing.T) {
	for _, mode := range []string{"stats-null", "stats-http-error", "foreign", "different-role"} {
		t.Run(mode, func(t *testing.T) {
			server, _ := fixtureServer(t, mode)
			report, err := Benchmark(context.Background(), configFor(server))
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range report.Samples {
				if s.Cache.Status != "unknown" || s.Cache.CachedTokens != nil || s.TTFT != nil || s.NativeDecode != nil {
					t.Fatalf("invented telemetry: %+v", s)
				}
			}
		})
	}
}
func TestCompletionErrorsAndUnknownUsage(t *testing.T) {
	for _, mode := range []string{"request-http-error", "malformed", "null", "no-choice", "no-text", "disconnect", "negative-usage", "response-error", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			server, _ := fixtureServer(t, mode)
			report, err := Benchmark(context.Background(), configFor(server))
			if err == nil || len(report.Samples) != 1 || report.Samples[0].Status != "error" || report.Summary.Successful != 0 || report.Summary.P50 != nil {
				t.Fatalf("accepted failure: %+v %v", report, err)
			}
		})
	}
	server, _ := fixtureServer(t, "missing-usage")
	report, err := Benchmark(context.Background(), configFor(server))
	if err != nil {
		t.Fatal(err)
	}
	if report.Samples[0].InputTokens != nil || report.Samples[0].OutputTokens != nil || report.Samples[0].UsageSource != "unavailable" {
		t.Fatal(report.Samples[0])
	}
}

func TestSafetyValidationHealthAndCancellation(t *testing.T) {
	server, _ := fixtureServer(t, "")
	valid := configFor(server)
	for _, modify := range []func(*Config){func(c *Config) { c.Endpoint = "https://127.0.0.1" }, func(c *Config) { c.Endpoint = "http://localhost" }, func(c *Config) { c.Endpoint = "http://127.0.0.1/path" }, func(c *Config) { c.Endpoint = ":bad" }, func(c *Config) { c.Pairs = 0 }, func(c *Config) { c.Pairs = 21 }, func(c *Config) { c.Timeout = 0 }, func(c *Config) { c.Timeout = 11 * time.Minute }, func(c *Config) { c.Roles = nil }, func(c *Config) { c.Roles = []string{"haiku", "haiku"} }, func(c *Config) { c.Roles = []string{"vendor"} }, func(c *Config) { c.StatsEndpoint = "https://example.com" }, func(c *Config) { c.StatsEndpoint = server.URL; c.Roles = []string{"haiku", "opus"} }} {
		cfg := valid
		modify(&cfg)
		if _, err := Benchmark(context.Background(), cfg); err == nil {
			t.Fatal("invalid config accepted", cfg)
		}
	}
	hybrid, state := fixtureServer(t, "hybrid")
	if _, err := Benchmark(context.Background(), configFor(hybrid)); err == nil || len(state.prompts) != 0 {
		t.Fatal("hybrid dispatch occurred")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Benchmark(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	badHealth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "null") }))
	defer badHealth.Close()
	if _, err := Benchmark(context.Background(), configFor(badHealth)); err == nil {
		t.Fatal("null health accepted")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/health", http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := Benchmark(context.Background(), configFor(redirect)); err == nil {
		t.Fatal("redirect followed")
	}
}

func TestCancellationDuringInference(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			io.WriteString(w, `{"mode":"serving","policy":"strict-local"}`)
		case "/v1/chat/completions":
			close(started)
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		default:
			io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		report, err := Benchmark(ctx, configFor(server))
		if len(report.Samples) != 1 || report.Samples[0].Status != "error" || report.Summary.Successful != 0 {
			result <- errors.New("invalid canceled report")
			return
		}
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("inference not started")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop benchmark")
	}
}

func TestSnapshotSchemasAndInvalidNumbers(t *testing.T) {
	for _, prefix := range []string{"", "health", "runtimes"} {
		m := `{"model":"test","stats":{"requests":1,"uptime_s":10},"prompt_cache":{"reused_tokens":2,"processed_tokens":3},"last_generation":{"native_generation_metadata":true,"cached_prompt_tokens":0,"ttft_ms":4,"generation_tps":5}}`
		if prefix == "health" {
			m = `{"health":` + m + `}`
		} else if prefix == "runtimes" {
			m = `{"runtimes":{"haiku":` + m + `}}`
		}
		s := snapshot([]byte(m), "haiku", "source")
		if s.Error != "" || s.Requests == nil || *s.Requests != 1 || s.Decode == nil || *s.Decode != 5 {
			t.Fatal(s)
		}
	}
	for _, raw := range []string{"null", "{}", `{"runtime_health":{"haiku":null}}`, `{"stats":{"requests":-1,"uptime_s":null,"prompt_cache":{"reused_tokens":1.5,"processed_tokens":"4"}}}`} {
		s := snapshot([]byte(raw), "haiku", "s")
		if s.Requests != nil {
			t.Fatal("invalid count accepted", s)
		}
	}
	for _, raw := range []string{`{}`, `{"n":null}`, `{"n":-1}`, `{"n":1.5}`, `{"n":"2"}`, `{"n":1e999}`} {
		m, err := object([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if number(m, "n", true) != nil {
			t.Fatal("invalid number accepted", raw)
		}
	}
	for _, raw := range []string{"null", "[]", "{} {}", "{"} {
		if _, err := object([]byte(raw)); err == nil {
			t.Fatal("invalid object accepted", raw)
		}
	}
}

func TestCounterResetAndNativeMetadataGates(t *testing.T) {
	n := func(v float64) *float64 { return &v }
	now := time.Now()
	before := Snapshot{Model: "native", Requests: n(2), Uptime: n(10), Reused: n(20), Processed: n(30)}
	after := Snapshot{Model: "native", Requests: n(3), Uptime: n(11), Reused: n(30), Processed: n(40), Native: true, Cached: n(10), TTFT: n(1), Decode: n(2)}
	a := &activity{}
	b := &activity{}
	json.Unmarshal([]byte(fmt.Sprintf(`{"last_completed":{"attempt_id":"new","role":"haiku","accepted":true,"started_at":%q,"finished_at":%q}}`, now.Format(time.RFC3339Nano), now.Add(time.Millisecond).Format(time.RFC3339Nano))), b)
	for _, modify := range []func(*Snapshot, *Snapshot, *activity){func(_, s *Snapshot, _ *activity) { s.Requests = n(1) }, func(_, s *Snapshot, _ *activity) { s.Uptime = n(9) }, func(_, s *Snapshot, _ *activity) { s.Model = "changed" }, func(_, s *Snapshot, _ *activity) { s.Reused = n(10) }, func(_, s *Snapshot, _ *activity) { s.Reused = nil }, func(_, s *Snapshot, _ *activity) { s.Processed = nil }, func(_, s *Snapshot, b *activity) { b.Queued = 1 }, func(_, s *Snapshot, b *activity) { b.Last.Role = "opus" }} {
		x, y := before, after
		copied := *b
		last := *b.Last
		copied.Last = &last
		modify(&x, &y, &copied)
		e, ttft, decode, _ := cacheEvidence(x, y, a, &copied, now, "haiku")
		if e.Status != "unknown" || ttft != nil || decode != nil {
			t.Fatal(e)
		}
	}
	after.Native = false
	e, ttft, decode, role := cacheEvidence(before, after, a, b, now, "haiku")
	if e.ReusedDelta == nil || e.CachedTokens != nil || ttft != nil || decode != nil || role != "haiku" {
		t.Fatal(e)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestCLIAndReportLoad(t *testing.T) {
	server, _ := fixtureServer(t, "")
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	var out, stderr bytes.Buffer
	args := []string{"--endpoint", server.URL, "--role", "haiku", "--pairs", "1", "--timeout", "1", "--output", path}
	if code := Run(args, nil, &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	loaded, err := Load(root)
	if err != nil || len(loaded.Samples) != 2 {
		t.Fatalf("%+v %v", loaded, err)
	}
	if code := Run([]string{"--unknown"}, nil, &out, &stderr); code != 2 {
		t.Fatal(code)
	}
	if code := Run([]string{"positional"}, nil, &out, &stderr); code != 2 {
		t.Fatal(code)
	}
	if code := Run([]string{"--pairs", "0"}, nil, &out, &stderr); code != 2 {
		t.Fatal(code)
	}
	args[len(args)-1] = ""
	if code := Run(args, nil, failWriter{}, &stderr); code != 1 {
		t.Fatal(code)
	}
	hybrid, _ := fixtureServer(t, "hybrid")
	if code := Run([]string{"--endpoint", hybrid.URL, "--output", ""}, nil, &out, &stderr); code != 1 {
		t.Fatal(code)
	}
	args[len(args)-1] = root
	if code := Run(args, nil, &out, &stderr); code != 1 {
		t.Fatal(code)
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("missing report accepted")
	}
	for _, raw := range []string{"null", `{"schema_version":2}`, `{"schema_version":1,"scope":"synthetic-benchmark","timestamp":"2026-01-01T00:00:00Z","samples":[{"pair":0}]}`, `{"schema_version":1,"scope":"synthetic-benchmark","timestamp":"2026-01-01T00:00:00Z","samples":[{"pair":1,"total_ms":-1}]}`, `{"schema_version":"wrong"}`} {
		os.WriteFile(path, []byte(raw), 0600)
		if _, err := Load(root); err == nil {
			t.Fatal("invalid report accepted", raw)
		}
	}
	os.Remove(path)
	if err := os.Symlink("other", path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("symlink report accepted")
	}
	if err := save(path, loaded); err == nil {
		t.Fatal("symlink output accepted")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0600)
	if err := save(filepath.Join(blocker, "output"), loaded); err == nil {
		t.Fatal("file parent accepted")
	}
}

func TestLoadSanitizesAndValidatesMeasurements(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "benchmark-latest.json")
	raw := `{"schema_version":1,"scope":"synthetic-benchmark","timestamp":"2026-01-01T00:00:00Z","prompt_set":"paired-prefix-v1","secret":"discard","summary":{"successful_requests":500,"total_p50_ms":999},"samples":[{"pair":1,"phase":"first","role":"haiku","protocol":"openai_chat_completions","status":"success","total_ms":5,"cost_usd":100}]}`
	os.WriteFile(path, []byte(raw), 0600)
	report, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "cost_usd") || report.Summary.Successful != 1 || report.Summary.P50 == nil || *report.Summary.P50 != 5 {
		t.Fatalf("unsanitized report: %s", encoded)
	}
	for _, altered := range []string{strings.Replace(raw, `"role":"haiku"`, `"role":"vendor"`, 1), strings.Replace(raw, `"status":"success"`, `"status":"invented"`, 1), strings.Replace(raw, `"total_ms":5`, `"total_ms":5,"input_tokens":-1`, 1)} {
		os.WriteFile(path, []byte(altered), 0600)
		if _, err := Load(root); err == nil {
			t.Fatal("invalid measurement accepted")
		}
	}
}

func FuzzTelemetrySnapshot(f *testing.F) {
	f.Add(`{"model":"native","stats":{"requests":1,"prompt_cache":{"reused_tokens":0,"processed_tokens":5}}}`)
	f.Add("null")
	f.Add(`{"stats":{"requests":-1}}`)
	f.Fuzz(func(t *testing.T, raw string) {
		s := snapshot([]byte(raw), "haiku", "fixture")
		for _, n := range []*float64{s.Requests, s.Uptime, s.Reused, s.Processed, s.Cached, s.TTFT, s.Decode} {
			if n != nil && *n < 0 {
				t.Fatal("negative telemetry escaped")
			}
		}
		if _, err := json.Marshal(s); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPairedBenchmarkSharesOnlyItsOwnSession(t *testing.T) {
	server, state := fixtureServer(t, "")
	cfg := configFor(server)
	cfg.Pairs = 2
	cfg.Roles = []string{"haiku", "sonnet"}
	for run := 0; run < 2; run++ {
		if _, err := Benchmark(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for i := 0; i < len(state.sessions); i += 2 {
		session := state.sessions[i]
		if session == "" || session != state.sessions[i+1] {
			t.Fatalf("pair sessions differ or absent: %q %q", session, state.sessions[i+1])
		}
		if seen[session] {
			t.Fatal("different pair, role or run reused session", session)
		}
		seen[session] = true
	}
	if len(seen) != 8 {
		t.Fatal("missing pairs", len(seen))
	}
}

func TestBenchmarkEntropyFailureMakesNoRequest(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			old := readBenchmarkRandom
			defer func() { readBenchmarkRandom = old }()
			readBenchmarkRandom = func(b []byte) (int, error) {
				if short {
					return len(b) - 1, nil
				}
				return 0, errors.New("fixture entropy unavailable")
			}
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				io.WriteString(w, `{"mode":"serving","policy":"strict-local"}`)
			}))
			defer server.Close()
			report, err := Benchmark(context.Background(), configFor(server))
			if err == nil || calls != 0 || len(report.Samples) != 0 {
				t.Fatal("entropy failure not closed", err, calls, report)
			}
		})
	}
}

func TestPairedSessionDoesNotClaimWarmthWithoutNativeReuse(t *testing.T) {
	server, state := fixtureServer(t, "zero-cache")
	report, err := Benchmark(context.Background(), configFor(server))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.sessions) != 2 || state.sessions[0] == "" || state.sessions[0] != state.sessions[1] {
		t.Fatal("pair session missing", state.sessions)
	}
	for _, sample := range report.Samples {
		if sample.Cache.Status != "zero_cached_tokens_observed" || sample.Cache.CachedTokens == nil || *sample.Cache.CachedTokens != 0 {
			t.Fatal("repeat order inferred warmth", sample)
		}
	}
}
