// Package analytics provides correlation analysis for causal insights.
package analytics

import (
	"database/sql"
	"fmt"
	"math"
)

// Correlation represents a statistical relationship between two variables.
type Correlation struct {
	Variable1   string  `json:"variable1"`
	Variable2   string  `json:"variable2"`
	Coefficient float64 `json:"coefficient"` // -1 to 1
	PValue      float64 `json:"p_value"`     // Statistical significance
	Significant bool    `json:"significant"` // p < 0.05
}

// CorrelationAnalyzer computes relationships between variables.
type CorrelationAnalyzer struct {
	db *sql.DB
}

// NewCorrelationAnalyzer creates a correlation analyzer.
func NewCorrelationAnalyzer(db *sql.DB) *CorrelationAnalyzer {
	return &CorrelationAnalyzer{db: db}
}

// AnalyzeCorrelations discovers significant correlations between all key variables.
// Variables analyzed: task_type, model, success_rate, latency, token_error, cost
func (ca *CorrelationAnalyzer) AnalyzeCorrelations(days int) ([]Correlation, error) {
	var correlations []Correlation

	// Define variable pairs to analyze
	pairs := []struct {
		var1, var2 string
	}{
		{"success_rate", "latency_ms"},
		{"success_rate", "token_error"},
		{"latency_ms", "actual_cost_usd"},
		{"token_error", "actual_cost_usd"},
		{"cache_hits", "actual_cost_usd"},
		{"batched", "savings"},
	}

	for _, pair := range pairs {
		corr, err := ca.computePearsonCorrelation(pair.var1, pair.var2, days)
		if err == nil && corr != nil {
			correlations = append(correlations, *corr)
		}
	}

	return correlations, nil
}

// computePearsonCorrelation calculates Pearson correlation coefficient between two metrics.
func (ca *CorrelationAnalyzer) computePearsonCorrelation(var1, var2 string, days int) (*Correlation, error) {
	// #nosec G201 - var1 and var2 are hardcoded metric names from AnalyzeCorrelations, not user input
	query := fmt.Sprintf(`
		SELECT
			SUM(%s) as sum_x,
			SUM(%s) as sum_y,
			SUM(%s * %s) as sum_xy,
			SUM(%s * %s) as sum_x2,
			SUM(%s * %s) as sum_y2,
			COUNT(*) as n
		FROM validation_metrics
		WHERE timestamp >= datetime('now', '-%d days')
		AND %s IS NOT NULL
		AND %s IS NOT NULL
	`, var1, var2, var1, var2, var1, var1, var2, var2, days, var1, var2)

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	var n int64

	err := ca.db.QueryRow(query).Scan(&sumX, &sumY, &sumXY, &sumX2, &sumY2, &n)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("correlation query failed: %w", err)
	}

	if n < 2 {
		return nil, nil
	}

	nf := float64(n)

	// Pearson correlation: r = (n*sumXY - sumX*sumY) / sqrt((n*sumX2 - sumX^2) * (n*sumY2 - sumY^2))
	numerator := nf*sumXY - sumX*sumY
	denominator := math.Sqrt((nf*sumX2 - sumX*sumX) * (nf*sumY2 - sumY*sumY))

	if denominator == 0 {
		return nil, nil
	}

	r := numerator / denominator

	// Clamp to [-1, 1]
	if r > 1 {
		r = 1
	} else if r < -1 {
		r = -1
	}

	// Compute p-value using t-distribution approximation
	tStat := r * math.Sqrt((nf-2)/(1-r*r))
	pValue := calculatePValue(tStat)

	return &Correlation{
		Variable1:   var1,
		Variable2:   var2,
		Coefficient: r,
		PValue:      pValue,
		Significant: pValue < 0.05,
	}, nil
}

// TaskModelCorrelation analyzes success rate by task type and model combination.
func (ca *CorrelationAnalyzer) TaskModelCorrelation(days int) (map[string]float64, error) {
	query := fmt.Sprintf(`
		SELECT
			task_type || ':' || model as task_model,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM validation_metrics
		WHERE timestamp >= datetime('now', '-%d days')
		GROUP BY task_type, model
		HAVING COUNT(*) >= 5
	`, days)

	rows, err := ca.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]float64)
	for rows.Next() {
		var taskModel string
		var successRate float64

		if err := rows.Scan(&taskModel, &successRate); err != nil {
			continue
		}

		results[taskModel] = successRate
	}

	return results, nil
}

// calculatePValue computes approximate p-value from t-statistic using normal approximation.
func calculatePValue(tStat float64) float64 {
	// Simplified p-value calculation using normal approximation
	// For exact p-value, would need t-distribution CDF
	absT := math.Abs(tStat)

	// Two-tailed test
	if absT > 3.0 {
		return 0.001
	} else if absT > 2.576 {
		return 0.01
	} else if absT > 1.96 {
		return 0.05
	} else if absT > 1.645 {
		return 0.10
	}

	return 0.5
}

// CacheEffectiveness analyzes impact of caching on overall costs.
func (ca *CorrelationAnalyzer) CacheEffectiveness(days int) (map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT
			CAST(SUM(CASE WHEN cached THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as cache_rate,
			SUM(CASE WHEN cached THEN actual_cost_usd ELSE 0 END) as cached_cost,
			SUM(CASE WHEN NOT cached THEN actual_cost_usd ELSE 0 END) as uncached_cost,
			SUM(actual_cost_usd) as total_cost,
			AVG(CASE WHEN cached THEN latency_ms ELSE NULL END) as cached_avg_latency,
			AVG(CASE WHEN NOT cached THEN latency_ms ELSE NULL END) as uncached_avg_latency
		FROM validation_metrics
		WHERE timestamp >= datetime('now', '-%d days')
	`, days)

	var cacheRate, cachedCost, uncachedCost, totalCost, cachedLatency, uncachedLatency sql.NullFloat64

	err := ca.db.QueryRow(query).Scan(&cacheRate, &cachedCost, &uncachedCost, &totalCost, &cachedLatency, &uncachedLatency)
	if err != nil {
		return nil, err
	}

	savings := 0.0
	if totalCost.Valid && uncachedCost.Valid {
		savings = (totalCost.Float64 - cachedCost.Float64) / totalCost.Float64
	}

	return map[string]interface{}{
		"cache_hit_rate":      cacheRate.Float64,
		"cached_cost_usd":     cachedCost.Float64,
		"uncached_cost_usd":   uncachedCost.Float64,
		"total_savings":       savings,
		"cached_avg_latency":  cachedLatency.Float64,
		"uncached_avg_latency": uncachedLatency.Float64,
	}, nil
}
