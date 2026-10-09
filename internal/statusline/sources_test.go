package statusline

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type localSource struct {
	name      string
	priority  int
	available bool
	release   chan struct{}
	err       error
}

func (s *localSource) Name() string      { return s.name }
func (s *localSource) Priority() int     { return s.priority }
func (s *localSource) IsAvailable() bool { return s.available }
func (s *localSource) Poll() (StatuslineData, error) {
	if s.release != nil {
		<-s.release
	}
	return StatuslineData{Source: s.name, InputTokens: intPointer(17)}, s.err
}

func TestRegistrySelectionErrorsAndTimeout(t *testing.T) {
	r := NewRegistry(0)
	if r.GetBest() != nil {
		t.Fatal("empty source")
	}
	if _, err := r.Poll(); err == nil {
		t.Fatal("empty poll")
	}
	r.Register(nil)
	r.Register(&localSource{name: "disabled"})
	low := &localSource{name: "low", priority: 5, available: true}
	high := &localSource{name: "high", priority: 1, available: true}
	r.Register(low)
	r.Register(high)
	if r.GetBest() != high || len(r.GetSources()) != 2 || !r.Health()["high"] {
		t.Fatal("selection")
	}
	if data, err := r.Poll(); err != nil || data.Source != "high" || (data.InputTokens == nil || *data.InputTokens != 17) {
		t.Fatal(data, err)
	}
	high.available = false
	if r.GetBest() != low {
		t.Fatal("unavailable selected")
	}
	low.available = false
	if r.GetBest() != nil {
		t.Fatal("unavailable fallback")
	}
	high.available = true
	high.err = errors.New("source failed")
	if _, err := r.Poll(); err == nil {
		t.Fatal("failure hidden")
	}
	high.err = nil
	high.release = make(chan struct{})
	r.timeout = time.Millisecond
	if _, err := r.Poll(); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatal(err)
	}
	close(high.release)
}

