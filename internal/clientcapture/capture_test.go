package clientcapture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func privatePath(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, name)
}

func invoke(t *testing.T, client string, event any, extra ...string) (map[string]any, string) {
	t.Helper()
	path := privatePath(t, "capture.jsonl")
	args := append([]string{"--client", client, "--output", path}, extra...)
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Run(args, bytes.NewReader(raw), &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	if out.Len() != 0 {
		t.Fatalf("hook output injected: %s", &out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatal(err)
	}
	return record, stderr.String()
}

func object(v any) map[string]any { value, _ := v.(map[string]any); return value }

func writeRows(t *testing.T, rows []map[string]any) string {
	t.Helper()
	path := privatePath(t, "transcript.jsonl")
	var b bytes.Buffer
	for _, row := range rows {
		if err := json.NewEncoder(&b).Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNotifyUnknownUsageRedactionAndArgv(t *testing.T) {
	event := map[string]any{"type": "agent-turn-complete", "thread-id": "s", "turn-id": "t", "input-messages": []string{"api_key=secret-value", `{"api_key": "json-secret"}`}, "last-assistant-message": "Bearer abcdefghijklmnop"}
	record, _ := invoke(t, "codex", event)
	raw, _ := json.Marshal(record)
	for _, secret := range []string{"secret-value", "json-secret", "abcdefghijklmnop"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("secret leaked: %s", secret)
		}
	}
	if record["session_id"] != "s" || record["turn_id"] != "t" || record["model"] != nil || object(record["usage"])["input_tokens"] != nil || object(record["quality"])["training_eligible"] != false {
		t.Fatalf("wrong unknown/provenance: %s", raw)
	}
	path := privatePath(t, "argv.jsonl")
	rawEvent, _ := json.Marshal(event)
	var out, stderr bytes.Buffer
	if Run([]string{"--client", "codex", "--output", path, string(rawEvent)}, strings.NewReader("not JSON"), &out, &stderr) != 0 {
		t.Fatal(stderr.String())
	}
}

func TestClaudeLatestTurnTextBlocksUsageAndLatency(t *testing.T) {
	rows := []map[string]any{
		{"type": "user", "uuid": "old", "message": map[string]any{"content": "old prompt"}},
		{"type": "assistant", "message": map[string]any{"id": "old", "content": "old answer", "usage": map[string]any{"input_tokens": 90}}},
		{"type": "user", "uuid": "new", "timestamp": "2026-10-06T00:00:00Z", "message": map[string]any{"content": "new prompt"}},
	}
	for _, text := range []string{"first", "second", "second"} {
		rows = append(rows, map[string]any{"type": "assistant", "timestamp": "2026-10-06T00:00:02Z", "message": map[string]any{"id": "m1", "model": "fixture-model", "content": []map[string]any{{"type": "text", "text": text}}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 3}}})
	}
	record, _ := invoke(t, "claude", map[string]any{"hook_event_name": "Stop", "session_id": "s", "transcript_path": writeRows(t, rows)})
	if fmt.Sprint(record["inputs"]) != "[new prompt]" || fmt.Sprint(record["outputs"]) != "[first second]" || record["turn_id"] != "new" || record["latency_ms"] != float64(2000) || object(record["usage"])["input_tokens"] != float64(10) {
		t.Fatalf("incorrect turn: %#v", record)
	}
}

func codexRows(version string, durable bool) []map[string]any {
	usage := map[string]any{"input_tokens": 100, "cached_input_tokens": 60, "output_tokens": 20, "reasoning_output_tokens": 5, "total_tokens": 120}
	rows := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "s", "session_id": "s", "cli_version": version, "model_provider": "openai"}},
		{"type": "event_msg", "payload": map[string]any{"type": "task_started", "turn_id": "t"}},
		{"type": "turn_context", "payload": map[string]any{"turn_id": "t", "model": "fixture-model"}},
		{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "fixture input"}}}},
		{"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "fixture output"}}}},
		{"type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": usage, "total_token_usage": usage}}},
	}
	if durable {
		row := map[string]any{"type": "token_usage_record", "payload": map[string]any{"thread_id": "s", "session_id": "s", "turn_id": "t", "response_id": "r", "usage": usage}}
		rows = append(rows, row, row)
	}
	return rows
}

