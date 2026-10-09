package testcoverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

const sample = "mode: atomic\nexample/live/a.go:1.1,2.1 9 1\nexample/live/a.go:3.1,4.1 1 0\nexample/legacy/b.go:1.1,2.1 5 0\n"

func TestCoverageCountsStatementsAndReportsWholeRepository(t *testing.T) {
	r, err := Analyze(strings.NewReader(sample), 90, []string{"example/live"})
	if err != nil || !r.Passed || r.Total != 15 || r.Covered != 9 || r.Percent != 60 || len(r.Packages) != 2 {
		t.Fatalf("wrong accounting: %#v %v", r, err)
	}
	all, err := Analyze(strings.NewReader(sample), 90, nil)
	if err != nil || all.Passed {
		t.Fatalf("repository below90 accepted: %#v %v", all, err)
	}
}
func TestCoverageMergesDuplicateBlocksWithoutInflatingDenominator(t *testing.T) {
	raw := sample + "example/live/a.go:3.1,4.1 1 12\nexample/live/a.go:1.1,2.1 9 3\n"
	r, err := Analyze(strings.NewReader(raw), 90, []string{"example/live"})
	if err != nil || r.Total != 15 || r.Covered != 10 || !r.Passed {
		t.Fatalf("duplicate blocks miscounted: %#v %v", r, err)
	}
}
func TestCoverageRejectsMalformedOrUnknownScope(t *testing.T) {
	for _, raw := range []string{"", "mode: invalid\n", "mode: set\n", "mode: set\nbad\n", "mode: atomic\na.go:1.1,2.1 -1 0\n", "mode: atomic\na.go:1.1,2.1 1 -1\n", "mode: atomic\na.go:1.1,2.1 18446744073709551615 1\n", "mode: atomic\na.go:1.1,2.1 1 0\na.go:1.1,2.1 2 1\n"} {
		if _, err := Analyze(strings.NewReader(raw), 90, nil); err == nil {
			t.Fatalf("bad profile accepted: %q", raw)
		}
	}
	for _, min := range []float64{-1, 0, 101} {
		if _, err := Analyze(strings.NewReader(sample), min, nil); err == nil {
			t.Fatalf("bad threshold accepted: %v", min)
		}
	}
	if _, err := Analyze(strings.NewReader(sample), 90, []string{"example/absent"}); err == nil {
		t.Fatal("missing scope accepted")
	}
}
func TestCoverageThresholdIsNotRoundedUp(t *testing.T) {
	r, err := Analyze(strings.NewReader("mode: count\nexample/a.go:1.1,2.1 8999 1\nexample/a.go:3.1,4.1 1001 0\n"), 90, nil)
	if err != nil || r.Passed {
		t.Fatalf("89.99%% accepted: %#v %v", r, err)
	}
}
func TestCoverageCLIRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--min", "NaN"}, {"extra"}} {
		var out, diagnostic bytes.Buffer
		if Run(args, nil, &out, &diagnostic) != 2 {
			t.Fatalf("bad flags accepted: %v", args)
		}
	}
}
func FuzzCoverageProfile(f *testing.F) {
	for _, s := range []string{sample, "mode: set\nx/a.go:1.1,2.1 1 0\n", "", sample + "example/live/a.go:1.1,2.1 9 4\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 65536 {
			t.Skip()
		}
		r, err := Analyze(strings.NewReader(raw), 90, nil)
		if err != nil {
			return
		}
		if r.Total <= 0 || r.Covered < 0 || r.Covered > r.Total || r.Percent < 0 || r.Percent > 100 {
			t.Fatalf("invalid coverage accounting: %#v", r)
		}
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestCoverageCLIReportsScopeAndFailures(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "coverage.out")
	if err := os.WriteFile(profile, []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args   []string
		code   int
		writer bool
	}{
		{[]string{"--profile", profile, "--min", "90", "--package", "example/live"}, 0, false},
		{[]string{"--profile", profile}, 1, false},
		{[]string{"--profile", profile, "--package", "example/absent"}, 1, false},
		{[]string{"--profile", filepath.Join(root, "missing")}, 1, false},
		{[]string{"--profile", profile, "--package", "example/live"}, 1, true},
		{[]string{"--profile", profile, "--package", ""}, 2, false},
		{[]string{"--min", "Inf"}, 2, false},
		{[]string{"--min", "0"}, 2, false},
		{[]string{"--min", "101"}, 2, false},
	} {
		t.Run(fmt.Sprint(tc.args), func(t *testing.T) {
			var out, diagnostic bytes.Buffer
			var writer io.Writer = &out
			if tc.writer {
				writer = failingWriter{}
			}
			code := Run(tc.args, nil, writer, &diagnostic)
			if code != tc.code {
				t.Fatalf("exit=%d want=%d output=%s error=%s", code, tc.code, out.String(), diagnostic.String())
			}
			if code == 0 {
				var report Report
				if json.Unmarshal(out.Bytes(), &report) != nil || !report.Passed || report.Percent != 60 || len(report.Scope) != 1 {
					t.Fatalf("CLI hid full report: %s", out.String())
				}
			}
		})
	}
	var scope scopes
	if scope.Set("example/live") != nil || scope.String() != "example/live" {
		t.Fatal("scope flag changed")
	}
}
func TestCoverageBoundsAndReadFailure(t *testing.T) {
	if _, err := Analyze(nil, 90, nil); err == nil {
		t.Fatal("nil profile accepted")
	}
	if _, err := Analyze(iotest.ErrReader(io.ErrUnexpectedEOF), 90, nil); err == nil {
		t.Fatal("unreadable profile accepted")
	}
	for _, raw := range []string{
		"mode: atomic\nx/a.go:1.1,2.1 1 18446744073709551615\n",
		"mode: atomic\nx/a.go:1.1,2.1 1000000000 1\nx/a.go:3.1,4.1 1 0\n",
		"mode: set\n" + strings.Repeat("x", 300000),
		"mode: set\n" + string([]byte{0xff}),
		strings.Repeat("x", 32*1024*1024+1),
	} {
		if _, err := Analyze(strings.NewReader(raw), 90, nil); err == nil {
			t.Fatal("oversized/malformed profile accepted")
		}
	}
	r, err := Analyze(strings.NewReader("mode: set\nx/a.go:1.1,2.1 1 1\ny/b.go:1.1,2.1 0 0\n"), 100, nil)
	if err != nil || !r.Passed || len(r.Packages) != 2 {
		t.Fatalf("zero-statement blocks changed accounting: %#v %v", r, err)
	}
	if _, err := Analyze(strings.NewReader("mode: set\nx/a.go:1.1,2.1 1 1\ny/b.go:1.1,2.1 0 0\n"), 90, []string{"y"}); err == nil {
		t.Fatal("zero-statement scope accepted")
	}
	for _, minimum := range []float64{math.NaN(), math.Inf(1)} {
		if _, err := Analyze(strings.NewReader(sample), minimum, nil); err == nil {
			t.Fatal("nonfinite threshold accepted")
		}
	}
}
