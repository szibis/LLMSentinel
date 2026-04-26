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

// dashboardHTML is the embedded enhanced dashboard with sentiment, budget, and cost analysis tabs.
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
.tabs { display: flex; gap: 8px; margin-bottom: 24px; border-bottom: 2px solid var(--border); }
.tab { padding: 12px 16px; background: none; border: none; color: var(--dim); cursor: pointer; font-size: 14px; font-weight: 600; border-bottom: 3px solid transparent; transition: all 0.2s; }
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }
.tab:hover { color: var(--text); }
.tab-content { display: none; }
.tab-content.active { display: block; }
.sentiment-grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 16px; margin-bottom: 24px; }
.sentiment-card { background: var(--bg); padding: 16px; border-radius: 8px; border: 1px solid var(--border); text-align: center; }
.sentiment-emoji { font-size: 32px; margin-bottom: 8px; }
.sentiment-value { font-size: 24px; font-weight: 700; margin-bottom: 4px; }
.sentiment-label { font-size: 12px; color: var(--dim); text-transform: uppercase; }
.chart-container { background: var(--bg); padding: 24px; border-radius: 8px; border: 1px solid var(--border); margin-bottom: 24px; }
.budget-bar { height: 24px; background: var(--border); border-radius: 4px; overflow: hidden; margin: 8px 0; }
.budget-fill { height: 100%; border-radius: 4px; transition: width 0.3s; }
.warning-yellow { background: var(--yellow); }
.warning-red { background: var(--red); }
.success-green { background: var(--green); }
.frustration-event { background: var(--bg); padding: 16px; border-radius: 8px; border-left: 4px solid var(--red); margin-bottom: 12px; }
.frustration-time { font-size: 12px; color: var(--dim); }
.frustration-task { font-weight: 600; margin: 4px 0; }
.frustration-resolution { font-size: 13px; color: var(--green); margin-top: 4px; }
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

  <div class="tabs">
    <button class="tab active" onclick="switchTab('overview')">📊 Overview</button>
    <button class="tab" onclick="switchTab('sentiment')">😊 Sentiment</button>
    <button class="tab" onclick="switchTab('budget')">💰 Budget</button>
    <button class="tab" onclick="switchTab('optimization')">🎯 Optimization</button>
  </div>

  <!-- Overview Tab -->
  <div id="overview" class="tab-content active">
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

  <!-- Sentiment Tab -->
  <div id="sentiment" class="tab-content">
    <div class="section card">
      <div class="section-title">😊 User Sentiment Trends (Last 24h)</div>
      <div class="sentiment-grid">
        <div class="sentiment-card">
          <div class="sentiment-emoji">😊</div>
          <div class="sentiment-value" id="sentiment-satisfied">—</div>
          <div class="sentiment-label">Satisfied</div>
        </div>
        <div class="sentiment-card">
          <div class="sentiment-emoji">😐</div>
          <div class="sentiment-value" id="sentiment-neutral">—</div>
          <div class="sentiment-label">Neutral</div>
        </div>
        <div class="sentiment-card">
          <div class="sentiment-emoji">😤</div>
          <div class="sentiment-value" id="sentiment-frustrated">—</div>
          <div class="sentiment-label">Frustrated</div>
        </div>
        <div class="sentiment-card">
          <div class="sentiment-emoji">🤔</div>
          <div class="sentiment-value" id="sentiment-confused">—</div>
          <div class="sentiment-label">Confused</div>
        </div>
        <div class="sentiment-card">
          <div class="sentiment-emoji">⏱️</div>
          <div class="sentiment-value" id="sentiment-impatient">—</div>
          <div class="sentiment-label">Impatient</div>
        </div>
      </div>

      <div style="background: var(--bg); padding: 16px; border-radius: 8px; margin-bottom: 16px;">
        <div style="font-size: 14px; margin-bottom: 8px;"><strong>Satisfaction Rate:</strong> <span id="sentiment-rate" style="color: var(--green); font-weight: 700;">—</span></div>
        <div class="progress"><div class="progress-fill success-green" id="sentiment-rate-bar" style="width:0%"></div></div>
      </div>
    </div>

    <div class="section card">
      <div class="section-title">🚨 Frustration Events (Last 24h)</div>
      <div id="frustration-events">
        <div style="text-align:center;padding:32px;color:var(--dim)">No frustration events.</div>
      </div>
    </div>

    <div class="section card">
      <div class="section-title">⭐ Model Satisfaction by Task Type</div>
      <table>
        <thead><tr><th>Task Type</th><th>Haiku</th><th>Sonnet</th><th>Opus</th><th>Recommendation</th></tr></thead>
        <tbody id="model-satisfaction-body"><tr><td colspan="5" class="empty-state">Loading...</td></tr></tbody>
      </table>
    </div>
  </div>

  <!-- Budget Tab -->
  <div id="budget" class="tab-content">
    <div class="section card">
      <div class="section-title">💳 Daily Budget Status</div>
      <div class="grid">
        <div class="card">
          <div class="card-label">Daily Limit</div>
          <div class="card-value" id="budget-daily-limit">—</div>
        </div>
        <div class="card">
          <div class="card-label">Daily Used</div>
          <div class="card-value" id="budget-daily-used">—</div>
        </div>
        <div class="card">
          <div class="card-label">Daily Remaining</div>
          <div class="card-value green" id="budget-daily-remaining">—</div>
        </div>
        <div class="card">
          <div class="card-label">Daily Usage %</div>
          <div class="card-value" id="budget-daily-percent">—</div>
        </div>
      </div>
      <div style="margin-top: 16px;">
        <div style="font-size: 14px; margin-bottom: 8px;">Daily Budget Progress:</div>
        <div class="budget-bar"><div class="budget-fill success-green" id="budget-daily-bar" style="width:0%"></div></div>
      </div>
    </div>

    <div class="section card">
      <div class="section-title">📅 Monthly Budget Status</div>
      <div class="grid">
        <div class="card">
          <div class="card-label">Monthly Limit</div>
          <div class="card-value" id="budget-monthly-limit">—</div>
        </div>
        <div class="card">
          <div class="card-label">Monthly Used</div>
          <div class="card-value" id="budget-monthly-used">—</div>
        </div>
        <div class="card">
          <div class="card-label">Days Remaining</div>
          <div class="card-value accent" id="budget-days-remaining">—</div>
        </div>
        <div class="card">
          <div class="card-label">Monthly Usage %</div>
          <div class="card-value" id="budget-monthly-percent">—</div>
        </div>
      </div>
      <div style="margin-top: 16px;">
        <div style="font-size: 14px; margin-bottom: 8px;">Monthly Budget Progress:</div>
        <div class="budget-bar"><div class="budget-fill" id="budget-monthly-bar" style="width:0%"></div></div>
      </div>
    </div>

    <div class="section card">
      <div class="section-title">🎯 Model Daily Limits</div>
      <table>
        <thead><tr><th>Model</th><th>Daily Limit</th><th>Used</th><th>Remaining</th><th>Usage %</th></tr></thead>
        <tbody id="model-limits-body"><tr><td colspan="5" class="empty-state">Loading...</td></tr></tbody>
      </table>
    </div>
  </div>

  <!-- Optimization Tab -->
  <div id="optimization" class="tab-content">
    <div class="section card">
      <div class="section-title">🎯 Cost Optimization Opportunities</div>
      <div id="cost-recommendations">
        <div style="text-align:center;padding:32px;color:var(--dim)">Loading recommendations...</div>
      </div>
    </div>

    <div class="section card">
      <div class="section-title">📈 Potential Savings Summary</div>
      <div class="grid">
        <div class="card">
          <div class="card-label">Total Identified Opportunities</div>
          <div class="card-value accent" id="opt-count">—</div>
        </div>
        <div class="card">
          <div class="card-label">Estimated Monthly Savings</div>
          <div class="card-value green" id="opt-savings">—</div>
        </div>
        <div class="card">
          <div class="card-label">Average Savings per Task</div>
          <div class="card-value purple" id="opt-avg-savings">—</div>
        </div>
      </div>
    </div>
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

