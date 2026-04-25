// claude-escalate is an intelligent model escalation system for Claude Code.
//
// It runs as a Claude Code UserPromptSubmit hook, analyzing each prompt to:
//   - Detect when the current model is stuck (frustration signals, circular reasoning)
//   - Suggest or perform model escalation (Haiku → Sonnet → Opus)
//   - Auto-downgrade when problems are solved (Opus → Sonnet → Haiku)
//   - Learn which task types need escalation and predict routing
//   - Serve a local dashboard for analytics and monitoring
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/szibis/claude-escalate/internal/classify"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/dashboard"
	"github.com/szibis/claude-escalate/internal/detect"
	"github.com/szibis/claude-escalate/internal/hook"
	"github.com/szibis/claude-escalate/internal/service"
	"github.com/szibis/claude-escalate/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "service":
		runService()
	case "hook":
		runHook()
	case "dashboard":
		runDashboard()
	case "stats":
		runStats()
	case "install-hook":
		runInstallHook()
	case "version":
		fmt.Printf("claude-escalate %s\n", config.Version)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `claude-escalate %s — Intelligent model escalation for Claude Code

Usage:
  claude-escalate hook            Run as Claude Code UserPromptSubmit hook
  claude-escalate dashboard       Start the local web dashboard
  claude-escalate stats           Show escalation statistics
  claude-escalate install-hook    Configure Claude Code to use this hook
  claude-escalate version         Show version

Dashboard flags:
  --port PORT    Dashboard port (default: 8077)

Stats subcommands:
  stats summary      Overall statistics
  stats types        Breakdown by task type
  stats predictions  Predictive escalation status
  stats history      Recent escalation events
  stats reset        Clear all data
`, config.Version)
}

