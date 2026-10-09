// Rendering-contract tests use a minimal DOM, not a visual browser substitute.
const {readFileSync} = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const {test} = require('node:test');

function fixture() {
  const html = readFileSync(path.join(__dirname, '../internal/labdashboard/dashboard.html'), 'utf8');
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(new Set(ids).size, ids.length, 'duplicate dashboard IDs');
  class Element {
    constructor() { this.children = []; this.attributes = {}; this.className = ''; this.value = '900'; this.selected = new Map(); this._text = ''; }
    get textContent() { return this._text + this.children.map(child => child.textContent).join(''); }
    set textContent(value) { this._text = String(value); this.children = []; }
    get lastChild() { return this.children.at(-1); }
    get classList() { return {contains: value => this.className.split(' ').includes(value)}; }
    appendChild(child) { this.children.push(child); return child; }
    replaceChildren(...children) { this.children = children; this._text = ''; }
    setAttribute(key, value) { this.attributes[key] = value; }
    querySelector(selector) { if (!this.selected.has(selector)) this.selected.set(selector, new Element()); return this.selected.get(selector); }
    addEventListener() {}
  }
  const elements = new Map(ids.map(id => [id, new Element()]));
  elements.get('refresh-period').value = '1000';
  const document = {getElementById(id) { assert.ok(elements.has(id), 'missing element: ' + id); return elements.get(id); }, createElement: () => new Element(), createElementNS: () => new Element()};
  const context = vm.createContext({document, Date, AbortSignal, setTimeout: () => 0, fetch: async () => ({ok: false, status: 503})});
  const script = html.match(/<script>([\s\S]+)<\/script>/)[1].replace(/\nrefresh\(\);\s*$/, '');
  vm.runInContext(script, context, {filename: 'sentinel-dashboard-renderer.js'});
  return {elements, context, draw(data) {context.data = data; vm.runInContext('draw(data)', context);}};
}

test('missing measurements remain unknown and reports stay visibly unavailable', () => {
  const f = fixture(); f.draw({sample_time: new Date().toISOString()});
  assert.match(f.elements.get('small').querySelector('.decode').textContent, /Unknown/);
  assert.match(f.elements.get('available').textContent, /Unknown/);
  assert.match(f.elements.get('benchmark-summary').textContent, /No benchmark evidence/);
  assert.match(f.elements.get('jes-summary').textContent, /No advisory evidence/);
  assert.match(f.elements.get('cache-detail').textContent, /Unknown/);
});

test('measured reports render charts, failure reasons and literal text safely', () => {
  const f = fixture(), now = new Date().toISOString(), hostile = '<img src=x onerror=alert(1)>';
  f.draw({sample_time: now, route_history: [{role: 'sonnet', upstream: 'large', observed_attempts: 2, protocol_accepted: 1, p50_ms: 20, p95_ms: 29}],
    benchmark: {timestamp: now, summary: {successful_requests: 1, total_p50_ms: 10, total_p95_ms: 10}, samples: [{started_at: now, pair: 1, phase: 'first', role: 'sonnet', status: 'success', total_ms: 10, ttft_ms: null, native_decode_tps: null, cache: {status: 'unknown', provenance: 'unavailable', reason: hostile}}]},
    jes_quality: {finished_at: now, training_eligible: 0, outcomes: [{sample_id: hostile, role: 'sonnet', model: '', score: null, recommendation: 'insufficient-evidence', training_eligible: false, reasons: ['model provenance incomplete']}]},
    cli_task_quality: {finished_at: now, results: [{task: 'literal-markers', role: 'sonnet', client: 'codex', passed: false, failure: hostile, tool_calls: 1, latency_ms: 10, input_tokens: null, output_tokens: null}]}});
  assert.equal(f.elements.get('route-history-rows').children.length, 1);
  assert.match(f.elements.get('route-history-rows').textContent, /20.0 ms \/ 29.0 ms/);
  assert.match(f.elements.get('benchmark-rows').textContent, /Unknown \/ Unknown/);
  assert.ok(f.elements.get('benchmark-latency-chart').children.some(node => node.attributes.r === 3.5));
  assert.match(f.elements.get('jes-rows').textContent, /Unknown.*Not admitted/);
  assert.equal(f.elements.get('cli-quality-rows').children[0].children[2].className, 'bad');
  assert.ok(f.elements.get('cli-quality-rows').textContent.includes(hostile));
  assert.equal(f.elements.get('cli-quality-rows').children[0].children[2].children.length, 0);
});

test('failed refresh preserves the displayed evidence and marks it stale', async () => {
  const f = fixture(); f.draw({sample_time: new Date().toISOString()});
  const before = f.elements.get('raw').textContent;
  await vm.runInContext('refresh()', f.context);
  assert.equal(f.elements.get('raw').textContent, before);
  assert.match(f.elements.get('online').textContent, /stale/);
  assert.match(f.elements.get('notice').textContent, /HTTP 503/);
});

test('benchmark history preserves separate run identities and measurements', () => {
  const f = fixture(), now = new Date().toISOString();
  f.draw({sample_time: now, benchmark_history: [
    {run_id: 'run-b', timestamp: now, samples: [{response_model: 'model-b'}], summary: {successful_requests: 2, total_p50_ms: 25, total_p95_ms: 30}},
    {run_id: 'run-a', timestamp: now, samples: [{response_model: 'model-a'}], summary: {successful_requests: 1, total_p50_ms: 10, total_p95_ms: 10}}
  ]});
  const table = f.elements.get('benchmark-history-rows');
  assert.ok(table, 'saved benchmark history is missing from the dashboard');
  const rows = table.children;
  assert.equal(rows.length, 2);
  assert.match(rows[0].textContent, /run-b.*model-b.*25.0 ms.*30.0 ms/);
  assert.match(rows[1].textContent, /run-a.*model-a.*10.0 ms.*10.0 ms/);
});
