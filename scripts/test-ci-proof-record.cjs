const assert = require('node:assert/strict')
const test = require('node:test')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { record } = require('./record-ci-proof.cjs')
const env = { HEAD_REVISION: 'a'.repeat(40), SOURCE_REVISION: 'b'.repeat(40), CONTROL_REVISION: 'b'.repeat(40), PROOF_OUTCOME: 'success', VALIDATE_OUTCOME: 'success', FRAMEWORK_OUTCOME: 'success' }
function fixture(t) { const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ci-record-')); t.after(() => fs.rmSync(root, { recursive: true, force: true })); return root }
function write(root, file, value) { fs.writeFileSync(path.join(root, file), JSON.stringify(value)) }
test('missing reports remain unknown and fail even when step claims success', t => {
  const root = fixture(t)
  for (const scope of ['api-regression', 'native-client-fixture', 'claude-mod']) assert.equal(record(scope, root, env).passed, false)
})
test('mod report needs all outcomes and measured counts without copying logs', t => {
  const root = fixture(t)
  fs.writeFileSync(path.join(root, 'framework.log'), 'PRIVATE LOG\n 15 pass\n 0 fail\n')
  write(root, 'native-mod.json', { client_version: '2.1.291', native_commands: 3, controller_gets: 3, provider_requests: 0, model_turns: 0, arbitrary: 'PRIVATE ANSWER' })
  assert.equal(record('claude-mod', root, env).passed, true)
  assert.equal(record('claude-mod', root, { ...env, VALIDATE_OUTCOME: 'skipped' }).passed, false)
  assert.equal(fs.readFileSync(path.join(root, 'claude-mod.json'), 'utf8').includes('PRIVATE'), false)
  fs.appendFileSync(path.join(root, 'framework.log'), ' 15 pass\n')
  assert.equal(record('claude-mod', root, env).passed, false)
})
test('native client success requires exact versions and actual tool continuation for both clients', t => {
  const root = fixture(t)
  const r = { passed: true, model_inference: false, external_provider_requests: 0, clients: [['claude-code', '2.1.291', '/v1/messages'], ['codex', '0.160.1', '/v1/responses']].map(([client, version, route]) => ({ client, version, passed: true, requests: [0, 1].map(tool_results => ({ method: 'POST', path: route, status: 200, model: 'sentinel-sonnet', tool_results })), requests_observed: 2, tool_continuations: 1, role: 'sonnet', requested_alias: 'sentinel-sonnet', backend_model: 'local', gateway_observation: { client: client === 'codex' ? 'openai_responses' : 'anthropic_messages', role: 'sonnet', model: 'sentinel-sonnet', accepted: true, thinking: false }, backend_requests_observed: 4, backend_requests: Array.from({ length: 2 }, () => [{ method: 'GET', path: '/health' }, { method: 'POST', path: '/v1/chat/completions' }]).flat() })) }
  write(root, 'native-clients.json', r)
  assert.equal(record('native-client-fixture', root, env).passed, true)
  r.clients[1].tool_continuations = 0
  write(root, 'native-clients.json', r)
  assert.equal(record('native-client-fixture', root, env).passed, false)
  r.clients[1].tool_continuations = 1
  r.clients[1].gateway_observation.accepted = false
  write(root, 'native-clients.json', r)
  assert.equal(record('native-client-fixture', root, env).passed, false)
  r.clients[1].gateway_observation.accepted = true
  r.clients[1].backend_requests[3].path = '/unexpected'
  write(root, 'native-clients.json', r)
  assert.equal(record('native-client-fixture', root, env).passed, false)
})
test('wrong revisions and stale regression evidence are rejected before rewriting', t => {
  const root = fixture(t)
  assert.throws(() => record('claude-mod', root, { ...env, SOURCE_REVISION: 'main' }))
  write(root, 'regression.json', { source_revision: 'c'.repeat(40), control_revision: env.CONTROL_REVISION })
  assert.throws(() => record('api-regression', root, env), /Stale/)
  assert.equal(JSON.parse(fs.readFileSync(path.join(root, 'regression.json'))).source_revision, 'c'.repeat(40))
})
