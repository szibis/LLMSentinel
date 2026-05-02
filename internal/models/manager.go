package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Model represents a loaded ML model
type Model struct {
	ID        string
	Type      ModelType
	Version   string
	Path      string
	LoadedAt  time.Time
	LastUsed  time.Time
	Inference InferenceFunc
}

// InferenceFunc is the function signature for model inference
type InferenceFunc func(ctx context.Context, input interface{}) (interface{}, error)

// ModelType enum for different model types
type ModelType string

const (
	ModelTypeIntent        ModelType = "intent"
	ModelTypeAnomalyDetect ModelType = "anomaly"
	ModelTypeEmbedding     ModelType = "embedding"
)

// Manager handles model lifecycle: download, load, cache, inference
type Manager struct {
	mu              sync.RWMutex
	loadedModels    map[string]*Model
	config          *ModelConfig
	cachePath       string
	downloadManager *DownloadManager
	inferenceCache  map[string]interface{} // Simple inference result cache
	cacheMu         sync.RWMutex
}

// NewManager creates a new model manager
func NewManager(cfg *ModelConfig) (*Manager, error) {
	if cfg == nil {
		cfg = DefaultModelConfig()
	}

	cachePath := cfg.CachePath
	if cachePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home dir: %w", err)
		}
		cachePath = filepath.Join(home, ".claude-escalate", "models")
	}

	// Create cache directory
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	dm := NewDownloadManager(cachePath)

	return &Manager{
		loadedModels:    make(map[string]*Model),
		config:          cfg,
		cachePath:       cachePath,
		downloadManager: dm,
		inferenceCache:  make(map[string]interface{}),
	}, nil
}

// LoadModel loads a model, downloading if necessary
func (m *Manager) LoadModel(ctx context.Context, modelType ModelType) (*Model, error) {
	m.mu.RLock()
	if model, exists := m.loadedModels[string(modelType)]; exists {
		m.mu.RUnlock()
		model.LastUsed = time.Now()
		return model, nil
	}
	m.mu.RUnlock()

	// Determine model config based on type
	var modelCfg ModelSubConfig
	switch modelType {
	case ModelTypeIntent:
		modelCfg = m.config.Intent
	case ModelTypeAnomalyDetect:
		modelCfg = m.config.SecurityAnomaly
	case ModelTypeEmbedding:
		modelCfg = m.config.SemanticEmbeddings
	default:
		return nil, fmt.Errorf("unknown model type: %s", modelType)
	}

	if !modelCfg.Enabled {
		return nil, fmt.Errorf("model %s is disabled", modelType)
	}

	// Download model if needed
	modelPath, err := m.downloadManager.EnsureModel(ctx, modelCfg.ModelID, modelCfg.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to download model: %w", err)
	}

	// Load model (stub for now - actual loading would use ONNX or TensorFlow)
	model := &Model{
		ID:       string(modelType),
		Type:     modelType,
		Version:  "1.0.0",
		Path:     modelPath,
		LoadedAt: time.Now(),
		LastUsed: time.Now(),
	}

	// Set inference function based on model type
	model.Inference = m.createInferenceFunc(modelType)

	m.mu.Lock()
	m.loadedModels[string(modelType)] = model
	m.mu.Unlock()

	return model, nil
}

// createInferenceFunc creates the appropriate inference function for a model type
func (m *Manager) createInferenceFunc(modelType ModelType) InferenceFunc {
	switch modelType {
	case ModelTypeIntent:
		return m.inferIntent
	case ModelTypeAnomalyDetect:
		return m.inferAnomaly
	case ModelTypeEmbedding:
		return m.inferEmbedding
	default:
		return func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, fmt.Errorf("unsupported model type")
		}
	}
}

