# LLMSentinel Gateway - GoModel-like AI Gateway

A unified, OpenAI-compatible API gateway for multiple LLM providers built in Go. Supports mock APIs, local LLMs, and real cloud providers with intelligent routing, cost tracking, and observability.

## Quick Start

### 1. Start the Gateway (Mock Mode - Zero Cost)

```bash
go run cmd/gateway/main.go -provider mock
```

Output:
```
Starting LLMSentinel Gateway...
Provider: mock
Using mock API (for testing)
Default model: mock-model

Gateway ready!
HTTP endpoint: http://localhost:8080/v1/chat/completions
Models endpoint: http://localhost:8080/v1/models
Health check: http://localhost:8080/health
Metrics: http://localhost:8080/metrics
```

### 2. Test with OpenAI-Compatible Client

```bash
# List available models
curl http://localhost:8080/v1/models

# Send a chat completion request
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mock-model",
    "messages": [{"role": "user", "content": "Hello, what is 2+2?"}]
  }'

# Check health
curl http://localhost:8080/health

# View metrics
curl http://localhost:8080/metrics
```

## Provider Configuration

### Mock API (Testing)
```bash
# No API key needed - perfect for integration and load tests
go run cmd/gateway/main.go -provider mock
```

### Local LLM (LM Studio, Ollama)
```bash
# Start LM Studio or similar on localhost:8000
go run cmd/gateway/main.go \
  -provider local \
  -local-url http://localhost:8000 \
  -local-model llama-2-7b
```

### Real Anthropic API
```bash
go run cmd/gateway/main.go \
  -provider real \
  -anthropic-key sk-ant-xxxxxxxxxxxx
```

Or use environment variables:
```bash
export ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxx
go run cmd/gateway/main.go -provider real
```

## Authentication

Protect the gateway with an API key:

```bash
go run cmd/gateway/main.go \
  -provider mock \
  -api-key my-secret-key-123
```

Then use it in requests:
```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer my-secret-key-123"

# or
curl http://localhost:8080/v1/models \
  -H "x-api-key: my-secret-key-123"
```

## Custom Port

```bash
go run cmd/gateway/main.go -provider mock -addr :3000
# Gateway now at http://localhost:3000
```

## Available Models

### Pre-configured Models

| Model ID | Provider | Capabilities | Cost |
|----------|----------|--------------|------|
| `claude-opus` | Anthropic | Chat, Batch, Reasoning, Code Analysis | $$$ |
| `claude-sonnet` | Anthropic | Chat, Batch, Code Analysis | $$ |
| `claude-haiku` | Anthropic | Chat, Batch | $ |
| `mock-model` | Mock | Chat, Batch | Free |
| `local-llm` | Local | Chat | Free |

### Adding Custom Models

In `internal/models/registry.go`, register new models:

```go
registry.RegisterModel(&ModelInfo{
    ID:          "my-model",
    Provider:    "anthropic",
    Name:        "my-model",
    DisplayName: "My Custom Model",
    Capabilities: []ModelCapability{
        CapabilityChat,
        CapabilityBatch,
    },
    MaxTokens:       4096,
    ContextWindow:   4096,
    CostPer1KInput:  1.5,
    CostPer1KOutput: 6.0,
    Enabled:         true,
    Description:     "My custom model description",
})
```

## OpenAI-Compatible API Reference

### Chat Completions

**Endpoint:** `POST /v1/chat/completions`

**Request:**
```json
{
  "model": "mock-model",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "max_tokens": 100,
  "temperature": 0.7
}
```

**Response:**
```json
{
  "id": "msg_mock_1234567890",
  "object": "chat.completion",
  "created": 1714521600,
  "model": "mock-model",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Mock response for model mock-model with 5 input tokens"
      },
      "finish_reason": "end_turn"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 10,
    "total_tokens": 15
  }
}
```

### List Models

**Endpoint:** `GET /v1/models`

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "mock-model",
      "object": "model",
      "owned_by": "mock"
    },
    {
      "id": "claude-opus",
      "object": "model",
      "owned_by": "anthropic"
    }
  ]
}
```

### Health Check

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "ok",
  "models": 3
}
```

### Metrics

**Endpoint:** `GET /metrics`

**Response:**
```json
{
  "total_requests": 42,
  "total_tokens": 15234,
  "total_cost": 0.45,
  "by_model": {
    "mock-model": {
      "requests": 42,
      "input_tokens": 5000,
      "output_tokens": 10234,
      "cost": 0.0,
      "last_used": "2026-05-01T12:34:56Z",
      "avg_latency": 50.5
    }
  },
  "by_provider": {
    "mock": {
      "requests": 42,
      "successful": 42,
      "failed": 0,
      "last_used": "2026-05-01T12:34:56Z"
    }
  }
}
```

## Usage for Phase 1 & Phase 2 Testing

### Integration Testing (Phase 1)

Start the gateway with mock API:
```bash
go run cmd/gateway/main.go -provider mock &
```

