package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/metrics"
)

// MetricsHandler handles metrics API endpoints
type MetricsHandler struct {
	sessionMetrics *metrics.SessionMetrics
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(sm *metrics.SessionMetrics) *MetricsHandler {
	return &MetricsHandler{
		sessionMetrics: sm,
	}
}

// RegisterMetricsEndpoints registers all metrics endpoints
func (mh *MetricsHandler) RegisterMetricsEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("/api/metrics/overview", mh.handleOverview)
	mux.HandleFunc("/api/metrics/daily", mh.handleDaily)
	mux.HandleFunc("/api/metrics/breakdown", mh.handleBreakdown)
	mux.HandleFunc("/api/metrics/projections", mh.handleProjections)
	mux.HandleFunc("/api/metrics/full", mh.handleFullMetrics)
	mux.HandleFunc("/api/metrics/export/json", mh.handleExportJSON)
	mux.HandleFunc("/api/metrics/export/csv", mh.handleExportCSV)
}

// handleOverview returns high-level metrics summary
func (mh *MetricsHandler) handleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	savingsPercent := mh.sessionMetrics.CalculateSavingsPercent()
	response := map[string]interface{}{
		"tokens_burned": map[string]interface{}{
			"input":          mh.sessionMetrics.TotalBurned.InputTokens,
			"output":         mh.sessionMetrics.TotalBurned.OutputTokens,
			"cache_read":     mh.sessionMetrics.TotalBurned.CacheReadTokens,
			"cache_write":    mh.sessionMetrics.TotalBurned.CacheWriteTokens,
			"total":          mh.sessionMetrics.TotalBurned.TotalTokens,
			"estimated_cost": mh.sessionMetrics.TotalBurned.EstimatedCostUSD,
		},
		"tokens_saved": map[string]interface{}{
			"total":             mh.sessionMetrics.TotalSaved.TotalTokensSaved,
			"savings_percent":   savingsPercent,
			"estimated_savings": mh.sessionMetrics.TotalSaved.EstimatedCostSavedUSD,
		},
		"requests":            mh.sessionMetrics.TotalRequests,
		"cache_hit_rate":      mh.sessionMetrics.AverageCacheHitRate,
		"false_positive_rate": mh.sessionMetrics.AvgFalsePositiveRate,
		"timestamp":           time.Now(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleDaily returns daily breakdown
func (mh *MetricsHandler) handleDaily(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get date range from query params (default to last 7 days)
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsedDays := parseInt(d); parsedDays > 0 {
			days = parsedDays
		}
	}

	dailyBreakdown := make([]map[string]interface{}, 0)
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		daily := mh.sessionMetrics.GetDailySummary(date)
		if daily == nil {
			continue
		}

		dailyBreakdown = append(dailyBreakdown, map[string]interface{}{
			"date": date.Format("2006-01-02"),
			"burned": map[string]interface{}{
				"input":  daily.Burned.InputTokens,
				"output": daily.Burned.OutputTokens,
				"total":  daily.Burned.TotalTokens,
				"cost":   daily.Burned.EstimatedCostUSD,
			},
			"saved": map[string]interface{}{
				"total":             daily.Saved.TotalTokensSaved,
				"estimated_savings": daily.Saved.EstimatedCostSavedUSD,
			},
			"requests":       daily.RequestCount,
			"cache_hit_rate": daily.CacheHitRate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"daily": dailyBreakdown,
	})
}

// handleBreakdown returns per-optimization breakdown
func (mh *MetricsHandler) handleBreakdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	breakdown := map[string]interface{}{
		"exact_dedup": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.ExactDedup.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.ExactDedup.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.ExactDedup.HitCount,
		},
		"semantic_cache": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.SemanticCache.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.SemanticCache.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.SemanticCache.HitCount,
		},
		"input_optimization": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.InputOptimization.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.InputOptimization.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.InputOptimization.HitCount,
		},
		"output_optimization": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.OutputOptimization.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.OutputOptimization.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.OutputOptimization.HitCount,
		},
		"rtk_proxy": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.RTKProxy.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.RTKProxy.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.RTKProxy.HitCount,
		},
		"batch_api": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.BatchAPI.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.BatchAPI.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.BatchAPI.HitCount,
		},
		"knowledge_graph": map[string]interface{}{
			"tokens_saved": mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.TokensSaved,
			"cost_saved":   mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.CostSavedUSD,
			"hit_count":    mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.HitCount,
		},
	}

	json.NewEncoder(w).Encode(breakdown)
}

