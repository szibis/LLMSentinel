# Intelligent Hybrid Routing - Auto Model Selection & Token Optimization

Automatically select the best model (local, mock, or cloud) based on task type, data sensitivity, and complexity. Includes transparent token optimization and intelligent fallback strategies.

## Quick Concept

```
User Request
    │
    ├─→ TaskDetector analyzes text
    │   - What type of work? (simple, reasoning, security, etc.)
    │   - How sensitive is the data? (public, secrets, confidential)
    │   - How complex? (1-10 rating)
    │
    ├─→ IntelligentRouter makes decision
    │   - Check custom rules
    │   - Match against task type
    │   - Consider sensitivity
    │   - Apply defaults
    │
    ├─→ Select best model
    │   - Secrets → Local/Offline
    │   - Simple → Cheap (Haiku, mock)
    │   - Complex → Capable (Opus)
    │
    ├─→ Optimize tokens
    │   - Simple: 70-80% reduction
    │   - Complex: Preserve context
    │   - Security: Add reminders
    │
    └─→ Execute with fallback
        - Try preferred model
        - Fall back if needed
        - Cache response
```

## Task Type Detection

The system automatically detects what kind of work is needed:

| Task Type | Keywords | Best Model | Cost |
|-----------|----------|-----------|------|
| **Simple** | "what is", "explain", "list", "define" | Haiku | Low |
| **Analysis** | "analyze", "review", "inspect", "parse" | Sonnet | Medium |
| **Reasoning** | "debug", "solve", "think through", "trace" | Opus | High |
| **Planning** | "design", "architect", "structure", "strategy" | Opus | High |
| **Code Gen** | "generate code", "implement", "create class" | Opus | High |
| **Creative** | "write", "create", "draft", "compose" | Sonnet | Medium |
| **Debugging** | "error", "broken", "crash", "fix" | Sonnet | Medium |
| **Security** | "secure", "encrypt", "vulnerability" | Local-First | Varies |

## Data Sensitivity Detection

The system detects sensitive content and routes appropriately:

```
Public Data
  └─→ Can use any model (cloud preferred for cost savings)

Internal Data
  └─→ Consider using local models if available

Confidential Data (medical, financial, personal)
  └─→ Use local/offline models when possible

Secrets (passwords, tokens, API keys, credentials)
  └─→ ALWAYS local/offline, never cloud
```

### Sensitive Pattern Examples
```
- "password", "secret", "token", "API_KEY"
- "credit card", "SSN", "medical", "health"
- "private", "confidential", "classified"
```

## Complexity Calculation

System estimates complexity 1-10 based on:
- **Text length**: More words = more complex
- **Code indicators**: `{}[]();<>=` characters suggest code
- **Structured data**: JSON/XML indicates complexity
- **Numbers/math**: Numerical content adds complexity

Examples:
```
Complexity 1-3 (Simple):
  "What is Python?"
  "How do I install Node?"
  "List the files"

Complexity 4-6 (Medium):
  "Analyze this code and find issues"
  "Design a cache system for API responses"
  
Complexity 7-10 (Complex):
  "Debug this race condition in concurrent code with detailed analysis"
  "Design a microservices architecture for handling 10k req/sec"
```

## Automatic Routing Rules

### Default Rules (Built-in)

**Rule 1: Secrets Always Local**
```
If: Data Sensitivity == "secrets"
Then: Use local-llm (or mock-model as fallback)
Why: Never send credentials to cloud APIs
```

**Rule 2: Simple Tasks Use Cheap Models**
```
If: Complexity ≤ 3 AND Task Type == "simple"
Then: Use claude-haiku (or mock-model)
Why: Save 80% on API costs
```

**Rule 3: Code Generation Needs Power**
```
If: Task Type == "codegen" AND Complexity ≥ 5
Then: Use claude-opus
Why: Complex code requires advanced reasoning
```

**Rule 4: Reasoning Requires Capability**
```
If: Task Type == "reasoning" AND Complexity ≥ 6
Then: Use claude-opus
Why: Complex logic needs best-in-class model
```

**Rule 5: Analysis Can Use Mid-Tier**
```
If: Task Type == "analysis" AND Complexity ≥ 4
Then: Use claude-sonnet
Why: Good balance of capability and cost
```

### Custom Rules

Define your own rules:

```go
router.AddRule(RoutingRule{
    Name: "internal-data-local",
    Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
        return ds == SensitivityInternal && c <= 5
    },
    PreferredModel: "local-llm",
    Fallback:      []string{"claude-haiku"},
    TokenSavings:  true,
})
```

