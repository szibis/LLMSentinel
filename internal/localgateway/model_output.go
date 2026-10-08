package localgateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

var toolActionPreamble = regexp.MustCompile(`(?i)^(i('ll| will| am|'m)|let me)\b`)
var toolExamplePreamble = regexp.MustCompile(`(?i)\b(example|sample|quoted?|demonstration)\b`)

// Normalize recognized model formats into one internal envelope. Tool names,
// schemas and client policy are still validated before any client tool is emitted.
func normalizeModelOutput(req claudeRequest, raw string) (localToolEnvelope, error) {
	out := localToolEnvelope{Calls: []localToolCall{}}
	if !utf8.ValidString(raw) {
		return out, errors.New("invalid model UTF-8")
	}
	value := strings.TrimSpace(raw)
	if req.ToolFormat == "gemma4" && (strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")) {
		// Native Gemma frames carry executable calls. A user's requested JSON
		// answer may contain protocol-looking field names and remains data.
		var answer any
		if err := unmarshalModelJSON(value, &answer); err != nil {
			return out, fmt.Errorf("invalid final JSON: %w", err)
		}
		out.Text = raw
		return out, nil
	}
	if req.JSONTools && toolActionPreamble.MatchString(value) {
		if start := strings.Index(value, "{"); start > 0 && strings.Contains(value[start:], `"tool_calls"`) && !toolExamplePreamble.MatchString(value[:start]) {
			prefix := strings.TrimSpace(value[:start])
			candidate := strings.TrimSpace(value[start:])
			if strings.HasSuffix(prefix, "```json") && strings.HasSuffix(candidate, "```") {
				prefix = strings.TrimSpace(strings.TrimSuffix(prefix, "```json"))
				candidate = strings.TrimSpace(strings.TrimSuffix(candidate, "```"))
			}
			parsed, err := normalizeModelOutput(req, candidate)
			if err != nil {
				return out, err
			}
			if len(parsed.Calls) == 0 {
				return out, errors.New("action preamble must precede a complete tool envelope")
			}
			if parsed.Text == "" {
				parsed.Text = prefix
			}
			return parsed, nil
		}
	}
	jsonValue := strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")
	taggedStart := strings.Index(value, "<tool_call>")
	gemmaStart := strings.Index(value, gemmaCallStart)
	gemmaOuter := taggedStart < 0 || (gemmaStart >= 0 && gemmaStart < taggedStart)
	if !jsonValue && gemmaOuter && (strings.Contains(value, "<|tool_call") || strings.Contains(value, "<tool_call|") || strings.Contains(value, "<|tool_response>")) {
		return parseGemmaToolOutput(value)
	}
	if !jsonValue && (strings.Contains(value, "<tool_call") || strings.Contains(value, "<function=") || strings.Contains(value, "<parameter=") || strings.Contains(value, "</tool_call>")) {
		first := strings.Index(value, "<tool_call>")
		if first < 0 {
			return out, errors.New("incomplete tagged tool call")
		}
		if strings.HasPrefix(strings.TrimSpace(value[first+len("<tool_call>"):]), "<function=") {
			return parseQwenToolOutput(value, req.Tools)
		}
		out.Text = strings.TrimSpace(value[:first])
		remaining := value[first:]
		for remaining != "" {
			if len(out.Calls) >= 8 || !strings.HasPrefix(remaining, "<tool_call>") {
				return out, errors.New("invalid tagged tool suffix or count")
			}
			remaining = strings.TrimPrefix(remaining, "<tool_call>")

			var call map[string]any
			decoder := json.NewDecoder(strings.NewReader(remaining))
			if decoder.Decode(&call) != nil {
				return out, errors.New("invalid tagged tool JSON")
			}
			end := int(decoder.InputOffset())
			if unmarshalModelJSON(remaining[:end], &call) != nil {
				return out, errors.New("invalid tagged tool JSON")
			}
			suffix := strings.TrimSpace(remaining[end:])
			if !strings.HasPrefix(suffix, "</tool_call>") {
				return out, errors.New("incomplete tagged tool call")
			}
			parsed, err := normalizeToolCall(call)
			if err != nil {
				return out, err
			}
			out.Calls = append(out.Calls, parsed)
			remaining = strings.TrimSpace(strings.TrimPrefix(suffix, "</tool_call>"))
		}
		return out, nil
	}
	if !strings.HasPrefix(value, "{") && !strings.HasPrefix(value, "[") {
		out.Text = raw
		return out, nil
	}
	var decoded any
	if err := unmarshalModelJSON(value, &decoded); err != nil {
		return out, fmt.Errorf("invalid model JSON: %w", err)
	}
	if object, ok := decoded.(map[string]any); ok {
		if calls, present := object["tool_calls"]; present {
			if content, present := object["content"]; present && content != nil {
				text, ok := content.(string)
				if !ok {
					return out, errors.New("ambiguous model envelope")
				}
				if _, competing := object["text"]; competing {
					return out, errors.New("ambiguous model envelope text")
				}
				out.Text = text
			}
			if _, competing := object["arguments"]; competing {
				return out, errors.New("ambiguous model envelope")
			}
			items, ok := calls.([]any)
			if !ok || len(items) > 8 {
				return out, errors.New("tool_calls must be an array")
			}
			if text, present := object["text"]; present {
				var ok bool
				out.Text, ok = text.(string)
				if !ok {
					return out, errors.New("tool envelope text must be a string")
				}
			}
			for _, item := range items {
				call, ok := item.(map[string]any)
				if !ok {
					return out, errors.New("tool call must be an object")
				}
				parsed, err := normalizeToolCall(call)
				if err != nil {
					return out, err
				}
				out.Calls = append(out.Calls, parsed)
			}
			return out, nil
		}
		if content, ok := object["content"].([]any); ok {
			if _, competing := object["arguments"]; competing {
				return out, errors.New("ambiguous model envelope")
			}
			return normalizeContentBlocks(content)
		}
		if _, present := object["arguments"]; present {
			call, err := normalizeToolCall(object)
			out.Calls = append(out.Calls, call)
			return out, err
		}
	}
	out.Text = raw
	return out, nil
}

