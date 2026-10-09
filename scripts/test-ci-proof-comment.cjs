const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { test } = require('node:test')
let publish
try { ({ publish } = require('./ci-proof-comment.cjs')) } catch (error) { if (error.code !== 'MODULE_NOT_FOUND') throw error }

const HEAD = 'a'.repeat(40), SOURCE = 'b'.repeat(40), CONTROL = SOURCE
const checks = ['messages', 'responses', 'chat-completions', 'capture', 'controls', 'claude-mod', 'quality-harness', 'telemetry']
const packages = ['localgateway', 'clientcapture', 'clientcontrol', 'claudemod', 'taskquality', 'labstatus']
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ci-proof-test-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const run = { id: 101, run_attempt: 2, workflow_id: 4, status: 'completed', conclusion: 'success', event: 'pull_request', head_sha: HEAD, head_branch: 'feature', pull_requests: [{ number: 7 }] }
  const summary = {
    'api-regression': ['regression.json', { version: 1, scope: 'api-regression', passed: true, model_inference: false, packages: packages.map(name => ({ name: `github.com/szibis/claude-escalate/internal/${name}`, test_cases: 20, passed: true })), checks: checks.map(name => ({ name, passed: true })), reasons: [] }],
    'native-client-fixture': ['native-clients.json', { version: 1, scope: 'native-client-fixture', passed: true, model_inference: false, external_provider_requests: 0, clients: [{ client: 'claude-code', version: '2.1.291', passed: true, requests: 2, tool_continuations: 1, request_path: '/v1/messages', role: 'sonnet', requested_alias: 'sentinel-sonnet', backend: 'synthetic-local' }, { client: 'codex', version: '0.160.1', passed: true, requests: 2, tool_continuations: 1, request_path: '/v1/responses', role: 'sonnet', requested_alias: 'sentinel-sonnet', backend: 'synthetic-local' }] }],
    'claude-mod': ['claude-mod.json', { version: 1, scope: 'claude-mod', client_version: '2.1.291', passed: true, framework_tests: 15, framework_failures: 0, native_commands: 3, controller_gets: 3, provider_requests: 0, model_turns: 0, model_inference: false }]
  }
  function seal(name, mutate = () => {}) {
    const dir = path.join(root, `${name}-101-2`)
    fs.mkdirSync(dir, { recursive: true })
    const [filename, original] = summary[name]
    const report = { ...original, source_revision: SOURCE, control_revision: CONTROL }
    mutate(report)
    const provenance = { head_revision: HEAD, source_revision: SOURCE, control_revision: CONTROL }
    fs.writeFileSync(path.join(dir, filename), JSON.stringify(report))
    fs.writeFileSync(path.join(dir, 'provenance.json'), JSON.stringify(provenance))
    const artifacts = [filename, 'provenance.json'].sort().map(filename => {
      const bytes = fs.readFileSync(path.join(dir, filename))
      return { path: filename, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') }
    })
    fs.writeFileSync(path.join(dir, 'evidence.json'), JSON.stringify({ version: 1, scope: 'synthetic-ci', provenance: { source_revision: SOURCE, control_revision: CONTROL, runtime_revision: null, model: null, client: null }, artifacts }))
    return dir
  }
  Object.keys(summary).forEach(name => seal(name))
  const state = { comments: [], writes: [], files: ['internal/claudemod/status.go'], pr: { number: 7, state: 'open', head: { sha: HEAD }, base: { repo: { full_name: 'trusted/sentinel' } } }, parents: [HEAD], jobs: ['api-regression', 'client-compatibility', 'claude-mod'].map(name => ({ name, conclusion: 'success', html_url: `https://github.com/trusted/sentinel/actions/runs/101/job/${name}` })) }
  const github = { rest: {
    actions: { listJobsForWorkflowRunAttempt: async () => ({ data: { jobs: state.jobs } }) },
    pulls: { get: async ({ pull_number }) => { assert.equal(pull_number, 7); return { data: state.pr } }, listFiles: async () => ({ data: state.files.map(filename => ({ filename })) }) },
    repos: { getCommit: async () => ({ data: { parents: state.parents.map(sha => ({ sha })) } }), listPullRequestsAssociatedWithCommit: async () => ({ data: [state.pr] }) },
    issues: { listComments: async () => ({ data: state.comments }), createComment: async value => { state.writes.push(value); return { data: { id: 99 } } }, updateComment: async value => { state.writes.push(value); return { data: { id: value.comment_id } } } }
  }, paginate: async (method, args) => { const { data } = await method(args); return Array.isArray(data) ? data : data.jobs } }
  const context = { repo: { owner: 'trusted', repo: 'sentinel' }, payload: { workflow_run: run, repository: { default_branch: 'main' } } }
  return { root, run, seal, state, github, context, invoke: () => publish({ github, context, artifactRoot: root }) }
}

