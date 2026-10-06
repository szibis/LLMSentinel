package qwensmoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

func positiveUsage(u document, input, output string, total bool) error {
	a, ok := u[input].(float64)
	b, ok2 := u[output].(float64)
	if !ok || !ok2 || a <= 0 || b <= 0 || math.Trunc(a) != a || math.Trunc(b) != b {
		return errors.New("missing or invalid positive native token usage")
	}
	if total && u["total_tokens"] != a+b {
		return errors.New("inconsistent total token usage")
	}
	return nil
}

func noThoughtLeak(value any) bool {
	switch v := value.(type) {
	case string:
		for _, tag := range []string{"<think>", "</think>", "<|channel>", "<channel|>"} {
			if strings.Contains(v, tag) {
				return false
			}
		}
	case document:
		for k, child := range v {
			if (k == "reasoning_content" || k == "thinking") && child != nil && child != "" {
				return false
			}
			if !noThoughtLeak(child) {
				return false
			}
		}
	case map[string]any:
		return noThoughtLeak(document(v))
	case []any:
		for _, child := range v {
			if !noThoughtLeak(child) {
				return false
			}
		}
	}
	return true
}

func validateProofJSON(protocol string, d document, tool bool) (document, error) {
	if !noThoughtLeak(d) {
		return nil, errors.New("raw reasoning leaked into provider response")
	}
	var answer strings.Builder
	var calls []document
	var stop string
	switch protocol {
	case "messages":
		if d["type"] != "message" || d["role"] != "assistant" || text(d["id"]) == "" {
			return nil, errors.New("invalid Messages identity")
		}
		stop = text(d["stop_reason"])
		for _, raw := range list(d["content"]) {
			b := mapping(raw)
			switch b["type"] {
			case "text":
				answer.WriteString(text(b["text"]))
			case "tool_use":
				calls = append(calls, b)
			default:
				return nil, errors.New("unexpected Messages block")
			}
		}
	case "responses":
		if d["status"] != "completed" || text(d["id"]) == "" {
			return nil, errors.New("responses did not complete")
		}
		stop = "end_turn"
		for _, raw := range list(d["output"]) {
			b := mapping(raw)
			switch b["type"] {
			case "message":
				if b["role"] != "assistant" || b["status"] != "completed" {
					return nil, errors.New("invalid output message")
				}
				for _, part := range list(b["content"]) {
					p := mapping(part)
					if p["type"] != "output_text" {
						return nil, errors.New("unexpected output part")
					}
					answer.WriteString(text(p["text"]))
				}
			case "function_call":
				var args document
				if json.Unmarshal([]byte(text(b["arguments"])), &args) != nil {
					return nil, errors.New("invalid function arguments")
				}
				calls = append(calls, document{"id": b["call_id"], "name": b["name"], "input": args})
				stop = "tool_use"
			default:
				return nil, errors.New("unexpected Responses output")
			}
		}
	case "chat":
		if d["object"] != "chat.completion" || text(d["id"]) == "" || len(list(d["choices"])) != 1 {
			return nil, errors.New("invalid Chat identity or choices")
		}
		c := mapping(list(d["choices"])[0])
		m := mapping(c["message"])
		if m["role"] != "assistant" {
			return nil, errors.New("invalid Chat role")
		}
		answer.WriteString(text(m["content"]))
		stop = text(c["finish_reason"])
		if stop == "stop" {
			stop = "end_turn"
		} else if stop == "tool_calls" {
			stop = "tool_use"
		}
		for _, raw := range list(m["tool_calls"]) {
			b := mapping(raw)
			f := mapping(b["function"])
			var args document
			if b["type"] != "function" || json.Unmarshal([]byte(text(f["arguments"])), &args) != nil {
				return nil, errors.New("invalid Chat function")
			}
			calls = append(calls, document{"id": b["id"], "name": f["name"], "input": args})
		}
	default:
		return nil, errors.New("unknown protocol")
	}
	u := mapping(d["usage"])
	input, output := "input_tokens", "output_tokens"
	if protocol == "chat" {
		input, output = "prompt_tokens", "completion_tokens"
	}
	if err := positiveUsage(u, input, output, protocol != "messages"); err != nil {
		return nil, err
	}
	if tool {
		if stop != "tool_use" || len(calls) != 1 || text(calls[0]["id"]) == "" || calls[0]["name"] != "record_marker" || len(mapping(calls[0]["input"])) != 1 || mapping(calls[0]["input"])["marker"] != marker || strings.TrimSpace(answer.String()) != "" {
			return nil, errors.New("required tool arguments or stop semantics mismatch")
		}
	} else if stop != "end_turn" || len(calls) != 0 || strings.TrimSpace(answer.String()) != marker {
		return nil, errors.New("final marker mismatch or reasoning leak")
	}
	return document{"usage": u, "stop": stop, "tool_calls": calls, "final_marker": !tool}, nil
}

