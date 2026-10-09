package evidence

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/szibis/claude-escalate/internal/taskquality"
)

const MaxRegressionEventBytes int64 = 128 << 20
const maxRegressionTests = 20000

var regressionPackages = []string{
	"github.com/szibis/claude-escalate/internal/localgateway",
	"github.com/szibis/claude-escalate/internal/clientcapture",
	"github.com/szibis/claude-escalate/internal/clientcontrol",
	"github.com/szibis/claude-escalate/internal/claudemod",
	"github.com/szibis/claude-escalate/internal/taskquality",
	"github.com/szibis/claude-escalate/internal/labstatus",
}

var regressionChecks = []struct {
	name         string
	packageIndex int
	tests        []string
}{
	{"messages", 0, []string{"TestClaudeToolRoundTripAndStream"}},
	{"responses", 0, []string{"TestResponsesTextProtocol", "TestResponsesValidatedToolStreamAndHistory"}},
	{"chat-completions", 0, []string{"TestChatCompletionsFunctionRoundTrip", "TestChatStreamingUsesCompletionChunks"}},
	{"capture", 1, nil}, {"controls", 2, nil}, {"claude-mod", 3, nil},
	{"quality-harness", 4, nil}, {"telemetry", 5, nil},
}

var regressionReasons = map[string]bool{
	"missing_package": true, "incomplete_package": true, "no_test_cases": true, "required_test_missing": true,
	"test_failed": true, "package_failed": true, "package_skipped": true, "incomplete_test": true,
	"invalid_event_log": true, "truncated_event_log": true, "invalid_event_sequence": true,
	"duplicate_final_event": true, "unexpected_package": true, "cached_package": true,
	"events_too_large": true, "event_read_failed": true, "events_unavailable": true,
	"event_log_changed": true, "build_failed": true, "test_case_limit": true,
}

type RegressionPackage struct {
	Name      string `json:"name"`
	TestCases int    `json:"test_cases"`
	Passed    bool   `json:"passed"`
}
type RegressionCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}
type RegressionReport struct {
	Version         int                 `json:"version"`
	Scope           string              `json:"scope"`
	SourceRevision  string              `json:"source_revision"`
	ControlRevision string              `json:"control_revision"`
	Passed          bool                `json:"passed"`
	ModelInference  bool                `json:"model_inference"`
	Packages        []RegressionPackage `json:"packages"`
	Checks          []RegressionCheck   `json:"checks"`
	Reasons         []string            `json:"reasons"`
}
type regressionEvent struct {
	Time        string
	Action      string
	Package     string
	Test        string
	Output      string
	OutputType  string
	Elapsed     float64
	ImportPath  string
	FailedBuild string
}

type regressionState struct {
	started, valid, failed bool
	final                  string
	tests                  map[string]string
}

type eventReader struct {
	io.Reader
	bytes int64
	last  byte
}

func (r *eventReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += int64(n)
	if n > 0 {
		r.last = p[n-1]
	}
	return n, err
}

func emptyRegression(source, control string) RegressionReport {
	r := RegressionReport{Version: 1, Scope: "api-regression", SourceRevision: source, ControlRevision: control, Packages: []RegressionPackage{}, Checks: []RegressionCheck{}, Reasons: []string{}}
	for _, name := range regressionPackages {
		r.Packages = append(r.Packages, RegressionPackage{Name: name})
	}
	for _, check := range regressionChecks {
		r.Checks = append(r.Checks, RegressionCheck{Name: check.name})
	}
	return r
}

