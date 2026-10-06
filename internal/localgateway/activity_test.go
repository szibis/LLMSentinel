package localgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readActivity(t *testing.T, g *Gateway) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "/sentinel/activity", nil))
	if w.Code != 200 {
		t.Fatalf("activity HTTP %d: %s", w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// Catches blocking observational access in learning mode or allowing writes.
func TestActivityReadOnlyAndAvailableInLearningMode(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, LearningOnly: true, Training: &TrainingConfig{Directory: filepath.Join(t.TempDir(), "training")}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	s := readActivity(t, g)
	if s["scope"] != "local-adapter-attempts" || len(s["active"].([]any)) != 0 || s["last_completed"] != nil || s["queued"] != float64(0) {
		t.Fatalf("invalid idle activity: %v", s)
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/sentinel/activity", nil))
	if w.Code != 405 {
		t.Fatalf("activity accepted mutation: %d", w.Code)
	}
}

// Catches reporting configured/default upstream instead of the selected route,
// leaking prompt/output, and failing to clear active attempts on backend errors.
func TestActivityTracksAdapterRoutesAndCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, client string
		fail                     bool
	}{
		{"messages", "/v1/messages", `{"model":"sentinel-sonnet","max_tokens":128,"messages":[{"role":"user","content":"private-prompt"}]}`, "anthropic_messages", false},
		{"responses", "/v1/responses", `{"model":"sentinel-sonnet","max_output_tokens":128,"input":"private-prompt"}`, "openai_responses", false},
		{"chat", "/v1/chat/completions", `{"model":"sentinel-sonnet","max_tokens":128,"messages":[{"role":"user","content":"private-prompt"}]}`, "openai_chat_completions", false},
		{"backend-error", "/v1/messages", `{"model":"sentinel-sonnet","max_tokens":128,"messages":[{"role":"user","content":"private-prompt"}]}`, "anthropic_messages", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseBackend := func() { releaseOnce.Do(func() { close(release) }) }
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
				if tc.fail {
					w.WriteHeader(503)
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"private-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`))
			}))
			defer backend.Close()
			defer releaseBackend()
			g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"sonnet": backend.URL + "/v1"}, ClaudeAdapter: true, Timeout: time.Second * 5, MaxRequestBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				g.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body)))
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				releaseBackend()
				t.Fatal("backend did not start")
			}
			s := readActivity(t, g)
			active := s["active"].([]any)
			if len(active) != 1 {
				releaseBackend()
				t.Fatalf("missing active attempt: %v", s)
			}
			a := active[0].(map[string]any)
			if a["upstream"] != backend.URL+"/v1" || a["role"] != "sonnet" || a["model"] != "sentinel-sonnet" || a["client"] != tc.client || a["thinking"] != false || a["request_id"] == "" || a["attempt_id"] == "" {
				releaseBackend()
				t.Fatalf("incorrect live route: %v", a)
			}
			if _, err := time.Parse(time.RFC3339Nano, a["started_at"].(string)); err != nil {
				releaseBackend()
				t.Fatal(err)
			}
			releaseBackend()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("request did not finish")
			}
			s = readActivity(t, g)
			last := s["last_completed"].(map[string]any)
			if len(s["active"].([]any)) != 0 || last["accepted"] != !tc.fail || last["attempt_id"] != a["attempt_id"] || last["latency_ms"].(float64) < 0 {
				t.Fatalf("incorrect completion: %v", s)
			}
			if _, err := time.Parse(time.RFC3339Nano, last["finished_at"].(string)); err != nil {
				t.Fatal(err)
			}
			if !tc.fail {
				u := last["usage"].(map[string]any)
				if u["input_tokens"] != float64(10) || u["output_tokens"] != float64(3) {
					t.Fatalf("incorrect usage: %v", u)
				}
			}
			encoded, _ := json.Marshal(s)
			if strings.Contains(string(encoded), "private-") {
				t.Fatalf("activity leaked content: %s", encoded)
			}
		})
	}
}

// Catches waiting requests being mislabeled as generating or retained after cancellation.
func TestActivityQueuedCancellation(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	g.inference <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = g.inferClaude(ctx, claudeRequest{Model: "local", MaxTokens: 128}, nil)
	}()
	deadline := time.Now().Add(time.Second)
	for readActivity(t, g)["queued"] != float64(1) {
		if time.Now().After(deadline) {
			t.Fatal("queue not observed")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("queued cancellation blocked")
	}
	s := readActivity(t, g)
	if s["queued"] != float64(0) || len(s["active"].([]any)) != 0 || s["last_completed"] != nil {
		t.Fatalf("queued request invented an attempt: %v", s)
	}
	<-g.inference
}

// Catches stale generating state on cancellation and unsafe concurrent polling.
func TestActivityActiveCancellation(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: 5 * time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	started, canceled := make(chan struct{}), make(chan struct{})
	g.claudeTransport = cancelTransport{canceled: canceled, started: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
		g.ServeHTTP(httptest.NewRecorder(), r)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("attempt did not start")
	}
	var polls sync.WaitGroup
	for i := 0; i < 16; i++ {
		polls.Add(1)
		go func() {
			defer polls.Done()
			for j := 0; j < 16; j++ {
				readActivity(t, g)
			}
		}()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("attempt did not cancel")
	}
	polls.Wait()
	s := readActivity(t, g)
	last := s["last_completed"].(map[string]any)
	if len(s["active"].([]any)) != 0 || last["accepted"] != false || last["usage"] != nil {
		t.Fatalf("canceled attempt claimed generation or usage: %v", s)
	}
}