// handleProjections returns monthly projections
func (mh *MetricsHandler) handleProjections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Calculate 7-day and 30-day projections
	proj7 := mh.sessionMetrics.ProjectMonthly(7)
	proj30 := mh.sessionMetrics.ProjectMonthly(30)

	response := map[string]interface{}{
		"projections_7day": map[string]interface{}{
			"projected_tokens_burned":   proj7.ProjectedTokensBurned,
			"projected_tokens_saved":    proj7.ProjectedTokensSaved,
			"projected_cost":            proj7.ProjectedCostUSD,
			"projected_savings":         proj7.ProjectedSavingsUSD,
			"projected_savings_percent": proj7.ProjectedSavingsPercent,
			"based_on_days":             proj7.BasedOnDays,
			"confidence":                proj7.ProjectionConfidence,
		},
		"projections_30day": map[string]interface{}{
			"projected_tokens_burned":   proj30.ProjectedTokensBurned,
			"projected_tokens_saved":    proj30.ProjectedTokensSaved,
			"projected_cost":            proj30.ProjectedCostUSD,
			"projected_savings":         proj30.ProjectedSavingsUSD,
			"projected_savings_percent": proj30.ProjectedSavingsPercent,
			"based_on_days":             proj30.BasedOnDays,
			"confidence":                proj30.ProjectionConfidence,
		},
	}

	json.NewEncoder(w).Encode(response)
}

// handleFullMetrics returns complete metrics in codeburn format
func (mh *MetricsHandler) handleFullMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mh.sessionMetrics.GetJSON())
}

// handleExportJSON exports metrics as JSON
func (mh *MetricsHandler) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=claude-escalate-metrics.json")

	json.NewEncoder(w).Encode(mh.sessionMetrics.GetJSON())
}

// handleExportCSV exports metrics as CSV
func (mh *MetricsHandler) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=claude-escalate-metrics.csv")

	csv := "Metric,Value,Unit\n"
	csv += "Tokens Burned (Total)," + formatInt(mh.sessionMetrics.TotalBurned.TotalTokens) + ",tokens\n"
	csv += "Tokens Burned (Input)," + formatInt(mh.sessionMetrics.TotalBurned.InputTokens) + ",tokens\n"
	csv += "Tokens Burned (Output)," + formatInt(mh.sessionMetrics.TotalBurned.OutputTokens) + ",tokens\n"
	csv += "Tokens Saved (Total)," + formatInt(mh.sessionMetrics.TotalSaved.TotalTokensSaved) + ",tokens\n"
	csv += "Cost (Estimated),$" + formatFloat(mh.sessionMetrics.TotalBurned.EstimatedCostUSD) + ",USD\n"
	csv += "Savings (Estimated),$" + formatFloat(mh.sessionMetrics.TotalSaved.EstimatedCostSavedUSD) + ",USD\n"
	csv += "Savings Percentage," + formatFloat(mh.sessionMetrics.CalculateSavingsPercent()) + ",%\n"
	csv += "Total Requests," + formatInt(mh.sessionMetrics.TotalRequests) + ",count\n"
	csv += "Cache Hit Rate," + formatFloat(mh.sessionMetrics.AverageCacheHitRate) + ",%\n"
	csv += "\nOptimization Layer,Tokens Saved,Cost Saved,Hit Count\n"
	csv += "Exact Dedup," + formatInt(mh.sessionMetrics.TotalBreakdown.ExactDedup.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.ExactDedup.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.ExactDedup.HitCount) + "\n"
	csv += "Semantic Cache," + formatInt(mh.sessionMetrics.TotalBreakdown.SemanticCache.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.SemanticCache.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.SemanticCache.HitCount) + "\n"
	csv += "Input Optimization," + formatInt(mh.sessionMetrics.TotalBreakdown.InputOptimization.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.InputOptimization.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.InputOptimization.HitCount) + "\n"
	csv += "Output Optimization," + formatInt(mh.sessionMetrics.TotalBreakdown.OutputOptimization.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.OutputOptimization.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.OutputOptimization.HitCount) + "\n"
	csv += "RTK Proxy," + formatInt(mh.sessionMetrics.TotalBreakdown.RTKProxy.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.RTKProxy.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.RTKProxy.HitCount) + "\n"
	csv += "Batch API," + formatInt(mh.sessionMetrics.TotalBreakdown.BatchAPI.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.BatchAPI.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.BatchAPI.HitCount) + "\n"
	csv += "Knowledge Graph," + formatInt(mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.TokensSaved) + ",$" + formatFloat(mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.CostSavedUSD) + "," + formatInt(mh.sessionMetrics.TotalBreakdown.KnowledgeGraph.HitCount) + "\n"

	w.Write([]byte(csv))
}

// Helper functions
func parseInt(s string) int {
	var i int
	_, _ = fmt.Sscanf(s, "%d", &i)
	return i
}

func formatInt(i int64) string {
	return fmt.Sprintf("%d", i)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
