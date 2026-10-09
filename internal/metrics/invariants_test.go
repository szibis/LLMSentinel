package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
)

func TestCollectorSnapshotIsolation(t *testing.T) {
	c := NewMetricsCollector()
	c.RecordCacheHit()
	c.RecordTokens(100, 20)
	c.RecordSecurityEvent("rate_limit")
	c.RecordLatency(1, 2, 3, 4, 5, 6)
	c.RecordOptimizerMetric("compress", 10, .5, false)
	before := c.GetMetrics()
	c.RecordCacheHit()
	c.RecordTokens(1, 2)
	c.RecordSecurityEvent("rate_limit")
	c.RecordLatency(2, 3, 4, 5, 6, 7)
	if before.CacheMetrics.TotalHits != 1 || before.TokenMetrics.TotalInputTokens != 100 || before.SecurityMetrics.RateLimitTriggered != 1 || before.LatencyMetrics.TotalMs != 21 {
		t.Fatal("snapshot changed after subsequent recording")
	}
	before.CacheMetrics.TotalHits = 999
	before.OptimizerMetrics["compress"].TokensSaved = 999
	after := c.GetMetrics()
	if after.CacheMetrics.TotalHits != 2 || after.OptimizerMetrics["compress"].TokensSaved != 10 {
		t.Fatal("caller can mutate collector")
	}
}

func TestHistoryRetentionAndAccounting(t *testing.T) {
	c := NewMetricsCollector()
	c.maxHistorySize = 2
	c.RecordFalsePositive()
	c.RecordTokenSavings(0)
	for i := 0; i < 3; i++ {
		c.RecordCacheHit()
		c.RecordRequest(i != 1)
		c.RecordOptimizerMetric("compress", 10, .5, i != 1)
		c.SaveSnapshot()
	}
	h := c.GetMetricsHistory()
	if len(h) != 2 || h[0].RequestCount != 2 || h[1].RequestCount != 3 || h[0].CacheMetrics.TotalHits != 2 || c.errorCount != 1 {
		t.Fatal(h)
	}
	h[0].CacheMetrics.TotalHits = 999
	if c.GetMetricsHistory()[0].CacheMetrics.TotalHits != 2 {
		t.Fatal("history exposed to caller mutation")
	}
	if c.Uptime() < 0 {
		t.Fatal("negative uptime")
	}
}

func TestEmitterAccountingAndExport(t *testing.T) {
	c := NewMetricsCollector()
	e := NewMetricsEmitter(c)
	ExampleCacheMetrics(e)
	ExampleTokenMetrics(e)
	ExampleRequestMetrics(e)
	ExampleSecurityMetrics(e)
	e.RecordRequest("error", "", "", 1)
	e.RecordSecurityEvent("unauthorized", "")
	e.CacheOperation("overall", "unknown")
	e.RecordTokens("unknown", 5, "")
	e.RecordSecurityEvent("unknown", "")
	ExampleQualityMetrics(e)
	ExampleGatewayStatus(e)
	ExampleLatencyMetrics(e)
	s := c.GetMetrics()
	if s.CacheMetrics.TotalHits != 2 || s.CacheMetrics.TotalMisses != 1 || s.CacheMetrics.FalsePositives != 1 || s.TokenMetrics.TotalInputTokens != 2000 || s.TokenMetrics.TotalOutputTokens != 500 || s.TokenMetrics.TokensSavedByOptimization != 5350 || s.RequestCount != 4 || c.errorCount != 1 || s.SecurityMetrics.UnauthorizedAttempts != 1 {
		t.Fatalf("accounting %+v", s)
	}
	p := NewPrometheusExporter(c)
	data := p.ExportJSON()
	tokens := data["tokens"].(map[string]interface{})
	if tokens["input_total"] != int64(2000) || tokens["saved_by_optimization"] != int64(5350) {
		t.Fatal(tokens)
	}
	if _, err := json.Marshal(data); err != nil {
		t.Fatal(err)
	}
	text := p.Export()
	if !strings.Contains(text, "claude_escalate_tokens_total{type=\"input\"} 2000\n") || !strings.Contains(text, "# TYPE claude_escalate_cache_hit_rate gauge") {
		t.Fatal("export missing measurements")
	}
	ExampleCostMetrics(e)
	nilEmitter := NewMetricsEmitter(nil)
	nilEmitter.CacheOperation("", "hit")
	nilEmitter.RecordTokens("input", 1, "")
	nilEmitter.RecordCost("burned", 1, "")
	nilEmitter.RecordLatency("total", 1)
	nilEmitter.RecordRequest("error", "", "", 0)
	nilEmitter.RecordSecurityEvent("rate_limit", "")
}

