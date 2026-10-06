package qwensmoke

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func messageFixture() document {
	return document{"type": "message", "role": "assistant", "id": "msg_test", "content": []any{document{"type": "text", "text": marker}}, "stop_reason": "end_turn", "usage": document{"input_tokens": float64(12), "output_tokens": float64(4)}}
}

func TestProofJSONRejectsBrokenContracts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(document)
	}{
		{"reasoning", func(d document) { mapping(list(d["content"])[0])["text"] = "<think>secret</think>" + marker }},
		{"hidden reasoning", func(d document) { d["reasoning_content"] = "private thoughts" }},
		{"usage missing", func(d document) { delete(d, "usage") }},
		{"usage zero", func(d document) { mapping(d["usage"])["output_tokens"] = float64(0) }},
		{"usage fractional", func(d document) { mapping(d["usage"])["input_tokens"] = float64(1.5) }},
		{"stop", func(d document) { d["stop_reason"] = "max_tokens" }},
		{"tool mismatch", func(d document) {
			d["stop_reason"] = "tool_use"
			d["content"] = []any{document{"type": "tool_use", "id": "call_test", "name": "record_marker", "input": document{"marker": "WRONG"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := messageFixture()
			tc.mutate(d)
			if _, err := validateProofJSON("messages", d, tc.name == "tool mismatch"); err == nil {
				t.Fatal("broken contract accepted")
			}
		})
	}
	if _, err := validateProofJSON("messages", messageFixture(), false); err != nil {
		t.Fatal(err)
	}
}

func messagesSSE() string {
	start, _ := json.Marshal(document{"type": "message_start", "message": document{"id": "msg_test", "type": "message", "role": "assistant"}})
	return "event: message_start\ndata: " + string(start) + "\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + marker + "\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":12,\"output_tokens\":4}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

func TestProofSSELifecycle(t *testing.T) {
	valid := messagesSSE()
	if _, err := validateProofSSE("messages", []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.ReplaceAll(valid, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", ""), strings.ReplaceAll(valid, "content_block_stop", "bogus"), strings.ReplaceAll(valid, marker, "<think>private</think>"+marker), valid + "data: invalid\n\n", strings.ReplaceAll(valid, "event: message_start", "event: mismatch"), strings.TrimSuffix(valid, "\n\n")} {
		if _, err := validateProofSSE("messages", []byte(body)); err == nil {
			t.Fatalf("malformed SSE accepted: %s", body)
		}
	}
}

func TestProofHTTPPreservesFailedEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: invalid\n\n"))
	}))
	defer server.Close()
	result := document{"checks": []any{}}
	err := runProtocolProof(context.Background(), server.URL, "small", "haiku", "messages", true, false, nil, result)
	if err == nil {
		t.Fatal("malformed stream accepted")
	}
	checks := result["checks"].([]any)
	if len(checks) != 1 || mapping(checks[0])["passed"] != false || mapping(checks[0])["http_status"] != 200 {
		t.Fatalf("failed proof lost: %v", result)
	}
}

func TestCacheAccountingRejectsCrossSessionReuse(t *testing.T) {
	valid := []document{{"prompt_tokens": float64(20), "processed_prompt_tokens": float64(20), "cached_prompt_tokens": float64(0)}, {"prompt_tokens": float64(20), "processed_prompt_tokens": float64(1), "cached_prompt_tokens": float64(19)}, {"prompt_tokens": float64(20), "processed_prompt_tokens": float64(20), "cached_prompt_tokens": float64(0)}}
	if err := validateCacheAccounting(valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		index int
		key   string
		value float64
	}{{"cross session", 2, "cached_prompt_tokens", 19}, {"no reuse", 1, "cached_prompt_tokens", 0}, {"invented accounting", 1, "processed_prompt_tokens", 20}, {"different tokenization", 1, "prompt_tokens", 21}, {"fractional", 0, "prompt_tokens", 20.5}} {
		t.Run(tc.name, func(t *testing.T) {
			samples := make([]document, 3)
			for i, d := range valid {
				samples[i] = document{}
				for k, v := range d {
					samples[i][k] = v
				}
			}
			samples[tc.index][tc.key] = tc.value
			if validateCacheAccounting(samples) == nil {
				t.Fatal("invalid cache accounting accepted")
			}
		})
	}
}

