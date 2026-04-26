package analytics

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newTestDB returns an in-memory SQLite DB with the schema required by
// SaveRecord. Each test gets its own DB to avoid cross-contamination.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := []string{
		`CREATE TABLE analytics_records (
			validation_id TEXT PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			phase1_data TEXT,
			phase2_data TEXT,
			phase3_data TEXT
		)`,
		`CREATE TABLE sentiment_outcomes (
			validation_id TEXT,
			task_type TEXT,
			model TEXT,
			sentiment TEXT,
			sentiment_type TEXT,
			success INTEGER,
			tokens INTEGER,
			duration REAL,
			timestamp DATETIME
		)`,
		`CREATE TABLE budget_history (
			model TEXT,
			tokens INTEGER,
			cost_usd REAL,
			timestamp DATETIME
		)`,
		`CREATE TABLE frustration_events (
			validation_id TEXT,
			timestamp DATETIME,
			sentiment TEXT,
			task_type TEXT,
			initial_model TEXT,
			escalated_to TEXT,
			resolved INTEGER,
			resolution_time REAL
		)`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v\n%s", err, s)
		}
	}
	return db
}

// validRecord builds a baseline AnalyticsRecord for tests.
func validRecord() AnalyticsRecord {
	r := AnalyticsRecord{
		ValidationID: "v-1",
		Timestamp:    time.Now(),
	}
	r.Phase1.TaskType = "code"
	r.Phase1.RoutedModel = "sonnet"
	r.Phase3.UserSentiment.ImplicitSentiment = "satisfied"
	r.Phase3.UserSentiment.TimeToSignal = 2 * time.Second
	r.Phase3.Learning.Success = true
	r.Phase3.ActualTotalTokens = 1234
	r.Phase3.ActualCostUSD = 0.12
	return r
}

// TestSaveRecord_AtomicSuccess verifies a normal SaveRecord persists rows
// to all three child tables.
func TestSaveRecord_AtomicSuccess(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	rec := validRecord()
	if err := store.SaveRecord(rec); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM analytics_records").Scan(&count); err != nil {
		t.Fatalf("count analytics_records: %v", err)
	}
	if count != 1 {
		t.Errorf("analytics_records count = %d, want 1", count)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sentiment_outcomes").Scan(&count); err != nil {
		t.Fatalf("count sentiment_outcomes: %v", err)
	}
	if count != 1 {
		t.Errorf("sentiment_outcomes count = %d, want 1", count)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM budget_history").Scan(&count); err != nil {
		t.Fatalf("count budget_history: %v", err)
	}
	if count != 1 {
		t.Errorf("budget_history count = %d, want 1", count)
	}
}

// TestSaveRecord_FrustrationEventConditional verifies frustration row is
// only written when FrustrationDetected is true.
func TestSaveRecord_FrustrationEventConditional(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	rec := validRecord()
	rec.Phase3.UserSentiment.FrustrationDetected = false
	if err := store.SaveRecord(rec); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM frustration_events").Scan(&count)
	if count != 0 {
		t.Errorf("frustration_events count = %d when not detected, want 0", count)
	}

	rec2 := validRecord()
	rec2.ValidationID = "v-2"
	rec2.Phase3.UserSentiment.FrustrationDetected = true
	if err := store.SaveRecord(rec2); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}
	_ = db.QueryRow("SELECT COUNT(*) FROM frustration_events").Scan(&count)
	if count != 1 {
		t.Errorf("frustration_events count = %d when detected, want 1", count)
	}
}

// TestSaveRecord_RollbackOnDuplicate verifies that a constraint violation
// during the FIRST insert (analytics_records primary-key clash) rolls back
// the entire transaction so no child rows are created.
func TestSaveRecord_RollbackOnDuplicate(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	rec := validRecord()
	if err := store.SaveRecord(rec); err != nil {
		t.Fatalf("first SaveRecord: %v", err)
	}

	beforeOutcomes := tableCount(t, db, "sentiment_outcomes")
	beforeBudget := tableCount(t, db, "budget_history")

	// Same validation_id violates PRIMARY KEY.
	err := store.SaveRecord(rec)
	if err == nil {
		t.Fatalf("expected error on duplicate validation_id")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "fail") {
		// just sanity check error message wraps something
	}

	if got := tableCount(t, db, "sentiment_outcomes"); got != beforeOutcomes {
		t.Errorf("sentiment_outcomes grew after rollback: before=%d after=%d", beforeOutcomes, got)
	}
	if got := tableCount(t, db, "budget_history"); got != beforeBudget {
		t.Errorf("budget_history grew after rollback: before=%d after=%d", beforeBudget, got)
	}
}

// TestSaveRecord_RollbackOnChildFailure verifies that a failure in a
// later child insert rolls back the primary insert too.
//
// We trigger this by dropping a child table after insert #1 succeeded
// for a brand-new record, forcing the second helper call to fail.
func TestSaveRecord_RollbackOnChildFailure(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	// Drop the budget_history table to force failure in
	// storeBudgetImpactWithTx (called after analytics_records and
	// sentiment_outcomes succeed).
	if _, err := db.Exec("DROP TABLE budget_history"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	rec := validRecord()
	if err := store.SaveRecord(rec); err == nil {
		t.Fatalf("expected error from missing budget_history")
	}

	// analytics_records insert ran first inside the transaction; it must
	// have rolled back too.
	if got := tableCount(t, db, "analytics_records"); got != 0 {
		t.Errorf("analytics_records count after rollback = %d, want 0", got)
	}
	if got := tableCount(t, db, "sentiment_outcomes"); got != 0 {
		t.Errorf("sentiment_outcomes count after rollback = %d, want 0", got)
	}
}

// TestSaveRecord_ErrorPropagation verifies the error wraps the helper's
// context so callers can identify which step failed.
func TestSaveRecord_ErrorPropagation(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	if _, err := db.Exec("DROP TABLE sentiment_outcomes"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	rec := validRecord()
	err := store.SaveRecord(rec)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "sentiment outcome") {
		t.Errorf("expected error to mention 'sentiment outcome', got: %v", err)
	}
}

func tableCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		// Table dropped; treat as zero rows.
		return 0
	}
	return n
}