func TestExportsReportObservedMeasurementsOnly(t *testing.T) {
	c := NewMetricsCollector()
	c.RecordCacheHit()
	c.RecordTokens(17, 3)
	c.RecordTokenSavings(7)
	c.RecordSecurityEvent("injection_attempt")
	c.RecordRequest(false)
	c.RecordLatency(10, 20, 30, 40, 50, 60)
	output := NewPrometheusExporter(c).Export()
	for _, fragment := range []string{"layer=\"semantic\"", "layer=\"exact\"", "pattern=\"sql\"", "pattern=\"xss\"", "quantile=", "model=", "quality_score", "gateway_status", "memory_bytes"} {
		if strings.Contains(output, fragment) {
			t.Errorf("fabricated series %s", fragment)
		}
	}
	for _, line := range []string{"claude_escalate_cache_operations_total{layer=\"overall\",operation=\"hit\"} 1", "claude_escalate_tokens_total{type=\"saved\"} 7", "claude_escalate_requests_total 1", "claude_escalate_latency_seconds{stage=\"cache_lookup\"} 0.010000"} {
		if !strings.Contains(output, line) {
			t.Errorf("observed series missing %s", line)
		}
	}
	e := NewOpenTelemetryExporter(c, &config.OpenTelemetryTarget{Enabled: true})
	for _, m := range e.snapshotToMetrics(c.GetMetrics()) {
		if m.Attributes["layer"] == "semantic" || m.Attributes["quantile"] != "" || m.Attributes["pattern"] != "" || m.Attributes["model"] != "" || m.Name == "quality_score" || m.Name == "gateway_status" {
			t.Errorf("fabricated metric %+v", m)
		}
		if m.Name == "uptime_seconds" && m.Value > 5 {
			t.Errorf("fabricated uptime %+v", m)
		}
	}
}

func TestSessionAccountingAndProjection(t *testing.T) {
	s := NewSessionMetrics()
	if s.CalculateSavingsPercent() != 0 || s.GetDailySummary(time.Now()) != nil || s.ProjectMonthly(0) != (MonthlyProjection{}) {
		t.Fatal("empty session")
	}
	s.RecordBurnedTokens(100, 50, 10, 20, .3)
	s.RecordBurnedTokens(100, 50, 10, 20, .3)
	types := []string{"exact_dedup", "semantic_cache", "input_optimization", "output_optimization", "rtk_proxy", "batch_api", "knowledge_graph"}
	for _, typ := range types {
		s.RecordOptimizationSaving(typ, 10, .1, 2)
	}
	daily := s.GetDailySummary(time.Now())
	if daily == nil || daily.RequestCount != 2 || daily.Burned.TotalTokens != 300 || daily.Saved.TotalTokensSaved != 70 {
		t.Fatal(daily)
	}
	breakdown := []TokenOptimizationStat{s.TotalBreakdown.ExactDedup, s.TotalBreakdown.SemanticCache, s.TotalBreakdown.InputOptimization, s.TotalBreakdown.OutputOptimization, s.TotalBreakdown.RTKProxy, s.TotalBreakdown.BatchAPI, s.TotalBreakdown.KnowledgeGraph}
	for _, stat := range breakdown {
		if stat.TokensSaved != 10 || stat.CostSavedUSD != .1 || stat.HitCount != 2 {
			t.Fatal(stat)
		}
	}
	if math.Abs(s.CalculateSavingsPercent()-70.0/370*100) > 1e-9 {
		t.Fatal("wrong savings")
	}
	p := s.ProjectMonthly(10)
	if p.ProjectedTokensBurned != 900 || p.ProjectedTokensSaved != 210 || p.BasedOnDays != 10 || math.Abs(p.ProjectedCostUSD-1.8) > 1e-9 {
		t.Fatal(p)
	}
	j := s.GetJSON()
	if j["requests"] != int64(2) {
		t.Fatal(j)
	}
	if _, err := json.Marshal(j); err != nil {
		t.Fatal(err)
	}
	// A degenerate baseline must not divide by zero.
	s.TotalSaved.TotalTokensSaved = -300
	if s.CalculateSavingsPercent() != 0 {
		t.Fatal("zero denominator")
	}
}

