package localgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Responses is a stateless, text/function-only protocol adapter. The caller
// supplies complete history and retains all tool execution and permissions.
type responsesTool struct {
	Type         string               `json:"type"`
	Name         string               `json:"name"`
	Description  string               `json:"description,omitempty"`
	Parameters   map[string]any       `json:"parameters,omitempty"`
	Strict       *bool                `json:"strict,omitempty"`
	DeferLoading bool                 `json:"defer_loading,omitempty"`
	Format       *responsesToolFormat `json:"format,omitempty"`
	Tools        []responsesTool      `json:"tools,omitempty"`
	namespace    string
}
type responsesToolFormat struct {
	Type       string `json:"type"`
	Syntax     string `json:"syntax"`
	Definition string `json:"definition"`
}

func (t responsesTool) qualifiedName() string {
	if t.namespace != "" {
		return t.namespace + "." + t.Name
	}
	return t.Name
}

var responseNamespaceName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

// Codex 0.160.1 sends native namespace wrappers containing functions and custom
// tools. Flatten exactly one level for Qwen, preserving wire identities.
func flattenResponsesTools(tools []responsesTool) ([]responsesTool, error) {
	flat := []responsesTool{}
	namespaces := map[string]bool{}
	identities := map[string]bool{}
	appendTool := func(tool responsesTool, namespace string) error {
		if len(flat) >= 128 {
			return errors.New("at most 128 callable tools are supported")
		}
		if tool.Type != "function" && tool.Type != "custom" {
			return fmt.Errorf("unsupported namespace tool type %q", tool.Type)
		}
		if tool.DeferLoading {
			return errors.New("deferred tool loading and tool search are unsupported")
		}
		if namespace != "" && !responseNamespaceName.MatchString(tool.Name) {
			return errors.New("invalid namespace function name")
		}
		tool.namespace = namespace
		identity := tool.qualifiedName()
		if identities[identity] {
			return errors.New("duplicate or ambiguous qualified tool name")
		}
		identities[identity] = true
		flat = append(flat, tool)
		return nil
	}
	for _, tool := range tools {
		if tool.Type != "namespace" {
			if err := appendTool(tool, ""); err != nil {
				return nil, err
			}
			continue
		}
		if len(namespaces) >= 16 || !responseNamespaceName.MatchString(tool.Name) || namespaces[tool.Name] || len(tool.Tools) == 0 {
			return nil, errors.New("namespaces must have unique valid names, contain tools and number at most 16")
		}
		namespaces[tool.Name] = true
		for _, child := range tool.Tools {
			if err := appendTool(child, tool.Name); err != nil {
				return nil, err
			}
		}
	}
	return flat, nil
}

type responsesRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Instructions       string          `json:"instructions"`
	MaxOutputTokens    *int            `json:"max_output_tokens"`
	Stream             bool            `json:"stream"`
	Store              *bool           `json:"store"`
	PreviousResponseID string          `json:"previous_response_id"`
	Background         bool            `json:"background"`
	Conversation       json.RawMessage `json:"conversation"`
	Tools              []responsesTool `json:"tools"`
	ToolChoice         json.RawMessage `json:"tool_choice"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls"`
	Text               struct {
		Format struct {
			Type string `json:"type"`
		} `json:"format"`
	} `json:"text"`
	Metadata  map[string]string `json:"metadata"`
	Reasoning struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	} `json:"reasoning"`
}

// Canonical single-environment Codex grammar; arbitrary grammars are not
// interpreted. The client still verifies filesystem context and permissions.
const canonicalPatchGrammar = `start: begin_patch hunk+ end_patch
begin_patch: "*** Begin Patch" LF
end_patch: "*** End Patch" LF?

hunk: add_hunk | delete_hunk | update_hunk
add_hunk: "*** Add File: " filename LF add_line+
delete_hunk: "*** Delete File: " filename LF
update_hunk: "*** Update File: " filename LF change_move? change?

filename: /(.+)/
add_line: "+" /(.*)/ LF -> line
change_move: "*** Move to: " filename LF
change: (change_context | change_line)+ eof_line?
change_context: ("@@" | "@@ " /(.+)/) LF
change_line: ("+" | "-" | " ") /(.*)/ LF
eof_line: "*** End of File" LF

%import common.LF
`

func patchGrammarRules(grammar string) string {
	var rules []string
	for _, line := range strings.Split(grammar, "\n") {
		if rule := strings.TrimSpace(line); rule != "" {
			rules = append(rules, rule)
		}
	}
	return strings.Join(rules, "\n")
}