// inferIntent performs intent classification
func (m *Manager) inferIntent(ctx context.Context, input interface{}) (interface{}, error) {
	// Check inference cache first
	m.cacheMu.RLock()
	cacheKey := fmt.Sprintf("intent:%v", input)
	if cached, exists := m.inferenceCache[cacheKey]; exists {
		m.cacheMu.RUnlock()
		return cached, nil
	}
	m.cacheMu.RUnlock()

	// TODO: Implement actual ONNX inference for DistilBERT
	// For now, return stub result
	result := map[string]interface{}{
		"intent":     "unknown",
		"confidence": 0.5,
	}

	m.cacheMu.Lock()
	m.inferenceCache[cacheKey] = result
	m.cacheMu.Unlock()

	return result, nil
}

// inferAnomaly performs security anomaly detection
func (m *Manager) inferAnomaly(ctx context.Context, input interface{}) (interface{}, error) {
	// Check cache
	m.cacheMu.RLock()
	cacheKey := fmt.Sprintf("anomaly:%v", input)
	if cached, exists := m.inferenceCache[cacheKey]; exists {
		m.cacheMu.RUnlock()
		return cached, nil
	}
	m.cacheMu.RUnlock()

	// TODO: Implement Isolation Forest anomaly detection
	result := map[string]interface{}{
		"is_anomaly": false,
		"score":      0.1,
	}

	m.cacheMu.Lock()
	m.inferenceCache[cacheKey] = result
	m.cacheMu.Unlock()

	return result, nil
}

// inferEmbedding performs semantic embedding
func (m *Manager) inferEmbedding(ctx context.Context, input interface{}) (interface{}, error) {
	// Check cache
	m.cacheMu.RLock()
	cacheKey := fmt.Sprintf("embedding:%v", input)
	if cached, exists := m.inferenceCache[cacheKey]; exists {
		m.cacheMu.RUnlock()
		return cached, nil
	}
	m.cacheMu.RUnlock()

	// TODO: Implement sentence-transformers embedding
	embedding := make([]float32, 384) // all-MiniLM-L6-v2 produces 384-dim vectors
	result := map[string]interface{}{
		"embedding": embedding,
		"dimension": 384,
	}

	m.cacheMu.Lock()
	m.inferenceCache[cacheKey] = result
	m.cacheMu.Unlock()

	return result, nil
}

// Infer performs inference on a loaded model
func (m *Manager) Infer(ctx context.Context, modelType ModelType, input interface{}) (interface{}, error) {
	model, err := m.LoadModel(ctx, modelType)
	if err != nil {
		return nil, err
	}

	// Create context with timeout if configured
	inferenceCtx := ctx
	var cancel context.CancelFunc

	var timeout time.Duration
	switch modelType {
	case ModelTypeIntent:
		timeout = m.config.Intent.InferenceTimeout
	case ModelTypeAnomalyDetect:
		timeout = m.config.SecurityAnomaly.InferenceTimeout
	case ModelTypeEmbedding:
		timeout = m.config.SemanticEmbeddings.InferenceTimeout
	}

	if timeout > 0 {
		inferenceCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return model.Inference(inferenceCtx, input)
}

// GetLoadedModels returns all currently loaded models
func (m *Manager) GetLoadedModels() map[string]*Model {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Model)
	for k, v := range m.loadedModels {
		result[k] = v
	}
	return result
}

// UnloadModel removes a model from memory
func (m *Manager) UnloadModel(modelType ModelType) error {
	m.mu.Lock()
	delete(m.loadedModels, string(modelType))
	m.mu.Unlock()

	// Clear inference cache for this model type
	m.cacheMu.Lock()
	prefix := fmt.Sprintf("%s:", modelType)
	for k := range m.inferenceCache {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(m.inferenceCache, k)
		}
	}
	m.cacheMu.Unlock()

	return nil
}

// ClearInferenceCache clears the inference result cache
func (m *Manager) ClearInferenceCache() {
	m.cacheMu.Lock()
	m.inferenceCache = make(map[string]interface{})
	m.cacheMu.Unlock()
}

// Health checks if models are healthy and accessible
func (m *Manager) Health() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health := make(map[string]bool)
	for modelType, model := range m.loadedModels {
		// Check if model file still exists
		_, err := os.Stat(model.Path)
		health[modelType] = err == nil
	}
	return health
}
