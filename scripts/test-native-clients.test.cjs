const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const os = require('node:os')
const test = require('node:test')
const file = path.join(__dirname, 'test-native-clients.cjs')
const fixture = fs.existsSync(file) ? require(file) : {}
const marker = 'NATIVE_FIXTURE_MARKER'
const filename = '/private/tmp/native/marker.txt'

function claudeEvents() {
  return [
    { type: 'assistant', message: { content: [{ type: 'tool_use', id: 'read1', name: 'Read', input: { file_path: filename } }] } },
    { type: 'user', message: { content: [{ type: 'tool_result', tool_use_id: 'read1', content: `1\t${marker}`, is_error: false }] } },
    { type: 'result', subtype: 'success', is_error: false, result: marker }
  ]
}

test('Claude proof requires correlated successful native read and exact final', () => {
  assert.equal(fixture.validateClaude(claudeEvents(), filename, marker), 1)
  for (const mutate of [
    e => { e[1].message.content[0].tool_use_id = 'other' },
    e => { e[1].message.content[0].is_error = true },
    e => { e[0].message.content[0].input.file_path = '/other/file' },
    e => { e[2].result = 'I read ' + marker },
    e => { e[2].is_error = true }
  ]) { const events = claudeEvents(); mutate(events); assert.throws(() => fixture.validateClaude(events, filename, marker)) }
})

test('Codex proof requires completed exact read command with exit zero and completed turn', () => {
  const events = [
    { type: 'item.completed', item: { id: 'read1', type: 'command_execution', command: "/bin/zsh -lc 'cat -- marker.txt'", aggregated_output: marker + '\n', status: 'completed', exit_code: 0 } },
    { type: 'item.completed', item: { id: 'answer1', type: 'agent_message', text: marker } },
    { type: 'turn.completed', usage: { input_tokens: 10, output_tokens: 3 } }
  ]
  assert.equal(fixture.validateCodex(events, marker), 1)
  for (const mutate of [
    e => { e[0].item.exit_code = 1 },
    e => { e[0].item.command = "echo 'cat -- marker.txt'" },
    e => { e[0].item.status = 'in_progress' },
    e => { e[1].item.text += ' extra' },
    e => { e.pop() }
  ]) { const copy = structuredClone(events); mutate(copy); assert.throws(() => fixture.validateCodex(copy, marker)) }
})

test('production protocol proof rejects unexpected endpoints, aliases and absent continuation', () => {
  const requests = [{ method: 'POST', path: '/v1/responses', model: 'sentinel-sonnet', status: 200, tool_results: 0 }, { method: 'POST', path: '/v1/responses', model: 'sentinel-sonnet', status: 200, tool_results: 1 }]
  assert.doesNotThrow(() => fixture.validateRequests(requests, 'codex'))
  for (const mutate of [
    r => { r[0].path = '/v1/chat/completions' },
    r => { r[0].model = 'anything' },
    r => { r[1].tool_results = 0 },
    r => { r[1].status = 422 },
    r => { r.push({ method: 'POST', path: '/other' }) }
  ]) { const copy = structuredClone(requests); mutate(copy); assert.throws(() => fixture.validateRequests(copy, 'codex')) }
})

test('startup failure writes a bounded public failed summary and cleans child processes', async () => {
  const root = fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()), 'native-proof-output-'))
  const output = path.join(root, 'report.json')
  try {
    // The real Node executable refuses gateway flags; no shell mock or model.
    const report = await fixture.runFixture(process.execPath, process.execPath, process.execPath, output)
    assert.equal(report.passed, false)
    assert.equal(report.error, 'fixture_validation_failed')
    assert.equal(report.failed_stage, 'gateway_startup')
    assert.equal(report.model_inference, false)
    assert.equal(report.external_provider_requests, 0)
    assert.ok(report.clients.every(client => !client.passed && client.requests.length === 0))
    const saved = fs.readFileSync(output, 'utf8')
    assert.deepEqual(JSON.parse(saved), report)
    assert.ok(!saved.includes(root) && !saved.includes('AssertionError') && saved.length < 10000)
    assert.equal(fs.statSync(output).mode & 0o777, 0o600)
  } finally { fs.rmSync(root, { recursive: true, force: true }) }
})

test('gateway activity must corroborate the exact alias and selected local role', () => {
  const snapshot = { scope: 'local-adapter-attempts', queued: 0, active: [], last_completed: { client: 'openai_responses', role: 'sonnet', model: 'sentinel-sonnet', upstream: 'http://127.0.0.1:19091/v1', thinking: false, accepted: true } }
  const want = { client: 'openai_responses', role: 'sonnet', model: 'sentinel-sonnet', thinking: false, accepted: true }
  assert.deepEqual(fixture.validateActivity(snapshot, 'codex', 'http://127.0.0.1:19091/v1'), want)
  for (const mutate of [
    s => { s.last_completed.role = 'haiku' },
    s => { s.last_completed.client = 'anthropic_messages' },
    s => { s.last_completed.model = 'local' },
    s => { s.last_completed.upstream = 'https://provider.invalid/v1' },
    s => { s.last_completed.accepted = false }
  ]) { const copy = structuredClone(snapshot); mutate(copy); assert.throws(() => fixture.validateActivity(copy, 'codex', 'http://127.0.0.1:19091/v1')) }
})