func TestCodexRolloutIdentityVersionScopesAndBilling(t *testing.T) {
	for _, tc := range []struct {
		version, session, turn, scope, source string
		durable, known                        bool
	}{
		{"0.160.1", "s", "t", "observed_turn_calls", "codex_rollout_token_usage_record", true, true},
		{"0.160.1", "s", "t", "last_call", "codex_rollout_token_count", false, true},
		{"future", "s", "t", "", "", true, false},
		{"0.160.1", "other", "t", "", "", true, false},
		{"0.160.1", "s", "other", "", "", true, false},
	} {
		t.Run(tc.version+tc.session+tc.turn+tc.scope, func(t *testing.T) {
			record, _ := invoke(t, "codex", map[string]any{"hook_event_name": "Stop", "session_id": tc.session, "turn_id": tc.turn, "last_assistant_message": "fallback", "transcript_path": writeRows(t, codexRows(tc.version, tc.durable))}, "--billing-class", "subscription")
			u := object(record["usage"])
			if tc.known {
				if u["input_tokens"] != float64(100) || u["output_tokens"] != float64(20) || u["reasoning_output_tokens"] != float64(5) || u["total_tokens"] != float64(120) || u["cache_read_input_tokens"] != float64(60) || u["scope"] != tc.scope || u["source"] != tc.source {
					t.Fatalf("usage %#v", u)
				}
				if tc.durable && u["reported_calls"] != float64(1) {
					t.Fatalf("duplicate response counted: %#v", u)
				}
				if fmt.Sprint(record["inputs"]) != "[fixture input]" {
					t.Fatal(record)
				}
			} else if u["input_tokens"] != nil {
				t.Fatalf("invented usage: %#v", u)
			}
			if object(record["billing"])["class"] != "subscription" || object(record["billing"])["cost_usd"] != nil {
				t.Fatal("invented billing")
			}
		})
	}
}

func TestMetadataRetainedAcrossTailAndInvalidUsageUnknown(t *testing.T) {
	rows := codexRows("0.160.1", true)
	padding := []map[string]any{rows[0]}
	for range 600 {
		padding = append(padding, map[string]any{"type": "event_msg", "payload": map[string]any{"type": "unrelated"}})
	}
	rows = append(padding, rows[1:]...)
	record, _ := invoke(t, "codex", map[string]any{"hook_event_name": "Stop", "session_id": "s", "turn_id": "t", "transcript_path": writeRows(t, rows)})
	if object(record["usage"])["input_tokens"] != float64(100) || record["truncated"] != true {
		t.Fatalf("tail metadata lost: %#v", record)
	}
	for _, invalid := range []map[string]any{{"input_tokens": 0, "cached_input_tokens": 0, "output_tokens": 0, "reasoning_output_tokens": 0, "total_tokens": 500}, {"input_tokens": true}, {"input_tokens": 100}} {
		bad := codexRows("0.160.1", false)
		object(object(bad[len(bad)-1]["payload"])["info"])["last_token_usage"] = invalid
		r, _ := invoke(t, "codex", map[string]any{"hook_event_name": "Stop", "session_id": "s", "turn_id": "t", "transcript_path": writeRows(t, bad)})
		if object(r["usage"])["input_tokens"] != nil {
			t.Fatal("invalid usage accepted")
		}
	}
}

func TestPrivateAppendConcurrentRecordsAndSymlinkRefusal(t *testing.T) {
	path := privatePath(t, "append.jsonl")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, stderr bytes.Buffer
			if Run([]string{"--client", "claude", "--output", path}, strings.NewReader(`{"hook_event_name":"UserPromptSubmit","prompt":"fixture"}`), &out, &stderr) != 0 {
				t.Error(stderr.String())
			}
		}()
	}
	wg.Wait()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 20 {
		t.Fatalf("records %d", len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal("interleaved records")
		}
	}
	link := filepath.Join(filepath.Dir(path), "symlink")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--client", "claude", "--output", link}, {"--client", "claude", "--output", privatePath(t, "read.jsonl")}} {
		var out, stderr bytes.Buffer
		event := `{"hook_event_name":"Stop","transcript_path":` + fmt.Sprintf("%q", link) + `}`
		if Run(args, strings.NewReader(event), &out, &stderr) == 0 {
			t.Fatal("symlink accepted")
		}
	}
}

