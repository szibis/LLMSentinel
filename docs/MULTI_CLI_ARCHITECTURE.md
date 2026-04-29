# Multi-CLI Architecture Design

**Document Type**: Technical Architecture  
**Status**: Design (Pre-Implementation)  
**Complexity**: High (4 provider integrations)  
**Time Estimate**: 8-13 weeks (phased)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      Escalate Multi-CLI System                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐            │
│  │ Claude CLI   │  │ Copilot CLI  │  │ Gemini CLI   │  ...       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘            │
│         │                 │                 │                    │
│  ┌──────▼─────────────────▼─────────────────▼────────────────┐  │
│  │            Provider Abstraction Layer                      │  │
│  │  (Generic interface for all CLI providers)                 │  │
│  └──────┬──────────────────────────────────────────────────┬──┘  │
│         │                                                  │      │
│  ┌──────▼──────────┐  ┌──────────────────┐  ┌──────────────▼─┐   │
│  │ ClaudeProvider  │  │ GeminiProvider   │  │ OpenAIProvider │   │
│  │ CopilotProvider │  │                  │  │                │   │
│  └──────┬──────────┘  └──────┬───────────┘  └────────┬───────┘   │
│         │                    │                       │           │
│  ┌──────▼────────────────────▼───────────────────────▼────────┐  │
│  │           Multi-Provider Execution Engine                  │  │
│  │  - Unified logging (all operations → .execution-log.jsonl) │  │
│  │  - Analytics (per-provider cost, token, duration)          │  │
│  │  - Sentiment detection (provider-specific)                 │  │
│  │  - Model escalation (per-provider hierarchies)             │  │
│  │  - Budget enforcement (per-provider budgets)               │  │
│  │  - Cross-provider fallback                                 │  │
│  └──────┬─────────────────────────────────────────────────────┘  │
│         │                                                         │
│  ┌──────▼─────────────────────────────────────────────────────┐  │
│  │         Unified Analytics & Dashboard Layer                │  │
│  │  - Real-time operation tracking (all providers)            │  │
│  │  - Cost aggregation (per-provider + total)                 │  │
│  │  - Pattern generation (cross-provider trends)              │  │
│  │  - Budget alerts (per-provider + global)                   │  │
│  │  - Sentiment trends (availability per provider)            │  │
│  └──────┬─────────────────────────────────────────────────────┘  │
│         │                                                         │
│  ┌──────▼─────────────────────────────────────────────────────┐  │
│  │     Storage & Persistence                                  │  │
│  │  - .execution-log.jsonl (unified operations log)           │  │
│  │  - analytics.db (SQLite: metrics, budgets, trends)         │  │
│  │  - EXECUTION_PATTERNS.md (cross-provider patterns)         │  │
│  │  - config.yaml (unified config, per-provider settings)     │  │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Component Details

### 1. Provider Abstraction Layer

#### Interface Definition

