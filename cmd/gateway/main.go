package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/szibis/claude-escalate/internal/gateway"
	"github.com/szibis/claude-escalate/internal/models"
	"github.com/szibis/claude-escalate/internal/provider"
)

func main() {
	var (
		listenAddr   = flag.String("addr", ":8080", "Server listen address")
		providerType = flag.String("provider", "mock", "Provider type: mock, local, or real")
		apiKey       = flag.String("api-key", "", "API key for authentication (empty = no auth)")
		localURL     = flag.String("local-url", "", "Local LLM base URL (for local provider)")
		localModel   = flag.String("local-model", "", "Local LLM model name (for local provider)")
		anthropicKey = flag.String("anthropic-key", "", "Anthropic API key (for real provider)")
		help         = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		printUsage()
		os.Exit(0)
	}

	// Get credentials from environment if not provided
	if *anthropicKey == "" {
		*anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if *localURL == "" {
		*localURL = os.Getenv("LOCAL_LLM_URL")
	}
	if *localModel == "" {
		*localModel = os.Getenv("LOCAL_LLM_MODEL")
	}

	// Create model registry
	registry := models.DefaultModelRegistry()

	// Create unified client
	unifiedClient := models.NewUnifiedClient(registry)

	// Create provider based on type
	fmt.Printf("Starting LLMSentinel Gateway...\n")
	fmt.Printf("Provider: %s\n", *providerType)

	providerConfig := provider.ProviderConfig{
		APIKey:     *anthropicKey,
		LocalURL:   *localURL,
		LocalModel: *localModel,
	}

	switch *providerType {
	case "mock":
		providerConfig.Type = provider.ProviderTypeMock
		fmt.Printf("Using mock API (for testing)\n")

	case "local":
		providerConfig.Type = provider.ProviderTypeLocal
		if *localURL == "" {
			fmt.Fprintf(os.Stderr, "Error: --local-url required for local provider\n")
			os.Exit(1)
		}
		if *localModel == "" {
			fmt.Fprintf(os.Stderr, "Error: --local-model required for local provider\n")
			os.Exit(1)
		}
		fmt.Printf("Using local LLM at %s (model: %s)\n", *localURL, *localModel)

	case "real":
		providerConfig.Type = provider.ProviderTypeReal
		if *anthropicKey == "" {
			fmt.Fprintf(os.Stderr, "Error: --anthropic-key required for real provider\n")
			os.Exit(1)
		}
		fmt.Printf("Using real Anthropic API\n")

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown provider type: %s\n", *providerType)
		os.Exit(1)
	}

	// Create provider
	factory := provider.NewFactory(providerConfig)
	prov, err := factory.CreateClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating provider: %v\n", err)
		os.Exit(1)
	}

	// Register provider for all models
	for _, model := range registry.GetAllModels() {
		// For mock provider, use mock for all models
		if *providerType == "mock" {
			unifiedClient.RegisterProvider(model.ID, prov)
		} else if model.Provider == *providerType {
			// For other providers, only register matching models
			unifiedClient.RegisterProvider(model.ID, prov)
		}
	}

	// Set default model
	if models := registry.GetEnabledModels(); len(models) > 0 {
		unifiedClient.SetDefaultModel(models[0].ID)
		fmt.Printf("Default model: %s\n", models[0].ID)
	}

	// Create and start server
	server := gateway.NewServer(unifiedClient, registry, *listenAddr)
	if *apiKey != "" {
		server.SetAPIKey(*apiKey)
		fmt.Printf("Authentication enabled\n")
	}

	fmt.Printf("\nGateway ready!\n")
	fmt.Printf("HTTP endpoint: http://localhost:8080/v1/chat/completions\n")
	fmt.Printf("Models endpoint: http://localhost:8080/v1/models\n")
	fmt.Printf("Health check: http://localhost:8080/health\n")
	fmt.Printf("Metrics: http://localhost:8080/metrics\n")

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`LLMSentinel Gateway - Unified LLM API Gateway

Usage:
  gateway [options]

Options:
  -addr string
      Server listen address (default ":8080")

  -provider string
      Provider type: mock, local, or real (default "mock")

  -api-key string
      API key for authentication (empty = no auth required)

  -anthropic-key string
      Anthropic API key (for real provider, or set ANTHROPIC_API_KEY env)

  -local-url string
      Local LLM base URL (for local provider, or set LOCAL_LLM_URL env)

  -local-model string
      Local LLM model name (for local provider, or set LOCAL_LLM_MODEL env)

  -help
      Show this help message

Examples:
  # Start with mock API (testing)
  gateway -provider mock

  # Start with local LLM
  gateway -provider local -local-url http://localhost:8000 -local-model llama-2-7b

  # Start with real Anthropic API
  gateway -provider real -anthropic-key sk-xxx

  # With authentication
  gateway -provider mock -api-key myapikey123

  # Custom port
  gateway -addr :3000 -provider mock

Environment Variables:
  ANTHROPIC_API_KEY     Anthropic API key for real provider
  LOCAL_LLM_URL         Local LLM base URL for local provider
  LOCAL_LLM_MODEL       Local LLM model name for local provider

Testing the Gateway:
  # List models
  curl http://localhost:8080/v1/models

  # Chat completion
  curl http://localhost:8080/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{
      "model": "mock-model",
      "messages": [{"role": "user", "content": "Hello!"}]
    }'

  # Health check
  curl http://localhost:8080/health

  # Metrics
  curl http://localhost:8080/metrics`)
}
