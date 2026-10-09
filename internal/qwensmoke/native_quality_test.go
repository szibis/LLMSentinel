package qwensmoke

import (
	"bytes"
	"context"
	"github.com/szibis/claude-escalate/internal/taskquality"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeQualityRejectsMalformedStrictGateBeforeWork(t *testing.T) {
	for _, value := range []string{"TRUE", "False", "1", "yes", " true ", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SENTINEL_EXTENDED_QUALITY_REQUIRED", value)
			t.Setenv("SENTINEL_CI_CLIENT_ROOT", "")
			t.Setenv("SENTINEL_CI_LAB_ROOT", "")
			artifacts := filepath.Join(t.TempDir(), "not-created")
			var out bytes.Buffer
			err := runNativeQuality(context.Background(), artifacts, document{}, &out)
			if err == nil || !strings.Contains(err.Error(), "extended quality requirement") {
				t.Fatalf("malformed strict gate not rejected before client work: %v", err)
			}
			if out.Len() != 0 {
				t.Fatal("client work emitted output")
			}
			if _, err := os.Stat(artifacts); !os.IsNotExist(err) {
				t.Fatal("artifact work started", err)
			}
		})
	}
}

func TestStrictQualityFlagParsing(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"true", true}} {
		got, err := parseExtendedQualityRequirement(tc.value)
		if err != nil || got != tc.want {
			t.Fatal(tc.value, got, err)
		}
	}
}

func TestSmokeRejectsMalformedStrictFlagBeforeRuntimeWork(t *testing.T) {
	t.Setenv("SENTINEL_EXTENDED_QUALITY_REQUIRED", "TRUE")
	artifacts := filepath.Join(t.TempDir(), "not-created")
	var out, diagnostic bytes.Buffer
	code := Run([]string{"--gateway", "synthetic-unused-gateway", "--integration-proofs", "--native-quality", "--artifacts", artifacts}, nil, &out, &diagnostic)
	if code != 2 || !strings.Contains(diagnostic.String(), "extended quality requirement") {
		t.Fatal(code, diagnostic.String())
	}
	if _, err := os.Stat(artifacts); !os.IsNotExist(err) {
		t.Fatal("runtime artifact work started", err)
	}
}

func nativeFixtureReport() *taskquality.Report {
	now := time.Now().UTC()
	report := &taskquality.Report{Version: 1, FixtureVersion: 2, Suite: "extended", Scope: "real-cli-task-probes", CorpusSHA256: strings.Repeat("a", 64), StartedAt: now.Add(-time.Minute), FinishedAt: now}
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		for _, client := range []string{"claude", "codex"} {
			for _, task := range []string{"exact-read", "coding-fix", "loki-evidence", "planning", "untrusted-evidence", "literal-markers", "long-context", "tool-recovery"} {
				zero := 0
				report.Results = append(report.Results, taskquality.Result{Task: task, Role: role, Client: client, Passed: true, ClientVersion: "synthetic gate unit fixture", CLIExitCode: &zero, Checks: map[string]bool{"fixture_read": true, "protocol_valid": true, "final_assertion": true, "independent_tests": true, "client_test_command": true, "edit_and_tests_preserved": true, "failed_read_before_success": true}})
			}
		}
	}
	return report
}

func TestNativeGateCannotPromoteClaimedPassWithoutIndependentChecks(t *testing.T) {
	for _, kind := range []string{"checks", "corpus", "scope", "timestamp", "exit", "client-version"} {
		report := nativeFixtureReport()
		switch kind {
		case "checks":
			report.Results[0].Checks = nil
		case "corpus":
			report.CorpusSHA256 = ""
		case "scope":
			report.Scope = "other"
		case "timestamp":
			report.FinishedAt = report.StartedAt.Add(-time.Second)
		case "exit":
			report.Results[0].CLIExitCode = nil
		case "client-version":
			report.Results[0].ClientVersion = ""
		}
		if _, err := nativeQualityGate(report, false); err == nil {
			t.Fatal("unsupported claim promoted", kind)
		}
	}
}

func TestNativeInvocationDoesNotReusePriorEvidenceOnCancellation(t *testing.T) {
	artifacts, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(artifacts, "native-quality")
	if err = os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err = taskquality.Save(old, *nativeFixtureReport()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_CI_CLIENT_ROOT", artifacts)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = runNativeQuality(ctx, artifacts, document{}, io.Discard); err == nil {
		t.Fatal("prior passing report hid canceled invocation")
	}
}
func TestNativeQualityGateRequiresCompleteEvidenceAndBaseline(t *testing.T) {
	report := nativeFixtureReport()
	if _, err := nativeQualityGate(report, false); err != nil {
		t.Fatal(err)
	}
	report.Results[5].Passed = false
	if summary, err := nativeQualityGate(report, false); err != nil || summary["passed"] != 47 {
		t.Fatal(summary, err)
	}
	if _, err := nativeQualityGate(report, true); err == nil {
		t.Fatal("required extended failure hidden")
	}
	report.Results[0].Passed = false
	if _, err := nativeQualityGate(report, false); err == nil {
		t.Fatal("baseline regression hidden")
	}
	report = nativeFixtureReport()
	report.Results = report.Results[:47]
	if _, err := nativeQualityGate(report, false); err == nil {
		t.Fatal("missing probe hidden")
	}
	report = nativeFixtureReport()
	report.Results[1] = report.Results[0]
	if _, err := nativeQualityGate(report, false); err == nil {
		t.Fatal("duplicate probe hidden")
	}
	if _, err := nativeQualityGate(nil, false); err == nil {
		t.Fatal("nil evidence")
	}
}