func TestEnvExplicitAccounting(t *testing.T) {
	for _, k := range []string{"CLAUDE_TOKENS_INPUT", "CLAUDE_TOKENS_OUTPUT", "CLAUDE_TOKENS_ACTUAL", "CLAUDE_CACHE_HIT_TOKENS", "CLAUDE_CACHE_CREATION_TOKENS", "CLAUDE_MODEL", "CLAUDE_CONTEXT_USAGE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	e := NewEnvVarSource()
	if e.Name() != "envvar" || e.Priority() != 5 || e.IsAvailable() {
		t.Fatal(e)
	}
	if _, err := e.Poll(); err == nil {
		t.Fatal("missing env accepted")
	}
	values := map[string]string{"CLAUDE_TOKENS_INPUT": "17", "CLAUDE_TOKENS_OUTPUT": "3", "CLAUDE_TOKENS_ACTUAL": "20", "CLAUDE_CACHE_HIT_TOKENS": "2", "CLAUDE_CACHE_CREATION_TOKENS": "1", "CLAUDE_MODEL": "synthetic", "CLAUDE_CONTEXT_USAGE": "25"}
	for k, v := range values {
		t.Setenv(k, v)
	}
	e = NewEnvVarSource()
	data, err := e.Poll()
	if err != nil || (data.InputTokens == nil || *data.InputTokens != 17) || (data.OutputTokens == nil || *data.OutputTokens != 3) || data.CacheHitTokens != 2 || data.CacheCreationTokens != 1 || data.Model != "synthetic" || data.ContextWindowUsage != 25 {
		t.Fatal(data, err)
	}
	for k := range values {
		t.Setenv(k, "invalid")
	}
	if _, err := e.Poll(); err != nil {
		t.Fatal(err)
	}
}

func TestEnvTotalDoesNotInferSplit(t *testing.T) {
	for _, k := range []string{"CLAUDE_TOKENS_INPUT", "CLAUDE_TOKENS_OUTPUT"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Setenv("CLAUDE_TOKENS_ACTUAL", "100")
	data, err := NewEnvVarSource().Poll()
	if err != nil {
		t.Fatal(err)
	}
	if data.InputTokens != nil || data.OutputTokens != nil || data.TotalTokens == nil || *data.TotalTokens != 100 {
		t.Fatalf("total-only observation invented token split: %+v", data)
	}
}

func TestNativeAndBaristaSyntheticFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	nativePath := filepath.Join(home, ".claude", "statusline.json")
	baristaPath := filepath.Join(home, ".claude", "data", "escalation", "barista-metrics.json")
	configPath := filepath.Join(home, ".claude", "barista.conf")
	if NewNativeSource("").IsAvailable() || NewBaristaSource("").IsAvailable() {
		t.Fatal("missing sources")
	}
	os.MkdirAll(filepath.Dir(baristaPath), 0700)
	os.WriteFile(configPath, []byte("synthetic"), 0600)
	native := NewNativeSource(nativePath)
	barista := NewBaristaSource("")
	if native.Name() != "claude-native" || native.Priority() != 2 || barista.Name() != "barista" || barista.Priority() != 1 {
		t.Fatal("identity")
	}
	if _, err := native.Poll(); err == nil {
		t.Fatal("missing native accepted")
	}
	if _, err := barista.Poll(); err == nil {
		t.Fatal("missing barista accepted")
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{"{", false}, {`{}`, false}, {`{"input_tokens":-1,"output_tokens":3}`, false}, {`{"input_tokens":1000001,"output_tokens":3}`, false},
		{`{"input_tokens":17,"output_tokens":3}`, true},
		{`{"input_tokens":17,"output_tokens":3,"cache_hit_tokens":2,"cache_creation_tokens":1,"model":"synthetic","cache_fill_percent":0.5}`, true},
	} {
		os.WriteFile(nativePath, []byte(tc.body), 0600)
		os.WriteFile(baristaPath, []byte(tc.body), 0600)
		native = NewNativeSource("")
		a, e := native.Poll()
		b, f := barista.Poll()
		if (e == nil) != tc.valid || (f == nil) != tc.valid {
			t.Fatal(tc, e, f)
		}
		if tc.valid && ((a.InputTokens == nil || *a.InputTokens != 17) || (b.OutputTokens == nil || *b.OutputTokens != 3)) {
			t.Fatal(a, b)
		}
	}
	nativeBody := `{"input_tokens":17,"output_tokens":3,"context_window":{"used":200,"total":100},"caching_enabled":true,"cache_fill_percent":0.5}`
	os.WriteFile(nativePath, []byte(nativeBody), 0600)
	a, err := native.Poll()
	if err != nil || a.ContextWindowUsage != 100 || !a.IsCaching {
		t.Fatal(a, err)
	}
	os.WriteFile(nativePath, []byte(`{"input_tokens":17,"output_tokens":3,"context_window":{"used":-1,"total":100}}`), 0600)
	if _, err := native.Poll(); err == nil {
		t.Fatal("negative context")
	}
	os.WriteFile(baristaPath, []byte(`{"input_tokens":17,"output_tokens":3,"context_usage_percent":25,"is_caching":true,"cache_fill_percent":0.5}`), 0600)
	b, err := barista.Poll()
	if err != nil || b.ContextWindowUsage != 25 || !b.IsCaching {
		t.Fatal(b, err)
	}
	old := time.Now().Add(-time.Minute)
	os.Chtimes(nativePath, old, old)
	os.Chtimes(baristaPath, old, old)
	if native.IsAvailable() || barista.IsAvailable() {
		t.Fatal("stale sources accepted")
	}
	os.Remove(nativePath)
	os.Remove(baristaPath)
	if native.IsAvailable() || barista.IsAvailable() {
		t.Fatal("deleted sources accepted")
	}
	// Stat succeeds, but opening a mode000 file must expose a source read error.
	os.WriteFile(nativePath, []byte(`{}`), 0000)
	os.WriteFile(baristaPath, []byte(`{}`), 0000)
	if _, err := native.Poll(); err == nil {
		t.Fatal("unreadable native")
	}
	if _, err := barista.Poll(); err == nil {
		t.Fatal("unreadable barista")
	}
}

type localTransport func(*http.Request) (*http.Response, error)

func (f localTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWebhookTransportErrorsAndOptionalMetrics(t *testing.T) {
	w := NewWebhookSource("https://8.8.8.8/metrics", "synthetic")
	if !w.IsAvailable() || w.Name() != "webhook" || w.Priority() != 3 {
		t.Fatal(w)
	}
	w.client.Transport = localTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer synthetic" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing headers")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"input_tokens":17,"output_tokens":3,"cache_hit_tokens":2,"cache_creation_tokens":1,"context_usage_percent":25,"model":"synthetic","is_caching":true,"cache_fill_percent":0.5}`))}, nil
	})
	data, err := w.Poll()
	if err != nil || data.CacheHitTokens != 2 || data.Model != "synthetic" || !data.IsCaching {
		t.Fatal(data, err)
	}
	w.client.Transport = localTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if _, err := w.Poll(); err == nil {
		t.Fatal("transport failure hidden")
	}
	w.client.Transport = localTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})
	if _, err := w.Poll(); err == nil {
		t.Fatal("server failure hidden")
	}
	w.url = ":invalid"
	if _, err := w.Poll(); err == nil {
		t.Fatal("invalid request")
	}
}

func intPointer(v int) *int { return &v }
