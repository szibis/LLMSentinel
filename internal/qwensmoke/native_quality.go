package qwensmoke

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/szibis/claude-escalate/internal/jes"
	"github.com/szibis/claude-escalate/internal/labbench"
	"github.com/szibis/claude-escalate/internal/taskquality"
)

func nativeQualityGate(report *taskquality.Report, requireExtended bool) (document, error) {
	summary := document{"scope": "synthetic-native-cli-quality", "extended_required": requireExtended}
	if report == nil || report.Version != 1 || report.FixtureVersion != 2 || report.Suite != "extended" || report.Scope != "real-cli-task-probes" || report.StartedAt.IsZero() || report.FinishedAt.Before(report.StartedAt) || len(report.Results) != 48 {
		return summary, errors.New("native quality evidence incomplete or unsupported")
	}
	corpus, err := hex.DecodeString(report.CorpusSHA256)
	if err != nil || len(corpus) != 32 {
		return summary, errors.New("native corpus identity missing")
	}
	tasks := map[string]bool{"exact-read": true, "coding-fix": true, "loki-evidence": true, "planning": true, "untrusted-evidence": false, "literal-markers": false, "long-context": false, "tool-recovery": false}
	seen := map[string]bool{}
	passed, baseline := 0, 0
	for _, result := range report.Results {
		base, known := tasks[result.Task]
		key := result.Role + "/" + result.Client + "/" + result.Task
		if !known || seen[key] || (result.Role != "haiku" && result.Role != "sonnet" && result.Role != "opus") || (result.Client != "claude" && result.Client != "codex") {
			return summary, errors.New("duplicate or unknown native quality case")
		}
		seen[key] = true
		if result.Passed {
			if result.CLIExitCode == nil || *result.CLIExitCode != 0 || result.ClientVersion == "" || !result.Checks["fixture_read"] || !result.Checks["final_assertion"] || !result.Checks["protocol_valid"] {
				return summary, errors.New("native pass lacks independently measured execution checks")
			}
			if result.Task == "coding-fix" && (!result.Checks["client_test_command"] || !result.Checks["independent_tests"] || !result.Checks["edit_and_tests_preserved"]) {
				return summary, errors.New("native coding pass lacks held-out verification")
			}
			if result.Task == "tool-recovery" && !result.Checks["failed_read_before_success"] {
				return summary, errors.New("native recovery pass lacks executed failure evidence")
			}
			passed++
			if base {
				baseline++
			}
		}
	}
	summary["passed"] = passed
	summary["total"] = 48
	summary["baseline_passed"] = baseline
	summary["baseline_total"] = 24
	summary["route_promotion"] = map[bool]string{true: "eligible for further review", false: "blocked by extended fixture failures"}[passed == 48]
	if baseline != 24 {
		return summary, errors.New("native baseline regression")
	}
	if requireExtended && passed != 48 {
		return summary, errors.New("required extended native quality failure")
	}
	return summary, nil
}

func parseExtendedQualityRequirement(value string) (bool, error) {
	switch value {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errors.New("extended quality requirement must be true, false, or unset")
	}
}

func runNativeQuality(ctx context.Context, artifacts string, result document, out io.Writer) error {
	requireExtended, err := parseExtendedQualityRequirement(os.Getenv("SENTINEL_EXTENDED_QUALITY_REQUIRED"))
	if err != nil {
		return err
	}
	clients := os.Getenv("SENTINEL_CI_CLIENT_ROOT")
	if clients == "" {
		if labRoot := os.Getenv("SENTINEL_CI_LAB_ROOT"); labRoot != "" {
			clients = filepath.Join(labRoot, "clients")
		}
	}
	if clients == "" {
		return errors.New("native CI requires configured isolated installed Claude Code and Codex clients")
	}
	absolute, err := filepath.Abs(artifacts)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return err
	}
	root, err := os.MkdirTemp(absolute, "native-quality-")
	if err != nil {
		return err
	}
	var diagnostic bytes.Buffer
	code := taskquality.RunCLIContext(ctx, []string{"--root", root, "--clients-root", clients, "--endpoint", gatewayURL, "--role", "all", "--client", "both", "--suite", "extended", "--timeout", "3m"}, nil, out, &diagnostic)
	report := taskquality.LoadCLI(root)
	summary, gateErr := nativeQualityGate(report, requireExtended)
	result["native_cli_quality"] = summary
	summary["evidence_directory"] = filepath.Base(root)
	if code == 2 || report == nil {
		return fmt.Errorf("native CI unavailable: %s", diagnostic.String())
	}
	if scoreCode := jes.Run([]string{"--task-report", filepath.Join(root, "task-cli-quality-latest.json"), "--source-revision", os.Getenv("SENTINEL_CI_SOURCE_SHA"), "--output", filepath.Join(root, "jes-quality-latest.json")}, nil, io.Discard, &diagnostic); scoreCode != 0 {
		return fmt.Errorf("native advisory evidence failed: %s", diagnostic.String())
	}
	if gateErr != nil {
		return gateErr
	}
	// Failed extended cases remain in the complete synthetic archive. Baseline
	// regressions fail CI; all extended cases must pass before route promotion.
	if code != 0 {
		fmt.Fprintln(out, "Extended quality failures retained; affected route promotion blocked.")
	}
	if code = labbench.RunContext(ctx, []string{"--endpoint", gatewayURL, "--stats-endpoint", runtimeURL + "/status", "--roles", "sonnet", "--pairs", "3", "--output", filepath.Join(artifacts, "benchmark-latest.json")}, nil, out, &diagnostic); code != 0 {
		return fmt.Errorf("native benchmark failed: %s", diagnostic.String())
	}
	return nil
}