test('publishes sealed API and both real CLI fixtures without implying real-model quality', async t => {
  assert.equal(typeof publish, 'function', 'publisher is missing')
  const f = fixture(t); await f.invoke()
  assert.equal(f.state.writes.length, 1)
  const body = f.state.writes[0].body
  assert.match(body, /API regression.*Passed/)
  assert.match(body, /6 packages; 120 test cases/)
  assert.match(body, /Claude Code.*2\.1\.291/)
  assert.match(body, /Codex.*0\.160\.1/)
  assert.match(body, /Real-model quality.*Not proven/)
  assert.match(body, /model inference: false/)
  assert.match(body, /promotion.*not authorized/i)
  assert.ok(body.includes(SOURCE) && body.includes(HEAD) && body.includes(CONTROL))
})

test('rejects tampering, missing summaries, traversal, private paths and symlinks without excerpts', async t => {
  for (const kind of ['tampered', 'missing', 'traversal', 'private', 'symlink', 'oversized', 'wrong-head', 'wrong-control']) {
    const f = fixture(t), dir = path.join(f.root, 'api-regression-101-2')
    if (kind === 'tampered') fs.appendFileSync(path.join(dir, 'regression.json'), 'PRIVATE SECRET')
    if (kind === 'missing') fs.unlinkSync(path.join(dir, 'regression.json'))
    if (kind === 'symlink') { fs.renameSync(path.join(dir, 'regression.json'), path.join(f.root, 'secret')); fs.symlinkSync(path.join(f.root, 'secret'), path.join(dir, 'regression.json')) }
    if (['traversal', 'private', 'oversized', 'wrong-control'].includes(kind)) {
      const manifest = JSON.parse(fs.readFileSync(path.join(dir, 'evidence.json')))
      if (kind === 'traversal') manifest.artifacts[0].path = '../secret'
      if (kind === 'private') manifest.artifacts[0].path = 'captures/private.json'
      if (kind === 'oversized') manifest.artifacts[0].bytes = 16 * 1024 * 1024 + 1
      if (kind === 'wrong-control') manifest.provenance.control_revision = 'c'.repeat(40)
      fs.writeFileSync(path.join(dir, 'evidence.json'), JSON.stringify(manifest))
    }
    if (kind === 'wrong-head') fs.writeFileSync(path.join(dir, 'provenance.json'), JSON.stringify({ head_revision: 'd'.repeat(40), source_revision: SOURCE, control_revision: CONTROL }))
    await f.invoke()
    assert.match(f.state.writes[0].body, /API regression.*Not proven/, kind)
    assert.doesNotMatch(f.state.writes[0].body, /PRIVATE SECRET|captures\/private|\.\.\/secret/)
  }
})

test('failed GitHub job overrides a passed sealed report', async t => {
  const f = fixture(t); f.state.jobs[0].conclusion = 'failure'; await f.invoke()
  assert.match(f.state.writes[0].body, /API regression.*Failed/)
})

test('stale PR head and newer bot attempt cannot receive an older proof', async t => {
  const f = fixture(t); f.state.pr.head.sha = 'e'.repeat(40); await f.invoke(); assert.equal(f.state.writes.length, 0)
  f.state.pr.head.sha = HEAD
  f.state.comments = [{ id: 8, user: { login: 'github-actions[bot]', type: 'Bot' }, body: '<!-- sentinel-ci-proof:pr-7 -->\n<!-- sentinel-ci-proof-run:102:1 -->' }]
  await f.invoke(); assert.equal(f.state.writes.length, 0)
  f.state.comments[0].body = '<!-- sentinel-ci-proof:pr-7 -->\n<!-- sentinel-ci-proof-run:101:3 -->'
  await f.invoke(); assert.equal(f.state.writes.length, 0)
})

