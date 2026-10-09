package security

import (
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuditFilesystemAndFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	a, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dir)
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatal(st, err)
	}
	fs, err := a.currentFile.Stat()
	if err != nil || fs.Mode().Perm() != 0600 {
		t.Fatal(fs, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.LogRateLimitTriggered("127.0.0.1"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, call := range []func() error{func() error { return a.LogInjectionAttempt("local", "sql", strings.Repeat("x", 150)) }, func() error { return a.LogValidationFailure("local", "bad") }, func() error { return a.LogUnauthorizedAccess("local", "private") }, func() error { return a.LogSecurityEvent("TEST", "CRITICAL", nil) }} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if a.GetEventCount() != 14 {
		t.Fatal(a.GetEventCount())
	}
	if err := a.rotateLogFile(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.currentFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "\n") != 14 || !strings.Contains(string(data), "input_sample="+strings.Repeat("x", 100)+"...") {
		t.Fatal(string(data))
	}
	if err := a.LogValidationFailure("local", "closed"); err == nil {
		t.Fatal("closed log accepted write")
	}
	if err := (&AuditLogger{}).Close(); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuditLogger(filepath.Join(blocker, "child")); err == nil {
		t.Fatal("bad directory accepted")
	}
	a.logDir = blocker
	if err := a.rotateLogFile(); err == nil {
		t.Fatal("bad rotation accepted")
	}
	if truncateString("short", 100) != "short" {
		t.Fatal("short string altered")
	}
}

func TestTokenBucketIsolationRefillAndReset(t *testing.T) {
	for _, perIP := range []bool{true, false} {
		rl := NewRateLimiter(2, perIP)
		defer rl.Close()
		if rl.GetRemaining("new") != 2 || !rl.Allow("a") || !rl.Allow("a") || rl.Allow("a") {
			t.Fatal("capacity invariant")
		}
		if got := rl.Allow("b"); got != perIP {
			t.Fatal("IP isolation", perIP, got)
		}
		key := "a"
		if !perIP {
			key = "global"
		}
		b := rl.ipBuckets[key]
		b.mu.Lock()
		b.lastRefill = time.Now().Add(-time.Minute)
		b.mu.Unlock()
		if !rl.Allow("a") || rl.GetRemaining("a") != 1 {
			t.Fatal("refill capped incorrectly")
		}
		rl.Reset("a")
		if rl.GetRemaining("a") != 2 || !rl.Allow("a") {
			t.Fatal("reset failed")
		}
	}
	rl := NewRateLimiter(0, true)
	defer rl.Close()
	if rl.Allow("a") {
		t.Fatal("zero budget allowed")
	}
	// Drive the cleanup channel directly: no wall-clock sleep or live traffic.
	ticks := make(chan time.Time)
	clean := &RateLimiter{ipBuckets: map[string]*tokenBucket{"old": {lastRefill: time.Now().Add(-11 * time.Minute)}, "fresh": {lastRefill: time.Now()}}, cleanupTicker: &time.Ticker{C: ticks}}
	done := make(chan struct{})
	go func() { clean.cleanupExpiredBuckets(); close(done) }()
	ticks <- time.Now()
	close(ticks)
	<-done
	if len(clean.ipBuckets) != 1 || clean.ipBuckets["fresh"] == nil {
		t.Fatal("expiry cleanup")
	}
	if min(1, 2) != 1 || min(3, 2) != 2 {
		t.Fatal("capacity clamp")
	}
}

func TestRateLimiterCloseTerminatesCleanup(t *testing.T) {
	count := func() int {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		return strings.Count(string(buf[:n]), "security.(*RateLimiter).cleanupExpiredBuckets(")
	}
	before := count()
	limiters := make([]*RateLimiter, 16)
	for i := range limiters {
		limiters[i] = NewRateLimiter(10, true)
	}
	for _, rl := range limiters {
		rl.Close()
		rl.Close()
	}
	deadline := time.Now().Add(time.Second)
	for count() > before && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if after := count(); after > before {
		t.Fatalf("cleanup goroutines leaked: before=%d after=%d", before, after)
	}
}

func TestValidatorSafetyContracts(t *testing.T) {
	v := NewValidator()
	for _, tc := range []struct {
		input string
		typ   InputType
		valid bool
	}{
		{"alice", InputTypeSQL, true}, {"' OR 1=1", InputTypeSQL, false}, {"alice -- note", InputTypeSQL, false}, {"DROP TABLE accounts", InputTypeSQL, false}, {"UPDATE accounts SET x=1 WHERE id=2", InputTypeSQL, true},
		{"echo hello", InputTypeCommand, true}, {"echo hello; rm", InputTypeCommand, false},
		{"hello", InputTypeWeb, true}, {"<script>alert(1)</script>", InputTypeWeb, false},
		{"", InputTypeJSON, false}, {"plain", InputTypeJSON, false}, {"{}", InputTypeJSON, true}, {"[]", InputTypeJSON, true},
		{"hello", "unknown", true}, {"javascript:evil", "unknown", false},
	} {
		ok, r := v.ValidateInput(tc.input, tc.typ)
		if ok != tc.valid || r.IsValid != ok || (!ok && len(r.Errors) == 0) {
			t.Fatalf("%q %+v", tc.input, r)
		}
	}
	for _, tc := range []struct {
		input string
		typ   OutputType
		want  string
	}{{"<a>", OutputTypeHTML, "&lt;a&gt;"}, {"O'Reilly", OutputTypeSQL, "O''Reilly"}, {"`$", OutputTypeShell, "\\`\\$"}, {"plain", "unknown", "plain"}} {
		got, r := v.ValidateOutput(tc.input, tc.typ)
		if got != tc.want || !r.IsValid {
			t.Fatal(got, r)
		}
	}
	if !v.IsHighRiskInput("delete") || v.IsHighRiskInput("hello") {
		t.Fatal("risk classification")
	}
	if got := compilePatterns([]string{"[", "safe"}); len(got) != 1 || !got[0].MatchString("safe") {
		t.Fatal("pattern compilation")
	}
}

func FuzzHTMLOutputEscaping(f *testing.F) {
	for _, s := range []string{"<script>alert(1)</script>", "\"'&", "hello", "\xff"} {
		f.Add(s)
	}
	v := NewValidator()
	f.Fuzz(func(t *testing.T, s string) {
		out, r := v.ValidateOutput(s, OutputTypeHTML)
		if out != html.EscapeString(s) || !r.IsValid || strings.ContainsAny(out, "<>\"'") {
			t.Fatalf("unsafe escaping %q", out)
		}
	})
}
