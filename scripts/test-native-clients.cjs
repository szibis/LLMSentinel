// Pinned real clients -> production Sentinel adapters -> synthetic loopback MLX.
// No model is loaded: this proves wire/tool compatibility, not model quality.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const http = require('node:http')
const os = require('node:os')
const path = require('node:path')
const { spawn } = require('node:child_process')

const VERSIONS = { 'claude-code': '2.1.291', codex: '0.160.1' }
const READ_COMMAND = 'cat -- marker.txt'
const LIMIT = 4 * 1024 * 1024

function contentText(content) {
  if (typeof content === 'string') return content
  assert.ok(Array.isArray(content), 'tool content must be text or text blocks')
  return content.map(block => { assert.equal(block.type, 'text'); assert.equal(typeof block.text, 'string'); return block.text }).join('\n')
}

function validateClaude(events, filename, marker) {
  const calls = new Map()
  const results = new Set()
  let final
  let completed = false
  for (const event of events) {
    assert.equal(completed, false, 'events after Claude completion')
    if (event.type === 'assistant') {
      for (const block of event.message.content) if (block.type === 'tool_use') {
        assert.equal(block.name, 'Read'); assert.equal(block.input.file_path, filename)
        assert.equal(typeof block.id, 'string'); assert.ok(block.id && !calls.has(block.id))
        calls.set(block.id, block)
      }
    } else if (event.type === 'user') {
      for (const block of event.message.content) if (block.type === 'tool_result') {
        assert.ok(calls.has(block.tool_use_id), 'uncorrelated Claude tool result')
        assert.ok(!results.has(block.tool_use_id), 'duplicate Claude tool result')
        assert.notEqual(block.is_error, true)
        assert.ok(contentText(block.content).includes(marker), 'Claude read lacks fixture marker')
        results.add(block.tool_use_id)
      }
    } else if (event.type === 'result') {
      assert.equal(event.subtype, 'success'); assert.equal(event.is_error, false)
      final = event.result; completed = true
    }
  }
  assert.equal(completed, true, 'Claude completion missing')
  assert.equal(final, marker, 'Claude exact final marker missing')
  assert.equal(calls.size, 1); assert.equal(results.size, 1)
  return results.size
}

function nativeReadCommand(command) {
  if (command === READ_COMMAND) return true
  for (const shell of ['/bin/zsh', '/bin/bash', '/bin/sh']) {
    if (command === `${shell} -lc '${READ_COMMAND}'` || command === `${shell} -lc "${READ_COMMAND}"`) return true
  }
  return false
}

function validateCodex(events, marker) {
  const reads = new Set()
  let final
  let completed = false
  for (const event of events) {
    assert.equal(completed, false, 'events after Codex completion')
    assert.notEqual(event.type, 'turn.failed'); assert.notEqual(event.type, 'error')
    if (event.type === 'item.completed') {
      const item = event.item
      if (item.type === 'command_execution') {
        assert.equal(item.status, 'completed'); assert.equal(item.exit_code, 0)
        assert.ok(nativeReadCommand(item.command), 'Codex executed an unexpected command')
        assert.equal(item.aggregated_output.trim(), marker)
        assert.ok(item.id && !reads.has(item.id)); reads.add(item.id)
      } else if (item.type === 'agent_message') final = item.text
    } else if (event.type === 'turn.completed') completed = true
  }
  assert.equal(completed, true, 'Codex completed turn missing')
  assert.equal(final, marker, 'Codex exact final marker missing')
  assert.equal(reads.size, 1)
  return reads.size
}

function validateRequests(requests, client) {
  assert.equal(requests.length, 2, 'expected one tool turn and one final turn')
  for (const [index, request] of requests.entries()) {
    assert.equal(request.method, 'POST')
    assert.equal(request.path, client === 'codex' ? '/v1/responses' : '/v1/messages')
    assert.equal(request.model, 'sentinel-sonnet')
    assert.equal(request.status, 200)
    assert.equal(request.tool_results, index)
  }
}

