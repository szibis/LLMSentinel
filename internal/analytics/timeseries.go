// Package analytics provides time-series metrics and trend analysis.
package analytics

import (
	"database/sql"
	"fmt"
	"time"
)

// MetricsTimeSeries represents aggregated metrics over a time bucket.
type MetricsTimeSeries struct {
	Timestamp       time.Time `json:"timestamp"`
	Bucket          string    `json:"bucket"` // "hourly", "daily", "weekly"
	TotalRequests   int64     `json:"total_requests"`
	CacheHits       int64     `json:"cache_hits"`
	CacheMisses     int64     `json:"cache_misses"`
	BatchQueued     int64     `json:"batch_queued"`
	DirectRequests  int64     `json:"direct_requests"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	EstimatedCostUSD float64  `json:"estimated_cost_usd"`
	ActualCostUSD   float64   `json:"actual_cost_usd"`
	SavingsUSD      float64   `json:"savings_usd"`
	SuccessRate     float64   `json:"success_rate"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	P50LatencyMs    float64   `json:"p50_latency_ms"`
	P95LatencyMs    float64   `json:"p95_latency_ms"`
	P99LatencyMs    float64   `json:"p99_latency_ms"`
}

// TimeSeriesStore handles time-series metrics persistence and aggregation.
type TimeSeriesStore struct {
	db *sql.DB
}

// NewTimeSeriesStore creates a time-series metrics store.
func NewTimeSeriesStore(db *sql.DB) *TimeSeriesStore {
	return &TimeSeriesStore{db: db}
}

