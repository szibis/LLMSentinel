package localgateway

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const agentProgressInstruction = "Complete the user's requested deliverable, not just a statement of what you will do next. General research and comparison questions concern the named subject, not automatically the local repository. Search project files only when relevant to the request. Use only actually available tools; do not invent web access, sources or findings. If the evidence or tools are insufficient, explain the limitation or ask a focused clarification. Do not repeat the same lookup against unchanged evidence; change approach or report the limitation."

var unfinishedPromise = regexp.MustCompile(`(?i)^(let me|i will|i'll) (look|search|inspect|explore|read|research|review|compare)\b`)
var deliverableRequest = regexp.MustCompile(`(?i)\b(read|inspect|search|research|review|compare|implement|fix|update|create|build)\b`)
var planningRequest = regexp.MustCompile(`(?i)\b(plan|outline)\b|\b(what|how) (will|would)\b`)
var multipleSentences = regexp.MustCompile(`[.!]\s+\S`)
var substantiveClaim = regexp.MustCompile(`(?i)\b(as|because|shows?|found|recommend|conclude|indicates?|suggests?)\b`)
var repeatedLookupRequest = regexp.MustCompile(`(?i)\b(poll|wait|watch|monitor|repeat|rerun|retry)\b|\b(three|four|five|[3-9]) times\b`)
var jsonOnlyRequest = regexp.MustCompile(`(?i)\b(return|reply|respond|output)\s+(only\s+)?(a\s+)?(valid\s+)?JSON\b`)
var requestedTestCommand = regexp.MustCompile(`(?i)\b(run|execute)\b[^\n.]{0,50}\b(go test|cargo test|npm test|pytest|make test)\b`)
var testLimitation = regexp.MustCompile(`(?i)\b(cannot|could not|unable|unavailable|denied|failed|failure)\b`)
var testCommandPrefix = regexp.MustCompile(`^(?:cd\s+(?:"[^"\n]+"|'[^'\n]+'|[^\s;&|]+)\s*&&\s*)?(?:(?:rtk\s+)?env\s+)?(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s;&|]+)\s+)*(?:rtk\s+)?(?:go test|cargo test|npm test|pytest|make test)(?:\s|$)`)
var commandSession = regexp.MustCompile(`(?m)^Process running with session ID ([0-9]+)$`)
var commandExit = regexp.MustCompile(`(?m)^Process exited with code ([0-9]+)$`)
var omitLineNumbers = regexp.MustCompile(`(?i)\b(without|omit|exclude|no)\s+line numbers\b`)
var readDisplayPrefix = regexp.MustCompile(`^\s*[0-9]+\t`)
var explicitFileRead = regexp.MustCompile(`(?i)^\s*(?:please\s+)?(read|inspect)\s+(?:the\s+)?\S+\.(txt|json|go|md|yaml|yml|toml|csv|rs|js|ts|py)\b`)
var exactTestCommand = regexp.MustCompile(`(?i)\b(run|execute)\s+exactly\s+(go test|cargo test|npm test|pytest|make test)\b`)
var verbatimContents = regexp.MustCompile(`(?i)\b(return|reply|output)\s+(?:only\s+)?(?:its|the|file)?\s*contents\s+exactly\b`)
var singleFileRead = regexp.MustCompile(`(?i)^\s*(?:please\s+)?(?:read|inspect)\s+(?:the\s+)?([^\s,;]+)`)
var directCat = regexp.MustCompile(`^cat\s+([^\s;&|<>$` + "`" + `]+)\s*$`)

// Only explicit verbatim requests and completed, unambiguous native cat reads
// permit comparison. This never substitutes an answer or interprets file data.
func verbatimReadMismatch(req claudeRequest, answer string) bool {
	var task, evidence string
	reads := map[string]bool{}
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
				_ = json.Unmarshal(message.Content, &task)
				evidence = ""
				reads = map[string]bool{}
			}
			continue
		}
		for _, part := range parts {
			if message.Role == "user" && part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				task = part.Text
				evidence = ""
				reads = map[string]bool{}
			}
			file := singleFileRead.FindStringSubmatch(task)
			if len(file) != 2 || !verbatimContents.MatchString(task) {
				continue
			}
			if part.Type == "tool_use" && (part.Name == "exec_command" || strings.HasSuffix(part.Name, ".exec_command")) {
				command, _ := part.Input["cmd"].(string)
				path := directCat.FindStringSubmatch(command)
				if len(path) == 2 {
					requested, actual := strings.Trim(file[1], "\"'"), strings.Trim(path[1], "\"'")
					if strings.Contains(requested, "/") {
						reads[part.ID] = filepath.Clean(actual) == filepath.Clean(requested)
					} else {
						reads[part.ID] = filepath.Base(actual) == requested
					}
				}
			}
			if part.Type == "tool_result" && reads[part.ToolUseID] && !part.IsError {
				result, _ := textContent(part.Content)
				if header, stdout, ok := nativeExecutionEvidence(result); ok {
					if exit := commandExit.FindStringSubmatch(header); exit != nil && exit[1] == "0" {
						evidence = stdout
					}
				}
			}
		}
	}
	return evidence != "" && strings.TrimSpace(evidence) != strings.TrimSpace(answer)
}

