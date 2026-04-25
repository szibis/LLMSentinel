package statusline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileSource polls a JSON file for metrics.
// Useful for custom integrations that write to a file.
type FileSource struct {
	path    string
	enabled bool
}

// NewFileSource creates a file polling source.
func NewFileSource(path string) *FileSource {
	if path == "" {
		path = filepath.Join(os.Getenv("HOME"), ".claude", "data", "escalation", "statusline.json")
	}

	// Check if file exists
	_, err := os.Stat(path)
	enabled := err == nil

	return &FileSource{
		path:    path,
		enabled: enabled,
	}
}

// Name returns the source name.
func (fs *FileSource) Name() string {
	return "file"
}

// IsAvailable checks if file exists.
func (fs *FileSource) IsAvailable() bool {
	if !fs.enabled {
		return false
	}

	fi, err := os.Stat(fs.path)
	if err != nil {
		return false
	}

	// File must be less than 5 seconds old
	return time.Since(fi.ModTime()) < 5*time.Second
}

// Priority returns file priority (4).
func (fs *FileSource) Priority() int {
	return 4
}

// Poll reads metrics from JSON file.
func (fs *FileSource) Poll() (StatuslineData, error) {
	if !fs.IsAvailable() {
		return StatuslineData{}, fmt.Errorf("file source not available")
	}

	file, err := os.Open(fs.path)
	if err != nil {
		return StatuslineData{}, fmt.Errorf("failed to read file: %w", err)
	}
	defer file.Close()

	var fileMetrics struct {
		InputTokens         int     `json:"input_tokens"`
		OutputTokens        int     `json:"output_tokens"`
		CacheHitTokens      int     `json:"cache_hit_tokens"`
		CacheCreationTokens int     `json:"cache_creation_tokens"`
		ContextUsage        int     `json:"context_usage_percent"`
		Model               string  `json:"model"`
		IsCaching           bool    `json:"is_caching"`
		CachePercent        float64 `json:"cache_fill_percent"`
	}

	if err := json.NewDecoder(file).Decode(&fileMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse file metrics: %w", err)
	}

	return StatuslineData{
		Source:              fs.Name(),
		Timestamp:           time.Now(),
		InputTokens:         fileMetrics.InputTokens,
		OutputTokens:        fileMetrics.OutputTokens,
		CacheHitTokens:      fileMetrics.CacheHitTokens,
		CacheCreationTokens: fileMetrics.CacheCreationTokens,
		ContextWindowUsage:  fileMetrics.ContextUsage,
		Model:               fileMetrics.Model,
		IsCaching:           fileMetrics.IsCaching,
		CacheFillPercentage: fileMetrics.CachePercent,
	}, nil
}