// runHook is the main Claude Code hook entry point.
// Reads JSON from stdin, processes the prompt, writes JSON to stdout.
func runHook() {
	input, err := hook.ReadInput()
	if err != nil || input.Prompt == "" {
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	cfg := config.DefaultConfig()

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}
	defer func() { _ = db.Close() }()

	settings, err := config.ReadClaudeSettings()
	if err != nil {
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	prompt := input.Prompt
	currentModel := settings.Model
	modelShort := config.ModelShortName(currentModel)

	// Skip meta-commands
	if detect.IsMetaCommand(prompt) {
		// Handle /escalate
		if isEsc, target := detect.IsEscalateCommand(prompt); isEsc {
			handleEscalate(db, currentModel, target)
			return
		}
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	// Classify task type and store context
	taskType := classify.Classify(prompt)

	// Log turn for circular reasoning detection
	concepts := detect.ExtractConcepts(prompt)
	_ = db.LogTurn(modelShort, strings.Join(concepts, ","))

	// Phase 5: Predictive escalation (only on Haiku)
	if config.ModelTierOf(currentModel) == config.TierHaiku && taskType != classify.TaskGeneral {
		count, _ := db.EscalationCountForType(string(taskType))
		if count >= cfg.PredictThreshold {
			_ = hook.WriteOutput(hook.WithHint(
				fmt.Sprintf("📊 Predictive: %s tasks historically need escalation (%d prior). Consider: /escalate to sonnet", taskType, count),
			))
			return
		}
	}

	// Phase 2: Frustration detection
	if detect.DetectFrustration(prompt) {
		attempts, _ := db.CountRecentAttempts(modelShort, 5)
		if attempts >= cfg.FrustrationRetries {
			suggestTarget := "sonnet"
			if modelShort == "sonnet" {
				suggestTarget = "opus"
			}
			if modelShort != "opus" {
				_ = hook.WriteOutput(hook.WithHint(
					fmt.Sprintf("💡 %s seems stuck (%d attempts). Try: /escalate to %s",
						capitalize(modelShort), attempts, suggestTarget),
				))
				return
			}
		}
	}

	// Phase 4: Circular reasoning detection (only on Haiku)
	if config.ModelTierOf(currentModel) == config.TierHaiku {
		turns, _ := db.RecentTurns(6)
		if len(turns) >= cfg.CircularTurns {
			var recentConcepts [][]string
			haikuCount := 0
			for _, t := range turns {
				if t.Model == "haiku" {
					haikuCount++
				}
				if t.Concepts != "" {
					recentConcepts = append(recentConcepts, strings.Split(t.Concepts, ","))
				}
			}
			if haikuCount >= 3 && detect.DetectCircularPattern(recentConcepts, cfg.CircularTurns) {
				_ = hook.WriteOutput(hook.WithHint(
					"🔄 Circular pattern detected (same concepts repeating). Consider: /escalate to sonnet",
				))
				return
			}
		}
	}

	// Phase 3: De-escalation on success signal
	if detect.DetectSuccess(prompt) && config.ModelTierOf(currentModel) > config.TierHaiku {
		handleDeEscalate(db, currentModel, string(taskType))
		return
	}

	_ = hook.WriteOutput(hook.PassThrough())
}

func handleEscalate(db *store.Store, currentModel, target string) {
	var modelID, label, effort string
	switch target {
	case "opus":
		modelID = config.ModelOpus
		label = "Opus (deep reasoning)"
		effort = "high"
	case "sonnet":
		modelID = config.ModelSonnet
		label = "Sonnet (precision code)"
		effort = "high"
	case "haiku":
		modelID = config.ModelHaiku
		label = "Haiku (cost-optimized)"
		effort = "low"
	default:
		modelID = config.ModelSonnet
		label = "Sonnet (default)"
		effort = "high"
	}

	if err := config.WriteClaudeSettings(modelID, effort); err != nil {
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	_ = db.LogEscalation(config.ModelShortName(currentModel), target, "general", "user_command")
	_ = db.SetSession("escalation_active", "true")

	_ = hook.WriteOutput(hook.WithHint(fmt.Sprintf("🚀 Escalated: %s", label)))
}

func handleDeEscalate(db *store.Store, currentModel, taskType string) {
	// Check if there's escalation context
	active, _ := db.GetSession("escalation_active")
	if active != "true" {
		// Check if we've been on expensive model for 2+ turns
		attempts, _ := db.CountRecentAttempts(config.ModelShortName(currentModel), 3)
		if attempts < 2 {
			_ = hook.WriteOutput(hook.PassThrough())
			return
		}
	}

	// Step down one tier
	var targetModel, label, effort string
	switch {
	case strings.Contains(currentModel, "opus"):
		targetModel = config.ModelSonnet
		label = "Sonnet (balanced)"
		effort = "medium"
		// Keep session for cascade
		_ = db.SetSession("escalation_active", "true")
	case strings.Contains(currentModel, "sonnet"):
		targetModel = config.ModelHaiku
		label = "Haiku (cost-optimized)"
		effort = "low"
		_ = db.DeleteSession("escalation_active")
	default:
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	if err := config.WriteClaudeSettings(targetModel, effort); err != nil {
		_ = hook.WriteOutput(hook.PassThrough())
		return
	}

	_ = db.LogEscalation(config.ModelShortName(currentModel), config.ModelShortName(targetModel), taskType, "success")
	_ = hook.WriteOutput(hook.WithHint(fmt.Sprintf("⬇️ Auto-downgrade: %s (problem solved, saving cost)", label)))
}

func runDashboard() {
	port := 8077
	bind := ""
	for i, arg := range os.Args {
		if arg == "--port" && i+1 < len(os.Args) {
			_, _ = fmt.Sscanf(os.Args[i+1], "%d", &port)
		}
		if arg == "--bind" && i+1 < len(os.Args) {
			bind = os.Args[i+1]
		}
	}
	// Environment variable override (for Docker)
	if v := os.Getenv("ESCALATE_BIND"); v != "" {
		bind = v
	}
	if v := os.Getenv("ESCALATE_DATA_DIR"); v != "" {
		// handled below via cfg
		_ = v
	}

	cfg := config.DefaultConfig()
	cfg.DashboardPort = port
	cfg.DashboardBind = bind
	if v := os.Getenv("ESCALATE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	if err := dashboard.Serve(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Dashboard error: %v\n", err)
		os.Exit(1)
	}
}

func runStats() {
	subcmd := "summary"
	if len(os.Args) >= 3 {
		subcmd = os.Args[2]
	}

	cfg := config.DefaultConfig()
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	switch subcmd {
	case "summary":
		printStatsSummary(db)
	case "types":
		printStatsTypes(db)
	case "predictions":
		printStatsPredictions(db, cfg)
	case "history":
		printStatsHistory(db)
	case "reset":
		fmt.Println("This will delete all escalation history. Use --confirm to proceed.")
	default:
		fmt.Fprintf(os.Stderr, "Unknown stats subcommand: %s\n", subcmd)
	}
}

func printStatsSummary(db *store.Store) {
	esc, deesc, turns, _ := db.TotalStats()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  claude-escalate Statistics")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("\n  Escalations:     %d\n", esc)
	fmt.Printf("  De-escalations:  %d\n", deesc)
	fmt.Printf("  Turns tracked:   %d\n", turns)
	if esc > 0 {
		fmt.Printf("  Success rate:    %.0f%%\n", float64(deesc)/float64(esc)*100)
	}
	fmt.Println()
}

func printStatsTypes(db *store.Store) {
	stats, _ := db.TaskTypeStatsAll()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Escalation by Task Type")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  Task Type        Escalations  Successes  Rate")
	fmt.Println("  ──────────────── ─────────── ───────── ─────")
	for _, st := range stats {
		fmt.Printf("  %-18s %5d       %5d      %3.0f%%\n",
			st.TaskType, st.Escalations, st.Successes, st.SuccessRate)
	}
	fmt.Println()
}

func printStatsPredictions(db *store.Store, cfg *config.Config) {
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Predictive Escalation Status")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	found := false
	for _, tt := range classify.AllTaskTypes() {
		count, _ := db.EscalationCountForType(string(tt))
		if count >= cfg.PredictThreshold {
			fmt.Printf("  ⚡ %s — %d escalations → will suggest proactively\n", tt, count)
			found = true
		} else if count >= 3 {
			fmt.Printf("  📊 %s — %d escalations → approaching threshold (%d)\n", tt, count, cfg.PredictThreshold)
			found = true
		}
	}
	if !found {
		fmt.Printf("  No task types have enough data yet (threshold: %d)\n", cfg.PredictThreshold)
	}
	fmt.Println()
}

func printStatsHistory(db *store.Store) {
	events, _ := db.RecentEscalations(20)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Recent Escalation History")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  Time               From     → To       Task Type      Reason")
	fmt.Println("  ────────────────── ──────── ───────── ─────────────── ──────────")
	for _, e := range events {
		fmt.Printf("  %-18s %-8s → %-8s %-15s %s\n",
			e.Timestamp.Format("01/02 15:04"),
			e.FromModel, e.ToModel, e.TaskType, e.Reason)
	}
	fmt.Println()
}

func runInstallHook() {
	fmt.Println("claude-escalate install-hook")
	fmt.Println()
	fmt.Println("Add this to your ~/.claude/settings.json under hooks.UserPromptSubmit:")
	fmt.Println()
	fmt.Println(`{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "claude-escalate hook",
            "timeout": 5
          }
        ]
      }
    ]
  }
}`)
	fmt.Println()
	fmt.Println("This single hook replaces all 6 bash scripts (detect, escalate, de-escalate, analyze, track, auto-effort).")
}

func runService() {
	cfg := config.DefaultConfig()

	// Parse flags
	port := "9000"
	for i, arg := range os.Args[2:] {
		if arg == "--port" && i+1 < len(os.Args)-2 {
			port = os.Args[i+3]
		}
	}

	svc, err := service.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
		os.Exit(1)
	}

	addr := "127.0.0.1:" + port
	if err := svc.Start(addr); err != nil {
		fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
		os.Exit(1)
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
