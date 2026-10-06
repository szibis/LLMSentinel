package qwensmoke

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeEOSAtBudgetRequiresResponseMarker(t *testing.T) {
	for _, tc := range []struct {
		native bool
		count  int
		finish string
		want   bool
	}{{true, 256, "stop", true}, {false, 256, "stop", false}, {true, 256, "length", false}, {true, 257, "stop", false}, {false, 255, "stop", true}} {
		body := fmt.Sprintf(`{"choices":[{"finish_reason":%q,"message":{"content":"QWEN_ROLE_READY"}}],"usage":{"completion_tokens":%d},"mlx_flash_compress":{"native_generation_metadata":%t}}`, tc.finish, tc.count, tc.native)
		var result document
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatal(err)
		}
		if _, err := validateChatResult(result, 256, false); (err == nil) != tc.want {
			t.Fatal(body, err)
		}
	}
}

func TestExactMarkerRequiresCompletedReasoning(t *testing.T) {
	if err := finalText("reasoning</think>"+marker, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text     string
		thinking bool
	}{{marker, true}, {marker + " extra", false}, {"wrong", false}} {
		if err := finalText(tc.text, tc.thinking); err == nil {
			t.Fatal("accepted invalid marker", tc)
		}
	}
}

func TestGemmaFinalMarkerRequiresCompleteThoughtChannel(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"<|channel>thought\nprivate\n<channel|>" + marker, true},
		{"private\n<channel|>" + marker, true},
		{"<|channel>thought\nprivate", false},
		{"<|channel>thought\nprivate\n<channel|>" + marker + "<|channel>thought", false},
	} {
		if err := finalText(tc.value, true); (err == nil) != tc.valid {
			t.Fatalf("%q: %v", tc.value, err)
		}
	}
}

func TestCachedModelSupportsActualFamilyTemplates(t *testing.T) {
	for _, tc := range []struct {
		family, template string
		valid            bool
	}{
		{"lfm2_moe", "<think></think>", true},
		{"gemma4", "enable_thinking <|channel>thought\n<channel|>", true},
		{"qwen3_5", "enable_thinking <think></think>", true},
		{"unsupported", "enable_thinking <think></think>", false},
		{"gemma4", "enable_thinking <think></think>", false},
	} {
		path := t.TempDir()
		for name, value := range map[string]string{"config.json": fmt.Sprintf(`{"model_type":%q}`, tc.family), "tokenizer.json": "{}", "tokenizer_config.json": fmt.Sprintf(`{"chat_template":%q}`, tc.template), "weights.safetensors": "weights"} {
			if err := os.WriteFile(filepath.Join(path, name), []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := validateModel(path); (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.family, err)
		}
	}
}

func TestRuntimeProfilesMatchCachedModelAndNativeCapabilities(t *testing.T) {
	for _, tc := range []struct {
		family, body string
		valid        bool
		count        int
	}{
		{"lfm2_moe", `{"capabilities":{"model_family":"lfm2_moe","thinking_control":false,"reasoning_format":"think","chat_template_kwargs":[]}}`, true, 1},
		{"gemma4", `{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"gemma","chat_template_kwargs":["enable_thinking"]}}`, true, 2},
		{"qwen3_5", `{"capabilities":{"chat_template_kwargs":["enable_thinking"]}}`, true, 2},
		{"gemma4", `{"capabilities":{"model_family":"lfm2_moe","thinking_control":false,"reasoning_format":"think"}}`, false, 0},
		{"gemma4", `{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"think","chat_template_kwargs":["enable_thinking"]}}`, false, 0},
		{"lfm2_moe", `{"capabilities":{"model_family":"lfm2_moe","thinking_control":true,"reasoning_format":"think"}}`, false, 0},
		{"unknown", `{"capabilities":{"model_family":"unknown","thinking_control":true,"reasoning_format":"think"}}`, false, 0},
	} {
		var health document
		if err := json.Unmarshal([]byte(tc.body), &health); err != nil {
			t.Fatal(err)
		}
		profile, err := runtimeProfile(health, tc.family)
		if (err == nil) != tc.valid {
			t.Fatalf("%s health %s: %v", tc.family, tc.body, err)
		}
		if err == nil && len(profile.requests()) != tc.count {
			t.Fatalf("wrong profile count: %v", profile.requests())
		}
	}
}

func TestNativeChatUsesOnlySupportedThinkingControls(t *testing.T) {
	for _, family := range []string{"lfm2_moe", "gemma4", "qwen3_5"} {
		profile := nativeProfile{Family: family, ThinkingControl: family != "lfm2_moe", ReasoningFormat: "think"}
		if family == "gemma4" {
			profile.ReasoningFormat = "gemma"
		}
		for _, thinking := range profile.requests() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload document
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if family == "lfm2_moe" {
					if _, ok := payload["chat_template_kwargs"]; ok {
						t.Error("LFM received a nonexistent toggle")
					}
				} else if mapping(payload["chat_template_kwargs"])["enable_thinking"] != thinking {
					t.Error("thinking profile lost")
				}
				answer := marker
				if family == "lfm2_moe" || thinking {
					answer = "private</think>" + marker
					if family == "gemma4" {
						answer = "<|channel>thought\nprivate\n<channel|>" + marker
					}
				}
				json.NewEncoder(w).Encode(document{"choices": []any{document{"finish_reason": "stop", "message": document{"content": answer}}}, "usage": document{"completion_tokens": 3}})
			}))
			_, err := generateChatAt(context.Background(), server.URL, thinking, profile)
			server.Close()
			if err != nil {
				t.Fatalf("%s %t: %v", family, thinking, err)
			}
		}
	}
}