```go
// internal/providers/provider.go

// ExecutionRequest represents a unified request across providers
type ExecutionRequest struct {
    Provider string              // "claude", "gemini", "openai", "copilot"
    Model    string              // "claude-opus", "gemini-pro", "gpt-4"
    Prompt   string              // User prompt
    Context  string              // Optional context/system prompt
    
    // Optional parameters
    Temperature float32
    MaxTokens   int
    TopP        float32
}

// ExecutionResponse represents response from any provider
type ExecutionResponse struct {
    Provider      string          // Which provider handled this
    Model         string          // Which model was used
    Content       string          // Generated content
    
    // Cost tracking
    TokensInput   int
    TokensOutput  int
    EstimatedCost float64         // Calculated from token count + pricing
    
    // Metadata
    Duration      time.Duration
    Timestamp     time.Time
    RawResponse   json.RawMessage // Store original response for audit
}

// Provider is the core interface all implementations must satisfy
type Provider interface {
    // Identity
    Name() string                            // "claude", "gemini", "openai", "copilot"
    
    // Authentication & Validation
    IsAuthenticated() bool
    ValidateAuth() error
    Authenticate(ctx context.Context) error // For OAuth flows
    
    // Model Information
    AvailableModels() []string              // ["claude-opus", "claude-sonnet", ...]
    EscalationChain() []string              // Cheap to expensive: [Haiku, Sonnet, Opus]
    DefaultModel() string
    
    // Pricing & Cost
    CostPerToken(model string) (input, output float64)  // Cost per 1K tokens
    IsTokenCountNative() bool                // True if API returns token counts
    MaxTokensPerRequest(model string) int
    
    // Capabilities
    Capabilities() ProviderCapabilities
    
    // Execution
    Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResponse, error)
    
    // Streaming (optional)
    ExecuteStream(ctx context.Context, req ExecutionRequest) (<-chan StreamChunk, error)
}

// ProviderCapabilities describes what this provider supports
type ProviderCapabilities struct {
    SentimentDetection      bool
    ModelEscalation         bool
    TokenCountingNative     bool
    ToolUseSupport          bool
    StreamingSupport        bool
    MultiModalInput         bool  // Image/audio/video
    ContextWindow           int
}

// ProviderFactory creates provider instances
type ProviderFactory struct {
    config Config
    cache  map[string]Provider
}

func (f *ProviderFactory) GetProvider(name string) (Provider, error) {
    if cached, ok := f.cache[name]; ok {
        return cached, nil
    }
    
    var provider Provider
    switch name {
    case "claude":
        provider = NewClaudeProvider(f.config.Providers.Claude)
    case "gemini":
        provider = NewGeminiProvider(f.config.Providers.Gemini)
    case "openai":
        provider = NewOpenAIProvider(f.config.Providers.OpenAI)
    case "copilot":
        provider = NewCopilotProvider(f.config.Providers.Copilot)
    default:
        return nil, fmt.Errorf("unknown provider: %s", name)
    }
    
    f.cache[name] = provider
    return provider, nil
}
```

#### Provider Implementations

```go
// internal/providers/claude.go
type ClaudeProvider struct {
    client *anthropic.Client
    config ClaudeConfig
    cache  *sync.Map  // Model cache
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) AvailableModels() []string {
    return []string{
        "claude-3-5-haiku-20241022",
        "claude-3-5-sonnet-20241022",
        "claude-opus-4-1-20250805",
    }
}

func (p *ClaudeProvider) EscalationChain() []string {
    return []string{
        "claude-3-5-haiku-20241022",
        "claude-3-5-sonnet-20241022",
        "claude-opus-4-1-20250805",
    }
}

func (p *ClaudeProvider) Capabilities() ProviderCapabilities {
    return ProviderCapabilities{
        SentimentDetection: true,
        ModelEscalation:    true,
        TokenCountingNative: true,
        ToolUseSupport:     true,
        StreamingSupport:   true,
        MultiModalInput:    true,
        ContextWindow:      200000,
    }
}

func (p *ClaudeProvider) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResponse, error) {
    // Use provided model or default
    model := req.Model
    if model == "" {
        model = p.config.DefaultModel
    }
    
    // Call Anthropic API
    resp, err := p.client.CreateMessage(ctx, anthropic.MessageRequest{
        Model:       model,
        MaxTokens:   req.MaxTokens,
        Temperature: req.Temperature,
        System:      req.Context,
        Messages: []anthropic.Message{
            {
                Role:    "user",
                Content: req.Prompt,
            },
        },
    })
    
    // Extract token count from response
    return &ExecutionResponse{
        Provider:      "claude",
        Model:         model,
        Content:       extractText(resp.Content),
        TokensInput:   resp.Usage.InputTokens,
        TokensOutput:  resp.Usage.OutputTokens,
        EstimatedCost: calculateCost(model, resp.Usage),
        Duration:      time.Since(startTime),
        Timestamp:     time.Now(),
        RawResponse:   mustMarshal(resp),
    }, nil
}

// Similar implementations for Gemini, OpenAI, Copilot...
```

### 2. Multi-Provider Execution Engine

