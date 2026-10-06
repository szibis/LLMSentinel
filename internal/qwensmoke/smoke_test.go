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
	for name, value := range map[string]string{"config.json": "{}", "tokenizer.json": "{}", "tokenizer_config.json": `{"chat_template":"enable_thinking"}`, "weights.safetensors": "weights"} { // #nosec G101 -- Public synthetic tokenizer metadata, no credentials.
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