// CreateBuckets creates the time-series tables if they don't exist.
func (tss *TimeSeriesStore) CreateBuckets() error {
	buckets := []string{
		`CREATE TABLE IF NOT EXISTS metrics_hourly (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL UNIQUE,
			total_requests INTEGER DEFAULT 0,
			cache_hits INTEGER DEFAULT 0,
			cache_misses INTEGER DEFAULT 0,
			batch_queued INTEGER DEFAULT 0,
			direct_requests INTEGER DEFAULT 0,
			total_cost_usd REAL DEFAULT 0,
			estimated_cost_usd REAL DEFAULT 0,
			actual_cost_usd REAL DEFAULT 0,
			savings_usd REAL DEFAULT 0,
			success_rate REAL DEFAULT 0,
			avg_latency_ms REAL DEFAULT 0,
			p50_latency_ms REAL DEFAULT 0,
			p95_latency_ms REAL DEFAULT 0,
			p99_latency_ms REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS metrics_daily (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL UNIQUE,
			total_requests INTEGER DEFAULT 0,
			cache_hits INTEGER DEFAULT 0,
			cache_misses INTEGER DEFAULT 0,
			batch_queued INTEGER DEFAULT 0,
			direct_requests INTEGER DEFAULT 0,
			total_cost_usd REAL DEFAULT 0,
			estimated_cost_usd REAL DEFAULT 0,
			actual_cost_usd REAL DEFAULT 0,
			savings_usd REAL DEFAULT 0,
			success_rate REAL DEFAULT 0,
			avg_latency_ms REAL DEFAULT 0,
			p50_latency_ms REAL DEFAULT 0,
			p95_latency_ms REAL DEFAULT 0,
			p99_latency_ms REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS metrics_weekly (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL UNIQUE,
			total_requests INTEGER DEFAULT 0,
			cache_hits INTEGER DEFAULT 0,
			cache_misses INTEGER DEFAULT 0,
			batch_queued INTEGER DEFAULT 0,
			direct_requests INTEGER DEFAULT 0,
			total_cost_usd REAL DEFAULT 0,
			estimated_cost_usd REAL DEFAULT 0,
			actual_cost_usd REAL DEFAULT 0,
			savings_usd REAL DEFAULT 0,
			success_rate REAL DEFAULT 0,
			avg_latency_ms REAL DEFAULT 0,
			p50_latency_ms REAL DEFAULT 0,
			p95_latency_ms REAL DEFAULT 0,
			p99_latency_ms REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range buckets {
		if _, err := tss.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create metrics table: %w", err)
		}
	}

	return nil
}

// AggregateHourly aggregates validation metrics into hourly buckets.
// Call this hourly via cron job.
func (tss *TimeSeriesStore) AggregateHourly() error {
	now := time.Now()
	hourStart := now.Add(-time.Hour).Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	// Aggregate from validation_metrics table
	query := `
		SELECT
			COALESCE(COUNT(*), 0) as total_requests,
			COALESCE(SUM(CASE WHEN cached THEN 1 ELSE 0 END), 0) as cache_hits,
			COALESCE(SUM(CASE WHEN cached THEN 0 ELSE 1 END), 0) as cache_misses,
			COALESCE(SUM(CASE WHEN batched THEN 1 ELSE 0 END), 0) as batch_queued,
			COALESCE(SUM(CASE WHEN cached OR batched THEN 0 ELSE 1 END), 0) as direct_requests,
			COALESCE(SUM(estimated_cost_usd), 0) as estimated_cost_usd,
			COALESCE(SUM(actual_cost_usd), 0) as actual_cost_usd,
			COALESCE(SUM(actual_cost_usd - estimated_cost_usd), 0) as savings_usd,
			COALESCE(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(AVG(latency_ms), 0) as avg_latency_ms
		FROM validation_metrics
		WHERE timestamp >= ? AND timestamp < ?
	`

	var mts MetricsTimeSeries
	var successCount int64

	err := tss.db.QueryRow(query, hourStart, hourEnd).Scan(
		&mts.TotalRequests,
		&mts.CacheHits,
		&mts.CacheMisses,
		&mts.BatchQueued,
		&mts.DirectRequests,
		&mts.EstimatedCostUSD,
		&mts.ActualCostUSD,
		&mts.SavingsUSD,
		&successCount,
		&mts.AvgLatencyMs,
	)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to aggregate hourly metrics: %w", err)
	}

	// Calculate success rate
	if mts.TotalRequests > 0 {
		mts.SuccessRate = float64(successCount) / float64(mts.TotalRequests)
		mts.TotalCostUSD = mts.ActualCostUSD
	}

	mts.Timestamp = hourStart
	mts.Bucket = "hourly"

	// Insert into metrics_hourly
	insertQuery := `
		INSERT INTO metrics_hourly (
			timestamp, total_requests, cache_hits, cache_misses, batch_queued, direct_requests,
			total_cost_usd, estimated_cost_usd, actual_cost_usd, savings_usd,
			success_rate, avg_latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(timestamp) DO UPDATE SET
			total_requests = excluded.total_requests,
			cache_hits = excluded.cache_hits,
			cache_misses = excluded.cache_misses,
			batch_queued = excluded.batch_queued,
			direct_requests = excluded.direct_requests,
			total_cost_usd = excluded.total_cost_usd,
			estimated_cost_usd = excluded.estimated_cost_usd,
			actual_cost_usd = excluded.actual_cost_usd,
			savings_usd = excluded.savings_usd,
			success_rate = excluded.success_rate,
			avg_latency_ms = excluded.avg_latency_ms
	`

	_, err = tss.db.Exec(insertQuery,
		mts.Timestamp, mts.TotalRequests, mts.CacheHits, mts.CacheMisses, mts.BatchQueued, mts.DirectRequests,
		mts.TotalCostUSD, mts.EstimatedCostUSD, mts.ActualCostUSD, mts.SavingsUSD, mts.SuccessRate, mts.AvgLatencyMs,
	)

	return err
}

// GetTrend retrieves time-series data for a specific metric over a date range.
// bucket: "hourly", "daily", or "weekly"
// days: number of days to look back
func (tss *TimeSeriesStore) GetTrend(bucket string, days int) ([]MetricsTimeSeries, error) {
	var tableName string
	switch bucket {
	case "hourly":
		tableName = "metrics_hourly"
	case "daily":
		tableName = "metrics_daily"
	case "weekly":
		tableName = "metrics_weekly"
	default:
		return nil, fmt.Errorf("invalid bucket: %s", bucket)
	}

	query := fmt.Sprintf(`
		SELECT
			timestamp, total_requests, cache_hits, cache_misses, batch_queued, direct_requests,
			total_cost_usd, estimated_cost_usd, actual_cost_usd, savings_usd,
			success_rate, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms
		FROM %s
		WHERE timestamp >= datetime('now', '-%d days')
		ORDER BY timestamp ASC
	`, tableName, days)

	rows, err := tss.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query trend: %w", err)
	}
	defer rows.Close()

	var trends []MetricsTimeSeries
	for rows.Next() {
		var mts MetricsTimeSeries
		err := rows.Scan(
			&mts.Timestamp, &mts.TotalRequests, &mts.CacheHits, &mts.CacheMisses, &mts.BatchQueued, &mts.DirectRequests,
			&mts.TotalCostUSD, &mts.EstimatedCostUSD, &mts.ActualCostUSD, &mts.SavingsUSD,
			&mts.SuccessRate, &mts.AvgLatencyMs, &mts.P50LatencyMs, &mts.P95LatencyMs, &mts.P99LatencyMs,
		)
		if err != nil {
			continue
		}

		mts.Bucket = bucket
		trends = append(trends, mts)
	}

	return trends, nil
}

// EnforceRetention deletes old records based on retention policy.
func (tss *TimeSeriesStore) EnforceRetention(hourlyDays, dailyDays, weeklyDays int) error {
	queries := []struct {
		table string
		days  int
	}{
		{"metrics_hourly", hourlyDays},
		{"metrics_daily", dailyDays},
		{"metrics_weekly", weeklyDays},
	}

	for _, q := range queries {
		deleteQuery := fmt.Sprintf(`
			DELETE FROM %s
			WHERE timestamp < datetime('now', '-%d days')
		`, q.table, q.days)

		if _, err := tss.db.Exec(deleteQuery); err != nil {
			return fmt.Errorf("failed to enforce retention on %s: %w", q.table, err)
		}
	}

	return nil
}