```go
// internal/execution/multi_provider_engine.go

type MultiProviderEngine struct {
    providers      map[string]Provider
    logger         *ExecutionLogger
    analytics      *AnalyticsEngine
    budgetTracker  *BudgetTracker
    fallbackPolicy FallbackPolicy
}

type FallbackPolicy struct {
    Enabled              bool
    FallbackOrder        []string  // ["gemini", "openai"] if primary fails
    FallbackOnBudgetHit  bool      // Escalate to another provider?
    MaxFallbackDepth     int       // Prevent infinite loops
}

// Execute handles cross-provider execution with fallback
func (e *MultiProviderEngine) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResponse, error) {
    // 1. Validate authentication
    provider, err := e.providers[req.Provider]
    if err != nil || !provider.IsAuthenticated() {
        if e.fallbackPolicy.Enabled {
            return e.executeWithFallback(ctx, req)
        }
        return nil, fmt.Errorf("provider %s not authenticated", req.Provider)
    }
    
    // 2. Check budget
    cost := e.estimateCost(provider, req.Model, req.Prompt)
    if !e.budgetTracker.CanSpend(req.Provider, cost) {
        if e.fallbackPolicy.FallbackOnBudgetHit {
            return e.executeWithFallback(ctx, req)
        }
        return nil, fmt.Errorf("budget exceeded for provider %s", req.Provider)
    }
    
    // 3. Execute
    resp, err := provider.Execute(ctx, req)
    
    // 4. Log & track
    e.logger.Log(&ExecutionLog{
        Provider:      req.Provider,
        Model:         req.Model,
        Timestamp:     time.Now(),
        TokensInput:   resp.TokensInput,
        TokensOutput:  resp.TokensOutput,
        Cost:          resp.EstimatedCost,
        Duration:      resp.Duration,
        Success:       err == nil,
    })
    
    e.budgetTracker.Track(req.Provider, resp.EstimatedCost)
    
    return resp, err
}

// executeWithFallback tries secondary providers
func (e *MultiProviderEngine) executeWithFallback(ctx context.Context, req ExecutionRequest) (*ExecutionResponse, error) {
    originalProvider := req.Provider
    depth := 0
    
    for _, fallbackProvider := range e.fallbackPolicy.FallbackOrder {
        depth++
        if depth > e.fallbackPolicy.MaxFallbackDepth {
            break
        }
        
        req.Provider = fallbackProvider
        resp, err := e.Execute(ctx, req)
        if err == nil {
            // Log fallback chain
            e.logger.LogFallback(originalProvider, fallbackProvider, depth)
            return resp, nil
        }
    }
    
    return nil, fmt.Errorf("all providers exhausted (original: %s)", originalProvider)
}
```

### 3. Unified Execution Logging

```go
// internal/execlog/multi_provider_logger.go

type MultiProviderEntry struct {
    // Standard execution log fields
    Timestamp      time.Time
    SessionID      string
    OperationID    string
    
    // Multi-provider fields
    Provider       string         // "claude", "gemini", "openai", "copilot"
    Model          string         // Specific model used
    OperationType  string         // "completion", "sentiment", "escalation"
    
    // Metrics
    TokensInput    int
    TokensOutput   int
    EstimatedCost  float64
    Duration       time.Duration
    
    // Status
    Success        bool
    ErrorMessage   string
    
    // Decision context
    DecisionContext string  // "user_input", "sentiment_escalation", "budget_warning"
    
    // Fallback chain (if applicable)
    FallbackChain  []string  // [original_provider, fallback1, fallback2]
    FallbackDepth  int       // How many fallbacks tried?
}

// ExecutionLogger writes to unified log
type ExecutionLogger struct {
    file   *os.File
    mu     sync.Mutex
    config LogConfig
}

func (l *ExecutionLogger) Log(entry MultiProviderEntry) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    bytes, _ := json.Marshal(entry)
    _, err := l.file.Write(append(bytes, '\n'))
    return err
}

// Analytics reads unified log
func (l *ExecutionLogger) ReadAllEntries() ([]MultiProviderEntry, error) {
    // Parse .execution-log.jsonl with provider field
}

func (l *ExecutionLogger) EntriesByProvider(provider string) ([]MultiProviderEntry, error) {
    all, _ := l.ReadAllEntries()
    filtered := make([]MultiProviderEntry, 0)
    for _, e := range all {
        if e.Provider == provider {
            filtered = append(filtered, e)
        }
    }
    return filtered, nil
}

func (l *ExecutionLogger) CostSummaryByProvider() map[string]CostSummary {
    all, _ := l.ReadAllEntries()
    
    summary := make(map[string]CostSummary)
    for _, e := range all {
        s := summary[e.Provider]
        s.TotalCost += e.EstimatedCost
        s.TotalTokensInput += e.TokensInput
        s.TotalTokensOutput += e.TokensOutput
        s.OperationCount++
        summary[e.Provider] = s
    }
    return summary
}
```