// SSE is decoded as framed events, never accepted by substring matching.
func validateProofSSE(protocol string, body []byte, requiredTool ...bool) (document, error) {
	tool := len(requiredTool) > 0 && requiredTool[0]
	s := strings.ReplaceAll(string(body), "\r\n", "\n")
	if !strings.HasSuffix(s, "\n\n") {
		return nil, errors.New("truncated SSE frame")
	}
	frames := strings.Split(strings.TrimSuffix(s, "\n\n"), "\n\n")
	events := []string{}
	var answer strings.Builder
	var usage document
	var final document
	var arguments strings.Builder
	call := document{}
	id := ""
	stop := ""
	started, ended, block, blockDone := false, false, false, false
	responsePhase := 0
	responseState := responseSSEState{}
	responseOrder := []string{"response.created", "response.in_progress", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}
	if tool {
		responseOrder = []string{"response.created", "response.in_progress", "response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.output_item.done", "response.completed"}
	}
	expectedStop := "end_turn"
	chatStop := "stop"
	if tool {
		expectedStop = "tool_use"
		chatStop = "tool_calls"
	}
	for _, frame := range frames {
		if frame == "" {
			continue
		}
		var name string
		data := []string{}
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, ":") {
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				name = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = append(data, strings.TrimPrefix(line, "data: "))
			} else {
				return nil, errors.New("invalid SSE field")
			}
		}
		if len(data) == 0 {
			continue
		}
		raw := strings.Join(data, "\n")
		if ended {
			return nil, errors.New("SSE data after terminal event")
		}
		if raw == "[DONE]" {
			if protocol != "chat" || !started || stop != chatStop || usage == nil {
				return nil, errors.New("premature Chat DONE")
			}
			ended = true
			events = append(events, "[DONE]")
			continue
		}
		d := document{}
		if json.Unmarshal([]byte(raw), &d) != nil {
			return nil, errors.New("malformed SSE JSON")
		}
		if !noThoughtLeak(d) {
			return nil, errors.New("raw reasoning leaked into provider SSE")
		}
		typ := text(d["type"])
		if protocol != "chat" && name != typ {
			return nil, errors.New("SSE event/type mismatch")
		}
		events = append(events, name)
		switch protocol {
		case "messages":
			switch typ {
			case "message_start":
				if started {
					return nil, errors.New("duplicate message start")
				}
				started = true
				id = text(mapping(d["message"])["id"])
			case "ping":
				if !started {
					return nil, errors.New("ping before start")
				}
			case "content_block_start":
				blockType := "text"
				if tool {
					blockType = "tool_use"
				}
				if !started || block || blockDone || d["index"] != float64(0) || mapping(d["content_block"])["type"] != blockType {
					return nil, errors.New("invalid content block start")
				}
				if tool {
					b := mapping(d["content_block"])
					call = document{"id": b["id"], "name": b["name"]}
					if len(mapping(b["input"])) != 0 {
						return nil, errors.New("tool block input must start empty")
					}
				}
				block = true
			case "content_block_delta":
				deltaType := "text_delta"
				if tool {
					deltaType = "input_json_delta"
				}
				if !block || d["index"] != float64(0) || mapping(d["delta"])["type"] != deltaType {
					return nil, errors.New("delta outside text block")
				}
				if tool {
					arguments.WriteString(text(mapping(d["delta"])["partial_json"]))
				} else {
					answer.WriteString(text(mapping(d["delta"])["text"]))
				}
			case "content_block_stop":
				if !block || d["index"] != float64(0) {
					return nil, errors.New("invalid content block stop")
				}
				block = false
				blockDone = true
			case "message_delta":
				if !blockDone || stop != "" {
					return nil, errors.New("invalid message delta order")
				}
				stop = text(mapping(d["delta"])["stop_reason"])
				usage = mapping(d["usage"])
			case "message_stop":
				if stop != expectedStop || block {
					return nil, errors.New("premature message stop")
				}
				ended = true
			default:
				return nil, errors.New("unexpected Messages event")
			}
		case "responses":
			if err := responseState.validate(d, tool, answer.String(), arguments.String()); err != nil {
				return nil, err
			}
			if (typ == "response.output_text.delta" && !tool && responsePhase == 5) || (typ == "response.function_call_arguments.delta" && tool && responsePhase == 4) {
				// More than one text delta is legal within the same content part.
			} else if responsePhase >= len(responseOrder) || typ != responseOrder[responsePhase] {
				return nil, errors.New("invalid Responses lifecycle order")
			} else {
				responsePhase++
			}
			if !started {
				if typ != "response.created" {
					return nil, errors.New("responses missing created")
				}
				started = true
				id = text(mapping(d["response"])["id"])
			} else if typ == "response.created" {
				return nil, errors.New("duplicate Responses created")
			}
			switch typ {
			case "response.output_item.added":
				if tool {
					b := mapping(d["item"])
					if b["type"] != "function_call" {
						return nil, errors.New("unexpected streamed tool item")
					}
					call = document{"id": b["call_id"], "name": b["name"]}
				}
			case "response.function_call_arguments.delta":
				arguments.WriteString(text(d["delta"]))
			case "response.function_call_arguments.done":
				if text(d["arguments"]) != arguments.String() {
					return nil, errors.New("streamed tool arguments disagree with done event")
				}
			case "response.output_text.delta":
				answer.WriteString(text(d["delta"]))
			case "response.completed":
				final = mapping(d["response"])
				if final["id"] != id {
					return nil, errors.New("responses identity changed")
				}
				ended = true
			case "response.created", "response.in_progress", "response.content_part.added", "response.output_text.done", "response.content_part.done", "response.output_item.done":
			default:
				return nil, errors.New("unexpected Responses event")
			}
		case "chat":
			if d["object"] != "chat.completion.chunk" || text(d["id"]) == "" {
				return nil, errors.New("invalid Chat chunk")
			}
			if !started {
				choices := list(d["choices"])
				if len(choices) != 1 || mapping(mapping(choices[0])["delta"])["role"] != "assistant" {
					return nil, errors.New("chat stream missing assistant start")
				}
				started = true
				id = text(d["id"])
			} else if d["id"] != id {
				return nil, errors.New("chat identity changed")
			}
			if len(list(d["choices"])) == 0 {
				if stop != chatStop {
					return nil, errors.New("usage before Chat stop")
				}
				usage = mapping(d["usage"])
				continue
			}
			if len(list(d["choices"])) != 1 || stop != "" {
				return nil, errors.New("invalid Chat choices order")
			}
			c := mapping(list(d["choices"])[0])
			delta := mapping(c["delta"])
			if len(list(delta["tool_calls"])) > 0 {
				if !tool || len(list(delta["tool_calls"])) != 1 {
					return nil, errors.New("unexpected Chat tool delta")
				}
				b := mapping(list(delta["tool_calls"])[0])
				if b["index"] != float64(0) {
					return nil, errors.New("invalid Chat tool index")
				}
				f := mapping(b["function"])
				if b["id"] != nil {
					if call["id"] != nil && call["id"] != b["id"] {
						return nil, errors.New("chat tool identity changed")
					}
					call["id"] = b["id"]
				}
				if f["name"] != nil {
					if call["name"] != nil && call["name"] != f["name"] {
						return nil, errors.New("chat tool name changed")
					}
					call["name"] = f["name"]
				}
				if b["type"] != nil && b["type"] != "function" {
					return nil, errors.New("invalid Chat tool type")
				}
				arguments.WriteString(text(f["arguments"]))
			}
			answer.WriteString(text(delta["content"]))
			if c["finish_reason"] != nil {
				stop = text(c["finish_reason"])
			}
		default:
			return nil, errors.New("unknown SSE protocol")
		}
	}
	if !started || !ended || id == "" || (!tool && strings.TrimSpace(answer.String()) != marker) || (tool && strings.TrimSpace(answer.String()) != "") {
		return nil, errors.New("incomplete SSE lifecycle or reasoning leak")
	}
	if protocol == "responses" {
		v, err := validateProofJSON(protocol, final, tool)
		if err != nil {
			return nil, err
		}
		if tool {
			calls := v["tool_calls"].([]document)
			var args document
			if json.Unmarshal([]byte(arguments.String()), &args) != nil || len(args) != 1 || args["marker"] != marker || len(calls) != 1 || calls[0]["id"] != call["id"] || calls[0]["name"] != call["name"] {
				return nil, errors.New("streamed tool differs from final output")
			}
		}
		v["events"] = events
		return v, nil
	}
	input, output := "input_tokens", "output_tokens"
	if protocol == "chat" {
		input, output = "prompt_tokens", "completion_tokens"
	}
	if err := positiveUsage(usage, input, output, protocol == "chat"); err != nil {
		return nil, err
	}
	if tool {
		var args document
		if json.Unmarshal([]byte(arguments.String()), &args) != nil || len(args) != 1 || args["marker"] != marker || text(call["id"]) == "" || call["name"] != "record_marker" {
			return nil, errors.New("invalid streamed tool arguments")
		}
		call["input"] = args
		return document{"events": events, "usage": usage, "stop": "tool_use", "tool_calls": []document{call}, "final_marker": false}, nil
	}
	return document{"events": events, "usage": usage, "stop": stop, "final_marker": true}, nil
}

