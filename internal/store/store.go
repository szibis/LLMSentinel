// Package store provides SQLite-backed persistent storage for escalation history,
// turn tracking, savings analytics, and predictive routing data.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages the SQLite database for escalation data.
type Store struct {
	db *sql.DB
}

// EscalationEvent represents a model escalation or de-escalation.
type EscalationEvent struct {
	ID        int64
	Timestamp time.Time
	FromModel string
	ToModel   string
	TaskType  string
	Reason    string // "user_command", "stuck_detected", "pattern_detected", "predictive", "success"
}

// Turn represents a conversation turn for circular reasoning detection.
type Turn struct {
	Timestamp time.Time
	Model     string
	Concepts  string
}

// TaskTypeStats holds aggregated statistics for a task type.
type TaskTypeStats struct {
	TaskType     string
	Escalations  int
	Successes    int
	SuccessRate  float64
	AvgFromTier  float64
	AvgToTier    float64
}

// DailySummary holds daily cost/savings data.
type DailySummary struct {
	Date             string
	Escalations      int
	DeEscalations    int
	TokensSaved      int64
	CostSavedUSD     float64
}

// Open creates or opens the SQLite database at the given directory.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "escalation.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS escalations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			from_model TEXT NOT NULL,
			to_model TEXT NOT NULL,
			task_type TEXT NOT NULL DEFAULT 'general',
			reason TEXT NOT NULL DEFAULT 'user_command'
		);

		CREATE TABLE IF NOT EXISTS turns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			model TEXT NOT NULL,
			concepts TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS sessions (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_escalations_task_type ON escalations(task_type);
		CREATE INDEX IF NOT EXISTS idx_escalations_timestamp ON escalations(timestamp);
		CREATE INDEX IF NOT EXISTS idx_turns_timestamp ON turns(timestamp);
	`)
	return err
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// LogEscalation records an escalation or de-escalation event.
func (s *Store) LogEscalation(fromModel, toModel, taskType, reason string) error {
	_, err := s.db.Exec(
		"INSERT INTO escalations (from_model, to_model, task_type, reason) VALUES (?, ?, ?, ?)",
		fromModel, toModel, taskType, reason,
	)
	return err
}

// LogTurn records a conversation turn with extracted concepts.
func (s *Store) LogTurn(model, concepts string) error {
	_, err := s.db.Exec(
		"INSERT INTO turns (model, concepts) VALUES (?, ?)",
		model, concepts,
	)
	// Prune old turns (keep last 100)
	s.db.Exec("DELETE FROM turns WHERE id NOT IN (SELECT id FROM turns ORDER BY id DESC LIMIT 100)")
	return err
}

// RecentTurns returns the last N turns.
func (s *Store) RecentTurns(n int) ([]Turn, error) {
	rows, err := s.db.Query(
		"SELECT timestamp, model, concepts FROM turns ORDER BY id DESC LIMIT ?", n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []Turn
	for rows.Next() {
		var t Turn
		if err := rows.Scan(&t.Timestamp, &t.Model, &t.Concepts); err != nil {
			return nil, err
		}
		turns = append(turns, t)
	}
	return turns, nil
}

// CountRecentAttempts counts recent turns on a specific model (last N turns).
func (s *Store) CountRecentAttempts(model string, last int) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM (SELECT model FROM turns ORDER BY id DESC LIMIT ?) WHERE model = ?",
		last, model,
	).Scan(&count)
	return count, err
}

// TaskTypeStatsAll returns escalation statistics grouped by task type.
func (s *Store) TaskTypeStatsAll() ([]TaskTypeStats, error) {
	rows, err := s.db.Query(`
		SELECT
			e.task_type,
			COUNT(*) as escalations,
			COALESCE(SUM(CASE WHEN e.reason = 'success' THEN 1 ELSE 0 END), 0) as successes
		FROM escalations e
		WHERE e.reason != 'success'
		GROUP BY e.task_type
		ORDER BY escalations DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TaskTypeStats
	for rows.Next() {
		var st TaskTypeStats
		if err := rows.Scan(&st.TaskType, &st.Escalations, &st.Successes); err != nil {
			return nil, err
		}

		// Count successes separately
		var successes int
		s.db.QueryRow(
			"SELECT COUNT(*) FROM escalations WHERE task_type = ? AND reason = 'success'",
			st.TaskType,
		).Scan(&successes)
		st.Successes = successes

		if st.Escalations > 0 {
			st.SuccessRate = float64(st.Successes) / float64(st.Escalations) * 100
		}
		stats = append(stats, st)
	}
	return stats, nil
}

// EscalationCountForType returns how many times a task type has been escalated.
func (s *Store) EscalationCountForType(taskType string) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM escalations WHERE task_type = ? AND reason != 'success'",
		taskType,
	).Scan(&count)
	return count, err
}

// TotalStats returns aggregate statistics for the dashboard.
func (s *Store) TotalStats() (escalations int, deescalations int, turns int, err error) {
	s.db.QueryRow("SELECT COUNT(*) FROM escalations WHERE reason != 'success'").Scan(&escalations)
	s.db.QueryRow("SELECT COUNT(*) FROM escalations WHERE reason = 'success'").Scan(&deescalations)
	s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&turns)
	return
}

// RecentEscalations returns the last N escalation events.
func (s *Store) RecentEscalations(n int) ([]EscalationEvent, error) {
	rows, err := s.db.Query(
		"SELECT id, timestamp, from_model, to_model, task_type, reason FROM escalations ORDER BY id DESC LIMIT ?", n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EscalationEvent
	for rows.Next() {
		var e EscalationEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.FromModel, &e.ToModel, &e.TaskType, &e.Reason); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// SetSession stores a key-value pair for session state.
func (s *Store) SetSession(key, value string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO sessions (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
		key, value,
	)
	return err
}

// GetSession retrieves a session value. Returns empty string if not found.
func (s *Store) GetSession(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM sessions WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// DeleteSession removes a session key.
func (s *Store) DeleteSession(key string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE key = ?", key)
	return err
}