### 4. Per-Provider Sentiment Detection

```go
// internal/sentiment/multi_provider_detector.go

type SentimentDetector interface {
    Detect(ctx context.Context, text string) (SentimentResult, error)
    Provider() string
}

// Claude implementation (full-featured, cheapest)
type ClaudeSentimentDetector struct {
    provider Provider
}

func (d *ClaudeSentimentDetector) Detect(ctx context.Context, text string) (SentimentResult, error) {
    // Use Claude Haiku for sentiment (cheap: $0.0008 per detection)
    resp, err := d.provider.Execute(ctx, ExecutionRequest{
        Model:  "claude-3-5-haiku-20241022",
        Prompt: sentimentPrompt(text),
    })
    
    // Parse response for sentiment score
    return parseSentimentResult(resp.Content)
}

// Gemini implementation (super cheap)
type GeminiSentimentDetector struct {
    provider Provider
}

func (d *GeminiSentimentDetector) Detect(ctx context.Context, text string) (SentimentResult, error) {
    // Use Gemini Flash for sentiment (super cheap: $0.00003 per detection)
    resp, err := d.provider.Execute(ctx, ExecutionRequest{
        Model:  "gemini-1.5-flash",
        Prompt: sentimentPrompt(text),
    })
    
    return parseSentimentResult(resp.Content)
}

// OpenAI implementation (more expensive than Claude)
type OpenAISentimentDetector struct {
    provider Provider
}

func (d *OpenAISentimentDetector) Detect(ctx context.Context, text string) (SentimentResult, error) {
    // Use GPT-3.5 for sentiment (cheap: $0.0005 per detection)
    resp, err := d.provider.Execute(ctx, ExecutionRequest{
        Model:  "gpt-3.5-turbo",
        Prompt: sentimentPrompt(text),
    })
    
    return parseSentimentResult(resp.Content)
}

// Factory with intelligent provider selection
type SentimentDetectorFactory struct {
    providers map[string]Provider
    preference string  // "claude" > "gemini" > "openai"
}

func (f *SentimentDetectorFactory) GetDetector() (SentimentDetector, error) {
    // Priority: Claude (most proven) > Gemini (super cheap) > OpenAI
    
    if p, ok := f.providers["claude"]; ok && p.IsAuthenticated() {
        return &ClaudeSentimentDetector{provider: p}, nil
    }
    
    if p, ok := f.providers["gemini"]; ok && p.IsAuthenticated() {
        return &GeminiSentimentDetector{provider: p}, nil
    }
    
    if p, ok := f.providers["openai"]; ok && p.IsAuthenticated() {
        return &OpenAISentimentDetector{provider: p}, nil
    }
    
    return nil, errors.New("no sentiment detection providers available")
}
```

### 5. Multi-Provider Budget Tracking