func responseStreamEvents(tool bool) []document {
	part := document{"type": "output_text", "text": marker, "annotations": []any{}, "logprobs": []any{}}
	item := document{"id": "msg_item", "type": "message", "role": "assistant", "status": "completed", "content": []any{part}}
	added := document{"id": "msg_item", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	pending := document{"id": "resp_test", "object": "response", "status": "in_progress", "output": []any{}, "usage": nil}
	final := document{"id": "resp_test", "object": "response", "status": "completed", "output": []any{item}, "usage": document{"input_tokens": 12, "output_tokens": 4, "total_tokens": 16}}
	events := []document{{"type": "response.created", "response": pending}, {"type": "response.in_progress", "response": pending}, {"type": "response.output_item.added", "output_index": 0, "item": added}}
	if tool {
		args := `{"marker":"QWEN_ROLE_READY"}`
		item = document{"id": "fc_item", "type": "function_call", "call_id": "toolu_test", "name": "record_marker", "arguments": args, "status": "completed"}
		added = document{"id": "fc_item", "type": "function_call", "call_id": "toolu_test", "name": "record_marker", "arguments": "", "status": "in_progress"}
		events[2]["item"] = added
		events = append(events, document{"type": "response.function_call_arguments.delta", "item_id": "fc_item", "output_index": 0, "delta": args}, document{"type": "response.function_call_arguments.done", "item_id": "fc_item", "output_index": 0, "arguments": args, "name": "record_marker"})
		final["output"] = []any{item}
	} else {
		events = append(events,
			document{"type": "response.content_part.added", "item_id": "msg_item", "output_index": 0, "content_index": 0, "part": document{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}},
			document{"type": "response.output_text.delta", "item_id": "msg_item", "output_index": 0, "content_index": 0, "delta": marker, "logprobs": []any{}},
			document{"type": "response.output_text.done", "item_id": "msg_item", "output_index": 0, "content_index": 0, "text": marker, "logprobs": []any{}},
			document{"type": "response.content_part.done", "item_id": "msg_item", "output_index": 0, "content_index": 0, "part": part})
	}
	return append(events, document{"type": "response.output_item.done", "output_index": 0, "item": item}, document{"type": "response.completed", "response": final})
}

func encodeResponseEvents(events []document) string {
	var out strings.Builder
	for i, event := range events {
		event["sequence_number"] = i
		b, _ := json.Marshal(event)
		out.WriteString("event: " + text(event["type"]) + "\ndata: " + string(b) + "\n\n")
	}
	return out.String()
}

func TestResponsesSSERequiresFullLifecycle(t *testing.T) {
	events := responseStreamEvents(false)
	if _, err := validateProofSSE("responses", []byte(encodeResponseEvents(events))); err != nil {
		t.Fatal(err)
	}
	for i, event := range events {
		bad := append([]document{}, events[:i]...)
		bad = append(bad, events[i+1:]...)
		if _, err := validateProofSSE("responses", []byte(encodeResponseEvents(bad))); err == nil {
			t.Fatalf("missing %s accepted", event["type"])
		}
	}
}

func TestResponsesSSERejectsUnassemblablePayloads(t *testing.T) {
	for _, tool := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			event  int
			mutate func(document)
		}{
			{"added item missing", 2, func(d document) { delete(d, "item") }},
			{"added index missing", 2, func(d document) { delete(d, "output_index") }},
			{"added id missing", 2, func(d document) { delete(mapping(d["item"]), "id") }},
			{"changed pending id", 1, func(d document) {
				d["response"] = document{"id": "wrong", "status": "in_progress", "output": []any{}, "usage": nil}
			}},
			{"changed delta item id", 3, func(d document) { d["item_id"] = "wrong" }},
			{"changed delta output index", 3, func(d document) { d["output_index"] = 1 }},
			{"done payload missing", -2, func(d document) { delete(d, "item") }},
			{"done index missing", -2, func(d document) { delete(d, "output_index") }},
			{"done item changed", -2, func(d document) {
				b := mapping(d["item"])
				copy := document{}
				for k, v := range b {
					copy[k] = v
				}
				copy["id"] = "wrong"
				d["item"] = copy
			}},
			{"completed output mismatch", -1, func(d document) {
				b := mapping(d["response"])
				copy := document{}
				for k, v := range b {
					copy[k] = v
				}
				copy["output"] = []any{}
				d["response"] = copy
			}},
		} {
			t.Run(fmt.Sprintf("tool=%t/%s", tool, tc.name), func(t *testing.T) {
				events := responseStreamEvents(tool)
				i := tc.event
				if i < 0 {
					i = len(events) + i
				}
				tc.mutate(events[i])
				if _, err := validateProofSSE("responses", []byte(encodeResponseEvents(events)), tool); err == nil {
					t.Fatal("unassemblable response accepted")
				}
			})
		}
	}
	for _, tc := range []struct {
		name   string
		event  int
		mutate func(document)
	}{
		{"part missing", 3, func(d document) { delete(d, "part") }},
		{"content index missing", 3, func(d document) { delete(d, "content_index") }},
		{"part wrong type", 3, func(d document) { mapping(d["part"])["type"] = "reasoning" }},
		{"done text missing", 5, func(d document) { delete(d, "text") }},
		{"done text mismatch", 5, func(d document) { d["text"] = "wrong" }},
		{"done content index mismatch", 5, func(d document) { d["content_index"] = 1 }},
		{"done part missing", 6, func(d document) { delete(d, "part") }},
		{"done part mismatch", 6, func(d document) {
			d["part"] = document{"type": "output_text", "text": "wrong", "annotations": []any{}, "logprobs": []any{}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := responseStreamEvents(false)
			tc.mutate(events[tc.event])
			if _, err := validateProofSSE("responses", []byte(encodeResponseEvents(events))); err == nil {
				t.Fatal("invalid content parity accepted")
			}
		})
	}
}

