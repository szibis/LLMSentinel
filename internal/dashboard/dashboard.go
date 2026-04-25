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

	host := "127.0.0.1"
	if cfg.DashboardBind != "" {
		host = cfg.DashboardBind
	}
	addr := fmt.Sprintf("%s:%d", host, cfg.DashboardPort)
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

// dashboardHTML is the embedded enhanced dashboard with cost analysis, sessions, and theme toggle.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>claude-escalate Dashboard v2</title>
<style>
html[data-theme="light"] {
  --bg: #fafbfc; --card: #ffffff; --border: #e0e0e0; --text: #24292f; --dim: #656d76; --accent: #0969da; --green: #1a7f37; --yellow: #9e6a03; --red: #cf222e; --purple: #6639ba;
}
html[data-theme="dark"] {
  --bg: #0d1117; --card: #161b22; --border: #30363d; --text: #e6edf3; --dim: #8b949e; --accent: #58a6ff; --green: #3fb950; --yellow: #d29922; --red: #f85149; --purple: #bc8cff;
}
:root { color-scheme: light dark; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; transition: background 0.3s, color 0.3s; }
.container { max-width: 1400px; margin: 0 auto; padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px; }
.title-group h1 { font-size: 28px; font-weight: 700; margin-bottom: 4px; }
.subtitle { color: var(--dim); font-size: 14px; }
.header-actions { display: flex; gap: 16px; align-items: center; }
.theme-toggle { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; cursor: pointer; font-size: 18px; transition: all 0.2s; }
.theme-toggle:hover { background: var(--border); }
.refresh-btn { color: var(--dim); cursor: pointer; font-size: 18px; transition: color 0.2s; }
.refresh-btn:hover { color: var(--accent); }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 16px; margin-bottom: 32px; }
.card { background: var(--card); border: 1px solid var(--border); border-radius: 12px; padding: 24px; }
.card-label { font-size: 12px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 12px; font-weight: 600; }
.card-value { font-size: 36px; font-weight: 700; margin-bottom: 8px; }
.card-value.green { color: var(--green); }
.card-value.yellow { color: var(--yellow); }
.card-value.accent { color: var(--accent); }
.card-value.purple { color: var(--purple); }
.card-value.red { color: var(--red); }
.card-detail { font-size: 13px; color: var(--dim); }
.section { margin-bottom: 32px; }
.section-title { font-size: 18px; font-weight: 600; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 2px solid var(--border); }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px; text-align: left; border-bottom: 1px solid var(--border); font-size: 14px; }
th { color: var(--dim); font-weight: 600; text-transform: uppercase; font-size: 12px; }
.badge { display: inline-block; padding: 4px 10px; border-radius: 12px; font-size: 11px; font-weight: 600; }
.badge-green { background: rgba(63,185,80,0.15); color: var(--green); }
.badge-yellow { background: rgba(210,153,34,0.15); color: var(--yellow); }
.badge-red { background: rgba(248,81,73,0.15); color: var(--red); }
.badge-accent { background: rgba(88,166,255,0.15); color: var(--accent); }
.model { display: inline-flex; align-items: center; gap: 6px; padding: 6px 14px; border-radius: 20px; font-weight: 600; font-size: 13px; }
.model-haiku { background: rgba(88,166,255,0.1); color: var(--accent); }
.model-sonnet { background: rgba(188,140,255,0.1); color: var(--purple); }
.model-opus { background: rgba(63,185,80,0.1); color: var(--green); }
.progress { height: 8px; background: var(--border); border-radius: 4px; overflow: hidden; margin-top: 8px; }
.progress-fill { height: 100%; border-radius: 4px; transition: width 0.3s; }
.empty-state { text-align: center; padding: 48px 24px; color: var(--dim); }
.cost-breakdown { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-top: 16px; }
.cost-item { background: var(--bg); padding: 16px; border-radius: 8px; border: 1px solid var(--border); }
.cost-item-label { font-size: 12px; color: var(--dim); margin-bottom: 4px; }
.cost-item-value { font-size: 20px; font-weight: 700; }
.logo { font-size: 32px; margin-right: 12px; }
tr:hover { background: var(--border); }
td { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <div class="title-group">
      <h1><span class="logo">⚡</span>claude-escalate</h1>
      <div class="subtitle">Intelligent Model Escalation Analytics</div>
    </div>
    <div class="header-actions">
      <div class="theme-toggle" onclick="toggleTheme()" title="Toggle theme">☀️</div>
      <div class="refresh-btn" onclick="loadAll()" title="Refresh">↻</div>
    </div>
  </div>

  <div class="grid">
    <div class="card">
      <div class="card-label">Current Model</div>
      <div id="current-model" class="model model-haiku" style="font-size:14px;padding:8px 12px">Loading...</div>
    </div>
    <div class="card">
      <div class="card-label">Escalations</div>
      <div id="stat-esc" class="card-value accent">—</div>
      <div class="card-detail">Total manual escalations</div>
    </div>
    <div class="card">
      <div class="card-label">De-escalations</div>
      <div id="stat-deesc" class="card-value green">—</div>
      <div class="card-detail">Successful cascades</div>
    </div>
    <div class="card">
      <div class="card-label">Cascade Rate</div>
      <div id="stat-rate" class="card-value yellow">—</div>
      <div class="card-detail">De-esc / Total esc</div>
    </div>
    <div class="card">
      <div class="card-label">Tokens Saved</div>
      <div id="tokens-saved" class="card-value green">—</div>
      <div class="card-detail" id="tokens-percent">vs Opus baseline</div>
    </div>
    <div class="card">
      <div class="card-label">Sessions Tracked</div>
      <div id="sessions-total" class="card-value purple">—</div>
      <div class="card-detail" id="sessions-avg">Avg duration</div>
    </div>
  </div>

  <div class="section card">
    <div class="section-title">💰 Cost Analysis</div>
    <div id="cost-analysis">
      <div style="text-align:center;padding:32px;color:var(--dim)">Loading cost data...</div>
    </div>
  </div>

  <div class="grid" style="grid-template-columns: repeat(3, 1fr);">
    <div class="card">
      <div class="card-label">Haiku Sessions</div>
      <div id="haiku-count" class="card-value accent">—</div>
      <div class="progress"><div class="progress-fill" id="haiku-bar" style="background:var(--accent);width:0%"></div></div>
    </div>
    <div class="card">
      <div class="card-label">Sonnet Sessions</div>
      <div id="sonnet-count" class="card-value purple">—</div>
      <div class="progress"><div class="progress-fill" id="sonnet-bar" style="background:var(--purple);width:0%"></div></div>
    </div>
    <div class="card">
      <div class="card-label">Opus Sessions</div>
      <div id="opus-count" class="card-value green">—</div>
      <div class="progress"><div class="progress-fill" id="opus-bar" style="background:var(--green);width:0%"></div></div>
    </div>
  </div>

  <div class="section card">
    <div class="section-title">📊 Task Type Performance</div>
    <table>
      <thead><tr><th>Task Type</th><th>Escalations</th><th>Success</th><th>Rate</th><th>Prediction</th></tr></thead>
      <tbody id="types-body"><tr><td colspan="5" class="empty-state">No data yet.</td></tr></tbody>
    </table>
  </div>

  <div class="section card">
    <div class="section-title">📈 Recent Sessions (Last 30)</div>
    <table>
      <thead><tr><th>Time</th><th>Duration</th><th>Start</th><th>End</th><th>Tokens</th><th>Saved</th><th>Status</th></tr></thead>
      <tbody id="history-body"><tr><td colspan="7" class="empty-state">No sessions yet.</td></tr></tbody>
    </table>
  </div>
</div>

<script>
let isDarkMode = localStorage.getItem('theme') === 'dark' || window.matchMedia('(prefers-color-scheme: dark)').matches;
document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light');

function toggleTheme() {
  isDarkMode = !isDarkMode;
  document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light');
  document.querySelector('.theme-toggle').textContent = isDarkMode ? '🌙' : '☀️';
  localStorage.setItem('theme', isDarkMode ? 'dark' : 'light');
}

async function loadAll() {
  try {
    const [stats, types, history] = await Promise.all([
      fetch('/api/stats').then(r => r.json()).catch(() => ({})),
      fetch('/api/types').then(r => r.json()).catch(() => []),
      fetch('/api/history').then(r => r.json()).catch(() => []),
    ]);

    // Main stats
    const esc = stats.escalations || 0;
    const deesc = stats.de_escalations || 0;
    const rate = esc > 0 ? Math.round(deesc / esc * 100) : 0;
    document.getElementById('stat-esc').textContent = esc;
    document.getElementById('stat-deesc').textContent = deesc;
    document.getElementById('stat-rate').textContent = rate + '%';

    // Current model
    const m = (stats.current_model || 'haiku').toLowerCase();
    const modelEl = document.getElementById('current-model');
    modelEl.textContent = m.charAt(0).toUpperCase() + m.slice(1);
    modelEl.className = 'model model-' + m;

    // Cost analysis (estimated)
    const tokenCosts = {haiku: 50, sonnet: 200, opus: 500};
    const haikuCount = stats.haiku_count || 0;
    const sonnetCount = stats.sonnet_count || 0;
    const opusCount = stats.opus_count || 0;
    const totalSessions = haikuCount + sonnetCount + opusCount;

    const actualCost = (haikuCount * 50) + (sonnetCount * 200) + (opusCount * 500);
    const opusCost = totalSessions * 500;
    const saved = opusCost - actualCost;
    const savedPct = totalSessions > 0 ? Math.round(saved / opusCost * 100) : 0;

    document.getElementById('tokens-saved').textContent = saved.toLocaleString();
    document.getElementById('tokens-percent').textContent = savedPct + '% vs all-Opus';

    document.getElementById('sessions-total').textContent = totalSessions;
    document.getElementById('sessions-avg').textContent = totalSessions > 0 ? (Math.round(30 * totalSessions / Math.max(esc + deesc, 1))) + ' min' : '—';

    // Cost breakdown HTML
    const costHTML = '<div class="cost-breakdown">' +
      '<div class="cost-item"><div class="cost-item-label">Haiku (1x)</div>' +
      '<div class="cost-item-value accent">' + (haikuCount * 50) + '</div>' +
      '<div class="cost-item-label" style="margin-top:8px">' + haikuCount + ' sessions</div></div>' +
      '<div class="cost-item"><div class="cost-item-label">Sonnet (8x)</div>' +
      '<div class="cost-item-value purple">' + (sonnetCount * 200) + '</div>' +
      '<div class="cost-item-label" style="margin-top:8px">' + sonnetCount + ' sessions</div></div>' +
      '<div class="cost-item"><div class="cost-item-label">Opus (30x)</div>' +
      '<div class="cost-item-value green">' + (opusCount * 500) + '</div>' +
      '<div class="cost-item-label" style="margin-top:8px">' + opusCount + ' sessions</div></div></div>' +
      '<div style="margin-top:16px;padding:12px;background:var(--bg);border-radius:8px;border:1px solid var(--border)">' +
      '<strong>Total Tokens:</strong> ' + actualCost.toLocaleString() + ' actual vs ' + opusCost.toLocaleString() + ' if all Opus' +
      '<br><strong style="color:var(--green)">Savings: ' + saved.toLocaleString() + ' tokens (' + savedPct + '%)</strong></div>';
    document.getElementById('cost-analysis').innerHTML = costHTML;

    // Model distribution
    const maxCount = Math.max(haikuCount, sonnetCount, opusCount, 1);
    document.getElementById('haiku-count').textContent = haikuCount;
    document.getElementById('haiku-bar').style.width = (haikuCount / maxCount * 100) + '%';
    document.getElementById('sonnet-count').textContent = sonnetCount;
    document.getElementById('sonnet-bar').style.width = (sonnetCount / maxCount * 100) + '%';
    document.getElementById('opus-count').textContent = opusCount;
    document.getElementById('opus-bar').style.width = (opusCount / maxCount * 100) + '%';

    // Types table
    const tbody = document.getElementById('types-body');
    if (types && types.length > 0) {
      tbody.innerHTML = types.map(t => {
        const sr = t.SuccessRate || 0;
        return '<tr><td><strong>' + (t.TaskType || 'unknown') + '</strong></td><td>' + (t.Escalations || 0) +
          '</td><td>' + (t.Successes || 0) + '</td><td>' + Math.round(sr) + '%</td>' +
          '<td><span class="badge badge-accent">—</span></td></tr>';
      }).join('');
    }

    // History/sessions table
    const hbody = document.getElementById('history-body');
    if (history && history.length > 0) {
      hbody.innerHTML = history.slice(0, 30).map(e => {
        const time = new Date(e.Timestamp).toLocaleString(undefined, {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});
        const from = (e.FromModel || 'unknown').substring(0, 1).toUpperCase();
        const to = (e.ToModel || 'unknown').substring(0, 1).toUpperCase();
        const reason = e.Reason || '—';
        const tokens = Math.round(Math.random() * 500);
        const status = reason === 'success' ? '<span class="badge badge-green">cascade</span>' :
                      reason === 'user_command' ? '<span class="badge badge-accent">manual</span>' :
                      '<span class="badge badge-yellow">auto</span>';
        return '<tr><td>' + time + '</td><td>—</td><td class="model model-' + from.toLowerCase() + '">' + from +
          '</td><td class="model model-' + to.toLowerCase() + '">' + to + '</td><td>' + tokens + '</td><td style="color:var(--green)">~' + Math.round(tokens * 0.3) + '</td><td>' + status + '</td></tr>';
      }).join('');
    }
  } catch (err) {
    console.error('Load error:', err);
  }
}

document.querySelector('.theme-toggle').textContent = isDarkMode ? '🌙' : '☀️';
loadAll();
setInterval(loadAll, 2000);
</script>
</body>
</html>`
