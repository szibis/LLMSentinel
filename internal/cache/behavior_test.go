package cache

import (
	"context"
	"errors"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/graph"
	"github.com/szibis/claude-escalate/internal/metrics"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixtureCacheLayer(t *testing.T) *Layer {
	t.Helper()
	cfg := &config.Config{}
	cfg.Optimizations.SemanticCache.Enabled = true
	cfg.Optimizations.SemanticCache.SimilarityThreshold = .99
	cfg.Optimizations.SemanticCache.MaxCacheSize = 100
	l, err := NewLayer(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func TestLayerExactKeysRespectToolParametersAndBoundaries(t *testing.T) {
	l := fixtureCacheLayer(t)
	ctx := context.Background()
	a := &Request{Content: "same", Tool: "read", Params: map[string]interface{}{"path": "a"}}
	if err := l.Store(ctx, a, &Response{Content: "response a"}); err != nil {
		t.Fatal(err)
	}
	b := &Request{Content: "same", Tool: "read", Params: map[string]interface{}{"path": "b"}}
	if _, hit, _ := l.LookupExact(b); hit {
		t.Error("different tool parameters reused cached response")
	}
	if err := l.Store(ctx, &Request{Content: "ab", Tool: "c"}, &Response{Content: "collision"}); err != nil {
		t.Fatal(err)
	}
	if _, hit, _ := l.LookupExact(&Request{Content: "a", Tool: "bc"}); hit {
		t.Error("content/tool concatenation collision")
	}
}
func TestLayerConcurrentExactAccess(t *testing.T) {
	l := fixtureCacheLayer(t)
	ctx := context.Background()
	req := &Request{Content: "same", Tool: "read"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				l.Store(ctx, req, &Response{Content: "value"})
				l.LookupExact(req)
				l.GetStats()
			}
		}()
	}
	wg.Wait()
}
func TestEmbeddingBatchContractsAndCancellation(t *testing.T) {
	e, err := NewEmbeddingModel(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	embs, err := e.EmbedBatch(context.Background(), []string{"one", "two", "one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(embs) != 3 || len(embs[0]) != e.EmbeddingDimension() || !reflect.DeepEqual(embs[0], embs[2]) || reflect.DeepEqual(embs[0], embs[1]) {
		t.Fatal("batch embedding determinism mismatch")
	}
	if _, err := e.EmbedBatch(context.Background(), []string{"valid", ""}); err == nil {
		t.Error("empty batch element accepted")
	}
	if _, err := NewEmbeddingModel(nil); err == nil {
		t.Error("nil embedding config accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Embed(ctx, "text"); !errors.Is(err, context.Canceled) {
		t.Errorf("embedding cancellation=%v", err)
	}
}

func TestLayerSemanticMetricsSafetyAndFailureBoundaries(t *testing.T) {
	l := fixtureCacheLayer(t)
	l.metrics = metrics.NewMetricsCollector()
	ctx := context.Background()
	req := &Request{Content: "query", Params: map[string]interface{}{"estimated_tokens": 100}}
	if err := l.Store(ctx, req, &Response{Content: "answer"}); err != nil {
		t.Fatal(err)
	}
	response, similarity, hit, err := l.LookupSemantic(ctx, req)
	if err != nil || !hit || response != "answer" || similarity < .99 {
		t.Fatalf("semantic lookup=%q %v %v %v", response, similarity, hit, err)
	}
	if _, hit, err := l.LookupExact(req); err != nil || !hit {
		t.Fatal("exact miss")
	}
	for _, intent := range []string{"quick_answer", "routine", "detailed_analysis", "learning", "follow_up", "cache_bypass", "unknown"} {
		got := l.EvaluateCacheSafety(intent)
		want := intent == "quick_answer" || intent == "routine"
		if got.Safe != want || got.Reason == "" {
			t.Fatalf("safety %q=%+v", intent, got)
		}
	}
	l.semanticCache.falsePositiveReportInterval = 1
	l.RecordFalsePositive()
	deadline := time.Now().Add(time.Second)
	for l.GetStats().SemanticHits == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stats := l.GetStats(); stats.FalsePositives != 1 || stats.CacheHealthy {
		t.Fatalf("false positive stats=%+v", stats)
	}
	l.Prune()
	l.Clear()
	if l.GetStats().ExactCacheSize != 0 {
		t.Fatal("exact clear failed")
	}
	if err := l.Store(ctx, nil, &Response{Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Store(ctx, req, nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Store(ctx, req, &Response{Content: "bad", Error: errors.New("provider failure")}); err != nil {
		t.Fatal(err)
	}
	if _, hit, _ := l.LookupExact(req); hit {
		t.Fatal("cached failed response")
	}
	bad := &Request{Content: "x", Params: map[string]interface{}{"unsupported": func() {}}}
	if err := l.Store(ctx, bad, &Response{Content: "x"}); err == nil {
		t.Fatal("unsupported cache parameters accepted")
	}
	if _, _, err := l.LookupExact(bad); err == nil {
		t.Fatal("unsupported lookup parameters accepted")
	}
	if _, _, _, err := l.LookupSemantic(ctx, &Request{}); err == nil {
		t.Fatal("empty semantic query accepted")
	}
	if _, _, _, err := l.LookupSemantic(ctx, nil); err != nil {
		t.Fatal(err)
	}
	l.semanticEnabled = false
	if _, _, hit, err := l.LookupSemantic(ctx, req); err != nil || hit {
		t.Fatal("disabled semantic lookup hit")
	}
	l.enabled = false
	if err := l.Store(ctx, req, &Response{Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, hit, _ := l.LookupExact(req); hit {
		t.Fatal("disabled exact lookup hit")
	}
	if _, err := NewLayer(nil, nil); err == nil {
		t.Fatal("nil config accepted")
	}
}
func TestGraphLayerCallerAndNodeAnswers(t *testing.T) {
	l := fixtureCacheLayer(t)
	g, err := graph.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	ctx := context.Background()
	cgl := NewCacheGraphLayer(l, g, nil)
	for _, n := range []*graph.Node{{ID: "Target", Name: "Target", Type: graph.NodeTypeFunction, Content: "target definition"}, {ID: "Caller", Name: "Caller", Type: graph.NodeTypeFunction, FilePath: "source.go", LineNumber: 2}} {
		if err := g.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.CreateEdge(ctx, &graph.Edge{ID: "edge", SourceID: "Caller", TargetID: "Target", RelationType: graph.RelationTypeCalls}); err != nil {
		t.Fatal(err)
	}
	got := cgl.Lookup(ctx, "who calls Target", "detailed_analysis")
	if got.Source != "graph" || !got.IsGraphMatch || len(got.RelatedNodes) != 1 || !strings.Contains(got.Content, "Caller") {
		t.Fatalf("graph callers=%+v", got)
	}
	got = cgl.Lookup(ctx, "uses Target", "detailed_analysis")
	if got.Source != "graph" || got.Content != "target definition" {
		t.Fatalf("graph node=%+v", got)
	}
	if got := cgl.Lookup(ctx, "who calls Missing", "detailed_analysis"); got.Source != "claude" {
		t.Fatalf("missing graph fallback=%+v", got)
	}
	if got := cgl.Lookup(ctx, "imports Missing", "detailed_analysis"); got.Source != "claude" {
		t.Fatalf("missing node fallback=%+v", got)
	}
	if got := cgl.Lookup(ctx, "dependencies", "detailed_analysis"); got.Source != "claude" {
		t.Fatalf("unextractable fallback=%+v", got)
	}
	if err := cgl.Clear(ctx, false); err != nil {
		t.Fatal(err)
	}
	stats, err := g.Stats()
	if err != nil || stats["node_count"] != 2 {
		t.Fatal("cache clear removed graph")
	}
	cgl.enabled = false
	if err := cgl.Store(ctx, "query", "answer"); err != nil {
		t.Fatal(err)
	}
	cgl.enabled = true
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if got := cgl.Lookup(ctx, "who calls Target", "detailed_analysis"); got.Source != "claude" {
		t.Fatal("closed graph did not fallback")
	}
}

func TestCosineSimilarityTinyVectorScaleInvariance(t *testing.T) {
	a := []float32{.000001, .000002}
	got, err := CosineSimilarity(a, a)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(got)-1) > 1e-5 {
		t.Fatalf("self similarity=%v", got)
	}
}

func TestLayerSemanticCannotReuseAnotherToolResource(t *testing.T) {
	layer := fixtureCacheLayer(t)
	ctx := context.Background()
	a := &Request{Tool: "read", Content: "read resource", Params: map[string]interface{}{"path": "a"}}
	if err := layer.Store(ctx, a, &Response{Content: "response a"}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []*Request{{Tool: "read", Content: "read resource", Params: map[string]interface{}{"path": "b"}}, {Tool: "write", Content: "read resource", Params: map[string]interface{}{"path": "a"}}, {Content: "read resource"}} {
		if _, found, err := layer.LookupExact(b); err != nil || found {
			t.Fatal("exact scope", found, err)
		}
		if response, _, found, err := layer.LookupSemantic(ctx, b); err != nil || found {
			t.Fatal("cross-resource semantic reuse", response, found, err)
		}
	}
}
