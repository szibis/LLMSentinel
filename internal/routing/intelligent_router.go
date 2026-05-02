package routing

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/szibis/claude-escalate/internal/models"
)

// TaskType represents the detected type of work
type TaskType string

const (
	TaskTypeSimple       TaskType = "simple"       // Simple questions, lookups
	TaskTypeAnalysis     TaskType = "analysis"     // Code analysis, data analysis
	TaskTypeReasoning    TaskType = "reasoning"    // Complex logic, problem solving
	TaskTypePlanning     TaskType = "planning"     // Architecture, planning
	TaskTypeCreative     TaskType = "creative"     // Content creation, writing
	TaskTypeSecurity     TaskType = "security"     // Sensitive data, security
	TaskTypeCodeGen      TaskType = "codegen"      // Code generation
	TaskTypeDebug        TaskType = "debug"        // Debugging, troubleshooting
)

// DataSensitivity indicates how sensitive the content is
type DataSensitivity string

const (
	SensitivityPublic    DataSensitivity = "public"    // Public data
	SensitivityInternal  DataSensitivity = "internal"  // Internal company data
	SensitivityConfidential DataSensitivity = "confidential" // Confidential/private data
	SensitivitySecrets   DataSensitivity = "secrets"   // Secrets, credentials, tokens
)

// TaskDetector analyzes text to detect task type and characteristics
type TaskDetector struct {
	contentAnalyzer *ContentAnalyzer
}

// ContentAnalyzer provides text analysis utilities
type ContentAnalyzer struct {
	keywordPatterns map[TaskType][]string
	sensitivePatterns []string
}

// RoutingRule defines custom routing rules
type RoutingRule struct {
	Name        string
	Condition   func(TaskType, DataSensitivity, int) bool // taskType, sensitivity, complexity
	PreferredModel string
	Fallback    []string // fallback models if preferred unavailable
	TokenSavings bool    // apply token optimization for this rule
}

// IntelligentRouter makes smart decisions about model selection
type IntelligentRouter struct {
	registry *models.ModelRegistry
	detector *TaskDetector
	rules    []RoutingRule
	defaults RoutingDefaults
}

// RoutingDefaults sets default behaviors
type RoutingDefaults struct {
	PreferLocal      bool              // prefer local models for cost savings
	PreferMock       bool              // prefer mock models for testing
	SensitiveOnly    DataSensitivity   // only use local/private for sensitive data
	TokenOptimization bool            // enable automatic token optimization
	FallbackStrategy string            // "cost", "speed", "capability"
}

// NewTaskDetector creates a new task detector
func NewTaskDetector() *TaskDetector {
	analyzer := &ContentAnalyzer{
		keywordPatterns: map[TaskType][]string{
			TaskTypeSimple: {
				"what is", "what's", "how do i", "why", "explain", "define",
				"tell me", "list", "count", "is there", "are there",
			},
			TaskTypeAnalysis: {
				"analyze", "analyze", "review", "inspect", "examine",
				"parse", "check", "verify", "validate", "compare",
			},
			TaskTypeReasoning: {
				"reason about", "think through", "solve", "debug", "trace",
				"understand", "complex", "edge case", "error handling",
			},
			TaskTypePlanning: {
				"design", "architect", "plan", "structure", "strategy",
				"approach", "refactor", "optimize", "scale",
			},
			TaskTypeCreative: {
				"write", "create", "draft", "compose", "generate story",
				"describe", "imagine", "brainstorm",
			},
			TaskTypeSecurity: {
				"security", "authenticate", "encrypt", "secure", "vulnerability",
				"attack", "threat", "exploit", "token", "credential",
			},
			TaskTypeCodeGen: {
				"generate code", "write function", "implement", "create class",
				"add method", "complete this", "fill in", "stub out",
			},
			TaskTypeDebug: {
				"debug", "error", "fix", "broken", "crash", "hang",
				"issue", "bug", "not working", "unexpected",
			},
		},
		sensitivePatterns: []string{
			"password", "secret", "token", "key", "credential",
			"private", "confidential", "api_key", "apikey",
			"ssn", "credit card", "medical", "health",
		},
	}

	return &TaskDetector{contentAnalyzer: analyzer}
}

