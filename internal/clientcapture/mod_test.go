package clientcapture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// These tests exercise the opt-in CLI boundary: losing validation, redaction,
// or the native-event branch must fail before any private spool is appended.
func TestModPromptPrivateObservation(t *testing.T) {
	record, _ := invoke(t, "claude", map[string]any{
		"mod_event": "prompt.submit", "session_id": "session", "turn_id": "turn", "agent_id": "agent",
		"prompt": "Review api_key=fixture-secret", "model": "reported-model",
		"transcript_path": "/a/transcript/that/must/not/be/read", "raw": map[string]any{"token": "discarded-secret"},
	}, "--mod-event")
	for key, want := range map[string]any{"source": "claude_code_mod", "provenance": "local_cli_observation", "event_type": "prompt.submit", "client": "claude", "session_id": "session", "turn_id": "turn", "agent_id": "agent", "model": "reported-model", "truncated": false} {
		if record[key] != want {
			t.Errorf("%s = %#v, want %#v", key, record[key], want)
		}
	}
	if fmt.Sprint(record["inputs"]) != "[Review [REDACTED]]" || fmt.Sprint(record["outputs"]) != "[]" {
		t.Fatal(record)
	}
	if object(record["quality"])["status"] != "unscored" || object(record["quality"])["training_eligible"] != false || object(record["usage"])["input_tokens"] != nil || object(record["billing"])["cost_usd"] != nil {
		t.Fatal(record)
	}
	for _, key := range []string{"transcript_path", "raw", "prompt", "mod_event"} {
		if _, ok := record[key]; ok {
			t.Fatalf("retained raw key %s", key)
		}
	}
	if record["event_id"] == "" || record["timestamp"] == "" {
		t.Fatal("missing append identity")
	}
}

func TestModTurnReportedUsageAndDuration(t *testing.T) {
	record, _ := invoke(t, "claude", map[string]any{"mod_event": "turn.complete", "session_id": "s", "answer": "Bearer private-secret", "duration_ms": 12.5, "is_aborted": true, "usage": map[string]any{"input_tokens": 10, "output_tokens": 4, "cache_read_input_tokens": 0, "ignored": map[string]any{"result": "discarded"}}}, "--mod-event")
	want := map[string]any{"input_tokens": float64(10), "output_tokens": float64(4), "cache_read_input_tokens": float64(0), "cache_creation_input_tokens": nil, "source": "claude_mod_turn_usage", "scope": "turn_reported", "provenance": "local_cli_observation"}
	if !reflect.DeepEqual(object(record["usage"]), want) {
		t.Fatalf("usage %#v, want %#v", record["usage"], want)
	}
	if fmt.Sprint(record["outputs"]) != "[[REDACTED]]" || fmt.Sprint(record["inputs"]) != "[]" || record["model"] != nil || record["turn_id"] != nil {
		t.Fatal(record)
	}
	if record["latency_ms"] != 12.5 || record["latency_source"] != "claude_mod_turn_duration" || object(record["capture_metadata"])["is_aborted"] != true {
		t.Fatal(record)
	}
}

func TestModToolReturnDoesNotCaptureToolContent(t *testing.T) {
	for _, stage := range []string{"returned", "error"} {
		record, _ := invoke(t, "claude", map[string]any{"mod_event": "tool.call", "session_id": "s", "tool_name": "Bash", "tool_use_id": "id", "stage": stage, "args": map[string]any{"command": "never retain command"}, "result": "never retain result", "prompt": "never retain prompt", "answer": "never retain answer"}, "--mod-event")
		want := map[string]any{"tool_name": "Bash", "tool_use_id": "id", "stage": stage, "stage_semantics": "host_return_only"}
		if !reflect.DeepEqual(object(record["capture_metadata"]), want) {
			t.Fatal(record)
		}
		if fmt.Sprint(record["inputs"]) != "[]" || fmt.Sprint(record["outputs"]) != "[]" || record["model"] != nil {
			t.Fatal(record)
		}
		raw, _ := json.Marshal(record)
		if strings.Contains(string(raw), "never retain") {
			t.Fatalf("tool content retained: %s", raw)
		}
	}
}

