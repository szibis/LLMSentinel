package gateway

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CLIAdapter implements ToolAdapter for shell commands
type CLIAdapter struct {
	maxExecutionTime time.Duration
	signature        *ToolSignature
}

// NewCLIAdapter creates a new CLI adapter
func NewCLIAdapter() *CLIAdapter {
	adapter := &CLIAdapter{
		maxExecutionTime: 30 * time.Second,
	}
	adapter.initializeSignature()
	return adapter
}

// Name returns the adapter name
func (a *CLIAdapter) Name() string {
	return "cli"
}

// Type returns the tool type
func (a *CLIAdapter) Type() ToolType {
	return ToolTypeCLI
}

// Execute executes a shell command
func (a *CLIAdapter) Execute(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
	command, ok := req.Params["command"].(string)
	if !ok {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   "command parameter required",
		}, nil
	}

	// Check for allowed commands (security whitelist)
	if !a.isCommandAllowed(command) {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   "command not in whitelist",
		}, nil
	}

	start := time.Now()

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, a.maxExecutionTime)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(execCtx, "sh", "-c", command)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	err := cmd.Run()

	output := stdout.String()
	if output == "" && stderr.String() != "" {
		output = stderr.String()
	}

	if err != nil {
		return &ToolResponse{
			ID:      req.ID,
			Success: false,
			Error:   err.Error(),
			Data: map[string]string{
				"stdout": stdout.String(),
				"stderr": stderr.String(),
			},
			Timing: ResponseTiming{
				TotalMs:    time.Since(start).Milliseconds(),
				ToolExecMs: time.Since(start).Milliseconds(),
			},
		}, nil
	}

	return &ToolResponse{
		ID:      req.ID,
		Success: true,
		Data: map[string]string{
			"output": output,
		},
		Timing: ResponseTiming{
			TotalMs:    time.Since(start).Milliseconds(),
			ToolExecMs: time.Since(start).Milliseconds(),
		},
	}, nil
}

// GetSignature returns the tool signature
func (a *CLIAdapter) GetSignature() *ToolSignature {
	return a.signature
}

// Health checks if the CLI adapter is functional
func (a *CLIAdapter) Health(ctx context.Context) error {
	// Try to execute a simple command
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo ok")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CLI adapter health check failed: %w", err)
	}
	return nil
}

// Close closes the CLI adapter
func (a *CLIAdapter) Close() error {
	return nil
}

// isCommandAllowed checks if a command is in the whitelist
func (a *CLIAdapter) isCommandAllowed(command string) bool {
	// Whitelist of safe commands
	allowedPrefixes := []string{
		"git",
		"ls",
		"cat",
		"echo",
		"grep",
		"find",
		"pwd",
		"date",
		"head",
		"tail",
		"wc",
		"sort",
		"uniq",
		"cut",
		"tr",
		"sed",
	}

	command = strings.TrimSpace(command)
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(command, prefix+" ") || command == prefix {
			return true
		}
	}

	return false
}

// initializeSignature sets up the tool signature
func (a *CLIAdapter) initializeSignature() {
	a.signature = &ToolSignature{
		Name:        "cli",
		Type:        ToolTypeCLI,
		Description: "Execute shell commands (whitelisted only)",
		Parameters: map[string]*ParamSchema{
			"command": {
				Type:        "string",
				Description: "Shell command to execute",
				Required:    true,
			},
		},
		Returns: &ParamSchema{
			Type:        "object",
			Description: "Command output and status",
		},
	}
}