```go
// internal/budgets/multi_provider_budget_tracker.go

type BudgetTracker struct {
    mu        sync.RWMutex
    budgets   map[string]*ProviderBudget  // Per provider
    config    BudgetConfig
    logger    Logger
}

type ProviderBudget struct {
    Provider     string
    DailyLimit   float64
    MonthlyLimit float64
    DailySpent   float64
    MonthlySpent float64
    LastReset    time.Time
    HardLimit    bool  // Block if exceeded?
}

func (bt *BudgetTracker) CanSpend(provider string, cost float64) bool {
    bt.mu.RLock()
    defer bt.mu.RUnlock()
    
    budget, ok := bt.budgets[provider]
    if !ok {
        return true  // No limit set
    }
    
    if budget.DailySpent+cost > budget.DailyLimit && budget.HardLimit {
        bt.logger.Warn("Daily budget exceeded", "provider", provider)
        return false
    }
    
    if budget.MonthlySpent+cost > budget.MonthlyLimit && budget.HardLimit {
        bt.logger.Warn("Monthly budget exceeded", "provider", provider)
        return false
    }
    
    return true
}

func (bt *BudgetTracker) Track(provider string, cost float64) {
    bt.mu.Lock()
    defer bt.mu.Unlock()
    
    budget := bt.budgets[provider]
    budget.DailySpent += cost
    budget.MonthlySpent += cost
    
    // Check soft limits (warning threshold)
    if budget.DailySpent > budget.DailyLimit*0.8 {
        bt.logger.Info("Daily budget 80% spent", "provider", provider)
    }
}

func (bt *BudgetTracker) Report() BudgetReport {
    bt.mu.RLock()
    defer bt.mu.RUnlock()
    
    report := BudgetReport{
        Timestamp: time.Now(),
        Budgets:   make([]ProviderBudgetStatus, 0),
    }
    
    totalDaily := 0.0
    totalMonthly := 0.0
    
    for provider, budget := range bt.budgets {
        status := ProviderBudgetStatus{
            Provider:       provider,
            DailySpent:     budget.DailySpent,
            DailyLimit:     budget.DailyLimit,
            DailyRemaining: budget.DailyLimit - budget.DailySpent,
            DailyPercent:   (budget.DailySpent / budget.DailyLimit) * 100,
            MonthlySpent:   budget.MonthlySpent,
            MonthlyLimit:   budget.MonthlyLimit,
            MonthlyPercent: (budget.MonthlySpent / budget.MonthlyLimit) * 100,
        }
        report.Budgets = append(report.Budgets, status)
        totalDaily += budget.DailySpent
        totalMonthly += budget.MonthlySpent
    }
    
    report.TotalDaily = totalDaily
    report.TotalMonthly = totalMonthly
    
    return report
}
```

### 6. Cross-Provider Analytics

```go
// internal/analytics/multi_provider_analytics.go

type MultiProviderAnalytics struct {
    logger  *ExecutionLogger
    tracker *BudgetTracker
}

func (a *MultiProviderAnalytics) ProviderComparison() ProviderComparison {
    entries, _ := a.logger.ReadAllEntries()
    
    comparison := ProviderComparison{
        Timestamp: time.Now(),
        Providers: make(map[string]ProviderStats),
    }
    
    for _, e := range entries {
        stats := comparison.Providers[e.Provider]
        stats.Provider = e.Provider
        stats.OperationCount++
        stats.TotalCost += e.EstimatedCost
        stats.TotalDuration += e.Duration
        stats.SuccessCount += cond(e.Success, 1, 0)
        comparison.Providers[e.Provider] = stats
    }
    
    // Calculate averages
    for provider, stats := range comparison.Providers {
        stats.AverageCost = stats.TotalCost / float64(stats.OperationCount)
        stats.AverageDuration = stats.TotalDuration / time.Duration(stats.OperationCount)
        stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.OperationCount)
        comparison.Providers[provider] = stats
    }
    
    return comparison
}

func (a *MultiProviderAnalytics) CostOptimization() CostOptimizations {
    entries, _ := a.logger.ReadAllEntries()
    
    opts := CostOptimizations{
        Recommendations: []string{},
    }
    
    // Analyze by provider
    providerCosts := make(map[string]float64)
    for _, e := range entries {
        providerCosts[e.Provider] += e.EstimatedCost
    }
    
    // Recommend switching if one provider is consistently cheaper
    if cheapest := findCheapestProvider(providerCosts); cheapest != "" {
        opts.Recommendations = append(opts.Recommendations,
            fmt.Sprintf("Switch primary provider to %s (%.2f%% savings)", 
                cheapest, savingsPercent(providerCosts, cheapest)))
    }
    
    // Recommend escalation strategy
    opts.Recommendations = append(opts.Recommendations,
        "Enable cross-provider fallback: escalate to Gemini if Claude over budget")
    
    return opts
}
```

---

## Configuration Schema (YAML)