// validatePatchSyntax recognizes the advertised grammar without touching files.
// Applying hunks to existing text remains Codex's responsibility.
func validatePatchSyntax(patch string) error {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	if len(lines) < 3 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return errors.New("apply_patch requires a complete Begin/End Patch envelope")
	}
	named := func(line, prefix string) bool { return strings.HasPrefix(line, prefix) && len(line) > len(prefix) }
	i, hunks := 1, 0
	for i < len(lines)-1 {
		line := lines[i]
		i++
		hunks++
		switch {
		case named(line, "*** Delete File: "):
		case named(line, "*** Add File: "):
			start := i
			for i < len(lines)-1 && strings.HasPrefix(lines[i], "+") {
				i++
			}
			if i == start {
				return errors.New("add patch requires + content lines")
			}
		case named(line, "*** Update File: "):
			if i < len(lines)-1 && named(lines[i], "*** Move to: ") {
				i++
			}
			changed := false
			for i < len(lines)-1 {
				part := lines[i]
				if part == "@@" || named(part, "@@ ") || strings.HasPrefix(part, "+") || strings.HasPrefix(part, "-") || strings.HasPrefix(part, " ") {
					changed = true
					i++
					continue
				}
				break
			}
			if i < len(lines)-1 && lines[i] == "*** End of File" {
				if !changed {
					return errors.New("end of file requires a change")
				}
				i++
			}
		default:
			return errors.New("invalid apply_patch hunk syntax: use *** Update File: path followed by @@ and -old/+new lines; do not use unified diff ---/+++ file headers")
		}
	}
	if hunks == 0 {
		return errors.New("patch requires at least one hunk")
	}
	return nil
}

func responsesError(w http.ResponseWriter, status int, message string) {
	kind := "server_error"
	if status >= 400 && status < 500 {
		kind = "invalid_request_error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": kind, "message": message, "param": nil, "code": nil}})
}

func responseInputText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if unmarshalModelJSON(string(raw), &parts) != nil || parts == nil {
		return "", errors.New("content must be a string or text content array")
	}
	var out []string
	for _, part := range parts {
		if (part.Type != "input_text" && part.Type != "output_text") || part.Text == nil {
			return "", fmt.Errorf("unsupported Responses content %q; only text is supported", part.Type)
		}
		out = append(out, *part.Text)
	}
	return strings.Join(out, "\n"), nil
}

