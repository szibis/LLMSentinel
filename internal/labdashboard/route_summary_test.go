package labdashboard

import (
	"testing"
	"time"
)

func TestRouteHistorySeparatesObservedPathsAndInvalidLatency(t *testing.T) {
	now := time.Now()
	events := []observedAttempt{{Role: "sonnet", Upstream: "large", LatencyMS: 10, Accepted: true, FinishedAt: now}, {Role: "sonnet", Upstream: "large", LatencyMS: 30, Accepted: false, FinishedAt: now}, {Role: "sonnet", Upstream: "other", LatencyMS: 100, Accepted: true, FinishedAt: now}, {Role: "opus", Upstream: "large", LatencyMS: -1, Accepted: true, FinishedAt: now}}
	rows := routeSummary(events)
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row.Role == "sonnet" && row.Upstream == "large" {
			if row.Count != 2 || row.Accepted != 1 || row.P50 == nil || *row.P50 != 20 || row.P95 == nil || *row.P95 != 29 {
				t.Fatal(row)
			}
		}
		if row.Role == "opus" && (row.P50 != nil || row.P95 != nil) {
			t.Fatal("negative fabricated latency", row)
		}
	}
	if got := routeSummary(nil); len(got) != 0 {
		t.Fatal(got)
	}
}
