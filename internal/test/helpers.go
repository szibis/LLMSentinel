package test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFixturesDir returns the path to test fixtures directory
func TestFixturesDir(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return filepath.Join(dir, "testdata")
}

// TempDir creates a temporary directory for test use
func TempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "claude-escalate-test-")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

// WriteTestFile writes content to a test file
func WriteTestFile(t *testing.T, dir, name, content string) string {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}

// ReadTestFile reads content from a test file
func ReadTestFile(t *testing.T, path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	return string(content)
}

// EnvVar sets an environment variable for the test duration
func EnvVar(t *testing.T, key, value string) string {
	old := os.Getenv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env var: %v", err)
	}
	t.Cleanup(func() {
		if old == "" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, old)
		}
	})
	return old
}

// AssertEqual checks if two values are equal
func AssertEqual(t *testing.T, got, want interface{}) {
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// AssertNotEqual checks if two values are not equal
func AssertNotEqual(t *testing.T, got, want interface{}) {
	if got == want {
		t.Errorf("got %v, don't want %v", got, want)
	}
}

// AssertTrue checks if condition is true
func AssertTrue(t *testing.T, condition bool, msg string) {
	if !condition {
		t.Errorf("expected true, %s", msg)
	}
}

// AssertFalse checks if condition is false
func AssertFalse(t *testing.T, condition bool, msg string) {
	if condition {
		t.Errorf("expected false, %s", msg)
	}
}

// AssertError checks if error is not nil
func AssertError(t *testing.T, err error, msg string) {
	if err == nil {
		t.Errorf("expected error: %s", msg)
	}
}

// AssertNoError checks if error is nil
func AssertNoError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// AssertStringContains checks if str contains substr
func AssertStringContains(t *testing.T, str, substr string) {
	if !contains(str, substr) {
		t.Errorf("string %q does not contain %q", str, substr)
	}
}

func contains(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
