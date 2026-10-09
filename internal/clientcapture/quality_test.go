package clientcapture

import (
	"bytes"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failOutput struct{}

func (failOutput) Write([]byte) (int, error) { return 0, errors.New("writer closed") }

func TestCaptureFileAndNormalizationBounds(t *testing.T) {
	path := privatePath(t, "private.jsonl")
	for _, record := range []map[string]any{{"bad": make(chan int)}, {"large": strings.Repeat("x", maxRecordBytes)}} {
		if err := appendPrivate(path, record); err == nil {
			t.Fatal("invalid record accepted")
		}
	}
	if err := appendPrivate(filepath.Join(filepath.Dir(path), "missing", "file"), map[string]any{"ok": true}); err == nil {
		t.Fatal("missing parent accepted")
	}
	if err := appendPrivate(filepath.Dir(path), map[string]any{"ok": true}); err == nil {
		t.Fatal("directory accepted")
	}
	if _, _, err := readTranscript(path, true); err == nil {
		t.Fatal("missing transcript accepted")
	}
	os.WriteFile(path, []byte(`{"type":"session_meta"}`+"\n"), 0600)
	rows, truncated, err := readTranscript(path, true)
	if err != nil || truncated || len(rows) != 2 {
		t.Fatalf("header: %v %v %v", rows, truncated, err)
	}
	if _, ok := checkedAdd(math.MaxInt64, 1); ok {
		t.Fatal("overflow accepted")
	}
	if _, err := normalize("codex", "direct_unknown", map[string]any{"type": "agent-turn-complete", "input-messages": []any{1}}); err == nil {
		t.Fatal("invalid message accepted")
	}
	if _, err := normalize("codex", "api", map[string]any{"type": "agent-turn-complete", "input-messages": "bad"}); err == nil {
		t.Fatal("invalid messages accepted")
	}
	event := map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s", "prompt": strings.Repeat("x", maxText+1)}
	record, err := normalize("claude", "subscription", event)
	if err != nil || record["truncated"] != true {
		t.Fatal(record, err)
	}
	event = map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": 5}
	if _, err := normalize("claude", "direct_unknown", event); err == nil {
		t.Fatal("invalid prompt accepted")
	}
	inputs := make([]any, maxRows+1)
	for i := range inputs {
		inputs[i] = "synthetic"
	}
	record, err = normalize("codex", "api", map[string]any{"type": "agent-turn-complete", "input-messages": inputs})
	if err != nil || record["truncated"] != true || len(record["inputs"].([]string)) != 32 {
		t.Fatal(record, err)
	}
}

func TestCaptureCollectorAndPreviewFailures(t *testing.T) {
	for _, address := range []string{":bad", "http://127.0.0.1:0", "http://[::1]:65536"} {
		if err := collectorAddress(address); err == nil {
			t.Fatal(address)
		}
	}
	if deliver("http://127.0.0.1", map[string]any{"bad": make(chan int)}) || deliver(":bad", map[string]any{}) {
		t.Fatal("invalid delivery accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	if deliver(server.URL, map[string]any{"synthetic": true}) {
		t.Fatal("failed collector accepted")
	}
	for _, client := range []string{"claude", "codex"} {
		if err := previewConfig(failOutput{}, client, privatePath(t, "out"), server.URL, "subscription", true); err == nil {
			t.Fatal("output error hidden")
		}
	}
	var out, stderr bytes.Buffer
	args := []string{"--client", "codex", "--output", privatePath(t, "capture.jsonl"), "--preview-config"}
	if Run(args, nil, failOutput{}, &stderr) != 1 {
		t.Fatal("failed preview accepted")
	}
	args = []string{"--client", "codex", "--output", privatePath(t, "capture.jsonl"), "--collector", server.URL}
	raw := `{"type":"agent-turn-complete","last-assistant-message":"synthetic"}`
	if Run(args, strings.NewReader(raw), &out, &stderr) != 0 || !strings.Contains(stderr.String(), "spool retained") {
		t.Fatal(stderr.String())
	}
}
