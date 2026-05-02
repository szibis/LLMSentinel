# Multi-CLI Support - Claude, Codex, Gemini, OpenAI

Make the mock API work with ANY CLI tool by supporting multiple API formats.

## Architecture

```
┌─────────────────────────────────────────┐
│   Claude CLI / Codex CLI / Gemini CLI   │
└────────────────────┬────────────────────┘
                     │ HTTP Request
                     │ (CLI-specific format)
                     ▼
        ┌────────────────────────┐
        │   HTTP Gateway         │
        │                        │
        │ Detect CLI tool        │
        │ (from User-Agent)      │
        └────────────┬───────────┘
                     │
        ┌────────────▼───────────┐
        │  Format Converter      │
        │                        │
        │ Claude ←→ OpenAI       │
        │ Claude ←→ Gemini       │
        │ Claude ←→ Codex        │
        └────────────┬───────────┘
                     │
        ┌────────────▼───────────┐
        │  Unified Backend       │
        │                        │
        │ Mock API               │
        │ Local LLM              │
        │ Real Anthropic API     │
        └────────────────────────┘
```

## Supported CLI Tools

### Claude CLI
```bash
# Native support - uses Claude API format
claude chat "Hello, how are you?"

# Response: Claude API format
{
  "id": "msg_...",
  "type": "message",
  "role": "assistant",
  "content": [{...}],
  "model": "claude-opus",
  "usage": {...}
}
```

### Codex CLI
```bash
# OpenAI-compatible format
codex chat "Hello"

# Response: OpenAI format
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "choices": [{...}],
  "usage": {...}
}
```

### Gemini CLI
```bash
# Google Gemini format
gemini chat "Hello"

# Response: Gemini format
{
  "candidates": [{...}],
  "usage_metadata": {...}
}
```

### OpenAI API
```bash
# Standard OpenAI format
curl https://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-..."

# Response: OpenAI format
```

## Implementation Strategy

### Step 1: Format Detection
Identify which CLI is calling by analyzing:
- HTTP User-Agent header
- Request format (body structure)
- Endpoint path
- Authentication method

```go
cliType := DetectCLI(r.Header.Get("User-Agent"))
// Returns: CLITypeClaude, CLITypeCodex, CLITypeGemini, or CLITypeOpenAI
```

### Step 2: Request Conversion
Convert incoming request from CLI format to internal Claude format:

```
Codex Request:
  {
    "model": "gpt-4",
    "messages": [{...}]
  }
        ↓
   Convert to Claude
        ↓
  {
    "model": "claude-opus",
    "messages": [{...}]
  }
```

### Step 3: Backend Processing
Process using unified backend (mock, local, or real):

```go
resp, err := unifiedClient.CreateMessage(ctx, message, model)
```

### Step 4: Response Conversion
Convert response back to CLI-specific format:

```
Claude Response:
  {
    "id": "msg_...",
    "content": [{...}],
    "usage": {...}
  }
        ↓
   Convert to Codex/Gemini/etc
        ↓
  OpenAI/Gemini/etc format
```

## Format Mapping

### Message Formats

| Field | Claude API | OpenAI | Gemini |
|-------|-----------|--------|--------|
| Messages | `messages[]` | `messages[]` | `contents[]` |
| Role | `user/assistant` | `user/assistant` | `user/model` |
| Content | `content[].text` | `content` | `parts[].text` |
| Model | `model` | `model` | `model` |
| Tokens | `usage` | `usage` | `usage_metadata` |

### Stop Reasons

| Claude | OpenAI | Gemini |
|--------|--------|--------|
| `end_turn` | `stop` | `STOP` |
| `max_tokens` | `length` | `MAX_TOKENS` |
| `stop_sequence` | `stop` | `STOP` |

## Real-World Examples

### Example 1: Claude CLI → Mock API

User runs:
```bash
# Using Claude CLI
claude chat "What is Python?"
```