func TestDailySummaryIsIndependentSnapshot(t *testing.T) {
	s := NewSessionMetrics()
	s.RecordBurnedTokens(10, 20, 1, 2, .1)
	s.RecordOptimizationSaving("exact_dedup", 5, .01, 1)
	before := s.GetDailySummary(time.Now())
	s.RecordBurnedTokens(1, 2, 0, 0, .01)
	if before.Burned.TotalTokens != 30 {
		t.Fatal("daily summary changed after recording")
	}
	before.Burned.TotalTokens = 999
	before.Breakdown.ExactDedup.TokensSaved = 999
	after := s.GetDailySummary(time.Now())
	if after.Burned.TotalTokens != 33 || after.Breakdown.ExactDedup.TokensSaved != 5 {
		t.Fatal("caller can mutate daily accounting")
	}
}

func TestSessionConcurrentJSONAndRecordingTerminates(t *testing.T) {
	s := NewSessionMetrics()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			s.RecordBurnedTokens(1, 1, 0, 0, 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			s.GetJSON()
		}
	}()
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent JSON export and recording deadlocked")
	}
}

type captureTarget struct {
	mu     sync.Mutex
	values []map[string]interface{}
	err    error
	signal chan struct{}
}

func (c *captureTarget) Name() string { return "capture" }
func (c *captureTarget) Publish(v map[string]interface{}) error {
	c.mu.Lock()
	c.values = append(c.values, v)
	c.mu.Unlock()
	if c.signal != nil {
		select {
		case c.signal <- struct{}{}:
		default:
		}
	}
	return c.err
}