func TestResponsesSSECorrelatesFragmentedDeltas(t *testing.T) {
	for _, tool := range []bool{false, true} {
		events := responseStreamEvents(tool)
		index := 4
		if tool {
			index = 3
		}
		first := document{}
		second := document{}
		for k, v := range events[index] {
			first[k] = v
			second[k] = v
		}
		value := text(first["delta"])
		first["delta"] = value[:5]
		second["delta"] = value[5:]
		fragmented := append([]document{}, events[:index]...)
		fragmented = append(fragmented, first, second)
		fragmented = append(fragmented, events[index+1:]...)
		body := encodeResponseEvents(fragmented)
		if _, err := validateProofSSE("responses", []byte(body), tool); err != nil {
			t.Fatalf("valid fragmented stream (tool=%t): %v", tool, err)
		}
		for _, bad := range []string{strings.Replace(body, `"sequence_number":3`, `"sequence_number":4`, 1), strings.Replace(body, `"sequence_number":3`, `"missing_sequence":3`, 1)} {
			if _, err := validateProofSSE("responses", []byte(bad), tool); err == nil {
				t.Fatal("broken response sequence accepted")
			}
		}
	}
}

func TestChatSSERequiresUsageAndStableIdentity(t *testing.T) {
	valid := "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"" + marker + "\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"total_tokens\":16}}\n\ndata: [DONE]\n\n"
	if _, err := validateProofSSE("chat", []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(valid, "\"total_tokens\":16", "\"total_tokens\":15", 1), strings.Replace(valid, "chatcmpl-test", "changed", 1), strings.TrimSuffix(valid, "data: [DONE]\n\n"), strings.Replace(valid, "\"role\":\"assistant\"", "\"role\":\"user\"", 1)} {
		if _, err := validateProofSSE("chat", []byte(bad)); err == nil {
			t.Fatal("malformed chat stream accepted")
		}
	}
}

func TestTelemetryRejectsInvalidIdentityAndCounters(t *testing.T) {
	for _, fault := range []string{"", "role", "count", "usage"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/sentinel/activity" {
					d := document{"scope": "local-adapter-attempts", "queued": 0, "active": []any{}, "last_completed": document{"accepted": true, "role": "haiku", "model": "sentinel-haiku", "client": "anthropic_messages", "request_id": "req_test", "attempt_id": "attempt_test", "usage": document{"input_tokens": 12, "output_tokens": 4}}}
					if fault == "role" {
						mapping(d["last_completed"])["role"] = "sonnet"
					}
					if fault == "usage" {
						mapping(d["last_completed"])["usage"] = nil
					}
					json.NewEncoder(w).Encode(d)
				} else {
					d := document{"mode": "serving", "policy": "local-only", "startup_billing_opt_in": false, "role_budgets": document{"haiku": 1024}, "quality_checks": document{"rejections": 0, "recovery_attempts": 0}}
					if fault == "count" {
						mapping(d["quality_checks"])["rejections"] = -1
					}
					json.NewEncoder(w).Encode(d)
				}
			}))
			defer server.Close()
			result := document{"checks": []any{}}
			err := runTelemetryProof(context.Background(), server.URL, "small", "haiku", result)
			if (err == nil) != (fault == "") {
				t.Fatalf("fault %q error %v", fault, err)
			}
		})
	}
}

