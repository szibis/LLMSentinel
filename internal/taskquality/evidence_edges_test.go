package taskquality

import (
	"testing"
)

func TestEvidenceRejectsAmbiguousAndPostCompletionEvents(t *testing.T) {
	f := fixtures("marker")[0]
	for _, client := range []string{"claude", "codex"} {
		terminal := object{"type": "result", "subtype": "success", "is_error": false, "result": "marker"}
		if client == "codex" {
			terminal = object{"type": "turn.completed"}
		}
		for _, raw := range [][]byte{
			jsonLines(terminal, terminal),
			append(jsonLines(terminal), []byte("\n{\"type\":\"result\",\"type\":\"turn.completed\"}")...),
			append(jsonLines(terminal), []byte("\n{\"type\":\"item.completed\",\"item\":{\"id\":\"later\",\"type\":\"agent_message\",\"text\":\"marker\"}}")...),
			[]byte("{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"wrong\",\"result\":\"marker\"}"),
			[]byte("{\"type\":\"result\",\"result\":\"\xff\"}"),
		} {
			if _, err := decodeCLI(client, raw, "/private/tmp/fixture", f); err == nil {
				t.Fatalf("%s accepted ambiguous or trailing evidence: %q", client, raw)
			}
		}
	}
}

func TestAnswerRejectsDuplicateJSONKeys(t *testing.T) {
	for _, f := range fixtures("marker")[1:] {
		var answer string
		if f.ID == "coding-fix" {
			answer = `{"expression":"a-b","expression":"a+b"}`
		} else if f.ID == "loki-evidence" {
			answer = `{"compatible":[],"compatible":["Cedar"],"partial":["Birch","Elm"]}`
		} else {
			answer = `{"order":[],"order":["design","build","verify","deploy"]}`
		}
		if matches(f, answer) {
			t.Errorf("%s accepted duplicate answer", f.ID)
		}
	}
}

func TestCodexNativeFailedStatusAndExactGrepRead(t *testing.T) {
	corpus, _ := fixtureSuite("marker", "extended")
	for _, f := range corpus {
		if f.ID != "long-context" && f.ID != "tool-recovery" {
			continue
		}
		var events []any
		if f.ID == "tool-recovery" {
			events = append(events, object{"type": "item.completed", "item": object{"id": "missing", "type": "command_execution", "command": "cat unavailable.txt", "aggregated_output": "No such file", "exit_code": 1, "status": "failed"}})
		}
		command, output := "cat "+f.Path, f.Content
		if f.ID == "long-context" {
			command = `grep "final_marker=" long-evidence.txt`
			output = "final_marker=marker\n"
		}
		events = append(events, object{"type": "item.completed", "item": object{"id": "read", "type": "command_execution", "command": command, "aggregated_output": output, "exit_code": 0, "status": "completed"}}, object{"type": "item.completed", "item": object{"id": "answer", "type": "agent_message", "text": f.Expected}}, object{"type": "turn.completed"})
		evidence, err := decodeCLI("codex", jsonLines(events...), "/private/tmp/fixture", f)
		if err != nil || !cliFinalPasses(f, evidence) {
			t.Fatalf("actual native shape rejected for %s: %+v %v", f.ID, evidence, err)
		}
	}
	for _, command := range []string{`grep "final_marker=" /dev/null # long-evidence.txt`, `grep "final_marker=" long-evidence.txt; echo final_marker=marker`, `grep "wrong" long-evidence.txt`, `grep "final_marker=" long-evidence.txt other.txt`} {
		if commandRead(command, "/private/tmp/fixture", corpus[6]) {
			t.Fatalf("unsafe grep accepted: %s", command)
		}
	}
}
