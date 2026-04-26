package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectToolsWithConfig(t *testing.T) {
	tests := []struct {
		name     string
		yamlFile string
		wantErr  bool
		check    func(t *testing.T, tools *DetectedTools)
	}{
		{
			name:     "valid config",
			yamlFile: "testdata/discovery_valid.yaml",
			wantErr:  false,
			check: func(t *testing.T, tools *DetectedTools) {
				if tools == nil {
					t.Error("expected non-nil DetectedTools")
				}
			},
		},
		{
			name:     "missing file",
			yamlFile: "testdata/nonexistent.yaml",
			wantErr:  true,
			check:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools, err := DetectToolsWithConfig(tt.yamlFile)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.check != nil && !tt.wantErr {
				tt.check(t, tools)
			}
		})
	}
}

func TestDetectTools(t *testing.T) {
	tools := DetectTools()

	if tools == nil {
		t.Error("expected non-nil DetectedTools")
		return
	}

	// At minimum, we should detect git
	if tools.GitPath == "" {
		t.Error("expected to detect git")
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, expanded string)
	}{
		{
			name: "tilde expansion",
			path: "~/test",
			check: func(t *testing.T, expanded string) {
				home, _ := os.UserHomeDir()
				if expanded != filepath.Join(home, "test") {
					t.Errorf("tilde not expanded correctly: %s", expanded)
				}
			},
		},
		{
			name: "no expansion needed",
			path: "/tmp/test",
			check: func(t *testing.T, expanded string) {
				if expanded != "/tmp/test" {
					t.Errorf("path should be unchanged: %s", expanded)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded := expandPath(tt.path)
			tt.check(t, expanded)
		})
	}
}

func TestFindTool(t *testing.T) {
	// Create temporary directory with a fake tool
	dir := t.TempDir()
	toolPath := filepath.Join(dir, "fake-tool")
	os.WriteFile(toolPath, []byte("#!/bin/sh\necho test"), 0o600)
	os.Chmod(toolPath, 0o700) // Make executable for test

	tests := []struct {
		name    string
		paths   []string
		wantErr bool
	}{
		{
			name:    "tool found with exact path",
			paths:   []string{toolPath},
			wantErr: false,
		},
		{
			name:    "tool found with glob pattern",
			paths:   []string{filepath.Join(dir, "*")},
			wantErr: false,
		},
		{
			name:    "tool not found",
			paths:   []string{"/nonexistent/path/to/tool"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := findTool(tt.paths)

			if tt.wantErr && found != "" {
				t.Error("expected empty result, got non-empty")
			}
			if !tt.wantErr && found == "" {
				t.Error("expected non-empty result, got empty")
			}
			if !tt.wantErr && found != toolPath {
				t.Logf("found tool: %s (expected %s) - pattern: %v", found, toolPath, tt.paths)
			}
		})
	}
}

// Benchmark test for concurrent path checking
func BenchmarkDetectTools(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DetectTools()
	}
}
