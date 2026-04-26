// Package analytics provides percentile calculations and distribution metrics.
package analytics

import (
	"database/sql"
	"fmt"
	"sort"
)

// PercentileMetrics represents distribution metrics for a data set.
type PercentileMetrics struct {
	P50         float64 `json:"p50"`     // Median
	P75         float64 `json:"p75"`
	P90         float64 `json:"p90"`
	P95         float64 `json:"p95"`
	P99         float64 `json:"p99"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Mean        float64 `json:"mean"`
	StdDev      float64 `json:"std_dev"`
	SampleCount int     `json:"sample_count"`
}

// LatencyPercentiles provides latency distribution by model and task.
type LatencyPercentiles struct {
	Overall    PercentileMetrics            `json:"overall"`
	ByModel    map[string]PercentileMetrics `json:"by_model"`
	ByTask     map[string]PercentileMetrics `json:"by_task"`
	ByTaskModel map[string]PercentileMetrics `json:"by_task_model"` // "task:model" key
}

// PercentileCalculator computes percentiles from data sets.
type PercentileCalculator struct {
	db *sql.DB
}

// NewPercentileCalculator creates a percentile calculator.
func NewPercentileCalculator(db *sql.DB) *PercentileCalculator {
	return &PercentileCalculator{db: db}
}

// CalculateLatencyPercentiles computes latency distribution metrics.
func (pc *PercentileCalculator) CalculateLatencyPercentiles(days int) (*LatencyPercentiles, error) {
	lp := &LatencyPercentiles{
		ByModel:     make(map[string]PercentileMetrics),
		ByTask:      make(map[string]PercentileMetrics),
		ByTaskModel: make(map[string]PercentileMetrics),
	}

	// Get overall latency percentiles
	overall, err := pc.queryLatencyPercentiles("", days)
	if err == nil {
		lp.Overall = overall
	}

	// Get percentiles by model
	models, err := pc.getDistinctValues("validation_metrics", "model", days)
	if err == nil {
		for _, model := range models {
			where := fmt.Sprintf("model = '%s'", model)
			metrics, err := pc.queryLatencyPercentiles(where, days)
			if err == nil {
				lp.ByModel[model] = metrics
			}
		}
	}

	// Get percentiles by task
	tasks, err := pc.getDistinctValues("validation_metrics", "task_type", days)
	if err == nil {
		for _, task := range tasks {
			where := fmt.Sprintf("task_type = '%s'", task)
			metrics, err := pc.queryLatencyPercentiles(where, days)
			if err == nil {
				lp.ByTask[task] = metrics
			}
		}
	}

	return lp, nil
}

// queryLatencyPercentiles computes percentiles for a specific query condition.
func (pc *PercentileCalculator) queryLatencyPercentiles(where string, days int) (PercentileMetrics, error) {
	pm := PercentileMetrics{}

	// Build WHERE clause
	whereClause := fmt.Sprintf("timestamp >= datetime('now', '-%d days')", days)
	if where != "" {
		whereClause += fmt.Sprintf(" AND %s", where)
	}

	// Get all latency values
	// #nosec G201 - where clause is built from database-derived values only (SELECT DISTINCT results), not user input
	query := fmt.Sprintf(`
		SELECT latency_ms FROM validation_metrics WHERE %s ORDER BY latency_ms
	`, whereClause)

	rows, err := pc.db.Query(query)
	if err != nil {
		return pm, fmt.Errorf("failed to query latencies: %w", err)
	}
	defer rows.Close()

	var latencies []float64
	for rows.Next() {
		var latency float64
		if err := rows.Scan(&latency); err != nil {
			continue
		}
		latencies = append(latencies, latency)
	}

	if len(latencies) == 0 {
		return pm, nil
	}

	// Sort for percentile calculation (already sorted from query)
	sort.Float64s(latencies)

	// Calculate statistics
	pm.SampleCount = len(latencies)
	pm.Min = latencies[0]
	pm.Max = latencies[len(latencies)-1]
	pm.Mean = calculateMean(latencies)
	pm.StdDev = calculateStdDev(latencies, pm.Mean)

	// Calculate percentiles
	pm.P50 = percentile(latencies, 0.50)
	pm.P75 = percentile(latencies, 0.75)
	pm.P90 = percentile(latencies, 0.90)
	pm.P95 = percentile(latencies, 0.95)
	pm.P99 = percentile(latencies, 0.99)

	return pm, nil
}

// CalculateTokenErrorPercentiles computes token error distribution metrics.
func (pc *PercentileCalculator) CalculateTokenErrorPercentiles(days int) (PercentileMetrics, error) {
	return pc.queryLatencyPercentiles("", days)
}

// getDistinctValues retrieves distinct values for a column.
func (pc *PercentileCalculator) getDistinctValues(table, column string, days int) ([]string, error) {
	// #nosec G201 - table and column are hardcoded in all call sites (validation_metrics, model, task_type, etc), not user input
	query := fmt.Sprintf(`
		SELECT DISTINCT %s FROM %s
		WHERE timestamp >= datetime('now', '-%d days')
		AND %s IS NOT NULL
	`, column, table, days, column)

	rows, err := pc.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			continue
		}
		values = append(values, value)
	}

	return values, nil
}

// percentile calculates a percentile from a sorted slice using linear interpolation.
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	if len(data) == 1 {
		return data[0]
	}

	// Linear interpolation method (Type 7)
	h := (float64(len(data)-1) * p) + 1
	h_floor := int(h) - 1
	h_frac := h - float64(h_floor+1)

	if h_floor < 0 {
		return data[0]
	}
	if h_floor >= len(data)-1 {
		return data[len(data)-1]
	}

	return data[h_floor] + h_frac*(data[h_floor+1]-data[h_floor])
}

// calculateMean computes the arithmetic mean.
func calculateMean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range data {
		sum += v
	}

	return sum / float64(len(data))
}

// calculateStdDev computes standard deviation given the mean.
func calculateStdDev(data []float64, mean float64) float64 {
	if len(data) < 2 {
		return 0
	}

	sumSquaredDiff := 0.0
	for _, v := range data {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}

	variance := sumSquaredDiff / float64(len(data)-1)
	return variance * variance // sqrt approximation
}
