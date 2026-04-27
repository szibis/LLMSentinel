// Claude Escalate Metrics CLI - View token tracking and optimization savings
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/szibis/claude-escalate/internal/metrics"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Parse global flags
	globalFlags := flag.NewFlagSet("", flag.ContinueOnError)
	formatFlag := globalFlags.String("format", "text", "Output format: text, json, csv")
	daysFlag := globalFlags.Int("days", 7, "Number of days to analyze")

	switch os.Args[1] {
	case "overview":
		cmdOverview()
	case "daily":
		cmdDaily(*daysFlag, *formatFlag)
	case "breakdown":
		cmdBreakdown(*formatFlag)
	case "projections":
		cmdProjections(*formatFlag)
	case "export":
		cmdExport(*formatFlag)
	case "status":
		cmdStatus()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func cmdOverview() {
	sm := metrics.NewSessionMetrics()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           Claude Escalate Metrics Overview                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	fmt.Println("\n📊 TOKENS BURNED (sent to Claude API)")
	fmt.Printf("   Input Tokens:        %15d\n", sm.TotalBurned.InputTokens)
	fmt.Printf("   Output Tokens:       %15d\n", sm.TotalBurned.OutputTokens)
	fmt.Printf("   Cache Read Tokens:   %15d\n", sm.TotalBurned.CacheReadTokens)
	fmt.Printf("   Cache Write Tokens:  %15d\n", sm.TotalBurned.CacheWriteTokens)
	fmt.Printf("   ├─ Total:            %15d\n", sm.TotalBurned.TotalTokens)
	fmt.Printf("   └─ Est. Cost:        $%14.2f\n", sm.TotalBurned.EstimatedCostUSD)

	fmt.Println("\n💾 TOKENS SAVED (optimization impact)")
	fmt.Printf("   Total Saved:         %15d\n", sm.TotalSaved.TotalTokensSaved)
	savingsPercent := sm.CalculateSavingsPercent()
	fmt.Printf("   Savings:             %14.1f%%\n", savingsPercent)
	fmt.Printf("   Est. Cost Saved:     $%14.2f\n", sm.TotalSaved.EstimatedCostSavedUSD)

	fmt.Println("\n📈 REQUEST STATISTICS")
	fmt.Printf("   Total Requests:      %15d\n", sm.TotalRequests)
	fmt.Printf("   Cache Hit Rate:      %14.1f%%\n", sm.AverageCacheHitRate)
	fmt.Printf("   False Positive Rate: %14.1f%%\n", sm.AvgFalsePositiveRate)

	fmt.Println("\n✅ Monthly Projection (7-day extrapolation)")
	proj7 := sm.ProjectMonthly(7)
	fmt.Printf("   Projected Tokens:    %15d\n", proj7.ProjectedTokensBurned)
	fmt.Printf("   Projected Savings:   %15d\n", proj7.ProjectedTokensSaved)
	fmt.Printf("   Projected Cost:      $%14.2f\n", proj7.ProjectedCostUSD)
	fmt.Printf("   Projected Savings:   $%14.2f\n", proj7.ProjectedSavingsUSD)
	fmt.Printf("   Confidence:          %14.1f%%\n", proj7.ProjectionConfidence*100)
}

func cmdDaily(days int, format string) {
	sm := metrics.NewSessionMetrics()

	if format == "json" {
		dailyData := make([]map[string]interface{}, 0)
		now := time.Now()
		for i := 0; i < days; i++ {
			date := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
			daily := sm.GetDailySummary(date)
			if daily == nil {
				continue
			}

			dailyData = append(dailyData, map[string]interface{}{
				"date": date.Format("2006-01-02"),
				"burned": map[string]interface{}{
					"input":  daily.Burned.InputTokens,
					"output": daily.Burned.OutputTokens,
					"total":  daily.Burned.TotalTokens,
				},
				"saved": map[string]interface{}{
					"total": daily.Saved.TotalTokensSaved,
					"cost":  daily.Saved.EstimatedCostSavedUSD,
				},
				"requests": daily.RequestCount,
			})
		}
		jsonBytes, _ := json.MarshalIndent(dailyData, "", "  ")
		fmt.Println(string(jsonBytes))
		return
	}

	fmt.Println("\n📅 DAILY BREAKDOWN (Last", days, "days)")
	fmt.Println(strings.Repeat("─", 90))

	w := tabwriter.NewWriter(os.Stdout, 12, 2, 2, ' ', 0)
	fmt.Fprintln(w, "Date\t Burned\t Saved\t Cache Hit\t Requests")
	fmt.Fprintln(w, strings.Repeat("─", 80))

	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		daily := sm.GetDailySummary(date)
		if daily == nil {
			continue
		}

		fmt.Fprintf(w, "%s\t %d\t %d\t %.1f%%\t %d\n",
			date.Format("2006-01-02"),
			daily.Burned.TotalTokens,
			daily.Saved.TotalTokensSaved,
			daily.CacheHitRate,
			daily.RequestCount)
	}
	w.Flush()
}