// responseSSEState verifies the payload clients need to assemble the single
// synthetic output, in addition to the lifecycle ordering checked above.
type responseSSEState struct {
	sequence   int
	responseID string
	added      document
	done       document
	part       document
}

func emptyJSONArray(value any) bool { a, ok := value.([]any); return ok && len(a) == 0 }

func responseTextPart(part document, expected string) bool {
	value, ok := part["text"].(string)
	return part["type"] == "output_text" && ok && value == expected && emptyJSONArray(part["annotations"]) && emptyJSONArray(part["logprobs"])
}

func (s *responseSSEState) validate(d document, tool bool, answer, arguments string) error {
	if d["sequence_number"] != float64(s.sequence) {
		return errors.New("invalid responses SSE sequence")
	}
	s.sequence++
	typ := text(d["type"])
	if typ == "response.created" || typ == "response.in_progress" {
		response := mapping(d["response"])
		if text(response["id"]) == "" || response["object"] != "response" || response["status"] != "in_progress" || !emptyJSONArray(response["output"]) || response["usage"] != nil {
			return errors.New("invalid pending responses identity or shape")
		}
		if typ == "response.created" {
			s.responseID = text(response["id"])
		} else if response["id"] != s.responseID {
			return errors.New("responses pending identity changed")
		}
		return nil
	}
	if typ == "response.completed" {
		response := mapping(d["response"])
		output := list(response["output"])
		if response["id"] != s.responseID || response["object"] != "response" || s.done == nil || len(output) != 1 || !reflect.DeepEqual(mapping(output[0]), s.done) {
			return errors.New("responses completed output differs from done item")
		}
		return nil
	}
	if d["output_index"] != float64(0) {
		return errors.New("missing or mismatched responses output index")
	}
	if typ == "response.output_item.added" {
		item := mapping(d["item"])
		if text(item["id"]) == "" || item["status"] != "in_progress" {
			return errors.New("invalid added responses item identity or status")
		}
		if tool {
			if item["type"] != "function_call" || text(item["call_id"]) == "" || item["name"] != "record_marker" || item["arguments"] != "" {
				return errors.New("invalid added responses function item")
			}
		} else if item["type"] != "message" || item["role"] != "assistant" || !emptyJSONArray(item["content"]) {
			return errors.New("invalid added responses message item")
		}
		s.added = item
		return nil
	}
	if s.added == nil {
		return errors.New("responses event preceded added item")
	}
	if typ == "response.output_item.done" {
		item := mapping(d["item"])
		if item["id"] != s.added["id"] || item["type"] != s.added["type"] || item["status"] != "completed" {
			return errors.New("responses done item identity or status mismatch")
		}
		if tool {
			if item["call_id"] != s.added["call_id"] || item["name"] != s.added["name"] || item["arguments"] != arguments {
				return errors.New("responses done function arguments or identity mismatch")
			}
		} else {
			parts := list(item["content"])
			if item["role"] != "assistant" || s.part == nil || len(parts) != 1 || !reflect.DeepEqual(mapping(parts[0]), s.part) {
				return errors.New("responses done message differs from done part")
			}
		}
		s.done = item
		return nil
	}
	if d["item_id"] != s.added["id"] {
		return errors.New("missing or mismatched responses item identity")
	}
	if tool {
		switch typ {
		case "response.function_call_arguments.delta":
			if _, ok := d["delta"].(string); !ok {
				return errors.New("missing responses function argument delta")
			}
		case "response.function_call_arguments.done":
			if d["name"] != s.added["name"] || d["arguments"] != arguments {
				return errors.New("responses function done name or arguments mismatch")
			}
		default:
			return errors.New("unexpected responses function payload")
		}
		return nil
	}
	if d["content_index"] != float64(0) {
		return errors.New("missing or mismatched responses content index")
	}
	switch typ {
	case "response.content_part.added":
		if !responseTextPart(mapping(d["part"]), "") {
			return errors.New("invalid added responses text part")
		}
	case "response.output_text.delta":
		if _, ok := d["delta"].(string); !ok || !emptyJSONArray(d["logprobs"]) {
			return errors.New("invalid responses text delta")
		}
	case "response.output_text.done":
		if d["text"] != answer || !emptyJSONArray(d["logprobs"]) {
			return errors.New("responses done text differs from deltas")
		}
	case "response.content_part.done":
		part := mapping(d["part"])
		if !responseTextPart(part, answer) {
			return errors.New("responses done part differs from deltas")
		}
		s.part = part
	default:
		return errors.New("unexpected responses content payload")
	}
	return nil
}

