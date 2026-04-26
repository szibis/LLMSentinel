package statusline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileSource polls a JSON file for metrics.
// Useful for custom integrations that write to a file.
type FileSource struct {
	path    string
	enabled bool
}

// validateFilePath ensures the path is within the safe directory (prevents traversal and symlink attacks).
func validateFilePath(configuredPath string) (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return "", fmt.Errorf("HOME environment variable not set")
	}

	// Define safe base directory
	safeBase := filepath.Join(homeDir, ".claude", "data", "escalation")

	// Clean the path to resolve .. and .
	if configuredPath == "" {
		configuredPath = filepath.Join(safeBase, "statusline.json")
	}

	// Get absolute path
	absPath, err := filepath.Abs(configuredPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	absSafeBase, err := filepath.Abs(safeBase)
	if err != nil {
		return "", fmt.Errorf("invalid safe base: %w", err)
	}

	// Ensure the path is within safe base directory (prefix check)
	cleanPath := filepath.Clean(absPath)
	cleanBase := filepath.Clean(absSafeBase)
	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) &&
		cleanPath != cleanBase {
		return "", fmt.Errorf("path outside allowed directory: %s", absPath)
	}

	// Resolve symlinks and re-validate (prevents symlink escape attacks).
	// Also resolve the safe base so platform-level symlinks (e.g. /var -> /private/var
	// on macOS) don't cause false positives.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// If EvalSymlinks succeeded (file exists), re-validate the real path
	if err == nil {
		cleanReal := filepath.Clean(realPath)
		realBase := cleanBase
		if rb, rbErr := filepath.EvalSymlinks(cleanBase); rbErr == nil {
			realBase = filepath.Clean(rb)
		}
		if !strings.HasPrefix(cleanReal, realBase+string(filepath.Separator)) &&
			cleanReal != realBase {
			return "", fmt.Errorf("resolved path outside allowed directory: %s -> %s", absPath, realPath)
		}
	}

	return absPath, nil
}

// NewFileSource creates a file polling source.
func NewFileSource(path string) *FileSource {
	validatedPath, err := validateFilePath(path)
	if err != nil {
		return &FileSource{path: path, enabled: false}
	}

	// Check if file exists (without separate Stat call to avoid TOCTOU)
	file, err := os.Open(validatedPath)
	enabled := err == nil
	if file != nil {
		file.Close()
	}

	return &FileSource{
		path:    validatedPath,
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
		InputTokens         *int    `json:"input_tokens"`
		OutputTokens        *int    `json:"output_tokens"`
		CacheHitTokens      *int    `json:"cache_hit_tokens"`
		CacheCreationTokens *int    `json:"cache_creation_tokens"`
		ContextUsage        *int    `json:"context_usage_percent"`
		Model               *string `json:"model"`
		IsCaching           *bool   `json:"is_caching"`
		CachePercent        *float64 `json:"cache_fill_percent"`
	}

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fileMetrics); err != nil {
		return StatuslineData{}, fmt.Errorf("failed to parse file metrics: %w", err)
	}

	// Validate required fields are present
	if fileMetrics.InputTokens == nil || fileMetrics.OutputTokens == nil {
		return StatuslineData{}, fmt.Errorf("file metrics missing required token fields")
	}

	// Validate token ranges
	if *fileMetrics.InputTokens < 0 || *fileMetrics.OutputTokens < 0 {
		return StatuslineData{}, fmt.Errorf("file metrics contain negative token counts")
	}

	const maxTokens = 1000000
	if *fileMetrics.InputTokens > maxTokens || *fileMetrics.OutputTokens > maxTokens {
		return StatuslineData{}, fmt.Errorf("file token counts exceed maximum allowed: %d", maxTokens)
	}

	// Default missing optional fields
	cacheHit := 0
	if fileMetrics.CacheHitTokens != nil && *fileMetrics.CacheHitTokens >= 0 {
		cacheHit = *fileMetrics.CacheHitTokens
	}

	cacheCreate := 0
	if fileMetrics.CacheCreationTokens != nil && *fileMetrics.CacheCreationTokens >= 0 {
		cacheCreate = *fileMetrics.CacheCreationTokens
	}

	contextUsage := 0
	if fileMetrics.ContextUsage != nil && *fileMetrics.ContextUsage >= 0 && *fileMetrics.ContextUsage <= 100 {
		contextUsage = *fileMetrics.ContextUsage
	}

	model := ""
	if fileMetrics.Model != nil {
		model = *fileMetrics.Model
	}

	caching := false
	if fileMetrics.IsCaching != nil {
		caching = *fileMetrics.IsCaching
	}

	cachePercent := 0.0
	if fileMetrics.CachePercent != nil && *fileMetrics.CachePercent >= 0.0 && *fileMetrics.CachePercent <= 1.0 {
		cachePercent = *fileMetrics.CachePercent
	}

	return StatuslineData{
		Source:              fs.Name(),
		Timestamp:           time.Now(),
		InputTokens:         *fileMetrics.InputTokens,
		OutputTokens:        *fileMetrics.OutputTokens,
		CacheHitTokens:      cacheHit,
		CacheCreationTokens: cacheCreate,
		ContextWindowUsage:  contextUsage,
		Model:               model,
		IsCaching:           caching,
		CacheFillPercentage: cachePercent,
	}, nil
}