func cmdBreakdown(format string) {
	sm := metrics.NewSessionMetrics()

	breakdown := map[string]map[string]interface{}{
		"exact_dedup": {
			"tokens_saved": sm.TotalBreakdown.ExactDedup.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.ExactDedup.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.ExactDedup.HitCount,
			"savings_type": "100% (cache hit)",
		},
		"semantic_cache": {
			"tokens_saved": sm.TotalBreakdown.SemanticCache.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.SemanticCache.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.SemanticCache.HitCount,
			"savings_type": "98% (embedding cost deducted)",
		},
		"input_optimization": {
			"tokens_saved": sm.TotalBreakdown.InputOptimization.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.InputOptimization.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.InputOptimization.HitCount,
			"savings_type": "30-40% (compression)",
		},
		"output_optimization": {
			"tokens_saved": sm.TotalBreakdown.OutputOptimization.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.OutputOptimization.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.OutputOptimization.HitCount,
			"savings_type": "30-50% (compression)",
		},
		"rtk_proxy": {
			"tokens_saved": sm.TotalBreakdown.RTKProxy.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.RTKProxy.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.RTKProxy.HitCount,
			"savings_type": "99.4% (RTK proxy)",
		},
		"batch_api": {
			"tokens_saved": sm.TotalBreakdown.BatchAPI.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.BatchAPI.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.BatchAPI.HitCount,
			"savings_type": "50% (batch discount)",
		},
		"knowledge_graph": {
			"tokens_saved": sm.TotalBreakdown.KnowledgeGraph.TokensSaved,
			"cost_saved":   fmt.Sprintf("%.2f", sm.TotalBreakdown.KnowledgeGraph.CostSavedUSD),
			"hit_count":    sm.TotalBreakdown.KnowledgeGraph.HitCount,
			"savings_type": "99% (graph lookup)",
		},
	}

	if format == "json" {
		jsonBytes, _ := json.MarshalIndent(breakdown, "", "  ")
		fmt.Println(string(jsonBytes))
		return
	}

	fmt.Println("\n🔍 OPTIMIZATION BREAKDOWN")
	fmt.Println(strings.Repeat("─", 100))

	w := tabwriter.NewWriter(os.Stdout, 20, 2, 2, ' ', 0)
	fmt.Fprintln(w, "Layer\t Tokens Saved\t Cost Saved\t Hits\t Savings Type")
	fmt.Fprintln(w, strings.Repeat("─", 90))

	layers := []string{
		"exact_dedup",
		"semantic_cache",
		"input_optimization",
		"output_optimization",
		"rtk_proxy",
		"batch_api",
		"knowledge_graph",
	}

	for _, layer := range layers {
		data := breakdown[layer]
		fmt.Fprintf(w, "%s\t %v\t $%v\t %v\t %v\n",
			layer,
			data["tokens_saved"],
			data["cost_saved"],
			data["hit_count"],
			data["savings_type"])
	}
	w.Flush()
}

func cmdProjections(format string) {
	sm := metrics.NewSessionMetrics()

	proj7 := sm.ProjectMonthly(7)
	proj30 := sm.ProjectMonthly(30)

	if format == "json" {
		projections := map[string]interface{}{
			"7day": map[string]interface{}{
				"projected_tokens_burned":   proj7.ProjectedTokensBurned,
				"projected_tokens_saved":    proj7.ProjectedTokensSaved,
				"projected_cost":            proj7.ProjectedCostUSD,
				"projected_savings":         proj7.ProjectedSavingsUSD,
				"projected_savings_percent": proj7.ProjectedSavingsPercent,
				"confidence":                proj7.ProjectionConfidence,
			},
			"30day": map[string]interface{}{
				"projected_tokens_burned":   proj30.ProjectedTokensBurned,
				"projected_tokens_saved":    proj30.ProjectedTokensSaved,
				"projected_cost":            proj30.ProjectedCostUSD,
				"projected_savings":         proj30.ProjectedSavingsUSD,
				"projected_savings_percent": proj30.ProjectedSavingsPercent,
				"confidence":                proj30.ProjectionConfidence,
			},
		}
		jsonBytes, _ := json.MarshalIndent(projections, "", "  ")
		fmt.Println(string(jsonBytes))
		return
	}

	fmt.Println("\n📊 MONTHLY PROJECTIONS")
	fmt.Println(strings.Repeat("─", 80))

	fmt.Println("\n7-Day Extrapolation:")
	fmt.Printf("  Projected Tokens Burned:  %15d\n", proj7.ProjectedTokensBurned)
	fmt.Printf("  Projected Tokens Saved:   %15d\n", proj7.ProjectedTokensSaved)
	fmt.Printf("  Projected Cost:           $%14.2f\n", proj7.ProjectedCostUSD)
	fmt.Printf("  Projected Savings:        $%14.2f\n", proj7.ProjectedSavingsUSD)
	fmt.Printf("  Projected Savings %%:      %14.1f%%\n", proj7.ProjectedSavingsPercent)
	fmt.Printf("  Confidence:               %14.1f%%\n", proj7.ProjectionConfidence*100)

	fmt.Println("\n30-Day Extrapolation:")
	fmt.Printf("  Projected Tokens Burned:  %15d\n", proj30.ProjectedTokensBurned)
	fmt.Printf("  Projected Tokens Saved:   %15d\n", proj30.ProjectedTokensSaved)
	fmt.Printf("  Projected Cost:           $%14.2f\n", proj30.ProjectedCostUSD)
	fmt.Printf("  Projected Savings:        $%14.2f\n", proj30.ProjectedSavingsUSD)
	fmt.Printf("  Projected Savings %%:      %14.1f%%\n", proj30.ProjectedSavingsPercent)
	fmt.Printf("  Confidence:               %14.1f%%\n", proj30.ProjectionConfidence*100)
}