func normalizeToolCall(object map[string]any) (localToolCall, error) {
	var out localToolCall
	if function, present := object["function"]; present {
		_, hasName := object["name"]
		_, hasInput := object["input"]
		_, hasArguments := object["arguments"]
		if hasName || hasInput || hasArguments || (object["type"] != nil && object["type"] != "function") {
			return out, errors.New("ambiguous function call")
		}
		var ok bool
		object, ok = function.(map[string]any)
		if !ok {
			return out, errors.New("function must be an object")
		}
	}
	name, ok := object["name"].(string)
	if !ok || name == "" {
		return out, errors.New("tool name must be a nonempty string")
	}
	input, hasInput := object["input"]
	arguments, hasArguments := object["arguments"]
	if hasInput == hasArguments {
		return out, errors.New("tool call requires one argument representation")
	}
	if hasArguments {
		input = arguments
		if serialized, ok := arguments.(string); ok {
			if unmarshalModelJSON(serialized, &input) != nil {
				return out, errors.New("invalid serialized tool arguments")
			}
		}
	}
	args, ok := input.(map[string]any)
	if !ok || args == nil {
		return out, errors.New("tool arguments must be an object")
	}
	return localToolCall{Name: name, Input: args}, nil
}

func unmarshalModelJSON(raw string, target any) error {
	if !utf8.ValidString(raw) {
		return errors.New("invalid UTF-8 JSON")
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	var check func(int) error
	check = func(depth int) error {
		if depth > 128 {
			return errors.New("model JSON nesting limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate model JSON key")
				}
				seen[name] = true
			}
			if err := check(depth + 1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := check(0); err != nil {
		return err
	}
	// encoding/json also matches struct tags case-insensitively. Prevent a
	// differently cased protocol key from overriding a canonical field, while
	// preserving case-sensitive keys in arbitrary dictionaries and tool data.
	decoder = json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return canonicalProtocolKeys(value, reflect.TypeOf(target))
}

func canonicalProtocolKeys(value any, kind reflect.Type) error {
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	switch kind.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		for i := 0; i < kind.NumField(); i++ {
			field := kind.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			for key := range object {
				if key != name && strings.EqualFold(key, name) {
					return errors.New("noncanonical protocol JSON field")
				}
			}
			if child, exists := object[name]; exists {
				if err := canonicalProtocolKeys(child, field.Type); err != nil {
					return err
				}
			}
		}
	case reflect.Slice, reflect.Array:
		if kind.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		if items, ok := value.([]any); ok {
			for _, child := range items {
				if err := canonicalProtocolKeys(child, kind.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func normalizeContentBlocks(blocks []any) (localToolEnvelope, error) {
	out := localToolEnvelope{Calls: []localToolCall{}}
	var text []string
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return out, errors.New("content block must be an object")
		}
		switch block["type"] {
		case "text":
			value, ok := block["text"].(string)
			if !ok {
				return out, errors.New("text block must contain text")
			}
			text = append(text, value)
		case "tool_use":
			if len(out.Calls) >= 8 {
				return out, errors.New("too many model tool calls")
			}
			call, err := normalizeToolCall(block)
			if err != nil {
				return out, err
			}
			out.Calls = append(out.Calls, call)
		default:
			return out, errors.New("unsupported model content block")
		}
	}
	out.Text = strings.Join(text, "\n")
	return out, nil
}
