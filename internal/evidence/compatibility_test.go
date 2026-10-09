package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var fixturePackages = []string{
	"github.com/szibis/claude-escalate/internal/localgateway",
	"github.com/szibis/claude-escalate/internal/clientcapture",
	"github.com/szibis/claude-escalate/internal/clientcontrol",
	"github.com/szibis/claude-escalate/internal/claudemod",
	"github.com/szibis/claude-escalate/internal/taskquality",
	"github.com/szibis/claude-escalate/internal/labstatus",
}

var fixtureAPITests = []string{"TestClaudeToolRoundTripAndStream", "TestResponsesTextProtocol", "TestResponsesValidatedToolStreamAndHistory", "TestChatCompletionsFunctionRoundTrip", "TestChatStreamingUsesCompletionChunks"}

func eventJSON(t *testing.T, action, pkg, test, output string) string {
	t.Helper()
	value := map[string]any{"Time": "2026-10-09T14:00:00Z", "Action": action, "Package": pkg}
	if test != "" {
		value["Test"] = test
	}
	if output != "" {
		value["Output"] = output
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func completeEvents(t *testing.T) string {
	t.Helper()
	var log strings.Builder
	for i, pkg := range fixturePackages {
		log.WriteString(eventJSON(t, "start", pkg, "", ""))
		names := []string{"TestSyntheticFixture"}
		if i == 0 {
			names = fixtureAPITests
		}
		for _, name := range names {
			log.WriteString(eventJSON(t, "run", pkg, name, ""))
			log.WriteString(eventJSON(t, "output", pkg, name, "PRIVATE_TEST_OUTPUT /private/path\n"))
			log.WriteString(eventJSON(t, "pass", pkg, name, ""))
		}
		if i == 1 {
			for _, name := range []string{"TestParallel", "TestParallel/child"} {
				log.WriteString(eventJSON(t, "run", pkg, name, ""))
			}
			log.WriteString(eventJSON(t, "pause", pkg, "TestParallel/child", ""))
			log.WriteString(eventJSON(t, "cont", pkg, "TestParallel/child", ""))
			log.WriteString(eventJSON(t, "pass", pkg, "TestParallel/child", ""))
			log.WriteString(eventJSON(t, "pass", pkg, "TestParallel", ""))
		}
		log.WriteString(eventJSON(t, "pass", pkg, "", ""))
	}
	return log.String()
}

// Counting output/run events or accepting incomplete API subsets would
// misrepresent the exact synthetic protocol regressions that actually passed.
func TestRegressionCountsCompleteNamedPassesAndFixedSafeChecks(t *testing.T) {
	report, err := Regression(strings.NewReader(completeEvents(t)), strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err != nil || !report.Passed || report.ModelInference || report.Version != 1 || report.Scope != "api-regression" {
		t.Fatalf("regression: %+v %v", report, err)
	}
	if report.SourceRevision != strings.Repeat("a", 40) || report.ControlRevision != strings.Repeat("b", 40) {
		t.Fatal("wrong provenance")
	}
	if len(report.Packages) != 6 || len(report.Checks) != 8 || len(report.Reasons) != 0 {
		t.Fatalf("wrong complete summary: %+v", report)
	}
	for i, pkg := range report.Packages {
		wantCount := 1
		if i == 0 {
			wantCount = 5
		}
		if i == 1 {
			wantCount = 3
		}
		if pkg.Name != fixturePackages[i] || pkg.TestCases != wantCount || !pkg.Passed {
			t.Fatalf("package counts: %+v", report.Packages)
		}
	}
	wantChecks := []string{"messages", "responses", "chat-completions", "capture", "controls", "claude-mod", "quality-harness", "telemetry"}
	for i, check := range report.Checks {
		if check.Name != wantChecks[i] || !check.Passed {
			t.Fatalf("wrong checks: %+v", report.Checks)
		}
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "/private/path") || strings.Contains(string(raw), "TestSyntheticFixture") {
		t.Fatalf("raw evidence leaked: %s", raw)
	}
	if err := VerifyRegression(report, strings.Repeat("a", 40), strings.Repeat("b", 40)); err != nil {
		t.Fatal(err)
	}
}

func TestRegressionRejectsIncompleteMalformedAndFailedEvents(t *testing.T) {
	complete := completeEvents(t)
	gateway := fixturePackages[0]
	capture := fixturePackages[1]
	for _, tc := range []struct{ name, log, reason string }{
		{"empty", "", "missing_package"},
		{"missing-package", strings.ReplaceAll(complete, eventJSON(t, "start", capture, "", "")+eventJSON(t, "run", capture, "TestSyntheticFixture", "")+eventJSON(t, "output", capture, "TestSyntheticFixture", "PRIVATE_TEST_OUTPUT /private/path\n")+eventJSON(t, "pass", capture, "TestSyntheticFixture", "")+eventJSON(t, "run", capture, "TestParallel", "")+eventJSON(t, "run", capture, "TestParallel/child", "")+eventJSON(t, "pause", capture, "TestParallel/child", "")+eventJSON(t, "cont", capture, "TestParallel/child", "")+eventJSON(t, "pass", capture, "TestParallel/child", "")+eventJSON(t, "pass", capture, "TestParallel", "")+eventJSON(t, "pass", capture, "", ""), ""), "missing_package"},
		{"missing-required-api", strings.ReplaceAll(complete, eventJSON(t, "run", gateway, fixtureAPITests[0], "")+eventJSON(t, "output", gateway, fixtureAPITests[0], "PRIVATE_TEST_OUTPUT /private/path\n")+eventJSON(t, "pass", gateway, fixtureAPITests[0], ""), ""), "required_test_missing"},
		{"test-failed", strings.Replace(complete, eventJSON(t, "pass", gateway, fixtureAPITests[0], ""), eventJSON(t, "fail", gateway, fixtureAPITests[0], ""), 1), "test_failed"},
		{"required-skipped", strings.Replace(complete, eventJSON(t, "pass", gateway, fixtureAPITests[0], ""), eventJSON(t, "skip", gateway, fixtureAPITests[0], ""), 1), "required_test_missing"},
		{"package-failed", strings.Replace(complete, eventJSON(t, "pass", capture, "", ""), eventJSON(t, "fail", capture, "", ""), 1), "package_failed"},
		{"package-truncated", strings.TrimSuffix(complete, eventJSON(t, "pass", fixturePackages[5], "", "")), "incomplete_package"},
		{"unfinished-test", strings.Replace(complete, eventJSON(t, "pass", capture, "TestSyntheticFixture", ""), "", 1), "incomplete_test"},
		{"missing-newline", strings.TrimSuffix(complete, "\n"), "truncated_event_log"},
		{"truncated-json", complete + `{"Action":"pass"`, "invalid_event_log"},
		{"non-json", complete + "PRIVATE_ERROR_LOG\n", "invalid_event_log"},
		{"duplicate-package-final", complete + eventJSON(t, "pass", capture, "", ""), "duplicate_final_event"},
		{"duplicate-test-final", strings.Replace(complete, eventJSON(t, "pass", gateway, fixtureAPITests[0], ""), strings.Repeat(eventJSON(t, "pass", gateway, fixtureAPITests[0], ""), 2), 1), "duplicate_final_event"},
		{"pass-without-run", strings.Replace(complete, eventJSON(t, "run", gateway, fixtureAPITests[0], ""), "", 1), "invalid_event_sequence"},
		{"duplicate-key", complete + `{"Action":"pass","Action":"fail","Package":"PRIVATE_PACKAGE"}` + "\n", "invalid_event_log"},
		{"unknown-event", complete + eventJSON(t, "PRIVATE_ACTION", capture, "", ""), "invalid_event_log"},
		{"unexpected-package", complete + eventJSON(t, "start", "PRIVATE_PACKAGE", "", ""), "unexpected_package"},
		{"cached-package", strings.Replace(complete, eventJSON(t, "pass", capture, "", ""), eventJSON(t, "output", capture, "", "ok fixture (cached)\n")+eventJSON(t, "pass", capture, "", ""), 1), "cached_package"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Regression(strings.NewReader(tc.log), strings.Repeat("a", 40), strings.Repeat("b", 40))
			if err == nil || report.Passed {
				t.Fatalf("bad log accepted: %+v %v", report, err)
			}
			if !containsReason(report.Reasons, tc.reason) {
				t.Fatalf("missing useful reason %s: %+v", tc.reason, report)
			}
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(err.Error(), "PRIVATE_") {
				t.Fatalf("private evidence leaked: %s %v", raw, err)
			}
			if err := VerifyRegression(report, strings.Repeat("a", 40), strings.Repeat("b", 40)); err != nil {
				t.Fatalf("failure report must remain structurally valid: %v", err)
			}
		})
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestRegressionRejectsEmptyPackagesAndAllRequiredSkipped(t *testing.T) {
	var log strings.Builder
	for _, pkg := range fixturePackages {
		log.WriteString(eventJSON(t, "start", pkg, "", ""))
		log.WriteString(eventJSON(t, "pass", pkg, "", ""))
	}
	report, err := Regression(strings.NewReader(log.String()), strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err == nil || report.Passed || !containsReason(report.Reasons, "no_test_cases") {
		t.Fatalf("empty package passes accepted: %+v %v", report, err)
	}
}

func TestRegressionStrictMetadataAndConsistentClaims(t *testing.T) {
	for _, source := range []string{"", "abc", strings.Repeat("A", 40), strings.Repeat("z", 40)} {
		if _, err := Regression(strings.NewReader(completeEvents(t)), source, strings.Repeat("b", 40)); err == nil {
			t.Fatal("invalid revision accepted", source)
		}
	}
	report, err := Regression(strings.NewReader(completeEvents(t)), strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err != nil {
		t.Fatal(err)
	}
	if VerifyRegression(report, strings.Repeat("c", 40), strings.Repeat("b", 40)) == nil || VerifyRegression(report, strings.Repeat("a", 40), strings.Repeat("c", 40)) == nil {
		t.Fatal("stale source/control metadata accepted")
	}
	for _, alter := range []func(*RegressionReport){
		func(r *RegressionReport) { r.ModelInference = true }, func(r *RegressionReport) { r.Scope = "real-clients" }, func(r *RegressionReport) { r.Packages[0].TestCases = 0 }, func(r *RegressionReport) { r.Packages[0].Name = "PRIVATE_PACKAGE" }, func(r *RegressionReport) { r.Packages[0].Passed = false }, func(r *RegressionReport) { r.Checks[0].Passed = false }, func(r *RegressionReport) { r.Reasons = []string{"PRIVATE_REASON"} }, func(r *RegressionReport) { r.Passed = false },
	} {
		raw, _ := json.Marshal(report)
		var changed RegressionReport
		json.Unmarshal(raw, &changed)
		alter(&changed)
		if VerifyRegression(changed, strings.Repeat("a", 40), strings.Repeat("b", 40)) == nil {
			t.Fatalf("inconsistent claim accepted: %+v", changed)
		}
	}
}

func TestRegressionCLIAlwaysWritesSafeFailureReport(t *testing.T) {
	root := evidenceTempDir(t)
	events := filepath.Join(root, "events.jsonl")
	path := filepath.Join(root, "regression.json")
	for _, tc := range []struct {
		log  string
		want int
	}{{completeEvents(t), 0}, {"PRIVATE_NON_JSON\n", 1}} {
		if err := os.WriteFile(events, []byte(tc.log), 0600); err != nil {
			t.Fatal(err)
		}
		var out, diagnostic bytes.Buffer
		code := Run([]string{"regression", "--events", events, "--report", path, "--source-revision", strings.Repeat("a", 40), "--control-revision", strings.Repeat("b", 40)}, nil, &out, &diagnostic)
		if code != tc.want {
			t.Fatalf("CLI outcome %d %s %s", code, &out, &diagnostic)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("report not written", err)
		}
		var report RegressionReport
		if json.Unmarshal(raw, &report) != nil || report.Passed != (tc.want == 0) {
			t.Fatalf("report outcome %s", raw)
		}
		if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(diagnostic.String(), "PRIVATE_") || strings.Contains(out.String(), root) || strings.Contains(diagnostic.String(), root) {
			t.Fatal("CLI disclosed private log/path")
		}
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("report permissions", err)
		}
	}
	var out, diagnostic bytes.Buffer
	before, _ := os.ReadFile(path)
	if Run([]string{"regression", "--events", events, "--report", path, "--source-revision", "PRIVATE_BAD_REVISION", "--control-revision", strings.Repeat("b", 40)}, nil, &out, &diagnostic) != 2 || strings.Contains(diagnostic.String(), "PRIVATE_") {
		t.Fatal("invalid metadata CLI disclosure")
	}
	if !reflect.DeepEqual(before, mustReadCompatibility(t, path)) {
		t.Fatal("bad revisions changed report")
	}
}

type regressionReadError struct{}

func (regressionReadError) Read([]byte) (int, error) { return 0, errors.New("PRIVATE_READER_ERROR") }

func TestRegressionReadErrorsAndOversizedLinesStayPrivate(t *testing.T) {
	for _, reader := range []io.Reader{io.MultiReader(strings.NewReader(completeEvents(t)), regressionReadError{}), strings.NewReader(strings.Repeat("x", (1<<20)+1)), nil} {
		report, err := Regression(reader, strings.Repeat("a", 40), strings.Repeat("b", 40))
		if err == nil || report.Passed || !containsReason(report.Reasons, "event_read_failed") {
			t.Fatalf("read failure accepted: %+v %v", report, err)
		}
		if strings.Contains(err.Error(), "PRIVATE_") {
			t.Fatal("reader error exposed")
		}
	}
}

func TestRegressionBuildEventsAndExactMetadataKeys(t *testing.T) {
	build := `{"Action":"build-output","ImportPath":"fixture/dependency","Output":"PRIVATE_BUILD_OUTPUT"}` + "\n"
	report, err := Regression(strings.NewReader(build+completeEvents(t)), strings.Repeat("a", 64), strings.Repeat("b", 64))
	if err != nil || !report.Passed {
		t.Fatalf("normal Go build output rejected: %+v %v", report, err)
	}
	for _, log := range []string{strings.Replace(build, "build-output", "build-fail", 1) + completeEvents(t), strings.Replace(completeEvents(t), `"Action":`, `"action":`, 1), strings.Replace(completeEvents(t), `"Action":`, `"Action":"PRIVATE_ALIAS","action":`, 1)} {
		report, err := Regression(strings.NewReader(log), strings.Repeat("a", 40), strings.Repeat("b", 40))
		if err == nil || report.Passed {
			t.Fatal("build failure or ambiguous metadata accepted")
		}
		raw, _ := json.Marshal(report)
		if strings.Contains(string(raw), "PRIVATE_") {
			t.Fatal("build contents exposed")
		}
	}
}

// Copied from the project's real Go 1.27.2 test -json output. Rejecting the
// documented OutputType metadata would turn valid production runs into failures.
func TestRegressionAcceptsGo127OutputTypesWithoutPublishingOutput(t *testing.T) {
	realFrame := `{"Time":"2026-10-09T12:46:18.388064+02:00","Action":"output","Package":"github.com/szibis/claude-escalate/internal/localgateway","Test":"TestClaudeToolRoundTripAndStream","Output":"=== RUN   TestClaudeToolRoundTripAndStream\n","OutputType":"frame"}` + "\n"
	original := eventJSON(t, "output", fixturePackages[0], fixtureAPITests[0], "PRIVATE_TEST_OUTPUT /private/path\n")
	for _, outputType := range []string{"", "frame", "error", "error-continue"} {
		t.Run(outputType, func(t *testing.T) {
			frame := strings.Replace(realFrame, `"OutputType":"frame"`, `"OutputType":"`+outputType+`"`, 1)
			log := strings.Replace(completeEvents(t), original, frame, 1)
			wantPassed := outputType == "" || outputType == "frame"
			if !wantPassed {
				log = strings.Replace(log, eventJSON(t, "pass", fixturePackages[0], fixtureAPITests[0], ""), eventJSON(t, "fail", fixturePackages[0], fixtureAPITests[0], ""), 1)
			}
			report, err := Regression(strings.NewReader(log), strings.Repeat("a", 40), strings.Repeat("b", 40))
			if report.Passed != wantPassed || (err == nil) != wantPassed || containsReason(report.Reasons, "invalid_event_log") {
				t.Fatalf("documented output metadata was rejected: %+v %v", report, err)
			}
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), "OutputType") || strings.Contains(string(raw), "=== RUN") || strings.Contains(string(raw), "PRIVATE_") {
				t.Fatal("output metadata or contents published")
			}
		})
	}
	for _, metadata := range []string{`"OutputType":"PRIVATE_UNKNOWN_TYPE"`, `"OutputType":42`, `"OutputType":null`, `"OutputType":"frame","OutputType":"error"`, `"outputType":"frame"`} {
		frame := strings.Replace(realFrame, `"OutputType":"frame"`, metadata, 1)
		report, err := Regression(strings.NewReader(strings.Replace(completeEvents(t), original, frame, 1)), strings.Repeat("a", 40), strings.Repeat("b", 40))
		if err == nil || report.Passed || !containsReason(report.Reasons, "invalid_event_log") {
			t.Fatalf("malformed output metadata accepted: %+v %v", report, err)
		}
	}
	frameOnRun := strings.Replace(eventJSON(t, "run", fixturePackages[0], fixtureAPITests[0], ""), `"Action":"run"`, `"Action":"run","OutputType":"frame"`, 1)
	log := strings.Replace(completeEvents(t), eventJSON(t, "run", fixturePackages[0], fixtureAPITests[0], ""), frameOnRun, 1)
	report, err := Regression(strings.NewReader(log), strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err == nil || report.Passed || !containsReason(report.Reasons, "invalid_event_log") {
		t.Fatal("output type on a non-output action accepted")
	}
}

func TestRegressionCLIBoundsMissingFilesAndSymlinks(t *testing.T) {
	root := evidenceTempDir(t)
	events := filepath.Join(root, "events.jsonl")
	path := filepath.Join(root, "report.json")
	run := func(events string) int {
		t.Helper()
		var out, diagnostic bytes.Buffer
		return Run([]string{"regression", "--events", events, "--report", path, "--source-revision", strings.Repeat("a", 40), "--control-revision", strings.Repeat("b", 40)}, nil, &out, &diagnostic)
	}
	if run(events) != 1 {
		t.Fatal("missing event file accepted")
	}
	var report RegressionReport
	if json.Unmarshal(mustReadCompatibility(t, path), &report) != nil || report.Passed || !containsReason(report.Reasons, "events_unavailable") {
		t.Fatal("missing file failure report absent")
	}
	file, err := os.Create(events)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((128 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if run(events) != 1 {
		t.Fatal("oversized event file accepted")
	}
	if json.Unmarshal(mustReadCompatibility(t, path), &report) != nil || report.Passed || !containsReason(report.Reasons, "events_too_large") {
		t.Fatal("oversized failure report absent")
	}
	if err := os.WriteFile(events, []byte(completeEvents(t)), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "event-link.jsonl")
	if err := os.Symlink(events, link); err != nil {
		t.Fatal(err)
	}
	if run(link) != 1 {
		t.Fatal("symlink event file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(events, path); err != nil {
		t.Fatal(err)
	}
	before := mustReadCompatibility(t, events)
	if run(events) != 1 || !bytes.Equal(before, mustReadCompatibility(t, events)) {
		t.Fatal("report symlink target changed")
	}
	if run(path) != 2 {
		t.Fatal("same input/output path accepted")
	}
}

func mustReadCompatibility(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