func TestIntegrationFlagRequiresGateway(t *testing.T) {
	var out strings.Builder
	if code := Run([]string{"--integration-proofs"}, nil, &out, &out); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestNativeAccountingMustMatchProviderUsage(t *testing.T) {
	u := document{"input_tokens": float64(20), "output_tokens": float64(4)}
	for _, fault := range []string{"", "native", "prompt", "generation", "stop"} {
		t.Run(fault, func(t *testing.T) {
			d := document{"native_generation_metadata": true, "prompt_tokens": float64(20), "generation_tokens": float64(4), "finish_reason": "stop"}
			switch fault {
			case "native":
				d["native_generation_metadata"] = false
			case "prompt":
				d["prompt_tokens"] = float64(19)
			case "generation":
				d["generation_tokens"] = float64(3)
			case "stop":
				d["finish_reason"] = "length"
			}
			err := validateNativeAccounting(d, u)
			if (err == nil) != (fault == "") {
				t.Fatalf("fault=%s err=%v", fault, err)
			}
		})
	}
}

func TestRequiredToolRoundTripHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var p document
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Error("invalid request")
		}
		requests++
		if requests == 1 {
			if mapping(p["tool_choice"])["name"] != "record_marker" {
				t.Error("required tool choice lost")
			}
			d := messageFixture()
			d["stop_reason"] = "tool_use"
			d["content"] = []any{document{"type": "tool_use", "id": "toolu_test", "name": "record_marker", "input": document{"marker": marker}}}
			json.NewEncoder(w).Encode(d)
			return
		}
		messages := list(p["messages"])
		if len(messages) != 3 {
			t.Errorf("continuation messages=%v", messages)
		} else {
			block := mapping(list(mapping(messages[2])["content"])[0])
			if block["type"] != "tool_result" || block["tool_use_id"] != "toolu_test" {
				t.Errorf("tool result identity lost: %v", block)
			}
		}
		json.NewEncoder(w).Encode(messageFixture())
	}))
	defer server.Close()
	result := document{"checks": []any{}}
	if err := runProtocolProof(context.Background(), server.URL, "small", "haiku", "messages", false, true, nil, result); err != nil {
		t.Fatal(err)
	}
	checks := result["checks"].([]any)
	p, err := toolContinuationPayload("haiku", mapping(mapping(checks[0])["validated_response"]))
	if err != nil {
		t.Fatal(err)
	}
	if err = runProtocolProof(context.Background(), server.URL, "small", "haiku", "messages", false, false, p, result); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
	if _, err = toolContinuationPayload("haiku", document{}); err == nil {
		t.Fatal("missing validated call accepted")
	}
}

func TestOpusContractsUseEffortRoleAndSSELifecycle(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p document
		json.NewDecoder(r.Body).Decode(&p)
		requests++
		if p["model"] != "sentinel-opus" || p["max_tokens"] != float64(8192) {
			t.Errorf("wrong Opus effort request: %v", p)
		}
		if p["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte(messagesSSE()))
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(messageFixture())
		}
	}))
	defer server.Close()
	result := document{"checks": []any{}}
	var out strings.Builder
	if err := runOpusContracts(context.Background(), server.URL, result, &out); err != nil {
		t.Fatal(err)
	}
	checks := result["checks"].([]any)
	if requests != 2 || len(checks) != 2 || mapping(checks[0])["role"] != "opus" || mapping(checks[1])["passed"] != true {
		t.Fatalf("Opus proof lost: %v", result)
	}
}