func proofExchange(ctx context.Context, endpoint string, payload document, session string) (int, string, []byte, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return 0, "", nil, errors.New("proof requires loopback HTTP")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return 0, "", nil, errors.New("invalid proof request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", session)
	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := client.Do(req)
	if err != nil {
		return 0, "", nil, errors.New("proof request failed or timed out")
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20+1))
	if err != nil || len(body) > 2<<20 {
		return r.StatusCode, r.Header.Get("Content-Type"), nil, errors.New("proof response oversized or unreadable")
	}
	return r.StatusCode, r.Header.Get("Content-Type"), body, nil
}

func proofPayload(protocol, role string, stream, tool bool) document {
	prompt := "Reply with exactly " + marker + " and no other final text."
	if tool {
		prompt = "Call record_marker once with marker " + marker + ". Do not answer in text."
	}
	p := document{"model": "sentinel-" + role, "stream": stream}
	schema := document{"type": "object", "properties": document{"marker": document{"type": "string", "enum": []string{marker}}}, "required": []string{"marker"}, "additionalProperties": false}
	switch protocol {
	case "messages":
		p["max_tokens"] = 8192
		p["messages"] = []any{document{"role": "user", "content": prompt}}
		if tool {
			p["tools"] = []any{document{"name": "record_marker", "description": "Record synthetic marker, no side effects.", "input_schema": schema}}
			p["tool_choice"] = document{"type": "tool", "name": "record_marker"}
		}
	case "responses":
		p["max_output_tokens"] = 8192
		p["input"] = prompt
		p["store"] = false
		if tool {
			p["tools"] = []any{document{"type": "function", "name": "record_marker", "description": "Record synthetic marker, no side effects.", "parameters": schema}}
			p["tool_choice"] = document{"type": "function", "name": "record_marker"}
			p["parallel_tool_calls"] = false
		}
	case "chat":
		p["max_tokens"] = 8192
		p["messages"] = []any{document{"role": "user", "content": prompt}}
		p["stream_options"] = document{"include_usage": true}
		if tool {
			p["tools"] = []any{document{"type": "function", "function": document{"name": "record_marker", "description": "Record synthetic marker, no side effects.", "parameters": schema}}}
			p["tool_choice"] = document{"type": "function", "function": document{"name": "record_marker"}}
		}
	}
	return p
}

