package localgateway

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// activityState observes local adapter attempts only. The inference semaphore
// keeps active bounded to one; completions retain metadata for just one attempt.
type activityState struct {
	mu     sync.Mutex
	queued int
	active *activityAttempt
	last   *activityCompletion
}

type activityAttempt struct {
	RequestID string    `json:"request_id"`
	AttemptID string    `json:"attempt_id"`
	Client    string    `json:"client"`
	Role      string    `json:"role"`
	Model     string    `json:"model"`
	Upstream  string    `json:"upstream"`
	Thinking  bool      `json:"thinking"`
	MaxTokens int       `json:"max_tokens"`
	StartedAt time.Time `json:"started_at"`
}

type activityCompletion struct {
	activityAttempt
	FinishedAt time.Time      `json:"finished_at"`
	LatencyMS  int64          `json:"latency_ms"`
	Accepted   bool           `json:"accepted"`
	Usage      map[string]int `json:"usage,omitempty"`
}

func (a *activityState) queue(delta int) {
	a.mu.Lock()
	a.queued += delta
	a.mu.Unlock()
}

func (a *activityState) start(attempt activityAttempt) {
	a.mu.Lock()
	a.active = &attempt
	a.mu.Unlock()
}

func (a *activityState) finish(attempt activityAttempt, accepted bool, usage map[string]int) {
	finished := time.Now().UTC()
	completion := &activityCompletion{activityAttempt: attempt, FinishedAt: finished, LatencyMS: finished.Sub(attempt.StartedAt).Milliseconds(), Accepted: accepted}
	if len(usage) > 0 {
		completion.Usage = make(map[string]int, len(usage))
		for key, value := range usage {
			completion.Usage[key] = value
		}
	}
	a.mu.Lock()
	a.active = nil
	a.last = completion
	a.mu.Unlock()
}

func (g *Gateway) activity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET")
		return
	}
	// Copy under the lock, then encode without holding up inference. Completion
	// records and their usage maps are immutable after publication.
	g.activityState.mu.Lock()
	active := []activityAttempt{}
	if g.activityState.active != nil {
		active = append(active, *g.activityState.active)
	}
	snapshot := struct {
		Scope         string              `json:"scope"`
		Queued        int                 `json:"queued"`
		Active        []activityAttempt   `json:"active"`
		LastCompleted *activityCompletion `json:"last_completed"`
	}{"local-adapter-attempts", g.activityState.queued, active, g.activityState.last}
	g.activityState.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(snapshot)
}