func TestGatewayRoleMarkerRejectsLeakedFamilyReasoning(t *testing.T) {
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		for _, answer := range []string{marker, "<think>private</think>" + marker, "<|channel>thought\nprivate\n<channel|>" + marker} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(document{"stop_reason": "end_turn", "content": []any{document{"type": "text", "text": answer}}})
			}))
			_, err := generateRole(context.Background(), server.URL, role, false)
			server.Close()
			if (err == nil) != (answer == marker) {
				t.Fatalf("%s leaked=%t: %v", role, answer != marker, err)
			}
		}
	}
}

func TestFamilyChatResultsRejectTruncationAndWrongReasoning(t *testing.T) {
	for _, tc := range []struct {
		answer, format, finish string
		count                  int
		valid                  bool
	}{
		{"<|channel>thought\nprivate\n<channel|>" + marker, "gemma", "stop", 10, true},
		{"private</think>" + marker, "think", "stop", 10, true},
		{marker, "think", "stop", 10, false},
		{"<think>private", "think", "stop", 10, false},
		{"<|channel>thought\nprivate", "gemma", "stop", 10, false},
		{"private</think>" + marker, "gemma", "stop", 10, false},
		{"<|channel>thought\nprivate\n<channel|>" + marker, "gemma", "length", 10, false},
		{"private</think>" + marker, "think", "stop", 256, false},
	} {
		body := fmt.Sprintf(`{"choices":[{"finish_reason":%q,"message":{"content":%q}}],"usage":{"completion_tokens":%d}}`, tc.finish, tc.answer, tc.count)
		var result document
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatal(err)
		}
		if _, err := validateChatResult(result, 256, true, tc.format); (err == nil) != tc.valid {
			t.Fatalf("%s: %v", body, err)
		}
	}
}

func TestReadinessAndHTTPAreBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	start := time.Now()
	if _, err := request(context.Background(), server.URL, nil, 50*time.Millisecond); err == nil || time.Since(start) > time.Second {
		t.Fatal("request timeout is unbounded", err)
	}
	if _, err := request(context.Background(), "http://localhost:1/health", nil, time.Second); err == nil {
		t.Fatal("DNS name accepted")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com", http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := request(context.Background(), redirect.URL, nil, time.Second); err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestPortPreflightPreservesLiveListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	if err = preflightPorts([]int{port}); err == nil {
		t.Fatal("occupied port accepted")
	}
	_ = listener.Close()
	if err = preflightPorts([]int{port}); err != nil {
		t.Fatal(err)
	}
}

func TestModelMissingShardAndThinkingTemplate(t *testing.T) {
	path := t.TempDir()
	for name, value := range map[string]string{"config.json": `{"model_type":"qwen3_5"}`, "tokenizer.json": "{}", "tokenizer_config.json": `{"chat_template":"enable_thinking"}`, "weights.safetensors": "weights"} { // #nosec G101 -- Public synthetic tokenizer metadata, no credentials.
		if err := os.WriteFile(filepath.Join(path, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateModel(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "model.safetensors.index.json"), []byte(`{"weight_map":{"a":"missing.safetensors"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateModel(path); err == nil || !strings.Contains(err.Error(), "shards") {
		t.Fatal(err)
	}
}

func TestToolMarkerSchemaAndFinish(t *testing.T) {
	for _, body := range []string{`{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"record_marker","input":{"marker":"QWEN_ROLE_READY"}}],"usage":{"input_tokens":8}}`, `{"stop_reason":"end_turn","content":[]}`, `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"record_marker","input":{"marker":"QWEN_ROLE_READY","extra":true}}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		_, err := generateRole(context.Background(), server.URL, "sonnet", true)
		server.Close()
		if (err == nil) != strings.Contains(body, `"input_tokens":8`) {
			t.Fatal("tool acceptance incorrect", body, err)
		}
	}
}

func TestLogsSanitizePaths(t *testing.T) {
	raw := t.TempDir()
	artifacts := t.TempDir()
	secret := filepath.Join(t.TempDir(), "model")
	t.Setenv("QWEN_SMALL_MODEL_PATH", secret)
	if err := os.WriteFile(filepath.Join(raw, "server.log"), []byte(secret+" "+raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sanitizedLogs(raw, artifacts); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(artifacts, "server.log"))
	if err != nil || strings.Contains(string(data), secret) || strings.Contains(string(data), raw) {
		t.Fatal(string(data), err)
	}
}
