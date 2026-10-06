package labdashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHistoryUsesFreshCountersAndLeavesGaps(t *testing.T) {
	h := newHistory()
	sample := func(at int, requests, tokens int, stale bool, run string) {
		runtime := map[string]any{"sample_time": at, "stale": stale, "model": "Qwen", "endpoint": "large", "stats": map[string]any{"requests": requests, "tokens_generated": tokens}, "last_generation": map[string]any{"generation_tps": 40, "native_generation_metadata": true}, "memory": map[string]any{"available_gb": 12}}
		raw, _ := json.Marshal(map[string]any{"sample_time": at, "run_id": run, "runtimes": map[string]any{"large": runtime}})
		h.record(raw, nil, time.Unix(int64(at), 0))
	}
	sample(100, 1, 20, false, "first")
	sample(104, 2, 60, false, "first")
	sample(104, 2, 60, false, "first") // cached telemetry must not invent another point
	points, _ := h.snapshot(time.Unix(104, 0))
	if len(points) != 2 || points[0].Runtimes["large"].TokensPerSecond != nil || points[1].Runtimes["large"].TokensPerSecond == nil || *points[1].Runtimes["large"].TokensPerSecond != 10 || *points[1].Runtimes["large"].RequestsPerMinute != 15 || points[1].Runtimes["large"].Decode == nil || *points[1].Runtimes["large"].Decode != 40 {
		t.Fatalf("incorrect counter history: %+v", points)
	}
	sample(108, 2, 60, true, "first")
	sample(112, 3, 80, false, "first")
	sample(116, 0, 0, false, "restart")
	points, _ = h.snapshot(time.Unix(116, 0))
	for _, i := range []int{2, 3, 4} {
		if points[i].Runtimes["large"].TokensPerSecond != nil || points[i].Runtimes["large"].Decode != nil {
			t.Fatalf("stale/restart invented rates: %+v", points[i])
		}
	}
	if points[2].AvailableGB != nil {
		t.Fatal("stale memory must remain a gap")
	}
}

func TestHistoryBoundsAndSanitizesObservedAttempts(t *testing.T) {
	h := newHistory()
	activity := json.RawMessage(`{"last_completed":{"attempt_id":"one","role":"sonnet","client":"anthropic_messages","upstream":"http://127.0.0.1:19091/v1","finished_at":"1970-01-01T00:01:40Z","latency_ms":500,"accepted":true,"prompt":"PRIVATE","usage":{"input_tokens":10,"output_tokens":20}}}`)
	h.record(nil, activity, time.Unix(100, 0))
	h.record(nil, activity, time.Unix(101, 0))
	_, events := h.snapshot(time.Unix(101, 0))
	raw, _ := json.Marshal(events)
	if len(events) != 1 || events[0].Role != "sonnet" || events[0].OutputTokens == nil || *events[0].OutputTokens != 20 {
		t.Fatalf("events lost/duplicated: %s", raw)
	}
	for _, b := range []string{"PRIVATE", "prompt", "attempt_id"} {
		if strings.Contains(string(raw), b) {
			t.Fatalf("private field retained: %s", raw)
		}
	}
	points, events := h.snapshot(time.Unix(1100, 0))
	if len(points) != 0 || len(events) != 0 {
		t.Fatal("history exceeded retention")
	}
}

func TestHistoryBoundsSamplesAndAttempts(t *testing.T) {
	h := newHistory()
	now := time.Unix(1000, 0)
	for i := 0; i < 1000; i++ {
		h.points = append(h.points, historyPoint{At: 1000})
		h.attempts = append(h.attempts, observedAttempt{FinishedAt: now})
	}
	points, events := h.snapshot(now)
	if len(points) != 900 || len(events) != 100 {
		t.Fatalf("unbounded history: %d samples, %d events", len(points), len(events))
	}
}

func TestHistoryDeduplicatesOutOfOrderClientPolls(t *testing.T) {
	h := newHistory()
	activity := func(id, finished string) json.RawMessage {
		return json.RawMessage(`{"last_completed":{"attempt_id":"` + id + `","role":"sonnet","finished_at":"` + finished + `"}}`)
	}
	a := activity("a", "1970-01-01T00:01:40Z")
	b := activity("b", "1970-01-01T00:01:44Z")
	for _, raw := range []json.RawMessage{b, a, b, a} {
		h.record(nil, raw, time.Unix(105, 0))
	}
	_, events := h.snapshot(time.Unix(105, 0))
	if len(events) != 2 || events[0].FinishedAt.Unix() != 100 || events[1].FinishedAt.Unix() != 104 {
		t.Fatalf("concurrent poll order duplicated/reordered attempts: %+v", events)
	}
}