Run integration tests against it:
```bash
# Tests can use standard OpenAI-compatible clients
OPENAI_API_KEY="" python test_integration.py --api-url http://localhost:8080
```

**Benefits:**
- Zero API costs
- Instant responses
- Perfect for validating API contracts
- Easy to simulate failures and edge cases

### Load Testing (Phase 2)

```bash
# Start gateway with mock API
go run cmd/gateway/main.go -provider mock &

# Run load tests
go run cmd/load-test/main.go \
  -url http://localhost:8080/v1/chat/completions \
  -duration 60s \
  -concurrency 1000 \
  -requests-per-sec 1000

# Monitor metrics
watch -n 1 'curl http://localhost:8080/metrics | jq'
```

**Load Testing Scenarios:**
1. Constant load: 1000 req/sec for 60min
2. Burst load: Spike to 5000 req/sec for 10sec
3. Mixed workload: 60% cache hits, 30% batch, 10% fresh

**Expected Results with Mock Provider:**
- Latency: <10ms (instant responses)
- Error rate: 0% (unless explicitly configured)
- Memory: <50MB
- CPU: Minimal

## Architecture

```
┌─────────────────────────────────────┐
│    Application/Test Client          │
│  (uses OpenAI-compatible SDK)       │
└────────────────┬────────────────────┘
                 │ HTTP
┌────────────────▼────────────────────┐
│   LLMSentinel Gateway Server        │
│  (internal/gateway/server.go)       │
│                                     │
│  - Request routing                  │
│  - Response caching                 │
│  - Metrics & observability          │
│  - Authentication                   │
└────────────────┬────────────────────┘
                 │
    ┌────────────┼────────────┐
    │            │            │
    ▼            ▼            ▼
┌────────┐  ┌────────┐  ┌────────┐
│  Mock  │  │ Local  │  │  Real  │
│   API  │  │  LLM   │  │  APIs  │
│ (free) │  │(free)  │  │($)     │
└────────┘  └────────┘  └────────┘
```

## Provider Implementation

### Mock Provider (`internal/mock/anthropic.go`)
- Simulates API responses without making real calls
- Configurable delays and failure rates
- Perfect for testing

### Local Provider (`internal/llm/local_adapter.go`)
- Connects to LM Studio, Ollama, etc.
- OpenAI-compatible `/v1/chat/completions`
- Self-hosted, free

### Real Provider (`internal/client/anthropic.go`)
- Connects to actual Anthropic API
- Use only for final validation

## Model Registry (`internal/models/registry.go`)

The registry manages available models with:
- Capabilities (Chat, Batch, Reasoning, etc.)
- Pricing information
- Provider information
- Enabled/disabled status

```go
// Find cheapest model with specific capability
model := registry.FindCheapestWithCapability(
    models.CapabilityChat,
    inputTokens,
    outputTokens,
)

// List all enabled models
for _, model := range registry.GetEnabledModels() {
    fmt.Printf("%s (provider: %s)\n", model.ID, model.Provider)
}
```

## Unified Client Routing

The unified client supports multiple routing strategies:

```go
// Capability-based routing
router := &CapabilityRouter{
    Capability: models.CapabilityReasoning,
    InputSize:  5000,
    OutputSize: 2000,
}
modelID, _ := router.SelectModel(registry)

// Cost-optimized routing
router := &CostOptimizedRouter{
    InputSize:  5000,
    OutputSize: 2000,
}

// Preferred model routing
router := &PreferredModelRouter{
    Preferences: []string{"claude-opus", "claude-sonnet", "claude-haiku"},
}
```

## Troubleshooting

### "Model not found"
Ensure the model ID is correct and enabled:
```bash
curl http://localhost:8080/v1/models | jq .
```

### Local LLM connection failed
Verify the local LLM is running:
```bash
curl http://localhost:8000/v1/models  # adjust port as needed
```

### High latency with mock API
The mock provider intentionally adds 10ms delay per request. For benchmarking, disable this:
- Edit `internal/mock/anthropic.go`
- Set `messageDelay: 0`

### Provider authentication failed
Check environment variables are set:
```bash
echo $ANTHROPIC_API_KEY
echo $LOCAL_LLM_URL
echo $LOCAL_LLM_MODEL
```

## Next Steps

1. **Phase 1: Integration Testing**
   - Use mock provider for cost-free testing
   - Validate API contracts
   - Test error handling

2. **Phase 2: Load Testing**
   - 1000 req/sec sustained
   - Monitor latency, throughput, memory
   - Test under burst load

3. **Production Deployment**
   - Switch to real provider
   - Enable authentication
   - Add request logging
   - Set up monitoring

## Related Documentation

- [Production Readiness Plan](../PLAN.md)
- [Security Audit](SECURITY_AUDIT.md)
- [Database Finalization](DATABASE_FINALIZATION.md)
- [GoModel Reference](https://github.com/ENTERPILOT/GoModel)

## License

MIT - See LICENSE file