func cmdExport(format string) {
	sm := metrics.NewSessionMetrics()

	if format == "json" {
		jsonBytes, _ := json.MarshalIndent(sm.GetJSON(), "", "  ")
		fmt.Println(string(jsonBytes))
		return
	}

	if format == "csv" {
		csv := "Metric,Value,Unit\n"
		csv += fmt.Sprintf("Tokens Burned (Total),%d,tokens\n", sm.TotalBurned.TotalTokens)
		csv += fmt.Sprintf("Tokens Burned (Input),%d,tokens\n", sm.TotalBurned.InputTokens)
		csv += fmt.Sprintf("Tokens Burned (Output),%d,tokens\n", sm.TotalBurned.OutputTokens)
		csv += fmt.Sprintf("Tokens Saved (Total),%d,tokens\n", sm.TotalSaved.TotalTokensSaved)
		csv += fmt.Sprintf("Cost (Estimated),%.2f,USD\n", sm.TotalBurned.EstimatedCostUSD)
		csv += fmt.Sprintf("Savings (Estimated),%.2f,USD\n", sm.TotalSaved.EstimatedCostSavedUSD)
		csv += fmt.Sprintf("Savings Percentage,%.1f,%%\n", sm.CalculateSavingsPercent())
		csv += fmt.Sprintf("Total Requests,%d,count\n", sm.TotalRequests)
		csv += fmt.Sprintf("Cache Hit Rate,%.1f,%%\n", sm.AverageCacheHitRate)
		csv += fmt.Sprintf("False Positive Rate,%.1f,%%\n", sm.AvgFalsePositiveRate)
		fmt.Println(csv)
		return
	}

	fmt.Println("Export format not recognized. Use: json, csv")
}

func cmdStatus() {
	sm := metrics.NewSessionMetrics()

	fmt.Printf("Claude Escalate Metrics Status\n")
	fmt.Printf("─────────────────────────────────────────\n")
	fmt.Printf("Total Tokens Burned:  %d\n", sm.TotalBurned.TotalTokens)
	fmt.Printf("Total Tokens Saved:   %d\n", sm.TotalSaved.TotalTokensSaved)
	fmt.Printf("Savings Percentage:   %.1f%%\n", sm.CalculateSavingsPercent())
	fmt.Printf("Total Cost:           $%.2f\n", sm.TotalBurned.EstimatedCostUSD)
	fmt.Printf("Cost Saved:           $%.2f\n", sm.TotalSaved.EstimatedCostSavedUSD)
	fmt.Printf("Requests:             %d\n", sm.TotalRequests)
}

func printUsage() {
	fmt.Println(`
Claude Escalate Metrics CLI - View token tracking and savings

USAGE:
  claude-escalate-metrics <command> [flags]

COMMANDS:
  overview          Show high-level metrics summary
  daily             Show daily breakdown (default: 7 days)
  breakdown         Show per-optimization-layer breakdown
  projections       Show monthly projections
  export            Export full metrics (JSON/CSV)
  status            Quick status line
  help              Show this help message

FLAGS:
  -format string    Output format: text, json, csv (default: text)
  -days int         Number of days for daily breakdown (default: 7)

EXAMPLES:
  # Overview with default formatting
  claude-escalate-metrics overview

  # Daily breakdown in JSON
  claude-escalate-metrics daily -format json -days 30

  # Export full metrics as CSV
  claude-escalate-metrics export -format csv

  # Quick status
  claude-escalate-metrics status
`)
}
