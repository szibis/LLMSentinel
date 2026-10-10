package clientcapture

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// modString accepts bounded scalar metadata only. Unknown fields never enter
// the record; in particular no transcript, tool arguments, or results are read.
func modString(event map[string]any, key string, required bool) (any, error) {
	value, exists := event[key]
	if !exists && !required {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" || len(text) > 256 || strings.ContainsFunc(text, unicode.IsControl) {
		return nil, errors.New("invalid mod metadata")
	}
	return text, nil
}

// normalizeMod deliberately builds the ordinary private record from a safe
// prompt skeleton, never passing the mod payload to transcript enrichment.
// A mod observation describes what the CLI reported, not a gateway request or
// a verified tool result, and remains unscored and ineligible for training.
func normalizeMod(billing string, event map[string]any) (map[string]any, error) {
	name := stringValue(event["mod_event"])
	if name != "prompt.submit" && name != "tool.call" && name != "turn.complete" {
		return nil, errors.New("unsupported mod event")
	}
	metadata := map[string]any{}
	for _, key := range []string{"session_id", "turn_id", "agent_id", "model", "tool_name", "tool_use_id"} {
		value, err := modString(event, key, key == "session_id" || (key == "tool_name" && name == "tool.call"))
		if err != nil {
			return nil, err
		}
		metadata[key] = value
	}
	texts := map[string]string{}
	for _, key := range []string{"prompt", "answer"} {
		value, exists := event[key]
		if !exists && !(key == "prompt" && name == "prompt.submit") {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid mod text")
		}
		texts[key] = text
	}
	stage, stageExists := event["stage"]
	if (stageExists || name == "tool.call") && stage != "returned" && stage != "error" {
		return nil, errors.New("invalid mod tool stage")
	}
	var duration any
	if value, exists := event["duration_ms"]; exists {
		number, ok := value.(json.Number)
		if !ok {
			return nil, errors.New("invalid mod duration")
		}
		parsed, err := number.Float64()
		if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, errors.New("invalid mod duration")
		}
		duration = parsed
	}
	if value, exists := event["is_aborted"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, errors.New("invalid mod aborted flag")
		}
	}
	counts := map[string]any{}
	if value, exists := event["usage"]; exists && value != nil {
		usage, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("invalid mod usage")
		}
		for _, key := range []string{"input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
			if value, exists := usage[key]; exists {
				count, ok := integer(value)
				if !ok {
					return nil, errors.New("invalid mod usage count")
				}
				counts[key] = count
			}
		}
	}
	record, err := normalize("claude", billing, map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": metadata["session_id"], "prompt": ""})
	if err != nil {
		return nil, err
	}
	record["source"], record["provenance"], record["event_type"] = "claude_code_mod", "local_cli_observation", name
	for _, key := range []string{"turn_id", "agent_id", "model"} {
		record[key] = metadata[key]
	}
	record["inputs"], record["outputs"] = []string{}, []string{}
	var captured string
	if name == "prompt.submit" {
		captured = texts["prompt"]
		record["inputs"] = []string{captured}
	} else if name == "turn.complete" {
		if answer, exists := texts["answer"]; exists {
			captured = answer
			record["outputs"] = []string{answer}
		}
		if duration != nil {
			record["latency_ms"], record["latency_source"] = duration, "claude_mod_turn_duration"
		}
		if len(counts) > 0 {
			usage := mapValue(record["usage"])
			for key, count := range counts {
				usage[key] = count
			}
			usage["source"], usage["scope"], usage["provenance"] = "claude_mod_turn_usage", "turn_reported", "local_cli_observation"
		}
	}
	record["truncated"] = utf8.RuneCountInString(captured) > maxText
	captureMetadata := map[string]any{}
	if name == "tool.call" {
		captureMetadata["tool_name"], captureMetadata["tool_use_id"] = metadata["tool_name"], metadata["tool_use_id"]
		captureMetadata["stage"], captureMetadata["stage_semantics"] = stage, "host_return_only"
	} else if name == "turn.complete" {
		if value, exists := event["is_aborted"]; exists {
			captureMetadata["is_aborted"] = value
		}
	}
	record["capture_metadata"] = captureMetadata
	return redact(record).(map[string]any), nil
}
