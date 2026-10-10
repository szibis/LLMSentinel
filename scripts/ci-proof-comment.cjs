// Trusted workflow_run publisher. Downloaded artifacts are data, never code.
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')

const SHA = /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/
const CHECKS = ['messages', 'responses', 'chat-completions', 'capture', 'controls', 'claude-mod', 'quality-harness', 'telemetry']
const PACKAGES = ['localgateway', 'clientcapture', 'clientcontrol', 'claudemod', 'taskquality', 'labstatus'].map(name => `github.com/szibis/claude-escalate/internal/${name}`)
const DEFINITIONS = [
  { artifact: 'api-regression', job: 'api-regression', report: 'regression.json', allowed: ['regression.json', 'regression-events.jsonl.gz', 'provenance.json'] },
  { artifact: 'native-client-fixture', job: 'client-compatibility', report: 'native-clients.json', allowed: ['native-clients.json', 'provenance.json'] },
  { artifact: 'claude-mod', job: 'claude-mod', report: 'claude-mod.json', allowed: ['claude-mod.json', 'provenance.json'] }
]
const count = value => Number.isSafeInteger(value) && value >= 0
const object = value => value && typeof value === 'object' && !Array.isArray(value)
const version = value => typeof value === 'string' && /^[0-9][0-9A-Za-z.+_-]{0,63}$/.test(value)
function check(condition) { if (!condition) throw new Error('Invalid public evidence') }

function readFile(root, relative, maximum) {
  check(typeof relative === 'string' && !relative.includes('\\') && relative === path.posix.normalize(relative) && !path.isAbsolute(relative) && !relative.split('/').some(part => part === '..' || part === '.' || !part))
  let current = root
  const parts = relative.split('/')
  for (const part of parts.slice(0, -1)) {
    current = path.join(current, part)
    check(fs.lstatSync(current).isDirectory() && !fs.lstatSync(current).isSymbolicLink())
  }
  const file = path.join(current, parts.at(-1))
  check(fs.lstatSync(file).isFile() && !fs.lstatSync(file).isSymbolicLink())
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW)
  try {
    const info = fs.fstatSync(fd)
    check(info.isFile() && info.size <= maximum)
    const data = fs.readFileSync(fd)
    check(data.length <= maximum && data.length === info.size)
    return data
  } finally { fs.closeSync(fd) }
}
function json(root, filename) { const result = JSON.parse(readFile(root, filename, 256 * 1024)); check(object(result)); return result }

function sealed(dir, manifestName, allowed, budget, retain = name => name.endsWith('.json')) {
  check(fs.lstatSync(dir).isDirectory() && !fs.lstatSync(dir).isSymbolicLink())
  const manifest = json(dir, manifestName), p = manifest.provenance
  check(manifest.version === 1 && manifest.scope === 'synthetic-ci' && object(p) && SHA.test(p.source_revision) && SHA.test(p.control_revision))
  check(p.model == null && p.client == null && (p.runtime_revision == null || SHA.test(p.runtime_revision)))
  check(Array.isArray(manifest.artifacts) && manifest.artifacts.length > 0 && manifest.artifacts.length <= 256)
  let previous = ''
  const data = new Map()
  for (const a of manifest.artifacts) {
    check(object(a) && typeof a.path === 'string' && allowed(a.path) && a.path > previous)
    previous = a.path
    check(count(a.bytes) && a.bytes <= 16 * 1024 * 1024 && /^[a-f0-9]{64}$/.test(a.sha256))
    budget.bytes += a.bytes; check(budget.bytes <= 64 * 1024 * 1024)
    const bytes = readFile(dir, a.path, 16 * 1024 * 1024)
    check(bytes.length === a.bytes && crypto.createHash('sha256').update(bytes).digest('hex') === a.sha256)
    // Retain only JSON summaries. Logs/profiles are hashed, never parsed/rendered.
    if (retain(a.path)) data.set(a.path, bytes)
  }
  return { manifest, data }
}

