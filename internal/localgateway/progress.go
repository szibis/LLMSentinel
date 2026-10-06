package localgateway

import (
	"encoding/json"
	"regexp"
	"strings"
)

const agentProgressInstruction = "Complete the user's requested deliverable, not just a statement of what you will do next. General research and comparison questions concern the named subject, not automatically the local repository. Search project files only when relevant to the request. Use only actually available tools; do not invent web access, sources or findings. If the evidence or tools are insufficient, explain the limitation or ask a focused clarification. Do not repeat the same lookup against unchanged evidence; change approach or report the limitation."

var unfinishedPromise = regexp.MustCompile(`(?i)^(let me|i will|i'll) (look|search|inspect|explore)\b`)
var substantiveClaim = regexp.MustCompile(`(?i)\b(as|because|shows?|found|recommend|conclude|indicates?|suggests?)\b`)
var repeatedLookupRequest = regexp.MustCompile(`(?i)\b(poll|wait|watch|monitor|repeat|rerun|retry)\b|\b(three|four|five|[3-9]) times\b`)

func readOnlyLookup(name string, input map[string]any) bool {
	switch name {
	case "Read", "Glob", "Grep":
		return true
	case "Bash":
		command, _ := input["command"].(string)
		command = strings.ReplaceAll(command, "2>/dev/null", "")
		command = strings.ReplaceAll(command, "2> /dev/null", "")
		if strings.ContainsAny(command, ";&`<>\n") || strings.Contains(command, "$(") || strings.Contains(command, "||") {
			return false
		}
		if strings.Contains(command, "-delete") || strings.Contains(command, "-exec") || strings.Contains(command, "-ok") || strings.Contains(command, "-fprint") || strings.Contains(command, "-fls") || strings.Contains(command, "--pre") {
			return false
		}
		for _, segment := range strings.Split(command, "|") {
			fields := strings.Fields(segment)
			if len(fields) == 0 {
				return false
			}
			switch fields[0] {
			case "find", "rg", "grep", "ls", "pwd", "cat", "head", "tail", "wc":
			case "xargs":
				if len(fields) < 2 || (fields[1] != "grep" && fields[1] != "rg") {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return false
}

// progressIssue recognizes narrow observable failure patterns, not semantic quality.
func progressIssue(req claudeRequest, blocks []claudeBlock) string {
	type lookup struct {
		key, id, result string
		complete        bool
	}
	var history []lookup
	toolResults := 0
	allowRepeated := false
	for _, message := range req.Messages {
		var parts []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     map[string]any  `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		}
		if json.Unmarshal(message.Content, &parts) != nil {
			if message.Role == "user" {
				history = nil
				toolResults = 0
				var text string
				_ = json.Unmarshal(message.Content, &text)
				allowRepeated = repeatedLookupRequest.MatchString(text)
			}
			continue
		}
		for _, part := range parts {
			if message.Role == "user" && part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				history = nil
				toolResults = 0
				allowRepeated = repeatedLookupRequest.MatchString(part.Text)
			}
			if part.Type == "tool_use" {
				if !readOnlyLookup(part.Name, part.Input) {
					history = nil
					continue
				}
				history = append(history, lookup{key: part.Name + string(mustJSON(part.Input)), id: part.ID})
				if len(history) > 2 {
					history = history[len(history)-2:]
				}
			}
			if part.Type == "tool_result" {
				toolResults++
				for i := range history {
					if history[i].id == part.ToolUseID {
						history[i].result = string(part.Content)
						history[i].complete = !part.IsError
					}
				}
			}
		}
	}
	hasCalls := false
	var answer strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			answer.WriteString(block.Text)
		}
		if block.Type != "tool_use" {
			continue
		}
		hasCalls = true
		key := block.Name + string(mustJSON(block.Input))
		if !allowRepeated && len(history) == 2 && history[0].complete && history[1].complete && history[0].key == key && history[1].key == key && history[0].result == history[1].result {
			return "repeated_unchanged_lookup"
		}
	}
	text := strings.TrimSpace(answer.String())
	if !hasCalls && toolResults > 0 && len(text) <= 320 && strings.Count(text, ".") <= 1 && !strings.ContainsAny(text, "\n?:;") && unfinishedPromise.MatchString(text) && !substantiveClaim.MatchString(text) {
		return "unfinished_final_answer"
	}
	return ""
}
