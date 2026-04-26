package models

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DownloadManager handles downloading and caching models
type DownloadManager struct {
	cachePath string
	client    *http.Client
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(cachePath string) *DownloadManager {
	return &DownloadManager{
		cachePath: cachePath,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// EnsureModel ensures a model is downloaded and cached
func (dm *DownloadManager) EnsureModel(ctx context.Context, modelID, source string) (string, error) {
	// Generate cache filename
	cacheFile := filepath.Join(dm.cachePath, modelID+".onnx")

	// Check if already cached
	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, nil
	}

	// Download model
	switch source {
	case "huggingface":
		return dm.downloadFromHuggingFace(ctx, modelID, cacheFile)
	case "local":
		// Local models might be pre-installed
		return dm.findLocalModel(modelID)
	default:
		return "", fmt.Errorf("unknown model source: %s", source)
	}
}

// downloadFromHuggingFace downloads a model from HuggingFace Hub
func (dm *DownloadManager) downloadFromHuggingFace(ctx context.Context, modelID, cacheFile string) (string, error) {
	// Map model IDs to HuggingFace URLs
	urls := map[string]string{
		"distilbert-base-uncased": "https://huggingface.co/distilbert-base-uncased/resolve/main/model.onnx",
		"all-MiniLM-L6-v2":        "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx",
	}

	modelURL, exists := urls[modelID]
	if !exists {
		return "", fmt.Errorf("unknown model ID: %s", modelID)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", modelURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Download with timeout and retry
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = dm.client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Second * time.Duration(attempt+1))
		}
	}

	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// Create cache directory
	if err := os.MkdirAll(dm.cachePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache dir: %w", err)
	}

	// Write to temporary file first
	tmpFile := cacheFile + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()

	// Copy with progress tracking
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("download write failed: %w", err)
	}

	// Atomic move
	if err := os.Rename(tmpFile, cacheFile); err != nil {
		return "", fmt.Errorf("failed to finalize cache: %w", err)
	}

	return cacheFile, nil
}

// findLocalModel finds a model in local system paths
func (dm *DownloadManager) findLocalModel(modelID string) (string, error) {
	// Check in cache path first
	cacheFile := filepath.Join(dm.cachePath, modelID+".onnx")
	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, nil
	}

	// Check in common system paths
	paths := []string{
		"/usr/local/share/claude-escalate/models",
		"/opt/claude-escalate/models",
	}

	for _, path := range paths {
		modelPath := filepath.Join(path, modelID+".onnx")
		if _, err := os.Stat(modelPath); err == nil {
			return modelPath, nil
		}
	}

	return "", fmt.Errorf("model not found: %s", modelID)
}

// VerifyModelIntegrity checks if a downloaded model is valid
func (dm *DownloadManager) VerifyModelIntegrity(modelPath, expectedHash string) error {
	f, err := os.Open(modelPath)
	if err != nil {
		return fmt.Errorf("failed to open model: %w", err)
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	actualHash := fmt.Sprintf("%x", hash.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// ListCachedModels lists all models in the cache
func (dm *DownloadManager) ListCachedModels() ([]string, error) {
	entries, err := os.ReadDir(dm.cachePath)
	if err != nil {
		return nil, err
	}

	var models []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".onnx" {
			models = append(models, entry.Name())
		}
	}

	return models, nil
}

// ClearCache removes all cached models
func (dm *DownloadManager) ClearCache() error {
	return os.RemoveAll(dm.cachePath)
}

// GetCacheSize returns the total size of cached models in bytes
func (dm *DownloadManager) GetCacheSize() (int64, error) {
	var totalSize int64

	entries, err := os.ReadDir(dm.cachePath)
	if err != nil {
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
		}
	}

	return totalSize, nil
}