async function currentSource(source, head, github, repo, cache) {
  if (source === head) return true
  if (!cache.has(source)) {
    const response = await github.rest.repos.getCommit({ ...repo, ref: source })
    cache.set(source, response.data.parents?.some(parent => parent.sha === head) === true)
  }
  return cache.get(source)
}

async function evidence(artifactRoot, definition, run, github, repo, commitCache, budget) {
  try {
    const root = fs.realpathSync(artifactRoot)
    const name = `${definition.artifact}-${run.id}-${run.run_attempt}`
    const dir = path.join(root, name)
    check(fs.lstatSync(dir).isDirectory() && !fs.lstatSync(dir).isSymbolicLink())
    const manifest = json(dir, 'evidence.json')
    const provenance = json(dir, 'provenance.json')
    check(manifest.version === 1 && manifest.scope === 'synthetic-ci' && object(manifest.provenance))
    check(SHA.test(provenance.source_revision) && SHA.test(provenance.control_revision))
    check(provenance.head_revision === (run.event === 'workflow_dispatch' ? provenance.source_revision : run.head_sha))
    if (run.event === 'workflow_dispatch') check(provenance.control_revision === run.head_sha)
    else check(provenance.control_revision === provenance.source_revision || provenance.control_revision === run.head_sha)
    check(manifest.provenance.source_revision === provenance.source_revision && manifest.provenance.control_revision === provenance.control_revision)
    check(manifest.provenance.model == null && manifest.provenance.client == null && manifest.provenance.runtime_revision == null)
    check(Array.isArray(manifest.artifacts) && manifest.artifacts.length >= 2 && manifest.artifacts.length <= definition.allowed.length)
    let previous = ''
    const data = new Map()
    for (const artifact of manifest.artifacts) {
      check(object(artifact) && definition.allowed.includes(artifact.path) && artifact.path > previous)
      previous = artifact.path
      check(count(artifact.bytes) && artifact.bytes <= 16 * 1024 * 1024 && /^[a-f0-9]{64}$/.test(artifact.sha256))
      budget.bytes += artifact.bytes
      check(budget.bytes <= 64 * 1024 * 1024)
      const bytes = readFile(dir, artifact.path, artifact.path === 'provenance.json' ? 256 * 1024 : 16 * 1024 * 1024)
      check(bytes.length === artifact.bytes && crypto.createHash('sha256').update(bytes).digest('hex') === artifact.sha256)
      if (artifact.path !== 'regression-events.jsonl.gz') data.set(artifact.path, bytes)
    }
    check(data.has('provenance.json') && data.has(definition.report))
    // Confirm the source is this head or an API-confirmed PR merge checkout.
    if (run.event === 'workflow_dispatch') {
      const key = `exists:${provenance.source_revision}`
      if (!commitCache.has(key)) {
        const response = await github.rest.repos.getCommit({ ...repo, ref: provenance.source_revision })
        commitCache.set(key, response.data.sha === provenance.source_revision)
      }
      check(commitCache.get(key))
    } else if (provenance.source_revision !== run.head_sha) {
      if (!commitCache.has(provenance.source_revision)) {
        const response = await github.rest.repos.getCommit({ ...repo, ref: provenance.source_revision })
        commitCache.set(provenance.source_revision, response.data.parents?.some(parent => parent.sha === run.head_sha) === true)
      }
      check(commitCache.get(provenance.source_revision))
    }
    const report = JSON.parse(data.get(definition.report))
    check(object(report) && report.version === 1 && report.scope === definition.artifact && report.source_revision === provenance.source_revision && report.control_revision === provenance.control_revision && typeof report.passed === 'boolean' && report.model_inference === false)
    if (definition.artifact === 'api-regression') {
      check(Array.isArray(report.packages) && report.packages.length === PACKAGES.length)
      check(PACKAGES.every(name => report.packages.filter(item => object(item) && item.name === name && typeof item.passed === 'boolean' && count(item.test_cases)).length === 1))
      const tests = report.packages.reduce((sum, item) => sum + item.test_cases, 0)
      check(count(tests) && (!report.passed || (tests > 0 && report.packages.every(item => item.passed && item.test_cases > 0))))
      check(Array.isArray(report.checks) && report.checks.length === CHECKS.length)
      check(CHECKS.every(name => report.checks.filter(item => object(item) && item.name === name && typeof item.passed === 'boolean').length === 1))
      check(!report.passed || report.checks.every(item => item.passed))
      // Reasons remain structured producer codes; never echo diagnostic prose.
      check(Array.isArray(report.reasons) && report.reasons.length <= 16 && report.reasons.every(reason => typeof reason === 'string' && /^[a-z0-9_-]{1,64}$/.test(reason)))
      check(!report.passed || report.reasons.length === 0)
    } else if (definition.artifact === 'native-client-fixture') {
      check(report.external_provider_requests === 0 && Array.isArray(report.clients) && report.clients.length === 2)
      check(['claude-code', 'codex'].every(client => report.clients.filter(item => item.client === client).length === 1))
      const versions = { 'claude-code': '2.1.291', codex: '0.160.1' }
      check(report.clients.every(item => object(item) && version(item.version) && item.version === versions[item.client] && typeof item.passed === 'boolean' && count(item.requests) && count(item.tool_continuations)))
      check(report.clients.every(item => item.request_path === (item.client === 'claude-code' ? '/v1/messages' : '/v1/responses') && item.role === 'sonnet' && item.requested_alias === 'sentinel-sonnet' && item.backend === 'synthetic-local'))
      check(!report.passed || report.clients.every(item => item.passed && item.requests === 2 && item.tool_continuations === 1))
    } else {
      check(report.client_version === '2.1.291' && count(report.framework_tests) && count(report.framework_failures) && count(report.native_commands) && count(report.controller_gets) && report.provider_requests === 0 && report.model_turns === 0)
      check(!report.passed || (report.framework_tests > 0 && report.framework_failures === 0 && report.native_commands === 3 && report.controller_gets === 3))
    }
    return { report, provenance }
  } catch { return null }
}

