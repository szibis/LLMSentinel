package observability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestOTELUnavailableTransportIsExplicit(t *testing.T) {
	metrics := NewPrometheusMetrics()
	e := NewOTELMetricsExporter(OTELConfig{}, metrics)
	if e.config.Interval <= 0 || e.config.TimeoutSeconds <= 0 || e.config.ExporterType == "" {
		t.Fatal("missing defaults")
	}
	if err := e.Start(context.Background()); err != nil || e.IsRunning() {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.UpdateConfig(OTELConfig{Enabled: true}); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatal(err)
	}
	if err := e.UpdateConfig(OTELConfig{Enabled: true, Endpoint: "http://unused.local"}); err == nil || e.IsRunning() {
		t.Fatal("unsupported transport claimed active export", err)
	}
	if err := e.pushMetrics(context.Background()); err == nil {
		t.Fatal("unsupported transport reported delivery")
	}
}

func TestOTELRecordsMatchObservedSnapshot(t *testing.T) {
	p := NewPrometheusMetrics()
	p.RecordRequest("synthetic-model", "code", 10, 0, .1, true, false)
	p.UpdateGauges(1, 2, 3, .1)
	p.SetPercentiles(1, 2, 3)
	e := NewOTELMetricsExporter(OTELConfig{}, p)
	records := e.buildOTLPMetrics(p.GetMetricsSnapshot())
	if len(records) != 5 {
		t.Fatal(records)
	}
	for _, record := range records {
		if record["name"] == "claude_escalate_requests_total" {
			points := record["sum"].(map[string]interface{})["dataPoints"].([]map[string]interface{})
			if points[0]["asInt"] != int64(1) {
				t.Fatal(record)
			}
		}
	}
	if _, err := json.Marshal(records); err != nil {
		t.Fatal(err)
	}
	if got := e.buildOTLPMetrics(map[string]interface{}{"total_requests": "unknown"}); len(got) != 0 {
		t.Fatal("unknown values manufactured", got)
	}
}
