package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func callCommand(t *testing.T, args []string, fn func()) string {
	t.Helper()
	oldFlags := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = oldFlags }()
	oldArgs, oldIn, oldOut, oldErr := os.Args, os.Stdin, os.Stdout, os.Stderr
	os.Args = append([]string{"llm-sentinel"}, args...)
	dir := t.TempDir()
	in, _ := os.Create(filepath.Join(dir, "in"))
	out, _ := os.Create(filepath.Join(dir, "out"))
	errOut, _ := os.Create(filepath.Join(dir, "err"))
	in.Seek(0, 0)
	os.Stdin, os.Stdout, os.Stderr = in, out, errOut
	defer func() {
		os.Args, os.Stdin, os.Stdout, os.Stderr = oldArgs, oldIn, oldOut, oldErr
		in.Close()
		out.Close()
		errOut.Close()
	}()
	fn()
	out.Seek(0, 0)
	errOut.Seek(0, 0)
	b, _ := io.ReadAll(out)
	return string(b)
}

func TestPercentileUsesOrderedSampleWithoutMutatingInput(t *testing.T) {
	values := []int64{100, 1, 50, 2}
	before := append([]int64(nil), values...)
	for _, tc := range []struct {
		p    float64
		want int64
	}{{0, 1}, {50, 50}, {100, 100}, {-1, 1}, {101, 100}} {
		if got := calculatePercentile(values, tc.p); got != tc.want {
			t.Errorf("percentile %v=%d want %d", tc.p, got, tc.want)
		}
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("sample mutated")
	}
	if calculatePercentile(nil, 50) != 0 {
		t.Fatal("empty sample")
	}
}
func TestLoadRunIsBoundedAndConsistent(t *testing.T) {
	before := runtime.NumGoroutine()
	cfg := LoadTestConfig{Duration: 150 * time.Millisecond, TargetRate: 500, Workers: 4, RampUpDuration: 40 * time.Millisecond, RampDownDuration: 40 * time.Millisecond, ReportInterval: 10 * time.Millisecond}
	var m *LoadTestMetrics
	out := callCommand(t, nil, func() { m = runLoadTest(cfg) })
	if m.TotalRequests == 0 || m.TotalRequests != m.SuccessCount+m.FailureCount || len(m.LatencyValues) != int(m.TotalRequests) || m.EndTime.Before(m.StartTime) {
		t.Fatal(m)
	}
	if !strings.Contains(out, "Requests:") {
		t.Fatal(out)
	}
	time.Sleep(40 * time.Millisecond)
	if got := runtime.NumGoroutine(); got > before {
		t.Fatalf("load run left reporting goroutine: before=%d after=%d", before, got)
	}
}
func TestLoadReportPassFailAndMain(t *testing.T) {
	for _, pass := range []bool{false, true} {
		m := &LoadTestMetrics{StartTime: time.Now().Add(-time.Second), EndTime: time.Now(), TotalRequests: 100, SuccessCount: 100, TotalLatencyMs: 100, LatencyValues: []int64{1, 2, 3}, MinLatencyMs: 1, MaxLatencyMs: 3}
		if !pass {
			m.SuccessCount = 2
			m.FailureCount = 8
			m.LatencyValues = []int64{500, 700}
			m.TotalRequests = 10
		}
		out := callCommand(t, nil, func() { printFinalReport(m, LoadTestConfig{TargetRate: 50}) })
		want := "PASS"
		if !pass {
			want = "FAIL"
		}
		if strings.Count(out, want) != 3 {
			t.Fatal(out)
		}
	}
	out := callCommand(t, nil, func() {
		printInterimReport(&LoadTestMetrics{StartTime: time.Now()})
		printFinalReport(&LoadTestMetrics{StartTime: time.Now().Add(-time.Second), EndTime: time.Now()}, LoadTestConfig{})
	})
	if !strings.Contains(out, "Total Requests: 0") {
		t.Fatal(out)
	}
	out = callCommand(t, []string{"--duration", "40ms", "--rate", "100", "--workers", "1", "--ramp-up", "0s", "--ramp-down", "0s", "--report", "10ms"}, main)
	if !strings.Contains(out, "LOAD TEST RESULTS") {
		t.Fatal(out)
	}
}
