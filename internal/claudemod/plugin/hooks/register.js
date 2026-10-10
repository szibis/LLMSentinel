// Host adapter only: gateway control, telemetry and private storage stay in Go.
const PANE = 'sentinel-panel'
const USAGE = 'Usage: /sentinel [panel|status|failures|training on|off|policy local-only|balanced|quality|profile haiku|sonnet|opus N|capture on|off]'
let config
let snapshot = null
let busy = false
let capture = false
let notice = ''
let surface = 'none'

function absolutePath(value) {
  return typeof value === 'string' && value.startsWith('/') && value !== '/' &&
    !/[\u0000-\u001f\u007f]/.test(value) && !value.includes('//') && !value.endsWith('/') &&
    !value.split('/').some(part => part === '.' || part === '..')
}

async function configuration($) {
  if (!config) {
    const value = JSON.parse(await $.fs.read($.plugin.root + '/sentinel-config.json'))
    const endpoint = new URL(value.endpoint)
    if (!absolutePath(value.executable) || !absolutePath(value.root) ||
        !/^http:\/\/(127\.0\.0\.1|\[::1\])(?::[1-9][0-9]{0,4})?$/.test(value.endpoint) ||
        endpoint.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(endpoint.hostname) ||
        endpoint.username || endpoint.password || endpoint.search || endpoint.hash || endpoint.pathname !== '/') {
      throw new Error('Invalid Sentinel configuration')
    }
    config = value
  }
  return config
}

async function controller($, args) {
  const c = await configuration($)
  const result = await $.process.run([c.executable, 'control', '--endpoint', c.endpoint, ...args], { timeoutMs: 2500 })
  if (result.exitCode !== 0) throw new Error('Controller unavailable')
  const value = JSON.parse(result.stdout)
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid status')
  return result.stdout.trim()
}

async function refresh($) {
  if (busy) return
  busy = true
  try {
    const c = await configuration($)
    const result = await $.process.run([c.executable, 'mod', '--root', c.root, '--endpoint', c.endpoint, 'status'], { timeoutMs: 4000 })
    if (result.exitCode !== 0) throw new Error('Snapshot unavailable')
    const value = JSON.parse(result.stdout)
    if (value?.scope !== 'claude-mod-status') throw new Error('Invalid snapshot')
    snapshot = value
    notice = ''
  } catch {
    snapshot = null
    notice = 'Snapshot unavailable; refresh to try again.'
  } finally {
    busy = false
    await $.ui.invalidate('ui.render')
  }
}

function text(value) {
  return value === null || value === undefined ? 'unknown' : String(value).replace(/[\u0000-\u001f\u007f]/g, ' ').slice(0, 200)
}

function qualityLines(q) {
  if (!q) return ['Quality evidence: unknown']
  return [
    `Quality: ${text(q.passed)}/${text(q.total)} retained cases; suite ${text(q.suite)}`,
    `Evidence finished: ${text(q.finished_at)}`,
    ...(Array.isArray(q.failed_cases) ? q.failed_cases.slice(0, 48).map(item => `${text(item.client)}/${text(item.role)} ${text(item.task)}: ${text(item.failed_checks?.join(', ') || 'failed; checks unavailable')}`) : []),
    'Read-only evidence; training and promotion are not authorized.'
  ]
}

function panelLines() {
  const c = snapshot?.control
  const t = snapshot?.telemetry
  const lines = [
    `Sentinel — sampled ${text(snapshot?.sampled_at)}`,
    `Gateway: ${text(t?.gateway)}; telemetry stale: ${text(t?.gateway_stale ?? (t?.gateway_sample_time ? false : null))}; observed at: ${text(t?.gateway_sample_time)}`,
    `Mode: ${text(c?.mode)}; policy: ${text(c?.policy)}; collector capture: ${text(c?.capture_enabled)}`,
    `Role budgets: ${text(c?.role_budgets ? JSON.stringify(c.role_budgets) : null)}`,
    `Local mod capture: ${capture ? 'ON (prompt and answer text included)' : 'OFF'} — this session only`,
    notice
  ].filter(Boolean)
  for (const [role, runtime] of Object.entries(t?.runtimes || {}).slice(0, 6)) {
    lines.push(`${text(role)}: ${text(runtime?.model)}; loaded ${text(runtime?.model_loaded)}; stale ${text(runtime?.stale ?? (runtime?.sample_time ? false : null))}; sampled ${text(runtime?.sample_time)}`)
    lines.push(`  Backend: ${text(runtime?.endpoint)}`)
    lines.push(`  Prompt cache: ${text(runtime?.prompt_cache ? JSON.stringify(runtime.prompt_cache) : null)}`)
  }
  return [...lines, ...qualityLines(snapshot?.cli_quality)]
}

async function record($, event) {
  if (!capture) return
  try {
    const c = await configuration($)
    const payload = { ...event, session_id: await $.session.id() }
    const model = await $.session.model()
    if (!payload.model && typeof model === 'string' && model) payload.model = model
    const result = await $.process.run([c.executable, 'capture', '--client', 'claude', '--mod-event', '--output', c.root + '/captures/claude-mod.jsonl'], { timeoutMs: 2500, stdin: JSON.stringify(payload) })
    if (result.exitCode !== 0) throw new Error('Capture unavailable')
  } catch {
    notice = 'Local mod capture failed; the user turn continues.'
    try { await $.ui.invalidate('ui.render') } catch { /* recording remains best effort */ }
  }
}

