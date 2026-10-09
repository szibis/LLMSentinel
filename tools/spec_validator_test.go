package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureReport(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old; f.Close() }()
	fn()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestRequirementStatesAndReports(t *testing.T) {
	dir := t.TempDir()
	sv := NewSpecValidator()
	sv.requirements = map[string]*Requirement{"complete": {ID: "complete", Title: "Complete", Files: []string{"complete.go"}, Tests: []string{"complete_test.go"}}, "partial": {ID: "partial", Title: "Partial", Files: []string{"partial.go"}, Tests: []string{"partial_test.go"}}, "missing": {ID: "missing", Title: "Missing", Files: []string{"missing.go"}}}
	for _, p := range []string{"complete.go", "complete_test.go", "partial.go"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("package fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result := sv.ValidateSpecCompliance(dir)
	if result.TotalRequirements != 3 || result.CompleteCount != 1 || result.ImplementedCount != 2 || result.TestedCount != 1 || len(result.UncoveredRequires) != 1 || len(result.PartiallyTestedReqs) != 1 {
		t.Fatalf("results=%+v", result)
	}
	out := captureReport(t, func() { sv.PrintReport(result) })
	for _, want := range []string{"complete", "Partial", "Missing", "LOW COMPLIANCE"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("report missing %q", want)
		}
	}
	for _, tc := range []struct {
		pct  float64
		want string
	}{{100, "ALL REQUIREMENTS"}, {95, "HIGH COMPLIANCE"}, {85, "MEDIUM COMPLIANCE"}} {
		result.Coverage = tc.pct
		out := captureReport(t, func() { sv.PrintReport(result) })
		if !strings.Contains(out, tc.want) {
			t.Errorf("coverage %v report=%q", tc.pct, out)
		}
	}
}
func TestSpecMainAndSecurityPatternReporting(t *testing.T) {
	dir := t.TempDir()
	sv := NewSpecValidator()
	for _, r := range sv.requirements {
		for _, p := range append(append([]string{}, r.Files...), r.Tests...) {
			full := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte("package fixture"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	p := filepath.Join(dir, "internal/security/patterns.go")
	if err := os.WriteFile(p, []byte("DROP DELETE UNION SELECT OR comment shell metacharacters $( eval system <script> javascript: onerror onclick onload"), 0600); err != nil {
		t.Fatal(err)
	}
	old := os.Args
	os.Args = []string{"spec-validator", dir}
	defer func() { os.Args = old }()
	out := captureReport(t, main)
	if !strings.Contains(out, "ALL REQUIREMENTS COVERED") || !strings.Contains(out, "SQL Injection: 5/5") {
		t.Fatalf("main report=%q", out)
	}
	missing := captureReport(t, func() { ValidateSecurityPatterns(t.TempDir()) })
	if !strings.Contains(missing, "Failed to read") {
		t.Fatal("missing security patterns not reported")
	}
}
func TestDirectoriesDoNotSatisfyFileRequirements(t *testing.T) {
	dir := t.TempDir()
	sv := NewSpecValidator()
	sv.requirements = map[string]*Requirement{"one": {Files: []string{"source.go"}, Tests: []string{"source_test.go"}}}
	for _, p := range []string{"source.go", "source_test.go"} {
		if err := os.Mkdir(filepath.Join(dir, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if result := sv.ValidateSpecCompliance(dir); result.CompleteCount != 0 {
		t.Fatalf("directories counted as files: %+v", result)
	}
}