func TestPublisherLifecycleAndFailure(t *testing.T) {
	c := NewMetricsCollector()
	c.RecordTokens(10, 20)
	p := NewMetricsPublisher(c, time.Millisecond)
	good := &captureTarget{signal: make(chan struct{}, 1)}
	p.AddTarget(good)
	before := p.LastPublishTime()
	if err := p.PublishNow(); err != nil || p.LastPublishTime().Before(before) {
		t.Fatal(err)
	}
	if p.GetExportedJSON()["tokens"].(map[string]interface{})["output_total"] != int64(20) || !strings.Contains(p.GetExportedMetrics(), "type=\"output\"} 20") {
		t.Fatal("publisher lost tokens")
	}
	<-good.signal
	p.Start()
	select {
	case <-good.signal:
	case <-time.After(time.Second):
		t.Fatal("periodic publish did not run")
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if len(c.GetMetricsHistory()) == 0 {
		t.Fatal("periodic snapshot missing")
	}
	bad := &captureTarget{err: errors.New("disk full")}
	p.AddTarget(bad)
	if err := p.PublishNow(); err == nil || !strings.Contains(err.Error(), "capture") {
		t.Fatal(err)
	}
	p.publishOnce() // periodic publishing contains target failures
	stub := NewPrometheusServerPublisher("unused")
	if stub.Name() != "prometheus_server" || stub.Publish(nil) != nil {
		t.Fatal("stub contract")
	}
}

func TestLocalPublisherFilesAndFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	p, err := NewLocalFilePublisher(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "local_file" {
		t.Fatal(p.Name())
	}
	if err := p.Publish(map[string]interface{}{"tokens": 3}); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(map[string]interface{}{"tokens": 4}); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 2 {
		t.Fatal(string(data))
	}
	for _, line := range lines {
		var m map[string]int
		if err := json.Unmarshal(line, &m); err != nil || m["tokens"] < 3 {
			t.Fatal(string(line), err)
		}
	}
	st, err := os.Stat(files[0])
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal(st, err)
	}
	if err := p.Publish(map[string]interface{}{"bad": math.NaN()}); err == nil {
		t.Fatal("nonfinite metric accepted")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalFilePublisher(filepath.Join(blocker, "child")); err == nil {
		t.Fatal("invalid directory accepted")
	}
	p.logDir = blocker
	if err := p.Publish(nil); err == nil {
		t.Fatal("invalid log path accepted")
	}
	t.Setenv("HOME", "")
	if _, err := NewLocalFilePublisher("~/logs"); err == nil {
		t.Fatal("missing home accepted")
	}
}

func TestLocalPublisherRejectsEmptyDirectory(t *testing.T) {
	if _, err := NewLocalFilePublisher(""); err == nil {
		t.Fatal("empty directory accepted")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOTelSerializationBatchAndFailures(t *testing.T) {
	c := NewMetricsCollector()
	c.RecordCacheHit()
	c.RecordTokens(100, 20)
	c.RecordTokenSavings(10)
	c.RecordLatency(1, 2, 3, 4, 5, 6)
	c.RecordSecurityEvent("injection_attempt")
	c.RecordSecurityEvent("rate_limit")
	c.RecordRequest(true)
	if NewOpenTelemetryExporter(c, nil) != nil || NewOpenTelemetryExporter(c, &config.OpenTelemetryTarget{}) != nil {
		t.Fatal("disabled exporter created")
	}
	var nilExporter *OpenTelemetryExporter
	if nilExporter.Export() != nil || nilExporter.AddMetric(OTelMetric{}) != nil || nilExporter.sendPayload(nil) != nil {
		t.Fatal("disabled export")
	}
	cfg := &config.OpenTelemetryTarget{Enabled: true, OTLPEndpoint: "http://local", Headers: map[string]string{"X-Test": "yes"}}
	e := NewOpenTelemetryExporter(c, cfg)
	if cfg.ServiceName == "" || cfg.ServiceVersion == "" || cfg.BatchSize != 512 || cfg.BatchTimeout != 5000 || cfg.ExporterType != "otlp" {
		t.Fatal(cfg)
	}
	requests := make(chan OTelPayload, 10)
	e.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v1/metrics" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Test") != "yes" {
			return nil, fmt.Errorf("bad request %v", r)
		}
		var payload OTelPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, err
		}
		requests <- payload
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	if err := e.Export(); err != nil {
		t.Fatal(err)
	}
	payload := <-requests
	found := false
	for _, m := range payload.Metrics {
		if m.Name == "tokens_total" && m.Attributes["type"] == "input" {
			found = true
			if m.Value != 100 {
				t.Fatal(m)
			}
		}
	}
	if !found || payload.ServiceName != cfg.ServiceName {
		t.Fatal(payload)
	}
	cfg.BatchSize = 2
	e.flushBatch()
	if err := e.AddMetric(OTelMetric{Name: "custom", Value: 1}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddMetric(OTelMetric{Name: "custom", Value: 2}); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-requests:
		if len(p.Metrics) != 2 || p.Metrics[1].Value != 2 {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("batch did not flush")
	}
	e.lastFlushTime = time.Now().Add(-time.Minute)
	e.AddMetric(OTelMetric{Name: "timeout"})
	select {
	case p := <-requests:
		if len(p.Metrics) != 1 {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout did not flush")
	}
	e.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})
	if err := e.Export(); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatal(err)
	}
	e.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("network failed") })
	if err := e.Export(); err == nil || !strings.Contains(err.Error(), "network failed") {
		t.Fatal(err)
	}
	if err := e.sendPayload(&OTelPayload{Metrics: []OTelMetric{{Value: math.NaN()}}}); err == nil {
		t.Fatal("marshal failure ignored")
	}
	cfg.OTLPEndpoint = ":bad"
	if err := e.Export(); err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatal(err)
	}
	// Verify routing defaults without opening sockets.
	e.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	for _, typ := range []string{"otlp", "jaeger", "custom"} {
		cfg.ExporterType = typ
		cfg.OTLPEndpoint = ""
		cfg.JaegerEndpoint = ""
		if err := e.Export(); err != nil {
			t.Fatal(err)
		}
	}
	cfg.JaegerEndpoint = "http://local/traces"
	cfg.ExporterType = "jaeger"
	if err := e.Export(); err != nil {
		t.Fatal(err)
	}
	cfg.OTLPEndpoint = "http://local/custom"
	cfg.ExporterType = "custom"
	if err := e.Export(); err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = false
	if e.Export() != nil || e.AddMetric(OTelMetric{}) != nil || e.sendPayload(nil) != nil {
		t.Fatal("disabled send")
	}
}
