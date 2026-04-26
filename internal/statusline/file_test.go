package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeHome rewrites $HOME to a temp dir for the duration of the test
// and pre-creates the safe escalation directory.
func withFakeHome(t *testing.T) (home, safe string) {
	t.Helper()
	home = t.TempDir()
	safe = filepath.Join(home, ".claude", "data", "escalation")
	if err := os.MkdirAll(safe, 0o755); err != nil {
		t.Fatalf("mkdir safe: %v", err)
	}

	prev := os.Getenv("HOME")
	t.Setenv("HOME", home)
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prev)
	})
	return home, safe
}

// TestValidateFilePath_Rejects path-traversal and out-of-tree paths.
func TestValidateFilePath_Rejects(t *testing.T) {
	_, _ = withFakeHome(t)

	cases := []struct {
		name string
		in   string
	}{
		{"traversal dotdot", "../../etc/passwd"},
		{"absolute outside", "/etc/passwd"},
		{"deep traversal", "../../../../../../etc/shadow"},
		{"sibling dir", "/tmp/notallowed.json"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := validateFilePath(c.in); err == nil {
				t.Errorf("expected error for %q, got nil", c.in)
			}
		})
	}
}

// TestValidateFilePath_Accepts paths inside the safe directory.
func TestValidateFilePath_Accepts(t *testing.T) {
	_, safe := withFakeHome(t)

	in := filepath.Join(safe, "statusline.json")
	out, err := validateFilePath(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, safe) {
		t.Errorf("validated path %q not under safe base %q", out, safe)
	}
}

// TestValidateFilePath_DefaultsToSafePath verifies an empty input falls
// back to the canonical safe location.
func TestValidateFilePath_DefaultsToSafePath(t *testing.T) {
	_, safe := withFakeHome(t)

	out, err := validateFilePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(safe, "statusline.json")
	if out != want {
		t.Errorf("default path = %q, want %q", out, want)
	}
}

// TestValidateFilePath_SymlinkEscape ensures a symlink whose target is
// outside the safe base is rejected.
func TestValidateFilePath_SymlinkEscape(t *testing.T) {
	_, safe := withFakeHome(t)

	// Create a target outside the safe base.
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "evil.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	// Create a symlink inside safe pointing to outside.
	link := filepath.Join(safe, "linked.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	if _, err := validateFilePath(link); err == nil {
		t.Errorf("expected symlink-escape rejection, got nil error")
	}
}

// TestFilePoll_MalformedJSON checks malformed JSON is rejected.
func TestFilePoll_MalformedJSON(t *testing.T) {
	_, safe := withFakeHome(t)
	path := filepath.Join(safe, "statusline.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := NewFileSource(path)
	if !fs.enabled {
		t.Fatalf("expected source to be enabled")
	}
	// Force IsAvailable() to be true regardless of mtime check by re-touching.
	now := os.Getenv("CLAUDE_TEST_NOW")
	_ = now
	if _, err := fs.Poll(); err == nil {
		t.Errorf("expected malformed JSON to fail Poll()")
	}
}

// TestFilePoll_MissingRequiredFields checks missing fields rejected.
func TestFilePoll_MissingRequiredFields(t *testing.T) {
	_, safe := withFakeHome(t)
	path := filepath.Join(safe, "statusline.json")
	if err := os.WriteFile(path, []byte(`{"model":"opus"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := NewFileSource(path)
	// Bypass age-based IsAvailable for the test path
	fs.enabled = true

	_, err := fs.Poll()
	if err == nil {
		t.Errorf("expected error for missing required fields, got nil")
	} else if !strings.Contains(err.Error(), "required") && !strings.Contains(err.Error(), "available") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestFilePoll_ValidPayload exercises the happy path.
func TestFilePoll_ValidPayload(t *testing.T) {
	_, safe := withFakeHome(t)
	path := filepath.Join(safe, "statusline.json")
	if err := os.WriteFile(path, []byte(`{"input_tokens":100,"output_tokens":200}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := NewFileSource(path)
	if !fs.IsAvailable() {
		t.Fatalf("expected source to be available")
	}
	data, err := fs.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if data.InputTokens != 100 || data.OutputTokens != 200 {
		t.Errorf("got %+v, want input=100 output=200", data)
	}
}

// TestFilePoll_NegativeTokens_Rejected ensures invalid ranges fail.
func TestFilePoll_NegativeTokens_Rejected(t *testing.T) {
	_, safe := withFakeHome(t)
	path := filepath.Join(safe, "statusline.json")
	if err := os.WriteFile(path, []byte(`{"input_tokens":-5,"output_tokens":1}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := NewFileSource(path)
	fs.enabled = true
	if _, err := fs.Poll(); err == nil {
		t.Errorf("expected negative-token rejection")
	}
}