func runProtocolProof(ctx context.Context, base, size, role, protocol string, stream, tool bool, payload document, result document, nativeBase ...string) error {
	endpoint := map[string]string{"messages": "/v1/messages", "responses": "/v1/responses", "chat": "/v1/chat/completions"}[protocol]
	if payload == nil {
		payload = proofPayload(protocol, role, stream, tool)
	}
	name := size + "-" + role + "-" + protocol + "-json"
	if stream {
		name = size + "-" + role + "-" + protocol + "-sse"
	}
	if tool {
		name += "-required-tool"
	}
	if payload["proof_continuation"] == true {
		name += "-tool-result"
		delete(payload, "proof_continuation")
	}
	check := document{"name": name, "endpoint": endpoint, "role": role, "model_size": size, "passed": false, "synthetic_request": payload, "assertions": []string{"provider lifecycle and stop semantics", "exact marker or exact tool arguments", "positive native input/output usage", "no raw reasoning"}}
	result["checks"] = append(result["checks"].([]any), check)
	status, contentType, body, err := proofExchange(ctx, base+endpoint, payload, "sentinel-integration-"+size+"-"+role)
	check["http_status"] = status
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("proof HTTP status %d", status)
	}
	var validated document
	if err == nil {
		if stream {
			if !strings.HasPrefix(contentType, "text/event-stream") {
				err = errors.New("missing SSE content type")
			} else {
				validated, err = validateProofSSE(protocol, body, tool)
			}
		} else {
			if !strings.HasPrefix(contentType, "application/json") {
				err = errors.New("missing JSON content type")
			} else {
				d := document{}
				if json.Unmarshal(body, &d) != nil {
					err = errors.New("invalid JSON proof")
				} else {
					validated, err = validateProofJSON(protocol, d, tool)
				}
			}
		}
	}
	// Only validated public synthetic fields are persisted; arbitrary backend text
	// (including error bodies and thoughts) never enters an artifact.
	if err != nil {
		check["error"] = err.Error()
		return fmt.Errorf("%s: %w", name, err)
	}
	check["validated_response"] = validated
	if len(nativeBase) > 0 {
		native, e := request(ctx, nativeBase[0]+"/status", nil, 5*time.Second)
		if e != nil {
			check["error"] = e.Error()
			return fmt.Errorf("%s: %w", name, e)
		}
		last := mapping(mapping(native["stats"])["last_generation"])
		u := mapping(validated["usage"])
		if protocol == "chat" {
			u = document{"input_tokens": u["prompt_tokens"], "output_tokens": u["completion_tokens"]}
		}
		if e = validateNativeAccounting(last, u); e != nil {
			check["error"] = e.Error()
			return fmt.Errorf("%s: %w", name, e)
		}
		check["native_accounting"] = document{"native_generation_metadata": true, "finish_reason": "stop", "prompt_tokens": last["prompt_tokens"], "generation_tokens": last["generation_tokens"]}
	}
	check["passed"] = true
	return nil
}