func TestModRejectsMalformedBeforeSpool(t *testing.T) {
	for _, raw := range []string{
		`{"mod_event":"unknown","session_id":"s"}`, `{"mod_event":"prompt.submit","session_id":"s"}`,
		`{"mod_event":"prompt.submit","session_id":"","prompt":"private-test-only"}`,
		`{"mod_event":"prompt.submit","session_id":1,"prompt":"private-test-only"}`,
		`{"mod_event":"prompt.submit","session_id":"s","prompt":null}`,
		`{"mod_event":"turn.complete","session_id":"s","answer":{}}`,
		`{"mod_event":"turn.complete","session_id":"s","turn_id":""}`,
		`{"mod_event":"turn.complete","session_id":"s","model":true}`,
		`{"mod_event":"turn.complete","session_id":"s","agent_id":{}}`,
		`{"mod_event":"turn.complete","session_id":"s","duration_ms":-1}`,
		`{"mod_event":"turn.complete","session_id":"s","duration_ms":1e999}`,
		`{"mod_event":"turn.complete","session_id":"s","duration_ms":"1"}`,
		`{"mod_event":"turn.complete","session_id":"s","is_aborted":1}`,
		`{"mod_event":"turn.complete","session_id":"s","usage":[]}`,
		`{"mod_event":"turn.complete","session_id":"s","usage":{"input_tokens":null}}`,
		`{"mod_event":"turn.complete","session_id":"s","usage":{"input_tokens":-1}}`,
		`{"mod_event":"turn.complete","session_id":"s","usage":{"input_tokens":1.5}}`,
		`{"mod_event":"turn.complete","session_id":"s","usage":{"input_tokens":9223372036854775808}}`,
		`{"mod_event":"tool.call","session_id":"s","tool_name":"Bash"}`,
		`{"mod_event":"tool.call","session_id":"s","stage":"returned"}`,
		`{"mod_event":"tool.call","session_id":"s","tool_name":"Bash","stage":"success"}`,
		`{"mod_event":"turn.complete","session_id":"` + strings.Repeat("s", 257) + `"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			path := privatePath(t, "bad.jsonl")
			var out, stderr bytes.Buffer
			if Run([]string{"--client", "claude", "--output", path, "--mod-event"}, strings.NewReader(raw), &out, &stderr) == 0 {
				t.Fatal("malformed event accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("malformed input touched spool")
			}
			if strings.Contains(stderr.String(), "private-test-only") || out.Len() != 0 {
				t.Fatal("event echoed")
			}
		})
	}
}

func TestModOptInAndFlagExclusion(t *testing.T) {
	for _, extra := range [][]string{nil, {"--mod-event", "--preview-config"}, {"--mod-event", "--native-hooks"}, {"--mod-event", "--client", "codex"}} {
		path := privatePath(t, "flags.jsonl")
		var out, stderr bytes.Buffer
		args := append([]string{"--client", "claude", "--output", path}, extra...)
		if Run(args, strings.NewReader(`{"mod_event":"turn.complete","session_id":"s"}`), &out, &stderr) == 0 {
			t.Fatal("invalid mod flags accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid flags touched spool")
		}
		if out.Len() != 0 {
			t.Fatal("invalid flags printed config")
		}
	}
}

func TestModTruncationAndCollectorFailureKeepPrivateSpool(t *testing.T) {
	path := privatePath(t, "mod.jsonl")
	var out, stderr bytes.Buffer
	raw, _ := json.Marshal(map[string]any{"mod_event": "turn.complete", "session_id": "s", "answer": strings.Repeat("界", maxText+1)})
	if Run([]string{"--client", "claude", "--output", path, "--mod-event", "--collector", "http://127.0.0.1:1/events"}, bytes.NewReader(raw), &out, &stderr) != 0 {
		t.Fatal(stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record["truncated"] != true || len([]rune(record["outputs"].([]any)[0].(string))) != maxText {
		t.Fatal("unmarked or incorrect truncation")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("capture not private")
	}
	if !strings.Contains(stderr.String(), "spool retained") || out.Len() != 0 {
		t.Fatal(stderr.String())
	}
}
