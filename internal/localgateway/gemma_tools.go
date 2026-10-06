package localgateway

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

const gemmaCallStart = "<|tool_call>"
const gemmaCallEnd = "<tool_call|>"
const gemmaString = `<|"|>`

var gemmaIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// Gemma's native syntax is data, never executable code. Parse balanced values
// rather than replacing punctuation or evaluating Python expressions.
func parseGemmaToolOutput(raw string) (localToolEnvelope, error) {
	out := localToolEnvelope{Calls: []localToolCall{}}
	first := strings.Index(raw, gemmaCallStart)
	if first < 0 {
		return out, errors.New("incomplete Gemma tool frame")
	}
	out.Text = strings.TrimSpace(raw[:first])
	p := gemmaParser{source: raw, position: first}
	for {
		p.space()
		if p.position == len(raw) {
			return out, nil
		}
		if p.take("<|tool_response>") {
			p.space()
			if p.position != len(raw) {
				return out, errors.New("model invented a Gemma tool response")
			}
			return out, nil
		}
		if len(out.Calls) == 8 || !p.take(gemmaCallStart) || !p.take("call:") {
			return out, errors.New("invalid Gemma call suffix or count")
		}
		start := p.position
		for p.position < len(raw) && raw[p.position] != '{' {
			p.position++
		}
		name := strings.TrimSpace(raw[start:p.position])
		if !gemmaIdentifier.MatchString(name) {
			return out, errors.New("invalid Gemma function name")
		}
		args, err := p.value(0)
		if err != nil {
			return out, err
		}
		object, ok := args.(map[string]any)
		if !ok {
			return out, errors.New("gemma arguments must be an object")
		}
		p.space()
		if !p.take(gemmaCallEnd) {
			return out, errors.New("incomplete Gemma call frame")
		}
		out.Calls = append(out.Calls, localToolCall{Name: name, Input: object})
	}
}

type gemmaParser struct {
	source   string
	position int
}

func (p *gemmaParser) space() {
	for p.position < len(p.source) && strings.ContainsRune(" \t\r\n", rune(p.source[p.position])) {
		p.position++
	}
}

func (p *gemmaParser) take(value string) bool {
	if !strings.HasPrefix(p.source[p.position:], value) {
		return false
	}
	p.position += len(value)
	return true
}

func (p *gemmaParser) quoted() (string, error) {
	if p.take(gemmaString) {
		end := strings.Index(p.source[p.position:], gemmaString)
		if end < 0 {
			return "", errors.New("incomplete Gemma string")
		}
		value := p.source[p.position : p.position+end]
		p.position += end + len(gemmaString)
		return value, nil
	}
	var value string
	decoder := json.NewDecoder(strings.NewReader(p.source[p.position:]))
	if err := decoder.Decode(&value); err != nil {
		return "", errors.New("invalid Gemma quoted string")
	}
	p.position += int(decoder.InputOffset())
	return value, nil
}

func (p *gemmaParser) value(depth int) (any, error) {
	p.space()
	if depth > 64 || p.position == len(p.source) {
		return nil, errors.New("incomplete or deeply nested Gemma value")
	}
	if strings.HasPrefix(p.source[p.position:], gemmaString) || p.source[p.position] == '"' {
		return p.quoted()
	}
	if p.take("{") {
		result := map[string]any{}
		p.space()
		if p.take("}") {
			return result, nil
		}
		for {
			p.space()
			var key string
			if p.position < len(p.source) && (strings.HasPrefix(p.source[p.position:], gemmaString) || p.source[p.position] == '"') {
				var err error
				key, err = p.quoted()
				if err != nil {
					return nil, err
				}
			} else {
				start := p.position
				for p.position < len(p.source) && p.source[p.position] != ':' {
					p.position++
				}
				key = strings.TrimSpace(p.source[start:p.position])
				if !gemmaIdentifier.MatchString(key) {
					return nil, errors.New("invalid Gemma argument key")
				}
			}
			if _, exists := result[key]; exists {
				return nil, errors.New("duplicate Gemma argument key")
			}
			p.space()
			if !p.take(":") {
				return nil, errors.New("missing Gemma argument separator")
			}
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			result[key] = value
			p.space()
			if p.take("}") {
				return result, nil
			}
			if !p.take(",") {
				return nil, errors.New("incomplete Gemma argument object")
			}
		}
	}
	if p.take("[") {
		result := []any{}
		p.space()
		if p.take("]") {
			return result, nil
		}
		for {
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
			p.space()
			if p.take("]") {
				return result, nil
			}
			if !p.take(",") {
				return nil, errors.New("incomplete Gemma argument array")
			}
		}
	}
	start := p.position
	for p.position < len(p.source) && !strings.ContainsRune(",}]", rune(p.source[p.position])) {
		p.position++
	}
	raw := strings.TrimSpace(p.source[start:p.position])
	if raw == "" || strings.Contains(raw, "<|") || strings.Contains(raw, "<tool_") {
		return nil, errors.New("invalid Gemma scalar")
	}
	var scalar any
	if json.Unmarshal([]byte(raw), &scalar) == nil {
		return scalar, nil
	}
	// Gemma also emits bare string literals, including paths. Preserve their
	// bytes; the registered tool schema determines whether a string is valid.
	return raw, nil
}
