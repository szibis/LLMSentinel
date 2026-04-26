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

	// Claude's native statusline JSON structure - using pointers for nil-detection.
	var nativeMetrics struct {
		ContextWindow *struct {
			Used  *int `json:"used"`
			Total *int `json:"total"`
		} `json:"context_window"`
		Model               *string  `json:"model"`
		InputTokens         *int     `json:"input_tokens"`
		OutputTokens        *int     `json:"output_tokens"`
		CacheHitTokens      *int     `json:"cache_hit_tokens"`
		CacheCreationTokens *int     `json:"cache_creation_tokens"`
		IsCaching           *bool    `json:"caching_enabled"`
		CacheFillPercent    *float64 `json:"cache_fill_percent"`
		UpdatedAt           *string  `json:"updated_at"`
	}

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&nativeMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse native statusline: %w", err)
	}

	// Validate required fields are present
	if nativeMetrics.InputTokens == nil || nativeMetrics.OutputTokens == nil {
		return StatuslineData{}, fmt.Errorf("native statusline missing required token fields")
	}

	// Validate token ranges to prevent integer overflow/underflow
	if *nativeMetrics.InputTokens < 0 || *nativeMetrics.OutputTokens < 0 {
		return StatuslineData{}, fmt.Errorf("native statusline contains negative token counts")
	}

	const maxTokens = 1000000
	if *nativeMetrics.InputTokens > maxTokens || *nativeMetrics.OutputTokens > maxTokens {
		return StatuslineData{}, fmt.Errorf("native statusline token counts exceed maximum allowed: %d", maxTokens)
	}

	// Default missing optional fields
	cacheHit := 0
	if nativeMetrics.CacheHitTokens != nil && *nativeMetrics.CacheHitTokens >= 0 {
		cacheHit = *nativeMetrics.CacheHitTokens
	}

	cacheCreate := 0
	if nativeMetrics.CacheCreationTokens != nil && *nativeMetrics.CacheCreationTokens >= 0 {
		cacheCreate = *nativeMetrics.CacheCreationTokens
	}

	// Calculate context usage percentage with overflow protection
	contextUsage := 0
	if nativeMetrics.ContextWindow != nil &&
		nativeMetrics.ContextWindow.Used != nil &&
		nativeMetrics.ContextWindow.Total != nil {
		used := *nativeMetrics.ContextWindow.Used
		total := *nativeMetrics.ContextWindow.Total
		if used < 0 || total < 0 {
			return StatuslineData{}, fmt.Errorf("native statusline context window contains negative values")
		}
		if total > 0 {
			// Prevent integer overflow: cap used at total
			if used > total {
				used = total
			}
			// Prevent multiplication overflow by using int64 math
			contextUsage = int((int64(used) * 100) / int64(total))
			if contextUsage < 0 || contextUsage > 100 {
				contextUsage = 0
			}
		}
	}

	model := ""
	if nativeMetrics.Model != nil {
		model = *nativeMetrics.Model
	}

	caching := false
	if nativeMetrics.IsCaching != nil {
		caching = *nativeMetrics.IsCaching
	}

	cachePercent := 0.0
	if nativeMetrics.CacheFillPercent != nil &&
		*nativeMetrics.CacheFillPercent >= 0.0 &&
		*nativeMetrics.CacheFillPercent <= 1.0 {
		cachePercent = *nativeMetrics.CacheFillPercent
	}

	return StatuslineData{
		Source:              ns.Name(),
		Timestamp:           time.Now(),
		InputTokens:         *nativeMetrics.InputTokens,
		OutputTokens:        *nativeMetrics.OutputTokens,
		CacheHitTokens:      cacheHit,
		CacheCreationTokens: cacheCreate,
		ContextWindowUsage:  contextUsage,
		Model:               model,
		IsCaching:           caching,
		CacheFillPercentage: cachePercent,
	}, nil
}
