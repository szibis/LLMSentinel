// Write only fixed public CI summaries. Never copy test output into comments.
const fs = require('node:fs')
const path = require('node:path')
const revision = /^[a-f0-9]{40}(?:[a-f0-9]{24})?$/
function readJSON(root, name) {
  try { return JSON.parse(fs.readFileSync(path.join(root, name), 'utf8')) } catch { return null }
}
function count(root, kind) {
  try {
    const matches = [...fs.readFileSync(path.join(root, 'framework.log'), 'utf8').matchAll(new RegExp(`^\\s*(\\d+) ${kind}\\s*$`, 'gm'))]
    return matches.length === 1 ? Number(matches[0][1]) : null
  } catch { return null }
}
function record(scope, root, env) {
  const p = { head_revision: env.HEAD_REVISION, source_revision: env.SOURCE_REVISION, control_revision: env.CONTROL_REVISION }
  if (!Object.values(p).every(value => typeof value === 'string' && revision.test(value))) throw new Error('Invalid proof revisions')
  let report
  let name
  if (scope === 'api-regression') {
    name = 'regression.json'
    report = readJSON(root, name)
    if (!report) report = { version: 1, scope, passed: false, model_inference: false, packages: [], checks: [], reasons: ['report_unavailable'] }
    if (report.source_revision && (report.source_revision !== p.source_revision || report.control_revision !== p.control_revision)) throw new Error('Stale regression report')
  } else if (scope === 'native-client-fixture') {
    name = 'native-clients.json'
    const r = readJSON(root, name)
    const versions = { 'claude-code': '2.1.291', codex: '0.160.1' }
    const clients = Object.entries(versions).map(([client, version]) => {
      const entries = r?.clients?.filter(item => item.client === client) || []
      const item = entries.length === 1 ? entries[0] : null
      const route = client === 'claude-code' ? '/v1/messages' : '/v1/responses'
      const calls = Array.isArray(item?.requests) ? item.requests : []
      const observed = calls.length === 2 && item.requests_observed === 2 && calls.every(call => call.method === 'POST' && call.path === route && call.status === 200 && call.model === 'sentinel-sonnet') && calls[0].tool_results === 0 && calls[1].tool_results === 1
      const activity = item?.gateway_observation
      const routed = activity?.client === (client === 'codex' ? 'openai_responses' : 'anthropic_messages') && activity?.role === 'sonnet' && activity?.model === 'sentinel-sonnet' && activity?.accepted === true && activity?.thinking === false
      const backend = Array.isArray(item?.backend_requests) ? item.backend_requests : []
      const backendObserved = item?.backend_requests_observed === 4 && backend.length === 4 && backend.every((call, index) => call?.method === (index % 2 ? 'POST' : 'GET') && call?.path === (index % 2 ? '/v1/chat/completions' : '/health'))
      return { client, version: item?.version === version ? version : 'unknown',
        passed: item?.passed === true && item?.version === version && observed && routed && backendObserved && item?.role === 'sonnet' && item?.requested_alias === 'sentinel-sonnet' && item?.backend_model === 'local' && item.tool_continuations === 1,
        requests: item ? calls.length : null, request_path: route, role: 'sonnet', requested_alias: 'sentinel-sonnet', backend: 'synthetic-local',
        tool_continuations: Number.isSafeInteger(item?.tool_continuations) && item.tool_continuations >= 0 ? item.tool_continuations : null }
    })
    report = { version: 1, scope, passed: env.PROOF_OUTCOME === 'success' && r?.passed === true && r?.model_inference === false && r?.external_provider_requests === 0 && clients.every(c => c.passed),
      model_inference: false, external_provider_requests: r?.external_provider_requests === 0 ? 0 : null, clients }
  } else if (scope === 'claude-mod') {
    name = 'claude-mod.json'
    const r = readJSON(root, 'native-mod.json')
    const passes = count(root, 'pass'), failures = count(root, 'fail')
    const keys = ['native_commands', 'controller_gets', 'provider_requests', 'model_turns']
    report = { version: 1, scope, model_inference: false, client_version: r?.client_version === '2.1.291' ? '2.1.291' : 'unknown', framework_tests: passes, framework_failures: failures,
      passed: env.VALIDATE_OUTCOME === 'success' && env.FRAMEWORK_OUTCOME === 'success' && env.PROOF_OUTCOME === 'success' && passes > 0 && failures === 0 && r?.client_version === '2.1.291' && r?.native_commands === 3 && r?.controller_gets === 3 && r?.provider_requests === 0 && r?.model_turns === 0 }
    for (const key of keys) report[key] = Number.isSafeInteger(r?.[key]) && r[key] >= 0 ? r[key] : null
  } else throw new Error('Unknown proof scope')
  report.source_revision = p.source_revision
  report.control_revision = p.control_revision
  fs.mkdirSync(root, { recursive: true })
  fs.writeFileSync(path.join(root, name), JSON.stringify(report, null, 2) + '\n')
  fs.writeFileSync(path.join(root, 'provenance.json'), JSON.stringify(p, null, 2) + '\n')
  return report
}
module.exports = { record }
if (require.main === module) {
  try {
    if (process.argv.length !== 4) throw new Error('Usage: record-ci-proof.cjs SCOPE DIRECTORY')
    const r = record(process.argv[2], process.argv[3], process.env)
    console.log(JSON.stringify({ scope: r.scope, passed: r.passed }))
  } catch (error) { console.error(error.message); process.exitCode = 1 }
}