func runIntegrationProofs(ctx context.Context, base, size string, result document, out io.Writer) error {
	role := "haiku"
	if size == "large" {
		role = "sonnet"
	}
	for _, protocol := range []string{"messages", "responses", "chat"} {
		for _, stream := range []bool{false, true} {
			fmt.Fprintf(out, "Checking %s %s API contract (stream=%t).\n", role, protocol, stream)
			if err := runProtocolProof(ctx, base, size, role, protocol, stream, false, nil, result, runtimeURL); err != nil {
				return err
			}
		}
	}
	if size == "large" {
		if err := runOpusContracts(ctx, base, result, out, runtimeURL); err != nil {
			return err
		}
	}
	for _, protocol := range []string{"messages", "responses", "chat"} {
		var validated document
		for _, stream := range []bool{false, true} {
			fmt.Fprintf(out, "Checking %s %s required tool contract (stream=%t).\n", role, protocol, stream)
			if err := runProtocolProof(ctx, base, size, role, protocol, stream, true, nil, result, runtimeURL); err != nil {
				return err
			}
			checks := result["checks"].([]any)
			validated = mapping(mapping(checks[len(checks)-1])["validated_response"])
		}
		p, err := toolContinuationPayload(role, validated, protocol)
		if err != nil {
			return err
		}
		if err = runProtocolProof(ctx, base, size, role, protocol, false, false, p, result, runtimeURL); err != nil {
			return err
		}
	}
	if err := runCacheProof(ctx, base, size, role, result); err != nil {
		return err
	}
	return runTelemetryProof(ctx, base, size, role, result)
}

