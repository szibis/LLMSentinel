package taskquality

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskConversationBothProtocols(t *testing.T) {
	for _, protocol := range []string{"messages", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			step := 0
			fixture := fixtures("random-marker")[0]
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				step++
				if step == 2 && !strings.Contains(toJSON(request), "random-marker") {
					t.Error("tool result missing from continuation")
				}
				var response any
				if protocol == "messages" {
					content := []any{map[string]any{"type": "tool_use", "id": "call1", "name": "read_fixture", "input": map[string]any{"path": fixture.Path}}}
					stop := "tool_use"
					if step == 2 {
						content = []any{map[string]any{"type": "text", "text": fixture.Expected}}
						stop = "end_turn"
					}
					response = map[string]any{"id": "msg1", "type": "message", "role": "assistant", "content": content, "stop_reason": stop, "usage": nil}
				} else {
					output := []any{map[string]any{"type": "function_call", "call_id": "call1", "name": "read_fixture", "arguments": toJSON(map[string]any{"path": fixture.Path})}}
					if step == 2 {
						output = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": fixture.Expected}}}}
					}
					response = map[string]any{"id": "resp1", "status": "completed", "output": output, "usage": nil}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			result := execute(context.Background(), server.Client(), server.URL, "sonnet", protocol, fixture, time.Second)
			if !result.Passed || result.Requests != 2 || result.ToolCalls != 1 || result.InputTokens != nil || len(result.Exchanges) != 2 {
				t.Fatalf("unexpected evidence: %+v", result)
			}
		})
	}
}

func TestRegressionsDoNotPass(t *testing.T) {
	for _, kind := range []string{"422", "promise", "wrong", "no-tool", "repeated", "truncated"} {
		t.Run(kind, func(t *testing.T) {
			f := fixtures("marker")[0]
			step := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				step++
				if kind == "422" {
					http.Error(w, "invalid tool-call format", http.StatusUnprocessableEntity)
					return
				}
				stop := "end_turn"
				answer := f.Expected
				if kind == "promise" {
					answer = "Let me search more broadly for Loki-related content."
				}
				if kind == "wrong" {
					answer = "1 marker"
				}
				if kind == "truncated" {
					stop = "max_tokens"
				}
				content := []any{map[string]any{"type": "text", "text": answer}}
				if kind != "no-tool" && (step == 1 || kind == "repeated") {
					stop = "tool_use"
					content = []any{map[string]any{"type": "tool_use", "id": "c", "name": "read_fixture", "input": map[string]any{"path": f.Path}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"content": content, "stop_reason": stop})
			}))
			defer s.Close()
			result := execute(context.Background(), s.Client(), s.URL, "sonnet", "messages", f, time.Second)
			if result.Passed || result.Failure == "" || result.Requests > 4 {
				t.Fatalf("regression accepted: %+v", result)
			}
		})
	}
}

func TestReportStorageAndMissingEvidence(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if Load(root) != nil {
		t.Fatal("invented report")
	}
	r := Report{Version: 1, Scope: "bounded-api-task-probes", FinishedAt: time.Now().UTC(), Results: []Result{{Task: "exact-read", Passed: false, Failure: "HTTP 422"}}}
	if err := Save(root, r); err != nil {
		t.Fatal(err)
	}
	loaded := Load(root)
	if loaded == nil || loaded.Results[0].Passed || loaded.Results[0].InputTokens != nil {
		t.Fatal("lost failure or unknown usage")
	}
}

func TestLocalEndpointOnly(t *testing.T) {
	for _, endpoint := range []string{"https://api.anthropic.com", "http://localhost:19090", "http://user@127.0.0.1:19090", "http://127.0.0.1:19090/path"} {
		if validEndpoint(endpoint) {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	if !validEndpoint("http://127.0.0.1:19090") {
		t.Fatal("local endpoint rejected")
	}
}

func TestCodingAcceptsEquivalentCorrections(t *testing.T) {
	f := fixtures("marker")[1]
	for _, expression := range []string{"a + b", "b + a", "(a) + (b)"} {
		if !matches(f, toJSON(map[string]any{"expression": expression})) {
			t.Errorf("correct fix rejected: %s", expression)
		}
	}
	for _, expression := range []string{"a - b", "42", "a + a", "a + b + 1"} {
		if matches(f, toJSON(map[string]any{"expression": expression})) {
			t.Errorf("incorrect fix accepted: %s", expression)
		}
	}
}

func TestUnknownUsageAcrossContinuation(t *testing.T) {
	for _, missingAt := range []int{1, 2} {
		step := 0
		f := fixtures("marker")[0]
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			step++
			usage := any(map[string]any{"input_tokens": 10, "output_tokens": 3})
			if step == missingAt {
				usage = map[string]any{"input_tokens": 0, "output_tokens": 0}
			}
			stop := "tool_use"
			content := []any{map[string]any{"type": "tool_use", "id": "c", "name": "read_fixture", "input": map[string]any{"path": f.Path}}}
			if step == 2 {
				stop = "end_turn"
				content = []any{map[string]any{"type": "text", "text": f.Expected}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"content": content, "stop_reason": stop, "usage": usage})
		}))
		result := execute(context.Background(), s.Client(), s.URL, "sonnet", "messages", f, time.Second)
		s.Close()
		if !result.Passed || result.InputTokens != nil || result.OutputTokens != nil {
			t.Fatalf("partial usage treated as total: %+v", result)
		}
	}
}