test('updates only its own bot comment, retaining human and other bot content for fork PRs', async t => {
  const f = fixture(t)
  f.state.comments = [{ id: 1, user: { login: 'human', type: 'User' }, body: '<!-- sentinel-ci-proof:pr-7 -->' }, { id: 2, user: { login: 'other[bot]', type: 'Bot' }, body: '<!-- sentinel-ci-proof:pr-7 -->' }]
  f.state.pr.head.repo = { full_name: 'fork/sentinel' }
  await f.invoke(); assert.equal(f.state.writes[0].comment_id, undefined)
  f.state.comments.push({ id: 3, user: { login: 'github-actions[bot]', type: 'Bot' }, body: '<!-- sentinel-ci-proof:pr-7 -->\n<!-- sentinel-ci-proof-run:100:1 -->' })
  f.state.writes = []; await f.invoke(); assert.equal(f.state.writes[0].comment_id, 3)
})

test('rejects unrelated source commits and flags shared routing changes as needing real-model proof', async t => {
  const f = fixture(t); f.state.files = ['internal/localgateway/claude.go']; f.state.parents = []; await f.invoke()
  assert.match(f.state.writes[0].body, /API regression.*Not proven/)
  assert.match(f.state.writes[0].body, /real-model quality.*required/i)
})

test('malicious versions cannot inject Markdown or claim a provider proof', async t => {
  const f = fixture(t); f.seal('native-client-fixture', report => { report.clients[0] = { ...report.clients[0], version: '2.1\n| Real-model | Passed |' } }); await f.invoke()
  assert.match(f.state.writes[0].body, /Claude Code.*Not proven/)
  assert.doesNotMatch(f.state.writes[0].body, /\| Real-model \| Passed/)
})

test('main push attaches only to its exact merged PR association', async t => {
  const f = fixture(t); f.run.event = 'push'; f.run.head_branch = 'main'; f.run.pull_requests = []
  f.state.pr.state = 'closed'; f.state.pr.merged = true; f.state.pr.merge_commit_sha = HEAD; f.state.pr.head.sha = 'e'.repeat(40)
  await f.invoke(); assert.equal(f.state.writes.length, 1)
  f.state.writes = []; f.state.pr.merge_commit_sha = 'f'.repeat(40); await f.invoke(); assert.equal(f.state.writes.length, 0)
})

test('provider attempts and invalid counts cannot become passed compatibility proofs', async t => {
  for (const mutation of [report => { report.external_provider_requests = 1 }, report => { report.clients[0].requests = -1 }, report => { report.clients[0].tool_continuations = 0 }, report => { report.clients[1].client = 'claude-code' }]) {
    const f = fixture(t); f.seal('native-client-fixture', mutation); await f.invoke()
    assert.match(f.state.writes[0].body, /Claude Code actual CLI fixture.*Not proven/)
    assert.match(f.state.writes[0].body, /Codex actual CLI fixture.*Not proven/)
  }
})

test('unrelated scope and empty successful regression cannot become API proof', async t => {
  for (const mutation of [report => { report.scope = 'real-model-quality' }, report => { report.packages.forEach(item => { item.test_cases = 0 }) }, report => { report.checks[0] = { name: 'unknown', passed: true } }, report => { report.checks[0].passed = false }, report => { report.packages[0].name = report.packages[1].name }, report => { report.packages[0].passed = false }]) {
    const f = fixture(t); f.seal('api-regression', mutation); await f.invoke()
    assert.match(f.state.writes[0].body, /API regression.*Not proven/)
  }
})

test('non-main pushes and unfinished runs do not publish', async t => {
  const f = fixture(t); f.run.status = 'in_progress'; await f.invoke(); assert.equal(f.state.writes.length, 0)
  f.run.status = 'completed'; f.run.event = 'push'; f.run.head_branch = 'feature'; await f.invoke(); assert.equal(f.state.writes.length, 0)
})

test('documentation changes still show the fast baseline and never expose extension fields', async t => {
  const f = fixture(t); f.state.files = ['docs/client-controls.md']
  f.seal('api-regression', report => { report.private_extension = 'PRIVATE TEST OUTPUT' })
  await f.invoke()
  assert.match(f.state.writes[0].body, /Documentation only/)
  assert.match(f.state.writes[0].body, /API regression.*Passed/)
  assert.doesNotMatch(f.state.writes[0].body, /PRIVATE TEST OUTPUT/)
})

