package gateway

import (
	"context"
	"fmt"
)

// ToolType represents the type of tool
type ToolType string

const (
	ToolTypeMCP    ToolType = "mcp"
	ToolTypeCLI    ToolType = "cli"
	ToolTypeREST   ToolType = "rest"
	ToolTypeDB     ToolType = "database"
	ToolTypeBinary ToolType = "binary"
)

// ToolAdapter defines the interface for tool implementations
type ToolAdapter interface {
	// Name returns the adapter name
	Name() string

	// Type returns the tool type
	Type() ToolType

	// Execute executes the tool with the given parameters
	Execute(ctx context.Context, params *ToolRequest) (*ToolResponse, error)

	// GetSignature returns the tool signature/metadata
	GetSignature() *ToolSignature

	// Health checks if the tool is healthy
	Health(ctx context.Context) error

	// Close closes any resources used by the adapter
	Close() error
}

// ToolRequest represents a request to execute a tool
type ToolRequest struct {
	ID       string                 `json:"id"`
	Tool     string                 `json:"tool"`
	Method   string                 `json:"method,omitempty"`
	Params   map[string]interface{} `json:"params"`
	Input    string                 `json:"input,omitempty"`
	Metadata map[string]string      `json:"metadata,omitempty"`
	NoCacheBypassed bool              `json:"no_cache,omitempty"`
}

// ToolResponse represents a response from a tool
type ToolResponse struct {
	ID       string                 `json:"id"`
	Success  bool                   `json:"success"`
	Data     interface{}            `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Timing   ResponseTiming         `json:"timing,omitempty"`
}

// ResponseTiming tracks response timing information
type ResponseTiming struct {
	TotalMs       int64  `json:"total_ms"`
	ToolExecMs    int64  `json:"tool_exec_ms"`
	ParsingMs     int64  `json:"parsing_ms"`
	SerializingMs int64  `json:"serializing_ms"`
}

// ToolSignature describes a tool's interface
type ToolSignature struct {
	Name        string                  `json:"name"`
	Type        ToolType                `json:"type"`
	Description string                  `json:"description"`
	Parameters  map[string]*ParamSchema `json:"parameters"`
	Returns     *ParamSchema            `json:"returns,omitempty"`
	Examples    []ToolExample           `json:"examples,omitempty"`
}

// ParamSchema describes a parameter
type ParamSchema struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	MinValue    *float64    `json:"min_value,omitempty"`
	MaxValue    *float64    `json:"max_value,omitempty"`
}

// ToolExample shows an example of using a tool
type ToolExample struct {
	Name    string                 `json:"name"`
	Input   map[string]interface{} `json:"input"`
	Output  interface{}            `json:"output"`
	Error   string                 `json:"error,omitempty"`
}

// GatewayRequest represents a request to the gateway
type GatewayRequest struct {
	Tool              string
	Method            string
	Params            map[string]interface{}
	Input             string
	Metadata          map[string]string
	CacheBypassForced bool
}

// GatewayResponse represents a response from the gateway
type GatewayResponse struct {
	Data              interface{}
	Error             string
	TokensSaved       int64
	TotalTokens       int64
	CachedResponse    bool
	OptimizationsApplied []string
	Quality           ResponseQuality
	Transparency      TransparencyInfo
}

// ResponseQuality tracks response quality metrics
type ResponseQuality struct {
	Fresh               bool
	CacheConfidence     float64
	AccuracyEstimate    float64
	RecommendedFreshness bool
}

// TransparencyInfo provides transparency about optimizations
type TransparencyInfo struct {
	Optimizations       []string  `json:"optimizations"`
	CachedResponse      bool      `json:"cached_response"`
	TokensSaved         int64     `json:"tokens_saved"`
	TotalTokens         int64     `json:"total_tokens"`
	SavingsPercent      float64   `json:"savings_percent"`
	CacheHitConfidence  float64   `json:"cache_confidence,omitempty"`
	CostWithout         string    `json:"cost_without_optimization"`
	CostWith            string    `json:"cost_with_optimization"`
	Message             string    `json:"message"`
}

// ExecutionError represents an error during tool execution
type ExecutionError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

// Error implements the error interface
func (e *ExecutionError) Error() string {
	return fmt.Sprintf("ExecutionError [%s]: %s", e.Code, e.Message)
}

// NewExecutionError creates a new execution error
func NewExecutionError(code, message string) *ExecutionError {
	return &ExecutionError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WithDetails adds details to the error
func (e *ExecutionError) WithDetails(key string, value interface{}) *ExecutionError {
	e.Details[key] = value
	return e
}