// DetectTask analyzes text and returns detected task type and sensitivity
func (d *TaskDetector) DetectTask(text string) (TaskType, DataSensitivity, int) {
	lower := strings.ToLower(text)

	// Check for sensitive content
	sensitivity := SensitivityPublic
	for _, pattern := range d.contentAnalyzer.sensitivePatterns {
		if strings.Contains(lower, pattern) {
			if strings.Contains(pattern, "password") || strings.Contains(pattern, "token") || strings.Contains(pattern, "key") {
				sensitivity = SensitivitySecrets
			} else {
				sensitivity = SensitivityConfidential
			}
			break
		}
	}

	// Detect task type by keyword matching
	taskType := TaskTypeSimple
	maxMatches := 0

	for tType, keywords := range d.contentAnalyzer.keywordPatterns {
		matches := 0
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				matches++
			}
		}
		if matches > maxMatches {
			maxMatches = matches
			taskType = tType
		}
	}

	// Calculate complexity based on text length and structure
	complexity := calculateComplexity(text)

	return taskType, sensitivity, complexity
}

// calculateComplexity estimates task complexity (1-10)
func calculateComplexity(text string) int {
	complexity := 1

	// Length-based complexity
	words := len(strings.Fields(text))
	if words > 100 {
		complexity += 3
	} else if words > 50 {
		complexity += 2
	} else if words > 20 {
		complexity += 1
	}

	// Check for special characters indicating code
	codeIndicators := 0
	for _, char := range text {
		if strings.ContainsRune("{}[]();</>=", char) {
			codeIndicators++
		}
	}
	if codeIndicators > 5 {
		complexity += 2
	}

	// Check for structured data (JSON, XML, etc.)
	if strings.Contains(text, "{") && strings.Contains(text, "}") {
		complexity += 1
	}

	// Check for numbers/math
	hasNumbers := false
	for _, char := range text {
		if unicode.IsDigit(char) {
			hasNumbers = true
			break
		}
	}
	if hasNumbers {
		complexity += 1
	}

	if complexity > 10 {
		complexity = 10
	}
	return complexity
}

// NewIntelligentRouter creates a new intelligent router
func NewIntelligentRouter(registry *models.ModelRegistry, defaults RoutingDefaults) *IntelligentRouter {
	detector := NewTaskDetector()

	router := &IntelligentRouter{
		registry: registry,
		detector: detector,
		rules:    []RoutingRule{},
		defaults: defaults,
	}

	// Register default rules
	router.registerDefaultRules()

	return router
}

// registerDefaultRules sets up sensible default routing rules
func (r *IntelligentRouter) registerDefaultRules() {
	// Rule 1: Secrets always go to local/offline if available
	r.AddRule(RoutingRule{
		Name: "secrets-local-only",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return ds == SensitivitySecrets
		},
		PreferredModel: "local-llm",
		Fallback:      []string{"mock-model"},
		TokenSavings:  false,
	})

	// Rule 2: Simple tasks use cheap models
	r.AddRule(RoutingRule{
		Name: "simple-cheap",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return c <= 3 && tt == TaskTypeSimple
		},
		PreferredModel: "claude-haiku",
		Fallback:      []string{"mock-model", "local-llm"},
		TokenSavings:  true,
	})

	// Rule 3: Code generation uses capable models
	r.AddRule(RoutingRule{
		Name: "codegen-capable",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return tt == TaskTypeCodeGen && c >= 5
		},
		PreferredModel: "claude-opus",
		Fallback:      []string{"claude-sonnet"},
		TokenSavings:  true,
	})

	// Rule 4: Reasoning needs capable models
	r.AddRule(RoutingRule{
		Name: "reasoning-capable",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return tt == TaskTypeReasoning && c >= 6
		},
		PreferredModel: "claude-opus",
		Fallback:      []string{"claude-sonnet"},
		TokenSavings:  true,
	})

	// Rule 5: Analysis can use mid-tier models
	r.AddRule(RoutingRule{
		Name: "analysis-mid-tier",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return tt == TaskTypeAnalysis && c >= 4
		},
		PreferredModel: "claude-sonnet",
		Fallback:      []string{"claude-haiku"},
		TokenSavings:  true,
	})

	// Rule 6: Debugging needs capability
	r.AddRule(RoutingRule{
		Name: "debug-capable",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return tt == TaskTypeDebug && c >= 5
		},
		PreferredModel: "claude-sonnet",
		Fallback:      []string{"claude-opus"},
		TokenSavings:  true,
	})

	// Rule 7: Creative work uses mid-tier
	r.AddRule(RoutingRule{
		Name: "creative-mid",
		Condition: func(tt TaskType, ds DataSensitivity, c int) bool {
			return tt == TaskTypeCreative
		},
		PreferredModel: "claude-sonnet",
		Fallback:      []string{"claude-opus"},
		TokenSavings:  true,
	})
}

