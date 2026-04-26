package analytics

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create validation_metrics table
	schema := `
		CREATE TABLE validation_metrics (
			id INTEGER PRIMARY KEY,
			timestamp DATETIME,
			task_type TEXT,
			model TEXT,
			cached BOOLEAN,
			batched BOOLEAN,
			estimated_cost_usd REAL,
			actual_cost_usd REAL,
			latency_ms REAL,
			token_error REAL
		);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestTimeSeriesStoreCreateBuckets(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewTimeSeriesStore(db)
	if err := store.CreateBuckets(); err != nil {
		t.Fatalf("failed to create buckets: %v", err)
	}

	// Verify tables exist
	tables := []string{"metrics_hourly", "metrics_daily", "metrics_weekly"}
	for _, table := range tables {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
		err := db.QueryRow(query, table).Scan(&count)
		if err != nil || count == 0 {
			t.Errorf("expected table %s to exist", table)
		}
	}
}

func TestAggregateHourly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewTimeSeriesStore(db)
	_ = store.CreateBuckets()

	// Insert test data
	now := time.Now()
	for i := 0; i < 5; i++ {
		query := `
			INSERT INTO validation_metrics (timestamp, task_type, model, cached, batched,
				estimated_cost_usd, actual_cost_usd, latency_ms, token_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		db.Exec(query, now.Add(-time.Duration(i)*time.Minute), "test", "haiku",
			i%2 == 0, i%3 == 0, 0.10, 0.09, 50.0, 0.05)
	}

	if err := store.AggregateHourly(); err != nil {
		t.Fatalf("aggregate failed: %v", err)
	}

	// Verify data was aggregated
	var count int
	db.QueryRow("SELECT COUNT(*) FROM metrics_hourly").Scan(&count)
	if count == 0 {
		t.Error("expected hourly metrics to be aggregated")
	}
}

func TestGetTrend(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewTimeSeriesStore(db)
	_ = store.CreateBuckets()

	// Insert test data for multiple days
	now := time.Now()
	for day := 0; day < 3; day++ {
		timestamp := now.AddDate(0, 0, -day)
		query := `
			INSERT INTO metrics_hourly (timestamp, total_requests, cache_hits, total_cost_usd)
			VALUES (?, ?, ?, ?)
		`
		db.Exec(query, timestamp, 100+day*10, 50+day*5, 10.0+float64(day))
	}

	trends, err := store.GetTrend("hourly", 7)
	if err != nil {
		t.Fatalf("get trend failed: %v", err)
	}

	if len(trends) == 0 {
		t.Error("expected trend data")
	}

	// Verify trend is ordered
	for i := 0; i < len(trends)-1; i++ {
		if trends[i].Timestamp.After(trends[i+1].Timestamp) {
			t.Error("trend should be ordered chronologically")
		}
	}
}

func TestEnforceRetention(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewTimeSeriesStore(db)
	_ = store.CreateBuckets()

	// Insert old and new data
	oldTime := time.Now().AddDate(0, 0, -10)
	newTime := time.Now().AddDate(0, 0, -2)

	db.Exec("INSERT INTO metrics_hourly (timestamp) VALUES (?)", oldTime)
	db.Exec("INSERT INTO metrics_hourly (timestamp) VALUES (?)", newTime)

	// Enforce 7-day retention
	if err := store.EnforceRetention(7, 30, 90); err != nil {
		t.Fatalf("enforce retention failed: %v", err)
	}

	// Verify old data is gone
	var count int
	db.QueryRow("SELECT COUNT(*) FROM metrics_hourly WHERE timestamp < ?", newTime).Scan(&count)
	if count != 0 {
		t.Errorf("expected old data removed, but found %d records", count)
	}

	// Verify new data remains
	db.QueryRow("SELECT COUNT(*) FROM metrics_hourly WHERE timestamp >= ?", newTime).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 new record, got %d", count)
	}
}

func TestMetricsTimeSeriesFields(t *testing.T) {
	mts := MetricsTimeSeries{
		Timestamp:       time.Now(),
		Bucket:          "hourly",
		TotalRequests:   1000,
		CacheHits:       500,
		SuccessRate:     0.95,
		TotalCostUSD:    100.50,
		P95LatencyMs:    250.5,
	}

	if mts.TotalRequests != 1000 {
		t.Error("TotalRequests not set correctly")
	}
	if mts.CacheHits != 500 {
		t.Error("CacheHits not set correctly")
	}
	if diff := mts.SuccessRate - 0.95; diff < -0.001 || diff > 0.001 {
		t.Error("SuccessRate not set correctly")
	}
}
