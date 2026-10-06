package labdashboard

import (
	"encoding/json"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/labstatus"
)

const historyWindow = 15 * time.Minute

type runtimePoint struct {
	CacheReuseRatio   *float64 `json:"cache_reuse_ratio"`
	ColdTTFT          *float64 `json:"cold_ttft_ms"`
	WarmTTFT          *float64 `json:"warm_ttft_ms"`
	Model             string   `json:"model"`
	Decode            *float64 `json:"decode_tps"`
	TokensPerSecond   *float64 `json:"tokens_per_s"`
	RequestsPerMinute *float64 `json:"requests_per_min"`
}
type historyPoint struct {
	At          float64                 `json:"at"`
	Runtimes    map[string]runtimePoint `json:"runtimes"`
	AvailableGB *float64                `json:"available_gb"`
	SwapGB      *float64                `json:"swap_gb"`
	Pressure    string                  `json:"pressure,omitempty"`
}
type observedAttempt struct {
	attemptID    string
	Role         string    `json:"role"`
	Client       string    `json:"client"`
	Upstream     string    `json:"upstream"`
	Thinking     bool      `json:"thinking"`
	FinishedAt   time.Time `json:"finished_at"`
	LatencyMS    int64     `json:"latency_ms"`
	Accepted     bool      `json:"accepted"`
	InputTokens  *int      `json:"input_tokens"`
	OutputTokens *int      `json:"output_tokens"`
}
type history struct {
	mu       sync.Mutex
	points   []historyPoint
	attempts []observedAttempt
	previous labstatus.Snapshot
}

func newHistory() *history { return &history{points: []historyPoint{}, attempts: []observedAttempt{}} }
func measurement(v any) *float64 {
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return nil
	}
	return &n
}
func (h *history) record(raw, activity json.RawMessage, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var s labstatus.Snapshot
	if json.Unmarshal(raw, &s) == nil && s.SampleTime > h.previous.SampleTime && s.SampleTime > 0 {
		p := historyPoint{At: s.SampleTime, Runtimes: map[string]runtimePoint{}}
		for _, name := range []string{"large", "small"} {
			r := s.Runtimes[name]
			if r == nil {
				continue
			}
			rp := runtimePoint{Model: r.Model}
			if !r.Stale && r.SampleTime == s.SampleTime {
				if p.AvailableGB == nil {
					p.AvailableGB = measurement(r.Memory["available_gb"])
					p.SwapGB = measurement(r.Memory["swap_used_gb"])
					p.Pressure, _ = r.Memory["pressure"].(string)
				}
				old := h.previous.Runtimes[name]
				if old != nil && !old.Stale && old.SampleTime == h.previous.SampleTime && s.RunID == h.previous.RunID && r.Model == old.Model && r.Endpoint == old.Endpoint {
					requests, ok := r.Stats["requests"]
					before, had := old.Stats["requests"]
					tokens, tokOK := r.Stats["tokens_generated"]
					oldTokens, oldTokOK := old.Stats["tokens_generated"]
					uptime, hasUptime := r.Stats["uptime_s"]
					oldUptime, hadUptime := old.Stats["uptime_s"]
					elapsed := s.SampleTime - h.previous.SampleTime
					if elapsed <= 8 && ok && had && tokOK && oldTokOK && requests >= before && tokens >= oldTokens && !(hasUptime && hadUptime && uptime < oldUptime) {
						reused, oldReused := measurement(r.PromptCache["reused_tokens"]), measurement(old.PromptCache["reused_tokens"])
						processed, oldProcessed := measurement(r.PromptCache["processed_tokens"]), measurement(old.PromptCache["processed_tokens"])
						if reused != nil && oldReused != nil && processed != nil && oldProcessed != nil && *reused >= *oldReused && *processed >= *oldProcessed {
							saved, worked := *reused-*oldReused, *processed-*oldProcessed
							if saved+worked > 0 {
								rp.CacheReuseRatio = measurement(saved / (saved + worked))
							}
						}
						rp.RequestsPerMinute = measurement((requests - before) * 60 / elapsed)
						rp.TokensPerSecond = measurement((tokens - oldTokens) / elapsed)
						if requests > before && r.LastGeneration["native_generation_metadata"] == true {
							rp.Decode = measurement(r.LastGeneration["generation_tps"])
							if cached := measurement(r.LastGeneration["cached_prompt_tokens"]); cached != nil {
								if *cached == 0 {
									rp.ColdTTFT = measurement(r.LastGeneration["ttft_ms"])
								} else {
									rp.WarmTTFT = measurement(r.LastGeneration["ttft_ms"])
								}
							}
						}
					}
				}
			}
			p.Runtimes[name] = rp
		}
		h.points = append(h.points, p)
		h.previous = s
	}
	var a struct {
		Last *struct {
			observedAttempt
			AttemptID string         `json:"attempt_id"`
			Usage     map[string]int `json:"usage"`
		} `json:"last_completed"`
	}
	if json.Unmarshal(activity, &a) == nil && a.Last != nil && a.Last.AttemptID != "" && !a.Last.FinishedAt.IsZero() {
		for _, event := range h.attempts {
			if event.attemptID == a.Last.AttemptID {
				h.prune(now)
				return
			}
		}
		event := a.Last.observedAttempt
		event.attemptID = a.Last.AttemptID
		if n, ok := a.Last.Usage["input_tokens"]; ok {
			event.InputTokens = &n
		}
		if n, ok := a.Last.Usage["output_tokens"]; ok {
			event.OutputTokens = &n
		}
		h.attempts = append(h.attempts, event)
		sort.SliceStable(h.attempts, func(i, j int) bool { return h.attempts[i].FinishedAt.Before(h.attempts[j].FinishedAt) })
	}
	h.prune(now)
}
func (h *history) prune(now time.Time) {
	cutoff := float64(now.Add(-historyWindow).UnixNano()) / 1e9
	for len(h.points) > 0 && (h.points[0].At < cutoff || len(h.points) > 900) {
		h.points = h.points[1:]
	}
	for len(h.attempts) > 0 && (h.attempts[0].FinishedAt.Before(now.Add(-historyWindow)) || len(h.attempts) > 100) {
		h.attempts = h.attempts[1:]
	}
}
func (h *history) snapshot(now time.Time) ([]historyPoint, []observedAttempt) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prune(now)
	return append([]historyPoint{}, h.points...), append([]observedAttempt{}, h.attempts...)
}