func TestFailedContinuationDoesNotExposePartialTotal(t *testing.T) {
	step := 0
	f := fixtures("marker")[0]
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		if step == 2 {
			http.Error(w, "invalid tool format", http.StatusUnprocessableEntity)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "c", "name": "read_fixture", "input": map[string]any{"path": f.Path}}}, "stop_reason": "tool_use", "usage": map[string]any{"input_tokens": 10, "output_tokens": 3}})
	}))
	defer s.Close()
	result := execute(context.Background(), s.Client(), s.URL, "sonnet", "messages", f, time.Second)
	if result.InputTokens != nil || result.OutputTokens != nil || result.Failure != "HTTP 422" {
		t.Fatalf("partial total retained: %+v", result)
	}
}

func TestVerboseFailureIsArchivedAndSummaryRemainsBounded(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := Report{Version: 1, Scope: "bounded-api-task-probes", FinishedAt: time.Now().UTC(), Results: []Result{{Task: "exact-read", Failure: "HTTP 422", Exchanges: []Exchange{{Response: json.RawMessage(toJSON(strings.Repeat("x", 1<<20)))}}}}}
	if err := Save(root, r); err != nil {
		t.Fatal(err)
	}
	loaded := Load(root)
	if loaded == nil || loaded.Results[0].Failure != "HTTP 422" || len(loaded.Results[0].Exchanges) != 0 {
		t.Fatal("missing bounded summary")
	}
	files, err := filepath.Glob(filepath.Join(root, "task-quality-runs", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("missing archive")
	}
	raw, err := os.ReadFile(files[0])
	if err != nil || len(raw) < 1<<20 {
		t.Fatal("lost verbose response evidence")
	}
}

func canonicalRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestRunRefusesUnsafeHealthBeforeInference(t *testing.T) {
	for _, kind := range []string{"paid", "hybrid", "unknown", "wrong-mode", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			posts := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					http.Error(w, "unexpected", http.StatusInternalServerError)
					return
				}
				if kind == "redirect" {
					http.Redirect(w, r, "http://127.0.0.1:1", http.StatusFound)
					return
				}
				mode := "serving"
				controls := map[string]any{"policy": "local-only", "startup_billing_opt_in": false}
				switch kind {
				case "paid":
					controls["startup_billing_opt_in"] = true
				case "hybrid":
					controls["policy"] = "balanced"
				case "unknown":
					delete(controls, "startup_billing_opt_in")
				case "wrong-mode":
					mode = "learning"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"mode": mode, "controls": controls})
			}))
			defer s.Close()
			var out, diagnostic strings.Builder
			code := Run([]string{"--root", canonicalRoot(t), "--endpoint", s.URL, "--task", "exact-read", "--protocol", "messages"}, nil, &out, &diagnostic)
			if code != 1 || posts != 0 {
				t.Fatalf("unsafe probe: code=%d posts=%d %s", code, posts, diagnostic.String())
			}
		})
	}
}
func TestRunArchivesActualTaskFailure(t *testing.T) {
	root := canonicalRoot(t)
	posts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(map[string]any{"mode": "serving", "controls": map[string]any{"policy": "local-only", "startup_billing_opt_in": false}})
			return
		}
		posts++
		http.Error(w, "invalid tool-call format", http.StatusUnprocessableEntity)
	}))
	defer s.Close()
	var out, diagnostic strings.Builder
	code := Run([]string{"--root", root, "--endpoint", s.URL, "--task", "exact-read", "--protocol", "responses"}, nil, &out, &diagnostic)
	report := Load(root)
	if code != 1 || posts != 1 || report == nil || report.Results[0].Failure != "HTTP 422" {
		t.Fatalf("failure lost: code=%d report=%+v %s", code, report, diagnostic.String())
	}
}
func TestResponsesRegressions(t *testing.T) {
	for _, kind := range []string{"incomplete", "invalid-arguments", "repeat", "wrong-final", "promise"} {
		t.Run(kind, func(t *testing.T) {
			f := fixtures("marker")[0]
			step := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				step++
				status := "completed"
				args := toJSON(map[string]any{"path": f.Path})
				if kind == "invalid-arguments" {
					args = "broken"
				}
				output := []any{map[string]any{"type": "function_call", "call_id": "c", "name": "read_fixture", "arguments": args}}
				if kind == "incomplete" {
					status = "incomplete"
				}
				if step == 2 && kind != "repeat" {
					answer := "wrong"
					if kind == "promise" {
						answer = "Let me look at the broader project."
					}
					output = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": answer}}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "output": output})
			}))
			defer s.Close()
			result := execute(context.Background(), s.Client(), s.URL, "sonnet", "responses", f, time.Second)
			if result.Passed || result.Failure == "" {
				t.Fatalf("regression accepted: %+v", result)
			}
		})
	}
}
func TestOversizedResponseKeepsExplicitExcerpt(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", (1<<20)+1))) }))
	defer s.Close()
	result := execute(context.Background(), s.Client(), s.URL, "sonnet", "messages", fixtures("marker")[0], time.Second)
	if result.Passed || len(result.Exchanges) != 1 || !strings.Contains(string(result.Exchanges[0].Response), "\"truncated\":true") || len(result.Exchanges[0].Response) > 5000 {
		t.Fatalf("oversized evidence lost: %+v", result)
	}
}
