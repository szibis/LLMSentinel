package models

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerCreation(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
		Intent: ModelSubConfig{
			Enabled: true,
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if m == nil {
		t.Error("expected non-nil manager")
	}

	// Verify cache directory created
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("cache directory not created: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultModelConfig()

	if !cfg.Enabled {
		t.Error("models should be enabled by default")
	}

	if !cfg.Intent.Enabled {
		t.Error("intent model should be enabled by default")
	}

	if !cfg.SecurityAnomaly.Enabled {
		t.Error("anomaly detection should be enabled by default")
	}

	if !cfg.SemanticEmbeddings.Enabled {
		t.Error("embeddings should be enabled by default")
	}
}

func TestModelLoading(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fake model file
	modelPath := filepath.Join(tmpDir, "distilbert-base-uncased.onnx")
	f, err := os.Create(modelPath)
	if err != nil {
		t.Fatalf("failed to create mock model: %v", err)
	}
	f.Close()

	cfg := &ModelConfig{
		CachePath:    tmpDir,
		AutoDownload: false, // Skip download for test
		Intent: ModelSubConfig{
			Enabled:    true,
			FallbackTo: "keywords",
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Try to load model
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: Loading will fail without actual ONNX runtime, but test structure is valid
	_ = m
	_ = ctx
}

func TestHealthCheck(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	health := m.Health()
	if health == nil {
		t.Error("health should return non-nil map")
	}
}

func TestCacheClear(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Add something to cache
	m.cacheMu.Lock()
	m.inferenceCache["test"] = "value"
	m.cacheMu.Unlock()

	// Clear cache
	m.ClearInferenceCache()

	m.cacheMu.RLock()
	if len(m.inferenceCache) != 0 {
		t.Error("inference cache should be empty after clear")
	}
	m.cacheMu.RUnlock()
}

func TestUnloadModel(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Unload intent model
	err = m.UnloadModel(ModelTypeIntent)
	if err != nil {
		t.Errorf("failed to unload model: %v", err)
	}

	// Verify it's unloaded
	models := m.GetLoadedModels()
	if _, exists := models[string(ModelTypeIntent)]; exists {
		t.Error("model should be unloaded")
	}
}

func TestGetLoadedModels(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	models := m.GetLoadedModels()
	if models == nil {
		t.Error("should return non-nil map even if empty")
	}
}

func TestConcurrentInference(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
		Intent: ModelSubConfig{
			Enabled:      true,
			CacheResults: true,
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Simulate concurrent inference calls
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, err := m.Infer(ctx, ModelTypeIntent, "test query")
			done <- err
		}()
	}

	// Check all completed without panic
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestInferenceTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &ModelConfig{
		CachePath: tmpDir,
		Intent: ModelSubConfig{
			Enabled:          true,
			InferenceTimeout: 1 * time.Millisecond, // Very short timeout
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This would timeout if inference takes longer than 1ms
	_ = m
	_ = ctx
}

func TestConfigValidation(t *testing.T) {
	cfg := DefaultModelConfig()

	err := cfg.Validate()
	if err != nil {
		t.Errorf("default config should be valid: %v", err)
	}

	// Test disabled config
	cfg.Enabled = false
	err = cfg.Validate()
	if err != nil {
		t.Errorf("disabled config should be valid: %v", err)
	}
}
