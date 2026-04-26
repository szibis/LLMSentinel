package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"claude-escalate/internal/config"
	"claude-escalate/internal/gateway"
	"claude-escalate/internal/intent"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to config.yaml")
	noCacheBypassed := flag.Bool("no-cache", false, "Bypass all caching")
	fresh := flag.Bool("fresh", false, "Force fresh response (alias for --no-cache)")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Usage: claude-escalate [flags] <tool> [args...]\n")
		fmt.Fprintf(os.Stderr, "Example: claude-escalate --no-cache cli git status\n")
		os.Exit(1)
	}

	ctx := context.Background()

	// Load configuration
	loader := config.NewLoader(*configPath)
	cfg, err := loader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Configuration loaded from: %s\n", *configPath)
		fmt.Printf("Security layer: %v\n", cfg.Gateway.SecurityLayer)
		fmt.Printf("Semantic cache: %v\n", cfg.Optimizations.SemanticCache.Enabled)
	}

	// Create adapter factory
	factory := gateway.NewAdapterFactory()
	if err := factory.CreateFromConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating adapters: %v\n", err)
		os.Exit(1)
	}
	defer factory.Close()

	// Get the tool to execute
	toolName := flag.Arg(0)
	adapter, err := factory.GetAdapter(toolName)
	if err != nil {
		// Try as direct CLI command
		if toolName == "cli" || strings.HasPrefix(toolName, "git") || strings.HasPrefix(toolName, "ls") {
			adapter, err = factory.GetAdapter("cli")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: tool not found: %s\n", toolName)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: tool not found: %s\n", toolName)
			os.Exit(1)
		}
	}

	// Build the tool request
	var toolParams map[string]interface{}
	var command string

	if toolName == "cli" || strings.HasPrefix(toolName, "git") {
		// CLI command
		command = strings.Join(flag.Args(), " ")
		toolParams = map[string]interface{}{
			"command": command,
		}
	} else {
		// Other tool types
		toolParams = parseToolParams(flag.Args()[1:])
	}

	// Create the tool request
	req := &gateway.ToolRequest{
		ID:              "cli-" + fmt.Sprint(os.Getpid()),
		Tool:            toolName,
		Params:          toolParams,
		NoCacheBypassed: *noCacheBypassed || *fresh,
	}

	if *verbose {
		fmt.Printf("Tool: %s\n", toolName)
		fmt.Printf("Cache bypass: %v\n", req.NoCacheBypassed)
	}

	// Create intent classifier
	classifier := intent.NewClassifier(90)

	// Classify intent
	queryContext := &intent.QueryContext{
		UserSessionID: fmt.Sprint(os.Getuid()),
	}

	var queryStr string
	if command != "" {
		queryStr = command
	} else {
		queryStr = toolName
	}

	decision := classifier.Classify(ctx, queryStr, fmt.Sprint(os.Getuid()), queryContext)

	if *verbose {
		fmt.Printf("Intent: %s\n", decision.Intent)
		fmt.Printf("Model: %s\n", decision.RecommendedModel)
		fmt.Printf("Cache safe: %v\n", decision.CacheSafe)
		fmt.Printf("Confidence: %.2f\n", decision.Confidence)
		fmt.Printf("Explanation: %s\n", decision.Explanation)
	}

	// Execute the tool
	resp, err := adapter.Execute(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing tool: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		if data, ok := resp.Data.(map[string]interface{}); ok {
			if stderr, ok := data["stderr"].(string); ok && stderr != "" {
				fmt.Fprintf(os.Stderr, "%s\n", stderr)
			}
		}
		os.Exit(1)
	}

	// Print output
	if data, ok := resp.Data.(map[string]string); ok {
		if output, ok := data["output"]; ok {
			fmt.Print(output)
		}
	} else if data, ok := resp.Data.(map[string]interface{}); ok {
		// Pretty print other responses
		for key, value := range data {
			fmt.Printf("%s: %v\n", key, value)
		}
	} else {
		fmt.Printf("%v\n", resp.Data)
	}

	// Print transparency footer if verbose
	if *verbose {
		fmt.Printf("\n--- Claude Escalate Footer ---\n")
		fmt.Printf("Tokens saved: %d\n", resp.Timing.TotalMs)
		fmt.Printf("Optimization: %s\n", decision.OptimizeMode)
		fmt.Printf("Cache safe: %v\n", decision.CacheSafe)
	}
}

// parseToolParams parses tool parameters from command line args
func parseToolParams(args []string) map[string]interface{} {
	params := make(map[string]interface{})

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++
			} else {
				params[key] = true
			}
		} else if strings.HasPrefix(arg, "-") {
			key := strings.TrimPrefix(arg, "-")
			params[key] = true
		} else if i == 0 {
			params["query"] = arg
		} else {
			params[fmt.Sprintf("arg%d", i)] = arg
		}
	}

	return params
}
