package localgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type chatProtocolRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"messages"`
	Tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	Functions           json.RawMessage `json:"functions"`
	ToolChoice          json.RawMessage `json:"tool_choice"`
	MaxTokens           *int            `json:"max_tokens"`
	MaxCompletionTokens *int            `json:"max_completion_tokens"`
	Stream              bool            `json:"stream"`
	ResponseFormat      json.RawMessage `json:"response_format"`
	StreamOptions       struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

func prepareChatProtocol(req chatProtocolRequest, roles bool, budget int) (claudeRequest, []map[string]string, error) {
	input := []any{}
	system := []string{}
	for _, m := range req.Messages {
		text := ""
		if len(m.Content) > 0 && string(m.Content) != "null" {
			if json.Unmarshal(m.Content, &text) != nil {
				return claudeRequest{}, nil, errors.New("chat completions supports text content only")
			}
		}
		switch m.Role {
		case "system", "developer":
			system = append(system, text)
		case "user", "assistant":
			if text != "" {
				input = append(input, map[string]any{"type": "message", "role": m.Role, "content": text})
			}
			for _, call := range m.ToolCalls {
				if m.Role != "assistant" || call.Type != "function" {
					return claudeRequest{}, nil, errors.New("only assistant function tool_calls are supported")
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
			}
		case "tool":
			input = append(input, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": text})
		default:
			return claudeRequest{}, nil, errors.New("unsupported Chat Completions message role")
		}
	}
	tools := []any{}
	for _, tool := range req.Tools {
		if tool.Type != "function" {
			return claudeRequest{}, nil, errors.New("only function tools are supported")
		}
		tools = append(tools, map[string]any{"type": "function", "name": tool.Function.Name, "description": tool.Function.Description, "parameters": tool.Function.Parameters})
	}
	if len(req.Functions) > 0 && string(req.Functions) != "null" {
		return claudeRequest{}, nil, errors.New("legacy functions are unsupported; use tools")
	}
	if len(req.ResponseFormat) > 0 && string(req.ResponseFormat) != "null" {
		var format struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(req.ResponseFormat, &format) != nil || format.Type != "text" {
			return claudeRequest{}, nil, errors.New("only text response_format is supported")
		}
	}
	choice := req.ToolChoice
	var named struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if len(choice) > 0 && json.Unmarshal(choice, &named) == nil && named.Type == "function" {
		choice = mustJSON(map[string]string{"type": "function", "name": named.Function.Name})
	}
	payload := map[string]any{"model": req.Model, "input": input, "instructions": strings.Join(system, "\n"), "tools": tools, "store": false}
	limit := req.MaxTokens
	if req.MaxCompletionTokens != nil {
		limit = req.MaxCompletionTokens
	}
	if limit != nil {
		payload["max_output_tokens"] = *limit
	}
	if len(choice) > 0 {
		payload["tool_choice"] = json.RawMessage(choice)
	}
	var converted responsesRequest
	if err := json.Unmarshal(mustJSON(payload), &converted); err != nil {
		return claudeRequest{}, nil, err
	}
	return prepareResponses(converted, roles, budget)
}

func (g *Gateway) chatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		responsesError(w, 405, "Use POST")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, g.cfg.MaxRequestBytes+1))
	if err != nil || int64(len(body)) > g.cfg.MaxRequestBytes {
		responsesError(w, 413, "request exceeds local limit")
		return
	}
	var req chatProtocolRequest
	if json.Unmarshal(body, &req) != nil {
		responsesError(w, 400, "invalid Chat Completions request")
		return
	}
	if !g.cfg.ClaudeAdapter {
		needsTools := len(req.Tools) > 0 || requestedCall(req.Functions) || requestedCall(req.ToolChoice)
		for _, message := range req.Messages {
			needsTools = needsTools || message.Role == "tool" || message.Role == "function" || len(message.ToolCalls) > 0
		}
		if needsTools {
			responsesError(w, 501, "tool adapter is disabled; no inference dispatched")
			return
		}
	}
	converted, messages, err := prepareChatProtocol(req, len(g.roles) > 0, g.cfg.ClaudeMaxTokens)
	if err != nil {
		responsesError(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(withTrainingClient(r.Context(), "openai_chat_completions"), g.cfg.Timeout)
	defer cancel()
	result, err := g.inferClaude(ctx, converted, messages)
	if err != nil {
		status, _ := outputErrorKind(err)
		responsesError(w, status, err.Error())
		return
	}
	message := map[string]any{"role": "assistant", "content": nil}
	calls := []any{}
	texts := []string{}
	for _, block := range result.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		} else if block.Type == "tool_use" {
			calls = append(calls, map[string]any{"id": block.ID, "type": "function", "function": map[string]any{"name": block.Name, "arguments": string(mustJSON(block.Input))}})
		}
	}
	if len(texts) > 0 {
		message["content"] = strings.Join(texts, "\n")
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	finish := "stop"
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	var usage any
	if result.UsageKnown {
		usage = map[string]int{"prompt_tokens": result.Usage["input_tokens"], "completion_tokens": result.Usage["output_tokens"], "total_tokens": result.Usage["input_tokens"] + result.Usage["output_tokens"]}
	}
	id := newID("chatcmpl-")
	created := time.Now().Unix()
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "object": "chat.completion", "created": created, "model": req.Model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": usage})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	chunk := func(delta any, reason any) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}}))
	}
	chunk(map[string]string{"role": "assistant"}, nil)
	if message["content"] != nil {
		chunk(map[string]any{"content": message["content"]}, nil)
	}
	for index, call := range calls {
		value := call.(map[string]any)
		value["index"] = index
		chunk(map[string]any{"tool_calls": []any{value}}, nil)
	}
	chunk(map[string]any{}, finish)
	if req.StreamOptions.IncludeUsage {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model, "choices": []any{}, "usage": usage}))
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