func prepareResponses(req responsesRequest, roleEndpoints bool, defaultBudget int) (claudeRequest, []map[string]string, error) {
	converted := claudeRequest{Model: req.Model, MaxTokens: defaultBudget, System: mustJSON(req.Instructions)}
	converted.Metadata.UserID = req.Metadata["session_id"]
	if req.Model == "local" && roleEndpoints {
		converted.Model = "sentinel-sonnet"
		if req.Reasoning.Effort == "high" || req.Reasoning.Effort == "xhigh" {
			converted.Model = "sentinel-opus"
		}
	}
	if roleEndpoints {
		converted.MaxTokens = 32768
	}
	if req.Model == "" {
		return converted, nil, errors.New("model is required")
	}
	if req.MaxOutputTokens != nil {
		if *req.MaxOutputTokens < 1 || *req.MaxOutputTokens > 32768 {
			return converted, nil, errors.New("max_output_tokens must be 1..32768")
		}
		converted.MaxTokens = *req.MaxOutputTokens
	}
	if req.PreviousResponseID != "" || (len(req.Conversation) > 0 && string(req.Conversation) != "null") {
		return converted, nil, errors.New("server-side history is unsupported; omit previous_response_id/conversation and supply full input history")
	}
	if req.Background || (req.Store != nil && *req.Store) {
		return converted, nil, errors.New("background and stored Responses are unsupported; use store:false and complete input history")
	}
	if req.Text.Format.Type != "" && req.Text.Format.Type != "text" {
		return converted, nil, errors.New("structured output formats are unsupported")
	}
	flatTools, err := flattenResponsesTools(req.Tools)
	if err != nil {
		return converted, nil, err
	}
	for _, tool := range flatTools {
		if tool.Type == "custom" {
			if tool.Name != "apply_patch" || tool.Format == nil || tool.Format.Type != "grammar" || tool.Format.Syntax != "lark" || patchGrammarRules(tool.Format.Definition) != patchGrammarRules(canonicalPatchGrammar) {
				return converted, nil, errors.New("only the canonical single-environment Codex apply_patch custom grammar is supported")
			}
			converted.Tools = append(converted.Tools, claudeTool{Name: tool.qualifiedName(), Description: tool.Description + " Supply raw patch text in the required input string argument. It must start with *** Begin Patch, then Add File/Delete File/Update File hunks, and end with *** End Patch, each on its own line. Added file content lines start with +. Update content lines start with +, -, or a space; optional context lines start with @@. This is NOT unified diff: never use ---/+++ file headers. Example structure: *** Begin Patch\n*** Update File: path\n@@\n-old text\n+new text\n*** End Patch\n. Supply actual newlines in the decoded input string.", Schema: map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string"}}, "required": []any{"input"}, "additionalProperties": false}})
			continue
		}
		if tool.Type != "function" {
			return converted, nil, fmt.Errorf("unsupported tool type %q; only client function tools and canonical apply_patch are supported", tool.Type)
		}
		if tool.Parameters == nil {
			return converted, nil, errors.New("function tools require object parameters")
		}
		converted.Tools = append(converted.Tools, claudeTool{Name: tool.qualifiedName(), Description: tool.Description, Schema: tool.Parameters})
	}
	if len(req.ToolChoice) > 0 && string(req.ToolChoice) != "null" {
		var choice string
		if json.Unmarshal(req.ToolChoice, &choice) == nil {
			switch choice {
			case "auto", "none":
				converted.ToolChoice.Type = choice
			case "required":
				converted.ToolChoice.Type = "any"
			default:
				return converted, nil, errors.New("unsupported tool_choice")
			}
		} else {
			var choice struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}
			if unmarshalModelJSON(string(req.ToolChoice), &choice) != nil || (choice.Type != "function" && choice.Type != "custom") || choice.Name == "" {
				return converted, nil, errors.New("tool_choice must name a function tool")
			}
			matches := false
			for _, tool := range flatTools {
				if tool.Name == choice.Name && tool.Type == choice.Type && tool.namespace == choice.Namespace {
					matches = true
				}
			}
			if !matches {
				return converted, nil, errors.New("tool_choice type and name must match an available tool")
			}
			converted.ToolChoice.Type = "tool"
			converted.ToolChoice.Name = (responsesTool{Name: choice.Name, namespace: choice.Namespace}).qualifiedName()
		}
	}
	var input string
	if json.Unmarshal(req.Input, &input) == nil {
		converted.Messages = append(converted.Messages, claudeMessage{Role: "user", Content: mustJSON(input)})
	} else {
		var items []json.RawMessage
		if json.Unmarshal(req.Input, &items) != nil || len(items) == 0 {
			return converted, nil, errors.New("input must be a string or nonempty input item array")
		}
		callKinds := map[string]string{}
		callNamespaces := map[string]string{}
		callNames := map[string]string{}
		for _, raw := range items {
			var item struct {
				Type      string          `json:"type"`
				Role      string          `json:"role"`
				Content   json.RawMessage `json:"content"`
				Name      string          `json:"name"`
				CallID    string          `json:"call_id"`
				Arguments string          `json:"arguments"`
				Output    json.RawMessage `json:"output"`
				Input     *string         `json:"input"`
				Namespace string          `json:"namespace"`
			}
			if unmarshalModelJSON(string(raw), &item) != nil {
				return converted, nil, errors.New("invalid input item")
			}
			switch item.Type {
			case "", "message":
				text, err := responseInputText(item.Content)
				if err != nil {
					return converted, nil, err
				}
				switch item.Role {
				case "system", "developer":
					// Preserve privileged instructions in their supplied order.
					var system string
					_ = json.Unmarshal(converted.System, &system)
					converted.System = mustJSON(system + "\n" + text)
				case "user", "assistant":
					converted.Messages = append(converted.Messages, claudeMessage{Role: item.Role, Content: mustJSON(text)})
				default:
					return converted, nil, errors.New("invalid Responses message role")
				}
			case "function_call":
				var args map[string]any
				if item.CallID == "" || item.Name == "" || unmarshalModelJSON(item.Arguments, &args) != nil || args == nil {
					return converted, nil, errors.New("function_call requires call_id, name and JSON object arguments")
				}
				callKinds[item.CallID] = "function_call_output"
				callNamespaces[item.CallID] = item.Namespace
				callNames[item.CallID] = item.Name
				name := (responsesTool{Name: item.Name, namespace: item.Namespace}).qualifiedName()
				converted.Messages = append(converted.Messages, claudeMessage{Role: "assistant", Content: mustJSON([]any{map[string]any{"type": "tool_use", "id": item.CallID, "name": name, "input": args}})})
			case "custom_tool_call":
				if item.Name != "apply_patch" || item.CallID == "" || item.Input == nil {
					return converted, nil, errors.New("invalid custom_tool_call")
				}
				if err := validatePatchSyntax(*item.Input); err != nil {
					return converted, nil, err
				}
				callKinds[item.CallID] = "custom_tool_call_output"
				callNamespaces[item.CallID] = item.Namespace
				callNames[item.CallID] = item.Name
				name := (responsesTool{Name: item.Name, namespace: item.Namespace}).qualifiedName()
				converted.Messages = append(converted.Messages, claudeMessage{Role: "assistant", Content: mustJSON([]any{map[string]any{"type": "tool_use", "id": item.CallID, "name": name, "input": map[string]string{"input": *item.Input}}})})
			case "function_call_output", "custom_tool_call_output":
				if callKinds[item.CallID] != item.Type {
					return converted, nil, errors.New("tool output type and call_id must match a prior call")
				}
				if (item.Namespace != "" && callNamespaces[item.CallID] != item.Namespace) || (item.Name != "" && callNames[item.CallID] != item.Name) {
					return converted, nil, errors.New("tool output namespace/name must match its prior call")
				}
				text, err := responseInputText(item.Output)
				if err != nil {
					return converted, nil, err
				}
				converted.Messages = append(converted.Messages, claudeMessage{Role: "user", Content: mustJSON([]any{map[string]any{"type": "tool_result", "tool_use_id": item.CallID, "content": text}})})
			default:
				return converted, nil, fmt.Errorf("unsupported Responses input item %q", item.Type)
			}
		}
	}
	converted.ValidateResult = func(response claudeResponse) error {
		calls := 0
		for _, block := range response.Content {
			if block.Type != "tool_use" {
				continue
			}
			calls++
			for _, tool := range flatTools {
				properties, _ := tool.Parameters["properties"].(map[string]any)
				if tool.qualifiedName() == block.Name && tool.Name == "exec_command" && properties["justification"] != nil && properties["sandbox_permissions"] != nil {
					if _, present := block.Input["justification"]; present && block.Input["sandbox_permissions"] != "require_escalated" {
						return invalidOutput("exec_command justification requires sandbox_permissions=require_escalated; omit justification for ordinary workspace reads, edits and tests")
					}
				}
				if tool.qualifiedName() != block.Name || tool.Type != "custom" {
					continue
				}
				patch, ok := block.Input["input"].(string)
				if !ok {
					return invalidOutput("apply_patch input must be text")
				}
				if err := validatePatchSyntax(patch); err != nil {
					return invalidOutput(err.Error())
				}
			}
		}
		if req.ParallelToolCalls != nil && !*req.ParallelToolCalls && calls > 1 {
			return invalidOutput("local model violated parallel_tool_calls:false; no calls were emitted")
		}
		return nil
	}
	messages, err := prepareClaude(converted)
	return converted, messages, err
}

func completedResponse(req responsesRequest, response claudeResponse) map[string]any {
	output := []any{}
	flatTools, _ := flattenResponsesTools(req.Tools)
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			output = append(output, map[string]any{"id": newID("msg_"), "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": block.Text, "annotations": []any{}, "logprobs": []any{}}}})
		case "tool_use":
			custom := false
			name, namespace := block.Name, ""
			for _, tool := range flatTools {
				if tool.qualifiedName() == block.Name {
					custom = tool.Type == "custom"
					name = tool.Name
					namespace = tool.namespace
				}
			}
			if custom {
				output = append(output, map[string]any{"id": newID("ctc_"), "type": "custom_tool_call", "call_id": block.ID, "name": name, "input": block.Input["input"], "status": "completed"})
			} else {
				output = append(output, map[string]any{"id": newID("fc_"), "type": "function_call", "call_id": block.ID, "name": name, "arguments": string(mustJSON(block.Input)), "status": "completed"})
			}
			if namespace != "" {
				output[len(output)-1].(map[string]any)["namespace"] = namespace
			}
		}
	}
	var usage any
	// The shared backend reports these totals. Missing backend usage must not
	// become invented token counts; this adapter has no tokenizer of its own.
	input, outputTokens := response.Usage["input_tokens"], response.Usage["output_tokens"]
	if response.UsageKnown {
		usage = map[string]any{"input_tokens": input, "output_tokens": outputTokens, "total_tokens": input + outputTokens}
	}
	choice := any("auto")
	if len(req.ToolChoice) > 0 {
		_ = json.Unmarshal(req.ToolChoice, &choice)
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	parallel := true
	if req.ParallelToolCalls != nil {
		parallel = *req.ParallelToolCalls
	}
	tools := req.Tools
	if tools == nil {
		tools = []responsesTool{}
	}
	return map[string]any{"id": newID("resp_"), "object": "response", "created_at": time.Now().Unix(), "status": "completed", "error": nil, "incomplete_details": nil, "instructions": req.Instructions, "model": req.Model, "output": output, "usage": usage, "metadata": metadata, "store": false, "background": false, "previous_response_id": nil, "max_output_tokens": req.MaxOutputTokens, "parallel_tool_calls": parallel, "tool_choice": choice, "tools": tools, "text": map[string]any{"format": map[string]string{"type": "text"}}, "reasoning": map[string]any{"effort": nil, "summary": nil}, "temperature": 0.1, "top_p": 1.0, "truncation": "disabled"}
}