System:
1. Claude CLI sends HTTP request with Claude API format
2. Gateway detects "Claude CLI" from User-Agent
3. Request already in correct format, pass through
4. Mock API processes
5. Response returned in Claude format
6. Claude CLI displays response

### Example 2: OpenAI API → Mock as Claude

User runs:
```bash
# Using OpenAI Python SDK pointing to local gateway
client = OpenAI(base_url="http://localhost:8080", api_key="...")
response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello"}]
)
```

System:
1. OpenAI SDK sends OpenAI-format request
2. Gateway detects OpenAI format
3. Converts: `gpt-4` → `claude-opus`, OpenAI format → Claude format
4. Mock API processes as Claude
5. Response converted: Claude format → OpenAI format
6. OpenAI SDK receives expected OpenAI response

### Example 3: Gemini CLI → Claude Backend

User runs:
```bash
# Using Gemini CLI
gemini chat "How does quantum computing work?"
```

System:
1. Gemini CLI sends Gemini-format request
2. Gateway detects Gemini format
3. Converts: Gemini format → Claude format
4. Backend (mock, local, or real Claude) processes
5. Response converted: Claude format → Gemini format
6. Gemini CLI displays response

## Configuration

### Environment Variables
```bash
# CLI format support
ENABLE_CLAUDE_FORMAT=true
ENABLE_OPENAI_FORMAT=true
ENABLE_GEMINI_FORMAT=true

# Provider backend
LLM_PROVIDER=mock              # Test
LLM_PROVIDER=local             # Self-hosted
LLM_PROVIDER=real              # Production
```

### HTTP Headers
```bash
# Claude CLI
User-Agent: claude-cli/1.0
Authorization: Bearer sk-...

# OpenAI SDK
User-Agent: OpenAI/python-client
Authorization: Bearer sk-...

# Gemini CLI
User-Agent: google-gemini-cli/1.0
Authorization: Bearer ...
```

## Testing Multi-CLI Support

### Test 1: Claude CLI Format
```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "User-Agent: claude-cli/1.0" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "Hi"}]
  }'
```

### Test 2: OpenAI Format
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "User-Agent: OpenAI/python" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hi"}]
  }'
```

### Test 3: Gemini Format
```bash
curl -X POST http://localhost:8080/v1/models/gemini-pro:generateContent \
  -H "User-Agent: google-gemini-cli" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{"text": "Hi"}]
    }]
  }'
```

## Implementation Checklist

- [ ] Implement CLI detection from User-Agent
- [ ] Create format converters (Claude ↔ OpenAI)
- [ ] Create format converters (Claude ↔ Gemini)
- [ ] Update gateway to detect and convert formats
- [ ] Test with Claude CLI
- [ ] Test with OpenAI Python SDK
- [ ] Test with Gemini CLI
- [ ] Validate token counting consistency
- [ ] Document multi-CLI setup
- [ ] Create multi-CLI integration tests

## Benefits

✅ **Single gateway for all CLI tools**
✅ **Transparent format conversion**
✅ **Same mock backend for all CLIs**
✅ **Zero-cost testing across platforms**
✅ **Easy to add new CLI tools**
✅ **Consistent token counting**
✅ **Production-ready format validation**

## Future Enhancements

1. **Format Auto-Detection**: Detect format from request body structure
2. **CLI-Specific Features**: Support tool_use, vision, streaming per CLI
3. **Rate Limiting per CLI**: Different limits for different tools
4. **Analytics per CLI**: Track usage by CLI tool
5. **Cost Optimization per CLI**: Different strategies per tool

## Files Involved

```
internal/
├── gateway/
│   ├── server.go          (Add format conversion)
│   └── adapter_cli.go     (Existing CLI execution)
└── mock/
    ├── anthropic.go       (Mock responses)
    └── compliance_validator.go (Format validation)
```

## Next Steps

1. Implement format detection in gateway
2. Add format conversion middleware
3. Test with real Claude CLI
4. Test with OpenAI SDK
5. Test with Gemini CLI
6. Document setup for each CLI tool
