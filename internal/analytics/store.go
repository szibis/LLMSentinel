package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store handles analytics persistence.
type Store struct {
	db *sql.DB
}

// NewStore creates an analytics store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveRecord persists a complete analytics record within a transaction.
// All operations succeed or all are rolled back (atomicity guaranteed).
func (s *Store) SaveRecord(record AnalyticsRecord) error {
	// Serialize Phase data as JSON
	phase1JSON, err := json.Marshal(record.Phase1)
	if err != nil {
		return fmt.Errorf("failed to marshal phase1 data: %w", err)
	}

	phase2JSON, err := json.Marshal(record.Phase2)
	if err != nil {
		return fmt.Errorf("failed to marshal phase2 data: %w", err)
	}

	phase3JSON, err := json.Marshal(record.Phase3)
	if err != nil {
		return fmt.Errorf("failed to marshal phase3 data: %w", err)
	}

	// Begin transaction for atomic save
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Defer rollback in case of any error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Save primary analytics record
	query := `
		INSERT INTO analytics_records (
			validation_id, timestamp,
			phase1_data, phase2_data, phase3_data
		) VALUES (?, ?, ?, ?, ?)
	`

	_, err = tx.Exec(query,
		record.ValidationID,
		record.Timestamp,
		string(phase1JSON),
		string(phase2JSON),
		string(phase3JSON),
	)

	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save analytics record: %w", err)
	}

	// Store sentiment outcome for learning
	if err := s.storeSentimentOutcomeWithTx(tx, record); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to store sentiment outcome: %w", err)
	}

	// Store budget impact
	if err := s.storeBudgetImpactWithTx(tx, record); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to store budget impact: %w", err)
	}

	// Store frustration event if applicable
	if record.Phase3.UserSentiment.FrustrationDetected {
		if err := s.storeFrustrationEventWithTx(tx, record); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to store frustration event: %w", err)
		}
	}

	// Commit all changes atomically
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRecord retrieves a complete analytics record.
func (s *Store) GetRecord(validationID string) (AnalyticsRecord, error) {
	query := `
		SELECT validation_id, timestamp, phase1_data, phase2_data, phase3_data
		FROM analytics_records
		WHERE validation_id = ?
	`

	var record AnalyticsRecord
	var phase1JSON, phase2JSON, phase3JSON string

	err := s.db.QueryRow(query, validationID).Scan(
		&record.ValidationID,
		&record.Timestamp,
		&phase1JSON,
		&phase2JSON,
		&phase3JSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return record, fmt.Errorf("record not found")
		}
		return record, fmt.Errorf("failed to get record: %w", err)
	}

	// Deserialize JSON
	if err := json.Unmarshal([]byte(phase1JSON), &record.Phase1); err != nil {
		return record, fmt.Errorf("failed to unmarshal phase1 data: %w", err)
	}

	if err := json.Unmarshal([]byte(phase2JSON), &record.Phase2); err != nil {
		return record, fmt.Errorf("failed to unmarshal phase2 data: %w", err)
	}

	if err := json.Unmarshal([]byte(phase3JSON), &record.Phase3); err != nil {
		return record, fmt.Errorf("failed to unmarshal phase3 data: %w", err)
	}

	return record, nil
}

// GetSentimentTrend retrieves sentiment patterns over time.
func (s *Store) GetSentimentTrend(hours int) (SentimentTrend, error) {
	trend := SentimentTrend{
		Timestamp: time.Now(),
		Period:    fmt.Sprintf("%dh", hours),
	}

	// Count sentiments in period
	query := `
		SELECT
			sentiment_type,
			COUNT(*) as count
		FROM sentiment_outcomes
		WHERE timestamp > datetime('now', '-' || ? || ' hours')
		GROUP BY sentiment_type
	`

	rows, err := s.db.Query(query, hours)
	if err != nil {
		return trend, err
	}
	defer rows.Close()

	totalCount := 0
	satisfiedCount := 0

	for rows.Next() {
		var sentimentType string
		var count int
		if err := rows.Scan(&sentimentType, &count); err != nil {
			continue
		}

		totalCount += count
		switch sentimentType {
		case "satisfied":
			trend.Summary.Satisfied = count
			satisfiedCount = count
		case "neutral":
			trend.Summary.Neutral = count
		case "frustrated":
			trend.Summary.Frustrated = count
		case "confused":
			trend.Summary.Confused = count
		case "impatient":
			trend.Summary.Impatient = count
		}
	}

	trend.Summary.Total = totalCount
	if totalCount > 0 {
		trend.Summary.SatisfactionRate = float64(satisfiedCount) / float64(totalCount)
	}

	// Fetch frustration events
	trend.Events = s.getFrustrationEvents(hours)

	// Fetch timeline
	trend.Timeline = s.getSentimentTimeline(hours)

	return trend, nil
}

