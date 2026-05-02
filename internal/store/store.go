// Package store provides bbolt-backed persistent storage for escalation history,
// turn tracking, and predictive routing data.
package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketEscalations = []byte("escalations")
	bucketTurns       = []byte("turns")
	bucketSessions    = []byte("sessions")
	bucketValidation  = []byte("validation_metrics")
)

// Store manages the bbolt database for escalation data.
type Store struct {
	db *bolt.DB
}

// EscalationEvent represents a model escalation or de-escalation.
type EscalationEvent struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	FromModel string    `json:"from_model"`
	ToModel   string    `json:"to_model"`
	TaskType  string    `json:"task_type"`
	Reason    string    `json:"reason"`
}

// Turn represents a conversation turn for circular reasoning detection.
type Turn struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Concepts  string    `json:"concepts"`
}

// TaskTypeStats holds aggregated statistics for a task type.
type TaskTypeStats struct {
	TaskType    string  `json:"TaskType"`
	Escalations int     `json:"Escalations"`
	Successes   int     `json:"Successes"`
	SuccessRate float64 `json:"SuccessRate"`
}

// ValidationMetric tracks estimated vs actual token usage.
type ValidationMetric struct {
	ID                    int64     `json:"id"`
	Timestamp             time.Time `json:"timestamp"`
	Prompt                string    `json:"prompt"`
	DetectedTaskType      string    `json:"detected_task_type"`
	DetectedEffort        string    `json:"detected_effort"`
	RoutedModel           string    `json:"routed_model"`
	EstimatedInputTokens  int       `json:"estimated_input_tokens"`
	EstimatedOutputTokens int       `json:"estimated_output_tokens"`
	EstimatedTotalTokens  int       `json:"estimated_total_tokens"`
	EstimatedCost         float64   `json:"estimated_cost"`
	ActualInputTokens     int       `json:"actual_input_tokens"`
	ActualOutputTokens    int       `json:"actual_output_tokens"`
	ActualTotalTokens     int       `json:"actual_total_tokens"`
	ActualCost            float64   `json:"actual_cost"`
	TokenError            float64   `json:"token_error_percent"`
	CostError             float64   `json:"cost_error_percent"`
	Validated             bool      `json:"validated"`
}

// Open creates or opens the bbolt database at the given directory.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "escalation.db")
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketEscalations, bucketTurns, bucketSessions, bucketValidation} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating buckets: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// GetDB returns the underlying bolt database for analytics access.
func (s *Store) GetDB() *bolt.DB {
	return s.db
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// LogEscalation records an escalation or de-escalation event.
func (s *Store) LogEscalation(fromModel, toModel, taskType, reason string) error {
	event := EscalationEvent{
		Timestamp: time.Now(),
		FromModel: fromModel,
		ToModel:   toModel,
		TaskType:  taskType,
		Reason:    reason,
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketEscalations)
		id, _ := b.NextSequence()
		if id > math.MaxInt64 {
			return fmt.Errorf("escalation ID exceeds max int64: %d", id)
		}
		event.ID = int64(id)
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return b.Put(itob(id), data)
	})
}

// LogTurn records a conversation turn with extracted concepts.
func (s *Store) LogTurn(model, concepts string) error {
	turn := Turn{
		Timestamp: time.Now(),
		Model:     model,
		Concepts:  concepts,
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTurns)
		id, _ := b.NextSequence()
		data, err := json.Marshal(turn)
		if err != nil {
			return err
		}
		if err := b.Put(itob(id), data); err != nil {
			return err
		}

		// Prune: keep last 100 turns
		count := b.Stats().KeyN
		if count > 100 {
			toDelete := count - 100
			c := b.Cursor()
			for k, _ := c.First(); k != nil && toDelete > 0; k, _ = c.Next() {
				if err := b.Delete(k); err != nil {
					return err
				}
				toDelete--
			}
		}
		return nil
	})
}

// RecentTurns returns the last N turns (newest first).
func (s *Store) RecentTurns(n int) ([]Turn, error) {
	var turns []Turn
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketTurns).Cursor()
		count := 0
		for k, v := c.Last(); k != nil && count < n; k, v = c.Prev() {
			var t Turn
			if err := json.Unmarshal(v, &t); err == nil {
				turns = append(turns, t)
			}
			count++
		}
		return nil
	})
	return turns, err
}

// CountRecentAttempts counts recent turns on a specific model (in the last N turns).
func (s *Store) CountRecentAttempts(model string, last int) (int, error) {
	count := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketTurns).Cursor()
		seen := 0
		for k, v := c.Last(); k != nil && seen < last; k, v = c.Prev() {
			var t Turn
			if err := json.Unmarshal(v, &t); err == nil && t.Model == model {
				count++
			}
			seen++
		}
		return nil
	})
	return count, err
}

// TaskTypeStatsAll returns escalation statistics grouped by task type.
func (s *Store) TaskTypeStatsAll() ([]TaskTypeStats, error) {
	escByType := make(map[string]int)
	succByType := make(map[string]int)

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEscalations).ForEach(func(k, v []byte) error {
			var e EscalationEvent
			if err := json.Unmarshal(v, &e); err != nil {
				return nil // skip malformed
			}
			if e.Reason == "success" {
				succByType[e.TaskType]++
			} else {
				escByType[e.TaskType]++
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	var stats []TaskTypeStats
	for taskType, esc := range escByType {
		st := TaskTypeStats{
			TaskType:    taskType,
			Escalations: esc,
			Successes:   succByType[taskType],
		}
		if esc > 0 {
			st.SuccessRate = float64(st.Successes) / float64(esc) * 100
		}
		stats = append(stats, st)
	}
	return stats, nil
}

// EscalationCountForType returns how many times a task type has been escalated.
func (s *Store) EscalationCountForType(taskType string) (int, error) {
	count := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEscalations).ForEach(func(k, v []byte) error {
			var e EscalationEvent
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			if e.TaskType == taskType && e.Reason != "success" {
				count++
			}
			return nil
		})
	})
	return count, err
}