// Regression validates a complete, uncached go test -json event log from the
// six fixed synthetic regression packages. Revisions identify the caller's
// tested checkout; source authenticity is established by the trusted CI job.
// Output text and test names never enter the fixed public report.
func Regression(reader io.Reader, source, control string) (RegressionReport, error) {
	if !validRevision(source) || !validRevision(control) {
		return RegressionReport{}, errors.New("regression revisions must be full lowercase Git hashes")
	}
	report := emptyRegression(source, control)
	states := map[string]*regressionState{}
	for _, name := range regressionPackages {
		states[name] = &regressionState{valid: true, tests: map[string]string{}}
	}
	reasons := map[string]bool{}
	add := func(reason string) { reasons[reason] = true }
	if reader == nil {
		reader = strings.NewReader("")
		add("event_read_failed")
	}
	input := &eventReader{Reader: io.LimitReader(reader, MaxRegressionEventBytes+1)}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 65536), (1<<20)+1)
	for scanner.Scan() {
		var fields map[string]json.RawMessage
		if taskquality.DecodeEvidenceJSON(scanner.Bytes(), &fields) != nil || fields == nil {
			add("invalid_event_log")
			break
		}
		known := true
		for field := range fields {
			switch field {
			case "Time", "Action", "Package", "Test", "Output", "OutputType", "Elapsed", "ImportPath", "FailedBuild":
			default:
				known = false
			}
		}
		var event regressionEvent
		if !known || json.Unmarshal(scanner.Bytes(), &event) != nil || event.Elapsed < 0 || (event.Time != "" && !validEventTime(event.Time)) {
			add("invalid_event_log")
			break
		}
		// Go 1.27 test2json classifies framing and error output. Validate the
		// documented metadata exactly; never copy either it or Output publicly.
		if rawType, exists := fields["OutputType"]; exists {
			var outputType *string
			if json.Unmarshal(rawType, &outputType) != nil || outputType == nil || event.Action != "output" || (*outputType != "" && *outputType != "frame" && *outputType != "error" && *outputType != "error-continue") {
				add("invalid_event_log")
				break
			}
		}
		if event.Action == "build-output" || event.Action == "build-fail" {
			if event.ImportPath == "" || event.Test != "" {
				add("invalid_event_log")
				break
			}
			if event.Action == "build-fail" {
				add("build_failed")
			}
			continue
		}
		state, exists := states[event.Package]
		if !exists {
			add("unexpected_package")
			continue
		}
		bad := func(reason string) { state.valid = false; add(reason) }
		switch event.Action {
		case "start", "run", "pause", "cont", "output", "pass", "fail", "skip":
		default:
			bad("invalid_event_log")
			continue
		}
		if state.final != "" {
			if event.Test == "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
				bad("duplicate_final_event")
			} else {
				bad("invalid_event_sequence")
			}
			continue
		}
		if event.Action == "start" {
			if state.started || event.Test != "" {
				bad("invalid_event_sequence")
			} else {
				state.started = true
			}
			continue
		}
		if !state.started {
			bad("invalid_event_sequence")
			continue
		}
		if event.Action == "output" {
			if event.Test == "" && strings.Contains(event.Output, "(cached)") {
				bad("cached_package")
			}
			continue
		}
		if event.Test == "" {
			if event.Action != "pass" && event.Action != "fail" && event.Action != "skip" {
				bad("invalid_event_sequence")
				continue
			}
			state.final = event.Action
			if event.Action == "fail" {
				add("package_failed")
				state.failed = true
			}
			if event.Action == "skip" {
				bad("package_skipped")
			}
			continue
		}
		if len(event.Test) > 1024 {
			bad("invalid_event_log")
			continue
		}
		previous := state.tests[event.Test]
		switch event.Action {
		case "run":
			if previous != "" {
				bad("invalid_event_sequence")
			} else if len(state.tests) >= maxRegressionTests {
				bad("test_case_limit")
			} else {
				state.tests[event.Test] = "run"
			}
		case "pause":
			if previous != "run" {
				bad("invalid_event_sequence")
			} else {
				state.tests[event.Test] = "pause"
			}
		case "cont":
			if previous != "pause" {
				bad("invalid_event_sequence")
			} else {
				state.tests[event.Test] = "run"
			}
		case "pass", "fail", "skip":
			if previous == "pass" || previous == "fail" || previous == "skip" {
				bad("duplicate_final_event")
				continue
			}
			if previous != "run" {
				bad("invalid_event_sequence")
				continue
			}
			state.tests[event.Test] = event.Action
			if event.Action == "fail" {
				state.failed = true
				add("test_failed")
			}
		default:
			bad("invalid_event_sequence")
		}
	}
	if input.bytes > MaxRegressionEventBytes {
		add("events_too_large")
	}
	if scanner.Err() != nil {
		add("event_read_failed")
	}
	if input.bytes > 0 && input.last != '\n' {
		add("truncated_event_log")
	}
	for i, pkg := range report.Packages {
		state := states[pkg.Name]
		if !state.started {
			add("missing_package")
		} else if state.final == "" {
			add("incomplete_package")
		}
		for _, end := range state.tests {
			if end == "pass" {
				report.Packages[i].TestCases++
			}
			if end == "run" || end == "pause" {
				state.valid = false
				add("incomplete_test")
			}
		}
		if report.Packages[i].TestCases == 0 {
			add("no_test_cases")
		}
		report.Packages[i].Passed = state.started && state.final == "pass" && state.valid && !state.failed && report.Packages[i].TestCases > 0
	}
	for i, check := range regressionChecks {
		passed := report.Packages[check.packageIndex].Passed
		for _, name := range check.tests {
			if states[regressionPackages[check.packageIndex]].tests[name] != "pass" {
				passed = false
				add("required_test_missing")
			}
		}
		report.Checks[i].Passed = passed
	}
	for reason := range reasons {
		report.Reasons = append(report.Reasons, reason)
	}
	sort.Strings(report.Reasons)
	report.Passed = len(report.Reasons) == 0
	if !report.Passed {
		return report, errors.New("regression evidence failed")
	}
	return report, nil
}