func runOpusContracts(ctx context.Context, base string, result document, out io.Writer, nativeBase ...string) error {
	for _, stream := range []bool{false, true} {
		fmt.Fprintf(out, "Checking opus Messages API contract (stream=%t).\n", stream)
		if err := runProtocolProof(ctx, base, "large", "opus", "messages", stream, false, nil, result, nativeBase...); err != nil {
			return err
		}
	}
	return nil
}

func toolContinuationPayload(role string, validated document, protocolNames ...string) (document, error) {
	calls, ok := validated["tool_calls"].([]document)
	if !ok || len(calls) != 1 {
		return nil, errors.New("tool continuation requires exactly one validated call")
	}
	call := calls[0]
	if text(call["id"]) == "" || call["name"] != "record_marker" || len(mapping(call["input"])) != 1 || mapping(call["input"])["marker"] != marker {
		return nil, errors.New("tool continuation has invalid arguments or identity")
	}
	protocol := "messages"
	if len(protocolNames) > 0 {
		protocol = protocolNames[0]
	}
	p := proofPayload(protocol, role, false, false)
	p["proof_continuation"] = true
	prompt := "Call record_marker once with marker " + marker + ". Do not answer in text."
	output := "Recorded successfully. Reply with exactly " + marker + " and no other final text."
	args, _ := json.Marshal(call["input"])
	switch protocol {
	case "messages":
		p["messages"] = []any{document{"role": "user", "content": prompt}, document{"role": "assistant", "content": []any{document{"type": "tool_use", "id": call["id"], "name": call["name"], "input": call["input"]}}}, document{"role": "user", "content": []any{document{"type": "tool_result", "tool_use_id": call["id"], "content": output}}}}
	case "responses":
		p["input"] = []any{document{"type": "message", "role": "user", "content": prompt}, document{"type": "function_call", "call_id": call["id"], "name": call["name"], "arguments": string(args)}, document{"type": "function_call_output", "call_id": call["id"], "output": output}}
	case "chat":
		p["messages"] = []any{document{"role": "user", "content": prompt}, document{"role": "assistant", "content": nil, "tool_calls": []any{document{"id": call["id"], "type": "function", "function": document{"name": call["name"], "arguments": string(args)}}}}, document{"role": "tool", "tool_call_id": call["id"], "content": output}}
	default:
		return nil, errors.New("unknown continuation protocol")
	}
	return p, nil
}

func runTelemetryProof(ctx context.Context, base, size, role string, result document) (proofErr error) {
	check := document{"name": size + "-" + role + "-telemetry", "passed": false, "endpoints": []string{"/sentinel/activity", "/sentinel/control"}}
	defer func() {
		if proofErr != nil {
			check["error"] = proofErr.Error()
		}
	}()
	result["checks"] = append(result["checks"].([]any), check)
	activity, err := request(ctx, base+"/sentinel/activity", nil, 5*time.Second)
	if err != nil {
		return err
	}
	last := mapping(activity["last_completed"])
	usage := mapping(last["usage"])
	if activity["scope"] != "local-adapter-attempts" || activity["queued"] != float64(0) || len(list(activity["active"])) != 0 || last["accepted"] != true || last["role"] != role || last["model"] != "sentinel-"+role || text(last["request_id"]) == "" || text(last["attempt_id"]) == "" || last["client"] != "anthropic_messages" || positiveUsage(usage, "input_tokens", "output_tokens", false) != nil {
		return errors.New("invalid idle activity identity or accounting")
	}
	control, err := request(ctx, base+"/sentinel/control", nil, 5*time.Second)
	if err != nil {
		return err
	}
	quality := mapping(control["quality_checks"])
	for _, key := range []string{"rejections", "recovery_attempts"} {
		n, ok := quality[key].(float64)
		if !ok || n < 0 || n != math.Trunc(n) {
			return errors.New("invalid quality counters")
		}
	}
	if control["mode"] != "serving" || control["policy"] != "local-only" || control["startup_billing_opt_in"] != false || mapping(control["role_budgets"])[role] == nil {
		return errors.New("invalid local control identity")
	}
	check["validated_response"] = document{"role": role, "client": last["client"], "accepted": true, "queued": 0, "active_count": 0, "usage": usage, "quality_checks": quality, "mode": control["mode"]}
	check["passed"] = true
	return nil
}

