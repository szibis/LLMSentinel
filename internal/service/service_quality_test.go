package service

import (
	"encoding/json"
	"errors"
	"github.com/szibis/claude-escalate/internal/budgets"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/intent"
	"github.com/szibis/claude-escalate/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type serviceRoundTrip func(*http.Request) (*http.Response, error)

func (f serviceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func qualityService(t *testing.T) *Service {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"model":"claude-sonnet-4-6","effortLevel":"medium","other":"keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Gateway.DataDir = t.TempDir()
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	s.escCfg.Sentiment.Enabled = false
	s.escCfg.Budgets.DailyUSD = 0
	return s
}

func call(t *testing.T, h http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}
func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode %d %q: %v", w.Code, w.Body.String(), err)
	}
	return data
}

func TestSupportedModelNames(t *testing.T) {
	for _, c := range []struct{ in, want string }{{"opus", "opus"}, {"sonnet", "sonnet"}, {"haiku", "haiku"}, {"claude-opus-4-7", "opus"}, {"claude-sonnet-4-6", "sonnet"}, {"claude-haiku-4-5-20251001", "haiku"}, {"claude-3-5-sonnet-20241022", "sonnet"}, {"unknown", "haiku"}} {
		if got := modelShortName(c.in); got != c.want {
			t.Errorf("modelShortName(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestServiceHTTPValidationAndReadEndpoints(t *testing.T) {
	s := qualityService(t)
	for _, c := range []struct {
		handler http.HandlerFunc
		body    string
	}{{s.handleValidate, `{"actual_input_tokens":80,"actual_output_tokens":20,"actual_total_tokens":100,"actual_cost":0.1}`}, {s.handleHookMetrics, `{"prompt":"hello","detected_effort":"low","routed_model":"haiku","estimated_total_tokens":200,"estimated_cost":0.2}`}} {
		w := call(t, c.handler, "POST", "/", c.body)
		if w.Code != 200 || decodeResponse(t, w)["success"] != true {
			t.Fatalf("response: %d %s", w.Code, w.Body)
		}
		if decodeResponse(t, w)["validation_id"].(float64) <= 0 {
			t.Fatalf("invalid validation ID: %s", w.Body)
		}
	}
	if err := s.db.LogValidationMetric(store.ValidationMetric{RoutedModel: "sonnet", DetectedEffort: "medium", EstimatedTotalTokens: 200, ActualTotalTokens: 100, EstimatedCost: .2, ActualCost: .1, TokenError: -50, Validated: true}); err != nil {
		t.Fatal(err)
	}
	for _, h := range []http.HandlerFunc{s.handleStats, s.handleHealth, s.handleValidationMetrics, s.handleValidationStats, s.handleStatusline, s.handleDecisionLearning, s.handleUserAnalytics} {
		w := call(t, h, "GET", "/", "")
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		decodeResponse(t, w)
	}
	w := call(t, s.handleValidationMetrics, "GET", "/", "")
	if decodeResponse(t, w)["count"] != float64(3) {
		t.Fatal(w.Body)
	}
	w = call(t, s.handleStatusline, "GET", "/", "")
	data := decodeResponse(t, w)
	if data["model"] != "sonnet" {
		t.Fatal(data)
	}
	w = call(t, s.handleDetectSignal, "POST", "/", `{"text":"/escalate"}`)
	if decodeResponse(t, w)["signal_type"] != "escalation" {
		t.Fatal(w.Body)
	}
	body, _ := json.Marshal(MakeDecisionRequest{ValidationID: strconv.Itoa(3), Signal: DetectSignalResponse{SignalType: SignalSuccess, Confidence: 1}})
	w = call(t, s.handleMakeDecision, "POST", "/", string(body))
	if w.Code != 200 || decodeResponse(t, w)["action"] != "cascade" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = call(t, s.handleMakeDecision, "POST", "/", `{"validation_id":"missing"}`)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = call(t, s.handleDashboard, "GET", "/", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Dashboard") {
		t.Fatal(w.Body)
	}
	if call(t, s.handleDashboard, "GET", "/missing", "").Code != 404 {
		t.Fatal("unknown dashboard route")
	}
}

func TestServiceRejectsWrongMethodsAndJSON(t *testing.T) {
	s := qualityService(t)
	for _, h := range []http.HandlerFunc{s.handleHook, s.handleEscalate, s.handleDeescalate, s.handleEffort, s.handleValidate, s.handleHookMetrics, s.handleDetectSignal, s.handleMakeDecision, s.handleFeedback} {
		if w := call(t, h, "GET", "/", ""); w.Code != 405 {
			t.Fatal(w.Code)
		}
		if w := call(t, h, "POST", "/", "{"); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	for _, h := range []http.HandlerFunc{s.handleValidationMetrics, s.handleValidationStats, s.handleStatusline, s.handleDecisionLearning, s.handleUserAnalytics} {
		if w := call(t, h, "POST", "/", ""); w.Code != 405 {
			t.Fatal(w.Code)
		}
	}
	for _, rating := range []string{"0", "6"} {
		if w := call(t, s.handleFeedback, "POST", "/", `{"rating":`+rating+`}`); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := call(t, s.handleFeedback, "POST", "/", `{"request_id":"r1","rating":5,"helpful":true,"accurate":true}`)
	if decodeResponse(t, w)["acknowledged"] != true {
		t.Fatal(w.Body)
	}
}

func TestServiceModelMutationEndpoints(t *testing.T) {
	s := qualityService(t)
	for _, target := range []string{"", "opus", "sonnet", "haiku"} {
		w := call(t, s.handleEscalate, "POST", "/", `{"target":"`+target+`"}`)
		if decodeResponse(t, w)["success"] != true {
			t.Fatal(w.Body)
		}
		expected := target
		if expected == "" {
			expected = "sonnet"
		}
		settings, err := config.ReadClaudeSettings()
		if err != nil || settings.Model != modelToFull(expected) {
			t.Fatalf("settings: %+v %v", settings, err)
		}
	}
	for _, level := range []string{"high", "medium", "low", "unknown"} {
		w := call(t, s.handleEffort, "POST", "/", `{"level":"`+level+`"}`)
		if decodeResponse(t, w)["model"] != effortToModel(level) {
			t.Fatal(w.Body)
		}
	}
	config.WriteClaudeSettings(modelToFull("opus"), "high")
	for _, want := range []string{"sonnet", "haiku", "haiku"} {
		w := call(t, s.handleDeescalate, "POST", "/", `{"reason":"success"}`)
		if decodeResponse(t, w)["model"] != want {
			t.Fatal(w.Body)
		}
	}
	os.Remove(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")) // #nosec G703 -- qualityService sets HOME to t.TempDir before this fixed fixture path is used.
	w := call(t, s.handleDeescalate, "POST", "/", `{}`)
	if decodeResponse(t, w)["error"] != "no current model" {
		t.Fatal(w.Body)
	}
	for _, prompt := range []string{"/escalate to opus", "thanks", "architecture redesign", "quick help", "ordinary prompt"} {
		w := call(t, s.handleHook, "POST", "/", `{"prompt":"`+prompt+`"}`)
		if decodeResponse(t, w)["continue"] != true {
			t.Fatal(w.Body)
		}
	}
}

func TestHookExplicitRoutingTakesPrecedence(t *testing.T) {
	s := qualityService(t)
	for _, c := range []struct{ prompt, current, want, action string }{{"/escalate to opus", "haiku", "opus", "escalate"}, {"thanks", "opus", "sonnet", "deescalate"}} {
		if err := config.WriteClaudeSettings(modelToFull(c.current), "medium"); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(HookRequest{Prompt: c.prompt})
		w := call(t, s.handleHook, "POST", "/", string(body))
		data := decodeResponse(t, w)
		settings, err := config.ReadClaudeSettings()
		if err != nil {
			t.Fatal(err)
		}
		if data["action"] != c.action || data["currentModel"] != c.want || settings.Model != modelToFull(c.want) {
			t.Errorf("prompt %q: response %+v settings %+v, want %s", c.prompt, data, settings, c.want)
		}
	}
}

func TestClosedDatabaseFailures(t *testing.T) {
	s := qualityService(t)
	s.db.Close()
	for _, c := range []struct {
		h            http.HandlerFunc
		method, body string
	}{{s.handleValidate, "POST", `{}`}, {s.handleHookMetrics, "POST", `{}`}, {s.handleValidationMetrics, "GET", ""}, {s.handleValidationStats, "GET", ""}, {s.handleDecisionLearning, "GET", ""}} {
		w := call(t, c.h, c.method, "/", c.body)
		if w.Code != 500 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	for _, h := range []http.HandlerFunc{s.handleHook, s.handleEscalate, s.handleDeescalate} {
		w := call(t, h, "POST", "/", `{"prompt":"/escalate to opus"}`)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
}

type disconnectedWriter struct{ header http.Header }

func (w *disconnectedWriter) Header() http.Header       { return w.header }
func (w *disconnectedWriter) WriteHeader(int)           {}
func (w *disconnectedWriter) Write([]byte) (int, error) { return 0, errors.New("client disconnected") }
func TestDisconnectedHTTPClients(t *testing.T) {
	s := qualityService(t)
	for _, h := range []http.HandlerFunc{s.handleHook, s.handleEscalate, s.handleDeescalate, s.handleEffort, s.handleStats, s.handleHealth, s.handleDashboard, s.handleValidate, s.handleHookMetrics, s.handleValidationMetrics, s.handleValidationStats, s.handleStatusline, s.handleDetectSignal, s.handleDecisionLearning} {
		method := "POST"
		if h == nil {
			t.Fatal("nil handler")
		}
		h(&disconnectedWriter{make(http.Header)}, httptest.NewRequest(method, "/", strings.NewReader(`{}`)))
	}
}

func TestFeedbackIdentityAndAggregateCalculations(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "local"
	if extractUserID(r) != "local" {
		t.Fatal("IP")
	}
	r.Header.Set("Authorization", "token")
	if extractUserID(r) != "token" {
		t.Fatal("auth")
	}
	r.Header.Set("X-User-ID", "user")
	if extractUserID(r) != "user" {
		t.Fatal("user")
	}
	p := &intent.UserFeedbackPattern{PositiveFeedbackCount: 3, NegativeFeedbackCount: 1, RecentAccuracy: .75}
	if calculateAverageRating(p) != 3.5 || calculateHelpfulPercentage(p) != 75 || calculateAccuracyPercentage(p) != 75 {
		t.Fatal("aggregates")
	}
	p = &intent.UserFeedbackPattern{}
	if calculateAverageRating(p) != 0 || calculateHelpfulPercentage(p) != 0 || calculateAccuracyPercentage(p) != 100 {
		t.Fatal("empty")
	}
	for _, c := range []struct{ model, want string }{{"haiku", "sonnet"}, {"sonnet", "opus"}, {"opus", "opus"}, {"unknown", "sonnet"}} {
		if escalateByOne(c.model) != c.want {
			t.Fatal(c)
		}
	}
	if isEscalateCommand("short") || extractEscalateTarget("/escalate") != "sonnet" || !containsAny("some architecture", []string{"architecture"}) || containsAny("x", []string{"long"}) {
		t.Fatal("prompt helpers")
	}
	if detectEffort(strings.Repeat("x", 260)) != "high" || modelToFull("invalid") != modelToFull("haiku") {
		t.Fatal("defaults")
	}
}

func TestServiceConstructionFailureAndServerRegistration(t *testing.T) {
	s := qualityService(t)
	if err := s.Start("bad address"); err == nil {
		t.Fatal("invalid address accepted")
	}
	cfg := &config.Config{}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("data"), 0600)
	cfg.Gateway.DataDir = file
	if _, err := New(cfg); err == nil {
		t.Fatal("file data dir accepted")
	}
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".claude", "escalation"), 0700); err != nil { // #nosec G703 -- qualityService sets HOME to t.TempDir before this fixed fixture path is used.
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(os.Getenv("HOME"), ".claude", "escalation", "config.yaml"), []byte("[invalid"), 0600) // #nosec G703 -- qualityService sets HOME to t.TempDir before this fixed fixture path is used.
	cfg.Gateway.DataDir = t.TempDir()
	fallback, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fallback.db.Close()
}

func TestHookConfiguredFrustrationAndBudgetActions(t *testing.T) {
	s := qualityService(t)
	s.escCfg.Sentiment.Enabled = true
	s.escCfg.Sentiment.FrustrationTriggerEscalate = true
	s.escCfg.Sentiment.FrustrationRiskThreshold = 0
	prompt := "I'm frustrated. This is broken and wrong, not working at all!"
	if s.sentimentDetector.Detect(prompt, false, 0).FrustrationRisk <= 0 {
		t.Fatal("fixture has no frustration signal")
	}
	body, _ := json.Marshal(HookRequest{Prompt: prompt})
	w := call(t, s.handleHook, "POST", "/", string(body))
	if decodeResponse(t, w)["action"] != "escalate_on_frustration" {
		t.Fatal(w.Body)
	}
	s.escCfg.Sentiment.Enabled = false
	s.escCfg.Budgets.DailyUSD = .001
	s.budgetEngine = budgets.NewEngine(budgets.BudgetConfig{DailyBudgetUSD: .001, MonthlyBudgetUSD: 10, HardLimit: true})
	if err := config.WriteClaudeSettings(modelToFull("opus"), "high"); err != nil {
		t.Fatal(err)
	}
	w = call(t, s.handleHook, "POST", "/", `{"prompt":"ordinary task"}`)
	data := decodeResponse(t, w)
	if data["action"] != "downgrade_for_budget" || data["message"] == "" {
		t.Fatal(data)
	}
}

func TestLocalHTTPMetricToDecisionWorkflow(t *testing.T) {
	s := qualityService(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/metrics/hook", s.handleHookMetrics)
	mux.HandleFunc("/api/decisions/make", s.handleMakeDecision)
	mux.HandleFunc("/api/validation/metrics", s.handleValidationMetrics)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := server.Client().Post(server.URL+"/api/metrics/hook", "application/json", strings.NewReader(`{"prompt":"quick help","detected_effort":"low","routed_model":"haiku","estimated_total_tokens":50}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("metric: %d %s %v", resp.StatusCode, data, err)
	}
	var metric struct {
		ID int64 `json:"validation_id"`
	}
	if err := json.Unmarshal(data, &metric); err != nil || metric.ID <= 0 {
		t.Fatalf("metric ID: %+v %v", metric, err)
	}
	request, _ := json.Marshal(MakeDecisionRequest{ValidationID: strconv.FormatInt(metric.ID, 10), Signal: DetectSignalResponse{SignalType: SignalEscalation, Confidence: 1}})
	resp, err = server.Client().Post(server.URL+"/api/decisions/make", "application/json", strings.NewReader(string(request)))
	if err != nil {
		t.Fatal(err)
	}
	var decision MakeDecisionResponse
	err = json.NewDecoder(resp.Body).Decode(&decision)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || decision.Action != "escalate" || decision.NextModel != "opus" || !decision.EscalateAvailable {
		t.Fatalf("decision: %+v status %d err %v", decision, resp.StatusCode, err)
	}
	resp, err = server.Client().Get(server.URL + "/api/validation/metrics")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Metrics []store.ValidationMetric `json:"metrics"`
	}
	err = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if err != nil || len(list.Metrics) != 1 || list.Metrics[0].ID != metric.ID || list.Metrics[0].Prompt != "quick help" {
		t.Fatalf("persisted metrics: %+v %v", list, err)
	}
}
