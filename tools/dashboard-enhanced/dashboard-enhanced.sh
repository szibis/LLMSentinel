#!/bin/bash
# Enhanced Web Dashboard with Session History, Cost Analysis & Savings
PORT="${1:-8077}"

python3 << 'EOF'
import http.server
import socketserver
import json
import subprocess
import sys
import os
from datetime import datetime, timedelta

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8077

class EnhancedDashboardHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        home = os.path.expanduser('~')
        binary_path = f"{home}/.claude/bin/escalation-manager"

        if '/api/dashboard' in self.path:
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()

            # Get stats from binary
            try:
                result = subprocess.run([binary_path, 'stats'], capture_output=True, text=True, timeout=2)
                stats_data = json.loads(result.stdout.strip()) if result.returncode == 0 else {}
            except:
                stats_data = {}

            # Get cost analysis
            try:
                cost_result = subprocess.run([f"{home}/.claude/bin/escalation-stats-enhanced", 'cost'],
                                            capture_output=True, text=True, timeout=2)
                cost_data = json.loads(cost_result.stdout.strip()) if cost_result.returncode == 0 else {}
            except:
                cost_data = {}

            # Get session summary
            try:
                summary_result = subprocess.run([f"{home}/.claude/bin/escalation-stats-enhanced", 'summary'],
                                               capture_output=True, text=True, timeout=2)
                summary_data = json.loads(summary_result.stdout.strip()) if summary_result.returncode == 0 else {}
            except:
                summary_data = {}

            # Get history (last 20 sessions)
            try:
                history_result = subprocess.run([f"{home}/.claude/bin/escalation-stats-enhanced", 'history', '20'],
                                               capture_output=True, text=True, timeout=2)
                history_data = json.loads(history_result.stdout.strip()) if history_result.returncode == 0 else []
            except:
                history_data = []

            # Combine all data
            response = {
                "timestamp": datetime.now().isoformat(),
                "currentState": stats_data.get("currentState", {}),
                "stats": stats_data.get("stats", {}),
                "costAnalysis": cost_data,
                "summary": summary_data,
                "sessionHistory": history_data[-10:] if history_data else [],
                "metrics": {
                    "totalTokensSaved": cost_data.get("costSaved", 0),
                    "costSavedPercent": cost_data.get("costSavedPercent", 0),
                    "avgCostPerSession": cost_data.get("avgCostPerSession", 0),
                    "totalSessions": summary_data.get("totalSessions", 0),
                    "successRate": summary_data.get("successRate", 0)
                }
            }

            self.wfile.write(json.dumps(response).encode())
            return

        # Serve enhanced HTML dashboard
        html = """<!DOCTYPE html>
<html>
<head>
    <title>Escalation Dashboard - Enhanced</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header {
            background: white;
            padding: 30px;
            border-radius: 12px;
            margin-bottom: 20px;
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        h1 { font-size: 2em; color: #667eea; margin-bottom: 5px; }
        .subtitle { color: #999; }
        .theme-toggle { font-size: 1.5em; cursor: pointer; }
        .grid-2col { display: grid; grid-template-columns: 1fr 1fr; gap: 15px; margin-bottom: 20px; }
        .grid-3col { display: grid; grid-template-columns: repeat(3, 1fr); gap: 15px; margin-bottom: 20px; }
        .card {
            background: white;
            padding: 25px;
            border-radius: 10px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            border-left: 5px solid #667eea;
        }
        .card-title { font-size: 0.85em; color: #999; text-transform: uppercase; margin-bottom: 12px; }
        .card-value { font-size: 2.2em; font-weight: bold; color: #667eea; }
        .card-detail { font-size: 0.9em; color: #999; margin-top: 8px; }
        .card-green { border-left-color: #51CF66; }
        .card-green .card-value { color: #51CF66; }
        .card-orange { border-left-color: #FFA502; }
        .card-orange .card-value { color: #FFA502; }
        .section {
            background: white;
            padding: 25px;
            border-radius: 10px;
            margin-bottom: 20px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .section-title {
            font-size: 1.2em;
            color: #667eea;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 2px solid #667eea;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        th {
            background: #f5f5f5;
            padding: 12px;
            text-align: left;
            font-weight: 600;
            border-bottom: 2px solid #ddd;
            font-size: 0.85em;
        }
        td {
            padding: 12px;
            border-bottom: 1px solid #eee;
            font-size: 0.95em;
        }
        tr:hover { background: #f9f9f9; }
        .progress-bar {
            height: 20px;
            background: #f0f0f0;
            border-radius: 10px;
            overflow: hidden;
            margin: 10px 0;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #667eea, #764ba2);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-size: 0.75em;
            font-weight: bold;
        }
        .stat-row { display: flex; justify-content: space-between; margin: 10px 0; padding: 10px 0; border-bottom: 1px solid #eee; }
        .stat-label { color: #666; }
        .stat-value { font-weight: bold; color: #333; }
        .savings-highlight { background: #d4edda; padding: 15px; border-radius: 8px; margin: 15px 0; }
        .model-badge {
            display: inline-block;
            padding: 8px 16px;
            border-radius: 6px;
            color: white;
            font-weight: bold;
            font-size: 0.95em;
        }
        .model-opus { background: #FF6B6B; }
        .model-sonnet { background: #FFA502; }
        .model-haiku { background: #51CF66; }
        .live-indicator { color: #51cf66; font-weight: bold; animation: pulse 1s infinite; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        .footer { text-align: center; color: white; padding: 20px; margin-top: 40px; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>📊 Escalation Dashboard <span class="live-indicator">● LIVE</span></h1>
                <p class="subtitle">Real-time metrics with enhanced cost analysis & session history</p>
            </div>
            <div class="theme-toggle" onclick="toggleTheme()">🌙</div>
        </header>

        <div class="grid-3col">
            <div class="card card-orange">
                <div class="card-title">💰 Total Tokens Saved</div>
                <div class="card-value" id="tokens-saved">0</div>
                <div class="card-detail">vs using Opus for all</div>
            </div>
            <div class="card card-green">
                <div class="card-title">📈 Savings Percentage</div>
                <div class="card-value" id="savings-percent">0%</div>
                <div class="card-detail">Cost reduction achieved</div>
            </div>
            <div class="card">
                <div class="card-title">✅ Success Rate</div>
                <div class="card-value" id="success-rate">0%</div>
                <div class="card-detail">Problems solved on first try</div>
            </div>
        </div>

        <div class="grid-2col">
            <div class="card">
                <div class="card-title">📊 Current Model</div>
                <div style="margin-top: 15px;">
                    <span class="model-badge" id="model-badge" style="background: #667eea;">—</span>
                </div>
                <div class="card-detail">Cost: <span id="model-cost">—</span></div>
                <div class="card-detail">Effort: <span id="effort-level">—</span></div>
            </div>
            <div class="card">
                <div class="card-title">📈 Session Statistics</div>
                <div class="stat-row">
                    <span class="stat-label">Total Sessions:</span>
                    <span class="stat-value" id="total-sessions">0</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Escalations:</span>
                    <span class="stat-value" id="escalations">0</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Avg Cost/Session:</span>
                    <span class="stat-value" id="avg-cost">0 tokens</span>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-title">💡 Cost Breakdown</div>
            <div class="stat-row">
                <span class="stat-label">Actual Tokens Used:</span>
                <span class="stat-value" id="actual-tokens">0</span>
            </div>
            <div class="stat-row">
                <span class="stat-label">If Opus for All:</span>
                <span class="stat-value" id="opus-equivalent">0</span>
            </div>
            <div class="savings-highlight">
                <strong>💰 Total Saved: <span id="total-saved">0</span> tokens</strong><br>
                <small><span id="saved-explanation">—</span></small>
            </div>
        </div>

        <div class="section">
            <div class="section-title">🎯 Model Distribution</div>
            <div class="stat-row">
                <span class="stat-label">🧠 Opus (30x cost)</span>
                <span class="stat-value"><span id="opus-count">0</span> sessions</span>
            </div>
            <div class="progress-bar">
                <div class="progress-fill" id="opus-bar" style="width: 0%">0%</div>
            </div>
            <div class="stat-row">
                <span class="stat-label">⚡ Sonnet (8x cost)</span>
                <span class="stat-value"><span id="sonnet-count">0</span> sessions</span>
            </div>
            <div class="progress-bar">
                <div class="progress-fill" id="sonnet-bar" style="background: linear-gradient(90deg, #FFA502, #FFB732); width: 0%;">0%</div>
            </div>
            <div class="stat-row">
                <span class="stat-label">🪶 Haiku (1x cost)</span>
                <span class="stat-value"><span id="haiku-count">0</span> sessions</span>
            </div>
            <div class="progress-bar">
                <div class="progress-fill" id="haiku-bar" style="background: linear-gradient(90deg, #51CF66, #69DB7C); width: 0%;">0%</div>
            </div>
        </div>

        <div class="section">
            <div class="section-title">📋 Recent Sessions (Last 10)</div>
            <table>
                <thead>
                    <tr>
                        <th>Timestamp</th>
                        <th>Event</th>
                        <th>Model</th>
                        <th>Task Type</th>
                        <th>Tokens</th>
                        <th>Saved</th>
                    </tr>
                </thead>
                <tbody id="history-table">
                    <tr><td colspan="6" style="text-align: center; color: #999;">No session history yet</td></tr>
                </tbody>
            </table>
        </div>

        <div class="footer">
            <p>🔄 Auto-refreshes every 2 seconds • Enhanced Dashboard v2.1</p>
            <p>Last update: <span id="last-update">—</span></p>
        </div>
    </div>

    <script>
        function update() {
            fetch('/api/dashboard')
                .then(r => r.json())
                .then(d => {
                    // Current state
                    const cs = d.currentState || {};
                    const st = d.stats || {};
                    const metrics = d.metrics || {};
                    const cost = d.costAnalysis || {};
                    const summary = d.summary || {};
                    const history = d.sessionHistory || [];

                    // Model badge
                    const mb = document.getElementById('model-badge');
                    mb.style.background = cs.modelColor || '#667eea';
                    mb.textContent = cs.model || '—';
                    document.getElementById('model-cost').textContent = cs.modelCost || '—';
                    document.getElementById('effort-level').textContent = (cs.effort || '—').toUpperCase();

                    // Main metrics
                    document.getElementById('tokens-saved').textContent = cost.costSaved || 0;
                    document.getElementById('savings-percent').textContent = (cost.costSavedPercent || 0) + '%';
                    document.getElementById('success-rate').textContent = (summary.successRate || 0) + '%';

                    // Sessions
                    document.getElementById('total-sessions').textContent = summary.totalSessions || 0;
                    document.getElementById('escalations').textContent = st.escalations || 0;
                    document.getElementById('avg-cost').textContent = (cost.avgCostPerSession || 0) + ' tokens';

                    // Cost analysis
                    document.getElementById('actual-tokens').textContent = cost.actualTokensCost || 0;
                    document.getElementById('opus-equivalent').textContent = cost.estimatedWithoutEscalation || 0;
                    document.getElementById('total-saved').textContent = cost.costSaved || 0;
                    const percent = cost.costSavedPercent || 0;
                    document.getElementById('saved-explanation').textContent =
                        `By using Haiku for simple tasks and Sonnet for medium tasks, you've saved ${percent}% of tokens compared to using Opus for everything.`;

                    // Model distribution
                    const breakdown = cost.tokenBreakdown || {};
                    const opus = breakdown.opus || {};
                    const sonnet = breakdown.sonnet || {};
                    const haiku = breakdown.haiku || {};
                    const total = (opus.count || 0) + (sonnet.count || 0) + (haiku.count || 0);

                    document.getElementById('opus-count').textContent = opus.count || 0;
                    document.getElementById('sonnet-count').textContent = sonnet.count || 0;
                    document.getElementById('haiku-count').textContent = haiku.count || 0;

                    if (total > 0) {
                        const opus_pct = Math.round((opus.count || 0) * 100 / total);
                        const sonnet_pct = Math.round((sonnet.count || 0) * 100 / total);
                        const haiku_pct = Math.round((haiku.count || 0) * 100 / total);

                        document.getElementById('opus-bar').style.width = opus_pct + '%';
                        document.getElementById('opus-bar').textContent = opus_pct + '%';
                        document.getElementById('sonnet-bar').style.width = sonnet_pct + '%';
                        document.getElementById('sonnet-bar').textContent = sonnet_pct + '%';
                        document.getElementById('haiku-bar').style.width = haiku_pct + '%';
                        document.getElementById('haiku-bar').textContent = haiku_pct + '%';
                    }

                    // History table
                    const tbody = document.getElementById('history-table');
                    if (history.length > 0) {
                        tbody.innerHTML = history.map(h => `
                            <tr>
                                <td>${h.timestamp || '—'}</td>
                                <td>${h.type || '—'}</td>
                                <td>${h.to ? h.to.split('-')[1] : '—'}</td>
                                <td>${h.task || '—'}</td>
                                <td>${h.tokens || 0}</td>
                                <td><strong>+${h.savings || 0}</strong></td>
                            </tr>
                        `).join('');
                    }

                    // Last update
                    document.getElementById('last-update').textContent = new Date().toLocaleTimeString();
                });
        }

        function toggleTheme() {
            const isDark = document.body.style.background.includes('#');
            if (isDark) {
                document.body.style.background = 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)';
                document.querySelector('.theme-toggle').textContent = '🌙';
            } else {
                document.body.style.background = '#1a1a1a';
                document.querySelector('.theme-toggle').textContent = '☀️';
            }
        }

        update();
        setInterval(update, 2000);
    </script>
</body>
</html>"""

        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        self.wfile.write(html.encode())

    def log_message(self, format, *args):
        pass

with socketserver.TCPServer(("", PORT), EnhancedDashboardHandler) as httpd:
    print(f"✅ Enhanced Dashboard: http://localhost:{PORT}")
    print("   • Shows cost savings analysis")
    print("   • Session history with token tracking")
    print("   • Real-time model distribution")
    print("   • Auto-refreshes every 2 seconds")
    print("   • Press Ctrl+C to stop")
    httpd.serve_forever()
EOF