```yaml
# ~/.claude/escalation/config.yaml (unified multi-provider)

execution_feedback_loop:
  enabled: true
  log_location: ~/.claude/data/escalation/.execution-log.jsonl

providers:
  claude:
    enabled: true
    auth_key_var: ANTHROPIC_API_KEY
    
    models:
      cheap: claude-3-5-haiku-20241022
      mid: claude-3-5-sonnet-20241022
      advanced: claude-opus-4-1-20250805
    
    defaults:
      model: claude-3-5-sonnet-20241022
      temperature: 0.7
      max_tokens: 4096
    
    costs:
      haiku:
        input_per_1k: 0.003
        output_per_1k: 0.015
      sonnet:
        input_per_1k: 0.003
        output_per_1k: 0.015
      opus:
        input_per_1k: 0.015
        output_per_1k: 0.075
    
    budgets:
      daily_usd: 5.00
      monthly_usd: 100.00
      hard_limit: true
    
    features:
      sentiment_detection: true
      model_escalation: true
      token_tracking: true
  
  gemini:
    enabled: false
    auth_key_var: GCLOUD_API_KEY
    
    models:
      cheap: gemini-1.5-flash
      advanced: gemini-2.0-pro
    
    defaults:
      model: gemini-1.5-flash
      temperature: 0.7
    
    costs:
      flash:
        input_per_1m: 0.075
        output_per_1m: 0.30
      pro:
        input_per_1k: 0.01
        output_per_1k: 0.04
    
    budgets:
      daily_usd: 10.00
      monthly_usd: 200.00
      hard_limit: true
    
    features:
      sentiment_detection: true
      model_escalation: true
      token_tracking: true
  
  openai:
    enabled: false
    auth_key_var: OPENAI_API_KEY
    
    models:
      cheap: gpt-3.5-turbo
      advanced: gpt-4o
    
    defaults:
      model: gpt-4o
      temperature: 0.7
    
    costs:
      gpt-3.5:
        input_per_1k: 0.0005
        output_per_1k: 0.0015
      gpt-4o:
        input_per_1k: 0.005
        output_per_1k: 0.015
    
    budgets:
      daily_usd: 2.00
      monthly_usd: 50.00
      hard_limit: false
    
    features:
      sentiment_detection: true
      model_escalation: true
      token_tracking: true
  
  copilot:
    enabled: false
    auth_key_var: GH_TOKEN
    
    budgets:
      daily_requests: 100
      monthly_requests: 1000
      hard_limit: false
    
    features:
      sentiment_detection: false  # Impossible
      model_escalation: false     # No control
      token_tracking: false       # No API access

# Multi-provider orchestration
execution:
  default_provider: claude
  
  # Fallback strategy when primary provider fails
  fallback:
    enabled: true
    order:
      - gemini
      - openai
    max_depth: 2  # Prevent infinite loops
    fallback_on_budget_hit: true
  
  # Cross-provider sentiment detection
  sentiment:
    enabled: true
    provider_priority:  # Try in order
      - claude
      - gemini
      - openai
    escalate_on_frustration: true
  
  # Multi-provider model escalation
  escalation:
    enabled: true
    strategies:
      # Claude escalation (best control)
      claude:
        cheap_to_advanced: ["haiku", "sonnet", "opus"]
        trigger_on_sentiment: true
      
      # Gemini escalation
      gemini:
        cheap_to_advanced: ["flash", "pro"]
        trigger_on_sentiment: true
      
      # OpenAI escalation (limited)
      openai:
        cheap_to_advanced: ["gpt-3.5-turbo", "gpt-4o"]
        trigger_on_sentiment: true
      
      # Cross-provider escalation (fallback between providers)
      cross_provider:
        enabled: true
        order: ["claude", "gemini", "openai"]
        trigger: budget_hit_on_primary
```

---

## Data Flow Example: Cross-Provider Escalation

