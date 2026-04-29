package execlog

import "time"

// Entry represents a single logged operation
type Entry struct {
	Timestamp             time.Time            `json:"timestamp"`
	SessionID             string               `json:"session_id"`
	OperationType         string               `json:"operation_type"`
	OperationID           string               `json:"operation_id"`
	Command               string               `json:"command"`
	CommandNormalized     string               `json:"command_normalized"`
	Status                string               `json:"status"`
	ExitCode              int                  `json:"exit_code"`
	DurationMS            int64                `json:"duration_ms"`
	OutputLines           int                  `json:"output_lines"`
	OutputSummary         string               `json:"output_summary"`
	TokensUsed            int                  `json:"tokens_used"`
	TokensEstimate        int                  `json:"tokens_estimate"`
	DecisionContext       string               `json:"decision_context"`
	CacheKey              string               `json:"cache_key"`
	Cached                bool                 `json:"cached"`
	RepetitionsThisSession int                `json:"repetitions_this_session"`
	Metadata              map[string]string    `json:"metadata"`
}

// OperationStats aggregates statistics for an operation type
type OperationStats struct {
	Operation        string
	AvgDurationMS    int64
	MaxDurationMS    int64
	MinDurationMS    int64
	ExecutionCount   int
	SuccessCount     int
	FailureCount     int
	TotalTimeMS      int64
	CachingPotential string // low, medium, high
}

// SessionMetrics summarizes a session's activity
type SessionMetrics struct {
	SessionID          string
	TotalOperations    int
	TotalDurationMS    int64
	AvgDurationMS      int64
	SuccessRate        float64
	OperationsByType   map[string]int
	EstimatedTokens    int
	CachingOpportunity int // count of repeated operations
}

// PatternData contains analyzed execution patterns
type PatternData struct {
	FastOperations      []OperationStats
	SlowOperations      []OperationStats
	CachingOpportunities []CachingOpportunity
	TokenSavingsTips    []string
	DecisionPatterns    map[string]string
}

// CachingOpportunity describes an operation that could benefit from caching
type CachingOpportunity struct {
	Operation        string
	Repetitions      int
	AvgDurationMS    int64
	TotalTimeMS      int64
	PotentialSavings int64
}