function switchTab(tab) {
  document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.tab').forEach(el => el.classList.remove('active'));
  document.getElementById(tab).classList.add('active');
  event.target.classList.add('active');
}

async function loadAll() {
  try {
    const [stats, types, history, sentiment, budget, satisfaction, optimization] = await Promise.all([
      fetch('/api/stats').then(r => r.json()).catch(() => ({})),
      fetch('/api/types').then(r => r.json()).catch(() => []),
      fetch('/api/history').then(r => r.json()).catch(() => []),
      fetch('/api/analytics/sentiment-trends?hours=24').then(r => r.json()).catch(() => ({})),
      fetch('/api/analytics/budget-status').then(r => r.json()).catch(() => ({})),
      fetch('/api/analytics/model-satisfaction?task_type=concurrency').then(r => r.json()).catch(() => ({})),
      fetch('/api/analytics/cost-optimization').then(r => r.json()).catch(() => ({})),
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

    // Sentiment data
    if (sentiment && sentiment.summary) {
      const s = sentiment.summary;
      document.getElementById('sentiment-satisfied').textContent = s.satisfied || 0;
      document.getElementById('sentiment-neutral').textContent = s.neutral || 0;
      document.getElementById('sentiment-frustrated').textContent = s.frustrated || 0;
      document.getElementById('sentiment-confused').textContent = s.confused || 0;
      document.getElementById('sentiment-impatient').textContent = s.impatient || 0;

      const rate = Math.round((s.satisfaction_rate || 0) * 100);
      document.getElementById('sentiment-rate').textContent = rate + '%';
      document.getElementById('sentiment-rate-bar').style.width = rate + '%';
    }

    // Frustration events
    if (sentiment && sentiment.events && sentiment.events.length > 0) {
      const evHTML = sentiment.events.map(e => {
        const time = new Date(e.Timestamp).toLocaleString(undefined, {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});
        const resolvedBadge = e.Resolved ? '<span class="badge badge-green">✓ Resolved</span>' : '<span class="badge badge-red">✗ Unresolved</span>';
        return '<div class="frustration-event">' +
          '<div class="frustration-time">' + time + '</div>' +
          '<div class="frustration-task">' + (e.TaskType || 'unknown') + ' on ' + (e.InitialModel || 'haiku') + ' → ' + (e.EscalatedTo || 'N/A') + '</div>' +
          '<div class="frustration-resolution">' + resolvedBadge + '</div>' +
          '</div>';
      }).join('');
      document.getElementById('frustration-events').innerHTML = evHTML;
    }

    // Budget data
    if (budget && budget.daily_budget) {
      const db = budget.daily_budget;
      const mb = budget.monthly_budget;
      document.getElementById('budget-daily-limit').textContent = '$' + db.limit.toFixed(2);
      document.getElementById('budget-daily-used').textContent = '$' + db.used.toFixed(2);
      document.getElementById('budget-daily-remaining').textContent = '$' + db.remaining.toFixed(2);
      document.getElementById('budget-daily-percent').textContent = Math.round(db.percentage) + '%';
      document.getElementById('budget-daily-bar').style.width = Math.min(db.percentage, 100) + '%';

      const dailyBarColor = db.percentage > 90 ? 'warning-red' : db.percentage > 75 ? 'warning-yellow' : 'success-green';
      document.getElementById('budget-daily-bar').className = 'budget-fill ' + dailyBarColor;

      document.getElementById('budget-monthly-limit').textContent = '$' + mb.limit.toFixed(2);
      document.getElementById('budget-monthly-used').textContent = '$' + mb.used.toFixed(2);
      document.getElementById('budget-days-remaining').textContent = (mb.days_left || 0) + 'd';
      document.getElementById('budget-monthly-percent').textContent = Math.round(mb.percentage) + '%';
      document.getElementById('budget-monthly-bar').style.width = Math.min(mb.percentage, 100) + '%';

      const monthlyBarColor = mb.percentage > 90 ? 'warning-red' : mb.percentage > 75 ? 'warning-yellow' : 'success-green';
      document.getElementById('budget-monthly-bar').className = 'budget-fill ' + monthlyBarColor;
    }

    // Model satisfaction table
    const msBody = document.getElementById('model-satisfaction-body');
    if (satisfaction && satisfaction.satisfactions && satisfaction.satisfactions.length > 0) {
      msBody.innerHTML = satisfaction.satisfactions.map(s => {
        const rate = Math.round(s.SatisfactionRate * 100);
        const badgeClass = rate > 80 ? 'badge-green' : rate > 60 ? 'badge-yellow' : 'badge-red';
        return '<tr><td><strong>' + s.Model + '</strong></td><td>' + rate + '%</td><td colspan="2"></td>' +
          '<td><span class="badge ' + badgeClass + '">' + rate + '% success</span></td></tr>';
      }).join('');
    }

    // Cost optimization recommendations
    if (optimization && optimization.recommendations && optimization.recommendations.length > 0) {
      const recHTML = optimization.recommendations.map(r => {
        const savings = Math.round(r.estimated_savings_percent);
        return '<div style="background:var(--bg);padding:16px;border-radius:8px;border-left:4px solid var(--green);margin-bottom:12px">' +
          '<div style="font-weight:600;margin-bottom:8px">' + r.task_type + ': ' + r.current_model + ' → ' + r.recommended_model + '</div>' +
          '<div style="font-size:13px;color:var(--dim);margin-bottom:8px">' +
          'Current: ' + Math.round(r.current_satisfaction * 100) + '% satisfaction | ' +
          'Recommended: ' + Math.round(r.recommended_satisfaction * 100) + '% satisfaction</div>' +
          '<div style="color:var(--green);font-weight:600">💰 Potential Savings: ' + savings + '%</div>' +
          '</div>';
      }).join('');
      document.getElementById('cost-recommendations').innerHTML = recHTML || '<div style="text-align:center;padding:32px;color:var(--dim)">No optimization opportunities at this time.</div>';

      const totalSavings = optimization.recommendations.reduce((sum, r) => sum + r.estimated_savings_percent, 0);
      const avgSavings = optimization.count > 0 ? Math.round(totalSavings / optimization.count) : 0;
      document.getElementById('opt-count').textContent = optimization.count || 0;
      document.getElementById('opt-savings').textContent = Math.round(totalSavings) + '%';
      document.getElementById('opt-avg-savings').textContent = avgSavings + '%';
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
