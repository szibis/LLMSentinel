package statusline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BaristaSource reads metrics from Barista statusline output.
// Barista writes JSON to ~/.claude/data/escalation/barista-metrics.json
type BaristaSource struct {
	configPath string
	dataPath   string
	enabled    bool
	lastRead   time.Time
}

// NewBaristaSource creates a Barista source.
func NewBaristaSource(configPath string) *BaristaSource {
	if configPath == "" {
		configPath = filepath.Join(os.Getenv("HOME"), ".claude", "barista.conf")
	}

	dataPath := filepath.Join(os.Getenv("HOME"), ".claude", "data", "escalation", "barista-metrics.json")

	// Check if Barista config exists
	_, err := os.Stat(configPath)
	enabled := err == nil

	return &BaristaSource{
		configPath: configPath,
		dataPath:   dataPath,
		enabled:    enabled,
		lastRead:   time.Now(),
	}
}

// Name returns the source name.
func (bs *BaristaSource) Name() string {
	return "barista"
}

// IsAvailable checks if Barista is configured.
func (bs *BaristaSource) IsAvailable() bool {
	if !bs.enabled {
		return false
	}

	// Check if metrics file exists and was recently updated
	fi, err := os.Stat(bs.dataPath)
	if err != nil {
		return false
	}

	// File must be less than 10 seconds old
	return time.Since(fi.ModTime()) < 10*time.Second
}

// Priority returns Barista priority (highest: 1).
func (bs *BaristaSource) Priority() int {
	return 1
}

// Poll reads metrics from Barista JSON file.
func (bs *BaristaSource) Poll() (StatuslineData, error) {
	if !bs.IsAvailable() {
		return StatuslineData{}, fmt.Errorf("barista source not available")
	}

	// Read metrics file
	file, err := os.Open(bs.dataPath)
	if err != nil {
		return StatuslineData{}, fmt.Errorf("failed to read barista metrics: %w", err)
	}
	defer file.Close()

	var baristaMetrics struct {
		InputTokens        int     `json:"input_tokens"`
		OutputTokens       int     `json:"output_tokens"`
		CacheHitTokens     int     `json:"cache_hit_tokens"`
		CacheCreationTokens int     `json:"cache_creation_tokens"`
		ContextUsage       int     `json:"context_usage_percent"`
		Model              string  `json:"model"`
		IsCaching          bool    `json:"is_caching"`
		CachePercent       float64 `json:"cache_fill_percent"`
		UpdatedAt          string  `json:"updated_at"`
	}

	if err := json.NewDecoder(file).Decode(&baristaMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse barista metrics: %w", err)
	}

	bs.lastRead = time.Now()

	return StatuslineData{
		Source:              bs.Name(),
		Timestamp:           time.Now(),
		InputTokens:         baristaMetrics.InputTokens,
		OutputTokens:        baristaMetrics.OutputTokens,
		CacheHitTokens:      baristaMetrics.CacheHitTokens,
		CacheCreationTokens: baristaMetrics.CacheCreationTokens,
		ContextWindowUsage:  baristaMetrics.ContextUsage,
		Model:               baristaMetrics.Model,
		IsCaching:           baristaMetrics.IsCaching,
		CacheFillPercentage: baristaMetrics.CachePercent,
	}, nil
}