func toolStreamFixture(protocol string) string {
	arguments := `{"marker":"QWEN_ROLE_READY"}`
	call := document{"id": "toolu_test", "type": "tool_use", "name": "record_marker", "input": document{"marker": "QWEN_ROLE_READY"}}
	var events []document
	switch protocol {
	case "messages":
		events = []document{{"type": "message_start", "message": document{"id": "msg_test"}}, {"type": "content_block_start", "index": 0, "content_block": document{"type": "tool_use", "id": "toolu_test", "name": "record_marker", "input": document{}}}, {"type": "content_block_delta", "index": 0, "delta": document{"type": "input_json_delta", "partial_json": arguments}}, {"type": "content_block_stop", "index": 0}, {"type": "message_delta", "delta": document{"stop_reason": "tool_use"}, "usage": document{"input_tokens": 12, "output_tokens": 4}}, {"type": "message_stop"}}
	case "responses":
		return encodeResponseEvents(responseStreamEvents(true))
	case "chat":
		tool := document{"index": 0, "id": call["id"], "type": "function", "function": document{"name": "record_marker", "arguments": arguments}}
		events = []document{
			{"id": "chatcmpl-test", "object": "chat.completion.chunk", "choices": []any{document{"delta": document{"role": "assistant"}, "finish_reason": nil}}},
			{"id": "chatcmpl-test", "object": "chat.completion.chunk", "choices": []any{document{"delta": document{"tool_calls": []any{tool}}, "finish_reason": nil}}},
			{"id": "chatcmpl-test", "object": "chat.completion.chunk", "choices": []any{document{"delta": document{}, "finish_reason": "tool_calls"}}},
			{"id": "chatcmpl-test", "object": "chat.completion.chunk", "choices": []any{}, "usage": document{"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}},
		}
	}
	var out strings.Builder
	for _, event := range events {
		b, _ := json.Marshal(event)
		if protocol != "chat" {
			out.WriteString("event: " + text(event["type"]) + "\n")
		}
		out.WriteString("data: " + string(b) + "\n\n")
	}
	if protocol == "chat" {
		out.WriteString("data: [DONE]\n\n")
	}
	return out.String()
}

func TestToolStreamsRequireExactArgumentsAndLifecycle(t *testing.T) {
	for _, protocol := range []string{"messages", "responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			valid := toolStreamFixture(protocol)
			if _, err := validateProofSSE(protocol, []byte(valid), true); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{strings.ReplaceAll(valid, "QWEN_ROLE_READY", "WRONG"), strings.ReplaceAll(valid, "record_marker", "invented_tool"), strings.ReplaceAll(valid, "toolu_test", ""), strings.ReplaceAll(valid, `\"marker\":`, `\"extra\":`), strings.ReplaceAll(valid, `{\"marker\":`, `not-json{`)} {
				if _, err := validateProofSSE(protocol, []byte(bad), true); err == nil {
					t.Fatal("invalid tool stream accepted")
				}
			}
			if _, err := validateProofSSE(protocol, []byte(valid), false); err == nil {
				t.Fatal("tool unexpectedly accepted for final marker")
			}
		})
	}
}

func TestFunctionToolHTTPRoundTrips(t *testing.T) {
	for _, protocol := range []string{"responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var p document
				json.NewDecoder(r.Body).Decode(&p)
				requests++
				if requests == 1 {
					tools := list(p["tools"])
					if len(tools) != 1 || p["tool_choice"] == nil {
						t.Error("required function schema or choice missing")
					}
					if protocol == "responses" {
						json.NewEncoder(w).Encode(document{"id": "resp_test", "status": "completed", "output": []any{document{"type": "function_call", "call_id": "toolu_test", "name": "record_marker", "arguments": `{"marker":"QWEN_ROLE_READY"}`, "status": "completed"}}, "usage": document{"input_tokens": 12, "output_tokens": 4, "total_tokens": 16}})
					} else {
						json.NewEncoder(w).Encode(document{"id": "chatcmpl-test", "object": "chat.completion", "choices": []any{document{"message": document{"role": "assistant", "content": nil, "tool_calls": []any{document{"id": "toolu_test", "type": "function", "function": document{"name": "record_marker", "arguments": `{"marker":"QWEN_ROLE_READY"}`}}}}, "finish_reason": "tool_calls"}}, "usage": document{"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}})
					}
					return
				}
				if protocol == "responses" {
					history := list(p["input"])
					if len(history) != 3 || mapping(history[2])["type"] != "function_call_output" || mapping(history[2])["call_id"] != "toolu_test" {
						t.Errorf("bad Responses continuation: %v", p)
					}
					json.NewEncoder(w).Encode(document{"id": "resp_test", "status": "completed", "output": []any{document{"type": "message", "role": "assistant", "status": "completed", "content": []any{document{"type": "output_text", "text": marker}}}}, "usage": document{"input_tokens": 12, "output_tokens": 4, "total_tokens": 16}})
				} else {
					history := list(p["messages"])
					if len(history) != 3 || mapping(history[2])["role"] != "tool" || mapping(history[2])["tool_call_id"] != "toolu_test" {
						t.Errorf("bad Chat continuation: %v", p)
					}
					json.NewEncoder(w).Encode(document{"id": "chatcmpl-test", "object": "chat.completion", "choices": []any{document{"message": document{"role": "assistant", "content": marker}, "finish_reason": "stop"}}, "usage": document{"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}})
				}
			}))
			defer server.Close()
			result := document{"checks": []any{}}
			if err := runProtocolProof(context.Background(), server.URL, "small", "haiku", protocol, false, true, nil, result); err != nil {
				t.Fatal(err)
			}
			checks := result["checks"].([]any)
			p, err := toolContinuationPayload("haiku", mapping(mapping(checks[0])["validated_response"]), protocol)
			if err != nil {
				t.Fatal(err)
			}
			if err = runProtocolProof(context.Background(), server.URL, "small", "haiku", protocol, false, false, p, result); err != nil {
				t.Fatal(err)
			}
		})
	}
}