func validEventTime(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

// VerifyRegression checks the fixed report schema, caller-expected revisions
// and internally consistent outcome claims. It does not authenticate event logs.
func VerifyRegression(report RegressionReport, source, control string) error {
	invalid := errors.New("invalid regression report or provenance")
	if !validRevision(source) || !validRevision(control) || report.SourceRevision != source || report.ControlRevision != control || report.Version != 1 || report.Scope != "api-regression" || report.ModelInference || len(report.Packages) != len(regressionPackages) || len(report.Checks) != len(regressionChecks) || report.Reasons == nil {
		return invalid
	}
	all := true
	for i, pkg := range report.Packages {
		if pkg.Name != regressionPackages[i] || pkg.TestCases < 0 || pkg.TestCases > maxRegressionTests || (pkg.Passed && pkg.TestCases == 0) {
			return invalid
		}
		all = all && pkg.Passed
	}
	for i, check := range report.Checks {
		if check.Name != regressionChecks[i].name || (check.Passed && !report.Packages[regressionChecks[i].packageIndex].Passed) {
			return invalid
		}
		if len(regressionChecks[i].tests) == 0 && check.Passed != report.Packages[regressionChecks[i].packageIndex].Passed {
			return invalid
		}
		all = all && check.Passed
	}
	for i, reason := range report.Reasons {
		if !regressionReasons[reason] || (i > 0 && report.Reasons[i-1] >= reason) {
			return invalid
		}
	}
	if report.Passed != (all && len(report.Reasons) == 0) || (!report.Passed && len(report.Reasons) == 0) {
		return invalid
	}
	return nil
}

func readRegressionEvents(path, source, control string) (RegressionReport, error) {
	unavailable := emptyRegression(source, control)
	unavailable.Reasons = []string{"events_unavailable"}
	dir, err := checkedDirectory(filepath.Dir(path))
	if err != nil {
		return unavailable, errors.New("regression events unavailable")
	}
	path = filepath.Join(dir, filepath.Base(path))
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return unavailable, errors.New("regression events unavailable")
	}
	if before.Size() > MaxRegressionEventBytes {
		unavailable.Reasons = []string{"events_too_large"}
		return unavailable, errors.New("regression events exceed limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return unavailable, errors.New("regression events unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return unavailable, errors.New("regression events unavailable")
	}
	report, regressionErr := Regression(file, source, control)
	after, err := file.Stat()
	final, finalErr := os.Lstat(path)
	if err != nil || finalErr != nil || !os.SameFile(before, after) || !os.SameFile(after, final) || !final.Mode().IsRegular() || after.Size() != before.Size() || final.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !final.ModTime().Equal(before.ModTime()) {
		report.Passed = false
		if !hasRegressionReason(report.Reasons, "event_log_changed") {
			report.Reasons = append(report.Reasons, "event_log_changed")
			sort.Strings(report.Reasons)
		}
		return report, errors.New("regression event log changed")
	}
	return report, regressionErr
}

func hasRegressionReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func writeRegressionReport(path string, report RegressionReport) error {
	if VerifyRegression(report, report.SourceRevision, report.ControlRevision) != nil {
		return errors.New("invalid regression report")
	}
	dir, err := checkedDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	path = filepath.Join(dir, filepath.Base(path))
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("regression target must be regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil || len(data) > MaxManifestBytes {
		return errors.New("invalid regression report size")
	}
	file, err := os.CreateTemp(dir, ".regression-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func runRegression(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("evidence regression", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	events := flags.String("events", "", "complete go test -json event file")
	reportPath := flags.String("report", "", "fixed regression report file")
	source := flags.String("source-revision", "", "full tested checkout revision")
	control := flags.String("control-revision", "", "full trusted control revision")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *events == "" || *reportPath == "" || !validRevision(*source) || !validRevision(*control) {
		fmt.Fprintln(stderr, "Invalid regression options; events, report and full lowercase revisions are required")
		return 2
	}
	eventsAbs, eventsErr := filepath.Abs(*events)
	reportAbs, reportErr := filepath.Abs(*reportPath)
	if eventsErr != nil || reportErr != nil || eventsAbs == reportAbs {
		fmt.Fprintln(stderr, "Regression input and output must be separate files")
		return 2
	}
	report, err := readRegressionEvents(*events, *source, *control)
	if writeRegressionReport(*reportPath, report) != nil {
		fmt.Fprintln(stderr, "Regression report could not be written safely")
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "Regression evidence failed; inspect fixed report reasons")
		return 1
	}
	if _, err := fmt.Fprintln(out, "API regression evidence passed; model inference was not performed"); err != nil {
		return 1
	}
	return 0
}
