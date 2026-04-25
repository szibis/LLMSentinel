package analytics

import (
	"time"
)

// AnalyticsRecord represents complete analytics for one validation.
type AnalyticsRecord struct {
	ValidationID string
	Timestamp    time.Time

	// Phase 1: Estimation
	Phase1 Phase1Data

	// Phase 2: Real-time
	Phase2 Phase2Data

	// Phase 3: Validation & Learning
	Phase3 Phase3Data
}

// Phase1Data captures pre-response estimation.
type Phase1Data struct {
	Prompt            string
	TaskType          string
	DetectedEffort    string // low, medium, high
	Complexity        float64
	SentimentBaseline string

	EstimatedInputTokens  int
	EstimatedOutputTokens int
	EstimatedTotalTokens  int
	EstimatedCostUSD      float64

	RoutedModel   string
	RoutingReason string
	Confidence    float64

	BudgetCheck struct {
		WithinBudget bool
		DailyUsed    float64
		DailyLimit   float64
		Warning      string
	}
}

// Phase2Data captures real-time metrics during generation.
type Phase2Data struct {
	InputTokensUsed    int
	OutputTokensSoFar  int
	TotalSoFar         int
	EstimatedRemaining int
	Trend              string // ON_TRACK, TRENDING_OVER, TRENDING_UNDER

	SentimentDuring struct {
		UserPausing      bool
		EditActivity     string
		FrustrationRisk  float64
		CurrentSentiment string
	}

	BudgetStatus struct {
		DailyUsedSoFar float64
		DailyRemaining float64
		OnTrack        bool
		Warning        string
	}
}

// Phase3Data captures post-response validation and learning.
type Phase3Data struct {
	ActualInputTokens         int
	ActualOutputTokens        int
	ActualCacheHitTokens      int
	ActualCacheCreationTokens int
	ActualTotalTokens         int
	ActualCostUSD             float64

	Accuracy struct {
		EstimatedTotal int
		ActualTotal    int
		ErrorPercent   float64
		ErrorMessage   string // EXCELLENT, GOOD, OK, POOR, TERRIBLE
	}

	UserSentiment struct {
		ExplicitSignal      string // success, failure, none
		ExplicitText        string
		SignalConfidence    float64
		ImplicitSentiment   string
		FrustrationDetected bool
		TimeToSignal        time.Duration
	}

	BudgetImpact struct {
		DailyUsedTotal    float64
		DailyRemaining    float64
		SessionUsed       int
		SessionRemaining  int
		CostUnderEstimate float64
	}

	DecisionMade struct {
		Action      string // escalate, de-escalate, continue
		NextModel   string
		Rationale   string
		Confidence  float64
		SavingsNext float64
	}

	Learning struct {
		TaskType              string
		InitialModel          string
		UserSentimentFinal    string
		TokensUsed            int
		Success               bool
		DurationSeconds       float64
		ModelSatisfactionRate float64
	}
}

// SentimentTrend tracks sentiment patterns over time.
type SentimentTrend struct {
	Period    string // "24h", "7d", "30d"
	Timestamp time.Time
	Summary   SentimentSummary
	Events    []FrustrationEvent
	Timeline  []SentimentTimeslot
}

// SentimentSummary aggregates sentiment counts.
type SentimentSummary struct {
	Satisfied        int
	Neutral          int
	Frustrated       int
	Confused         int
	Impatient        int
	Total            int
	SatisfactionRate float64
}

// FrustrationEvent records when frustration was detected.
type FrustrationEvent struct {
	Timestamp      time.Time
	Sentiment      string
	TaskType       string
	InitialModel   string
	EscalatedTo    string
	Resolved       bool
	ResolutionTime time.Duration
}

// SentimentTimeslot is one time bucket in a timeline.
type SentimentTimeslot struct {
	Hour       int
	Satisfied  int
	Neutral    int
	Frustrated int
	Confused   int
	Impatient  int
}

// BudgetStatus tracks spending against limits.
type BudgetStatus struct {
	Timestamp time.Time
	Period    string // "daily", "monthly"

	DailyBudget struct {
		Limit      float64
		Used       float64
		Remaining  float64
		Percentage float64
		Projected  float64
		Warning    string
	}

	MonthlyBudget struct {
		Limit      float64
		Used       float64
		Remaining  float64
		Percentage float64
		DaysLeft   int
		Projected  float64
	}

	ModelUsage map[string]struct {
		Limit      float64
		Used       float64
		Percentage float64
	}
}

// ModelSatisfaction tracks success rates by (task_type, model).
type ModelSatisfaction struct {
	TaskType         string
	Model            string
	SatisfactionRate float64
	SampleCount      int
	SuccessCount     int
}

// CostOptimization suggests ways to reduce spending.
type CostOptimization struct {
	TaskType                string
	CurrentModel            string
	CurrentSatisfaction     float64
	RecommendedModel        string
	RecommendedSatisfaction float64
	EstimatedSavings        float64
	SavingsPercent          float64
}
