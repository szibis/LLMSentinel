package statusline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NativeSource reads metrics from Claude Code's native statusline.
// Looks for ~/.claude/statusline.json or similar native exports.
type NativeSource struct {
	path    string
	enabled bool
}

// NewNativeSource creates a Claude native source.
func NewNativeSource(path string) *NativeSource {
	if path == "" {
		path = filepath.Join(os.Getenv("HOME"), ".claude", "statusline.json")
	}

	_, err := os.Stat(path)
	enabled := err == nil

	return &NativeSource{
		path:    path,
		enabled: enabled,
	}
}

// Name returns the source name.
func (ns *NativeSource) Name() string {
	return "claude-native"
}

// IsAvailable checks if native statusline is available.
func (ns *NativeSource) IsAvailable() bool {
	if !ns.enabled {
		return false
	}

	fi, err := os.Stat(ns.path)
	if err != nil {
		return false
	}

	// File must be less than 3 seconds old (fresh)
	return time.Since(fi.ModTime()) < 3*time.Second
}

// Priority returns native priority (2).
func (ns *NativeSource) Priority() int {
	return 2
}

// Poll reads metrics from Claude native statusline.
func (ns *NativeSource) Poll() (StatuslineData, error) {
	if !ns.IsAvailable() {
		return StatuslineData{}, fmt.Errorf("native statusline not available")
	}

	file, err := os.Open(ns.path)
	if err != nil {
		return StatuslineData{}, fmt.Errorf("failed to read native statusline: %w", err)
	}
	defer file.Close()

	// Claude's native statusline JSON structure
	var nativeMetrics struct {
		ContextWindow struct {
			Used  int `json:"used"`
			Total int `json:"total"`
		} `json:"context_window"`
		Model               string  `json:"model"`
		InputTokens         int     `json:"input_tokens"`
		OutputTokens        int     `json:"output_tokens"`
		CacheHitTokens      int     `json:"cache_hit_tokens"`
		CacheCreationTokens int     `json:"cache_creation_tokens"`
		IsCaching           bool    `json:"caching_enabled"`
		CacheFillPercent    float64 `json:"cache_fill_percent"`
		UpdatedAt           string  `json:"updated_at"`
	}

	if err := json.NewDecoder(file).Decode(&nativeMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse native statusline: %w", err)
	}

	// Calculate context usage percentage
	contextUsage := 0
	if nativeMetrics.ContextWindow.Total > 0 {
		contextUsage = (nativeMetrics.ContextWindow.Used * 100) / nativeMetrics.ContextWindow.Total
	}

	return StatuslineData{
		Source:              ns.Name(),
		Timestamp:           time.Now(),
		InputTokens:         nativeMetrics.InputTokens,
		OutputTokens:        nativeMetrics.OutputTokens,
		CacheHitTokens:      nativeMetrics.CacheHitTokens,
		CacheCreationTokens: nativeMetrics.CacheCreationTokens,
		ContextWindowUsage:  contextUsage,
		Model:               nativeMetrics.Model,
		IsCaching:           nativeMetrics.IsCaching,
		CacheFillPercentage: nativeMetrics.CacheFillPercent,
	}, nil
}