// AddRule adds a custom routing rule
func (r *IntelligentRouter) AddRule(rule RoutingRule) {
	r.rules = append(r.rules, rule)
}

// RouteRequest selects the best model for a request
func (r *IntelligentRouter) RouteRequest(text string) (string, TaskType, DataSensitivity, error) {
	// Detect task characteristics
	taskType, sensitivity, complexity := r.detector.DetectTask(text)

	// Check custom rules
	for _, rule := range r.rules {
		if rule.Condition(taskType, sensitivity, complexity) {
			model := rule.PreferredModel
			modelInfo, exists := r.registry.GetModel(model)
			if exists && modelInfo.Enabled {
				return model, taskType, sensitivity, nil
			}

			// Try fallbacks
			for _, fallback := range rule.Fallback {
				fbModel, exists := r.registry.GetModel(fallback)
				if exists && fbModel.Enabled {
					return fallback, taskType, sensitivity, nil
				}
			}
		}
	}

	// Fallback: use cost-optimized selection
	cheapest := r.registry.FindCheapest(100, 100) // rough estimates
	if cheapest != nil {
		return cheapest.ID, taskType, sensitivity, nil
	}

	return "", taskType, sensitivity, fmt.Errorf("no model available")
}

// GetTokenOptimizationStrategy returns optimization hints based on task type
func (r *IntelligentRouter) GetTokenOptimizationStrategy(taskType TaskType) TokenOptimizationStrategy {
	strategy := TokenOptimizationStrategy{
		Aggressive: false,
		MaxTokens: 4096,
	}

	switch taskType {
	case TaskTypeSimple:
		strategy.Aggressive = true
		strategy.MaxTokens = 1024
		strategy.RemoveExamples = true
		strategy.Summarize = true

	case TaskTypeCodeGen:
		strategy.Aggressive = false
		strategy.MaxTokens = 8192
		strategy.KeepContext = true

	case TaskTypeReasoning:
		strategy.Aggressive = false
		strategy.MaxTokens = 8192
		strategy.KeepContext = true

	case TaskTypeSecurity:
		strategy.Aggressive = false
		strategy.MaxTokens = 2048
		strategy.KeepContext = true
		strategy.AddSecurityContext = true

	default:
		strategy.Aggressive = true
		strategy.MaxTokens = 4096
	}

	return strategy
}

// TokenOptimizationStrategy defines how to optimize tokens for a task
type TokenOptimizationStrategy struct {
	Aggressive         bool // aggressive token reduction
	MaxTokens          int
	RemoveExamples     bool // remove examples from prompts
	Summarize          bool // summarize context
	KeepContext        bool // preserve full context
	AddSecurityContext bool // add security reminders
}

// GetModelCharacteristics returns model-specific optimization hints
func (r *IntelligentRouter) GetModelCharacteristics(modelID string) ModelCharacteristics {
	model, exists := r.registry.GetModel(modelID)
	if !exists {
		return ModelCharacteristics{}
	}

	chars := ModelCharacteristics{
		ID:              model.ID,
		MaxTokens:       model.MaxTokens,
		ContextWindow:   model.ContextWindow,
		CostPerMillionInput: model.CostPer1KInput * 1000,
		CostPerMillionOutput: model.CostPer1KOutput * 1000,
		IsLocal:         model.Provider == "local",
		IsMock:          model.Provider == "mock",
	}

	return chars
}

// ModelCharacteristics describes a model's capabilities
type ModelCharacteristics struct {
	ID                    string
	MaxTokens             int
	ContextWindow         int
	CostPerMillionInput   float64
	CostPerMillionOutput  float64
	IsLocal               bool
	IsMock                bool
}