function sealExtra(f, name, manifestName, files, provenance) {
  const dir = path.join(f.root, name)
  fs.mkdirSync(dir, { recursive: true })
  const artifacts = Object.keys(files).sort().map(filename => {
    fs.mkdirSync(path.dirname(path.join(dir, filename)), { recursive: true })
    const bytes = Buffer.from(typeof files[filename] === 'string' ? files[filename] : JSON.stringify(files[filename]))
    fs.writeFileSync(path.join(dir, filename), bytes)
    return { path: filename, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') }
  })
  fs.writeFileSync(path.join(dir, manifestName), JSON.stringify({ version: 1, scope: 'synthetic-ci', provenance: { source_revision: SOURCE, control_revision: CONTROL, runtime_revision: null, model: null, client: null, ...provenance }, artifacts }))
  return dir
}
function coverage(f, mutate = () => {}) {
  const report = { statements: 1000, covered: 910, percent: 91, minimum: 90, scope: [], packages: [{ name: 'github.com/szibis/claude-escalate/internal/localgateway', statements: 1000, covered: 910, percent: 91 }], passed: true }
  const status = { test_step_outcome: 'success', test_log_compression_outcome: 'success', source_revision: SOURCE, control_revision: CONTROL }
  mutate(report, status)
  f.state.jobs.push({ name: 'test', conclusion: 'success' })
  return sealExtra(f, 'coverage', 'test-evidence.json', { 'coverage-summary.json': report, 'test-status.json': status, 'coverage.out': 'mode: atomic\n', 'test-results.jsonl.gz': 'sealed compressed event fixture' })
}
function quality(f, mutate = () => {}) {
  f.run.event = 'push'; f.run.head_branch = 'main'; f.run.pull_requests = []
  f.state.pr.state = 'closed'; f.state.pr.merged = true; f.state.pr.merge_commit_sha = HEAD
  f.state.jobs.push({ name: 'qwen-metal / Real MLX client API and cache proofs', conclusion: 'success' })
  const tasks = ['exact-read', 'coding-fix', 'loki-evidence', 'planning', 'untrusted-evidence', 'literal-markers', 'long-context', 'tool-recovery']
  const report = { version: 1, fixture_version: 2, suite: 'extended', scope: 'real-cli-task-probes', corpus_sha256: 'f'.repeat(64), started_at: '2026-10-09T01:00:00Z', finished_at: '2026-10-09T01:01:00Z', results: [] }
  for (const role of ['haiku', 'sonnet', 'opus']) for (const client of ['claude', 'codex']) for (const task of tasks) report.results.push({ task, role, client, client_version: client === 'claude' ? '2.1.291 (Claude Code)' : '0.160.1', cli_exit_code: 0, passed: true, checks: { protocol_valid: true, fixture_read: true, final_assertion: true, client_test_command: true, independent_tests: true, edit_and_tests_preserved: true, failed_read_before_success: true }, final: 'PRIVATE RAW ANSWER' })
  const results = { passed: true, checks: [], native_cli_quality: { scope: 'synthetic-native-cli-quality', evidence_directory: 'native-quality-123', passed: 48, total: 48, baseline_passed: 24, baseline_total: 24, extended_required: false } }
  mutate(report, results)
  return sealExtra(f, 'qwen-metal-101-2', 'evidence.json', { 'results.json': results, 'revisions.json': { sentinel_sha: HEAD, controls_sha: HEAD, mlx_sha: 'd'.repeat(40) }, 'native-quality-123/task-cli-quality-latest.json': report, 'benchmark-latest.json': { raw: 'PRIVATE BENCHMARK OUTPUT' }, 'benchmark-history/20261009T010100.000000000Z-0123456789abcdef0123456789abcdef.json': { raw: 'PRIVATE BENCHMARK OUTPUT' } }, { source_revision: HEAD, control_revision: HEAD, runtime_revision: 'd'.repeat(40) })
}

test('renders independently checked sealed coverage counts for the current source', async t => {
  const f = fixture(t); coverage(f); await f.invoke()
  assert.match(f.state.writes[0].body, /Coverage.*Passed.*91(?:\.0+)?%.*910\/1000 statements/)
})

test('coverage cannot pass with false percentages, stale provenance, scoped results or failed test execution', async t => {
  for (const mutate of [(r, s) => { s.source_revision = 'e'.repeat(40) }, r => { r.percent = 100 }, r => { r.covered = 2000 }, r => { r.scope = ['internal/localgateway'] }, (r, s) => { s.test_step_outcome = 'failure' }]) {
    const f = fixture(t); coverage(f, mutate); await f.invoke()
    assert.doesNotMatch(f.state.writes[0].body, /Coverage.*Passed/)
  }
})

test('current-source sealed complete native quality reports can prove 48 cases without exposing answers', async t => {
  const f = fixture(t); quality(f)
  f.state.jobs.push({ name: 'qwen-metal / Model-free smoke harness tests', conclusion: 'success' }, { name: 'qwen-metal / Hardware availability', conclusion: 'success' })
  await f.invoke()
  assert.match(f.state.writes[0].body, /Real-model quality.*Passed.*48\/48.*24\/24/)
  assert.doesNotMatch(f.state.writes[0].body, /PRIVATE RAW ANSWER|PRIVATE BENCHMARK OUTPUT/)
})

test('real-model counts never pass after tampering, unsupported partial reports or stale source', async t => {
  for (const kind of ['tamper', 'partial', 'stale', 'false-count']) {
    const f = fixture(t), dir = quality(f, (report, result) => { if (kind === 'partial') report.results.pop(); if (kind === 'false-count') result.native_cli_quality.passed = 47 })
    if (kind === 'tamper') fs.appendFileSync(path.join(dir, 'results.json'), 'PRIVATE SECRET')
    if (kind === 'stale') { const m = JSON.parse(fs.readFileSync(path.join(dir, 'evidence.json'))); m.provenance.source_revision = 'e'.repeat(40); fs.writeFileSync(path.join(dir, 'evidence.json'), JSON.stringify(m)) }
    await f.invoke(); assert.doesNotMatch(f.state.writes[0].body, /Real-model quality.*Passed/)
  }
})

test('early required-tool failure stays failed and native quality is explicitly not run', async t => {
  const f = fixture(t)
  const dir = quality(f, (report, result) => { delete result.native_cli_quality; result.passed = false; result.error = 'small-haiku-messages-json-required-tool HTTP 422: PRIVATE REQUEST' })
  fs.unlinkSync(path.join(dir, 'native-quality-123/task-cli-quality-latest.json'))
  const manifest = JSON.parse(fs.readFileSync(path.join(dir, 'evidence.json'))); manifest.artifacts = manifest.artifacts.filter(item => !item.path.includes('task-cli-quality'))
  fs.writeFileSync(path.join(dir, 'evidence.json'), JSON.stringify(manifest))
  f.state.jobs.at(-1).conclusion = 'failure'
  await f.invoke()
  assert.match(f.state.writes[0].body, /Real-model quality.*Failed.*required-tool.*422.*native quality.*not run/i)
  assert.doesNotMatch(f.state.writes[0].body, /PRIVATE REQUEST/)
})

test('native fixture rows render only the fixed protocol routes and observed sonnet role', async t => {
  const f = fixture(t); await f.invoke()
  assert.match(f.state.writes[0].body, /Claude Code actual CLI fixture.*\/v1\/messages.*sonnet/)
  assert.match(f.state.writes[0].body, /Codex actual CLI fixture.*\/v1\/responses.*sonnet/)
  f.seal('native-client-fixture', report => { report.clients[0].request_path = '/private/secret' })
  f.state.writes = []; await f.invoke()
  assert.doesNotMatch(f.state.writes[0].body, /Claude Code actual CLI fixture.*Passed|\/private\/secret/)
})

test('API contradictions and native continuation overcounts cannot become green proofs', async t => {
  for (const mutate of [r => { r.reasons = ['report_unavailable'] }, r => { r.packages[0].test_cases = 0 }]) {
    const f = fixture(t); f.seal('api-regression', mutate); await f.invoke()
    assert.doesNotMatch(f.state.writes[0].body, /API regression.*Passed/)
  }
  for (const mutate of [r => { r.clients[0].requests = 3 }, r => { r.clients[0].tool_continuations = 2 }]) {
    const f = fixture(t); f.seal('native-client-fixture', mutate); await f.invoke()
    assert.doesNotMatch(f.state.writes[0].body, /Claude Code actual CLI fixture.*Passed/)
  }
})

function dispatch(f) {
  f.run.event = 'workflow_dispatch'; f.run.head_branch = 'main'; f.run.pull_requests = []
  f.state.pr.head.sha = SOURCE
  f.github.rest.repos.getCommit = async ({ ref }) => ({ data: { sha: ref, parents: [] } })
  for (const name of ['api-regression', 'native-client-fixture', 'claude-mod']) {
    const dir = path.join(f.root, `${name}-101-2`), manifestPath = path.join(dir, 'evidence.json')
    const manifest = JSON.parse(fs.readFileSync(manifestPath))
    manifest.provenance.control_revision = HEAD
    for (const a of manifest.artifacts) {
      const file = path.join(dir, a.path), value = JSON.parse(fs.readFileSync(file))
      value.control_revision = HEAD
      if (a.path === 'provenance.json') value.head_revision = SOURCE
      const bytes = Buffer.from(JSON.stringify(value)); fs.writeFileSync(file, bytes)
      a.bytes = bytes.length; a.sha256 = crypto.createHash('sha256').update(bytes).digest('hex')
    }
    fs.writeFileSync(manifestPath, JSON.stringify(manifest))
  }
}

test('dispatched separate-source builds associate only consistent sealed reports and a current PR', async t => {
  const f = fixture(t); dispatch(f); await f.invoke()
  assert.equal(f.state.writes.length, 1)
  assert.match(f.state.writes[0].body, /API regression.*Passed/)
  assert.ok(f.state.writes[0].body.includes(`Head: \`${SOURCE}\``))
})

test('dispatch does not comment on inconsistent controls, missing proofs or stale PR source', async t => {
  for (const kind of ['wrong-control', 'missing', 'stale']) {
    const f = fixture(t); dispatch(f)
    if (kind === 'wrong-control') { const file = path.join(f.root, 'claude-mod-101-2/evidence.json'), m = JSON.parse(fs.readFileSync(file)); m.provenance.control_revision = 'f'.repeat(40); fs.writeFileSync(file, JSON.stringify(m)) }
    if (kind === 'missing') fs.rmSync(path.join(f.root, 'claude-mod-101-2'), { recursive: true })
    if (kind === 'stale') f.state.pr.head.sha = 'e'.repeat(40)
    await f.invoke(); assert.equal(f.state.writes.length, 0)
  }
})

test('failed compatibility job preserves each validated client subcheck', async t => {
  const f = fixture(t)
  f.seal('native-client-fixture', r => { r.passed = false; r.clients[1].passed = false })
  f.state.jobs.find(j => j.name === 'client-compatibility').conclusion = 'failure'
  await f.invoke()
  assert.match(f.state.writes[0].body, /Claude Code actual CLI fixture.*Failed.*subcheck passed/)
  assert.match(f.state.writes[0].body, /Codex actual CLI fixture.*Failed.*subcheck failed/)
})

test('sealed coverage and model controls from an unrelated revision remain unproven', async t => {
  for (const kind of ['coverage', 'model']) {
    const f = fixture(t), dir = kind === 'coverage' ? coverage(f) : quality(f)
    const manifestName = kind === 'coverage' ? 'test-evidence.json' : 'evidence.json'
    const manifest = JSON.parse(fs.readFileSync(path.join(dir, manifestName)))
    manifest.provenance.control_revision = 'e'.repeat(40)
    const metadataName = kind === 'coverage' ? 'test-status.json' : 'revisions.json'
    const metadata = JSON.parse(fs.readFileSync(path.join(dir, metadataName)))
    metadata[kind === 'coverage' ? 'control_revision' : 'controls_sha'] = 'e'.repeat(40)
    const bytes = Buffer.from(JSON.stringify(metadata)); fs.writeFileSync(path.join(dir, metadataName), bytes)
    const entry = manifest.artifacts.find(a => a.path === metadataName); entry.bytes = bytes.length; entry.sha256 = crypto.createHash('sha256').update(bytes).digest('hex')
    fs.writeFileSync(path.join(dir, manifestName), JSON.stringify(manifest))
    await f.invoke()
    assert.doesNotMatch(f.state.writes[0].body, new RegExp(`${kind === 'coverage' ? 'Coverage' : 'Real-model quality'}.*Passed`))
  }
})