## Token Optimization Strategies

### Per-Task-Type Optimization

**Simple Tasks (70-80% savings)**
```
Original: "Please explain quantum computing with examples"
Optimized: "Explain quantum computing"

Techniques:
- Remove examples ("e.g.", "For example:")
- Remove extra details
- Reduce system message
- Aggressive summarization
```

**Complex Tasks (0-10% savings)**
```
Original: "Debug this race condition: [complex code]..."
Optimized: "Debug this race condition: [complex code]..." (unchanged)

Techniques:
- Preserve all context
- Keep code intact
- Maintain structure
- Full system message
```

**Security Tasks (special handling)**
```
Original: "[user request with secrets]"
Optimized: "[SENSITIVE: Don't log/store] [user request]"

Techniques:
- Add security reminder
- Don't aggressive reduce
- Preserve context for accuracy
- Flag as sensitive internally
```

## Hybrid Execution - Local + Cloud Fallback

Intelligently try local first, fall back to cloud:

```go
executor := routing.NewHybridExecutor(processor, HybridPreferences{
    PreferLocal:     true,
    MaxLocalTokens:  2048,
    OfflineFirstFor: []TaskType{TaskTypeSecurity, TaskTypeDebug},
    FallbackToCloud: true,
})

// Automatic hybrid execution
resp, info, _ := executor.Execute(ctx, userRequest)

// Execution details
fmt.Printf("Executed on: %s\n", info.Attempts[0].Model)
fmt.Printf("Executed locally: %v\n", info.ExecutedLocally)
if len(info.Attempts) > 1 {
    fmt.Printf("Fallback used: %s → %s\n", 
        info.Attempts[0].Model,
        info.Attempts[1].Model)
}
```

### Fallback Sequence

```
1. Try preferred model (based on routing)
   ├─ Success? Return response
   ├─ Fail? Check if fallback enabled
   │
2. Try fallback model #1
   ├─ Success? Return response
   ├─ Fail? Try next fallback
   │
3. Try fallback model #2, etc.
   │
4. All failed? Return error with execution history
```

## Response Caching

Avoid duplicate API calls:

```go
processor := routing.NewRequestProcessor(router, client)

// First request - hits API
resp1, processed1, _ := processor.ExecuteRequest(ctx, "What is 2+2?")

// Second identical request - served from cache (0ms, 0 cost)
resp2, processed2, _ := processor.ExecuteRequest(ctx, "What is 2+2?")

// Cache stats
stats := processor.GetCacheStats()
fmt.Printf("Cached responses: %d\n", stats["cached_responses"])
```

## Real-World Examples

### Example 1: Simple Question → Auto Routing
```
Input:  "What is Python?"
Task:   TaskTypeSimple
Sensitivity: Public
Complexity: 1

Decision:
  ├─ Routing rule "simple-cheap" matches
  ├─ Select: claude-haiku
  ├─ Optimize: Yes (remove unnecessary words)
  ├─ Tokens saved: 75%
  └─ Cost: $0.00008

Execution:
  └─ Success on haiku, no fallback needed
```

### Example 2: Security Task → Force Local
```
Input:  "Encrypt this password: secret123 using RSA"
Task:   TaskTypeSecurity
Sensitivity: Secrets (detected "password")
Complexity: 4

Decision:
  ├─ Sensitivity == Secrets
  ├─ Routing rule "secrets-local-only" matches
  ├─ Select: local-llm
  ├─ Add security context: Yes
  └─ Cloud model: Not used (never sent)

Execution:
  ├─ Try local-llm: Success
  ├─ Never attempt cloud
  └─ Cost: $0.00000 (fully local)
```

### Example 3: Complex Reasoning → Auto Escalate
```
Input:  "Design a distributed cache system for handling 10k req/sec with cache invalidation"
Task:   TaskTypePlanning
Sensitivity: Internal
Complexity: 9

Decision:
  ├─ Complexity >= 6 AND TaskTypePlanning
  ├─ Routing rule "reasoning-capable" matches
  ├─ Select: claude-opus
  ├─ Optimize: No (preserve full context)
  ├─ Fallback: claude-sonnet
  └─ Cost: $0.003-0.015 (proportional to output)

Execution:
  ├─ Try opus: Success
  ├─ Response cached
  └─ Same request later: instant (from cache)
```