func streamResponse(w http.ResponseWriter, response map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sequence := 0
	send := func(kind string, data map[string]any) {
		data["type"] = kind
		data["sequence_number"] = sequence
		sequence++
		emit(w, kind, data)
	}
	pending := make(map[string]any, len(response))
	for key, value := range response {
		pending[key] = value
	}
	pending["status"] = "in_progress"
	pending["output"] = []any{}
	pending["usage"] = nil
	send("response.created", map[string]any{"response": pending})
	send("response.in_progress", map[string]any{"response": pending})
	for index, raw := range response["output"].([]any) {
		item := raw.(map[string]any)
		added := make(map[string]any, len(item))
		for key, value := range item {
			added[key] = value
		}
		added["status"] = "in_progress"
		if item["type"] == "message" {
			added["content"] = []any{}
		} else if item["type"] == "custom_tool_call" {
			added["input"] = ""
		} else {
			added["arguments"] = ""
		}
		send("response.output_item.added", map[string]any{"output_index": index, "item": added})
		if item["type"] == "message" {
			for contentIndex, rawPart := range item["content"].([]any) {
				part := rawPart.(map[string]any)
				fields := func() map[string]any {
					return map[string]any{"item_id": item["id"], "output_index": index, "content_index": contentIndex}
				}
				data := fields()
				data["part"] = map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
				send("response.content_part.added", data)
				data = fields()
				data["delta"] = part["text"]
				data["logprobs"] = []any{}
				send("response.output_text.delta", data)
				data = fields()
				data["text"] = part["text"]
				data["logprobs"] = []any{}
				send("response.output_text.done", data)
				data = fields()
				data["part"] = part
				send("response.content_part.done", data)
			}
		} else if item["type"] == "custom_tool_call" {
			send("response.custom_tool_call_input.delta", map[string]any{"item_id": item["id"], "output_index": index, "delta": item["input"]})
			send("response.custom_tool_call_input.done", map[string]any{"item_id": item["id"], "output_index": index, "input": item["input"]})
		} else {
			send("response.function_call_arguments.delta", map[string]any{"item_id": item["id"], "output_index": index, "delta": item["arguments"]})
			send("response.function_call_arguments.done", map[string]any{"item_id": item["id"], "output_index": index, "arguments": item["arguments"], "name": item["name"]})
		}
		send("response.output_item.done", map[string]any{"output_index": index, "item": item})
	}
	send("response.completed", map[string]any{"response": response})
}

func (g *Gateway) responses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		responsesError(w, 405, "Use POST")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, g.cfg.MaxRequestBytes))
	_ = r.Body.Close()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			responsesError(w, 413, "Request exceeds configured body limit")
		} else {
			responsesError(w, 400, "Cannot read request body")
		}
		return
	}
	var req responsesRequest
	trimmed := strings.TrimSpace(string(body))
	if !strings.HasPrefix(trimmed, "{") || unmarshalModelJSON(string(body), &req) != nil {
		responsesError(w, 400, "Expected a Responses JSON object")
		return
	}
	converted, messages, err := prepareResponses(req, len(g.roles) > 0, g.cfg.ClaudeMaxTokens)
	if err != nil {
		responsesError(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(withTrainingClient(r.Context(), "openai_responses"), g.cfg.Timeout)
	defer cancel()
	response, err := g.inferClaude(ctx, converted, messages)
	if err != nil {
		status, _ := outputErrorKind(err)
		responsesError(w, status, err.Error())
		return
	}
	completed := completedResponse(req, response)
	if req.Stream {
		streamResponse(w, completed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(completed)
}
