package localgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLearningModeOnlyCapturesCopiesAndControls(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true, LearningOnly: true, Training: &TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
		if w.Code != 403 {
			t.Fatalf("learning inferred %s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "/sentinel/control", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"learning"`) {
		t.Fatalf("missing controls %d %s", w.Code, w.Body.String())
	}
	request := httptest.NewRequest("POST", "/sentinel/control", strings.NewReader(`{"capture_enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	g.ServeHTTP(w, request)
	if w.Code != 200 || g.training.captureEnabled() {
		t.Fatal("capture did not pause")
	}
}

func TestCopiedCapturePreservesUnknownUsageAndPause(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, LearningOnly: true, Training: &TrainingConfig{Directory: dir}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	post := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w
	}
	copy := `{"client":"codex","source":"client_hook","event_id":"fixture-turn","inputs":["synthetic question"],"outputs":["synthetic answer"],"usage":{"input_tokens":null,"output_tokens":17},"billing":{"cost_usd":null}}`
	if w := post("/sentinel/training/events", copy); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	before, err := os.ReadFile(filepath.Join(dir, "training.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(before, &event); err != nil {
		t.Fatal(err)
	}
	ref := event["reference"].(map[string]any)
	if ref["usage"].(map[string]any)["input_tokens"] != nil || event["quality"].(map[string]any)["training_eligible"] != false {
		t.Fatal("invented measurement or label")
	}
	if w := post("/sentinel/control", `{"capture_enabled":false}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := post("/sentinel/training/events", copy); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(dir, "training.jsonl"))
	if string(before) != string(after) {
		t.Fatal("paused collector persisted a new event")
	}
}

func TestLiveBudgetControlsActualNextInference(t *testing.T) {
	var observed int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think"}}`))
			return
		}
		var request struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		observed = request.MaxTokens
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`))
	}))
	defer backend.Close()
	g, err := New(Config{Upstream: backend.URL + "/v1", RoleUpstreams: map[string]string{"sonnet": backend.URL + "/v1"}, Router: RoleRouter{}, Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	r := httptest.NewRequest("POST", "/sentinel/control", strings.NewReader(`{"role_budgets":{"sonnet":321}}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-sonnet","max_tokens":1000,"messages":[{"role":"user","content":"hello"}]}`)))
	if w.Code != 200 || observed != 321 {
		t.Fatalf("budget did not govern generation: HTTP %d, budget %d", w.Code, observed)
	}
}

func TestControlValidatesBeforeChangingPolicy(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, body := range []string{`{"capture_enabled":true}`, `{"mode":"hybrid"}`, `{"role_budgets":{"opus":40000}}`, `{"policy":"quality"}`} {
		req := httptest.NewRequest("POST", "/sentinel/control", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		if w.Code < 400 {
			t.Fatalf("unsupported change accepted %s", body)
		}
	}
	req := httptest.NewRequest("POST", "/sentinel/control", strings.NewReader(`{"role_budgets":{"opus":16384}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var status map[string]any
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["role_budgets"].(map[string]any)["opus"] != float64(16384) {
		t.Fatal("budget not updated")
	}
}
