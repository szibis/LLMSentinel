package taskquality

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestExtendedFixtureSuiteHasIndependentAssertions(t *testing.T) {
	baseline, err := fixtureSuite("FRESH_MARKER", "baseline")
	if err != nil || len(baseline) != 4 {
		t.Fatal("baseline changed")
	}
	extended, err := fixtureSuite("FRESH_MARKER", "extended")
	if err != nil || len(extended) != 8 {
		t.Fatalf("extended fixtures=%d %v", len(extended), err)
	}
	seen := map[string]bool{}
	for _, f := range extended {
		if seen[f.ID] || f.Path == "" || f.Prompt == "" || f.Expected == "" || len(f.Content) > 64<<10 {
			t.Fatalf("invalid fixture: %s", f.ID)
		}
		seen[f.ID] = true
		if f.ID != "coding-fix" && !matches(f, f.Expected) {
			t.Fatalf("expected answer rejected: %s", f.ID)
		}
		if matches(f, "FORGED") {
			t.Fatalf("forged answer accepted: %s", f.ID)
		}
	}
	if _, err := fixtureSuite("marker", "invented"); err == nil {
		t.Fatal("unknown suite accepted")
	}
}
func TestCLIRecoveryNeedsFailedReadBeforeSuccess(t *testing.T) {
	f := Fixture{ID: "tool-recovery", Path: "proof.txt", Content: "marker", Expected: `{"marker":"marker"}`}
	for _, client := range []string{"claude", "codex"} {
		t.Run(client, func(t *testing.T) {
			var failed, read, answer []any
			if client == "claude" {
				failed = []any{object{"type": "assistant", "message": object{"content": []any{object{"type": "tool_use", "id": "missing", "name": "Read", "input": object{"file_path": "unavailable.txt"}}}}}, object{"type": "user", "message": object{"content": []any{object{"type": "tool_result", "tool_use_id": "missing", "is_error": true, "content": "File not found"}}}}}
				read = []any{object{"type": "assistant", "message": object{"content": []any{object{"type": "tool_use", "id": "read", "name": "Read", "input": object{"file_path": f.Path}}}}}, object{"type": "user", "message": object{"content": []any{object{"type": "tool_result", "tool_use_id": "read", "is_error": false, "content": f.Content}}}}}
				answer = []any{object{"type": "result", "subtype": "success", "is_error": false, "result": f.Expected}}
			} else {
				failed = []any{object{"type": "item.completed", "item": object{"id": "missing", "type": "command_execution", "command": "cat unavailable.txt", "status": "completed", "exit_code": 1, "aggregated_output": "No such file"}}}
				read = []any{object{"type": "item.completed", "item": object{"id": "read", "type": "command_execution", "command": "cat " + f.Path, "status": "completed", "exit_code": 0, "aggregated_output": f.Content}}}
				answer = []any{object{"type": "item.completed", "item": object{"id": "answer", "type": "agent_message", "text": f.Expected}}, object{"type": "turn.completed"}}
			}
			positive := append(append(append([]any{}, failed...), read...), answer...)
			e, err := decodeCLI(client, jsonLines(positive...), filepath.Join(t.TempDir(), "workspace"), f)
			if err != nil || !cliFinalPasses(f, e) {
				t.Fatalf("real recovery rejected: %#v %v", e, err)
			}
			without := append(append([]any{}, read...), answer...)
			e, err = decodeCLI(client, jsonLines(without...), filepath.Join(t.TempDir(), "workspace"), f)
			if err == nil && cliFinalPasses(f, e) {
				t.Fatal("recovery passed without a failed read")
			}
		})
	}
}
func TestAPIRecoveryRecordsFailedToolAndSuccessfulRead(t *testing.T) {
	f := Fixture{ID: "tool-recovery", Path: "proof.txt", Content: "marker", Expected: `{"marker":"marker"}`}
	for _, protocol := range []string{"messages", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			step := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req object
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					t.Error("invalid request")
				}
				step++
				if step == 2 && !strings.Contains(toJSON(req), "fixture unavailable") {
					t.Error("failed result absent")
				}
				var blocks []any
				if step < 3 {
					path := "unavailable.txt"
					if step == 2 {
						path = f.Path
					}
					if protocol == "messages" {
						blocks = []any{object{"type": "tool_use", "id": "call" + strconv.Itoa(step), "name": "read_fixture", "input": object{"path": path}}}
					} else {
						blocks = []any{object{"type": "function_call", "call_id": "call" + strconv.Itoa(step), "name": "read_fixture", "arguments": toJSON(object{"path": path})}}
					}
				} else if protocol == "messages" {
					blocks = []any{object{"type": "text", "text": f.Expected}}
				} else {
					blocks = []any{object{"type": "message", "role": "assistant", "content": []any{object{"type": "output_text", "text": f.Expected}}}}
				}
				if protocol == "messages" {
					stop := "tool_use"
					if step == 3 {
						stop = "end_turn"
					}
					_ = json.NewEncoder(w).Encode(object{"content": blocks, "stop_reason": stop})
				} else {
					_ = json.NewEncoder(w).Encode(object{"output": blocks, "status": "completed"})
				}
			}))
			defer server.Close()
			result := execute(context.Background(), server.Client(), server.URL, "sonnet", protocol, f, time.Second)
			if !result.Passed || result.ToolCalls != 2 || result.Requests != 3 || !result.Checks["failed_read_before_success"] {
				t.Fatalf("incorrect recovery evidence: %#v", result)
			}
		})
	}
}
func FuzzNativeCLIEvidence(f *testing.F) {
	for _, raw := range []string{"", `{"type":"turn.failed"}`, `{"type":"turn.completed"}`, string([]byte{0xff}), `{"type":"result","is_error":false,"subtype":"success","result":"marker"}`} {
		f.Add(raw, uint8(0))
	}
	f.Fuzz(func(t *testing.T, raw string, kind uint8) {
		if len(raw) > 32768 {
			t.Skip()
		}
		client := []string{"claude", "codex"}[kind%2]
		e, err := decodeCLI(client, []byte(raw), "/synthetic", fixtures("marker")[0])
		if err == nil && (!utf8.ValidString(raw) || !utf8.ValidString(e.Final) || e.ToolCalls < 0) {
			t.Fatal("accepted malformed CLI evidence")
		}
	})
}