// GetBudgetStatus returns current budget usage.
func (s *Store) GetBudgetStatus() (BudgetStatus, error) {
	status := BudgetStatus{
		Timestamp: time.Now(),
		ModelUsage: make(map[string]struct {
			Limit      float64
			Used       float64
			Percentage float64
		}),
	}

	// Query daily spending
	query := `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_history
		WHERE DATE(timestamp) = DATE('now')
	`

	var dailyUsed float64
	s.db.QueryRow(query).Scan(&dailyUsed)
	status.DailyBudget.Used = dailyUsed

	// Query monthly spending
	query = `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_history
		WHERE strftime('%Y-%m', timestamp) = strftime('%Y-%m', 'now')
	`

	var monthlyUsed float64
	s.db.QueryRow(query).Scan(&monthlyUsed)
	status.MonthlyBudget.Used = monthlyUsed

	return status, nil
}

// GetModelSatisfaction returns success rates by (task_type, model).
func (s *Store) GetModelSatisfaction(taskType string) ([]ModelSatisfaction, error) {
	var satisfactions []ModelSatisfaction

	query := `
		SELECT
			task_type,
			model,
			CAST(SUM(CASE WHEN success THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as satisfaction_rate,
			COUNT(*) as sample_count,
			SUM(CASE WHEN success THEN 1 ELSE 0 END) as success_count
		FROM sentiment_outcomes
		WHERE task_type = ?
		GROUP BY task_type, model
		ORDER BY satisfaction_rate DESC
	`

	rows, err := s.db.Query(query, taskType)
	if err != nil {
		return satisfactions, err
	}
	defer rows.Close()

	for rows.Next() {
		var s ModelSatisfaction
		if err := rows.Scan(
			&s.TaskType,
			&s.Model,
			&s.SatisfactionRate,
			&s.SampleCount,
			&s.SuccessCount,
		); err != nil {
			continue
		}
		satisfactions = append(satisfactions, s)
	}

	return satisfactions, nil
}

// Helper functions

func (s *Store) storeSentimentOutcome(record AnalyticsRecord) error {
	query := `
		INSERT INTO sentiment_outcomes (
			validation_id, task_type, model, sentiment,
			success, tokens, duration, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		record.ValidationID,
		record.Phase1.TaskType,
		record.Phase1.RoutedModel,
		record.Phase3.UserSentiment.ImplicitSentiment,
		record.Phase3.Learning.Success,
		record.Phase3.ActualTotalTokens,
		record.Phase3.Learning.DurationSeconds,
		record.Timestamp,
	)

	return err
}

func (s *Store) storeBudgetImpact(record AnalyticsRecord) error {
	query := `
		INSERT INTO budget_history (
			model, tokens, cost_usd, timestamp
		) VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		record.Phase1.RoutedModel,
		record.Phase3.ActualTotalTokens,
		record.Phase3.ActualCostUSD,
		record.Timestamp,
	)

	return err
}