// TotalStats returns aggregate statistics.
func (s *Store) TotalStats() (escalations int, deescalations int, turns int, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		if ferr := tx.Bucket(bucketEscalations).ForEach(func(k, v []byte) error {
			var e EscalationEvent
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			if e.Reason == "success" {
				deescalations++
			} else {
				escalations++
			}
			return nil
		}); ferr != nil {
			return ferr
		}
		turns = tx.Bucket(bucketTurns).Stats().KeyN
		return nil
	})
	return
}

// RecentEscalations returns the last N escalation events (newest first).
func (s *Store) RecentEscalations(n int) ([]EscalationEvent, error) {
	var events []EscalationEvent
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketEscalations).Cursor()
		count := 0
		for k, v := c.Last(); k != nil && count < n; k, v = c.Prev() {
			var e EscalationEvent
			if err := json.Unmarshal(v, &e); err == nil {
				events = append(events, e)
			}
			count++
		}
		return nil
	})
	return events, err
}

// SetSession stores a key-value pair for session state.
func (s *Store) SetSession(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).Put([]byte(key), []byte(value))
	})
}

// GetSession retrieves a session value. Returns empty string if not found.
func (s *Store) GetSession(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketSessions).Get([]byte(key))
		if v != nil {
			value = string(v)
		}
		return nil
	})
	return value, err
}

// DeleteSession removes a session key.
func (s *Store) DeleteSession(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).Delete([]byte(key))
	})
}

// LogValidationMetric records estimated vs actual token usage for validation.
func (s *Store) LogValidationMetric(metric ValidationMetric) error {
	metric.Timestamp = time.Now()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketValidation)
		id, _ := b.NextSequence()
		if id > math.MaxInt64 {
			return fmt.Errorf("validation ID exceeds max int64: %d", id)
		}
		metric.ID = int64(id)
		data, err := json.Marshal(metric)
		if err != nil {
			return err
		}
		return b.Put(itob(id), data)
	})
}

// GetValidationMetrics retrieves the last N validation metrics.
func (s *Store) GetValidationMetrics(limit int) ([]ValidationMetric, error) {
	var metrics []ValidationMetric
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketValidation)
		c := b.Cursor()
		count := 0
		for k, v := c.Last(); k != nil && count < limit; k, v = c.Prev() {
			var m ValidationMetric
			if err := json.Unmarshal(v, &m); err != nil {
				continue
			}
			metrics = append(metrics, m)
			count++
		}
		return nil
	})
	return metrics, err
}

// GetValidationStats returns summary statistics for validation metrics.
func (s *Store) GetValidationStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"total_metrics":        0,
		"validated":            0,
		"avg_token_error":      0.0,
		"avg_cost_error":       0.0,
		"estimated_total":      0,
		"actual_total":         0,
		"estimated_cost_total": 0.0,
		"actual_cost_total":    0.0,
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketValidation)
		c := b.Cursor()

		var totalMetrics, validated int
		var sumTokenError, sumCostError float64
		var totalEstTokens, totalActTokens int
		var totalEstCost, totalActCost float64

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var m ValidationMetric
			if err := json.Unmarshal(v, &m); err != nil {
				continue
			}
			totalMetrics++
			if m.Validated {
				validated++
				sumTokenError += m.TokenError
				sumCostError += m.CostError
			}
			totalEstTokens += m.EstimatedTotalTokens
			totalActTokens += m.ActualTotalTokens
			totalEstCost += m.EstimatedCost
			totalActCost += m.ActualCost
		}

		if totalMetrics > 0 {
			stats["total_metrics"] = totalMetrics
			stats["validated"] = validated
			if validated > 0 {
				stats["avg_token_error"] = sumTokenError / float64(validated)
				stats["avg_cost_error"] = sumCostError / float64(validated)
			}
			stats["estimated_total"] = totalEstTokens
			stats["actual_total"] = totalActTokens
			stats["estimated_cost_total"] = totalEstCost
			stats["actual_cost_total"] = totalActCost
		}
		return nil
	})
	return stats, err
}

// GetValidationMetric retrieves a single validation metric by ID.
func (s *Store) GetValidationMetric(id string) (ValidationMetric, error) {
	var metric ValidationMetric
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketValidation)
		v := b.Get([]byte(id))
		if v == nil {
			return fmt.Errorf("validation metric not found: %s", id)
		}
		return json.Unmarshal(v, &metric)
	})
	return metric, err
}

// GetAllValidationMetrics retrieves all validation metrics.
func (s *Store) GetAllValidationMetrics() ([]ValidationMetric, error) {
	var metrics []ValidationMetric
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketValidation)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var m ValidationMetric
			if err := json.Unmarshal(v, &m); err != nil {
				continue
			}
			metrics = append(metrics, m)
		}
		return nil
	})
	return metrics, err
}