async function coverageEvidence(artifactRoot, run, github, repo, cache, budget, workflowHead) {
  try {
    const dir = path.join(fs.realpathSync(artifactRoot), 'coverage')
    const { manifest, data } = sealed(dir, 'test-evidence.json', name => ['coverage.out', 'coverage-summary.json', 'test-status.json', 'test-results.jsonl.gz'].includes(name), budget)
    const p = manifest.provenance
    check(run.event === 'workflow_dispatch' ? p.control_revision === workflowHead : (p.control_revision === p.source_revision || p.control_revision === run.head_sha))
    check(p.runtime_revision == null && data.has('coverage-summary.json') && data.has('test-status.json'))
    check(manifest.artifacts.some(a => a.path === 'coverage.out') && manifest.artifacts.some(a => a.path === 'test-results.jsonl.gz'))
    check(await currentSource(p.source_revision, run.head_sha, github, repo, cache))
    const r = JSON.parse(data.get('coverage-summary.json')), status = JSON.parse(data.get('test-status.json'))
    check(object(r) && object(status) && status.source_revision === p.source_revision && status.control_revision === p.control_revision)
    check(['success', 'failure', 'skipped', 'cancelled'].includes(status.test_step_outcome) && ['success', 'failure', 'skipped', 'cancelled'].includes(status.test_log_compression_outcome))
    check(count(r.statements) && r.statements > 0 && count(r.covered) && r.covered <= r.statements && r.minimum === 90 && Array.isArray(r.scope) && r.scope.length === 0 && typeof r.passed === 'boolean')
    check(typeof r.percent === 'number' && Number.isFinite(r.percent) && Math.abs(r.percent - 100 * r.covered / r.statements) < 1e-9 && r.passed === (r.percent >= r.minimum))
    check(Array.isArray(r.packages) && r.packages.length > 0 && r.packages.length <= 256)
    const names = new Set()
    let statements = 0, covered = 0
    for (const item of r.packages) {
      check(object(item) && typeof item.name === 'string' && /^github\.com\/szibis\/claude-escalate\/[A-Za-z0-9_/-]{1,128}$/.test(item.name) && !names.has(item.name))
      names.add(item.name)
      check(count(item.statements) && count(item.covered) && item.covered <= item.statements && typeof item.percent === 'number' && Number.isFinite(item.percent))
      check(Math.abs(item.percent - (item.statements ? 100 * item.covered / item.statements : 0)) < 1e-9)
      statements += item.statements; covered += item.covered
    }
    check(count(statements) && count(covered) && statements === r.statements && covered === r.covered)
    return { report: { ...r, passed: r.passed && status.test_step_outcome === 'success' && status.test_log_compression_outcome === 'success' }, provenance: p }
  } catch { return null }
}