func (s *Store) storeFrustrationEvent(record AnalyticsRecord) error {
	query := `
		INSERT INTO frustration_events (
			validation_id, timestamp, sentiment, task_type,
			initial_model, escalated_to, resolved, resolution_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	escalatedTo := ""
	if record.Phase3.DecisionMade.Action == "escalate" {
		escalatedTo = record.Phase3.DecisionMade.NextModel
	}

	_, err := s.db.Exec(query,
		record.ValidationID,
		record.Timestamp,
		record.Phase3.UserSentiment.ImplicitSentiment,
		record.Phase1.TaskType,
		record.Phase1.RoutedModel,
		escalatedTo,
		record.Phase3.Learning.Success,
		record.Phase3.UserSentiment.TimeToSignal.Seconds(),
	)

	return err
}

// Transaction-aware versions for atomic SaveRecord

func (s *Store) storeSentimentOutcomeWithTx(tx *sql.Tx, record AnalyticsRecord) error {
	query := `
		INSERT INTO sentiment_outcomes (
			validation_id, task_type, model, sentiment,
			success, tokens, duration, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := tx.Exec(query,
		record.ValidationID,
		record.Phase1.TaskType,
		record.Phase1.RoutedModel,
		record.Phase3.UserSentiment.ImplicitSentiment,
		record.Phase3.Learning.Success,
		record.Phase3.ActualTotalTokens,
		record.Phase3.Learning.DurationSeconds,
		record.Timestamp,
	)

	return err
}

func (s *Store) storeBudgetImpactWithTx(tx *sql.Tx, record AnalyticsRecord) error {
	query := `
		INSERT INTO budget_history (
			model, tokens, cost_usd, timestamp
		) VALUES (?, ?, ?, ?)
	`

	_, err := tx.Exec(query,
		record.Phase1.RoutedModel,
		record.Phase3.ActualTotalTokens,
		record.Phase3.ActualCostUSD,
		record.Timestamp,
	)

	return err
}

func (s *Store) storeFrustrationEventWithTx(tx *sql.Tx, record AnalyticsRecord) error {
	query := `
		INSERT INTO frustration_events (
			validation_id, timestamp, sentiment, task_type,
			initial_model, escalated_to, resolved, resolution_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	escalatedTo := ""
	if record.Phase3.DecisionMade.Action == "escalate" {
		escalatedTo = record.Phase3.DecisionMade.NextModel
	}

	_, err := tx.Exec(query,
		record.ValidationID,
		record.Timestamp,
		record.Phase3.UserSentiment.ImplicitSentiment,
		record.Phase1.TaskType,
		record.Phase1.RoutedModel,
		escalatedTo,
		record.Phase3.Learning.Success,
		record.Phase3.UserSentiment.TimeToSignal.Seconds(),
	)

	return err
}

func (s *Store) getFrustrationEvents(hours int) []FrustrationEvent {
	var events []FrustrationEvent

	query := `
		SELECT timestamp, sentiment, task_type, initial_model, escalated_to, resolved, resolution_time
		FROM frustration_events
		WHERE timestamp > datetime('now', '-' || ? || ' hours')
		ORDER BY timestamp DESC
	`

	rows, err := s.db.Query(query, hours)
	if err != nil {
		return events
	}
	defer rows.Close()

	for rows.Next() {
		var e FrustrationEvent
		var resolutionTimeSecs float64
		if err := rows.Scan(
			&e.Timestamp,
			&e.Sentiment,
			&e.TaskType,
			&e.InitialModel,
			&e.EscalatedTo,
			&e.Resolved,
			&resolutionTimeSecs,
		); err != nil {
			continue
		}
		e.ResolutionTime = time.Duration(resolutionTimeSecs) * time.Second
		events = append(events, e)
	}

	return events
}

func (s *Store) getSentimentTimeline(hours int) []SentimentTimeslot {
	var timeline []SentimentTimeslot

	query := `
		SELECT
			strftime('%H', timestamp) as hour,
			sentiment_type,
			COUNT(*) as count
		FROM sentiment_outcomes
		WHERE timestamp > datetime('now', '-' || ? || ' hours')
		GROUP BY hour, sentiment_type
		ORDER BY hour
	`

	rows, err := s.db.Query(query, hours)
	if err != nil {
		return timeline
	}
	defer rows.Close()

	slotMap := make(map[int]*SentimentTimeslot)

	for rows.Next() {
		var hour string
		var sentimentType string
		var count int

		if err := rows.Scan(&hour, &sentimentType, &count); err != nil {
			continue
		}

		// Parse hour
		var hourInt int
		fmt.Sscanf(hour, "%d", &hourInt)

		if _, exists := slotMap[hourInt]; !exists {
			slotMap[hourInt] = &SentimentTimeslot{Hour: hourInt}
		}

		switch sentimentType {
		case "satisfied":
			slotMap[hourInt].Satisfied = count
		case "neutral":
			slotMap[hourInt].Neutral = count
		case "frustrated":
			slotMap[hourInt].Frustrated = count
		case "confused":
			slotMap[hourInt].Confused = count
		case "impatient":
			slotMap[hourInt].Impatient = count
		}
	}

	// Convert map to ordered slice
	for h := 0; h < 24; h++ {
		if slot, exists := slotMap[h]; exists {
			timeline = append(timeline, *slot)
		}
	}

	return timeline
}
