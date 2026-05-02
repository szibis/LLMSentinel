# Claude API Compliance - Full Compatibility Layer

Strategy to make any model mock the Claude API 100% accurately.

## What We Need

1. **Exact Claude API Response Format**
   - Message object structure
   - Error format and codes  
   - Token counting precision
   - Timing and metadata

2. **Any Model Backend**
   - Llama, Mistral, local anything
   - Should respond like Claude
   - Same output quality tier mapping

3. **Multi-CLI Support**
   - Claude CLI can talk to our mock
   - Codex CLI can talk to our mock
   - Gemini CLI can talk to our mock
   - All use same gateway

## Implementation Plan

### Phase 1: Claude API Compliance Validator
- Create strict validator for response format
- Compare mock vs real Claude responses
- Identify differences
- Fix until 100% compatible

### Phase 2: Local Model Adapters
- Llama → Claude format converter
- Mistral → Claude format converter
- Generic LLM → Claude format
- Quality tier detection (map to Haiku/Sonnet/Opus)

### Phase 3: Multi-CLI Gateway
- Expose via HTTP (OpenAI-compatible AND Claude-compatible)
- CLI detection (what tool is calling?)
- Response format adaptation per CLI tool
- Authentication per tool

### Phase 4: Validation Tests
- Real vs Mock comparison tests
- CLI integration tests
- Quality metrics matching
- Token counting validation

## Key Compliance Points

### Response Format
```json
{
  "id": "msg_013909...",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "..."
    }
  ],
  "model": "claude-opus",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 123,
    "output_tokens": 456
  }
}
```

### Error Format
```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "..."
  }
}
```

### Token Counting
- Input: actual token count (Claude formula)
- Output: actual token count
- Caching: token delta shown separately
- Matching real API to ±5% tolerance

## Success Criteria

✅ Mock API response matches real Claude API 100%
✅ Any local model maps to Claude quality tier
✅ Token counts accurate to ±5%
✅ Claude CLI works with mock gateway
✅ Codex CLI works with mock gateway
✅ Gemini CLI works with mock gateway
✅ Integration tests validate every field