function qualityCases(report) {
  const baseTasks = ['exact-read', 'coding-fix', 'loki-evidence', 'planning']
  const tasks = [...baseTasks, 'untrusted-evidence', 'literal-markers', 'long-context', 'tool-recovery']
  check(object(report) && report.version === 1 && report.fixture_version === 2 && report.scope === 'real-cli-task-probes' && report.suite === 'extended' && /^[a-f0-9]{64}$/.test(report.corpus_sha256))
  const start = Date.parse(report.started_at), finish = Date.parse(report.finished_at)
  check(Number.isFinite(start) && Number.isFinite(finish) && finish >= start && Array.isArray(report.results) && report.results.length === 48)
  const seen = new Set()
  let passed = 0, baseline = 0
  const failures = []
  for (const r of report.results) {
    check(object(r) && tasks.includes(r.task) && ['haiku', 'sonnet', 'opus'].includes(r.role) && ['claude', 'codex'].includes(r.client) && typeof r.passed === 'boolean')
    const key = `${r.client}/${r.role}/${r.task}`; check(!seen.has(key)); seen.add(key)
    if (r.passed) {
      check(r.cli_exit_code === 0 && typeof r.client_version === 'string' && r.client_version.length > 0 && r.client_version.length <= 128 && object(r.checks))
      check(['protocol_valid', 'fixture_read', 'final_assertion'].every(name => r.checks[name] === true))
      if (r.task === 'coding-fix') check(['client_test_command', 'independent_tests', 'edit_and_tests_preserved'].every(name => r.checks[name] === true))
      if (r.task === 'tool-recovery') check(r.checks.failed_read_before_success === true)
      passed++; if (baseTasks.includes(r.task)) baseline++
    } else failures.push(key)
  }
  return { passed, total: 48, baseline, baselineTotal: 24, failures }
}

function requiredToolFailure(error) {
  if (typeof error !== 'string') return null
  const match = /\b(small-haiku|large-sonnet|large-opus)-(messages|responses|chat)-(json|sse)-required-tool\b[^\r\n]{0,120}?\bHTTP\s*([45][0-9]{2})\b/.exec(error)
  return match ? `${match[1]}-${match[2]}-${match[3]}-required-tool HTTP ${match[4]}` : null
}

