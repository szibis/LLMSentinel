// Package dashboard serves a local web UI for monitoring escalation analytics.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/store"
)

// Serve starts the dashboard HTTP server.
func Serve(cfg *config.Config) error {
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r, db)
	})
	mux.HandleFunc("/api/types", func(w http.ResponseWriter, r *http.Request) {
		handleTypes(w, r, db)
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		handleHistory(w, r, db)
	})
	mux.HandleFunc("/api/predictions", func(w http.ResponseWriter, r *http.Request) {
		handlePredictions(w, r, db, cfg)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok", "version": config.Version})
	})

	// Serve embedded dashboard UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(dashboardHTML))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.DashboardPort)
	fmt.Printf("claude-escalate dashboard running at http://%s\n", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func handleStats(w http.ResponseWriter, _ *http.Request, db *store.Store) {
	esc, deesc, turns, _ := db.TotalStats()
	settings, _ := config.ReadClaudeSettings()
	currentModel := "unknown"
	if settings != nil {
		currentModel = config.ModelShortName(settings.Model)
	}

	writeJSON(w, map[string]interface{}{
		"escalations":    esc,
		"de_escalations": deesc,
		"turns":          turns,
		"current_model":  currentModel,
		"version":        config.Version,
	})
}

func handleTypes(w http.ResponseWriter, _ *http.Request, db *store.Store) {
	stats, _ := db.TaskTypeStatsAll()
	writeJSON(w, stats)
}

func handleHistory(w http.ResponseWriter, _ *http.Request, db *store.Store) {
	events, _ := db.RecentEscalations(50)
	writeJSON(w, events)
}

func handlePredictions(w http.ResponseWriter, _ *http.Request, db *store.Store, cfg *config.Config) {
	type prediction struct {
		TaskType    string `json:"task_type"`
		Escalations int    `json:"escalations"`
		Active      bool   `json:"active"`
		Threshold   int    `json:"threshold"`
	}

	var predictions []prediction
	for _, tt := range []string{"concurrency", "parsing", "optimization", "debugging", "architecture", "security", "database", "networking", "testing", "devops"} {
		count, _ := db.EscalationCountForType(tt)
		if count > 0 {
			predictions = append(predictions, prediction{
				TaskType:    tt,
				Escalations: count,
				Active:      count >= cfg.PredictThreshold,
				Threshold:   cfg.PredictThreshold,
			})
		}
	}

	writeJSON(w, predictions)
}

// dashboardHTML is the embedded single-page dashboard.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>claude-escalate Dashboard</title>
<style>
  :root { --bg: #0d1117; --card: #161b22; --border: #30363d; --text: #e6edf3; --dim: #8b949e; --accent: #58a6ff; --green: #3fb950; --yellow: #d29922; --red: #f85149; --purple: #bc8cff; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; }
  .container { max-width: 1200px; margin: 0 auto; padding: 24px; }
  h1 { font-size: 24px; font-weight: 600; margin-bottom: 4px; }
  .subtitle { color: var(--dim); font-size: 14px; margin-bottom: 24px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-bottom: 24px; }
  .card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 20px; }
  .card-label { font-size: 12px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 8px; }
  .card-value { font-size: 32px; font-weight: 700; }
  .card-value.green { color: var(--green); }
  .card-value.yellow { color: var(--yellow); }
  .card-value.accent { color: var(--accent); }
  .card-value.purple { color: var(--purple); }
  .section { margin-bottom: 24px; }
  .section-title { font-size: 16px; font-weight: 600; margin-bottom: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
  table { width: 100%; border-collapse: collapse; }
  th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid var(--border); font-size: 14px; }
  th { color: var(--dim); font-weight: 500; font-size: 12px; text-transform: uppercase; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; font-weight: 500; }
  .badge-green { background: rgba(63,185,80,0.15); color: var(--green); }
  .badge-yellow { background: rgba(210,153,34,0.15); color: var(--yellow); }
  .badge-red { background: rgba(248,81,73,0.15); color: var(--red); }
  .badge-accent { background: rgba(88,166,255,0.15); color: var(--accent); }
  .model-indicator { display: inline-flex; align-items: center; gap: 6px; padding: 4px 12px; border-radius: 16px; font-weight: 600; font-size: 14px; }
  .model-haiku { background: rgba(88,166,255,0.1); color: var(--accent); }
  .model-sonnet { background: rgba(188,140,255,0.1); color: var(--purple); }
  .model-opus { background: rgba(63,185,80,0.1); color: var(--green); }
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px; }
  .refresh { color: var(--dim); font-size: 12px; cursor: pointer; }
  .refresh:hover { color: var(--accent); }
  .progress-bar { height: 6px; background: var(--border); border-radius: 3px; overflow: hidden; margin-top: 8px; }
  .progress-fill { height: 100%; border-radius: 3px; transition: width 0.3s; }
  .empty-state { text-align: center; padding: 48px; color: var(--dim); }
  .logo { font-size: 28px; margin-right: 8px; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <div>
      <h1><span class="logo">⚡</span>claude-escalate</h1>
      <div class="subtitle">Intelligent Model Escalation Dashboard</div>
    </div>
    <div>
      <span id="current-model" class="model-indicator model-haiku">Loading...</span>
      <div class="refresh" onclick="loadAll()" style="margin-top:8px;text-align:right">↻ Refresh</div>
    </div>
  </div>

  <div class="grid">
    <div class="card"><div class="card-label">Escalations</div><div id="stat-esc" class="card-value accent">—</div></div>
    <div class="card"><div class="card-label">De-escalations</div><div id="stat-deesc" class="card-value green">—</div></div>
    <div class="card"><div class="card-label">Success Rate</div><div id="stat-rate" class="card-value yellow">—</div></div>
    <div class="card"><div class="card-label">Turns Tracked</div><div id="stat-turns" class="card-value purple">—</div></div>
  </div>

  <div class="section card">
    <div class="section-title">Task Type Performance</div>
    <table>
      <thead><tr><th>Task Type</th><th>Escalations</th><th>Successes</th><th>Success Rate</th><th>Prediction</th></tr></thead>
      <tbody id="types-body"><tr><td colspan="5" class="empty-state">No data yet. Use Claude Code to generate escalation history.</td></tr></tbody>
    </table>
  </div>

  <div class="section card">
    <div class="section-title">Recent History</div>
    <table>
      <thead><tr><th>Time</th><th>From</th><th>To</th><th>Task Type</th><th>Reason</th></tr></thead>
      <tbody id="history-body"><tr><td colspan="5" class="empty-state">No escalation events yet.</td></tr></tbody>
    </table>
  </div>
</div>

<script>
async function loadAll() {
  try {
    const [stats, types, history, predictions] = await Promise.all([
      fetch('/api/stats').then(r => r.json()),
      fetch('/api/types').then(r => r.json()),
      fetch('/api/history').then(r => r.json()),
      fetch('/api/predictions').then(r => r.json()),
    ]);

    document.getElementById('stat-esc').textContent = stats.escalations;
    document.getElementById('stat-deesc').textContent = stats.de_escalations;
    document.getElementById('stat-turns').textContent = stats.turns;
    const rate = stats.escalations > 0 ? Math.round(stats.de_escalations / stats.escalations * 100) : 0;
    document.getElementById('stat-rate').textContent = rate + '%';

    const modelEl = document.getElementById('current-model');
    const m = stats.current_model || 'unknown';
    modelEl.textContent = m.charAt(0).toUpperCase() + m.slice(1);
    modelEl.className = 'model-indicator model-' + m;

    // Types table
    const predMap = {};
    if (predictions) predictions.forEach(p => predMap[p.task_type] = p);

    const tbody = document.getElementById('types-body');
    if (types && types.length > 0) {
      tbody.innerHTML = types.map(t => {
        const pred = predMap[t.TaskType];
        const predBadge = pred && pred.active
          ? '<span class="badge badge-green">Active</span>'
          : pred ? '<span class="badge badge-yellow">' + pred.escalations + '/' + pred.threshold + '</span>' : '—';
        return '<tr><td><strong>' + t.TaskType + '</strong></td><td>' + t.Escalations + '</td><td>' + t.Successes +
          '</td><td>' + Math.round(t.SuccessRate) + '%</td><td>' + predBadge + '</td></tr>';
      }).join('');
    }

    // History table
    const hbody = document.getElementById('history-body');
    if (history && history.length > 0) {
      hbody.innerHTML = history.slice(0, 20).map(e => {
        const time = new Date(e.Timestamp).toLocaleString(undefined, {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});
        const reasonBadge = e.Reason === 'success'
          ? '<span class="badge badge-green">success</span>'
          : e.Reason === 'user_command'
            ? '<span class="badge badge-accent">manual</span>'
            : '<span class="badge badge-yellow">' + e.Reason + '</span>';
        return '<tr><td>' + time + '</td><td>' + e.FromModel + '</td><td>' + e.ToModel +
          '</td><td>' + e.TaskType + '</td><td>' + reasonBadge + '</td></tr>';
      }).join('');
    }
  } catch (err) {
    console.error('Dashboard load error:', err);
  }
}

loadAll();
setInterval(loadAll, 15000);
</script>
</body>
</html>`
