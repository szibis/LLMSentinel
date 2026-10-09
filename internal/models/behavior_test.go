package models

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type modelRoundTripper func(*http.Request) (*http.Response, error)

func (f modelRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedModelBody struct {
	io.Reader
	closed *int
}

func (b *trackedModelBody) Close() error { *b.closed++; return nil }
func offlineModelManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	for _, id := range []string{"intent-fixture", "anomaly-fixture", "embedding-fixture"} {
		if err := os.WriteFile(filepath.Join(dir, id+".onnx"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &ModelConfig{CachePath: dir, Intent: ModelSubConfig{Enabled: true, ModelID: "intent-fixture", Source: "local", InferenceTimeout: time.Second}, SecurityAnomaly: ModelSubConfig{Enabled: true, ModelID: "anomaly-fixture", Source: "local", InferenceTimeout: time.Second}, SemanticEmbeddings: ModelSubConfig{Enabled: true, ModelID: "embedding-fixture", Source: "local", InferenceTimeout: time.Second}}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestModelsOfflineLoadInferHealthAndUnload(t *testing.T) {
	m := offlineModelManager(t)
	ctx := context.Background()
	for _, typ := range []ModelType{ModelTypeIntent, ModelTypeAnomalyDetect, ModelTypeEmbedding} {
		for i := 0; i < 2; i++ {
			got, err := m.Infer(ctx, typ, "query")
			if err != nil || got == nil {
				t.Fatalf("infer %q=%v %v", typ, got, err)
			}
		}
		if !m.Health()[string(typ)] {
			t.Errorf("model %q not healthy", typ)
		}
	}
	model, err := m.LoadModel(ctx, ModelTypeIntent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(model.Path); err != nil {
		t.Fatal(err)
	}
	if m.Health()["intent"] {
		t.Fatal("missing model reported healthy")
	}
	if err := m.UnloadModel(ModelTypeIntent); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetLoadedModels()["intent"]; ok {
		t.Fatal("model remains loaded")
	}
	m.cacheMu.RLock()
	_, intentCached := m.inferenceCache["intent:query"]
	_, embedCached := m.inferenceCache["embedding:query"]
	m.cacheMu.RUnlock()
	if intentCached || !embedCached {
		t.Fatal("unload cache isolation failed")
	}
	if _, err := m.LoadModel(ctx, ModelType("unknown")); err == nil {
		t.Fatal("unknown model type accepted")
	}
	m.config.Intent.Enabled = false
	if _, err := m.Infer(ctx, ModelTypeIntent, "query"); err == nil {
		t.Fatal("disabled model accepted")
	}
	if _, err := m.createInferenceFunc(ModelType("unknown"))(ctx, "query"); err == nil {
		t.Fatal("unknown inference function accepted")
	}
}
func TestDownloadModelCachedAndFilesystemContracts(t *testing.T) {
	dir := t.TempDir()
	dm := NewDownloadManager(dir)
	dm.client = &http.Client{Transport: modelRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("model bytes")), Request: r}, nil
	})}
	path, err := dm.EnsureModel(context.Background(), "distilbert-base-uncased", "huggingface")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "model bytes" {
		t.Fatalf("model=%q %v", data, err)
	}
	sum := sha256.Sum256(data)
	if err := dm.VerifyModelIntegrity(path, fmt.Sprintf("%x", sum)); err != nil {
		t.Fatal(err)
	}
	if err := dm.VerifyModelIntegrity(path, "wrong"); err == nil {
		t.Fatal("wrong integrity accepted")
	}
	if err := dm.VerifyModelIntegrity(filepath.Join(dir, "missing"), "wrong"); err == nil {
		t.Fatal("missing integrity accepted")
	}
	if err := dm.VerifyModelIntegrity(dir, "wrong"); err == nil {
		t.Fatal("directory integrity accepted")
	}
	if got, err := dm.EnsureModel(context.Background(), "distilbert-base-uncased", "local"); err != nil || got != path {
		t.Fatal("cached model missed")
	}
	if got, err := dm.findLocalModel("distilbert-base-uncased"); err != nil || got != path {
		t.Fatal("local model missed")
	}
	if _, err := dm.EnsureModel(context.Background(), "missing", "local"); err == nil {
		t.Fatal("missing local model accepted")
	}
	if _, err := dm.EnsureModel(context.Background(), "missing", "unknown"); err == nil {
		t.Fatal("unknown source accepted")
	}
	if _, err := dm.EnsureModel(context.Background(), "unknown", "huggingface"); err == nil {
		t.Fatal("unknown remote model accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.onnx"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("note"), 0600); err != nil {
		t.Fatal(err)
	}
	cached, err := dm.ListCachedModels()
	if err != nil || len(cached) != 1 {
		t.Fatalf("cached list=%v %v", cached, err)
	}
	size, err := dm.GetCacheSize()
	if err != nil || size != int64(len(data)+4) {
		t.Fatalf("size=%d %v", size, err)
	}
	if err := dm.ClearCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := dm.ListCachedModels(); err == nil {
		t.Fatal("missing cache list accepted")
	}
	if _, err := dm.GetCacheSize(); err == nil {
		t.Fatal("missing cache size accepted")
	}
}
func TestDownloadRetryClosesFailedBodiesAndCancels(t *testing.T) {
	dm := NewDownloadManager(t.TempDir())
	closed, calls := 0, 0
	dm.client = &http.Client{Transport: modelRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		status := 200
		if calls == 1 {
			status = 503
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: &trackedModelBody{Reader: strings.NewReader("model"), closed: &closed}, Request: r}, nil
	})}
	if _, err := dm.EnsureModel(context.Background(), "distilbert-base-uncased", "huggingface"); err != nil {
		t.Fatal(err)
	}
	if closed != 2 {
		t.Errorf("retry response bodies closed=%d want2", closed)
	}
	dm = NewDownloadManager(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	dm.client = &http.Client{Transport: modelRoundTripper(func(r *http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })}
	start := time.Now()
	_, err := dm.EnsureModel(ctx, "distilbert-base-uncased", "huggingface")
	if !errors.Is(err, context.Canceled) || time.Since(start) > 200*time.Millisecond {
		t.Errorf("download cancellation=%v duration=%v", err, time.Since(start))
	}
}
func TestDefaultManagerExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	m, err := NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.cachePath != filepath.Join(home, ".claude-escalate", "models") {
		t.Fatalf("default cache path=%q", m.cachePath)
	}
}
func TestModelIDCannotEscapeCacheDirectory(t *testing.T) {
	base := t.TempDir()
	cache := filepath.Join(base, "cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside.onnx"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDownloadManager(cache).EnsureModel(context.Background(), "../outside", "local"); err == nil {
		t.Fatal("model ID escaped cache root")
	}
}

func TestDisabledAutoDownloadDoesNotContactTransport(t *testing.T) {
	m, err := NewManager(&ModelConfig{CachePath: t.TempDir(), AutoDownload: false, Intent: ModelSubConfig{Enabled: true, ModelID: "distilbert-base-uncased", Source: "huggingface"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	m.downloadManager.client = &http.Client{Transport: modelRoundTripper(func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, context.Canceled })}
	if _, err := m.LoadModel(ctx, ModelTypeIntent); err == nil {
		t.Fatal("uncached missing model accepted")
	}
	if calls != 0 {
		t.Errorf("AutoDownload=false made %d transport calls", calls)
	}
}
func TestLoadedModelConcurrentInferenceAndSnapshots(t *testing.T) {
	m := offlineModelManager(t)
	if _, err := m.LoadModel(context.Background(), ModelTypeIntent); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				m.Infer(context.Background(), ModelTypeIntent, "query")
				for _, model := range m.GetLoadedModels() {
					_ = model.LastUsed
				}
			}
		}()
	}
	wg.Wait()
}

func TestOfflineLoadCannotEscapeCacheAndReturnsSnapshot(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	os.Mkdir(cache, 0700)
	os.WriteFile(filepath.Join(dir, "outside.onnx"), []byte("outside"), 0600)
	cfg := &ModelConfig{CachePath: cache, Intent: ModelSubConfig{Enabled: true, ModelID: "../outside", Source: "local"}}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.LoadModel(context.Background(), ModelTypeIntent); err == nil {
		t.Fatal("offline load escaped cache")
	}
	m = offlineModelManager(t)
	model, err := m.LoadModel(context.Background(), ModelTypeIntent)
	if err != nil {
		t.Fatal(err)
	}
	model.LastUsed = time.Time{}
	model.Inference = nil
	next, err := m.LoadModel(context.Background(), ModelTypeIntent)
	if err != nil || next.LastUsed.IsZero() || next.Inference == nil {
		t.Fatal("caller mutated manager state", next, err)
	}
}
