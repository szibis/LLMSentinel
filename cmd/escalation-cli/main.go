package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/szibis/claude-escalate/internal/cli"
	"github.com/szibis/claude-escalate/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "set-budget":
		handleSetBudget(args)
	case "config":
		handleConfig(args)
	case "dashboard":
		handleDashboard(args)
	case "monitor":
		handleMonitor(args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func handleSetBudget(args []string) {
	fs := flag.NewFlagSet("set-budget", flag.ExitOnError)
	daily := fs.Float64("daily", 0, "Daily budget in USD (e.g., 10.00)")
	monthly := fs.Float64("monthly", 0, "Monthly budget in USD (e.g., 100.00)")
	sessionTokens := fs.Int("session", 0, "Session budget in tokens (e.g., 10000)")
	fs.Parse(args)

	cfg, err := config.LoadEscalationConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	updated := false
	if *daily > 0 {
		cfg.Budgets.DailyUSD = *daily
		updated = true
	}
	if *monthly > 0 {
		cfg.Budgets.MonthlyUSD = *monthly
		updated = true
	}
	if *sessionTokens > 0 {
		cfg.Budgets.SessionTokens = *sessionTokens
		updated = true
	}

	if !updated {
		fmt.Println("No budget parameters provided. Usage:")
		fmt.Println("  escalation-manager set-budget --daily 10.00")
		fmt.Println("  escalation-manager set-budget --monthly 100.00")
		fmt.Println("  escalation-manager set-budget --session 10000")
		os.Exit(1)
	}

	if err := config.SaveEscalationConfig(cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Budget configuration updated:")
	if *daily > 0 {
		fmt.Printf("   Daily:   $%.2f\n", *daily)
	}
	if *monthly > 0 {
		fmt.Printf("   Monthly: $%.2f\n", *monthly)
	}
	if *sessionTokens > 0 {
		fmt.Printf("   Session: %d tokens\n", *sessionTokens)
	}
}

func handleConfig(args []string) {
	if len(args) == 0 {
		// Show current config
		cfg, err := config.LoadEscalationConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                    ESCALATION CONFIGURATION                    ║")
		fmt.Println("╚════════════════════════════════════════════════════════════════╝")
		fmt.Println()

		fmt.Println("Budgets:")
		fmt.Printf("  Daily:   $%.2f\n", cfg.Budgets.DailyUSD)
		fmt.Printf("  Monthly: $%.2f\n", cfg.Budgets.MonthlyUSD)
		fmt.Printf("  Session: %d tokens\n", cfg.Budgets.SessionTokens)
		fmt.Printf("  Hard Limit: %v, Soft Limit: %v\n", cfg.Budgets.HardLimit, cfg.Budgets.SoftLimit)

		fmt.Println("\nSentiment Detection:")
		fmt.Printf("  Enabled: %v\n", cfg.Sentiment.Enabled)
		fmt.Printf("  Frustration Escalation: %v\n", cfg.Sentiment.FrustrationTriggerEscalate)
		fmt.Printf("  Frustration Threshold: %.2f\n", cfg.Sentiment.FrustrationRiskThreshold)
		fmt.Printf("  Learning Enabled: %v\n", cfg.Sentiment.LearningEnabled)

		fmt.Println("\nStatusline Sources:")
		for _, src := range cfg.Statusline.Sources {
			status := "enabled"
			if !src.Enabled {
				status = "disabled"
			}
			fmt.Printf("  %s [%s]\n", src.Type, status)
		}

		fmt.Println()
		return
	}

	// Parse set operations: config set sentiment.enabled true
	if len(args) >= 3 && args[0] == "set" {
		key := args[1]
		value := strings.Join(args[2:], " ")

		cfg, err := config.LoadEscalationConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Parse and set value
		setConfigValue(cfg, key, value)

		if err := config.SaveEscalationConfig(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Config updated: %s = %s\n", key, value)
		return
	}

	fmt.Println("Config usage:")
	fmt.Println("  escalation-manager config                           # Show current config")
	fmt.Println("  escalation-manager config set <key> <value>         # Set config value")
	fmt.Println("\nExamples:")
	fmt.Println("  escalation-manager config set sentiment.enabled true")
	fmt.Println("  escalation-manager config set budgets.daily_usd 10.0")
}

func setConfigValue(cfg *config.EscalationConfig, key, value string) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		fmt.Printf("Invalid key format: %s (use section.key)\n", key)
		return
	}

	section := parts[0]
	subkey := parts[1]

	switch section {
	case "sentiment":
		switch subkey {
		case "enabled":
			cfg.Sentiment.Enabled = value == "true"
		case "frustration_trigger_escalate":
			cfg.Sentiment.FrustrationTriggerEscalate = value == "true"
		}
	case "budgets":
		switch subkey {
		case "daily_usd":
			fmt.Sscanf(value, "%f", &cfg.Budgets.DailyUSD)
		case "monthly_usd":
			fmt.Sscanf(value, "%f", &cfg.Budgets.MonthlyUSD)
		case "hard_limit":
			cfg.Budgets.HardLimit = value == "true"
		case "soft_limit":
			cfg.Budgets.SoftLimit = value == "true"
		}
	}
}

func handleDashboard(args []string) {
	serverURL := "http://localhost:9000"
	if len(args) > 0 && args[0] == "--server" && len(args) > 1 {
		serverURL = args[1]
	}

	dashboard := cli.NewDashboardCLI(serverURL)

	// Parse view type
	if len(args) > 0 {
		switch args[0] {
		case "--sentiment":
			if err := dashboard.SentimentDashboard(); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--budget":
			if err := dashboard.BudgetDashboard(); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--optimization":
			if err := dashboard.CostOptimizationDashboard(); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// Show all views
	if err := dashboard.FullDashboard(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func handleMonitor(args []string) {
	fmt.Println("Monitor mode: Starting token metrics daemon...")
	fmt.Println("(This would connect to barista/statusline and stream metrics)")
	fmt.Println("Feature coming in Phase 7.2")
}

func printUsage() {
	fmt.Print(`
escalation-manager - Claude model escalation and token budget management

USAGE:
  escalation-manager <command> [options]

COMMANDS:
  set-budget        Configure token budgets
    --daily AMOUNT    Set daily budget (e.g., 10.00)
    --monthly AMOUNT  Set monthly budget (e.g., 100.00)
    --session TOKENS  Set session budget (e.g., 10000)

  config            View or update configuration
    (no args)         Show current configuration
    set <key> <val>   Set configuration value
                      Example: config set sentiment.enabled true

  dashboard         Display analytics dashboards
    (no args)         Show all views (sentiment, budget, optimization)
    --sentiment       Show sentiment trends only
    --budget          Show budget status only
    --optimization    Show cost optimization only
    --server URL      Connect to custom server (default: http://localhost:9000)

  monitor           Start token metrics daemon
    Continuously monitor and report token metrics

EXAMPLES:
  escalation-manager set-budget --daily 10.00 --monthly 100.00
  escalation-manager config
  escalation-manager config set sentiment.enabled true
  escalation-manager dashboard --sentiment
  escalation-manager dashboard --budget
  escalation-manager dashboard --optimization

For help: escalation-manager help
`)
}
