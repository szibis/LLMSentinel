package optimization

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/szibis/claude-escalate/internal/batch"
)

func TestBatchInputSavingsIncludePercent(t *testing.T) {
	o := NewInputOptimizer()
	req := &PipelineRequest{Query: "Find all functions that have documentation", Params: map[string]interface{}{"include_metadata": false, "max_results": 100}}
	_, single, err := o.OptimizeInput(context.Background(), req)
	if err != nil || single.TotalTokens <= 0 || single.Percent <= 0 {
		t.Fatal(single, err)
	}
	requests, total, err := o.OptimizeInputBatch(context.Background(), []*PipelineRequest{req, req})
	if err != nil || len(requests) != 2 || total.TotalTokens != 2*single.TotalTokens || math.Abs(total.Percent-single.Percent) > 1e-9 {
		t.Fatalf("batch savings %+v single %+v err=%v", total, single, err)
	}
	if _, _, err := o.OptimizeInputBatch(context.Background(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	if _, _, err := o.OptimizeInputBatch(context.Background(), []*PipelineRequest{nil}); err == nil {
		t.Fatal("nil request accepted")
	}
	if o.GetOptimizationMetrics()["dedup"] == nil {
		t.Fatal("missing dedup stats")
	}
}

func TestOptimizationFailureAndMeasuredStats(t *testing.T) {
	c := NewParameterCompressor()
	if _, err := c.Compress(map[string]interface{}{"x": math.NaN()}); err == nil {
		t.Fatal("nonfinite params accepted")
	}
	if _, err := c.Decompress("{"); err == nil {
		t.Fatal("bad compressed data")
	}
	if _, err := c.AbbreviateKeys(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RemoveDefaults(nil); err != nil {
		t.Fatal(err)
	}
	if c.abbreviateKey("_") != "_" || c.expandKey("unknown") != "unknown" {
		t.Fatal("unknown abbreviation")
	}
	if !valuesEqual(nil, nil) || valuesEqual(nil, 1) || valuesEqual(1, nil) {
		t.Fatal("nil comparison")
	}
	for _, tc := range []struct {
		original, compressed string
		percent              float64
		bytes                int
	}{{"1234", "12", 50, 2}, {"", "", 0, 0}, {"1", "123", 0, -2}} {
		s := GetCompressionStats(tc.original, tc.compressed)
		if s.SavingsPercent != tc.percent || s.BytesSaved != tc.bytes {
			t.Fatal(s)
		}
	}
	if SortParamsByKey(nil) != nil {
		t.Fatal("nil params")
	}
	params := map[string]interface{}{"z": 2, "a": 1}
	sorted := SortParamsByKey(params)
	sorted["z"] = 3
	if params["z"] != 2 {
		t.Fatal("sort aliases params")
	}
	f := NewInputFormatter()
	if _, err := f.CompactJSON(nil); err == nil {
		t.Fatal("nil accepted")
	}
	if _, err := f.CompactJSON(&PipelineRequest{Params: map[string]interface{}{"bad": math.NaN()}}); err == nil {
		t.Fatal("marshal failure hidden")
	}
	if _, err := f.StructuredFormat(""); err == nil {
		t.Fatal("empty structured input")
	}
	s := f.GetStats("1234", "12")
	if s.SavingsPercent != 50 || f.GetStats("", "").SavingsPercent != 0 {
		t.Fatal(s)
	}
	rd := NewRequestDeduplicator()
	if _, err := rd.Hash(nil); err == nil {
		t.Fatal("nil hash accepted")
	}
	if _, err := rd.Hash(&PipelineRequest{Params: map[string]interface{}{"bad": math.NaN()}}); err == nil {
		t.Fatal("bad params hash")
	}
	m := NewMetrics()
	if m.GetBatchStats()["batch_rate"] != float64(0) {
		t.Fatal("empty batch rate")
	}
	m.RecordBatchDecision("synthetic", "haiku", true)
	m.RecordCacheHit("synthetic", "haiku")
	m.RecordDirect("synthetic", "haiku")
	m.RecordModelSwitch("synthetic", "opus", "haiku")
	if m.GetBatchStats()["events_count"] != 1 || m.GetCacheStats()["events_count"] != 1 || m.GetSwitchStats()["events_count"] != 1 {
		t.Fatal("lost events")
	}
	a := m.ExportRealWorldAnalysis()
	if a["total_requests"] == nil {
		t.Fatal(a)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatal(err)
	}
	o := NewOptimizer()
	o.SetBatchStrategy(batch.StrategyNever)
	if o.estimateCost("synthetic", "unpriced-model", 1) != 0 {
		t.Fatal("unknown costs invented")
	}
	if _, err := f.RemoveUnnecessaryWhitespace(strings.Repeat(" ", 5)); err != nil {
		t.Fatal(err)
	}
}