export function register(on) {
  on('session.start', async ($, e, next) => {
    surface = e.surface
    capture = false
    snapshot = null
    notice = ''
    await $.command.register({ name: 'sentinel', description: 'Native Sentinel panel, local controls and opt-in capture' })
    // Registration resolves before the headless host refreshes its command
    // list in 2.1.291. Observe readiness before the first prompt is parsed.
    for (let attempt = 0; attempt < 20; attempt++) {
      const commands = await $.command.list()
      if (commands.some(command => command.name === 'sentinel')) break
      await $.clock.sleep(25)
    }
    return next(e)
  })
  on('classic.SessionStart', async ($, e, next) => {
    capture = false
    snapshot = null
    notice = ''
    return next(e)
  })
  on('command.run', { command: 'sentinel' }, async ($, e) => {
    const args = e.args.trim().split(/\s+/).filter(Boolean)
    if (!args.length || (args.length === 1 && args[0] === 'panel')) {
      await refresh($)
      if (!['terminal', 'desktop'].includes(surface)) return { text: panelLines().join('\n') }
      try {
        const opened = await $.ui.open({ id: PANE, title: 'Sentinel', focus: true, closeOnEscape: true })
        return { text: opened.isPlaced ? 'Sentinel panel opened.' : panelLines().join('\n') }
      } catch { return { text: panelLines().join('\n') } }
    }
    if (args.length === 2 && args[0] === 'capture' && ['on', 'off'].includes(args[1])) {
      capture = args[1] === 'on'
      await $.ui.invalidate('ui.render')
      return { text: `Local mod capture ${capture ? 'ON: bounded, redacted prompt and answer text plus tool metadata' : 'OFF'}. This session only; collector training capture is separate.` }
    }
    if (args.length === 1 && args[0] === 'failures') {
      await refresh($)
      return { text: qualityLines(snapshot?.cli_quality).join('\n') }
    }
    const valid = (args.length === 1 && args[0] === 'status') ||
      (args.length === 2 && args[0] === 'training' && ['on', 'off'].includes(args[1])) ||
      (args.length === 2 && args[0] === 'policy' && ['local-only', 'balanced', 'quality'].includes(args[1])) ||
      (args.length === 3 && args[0] === 'profile' && ['haiku', 'sonnet', 'opus'].includes(args[1]) && /^[1-9][0-9]*$/.test(args[2]) && Number(args[2]) <= 32768)
    if (!valid) return { text: USAGE }
    try {
      const value = await controller($, args)
      snapshot = null
      await $.ui.invalidate('ui.render')
      return { text: value }
    } catch { return { text: 'Sentinel action rejected or unavailable. Refresh status to inspect state; the action was not retried.' } }
  })
  on('ui.render', { component: 'Pane' }, async ($, e, next) => {
    if (e.requestId !== PANE) return next(e)
    const { Box, Text, Button } = $.ui.resolve(e)
    return Box({ flexDirection: 'column', children: [
      ...panelLines().map(line => Text({ children: [line] })),
      Button({ key: 'refresh', label: 'Refresh', hotkey: 'r', onPress: async () => { await refresh($) } }),
      Button({ key: 'capture', label: capture ? 'Stop local capture' : 'Start local capture (includes text)', hotkey: 'c', onPress: async () => { capture = !capture; await $.ui.invalidate('ui.render') } })
    ] })
  })
  // An unrecognized slash command must never become a paid/inference prompt
  // if the host has not refreshed its command cache or registration failed.
  on('prompt.submit', { text: /^\s*\/sentinel(?:\s|$)/ }, () => ({
    drop: 'Sentinel native command unavailable. Reload the plugin or use sentinel-tools control directly.'
  })).catch(() => ({ drop: 'Sentinel native command unavailable. Reload the plugin or use sentinel-tools control directly.' }))
  on('prompt.submit', async ($, e, next) => {
    await record($, { mod_event: 'prompt.submit', prompt: e.text })
    return next(e)
  })
  on('tool.call', async ($, e, next) => {
    const event = { mod_event: 'tool.call', tool_name: e.tool }
    if (e.tool_use_id) event.tool_use_id = e.tool_use_id
    if (e.agentId) event.agent_id = e.agentId
    try {
      const result = await next(e)
      await record($, { ...event, stage: 'returned' })
      return result
    } catch (error) {
      await record($, { ...event, stage: 'error' })
      throw error
    }
  })
  on('turn.complete', async ($, e, next) => {
    const event = { mod_event: 'turn.complete', answer: e.answer, turn_id: e.turnId, duration_ms: e.durationMs, is_aborted: e.isAborted }
    if (e.agentId) event.agent_id = e.agentId
    if (e.usage) {
      if (typeof e.usage.model === 'string' && e.usage.model) event.model = e.usage.model
      event.usage = {}
      for (const key of ['input_tokens', 'output_tokens', 'cache_read_input_tokens', 'cache_creation_input_tokens']) {
        if (Number.isSafeInteger(e.usage[key]) && e.usage[key] >= 0) event.usage[key] = e.usage[key]
      }
    }
    await record($, event)
    return next(e)
  })
}