```
User Query: "Analyze this complex system architecture"
    │
    ├─ Session starts with Claude as primary provider
    │
    ├─ Sentiment Detection: Check if user is frustrated
    │  ├─ Use Claude Haiku (native, cheap)
    │  └─ Frustration Level: 0.2 (low, proceed normally)
    │
    ├─ Model Selection: Claude + budget check
    │  ├─ Default: Claude Sonnet ($0.003 per 1K input)
    │  ├─ Daily budget: $5.00
    │  ├─ Daily spent: $2.50
    │  ├─ Can afford this query: YES
    │  └─ Proceed with Claude Sonnet
    │
    ├─ Execution on Claude
    │  ├─ Tokens: 150 input, 600 output
    │  ├─ Cost: $0.00945
    │  ├─ Response time: 2.3s
    │  └─ Success: YES
    │
    └─ Logging & Analytics
       ├─ Write to .execution-log.jsonl:
       │  {
       │    "provider": "claude",
       │    "model": "claude-3-5-sonnet-20241022",
       │    "tokens_input": 150,
       │    "tokens_output": 600,
       │    "estimated_cost": 0.00945,
       │    "duration_ms": 2300
       │  }
       │
       ├─ Update budget tracker: $2.50 → $2.50945
       │
       └─ Dashboard updates real-time
          ├─ Cost chart updated
          ├─ Provider breakdown: Claude 98%, Gemini 2%
          └─ Trends: Claude trending up, need to consider Gemini fallback

---

Next Query (After Claude daily budget hit):
User Query: "Explain this function"
    │
    ├─ Budget check: Claude daily budget EXCEEDED ($5.00)
    │
    ├─ Fallback triggered: Switch to Gemini
    │  ├─ Gemini budget: $10.00 daily (not hit)
    │  └─ Model: Gemini Flash (super cheap)
    │
    ├─ Sentiment Detection: Use Gemini Flash (same API call)
    │  └─ Reuse sentiment detection for same cost
    │
    ├─ Execution on Gemini
    │  ├─ Tokens: 120 input, 300 output
    │  ├─ Cost: $0.000135 (6.7x cheaper than Sonnet!)
    │  ├─ Response time: 1.5s (faster too!)
    │  └─ Success: YES
    │
    └─ Logging & Analytics
       ├─ Write to .execution-log.jsonl:
       │  {
       │    "provider": "gemini",
       │    "model": "gemini-1.5-flash",
       │    "tokens_input": 120,
       │    "tokens_output": 300,
       │    "estimated_cost": 0.000135,
       │    "duration_ms": 1500,
       │    "fallback_chain": ["claude"],  ← Shows it fell back from Claude
       │    "fallback_depth": 1
       │  }
       │
       └─ Dashboard now shows:
          ├─ Cross-provider comparison activated
          ├─ Cost breakdown: Claude: $5.00, Gemini: $0.0008
          ├─ Speed comparison: Claude avg 2.3s, Gemini avg 1.5s
          └─ Recommendation: "Consider Gemini Flash as primary for simple queries (7x cheaper)"
```

---

## Deliverables Summary

### Phase 1: Foundation (Weeks 1-3)
- [ ] `internal/providers/provider.go` — Abstract interface
- [ ] `internal/providers/claude.go` — Claude implementation
- [ ] `internal/providers/gemini.go` — Gemini implementation
- [ ] `internal/providers/openai.go` — OpenAI implementation
- [ ] `internal/providers/copilot.go` — Copilot implementation (limited)
- [ ] `internal/execution/multi_provider_engine.go` — Unified execution
- [ ] `internal/execlog/multi_provider_logger.go` — Unified logging
- [ ] Updated config schema (YAML)
- [ ] Unit tests for each provider

### Phase 2: Analytics (Weeks 4-6)
- [ ] `internal/analytics/multi_provider_analytics.go` — Cross-provider analytics
- [ ] Dashboard updates: per-provider cost breakdown
- [ ] Sentiment detection per provider
- [ ] Pattern generation: provider-specific + cross-provider

### Phase 3: Escalation (Weeks 7-10)
- [ ] Per-provider model escalation
- [ ] Cross-provider fallback orchestration
- [ ] Cost-aware escalation decisions

### Phase 4: Budget (Weeks 11-13)
- [ ] Per-provider budget enforcement
- [ ] Cross-provider spending alerts
- [ ] Budget analytics & reports

---

## Success Criteria

✅ All 4 providers integrated via unified interface  
✅ Execution logging works on all providers  
✅ Analytics dashboard shows multi-provider comparison  
✅ Sentiment detection works on 3/4 providers (Claude, Gemini, OpenAI)  
✅ Model escalation works on 3/4 providers (Copilot excluded)  
✅ Cross-provider fallback prevents budget overages  
✅ Single config file manages all 4 providers  
✅ No breaking changes to existing Claude-only users  
✅ Logging overhead < 50ms per operation  