function validateActivity(snapshot, client, upstream) {
  assert.equal(snapshot.scope, 'local-adapter-attempts')
  assert.equal(snapshot.queued, 0); assert.deepEqual(snapshot.active, [])
  const last = snapshot.last_completed
  assert.ok(last, 'production gateway activity missing')
  assert.equal(last.client, client === 'codex' ? 'openai_responses' : 'anthropic_messages')
  assert.equal(last.role, 'sonnet'); assert.equal(last.model, 'sentinel-sonnet')
  assert.equal(last.upstream, upstream); assert.equal(last.thinking, false); assert.equal(last.accepted, true)
  return { client: last.client, role: last.role, model: last.model, thinking: last.thinking, accepted: last.accepted }
}

function privateDirectory(parent, name) {
  const dir = path.join(parent, name)
  fs.mkdirSync(dir, { mode: 0o700 }); return dir
}

function environment(root) {
  return {
    PATH: process.env.PATH || '/usr/bin:/bin', HOME: privateDirectory(root, 'home'),
    TMPDIR: privateDirectory(root, 'tmp'), LANG: 'C', LC_ALL: 'C', TERM: 'dumb',
    DISABLE_TELEMETRY: '1', DISABLE_ERROR_REPORTING: '1', DISABLE_AUTOUPDATER: '1', DISABLE_UPDATES: '1',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1', CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING: '1',
    CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS: '1', MAX_THINKING_TOKENS: '0', ENABLE_TOOL_SEARCH: 'false'
  }
}

