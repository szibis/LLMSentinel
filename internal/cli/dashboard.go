package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DashboardCLI provides command-line dashboard views.
type DashboardCLI struct {
	serverURL string
}

// NewDashboardCLI creates a new CLI dashboard.
func NewDashboardCLI(serverURL string) *DashboardCLI {
	return &DashboardCLI{serverURL: serverURL}
}

// SentimentDashboard displays sentiment trends in the terminal.
func (d *DashboardCLI) SentimentDashboard() error {
	resp, err := http.Get(d.serverURL + "/api/analytics/sentiment-trends?hours=24")
	if err != nil {
		return fmt.Errorf("failed to fetch sentiment data: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to parse sentiment data: %w", err)
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    😊 SENTIMENT DASHBOARD (24h)                ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if summary, ok := data["summary"].(map[string]interface{}); ok {
		fmt.Printf("Satisfaction Rate: ")
		if rate, ok := summary["satisfaction_rate"].(float64); ok {
			pct := int(rate * 100)
			fmt.Printf("%d%%", pct)
			fmt.Print(" ")
			if pct >= 80 {
				fmt.Println("✅ Excellent")
			} else if pct >= 60 {
				fmt.Println("⚠️  Good")
			} else {
				fmt.Println("❌ Needs Attention")
			}
		}

		// Sentiment breakdown
		fmt.Println("\nSentiment Distribution:")
		sentiments := []struct {
			emoji string
			label string
			key   string
		}{
			{"😊", "Satisfied", "satisfied"},
			{"😐", "Neutral", "neutral"},
			{"😤", "Frustrated", "frustrated"},
			{"🤔", "Confused", "confused"},
			{"⏱️ ", "Impatient", "impatient"},
		}

		for _, s := range sentiments {
			if count, ok := summary[s.key].(float64); ok {
				total, _ := summary["total"].(float64)
				pct := int(count * 100 / total)
				barLen := pct / 5
				bar := ""
				for i := 0; i < barLen; i++ {
					bar += "█"
				}
				for i := barLen; i < 20; i++ {
					bar += "░"
				}
				fmt.Printf("  %s %-12s [%s] %3d%% (%d)\n", s.emoji, s.label, bar, pct, int(count))
			}
		}
	}

	// Frustration events
	if events, ok := data["events"].([]interface{}); ok && len(events) > 0 {
		fmt.Println("\n🚨 Frustration Events:")
		for _, ev := range events {
			if e, ok := ev.(map[string]interface{}); ok {
				timestamp, _ := e["Timestamp"].(string)
				taskType, _ := e["TaskType"].(string)
				initialModel, _ := e["InitialModel"].(string)
				escalatedTo, _ := e["EscalatedTo"].(string)
				resolved, _ := e["Resolved"].(bool)

				t, _ := time.Parse(time.RFC3339, timestamp)
				timeStr := t.Format("15:04")

				status := "❌"
				if resolved {
					status = "✅"
				}

				fmt.Printf("  %s [%s] %s (%s → %s)\n", status, timeStr, taskType, initialModel, escalatedTo)
			}
		}
	}

	fmt.Println()
	return nil
}

// BudgetDashboard displays budget status in the terminal.
func (d *DashboardCLI) BudgetDashboard() error {
	resp, err := http.Get(d.serverURL + "/api/analytics/budget-status")
	if err != nil {
		return fmt.Errorf("failed to fetch budget data: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to parse budget data: %w", err)
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     💰 BUDGET DASHBOARD                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Daily budget
	if daily, ok := data["daily_budget"].(map[string]interface{}); ok {
		fmt.Println("Daily Budget:")
		limit, _ := daily["limit"].(float64)
		used, _ := daily["used"].(float64)
		remaining, _ := daily["remaining"].(float64)
		percentage, _ := daily["percentage"].(float64)

		fmt.Printf("  Limit:     $%.2f\n", limit)
		fmt.Printf("  Used:      $%.2f\n", used)
		fmt.Printf("  Remaining: $%.2f\n", remaining)
		fmt.Printf("  Usage:     %.1f%%", percentage)

		barLen := int(percentage / 5)
		bar := ""
		for i := 0; i < barLen; i++ {
			bar += "█"
		}
		for i := barLen; i < 20; i++ {
			bar += "░"
		}

		status := "✅"
		if percentage > 90 {
			status = "🔴"
		} else if percentage > 75 {
			status = "🟡"
		}
		fmt.Printf(" %s [%s]\n", status, bar)
	}

	// Monthly budget
	fmt.Println("\nMonthly Budget:")
	if monthly, ok := data["monthly_budget"].(map[string]interface{}); ok {
		limit, _ := monthly["limit"].(float64)
		used, _ := monthly["used"].(float64)
		remaining, _ := monthly["remaining"].(float64)
		daysLeft, _ := monthly["days_left"].(float64)
		percentage, _ := monthly["percentage"].(float64)

		fmt.Printf("  Limit:      $%.2f\n", limit)
		fmt.Printf("  Used:       $%.2f\n", used)
		fmt.Printf("  Remaining:  $%.2f\n", remaining)
		fmt.Printf("  Days Left:  %.0f days\n", daysLeft)
		fmt.Printf("  Usage:      %.1f%%", percentage)

		barLen := int(percentage / 5)
		bar := ""
		for i := 0; i < barLen; i++ {
			bar += "█"
		}
		for i := barLen; i < 20; i++ {
			bar += "░"
		}

		status := "✅"
		if percentage > 90 {
			status = "🔴"
		} else if percentage > 75 {
			status = "🟡"
		}
		fmt.Printf(" %s [%s]\n", status, bar)
	}

	// Model usage
	fmt.Println("\nModel Usage:")
	if models, ok := data["model_usage"].(map[string]interface{}); ok {
		for model, usage := range models {
			if u, ok := usage.(map[string]interface{}); ok {
				used, _ := u["Used"].(float64)
				limit, _ := u["Limit"].(float64)
				pct, _ := u["Percentage"].(float64)

				fmt.Printf("  %-8s: $%.2f / $%.2f (%.1f%%)\n", model, used, limit, pct)
			}
		}
	}

	fmt.Println()
	return nil
}

// CostOptimizationDashboard displays cost optimization recommendations.
func (d *DashboardCLI) CostOptimizationDashboard() error {
	resp, err := http.Get(d.serverURL + "/api/analytics/cost-optimization")
	if err != nil {
		return fmt.Errorf("failed to fetch optimization data: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to parse optimization data: %w", err)
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                🎯 COST OPTIMIZATION DASHBOARD                  ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if recs, ok := data["recommendations"].([]interface{}); ok && len(recs) > 0 {
		fmt.Printf("Found %d optimization opportunities:\n\n", len(recs))

		totalSavings := 0.0
		for i, rec := range recs {
			if r, ok := rec.(map[string]interface{}); ok {
				taskType, _ := r["task_type"].(string)
				currentModel, _ := r["current_model"].(string)
				recommendedModel, _ := r["recommended_model"].(string)
				currentSat, _ := r["current_satisfaction"].(float64)
				recommendedSat, _ := r["recommended_satisfaction"].(float64)
				savings, _ := r["estimated_savings_percent"].(float64)

				totalSavings += savings

				fmt.Printf("%d. %s\n", i+1, taskType)
				fmt.Printf("   Current:    %s (%.0f%% satisfaction)\n", currentModel, currentSat*100)
				fmt.Printf("   Recommend:  %s (%.0f%% satisfaction)\n", recommendedModel, recommendedSat*100)
				fmt.Printf("   Savings:    💰 %.1f%% cost reduction\n\n", savings)
			}
		}

		avgSavings := totalSavings / float64(len(recs))
		fmt.Printf("Average Savings: %.1f%%\n", avgSavings)
		fmt.Printf("Estimated Annual Impact: $%.2f saved\n\n", avgSavings*1200) // rough estimate
	} else {
		fmt.Println("✅ No optimization opportunities found. Budget is well-optimized!")
		fmt.Println()
	}

	return nil
}

// FullDashboard displays all data in a unified view.
func (d *DashboardCLI) FullDashboard() error {
	if err := d.SentimentDashboard(); err != nil {
		return err
	}
	if err := d.BudgetDashboard(); err != nil {
		return err
	}
	if err := d.CostOptimizationDashboard(); err != nil {
		return err
	}
	return nil
}

// fetchJSON is a helper to fetch and parse JSON from the API.
func fetchJSON(url string, v interface{}) error {
	// #nosec G107 - URL is constructed internally from localhost:port configuration, not from user input
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