func nativeExecutionEvidence(result string) (header, stdout string, ok bool) {
	parts := strings.SplitN(result, "\nOutput:\n", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "Chunk ID: ") || !strings.Contains(parts[0], "\nWall time: ") || (commandExit.FindStringSubmatch(parts[0]) == nil && commandSession.FindStringSubmatch(parts[0]) == nil) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func isTestExecution(name string, input map[string]any) bool {
	if name != "Bash" && name != "exec_command" && !strings.HasSuffix(name, ".exec_command") {
		return false
	}
	command, _ := input["command"].(string)
	if command == "" {
		command, _ = input["cmd"].(string)
	}
	return testCommandPrefix.MatchString(strings.TrimSpace(command))
}

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
	userTask := ""
	testCalls := map[string]bool{}
	testNative := map[string]bool{}
	testSessions := map[string]bool{}
	testCompleted := false
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
				userTask = text
				testCalls = map[string]bool{}
				testNative = map[string]bool{}
				testSessions = map[string]bool{}
				testCompleted = false
				allowRepeated = repeatedLookupRequest.MatchString(text)
			}
			continue
		}
		for _, part := range parts {
			if message.Role == "user" && part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				history = nil
				toolResults = 0
				allowRepeated = repeatedLookupRequest.MatchString(part.Text)
				userTask = part.Text
				testCalls = map[string]bool{}
				testNative = map[string]bool{}
				testSessions = map[string]bool{}
				testCompleted = false
			}
			if part.Type == "tool_use" {
				if isTestExecution(part.Name, part.Input) {
					testCalls[part.ID] = true
					testNative[part.ID] = part.Name != "Bash"
				}
				if part.Name == "write_stdin" || strings.HasSuffix(part.Name, ".write_stdin") {
					if session, ok := part.Input["session_id"].(float64); ok && testSessions[strconv.FormatFloat(session, 'f', -1, 64)] {
						testCalls[part.ID], testNative[part.ID] = true, true
					}
				}
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
				if testCalls[part.ToolUseID] && !part.IsError {
					evidence, _ := textContent(part.Content)
					// Native execution status lives before Output; command stdout
					// cannot spoof a finished session by printing status text.
					header := strings.SplitN(evidence, "\nOutput:", 2)[0]
					if session := commandSession.FindStringSubmatch(header); session != nil {
						testSessions[session[1]] = true
					} else if testNative[part.ToolUseID] {
						if exit := commandExit.FindStringSubmatch(header); exit != nil && exit[1] == "0" {
							testCompleted = true
						}
					} else {
						testCompleted = true
					}
				}
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
		if exactTestCommand.MatchString(userTask) && isTestExecution(block.Name, block.Input) {
			command, _ := block.Input["cmd"].(string)
			if command == "" {
				command, _ = block.Input["command"].(string)
			}
			if strings.HasPrefix(strings.TrimSpace(command), "cd ") {
				return "requested_test_command_wrapped"
			}
		}
		key := block.Name + string(mustJSON(block.Input))
		if !allowRepeated && block.Name == "Read" && len(history) > 0 {
			latest := history[len(history)-1]
			evidence, _ := textContent(json.RawMessage(latest.result))
			if latest.complete && latest.key == key && strings.HasPrefix(evidence, "Wasted call — file unchanged since your last Read.") {
				return "repeated_unchanged_lookup"
			}
		}
		if !allowRepeated && len(history) == 2 && history[0].complete && history[1].complete && history[0].key == key && history[1].key == key && history[0].result == history[1].result {
			return "repeated_unchanged_lookup"
		}
	}
	text := strings.TrimSpace(answer.String())
	if !hasCalls && verbatimReadMismatch(req, text) {
		return "verbatim_read_mismatch"
	}
	if !hasCalls && omitLineNumbers.MatchString(userTask) && readDisplayPrefix.MatchString(text) {
		for _, lookup := range history {
			if lookup.complete && strings.HasPrefix(lookup.key, "Read{") {
				evidence, _ := textContent(json.RawMessage(lookup.result))
				if strings.TrimSpace(evidence) == text {
					return "read_display_metadata"
				}
			}
		}
	}
	if !hasCalls && jsonOnlyRequest.MatchString(userTask) && !json.Valid([]byte(text)) {
		return "requested_json_missing"
	}
	executionAvailable := false
	readAvailable := false
	for _, tool := range req.Tools {
		executionAvailable = executionAvailable || tool.Name == "Bash" || tool.Name == "exec_command" || strings.HasSuffix(tool.Name, ".exec_command")
		readAvailable = readAvailable || tool.Name == "Read"
	}
	if !hasCalls && (readAvailable || executionAvailable) && explicitFileRead.MatchString(userTask) && toolResults == 0 && !testLimitation.MatchString(text) {
		return "requested_read_not_executed"
	}
	if !hasCalls && executionAvailable && requestedTestCommand.MatchString(userTask) && !testCompleted && !testLimitation.MatchString(text) {
		return "requested_tests_not_executed"
	}
	needsDeliverable := toolResults > 0 || deliverableRequest.MatchString(userTask)
	requestedPromise := planningRequest.MatchString(userTask) || (text != "" && strings.Contains(userTask, text))
	if !hasCalls && needsDeliverable && !requestedPromise && len(text) <= 320 && !multipleSentences.MatchString(text) && !strings.ContainsAny(text, "\n?:;") && unfinishedPromise.MatchString(text) && !substantiveClaim.MatchString(text) {
		return "unfinished_final_answer"
	}
	return ""
}
