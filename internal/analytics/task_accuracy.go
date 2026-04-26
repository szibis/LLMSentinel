// Package analytics provides task-model accuracy tracking and analysis.
package analytics

import (
	"database/sql"
	"fmt"
)

// TaskModelAccuracy tracks success rates for specific task-model combinations.
type TaskModelAccuracy struct {
	TaskType      string  `json:"task_type"`
	Model         string  `json:"model"`
	SuccessCount  int     `json:"success_count"`
	TotalCount    int     `json:"total_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgTokenError float64 `json:"avg_token_error"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// TaskDifficulty ranks tasks by success rate.
type TaskDifficulty struct {
	TaskType    string  `json:"task_type"`
	SuccessRate float64 `json:"success_rate"`
	TotalCount  int     `json:"total_count"`
}

// TaskAccuracyAnalyzer computes task-model accuracy metrics.
type TaskAccuracyAnalyzer struct {
	db *sql.DB
}

// NewTaskAccuracyAnalyzer creates a task accuracy analyzer.
func NewTaskAccuracyAnalyzer(db *sql.DB) *TaskAccuracyAnalyzer {
	return &TaskAccuracyAnalyzer{db: db}
}

// CalculateTaskModelAccuracy computes success rates for a specific task-model pair.
func (taa *TaskAccuracyAnalyzer) CalculateTaskModelAccuracy(taskType, model string, days int) (*TaskModelAccuracy, error) {
	query := `
		SELECT
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) as success_count,
			COUNT(*) as total_count,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate,
			AVG(token_error) as avg_token_error,
			AVG(latency_ms) as avg_latency_ms
		FROM validation_metrics
		WHERE task_type = ? AND model = ?
		AND timestamp >= datetime('now', '-' || ? || ' days')
	`

	var acc TaskModelAccuracy
	var successCount float64

	err := taa.db.QueryRow(query, taskType, model, days).Scan(
		&successCount, &acc.TotalCount, &acc.SuccessRate, &acc.AvgTokenError, &acc.AvgLatencyMs,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no data for task %s with model %s", taskType, model)
		}
		return nil, err
	}

	acc.TaskType = taskType
	acc.Model = model
	acc.SuccessCount = int(successCount)

	return &acc, nil
}

// GetAllTaskAccuracies returns accuracy metrics for all task-model combinations.
func (taa *TaskAccuracyAnalyzer) GetAllTaskAccuracies(days int, minSamples int) ([]TaskModelAccuracy, error) {
	query := fmt.Sprintf(`
		SELECT
			task_type,
			model,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) as success_count,
			COUNT(*) as total_count,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate,
			AVG(token_error) as avg_token_error,
			AVG(latency_ms) as avg_latency_ms
		FROM validation_metrics
		WHERE timestamp >= datetime('now', '-%d days')
		GROUP BY task_type, model
		HAVING COUNT(*) >= %d
		ORDER BY task_type, success_rate DESC
	`, days, minSamples)

	rows, err := taa.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accuracies []TaskModelAccuracy
	for rows.Next() {
		var acc TaskModelAccuracy
		var successCount float64

		err := rows.Scan(
			&acc.TaskType, &acc.Model, &successCount, &acc.TotalCount, &acc.SuccessRate,
			&acc.AvgTokenError, &acc.AvgLatencyMs,
		)
		if err != nil {
			continue
		}

		acc.SuccessCount = int(successCount)
		accuracies = append(accuracies, acc)
	}

	return accuracies, nil
}

// GetBestModelForTask finds the cheapest model with >80% success rate for a task.
func (taa *TaskAccuracyAnalyzer) GetBestModelForTask(taskType string, days int) (string, float64, error) {
	query := `
		SELECT
			model,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM validation_metrics
		WHERE task_type = ? AND timestamp >= datetime('now', '-' || ? || ' days')
		GROUP BY model
		HAVING success_rate >= 0.80
		ORDER BY model ASC
	`

	rows, err := taa.db.Query(query, taskType, days)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	// Return first model (cheapest: haiku < sonnet < opus alphabetically)
	if rows.Next() {
		var model string
		var successRate float64

		if err := rows.Scan(&model, &successRate); err != nil {
			return "", 0, err
		}

		return model, successRate, nil
	}

	return "", 0, fmt.Errorf("no model found with >80%% success for task %s", taskType)
}

// GetTaskDifficulty ranks tasks by success rate (lower rate = harder).
func (taa *TaskAccuracyAnalyzer) GetTaskDifficulty(days int) ([]TaskDifficulty, error) {
	query := fmt.Sprintf(`
		SELECT
			task_type,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate,
			COUNT(*) as total_count
		FROM validation_metrics
		WHERE timestamp >= datetime('now', '-%d days')
		GROUP BY task_type
		ORDER BY success_rate ASC
	`, days)

	rows, err := taa.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TaskDifficulty
	for rows.Next() {
		var td TaskDifficulty

		if err := rows.Scan(&td.TaskType, &td.SuccessRate, &td.TotalCount); err != nil {
			continue
		}

		results = append(results, td)
	}

	return results, nil
}

// GetTasksByModel returns all tasks handled by a specific model with accuracy.
func (taa *TaskAccuracyAnalyzer) GetTasksByModel(model string, days int) ([]TaskModelAccuracy, error) {
	query := fmt.Sprintf(`
		SELECT
			task_type,
			model,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) as success_count,
			COUNT(*) as total_count,
			CAST(SUM(CASE WHEN token_error < 0.15 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate,
			AVG(token_error) as avg_token_error,
			AVG(latency_ms) as avg_latency_ms
		FROM validation_metrics
		WHERE model = ? AND timestamp >= datetime('now', '-%d days')
		GROUP BY task_type
		ORDER BY success_rate DESC
	`, days)

	rows, err := taa.db.Query(query, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accuracies []TaskModelAccuracy
	for rows.Next() {
		var acc TaskModelAccuracy
		var successCount float64

		err := rows.Scan(
			&acc.TaskType, &acc.Model, &successCount, &acc.TotalCount, &acc.SuccessRate,
			&acc.AvgTokenError, &acc.AvgLatencyMs,
		)
		if err != nil {
			continue
		}

		acc.SuccessCount = int(successCount)
		accuracies = append(accuracies, acc)
	}

	return accuracies, nil
}