func TestCollectorCopyFailureAndRedirectNeverLoseSpool(t *testing.T) {
	var received map[string]any
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error(r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.WriteHeader(204)
	}))
	defer collector.Close()
	record, _ := invoke(t, "claude", map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "fixture"}, "--collector", collector.URL+"/events")
	if received["event_id"] != record["event_id"] {
		t.Fatal("collector copy differs")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, collector.URL+"/other", http.StatusFound)
	}))
	defer redirect.Close()
	_, stderr := invoke(t, "claude", map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "fixture"}, "--collector", redirect.URL)
	if !strings.Contains(stderr, "spool retained") {
		t.Fatal(stderr)
	}
	_, stderr = invoke(t, "claude", map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "fixture"}, "--collector", "http://127.0.0.1:1/events")
	if !strings.Contains(stderr, "spool retained") {
		t.Fatal(stderr)
	}
}

func TestPreviewFlagsBoundsAndNoPrivateErrorEcho(t *testing.T) {
	for _, client := range []string{"claude", "codex"} {
		for _, native := range []bool{false, true} {
			path := privatePath(t, "preview.jsonl")
			args := []string{"--client", client, "--output", path, "--preview-config", "--billing-class", "api"}
			if native {
				args = append(args, "--native-hooks")
			}
			var out, stderr bytes.Buffer
			if Run(args, nil, &out, &stderr) != 0 {
				t.Fatal(stderr.String())
			}
			if !strings.Contains(out.String(), "capture") {
				t.Fatal(out.String())
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("preview wrote capture")
			}
			if client == "codex" && !native {
				if !strings.HasPrefix(out.String(), "notify = [") {
					t.Fatal(out.String())
				}
			} else if !json.Valid(out.Bytes()) {
				t.Fatal(out.String())
			}
		}
	}
	for _, collector := range []string{"https://127.0.0.1/events", "http://localhost/events", "http://example.com/events", "http://user:secret@127.0.0.1/events"} {
		var out, stderr bytes.Buffer
		if Run([]string{"--client", "claude", "--output", privatePath(t, "bad.jsonl"), "--collector", collector}, strings.NewReader(`{"hook_event_name":"UserPromptSubmit","prompt":"private-test-only"}`), &out, &stderr) == 0 {
			t.Fatal("unsafe collector accepted")
		}
		if strings.Contains(stderr.String(), "private-test-only") || strings.Contains(stderr.String(), "user:secret") {
			t.Fatal("private error echoed")
		}
	}
	for _, raw := range []string{strings.Repeat("x", 1024*1024+1), `[]`, `{"hook_event_name":"Unknown","prompt":"private-test-only"}`, `{} {}`} {
		var out, stderr bytes.Buffer
		if Run([]string{"--client", "claude", "--output", privatePath(t, "bad.jsonl")}, strings.NewReader(raw), &out, &stderr) == 0 {
			t.Fatal("invalid event accepted")
		}
		if strings.Contains(stderr.String(), "private-test-only") {
			t.Fatal("private input echoed")
		}
	}
}

func TestNestedRedactionAndRecordLimits(t *testing.T) {
	value := map[string]any{"nested": []any{map[string]any{"text": "password=fixture-secret"}}}
	raw, err := json.Marshal(redact(value))
	if err != nil || strings.Contains(string(raw), "fixture-secret") {
		t.Fatalf("nested value escaped redaction: %s, %v", raw, err)
	}
	text := strings.Repeat("界", maxText+1)
	if got := redact(text).(string); len([]rune(got)) != maxText {
		t.Fatal("Unicode text limit was not applied")
	}
	path := privatePath(t, "oversized.jsonl")
	if err := appendPrivate(path, map[string]any{"raw": strings.Repeat("x", maxRecordBytes)}); err == nil {
		t.Fatal("oversized record accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("oversized record touched spool")
	}
}

func TestPreviewRejectsDanglingSymlink(t *testing.T) {
	path := privatePath(t, "dangling")
	if err := os.Symlink(filepath.Join(filepath.Dir(path), "missing"), path); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if Run([]string{"--client", "claude", "--output", path, "--preview-config"}, nil, &out, &stderr) == 0 {
		t.Fatal("preview accepted dangling symlink")
	}
}