### Example 4: Debugging with Hybrid Fallback
```
Input:  "[error logs] Why is my database query slow?"
Task:   TaskTypeDebug
Sensitivity: Internal
Complexity: 6

Decision:
  ├─ Task Type: Debug
  ├─ HybridPreferences.OfflineFirstFor includes Debug
  ├─ Preferred: local-llm
  ├─ Fallback: claude-sonnet
  └─ FallbackToCloud: true

Execution:
  Attempt 1: Try local-llm
    ├─ Request sent to local inference server
    ├─ Local server timeout (slow)
    └─ Fallback enabled: Continue
  
  Attempt 2: Try claude-sonnet
    ├─ Request sent to Anthropic API
    ├─ Success: Return response
    └─ Log: "Local timeout → Cloud fallback"
```

## Configuration

### Environment Variables
```bash
# Task detection sensitivity
TASK_DETECTION_MODE=strict      # "strict", "lenient"

# Routing preferences
PREFER_LOCAL=true               # Prefer local models
PREFER_MOCK=false               # Prefer mock (testing)
PREFER_CHEAP=true               # Prefer cost-optimized models

# Token optimization
AGGRESSIVE_TOKEN_REDUCTION=true # Aggressive vs conservative

# Hybrid execution
HYBRID_FALLBACK=true            # Enable cloud fallback
MAX_LOCAL_TOKENS=2048           # Max tokens for local models
OFFLINE_FIRST_TASKS="security,debug"  # CSV of task types
```

### Programmatic Configuration
```go
detector := routing.NewTaskDetector()

defaults := routing.RoutingDefaults{
    PreferLocal:      true,
    PreferMock:       false,
    SensitiveOnly:    routing.SensitivitySecrets,
    TokenOptimization: true,
    FallbackStrategy: "cost",
}

router := routing.NewIntelligentRouter(registry, defaults)
```

## Performance Impact

### Latency
- **Task detection**: <1ms (regex matching)
- **Routing decision**: <1ms (rule evaluation)
- **Token optimization**: 1-5ms (text processing)
- **Cache lookup**: <0.1ms
- **Total overhead**: <10ms

### Cost Savings
| Workload | Without | With Routing | Savings |
|----------|---------|--------------|---------|
| 80% simple tasks | $100 | $20 | 80% |
| Mixed (20% complex) | $100 | $35 | 65% |
| All complex tasks | $100 | $95 | 5% |
| With cache hits (30%) | $100 | $15 | 85% |

### Example: $10K/month optimization
```
Baseline: 100M tokens/month = $10,000

With Intelligent Routing:
  ├─ 60% simple tasks → Haiku: $4,000 → $800 (80% savings)
  ├─ 30% mid tasks → Sonnet: $4,500 → $3,500 (22% savings)
  ├─ 10% complex → Opus: $1,500 → $1,500 (0% savings)
  └─ Cache hits (20% reduction): $9,800 → $7,840

Result: $10,000 → $3,700 = 63% savings
```

## Best Practices

1. **Start with defaults**: Built-in rules cover most cases
2. **Add custom rules carefully**: Each rule adds latency
3. **Monitor decisions**: Log which model was selected for analysis
4. **Test fallbacks**: Ensure cloud fallback works in staging
5. **Cache responsibly**: Clear cache when user context changes
6. **Adjust complexity**: Tune complexity thresholds for your domain
7. **Security first**: Always err on side of local for sensitive data

## Troubleshooting

**Q: My simple task is using Opus instead of Haiku**
A: Check if it was detected as different task type:
```go
detector := routing.NewTaskDetector()
tt, _, c := detector.DetectTask(yourText)
fmt.Printf("Detected: %s, Complexity: %d\n", tt, c)
```

**Q: Local model is timing out, cloud fallback not working**
A: Ensure hybrid preferences are configured:
```go
prefs := routing.HybridPreferences{
    FallbackToCloud: true,  // Must be true
}
```

**Q: Response cache is stale**
A: Clear when context changes:
```go
processor.ClearCache()  // Fresh cache for new context
```

**Q: Token savings not as expected**
A: Check optimization strategy:
```go
strategy := router.GetTokenOptimizationStrategy(taskType)
fmt.Printf("Aggressive: %v\n", strategy.Aggressive)
```

## Related Documentation

- [Gateway Setup](GATEWAY_SETUP.md) - HTTP gateway configuration
- [Production Readiness Plan](PLAN.md) - Phase 1-5 implementation
- [Model Registry](internal/models/registry.go) - Available models
- [GoModel](https://github.com/ENTERPILOT/GoModel) - Reference implementation

## License

MIT - See LICENSE file
