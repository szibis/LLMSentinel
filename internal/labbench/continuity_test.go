package labbench

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNativeAttributionRequiresBothUptimeObservations(t *testing.T) {
	for _, missing := range []string{"before", "after", "both"} {
		t.Run(missing, func(t *testing.T) {
			s := attributedSample()
			if missing != "after" {
				s.Before.Uptime = nil
			}
			if missing != "before" {
				s.After.Uptime = nil
			}
			var a, b activity
			raw, _ := json.Marshal(map[string]any{"last_completed": map[string]any{"attempt_id": "new", "role": s.Role, "accepted": true, "started_at": s.StartedAt, "finished_at": s.StartedAt.Add(time.Second)}})
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			e, ttft, decode, _ := cacheEvidence(s.Before, s.After, &a, &b, s.StartedAt, s.Role)
			if e.Status != "unknown" || e.CachedTokens != nil || ttft != nil || decode != nil {
				t.Fatalf("live missing continuity accepted: %+v", e)
			}
			report, err := Load(persistedReport(t, s))
			if err != nil {
				t.Fatal(err)
			}
			got := report.Samples[0]
			if got.Cache.Status != "unknown" || got.Cache.CachedTokens != nil || got.TTFT != nil || got.NativeDecode != nil {
				t.Fatalf("persisted missing continuity accepted: %+v", got)
			}
		})
	}
}