func validateCacheAccounting(samples []document) error {
	if len(samples) != 3 {
		return errors.New("cache proof requires three observations")
	}
	for _, d := range samples {
		p, ok := d["prompt_tokens"].(float64)
		c, ok2 := d["cached_prompt_tokens"].(float64)
		n, ok3 := d["processed_prompt_tokens"].(float64)
		if !ok || !ok2 || !ok3 || p <= 0 || c < 0 || n < 0 || math.Trunc(p) != p || math.Trunc(c) != c || math.Trunc(n) != n || c+n != p {
			return errors.New("invalid native cache token accounting")
		}
	}
	if samples[0]["cached_prompt_tokens"] != float64(0) || samples[1]["cached_prompt_tokens"].(float64) <= 0 || samples[2]["cached_prompt_tokens"] != float64(0) || samples[0]["prompt_tokens"] != samples[1]["prompt_tokens"] || samples[0]["prompt_tokens"] != samples[2]["prompt_tokens"] {
		return errors.New("cache reuse or session isolation failed")
	}
	return nil
}

func validateNativeAccounting(native, provider document) error {
	if native["native_generation_metadata"] != true || native["finish_reason"] != "stop" || native["prompt_tokens"] != provider["input_tokens"] || native["generation_tokens"] != provider["output_tokens"] {
		return errors.New("native EOS or token accounting disagrees with provider usage")
	}
	return positiveUsage(provider, "input_tokens", "output_tokens", false)
}

func runCacheProof(ctx context.Context, base, size, role string, result document) (proofErr error) {
	check := document{"name": size + "-" + role + "-session-cache", "endpoint": "/v1/messages", "native_endpoint": "/status", "passed": false, "assertions": []string{"same request token accounting stable", "same session reuses prompt tokens", "new session does not reuse prompt tokens", "cached plus processed equals prompt"}}
	defer func() {
		if proofErr != nil {
			check["error"] = proofErr.Error()
		}
	}()
	result["checks"] = append(result["checks"].([]any), check)
	samples := []document{}
	check["native_accounting"] = []any{}
	payload := proofPayload("messages", role, false, false)
	payload["messages"] = []any{document{"role": "user", "content": "Synthetic session isolation proof. Reply with exactly " + marker + " and no other final text."}}
	check["synthetic_request"] = payload
	for _, session := range []string{"sentinel-cache-a-" + size, "sentinel-cache-a-" + size, "sentinel-cache-b-" + size} {
		status, _, body, err := proofExchange(ctx, base+"/v1/messages", payload, session)
		check["http_status"] = status
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("cache proof HTTP status %d", status)
		}
		d := document{}
		if json.Unmarshal(body, &d) != nil {
			return errors.New("invalid cache response")
		}
		if _, err = validateProofJSON("messages", d, false); err != nil {
			return err
		}
		native, err := request(ctx, runtimeURL+"/status", nil, 5*time.Second)
		if err != nil {
			return err
		}
		last := mapping(mapping(native["stats"])["last_generation"])
		sample := document{}
		for _, key := range []string{"prompt_tokens", "processed_prompt_tokens", "cached_prompt_tokens", "generation_tokens", "native_generation_metadata", "finish_reason"} {
			sample[key] = last[key]
		}
		samples = append(samples, sample)
		check["native_accounting"] = append(check["native_accounting"].([]any), sample)
		if err := validateNativeAccounting(sample, mapping(d["usage"])); err != nil {
			check["error"] = err.Error()
			return err
		}
	}
	if err := validateCacheAccounting(samples); err != nil {
		check["error"] = err.Error()
		return err
	}
	check["passed"] = true
	return nil
}
