// Package claudemod provides finite native snapshots for the Claude Code mod.
package claudemod

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/labstatus"
	"github.com/szibis/claude-escalate/internal/taskquality"
)

const defaultEndpoint = "http://127.0.0.1:19090"

type snapshot struct {
	Scope            string         `json:"scope"`
	SampledAt        time.Time      `json:"sampled_at"`
	Endpoint         string         `json:"endpoint"`
	Control          map[string]any `json:"control"`
	Telemetry        map[string]any `json:"telemetry"`
	CLIQuality       *quality       `json:"cli_quality"`
	TrainingEligible bool           `json:"training_eligible"`
	Promotion        string         `json:"promotion"`
}

type quality struct {
	Suite        string       `json:"suite"`
	FinishedAt   time.Time    `json:"finished_at"`
	CorpusSHA256 string       `json:"corpus_sha256"`
	Passed       int          `json:"passed"`
	Total        int          `json:"total"`
	FailedCases  []failedCase `json:"failed_cases"`
}

type failedCase struct {
	Task         string   `json:"task"`
	Role         string   `json:"role"`
	Client       string   `json:"client"`
	FailedChecks []string `json:"failed_checks"`
}

// component preserves native output, including nulls and extension fields.
// Diagnostics stay private and unavailable components remain explicitly nil.
func component(run func([]string, io.Reader, io.Writer, io.Writer) int, args []string) map[string]any {
	var out bytes.Buffer
	if run(args, nil, &out, io.Discard) != 0 {
		return nil
	}
	var result map[string]any
	decoder := json.NewDecoder(&out)
	decoder.UseNumber()
	if decoder.Decode(&result) != nil || result == nil {
		return nil
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil
	}
	return result
}

// summarize deliberately copies only report metadata and supported check names.
// Evidence, answers, paths and diagnostics never enter the mod snapshot.
func summarize(report *taskquality.Report) *quality {
	if report == nil {
		return nil
	}
	result := &quality{
		Suite: report.Suite, FinishedAt: report.FinishedAt,
		CorpusSHA256: report.CorpusSHA256, Total: len(report.Results),
		FailedCases: []failedCase{},
	}
	for _, item := range report.Results {
		if item.Passed {
			result.Passed++
			continue
		}
		failed := failedCase{Task: item.Task, Role: item.Role, Client: item.Client, FailedChecks: []string{}}
		for check, passed := range item.Checks {
			if passed {
				continue
			}
			switch check {
			case "protocol_valid", "fixture_read", "final_assertion", "client_test_command", "edit_and_tests_preserved", "independent_tests":
				failed.FailedChecks = append(failed.FailedChecks, check)
			case "failed_read_before_success":
				if item.Task == "tool-recovery" {
					failed.FailedChecks = append(failed.FailedChecks, check)
				}
			}
		}
		sort.Strings(failed.FailedChecks)
		result.FailedCases = append(result.FailedCases, failed)
	}
	return result
}

// Run implements one finite status snapshot. Flags precede the status action.
// It calls only existing local control/telemetry helpers and reads CLI evidence;
// it never starts a service, invokes a model or authorizes training promotion.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("mod", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "canonical absolute lab root")
	endpoint := flags.String("endpoint", defaultEndpoint, "literal HTTP loopback origin")
	if flags.Parse(args) != nil || flags.NArg() != 1 || flags.Arg(0) != "status" {
		fmt.Fprintln(stderr, "Invalid mod options; use mod --root ROOT --endpoint URL status")
		return 2
	}
	if clientcontrol.ValidateDirectory(*root) != nil {
		fmt.Fprintln(stderr, "Invalid mod root; use a canonical absolute directory without symlinks")
		return 2
	}
	if _, err := clientcontrol.Assets("claude", *endpoint, "/canonical/executable"); err != nil {
		fmt.Fprintln(stderr, "Invalid mod endpoint; use a literal HTTP loopback origin")
		return 2
	}
	status := snapshot{
		Scope: "claude-mod-status", SampledAt: time.Now().UTC(), Endpoint: *endpoint,
		Control:          component(clientcontrol.Run, []string{"--endpoint", *endpoint, "status"}),
		CLIQuality:       summarize(taskquality.LoadCLI(*root)),
		TrainingEligible: false, Promotion: "not_authorized",
	}
	// labstatus describes the fixed default gateway and its configured runtimes.
	// A custom controller must never borrow telemetry from that unrelated lab.
	if *endpoint == defaultEndpoint {
		status.Telemetry = component(labstatus.Run, []string{"--root", *root, "--json"})
	}
	if json.NewEncoder(out).Encode(status) != nil {
		fmt.Fprintln(stderr, "Mod status output failed")
		return 1
	}
	return 0
}