function modelEvidence(artifactRoot, run, budget) {
  try {
    // Hardware runs only on a main push. Never import another run's model proof.
    check(run.event === 'push')
    const dir = path.join(fs.realpathSync(artifactRoot), `qwen-metal-${run.id}-${run.run_attempt}`)
    const allowed = name => ['results.json', 'revisions.json', 'benchmark-latest.json', 'large-gateway.log', 'large-runtime.log', 'small-gateway.log', 'small-runtime.log'].includes(name) || /^native-quality-[0-9]+\/(?:task-cli-quality-latest\.json|jes-quality-latest\.json|task-quality-runs\/run-[0-9]+\.json)$/.test(name) || /^benchmark-history\/[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[a-f0-9]{32}\.json$/.test(name)
    const { manifest, data } = sealed(dir, 'evidence.json', allowed, budget, name => ['results.json', 'revisions.json'].includes(name) || /^native-quality-[0-9]+\/task-cli-quality-latest\.json$/.test(name))
    const p = manifest.provenance
    check(p.source_revision === run.head_sha && p.control_revision === run.head_sha && SHA.test(p.runtime_revision) && data.has('results.json') && data.has('revisions.json'))
    const revisions = JSON.parse(data.get('revisions.json')), result = JSON.parse(data.get('results.json'))
    check(object(revisions) && revisions.sentinel_sha === p.source_revision && revisions.controls_sha === p.control_revision && revisions.mlx_sha === p.runtime_revision)
    check(object(result) && typeof result.passed === 'boolean')
    const native = result.native_cli_quality
    if (!native) return { provenance: p, passed: false, failed: !result.passed, reason: requiredToolFailure(result.error) || 'smoke failed or native quality unavailable', notRun: true }
    check(object(native) && native.scope === 'synthetic-native-cli-quality' && /^native-quality-[0-9]+$/.test(native.evidence_directory))
    const reportPath = `${native.evidence_directory}/task-cli-quality-latest.json`
    check(data.has(reportPath))
    const summary = qualityCases(JSON.parse(data.get(reportPath)))
    check(native.passed === summary.passed && native.total === summary.total && native.baseline_passed === summary.baseline && native.baseline_total === summary.baselineTotal)
    return { provenance: p, ...summary, passedCases: summary.passed, passed: result.passed && summary.passed === 48 && summary.baseline === 24, failed: !result.passed || summary.passed < 48 }
  } catch { return null }
}

async function list(github, method, args, field) {
  if (github.paginate) return github.paginate(method, { ...args, per_page: 100 })
  const items = []
  for (let page = 1; page <= 100; page++) {
    const { data } = await method({ ...args, per_page: 100, page })
    const batch = field ? data[field] : data
    check(Array.isArray(batch)); items.push(...batch)
    if (batch.length < 100) return items
  }
  throw new Error('GitHub pagination exceeds limit')
}

function jobStatus(jobs, name, proof) {
  const matches = jobs.filter(job => job.name === name)
  if (matches.some(job => ['failure', 'cancelled', 'timed_out', 'action_required', 'startup_failure'].includes(job.conclusion))) return 'Failed'
  if (matches.length !== 1 || matches[0].conclusion !== 'success' || !proof) return 'Not proven'
  return proof.report.passed ? 'Passed (synthetic fixture)' : 'Failed'
}

function changeScope(files) {
  if (files.some(file => /^(internal\/localgateway\/|cmd\/sentinel-gateway\/)/.test(file))) return 'Shared API/routing changed; real-model quality proof is required.'
  if (files.some(file => /^internal\/claudemod\//.test(file))) return 'Claude mod changed; native Claude Code proof and shared regression baseline are required.'
  if (files.some(file => /^(internal\/(clientcapture|clientcontrol|lab)\/|scripts\/test-native-clients)/.test(file))) return 'Shared client integration changed; Claude Code and Codex compatibility proofs are required.'
  if (files.length && files.every(file => /^(docs\/|README\.md$|[^/]+\.md$)/.test(file))) return 'Documentation only; the same fast API and native-client baseline is shown.'
  return 'Shared regression and native-client compatibility baseline is shown; real-model behavior remains unproven.'
}

function render(number, run, repo, proofs, jobs, files, coverage, model) {
  const url = `https://github.com/${repo.owner}/${repo.repo}/actions/runs/${run.id}/attempts/${run.run_attempt}`
  const artifacts = `https://github.com/${repo.owner}/${repo.repo}/actions/runs/${run.id}#artifacts`
  const rows = []
  const api = proofs[0], native = proofs[1], mod = proofs[2]
  rows.push(`| API regression | ${jobStatus(jobs, 'api-regression', api)} | ${api ? `${api.report.packages.length} packages; ${api.report.packages.reduce((sum, item) => sum + item.test_cases, 0)} test cases; 8 checks: ${CHECKS.join(', ')}; report v1` : 'Missing, invalid or stale sealed report; counts/schema unknown'} |`)
  for (const [id, label] of [['claude-code', 'Claude Code'], ['codex', 'Codex']]) {
    const client = native?.report.clients.find(client => client.client === id)
    rows.push(`| ${label} actual CLI fixture | ${jobStatus(jobs, 'client-compatibility', native)}; client subcheck ${client ? (client.passed ? 'passed' : 'failed') : 'unknown'} | ${client ? `${client.version}; ${client.request_path}; role ${client.role} (${client.requested_alias}); ${client.backend}; ${client.requests} requests; ${client.tool_continuations} tool continuations; report v1` : 'Version, counts, route, role and schema unknown'} |`)
  }
  rows.push(`| Claude mod | ${jobStatus(jobs, 'claude-mod', mod)} | ${mod ? `${mod.report.client_version}; ${mod.report.framework_tests} framework tests; ${mod.report.native_commands} native commands; ${mod.report.controller_gets} controller GETs; report v1` : 'Version, counts and schema unknown'} |`)
  const qwen = jobs.filter(job => job.name === 'Real MLX client API and cache proofs' || job.name?.endsWith('/ Real MLX client API and cache proofs'))
  const hardware = qwen.length ? qwen.map(job => ['success', 'failure', 'skipped', 'cancelled', 'timed_out'].includes(job.conclusion) ? job.conclusion : 'unknown').join(', ') : 'skipped or unavailable'
  const hardwareFailed = qwen.some(job => ['failure', 'cancelled', 'timed_out', 'action_required', 'startup_failure'].includes(job.conclusion))
  const modelVerdict = hardwareFailed || model?.failed ? 'Failed / not proven' : model?.passed && qwen.length === 1 && qwen[0].conclusion === 'success' ? 'Passed (real local-model inference)' : 'Not proven / skipped'
  const modelDetails = !model ? 'no current-source sealed quality report; counts unknown' : model.notRun ? `${model.reason}; native quality not run; counts unknown` : `${model.passedCases}/48 CLI cases; baseline ${model.baseline}/24; ${model.failures.length ? `failed cases: ${model.failures.join(', ')}` : 'all retained cases passed'}; model inference: true; fixture v2`
  rows.push(`| Real-model quality | ${modelVerdict} | Hardware job: ${hardware}; ${modelDetails} |`)
  rows.push(`| Coverage | ${jobStatus(jobs, 'test', coverage)} | ${coverage ? `${coverage.report.percent.toFixed(6)}%; ${coverage.report.covered}/${coverage.report.statements} statements; ${coverage.report.packages.length} packages; minimum ${coverage.report.minimum}%; whole repository` : 'Percentage and counts unknown; missing, invalid or stale sealed coverage artifact'} |`)
  const revisions = [...proofs, coverage, model].filter(Boolean).map(proof => `Tested/source: \`${proof.provenance.source_revision}\`; controls: \`${proof.provenance.control_revision}\`.`)
  return [`<!-- sentinel-ci-proof:pr-${number} -->`, `<!-- sentinel-ci-proof-run:${run.id}:${run.run_attempt} -->`, '### CI proofs', '', changeScope(files), '', `Head: \`${run.head_sha}\`.`, ...new Set(revisions.length ? revisions : ['Tested/source and controls revisions: unknown.']), '', `Build result: ${['success', 'failure', 'cancelled', 'timed_out'].includes(run.conclusion) ? run.conclusion : 'unknown'}. [Run and job logs](${url}) · [Proofs and existing coverage artifacts](${artifacts})`, '', '| Proof | Verdict | Evidence |', '| --- | --- | --- |', ...rows, '', 'Synthetic fixtures: model inference: false; external provider/model requests: zero when proven. These checks establish regression and protocol compatibility; training/promotion is not authorized.'].join('\n')
}

async function publish({ github, context, artifactRoot, core }) {
  const run = context.payload.workflow_run
  const repo = context.repo
  check(object(run) && SHA.test(run.head_sha) && count(run.id) && run.id > 0 && count(run.run_attempt) && run.run_attempt > 0)
  if (run.status !== 'completed' || !['pull_request', 'push', 'workflow_dispatch'].includes(run.event)) return { published: 0 }
  const onMain = run.event === 'push' && run.head_branch === context.payload.repository?.default_branch
  if (run.event === 'push' && !onMain) return { published: 0 }
  const jobs = await list(github, github.rest.actions.listJobsForWorkflowRunAttempt, { ...repo, run_id: run.id, attempt_number: run.run_attempt }, 'jobs')
  const cache = new Map(), budget = { bytes: 0 }
  const proofs = []
  for (const definition of DEFINITIONS) proofs.push(await evidence(artifactRoot, definition, run, github, repo, cache, budget))
  let testedRun = run
  if (run.event === 'workflow_dispatch') {
    const present = proofs.filter(Boolean)
    const sources = new Set(present.map(proof => proof.provenance.source_revision))
    // workflow_run omits dispatch inputs. Separate-source association therefore
    // requires all three independent sealed summaries and trusted control SHA.
    if (sources.size > 1 || (present.length !== proofs.length && present.some(proof => proof.provenance.source_revision !== run.head_sha))) return { published: 0 }
    if (present.length === proofs.length) testedRun = { ...run, head_sha: present[0].provenance.source_revision }
  }
  const candidates = onMain || run.event === 'workflow_dispatch'
    ? await list(github, github.rest.repos.listPullRequestsAssociatedWithCommit, { ...repo, commit_sha: testedRun.head_sha })
    : (run.pull_requests || [])
  const coverage = await coverageEvidence(artifactRoot, testedRun, github, repo, cache, budget, run.head_sha)
  const model = modelEvidence(artifactRoot, run, budget)
  let published = 0
  for (const number of new Set(candidates.map(pr => pr.number).filter(number => count(number) && number > 0))) {
    const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: number })
    if (pr.base?.repo?.full_name !== `${repo.owner}/${repo.repo}`) continue
    if (onMain ? !(pr.merged && pr.merge_commit_sha === testedRun.head_sha) : !(pr.state === 'open' && pr.head?.sha === testedRun.head_sha)) continue
    const comments = await list(github, github.rest.issues.listComments, { ...repo, issue_number: number })
    const marker = `<!-- sentinel-ci-proof:pr-${number} -->`
    const own = comments.filter(comment => comment.user?.type === 'Bot' && comment.user?.login === 'github-actions[bot]' && comment.body?.includes(marker))
    if (own.some(comment => {
      const previous = /<!-- sentinel-ci-proof-run:(\d+):(\d+) -->/.exec(comment.body)
      return previous && (BigInt(previous[1]) > BigInt(run.id) || (BigInt(previous[1]) === BigInt(run.id) && BigInt(previous[2]) > BigInt(run.run_attempt)))
    })) continue
    const files = await list(github, github.rest.pulls.listFiles, { ...repo, pull_number: number })
    const body = render(number, testedRun, repo, proofs, jobs, files.map(file => file.filename), coverage, model)
    // Check the current PR again immediately before writing; workflows serialize
    // this publisher by repository so parallel runs cannot race comment updates.
    const { data: current } = await github.rest.pulls.get({ ...repo, pull_number: number })
    if (onMain ? !(current.merged && current.merge_commit_sha === testedRun.head_sha) : !(current.state === 'open' && current.head?.sha === testedRun.head_sha)) continue
    const previous = own.sort((a, b) => b.id - a.id)[0]
    if (previous) await github.rest.issues.updateComment({ ...repo, comment_id: previous.id, body })
    else await github.rest.issues.createComment({ ...repo, issue_number: number, body })
    if (core?.summary) await core.summary.addRaw(body).write()
    published++
  }
  return { published }
}

module.exports = { publish }