function launch(executable, args, env, cwd, timeoutMs, children) {
  const child = spawn(executable, args, { env, cwd, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  children.add(child)
  let stdout = ''; let stderr = ''; let failure
  const kill = () => { try { process.kill(-child.pid, 'SIGKILL') } catch (error) { if (error.code !== 'ESRCH') throw error } }
  const timer = setTimeout(() => { failure = new Error('native fixture process timed out'); kill() }, timeoutMs)
  const finished = new Promise((resolve, reject) => {
    child.stdout.on('data', data => { stdout += data; if (stdout.length > LIMIT) { failure = new Error('native stdout exceeded bound'); kill() } })
    child.stderr.on('data', data => { stderr += data; if (stderr.length > LIMIT) { failure = new Error('native stderr exceeded bound'); kill() } })
    child.on('error', error => { clearTimeout(timer); children.delete(child); reject(error) })
    child.on('close', (code, signal) => {
      clearTimeout(timer); children.delete(child)
      if (failure) reject(failure)
      else resolve({ code, signal, stdout, stderr })
    })
  })
  return { child, finished, output: () => ({ stdout, stderr }) }
}

async function listen(server) {
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  return `http://127.0.0.1:${server.address().port}`
}

async function body(req) {
  const chunks = []; let size = 0
  for await (const chunk of req) { size += chunk.length; assert.ok(size <= LIMIT, 'fixture HTTP body exceeded bound'); chunks.push(chunk) }
  return Buffer.concat(chunks)
}

function getJSON(url) {
  return new Promise((resolve, reject) => {
    const request = http.get(url, async response => {
      try { assert.equal(response.statusCode, 200); resolve(JSON.parse(await body(response))) } catch (error) { reject(error) }
    })
    request.setTimeout(2000, () => request.destroy(new Error('gateway activity timed out')))
    request.on('error', reject)
  })
}

function codexCatalog() {
  return { models: [{
    slug: 'sentinel-sonnet', display_name: 'Synthetic Sentinel Sonnet role', description: 'Native wire compatibility fixture, no model inference',
    default_reasoning_level: 'medium', supported_reasoning_levels: [{ effort: 'medium', description: 'Fixture' }],
    shell_type: 'unified_exec', visibility: 'list', supported_in_api: true, priority: 1, availability_nux: null, upgrade: null,
    base_instructions: 'Read the requested file using the provided native tool. Return its contents exactly after successful execution.',
    include_skills_usage_instructions: true, include_plugin_usage_instructions: true, include_apps_usage_instructions: true,
    supports_reasoning_summary_parameter: false, default_reasoning_summary: 'none', support_verbosity: false, default_verbosity: null,
    apply_patch_tool_type: 'freeform', web_search_tool_type: 'text', truncation_policy: { mode: 'tokens', limit: 10000 },
    context_window: 32768, max_context_window: 32768, auto_compact_token_limit: 24576, effective_context_window_percent: 95,
    experimental_supported_tools: [], input_modalities: ['text'], supports_search_tool: false
  }] }
}

function toolCall(payload, client, filename, workspace) {
  if (client === 'claude-code') {
    assert.ok(payload.tools.some(tool => tool.name === 'Read'), 'native Claude Read schema absent')
    return { name: 'Read', input: { file_path: filename } }
  }
  const tools = payload.tools.flatMap(tool => tool.type === 'namespace' ? tool.tools.map(child => ({ ...child, qualified: `${tool.name}.${child.name}` })) : [{ ...tool, qualified: tool.name }])
  const exec = tools.find(tool => tool.name === 'exec_command')
  assert.ok(exec && exec.type === 'function', 'native Codex exec_command schema absent')
  return { name: exec.qualified, input: { cmd: READ_COMMAND, workdir: workspace, max_output_tokens: 128 } }
}

function toolResults(payload, client, marker) {
  const results = client === 'claude-code'
    ? payload.messages.flatMap(message => Array.isArray(message.content) ? message.content.filter(block => block.type === 'tool_result') : [])
    : payload.input.filter(item => item.type === 'function_call_output')
  if (results.length) {
    assert.equal(results.length, 1)
    assert.notEqual(results[0].is_error, true)
    const output = client === 'claude-code' ? contentText(results[0].content) : contentText(results[0].output)
    assert.ok(output.includes(marker), 'native continuation omitted actual read evidence')
    if (client === 'codex') assert.match(output, /Process exited with code 0/)
  }
  return results.length
}

function qwenCall(call) {
  return `<tool_call>\n<function=${call.name}>\n` + Object.entries(call.input).map(([key, value]) => `<parameter=${key}>\n${typeof value === 'string' ? value : JSON.stringify(value)}\n</parameter>\n`).join('') + '</function>\n</tool_call>'
}

async function runFixture(gatewayExecutable, claude, codex, outputPath) {
  const report = { scope: 'native-client-fixture', clients: Object.entries(VERSIONS).map(([client, version]) => ({ client, version, passed: false, requests: [], requests_observed: 0, tool_continuations: 0, backend_requests: [], backend_requests_observed: 0 })), passed: false, model_inference: false, external_provider_requests: 0 }
  const root = fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()), 'sentinel-native-fixture-'))
  fs.chmodSync(root, 0o700)
  const children = new Set(); const servers = []; const failures = []
  let active
  let gatewayRun
  let gatewayFailure
  let phase = 'executable_validation'
  const rejectRequest = (error, res) => { failures.push(error.message); if (!res.headersSent) { res.writeHead(400, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ error: error.message })) } else res.destroy(error) }
  try {
    ;[gatewayExecutable, claude, codex] = [gatewayExecutable, claude, codex].map(value => {
      const canonical = fs.realpathSync(path.resolve(value)); const info = fs.statSync(canonical)
      assert.ok(info.isFile() && (info.mode & 0o111), 'fixture executable is not a regular executable file')
      return canonical
    })
    phase = 'gateway_startup'
    const gatewayEnv = environment(privateDirectory(root, 'gateway'))
    const backend = http.createServer(async (req, res) => {
      try {
        assert.ok(active, 'backend request outside a client case')
        active.record.backend_requests_observed++
        if (active.backend_requests.length < 8) active.backend_requests.push({ method: req.method === 'POST' ? 'POST' : req.method === 'GET' ? 'GET' : 'unexpected', path: ['/health', '/v1/chat/completions'].includes(req.url) ? req.url : 'unexpected' })
        assert.ok(active.record.backend_requests_observed <= 4, 'unexpected backend request')
        res.setHeader('Content-Type', 'application/json')
        if (req.method === 'GET' && req.url === '/health') {
          res.end(JSON.stringify({ status: 'synthetic-fixture', model_loaded: false, capabilities: { model_family: 'qwen', thinking_control: true, reasoning_format: 'think' } })); return
        }
        assert.equal(req.method, 'POST'); assert.equal(req.url, '/v1/chat/completions')
        const payload = JSON.parse(await body(req))
        assert.equal(payload.model, 'local'); assert.equal(payload.stream, false)
        assert.equal(payload.chat_template_kwargs.enable_thinking, false)
        active.backend_posts++
        assert.ok(active.backend_posts <= 2, 'unexpected backend retry')
        const output = active.backend_posts === 1 ? qwenCall(active.call) : active.marker
        if (active.backend_posts === 2) assert.equal(active.requests[1].tool_results, 1)
        res.end(JSON.stringify({ choices: [{ message: { content: output }, finish_reason: 'stop' }], usage: { prompt_tokens: 20, completion_tokens: 8 } }))
      } catch (error) { rejectRequest(error, res) }
    })
    servers.push(backend)
    const backendURL = await listen(backend)
    gatewayRun = launch(gatewayExecutable, ['--listen', '127.0.0.1:0', '--upstream', backendURL + '/v1', '--role-haiku-upstream', backendURL + '/v1', '--role-sonnet-upstream', backendURL + '/v1', '--role-opus-upstream', backendURL + '/v1', '--timeout', '10s'], gatewayEnv, root, 150000, children)
    // Observe the background process immediately; join its original promise
    // during cleanup and surface any startup rejection in the readiness loop.
    gatewayRun.finished.then(() => {}, error => { gatewayFailure = error })
    // Read the actual bound address: no reserve/release port race or global lab.
    let gatewayURL
    const deadline = Date.now() + 10000
    while (!gatewayURL && Date.now() < deadline) {
      if (gatewayFailure) throw gatewayFailure
      const match = gatewayRun.output().stderr.match(/Sentinel gateway: (http:\/\/127\.0\.0\.1:\d+)/)
      if (match) gatewayURL = match[1]
      else { assert.equal(gatewayRun.child.exitCode, null, 'gateway exited during startup'); await new Promise(resolve => setTimeout(resolve, 10)) }
    }
    assert.ok(gatewayURL, 'gateway startup address missing')
    const audit = http.createServer(async (req, res) => {
      try {
        assert.ok(active, 'client request outside fixture case')
        const wirePath = req.url.split('?')[0]
        const request = { method: req.method === 'POST' ? 'POST' : 'unexpected', path: ['/v1/messages', '/v1/responses'].includes(wirePath) ? wirePath : 'unexpected', model: null, status: null, tool_results: 0 }
        active.record.requests_observed++
        if (active.requests.length < 8) active.requests.push(request)
        assert.equal(req.method, 'POST')
        assert.equal(wirePath, active.client === 'claude-code' ? '/v1/messages' : '/v1/responses')
        assert.ok(active.record.requests_observed <= 2, 'unexpected native client retry')
        const raw = await body(req); const payload = JSON.parse(raw)
        request.model = payload.model === 'sentinel-sonnet' ? 'sentinel-sonnet' : 'unexpected'
        assert.equal(payload.model, 'sentinel-sonnet'); assert.equal(payload.stream, true)
        request.tool_results = toolResults(payload, active.client, active.marker)
        assert.equal(request.tool_results, active.requests.length - 1)
        if (active.requests.length === 1) active.call = toolCall(payload, active.client, active.filename, active.workspace)
        const forwarded = http.request(gatewayURL + req.url, { method: req.method, headers: { ...req.headers, host: new URL(gatewayURL).host } }, upstream => {
          request.status = upstream.statusCode
          res.writeHead(upstream.statusCode, upstream.headers)
          upstream.pipe(res)
          upstream.on('error', error => rejectRequest(error, res))
        })
        forwarded.setTimeout(15000, () => forwarded.destroy(new Error('production gateway timed out')))
        forwarded.on('error', error => rejectRequest(error, res))
        res.on('close', () => forwarded.destroy())
        forwarded.end(raw)
      } catch (error) { rejectRequest(error, res) }
    })
    servers.push(audit)
    const endpoint = await listen(audit)
    for (const record of report.clients) {
      phase = 'client_configuration'
      const caseRoot = privateDirectory(root, record.client)
      const workspace = privateDirectory(caseRoot, 'workspace')
      const env = environment(caseRoot)
      const filename = path.join(workspace, 'marker.txt')
      const marker = 'SENTINEL_NATIVE_' + crypto.randomBytes(12).toString('hex')
      fs.writeFileSync(filename, marker + '\n', { mode: 0o600 })
      const executable = record.client === 'claude-code' ? claude : codex
      active = { ...record, record, workspace, filename, marker, backend_posts: 0 }
      let args
      const prompt = 'Read ' + filename + ' using your native file tool. After its successful result, reply with only the exact file content, without line-number display metadata or extra text.'
      if (record.client === 'claude-code') {
        env.CLAUDE_CONFIG_DIR = privateDirectory(caseRoot, 'claude')
        env.ANTHROPIC_BASE_URL = endpoint; env.ANTHROPIC_API_KEY = 'synthetic-local-fixture'
        env.ANTHROPIC_DEFAULT_SONNET_MODEL = 'sentinel-sonnet'
        fs.writeFileSync(path.join(env.CLAUDE_CONFIG_DIR, 'settings.json'), '{}', { mode: 0o600 })
        args = ['--bare', '--model', 'sonnet', '--print', '--verbose', '--output-format', 'stream-json', '--no-session-persistence', '--setting-sources', 'user', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}', '--permission-mode', 'dontAsk', '--tools', 'Read', '--max-turns', '3', '--allowedTools', 'Read(/' + filename + ')', '--', prompt]
      } else {
        env.CODEX_HOME = privateDirectory(caseRoot, 'codex'); env.SENTINEL_LAB_TOKEN = 'synthetic-local-fixture'
        const catalog = path.join(env.CODEX_HOME, 'catalog.json')
        fs.writeFileSync(catalog, JSON.stringify(codexCatalog()), { mode: 0o600 })
        const config = `model_catalog_json = ${JSON.stringify(catalog)}\nmodel_provider = "sentinel"\napproval_policy = "never"\nsandbox_mode = "workspace-write"\nweb_search = "disabled"\ncli_auth_credentials_store = "file"\n[analytics]\nenabled = false\n[model_providers.sentinel]\nname = "Synthetic local Sentinel fixture"\nbase_url = ${JSON.stringify(endpoint + '/v1')}\nenv_key = "SENTINEL_LAB_TOKEN"\nwire_api = "responses"\nrequires_openai_auth = false\nrequest_max_retries = 0\nstream_max_retries = 0\nstream_idle_timeout_ms = 15000\nhttp_headers = { "X-Session-ID" = "synthetic-native-fixture" }\n`
        fs.writeFileSync(path.join(env.CODEX_HOME, 'config.toml'), config, { mode: 0o600 })
        args = ['exec', '--ephemeral', '--skip-git-repo-check', '--json', '--sandbox', 'workspace-write', '-c', 'approval_policy="never"', '-c', 'model_reasoning_effort="medium"', '-m', 'sentinel-sonnet', '-C', workspace, prompt]
      }
      phase = 'client_version'
      const version = await launch(executable, ['--version'], env, workspace, 10000, children).finished
      assert.equal(version.code, 0, 'client version command failed')
      assert.match(version.stdout.trim(), new RegExp(`^(?:${record.client === 'codex' ? 'codex-cli ' : ''})${record.version.replaceAll('.', '\\.')}(${record.client === 'claude-code' ? ' \\(Claude Code\\)' : ''})?$`), 'client version differs from pinned compatibility target')
      phase = 'client_execution'
      const result = await launch(executable, args, env, workspace, 45000, children).finished
      assert.equal(result.code, 0, record.client + ' failed: ' + result.stderr.slice(-2000))
      phase = 'native_tool_evidence'
      const events = result.stdout.trim().split('\n').map(line => JSON.parse(line))
      record.tool_continuations = record.client === 'claude-code' ? validateClaude(events, filename, marker) : validateCodex(events, marker)
      phase = 'production_request_evidence'
      validateRequests(record.requests, record.client)
      assert.equal(active.backend_posts, 2, 'production gateway did not complete both synthetic turns')
      assert.equal(record.backend_requests_observed, 4)
      assert.deepEqual(failures, [], 'fixture rejected unexpected requests')
      phase = 'gateway_role_evidence'
      record.gateway_observation = validateActivity(await getJSON(gatewayURL + '/sentinel/activity'), record.client, backendURL + '/v1')
      record.backend_requests = active.backend_requests
      record.role = 'sonnet'; record.requested_alias = 'sentinel-sonnet'; record.backend_model = 'local'
      record.passed = true
    }
    report.passed = true
  } catch {
    // Public CI artifacts never receive raw assertions, transcripts, paths,
    // random file contents, or native subprocess diagnostics.
    report.error = 'fixture_validation_failed'
    report.failed_stage = phase
    if (active) report.failed_client = active.client
  } finally {
    for (const child of children) { try { process.kill(-child.pid, 'SIGKILL') } catch (error) { if (error.code !== 'ESRCH') { report.passed = false; report.error = 'fixture_cleanup_failed'; report.failed_stage = 'cleanup' } } }
    if (gatewayRun) {
      try { await gatewayRun.finished } catch { if (report.passed) { report.passed = false; report.error = 'fixture_cleanup_failed'; report.failed_stage = 'cleanup' } }
    }
    for (const server of servers.reverse()) { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)) }
    fs.rmSync(root, { recursive: true, force: true })
  }
  if (outputPath) {
    const output = path.resolve(outputPath)
    assert.equal(fs.realpathSync(path.dirname(output)), path.dirname(output), 'output directory contains symlinks')
    const descriptor = fs.openSync(output, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_NOFOLLOW, 0o600)
    try {
      const info = fs.fstatSync(descriptor)
      assert.ok(info.isFile() && info.uid === process.getuid(), 'output file must be private and owner controlled')
      fs.fchmodSync(descriptor, 0o600); fs.ftruncateSync(descriptor, 0)
      fs.writeFileSync(descriptor, JSON.stringify(report, null, 2) + '\n'); fs.fsyncSync(descriptor)
    } finally { fs.closeSync(descriptor) }
  }
  return report
}

module.exports = { validateClaude, validateCodex, validateRequests, validateActivity, runFixture }
if (require.main === module) {
  ;(async () => {
    assert.ok(process.argv.length === 5 || process.argv.length === 6, 'usage: node test-native-clients.cjs SENTINEL_GATEWAY CLAUDE CODEX [outputJSON]')
    const report = await runFixture(...process.argv.slice(2, 5), process.argv[5])
    console.log(JSON.stringify(report))
    if (!report.passed) process.exitCode = 1
  })().catch(error => { console.error(error.message); process.exitCode = 1 })
}
