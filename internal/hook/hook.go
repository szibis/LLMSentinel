// Package hook implements the Claude Code hook JSON protocol.
// Hooks receive JSON on stdin and output JSON on stdout.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Input is the JSON structure received from Claude Code on stdin.
type Input struct {
	Prompt string `json:"prompt"`
}

// Output is the JSON structure returned to Claude Code on stdout.
type Output struct {
	Continue        bool                   `json:"continue"`
	SuppressOutput  bool                   `json:"suppressOutput"`
	HookOutput      map[string]interface{} `json:"hookSpecificOutput,omitempty"`
}

// ReadInput reads and parses the hook input from stdin.
func ReadInput() (*Input, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}
	return &input, nil
}

// PassThrough returns a no-op output that lets the prompt continue.
func PassThrough() *Output {
	return &Output{
		Continue:       true,
		SuppressOutput: true,
	}
}

// WithHint returns an output with an advisory message shown to the user.
func WithHint(message string) *Output {
	return &Output{
		Continue:       true,
		SuppressOutput: true,
		HookOutput: map[string]interface{}{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": message,
		},
	}
}

// WriteOutput writes the hook output as JSON to stdout.
func WriteOutput(out *Output) error {
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
