import { expect, test } from 'claude-code/testing'

test('status executes the configured Go controller without starting a model turn', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', ($, e) => {
    expect(e.argv).toEqual(['/private/tmp/sentinel-tools', 'control', '--endpoint', 'http://127.0.0.1:19090', 'status'])
    runs += 1
    return { value: { exitCode: 0, stdout: '{"mode":"serving","policy":"local-only","capture_enabled":false,"counter":9007199254740993}', stderr: '' } }
  })
  const answer = await $.command.run({ command: 'sentinel', args: 'status' })
  expect(JSON.parse(answer.text)).toMatchObject({ mode: 'serving', capture_enabled: false })
  expect(answer.text).toContain('9007199254740993')
  expect(runs).toBe(1)
})

test('invalid arguments never execute a subprocess', async ($, on) => {
  let runs = 0
  on('process.run', () => { runs++; return { value: { exitCode: 0, stdout: '{}', stderr: '' } } })
  for (const args of ['policy remote', 'training on extra', 'profile opus -1', 'profile opus 32769', 'profile opus 9007199254740993', 'status; rm', 'capture yes']) {
    const answer = await $.command.run({ command: 'sentinel', args })
    expect(answer.text).toContain('Usage:')
  }
  expect(runs).toBe(0)
})

test('rejected mutations show rejection without retry or success claim', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', ($, e) => {
    expect(e.argv.slice(-2)).toEqual(['policy', 'quality'])
    runs++
    return { value: { exitCode: 1, stdout: '', stderr: 'PRIVATE RESPONSE' } }
  })
  const answer = await $.command.run({ command: 'sentinel', args: 'policy quality' })
  expect(answer.text).toContain('rejected or unavailable')
  expect(answer.text).not.toContain('PRIVATE')
  expect(runs).toBe(1)
})

test('capture is opt-in and records tool metadata without tool content', async ($, on) => {
  const records: any[] = []
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('session.id', () => ({ value: 'session-1' }))
  on('session.model', () => ({ value: 'sonnet' }))
  on('process.run', ($, e) => {
    expect(e.argv).toEqual(['/private/tmp/sentinel-tools', 'capture', '--client', 'claude', '--mod-event', '--output', '/private/tmp/lab/captures/claude-mod.jsonl'])
    records.push(JSON.parse(e.init.stdin))
    return { value: { exitCode: 0, stdout: '', stderr: '' } }
  })
  on('tool.call', () => ({ result: 'PRIVATE TOOL RESULT' }))
  const event = { tool: 'Read', file_path: '/PRIVATE/FILE', tool_use_id: 'tool-1' }
  await $.tool.call(event)
  expect(records.length).toBe(0)
  await $.command.run({ command: 'sentinel', args: 'capture on' })
  const result = await $.tool.call(event)
  expect(result).toEqual({ result: 'PRIVATE TOOL RESULT' })
  expect(records.length).toBe(1)
  expect(records[0]).toMatchObject({ mod_event: 'tool.call', session_id: 'session-1', tool_name: 'Read', tool_use_id: 'tool-1', stage: 'returned' })
  expect(JSON.stringify(records)).not.toContain('PRIVATE')
  await $.command.run({ command: 'sentinel', args: 'capture off' })
  await $.tool.call(event)
  expect(records.length).toBe(1)
})

test('capture failure does not replace a prompt or final answer', async ($, on) => {
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('session.id', () => ({ value: 'session-1' }))
  on('session.model', () => ({ value: 'sonnet' }))
  on('process.run', () => ({ deny: 'unavailable' }))
  on('prompt.submit', ($, e) => ({ text: e.text }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  await $.command.run({ command: 'sentinel', args: 'capture on' })
  expect(await $.prompt.submit({ text: 'original prompt' })).toEqual({ text: 'original prompt' })
  expect(await $.turn.complete({ turnId: 'turn-1', answer: 'original answer', durationMs: 20, isAborted: false, usage: null })).toEqual({ text: 'original answer' })
})

const PANE = {
  plugin: 'sentinel', component: 'Pane', requestId: 'sentinel-panel',
  viewport: { columns: 100, rows: 30 },
  props: { title: 'Sentinel', isFocused: true, bodyColumns: 60, placement: 'inline', scroll: { offset: 0, bodyRows: 24 }, view: {} }
} as const

test('terminal and desktop panes show unknowns, refresh, and retained failures', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', ($, e) => {
    expect(e.argv).toEqual(['/private/tmp/sentinel-tools', 'mod', '--root', '/private/tmp/lab', '--endpoint', 'http://127.0.0.1:19090', 'status'])
    runs++
    return { value: { exitCode: 0, stdout: JSON.stringify({ scope: 'claude-mod-status', sampled_at: '2026-10-09T10:00:00Z', control: { mode: 'serving', policy: 'local-only', capture_enabled: false }, telemetry: null, cli_quality: { passed: 1, total: 4, suite: 'subset', finished_at: '2026-10-08T09:00:00Z', failed_cases: [{ client: 'claude', role: 'haiku', task: 'tool-recovery', failed_checks: ['failed_read_before_success'] }] } }), stderr: '' } }
  })
  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface })
    if (runs === 0) expect(await ui.find({ type: 'Text', text: /sampled unknown/ })).toBeDefined()
    await ui.press({ key: 'refresh' })
    expect(await ui.find({ type: 'Text', text: /sampled 2026-10-09/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /Gateway: unknown/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /Quality: 1\/4 retained cases; suite subset/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /failed_read_before_success/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /promotion are not authorized/ })).toBeDefined()
    await ui.unmount()
  }
  expect(runs).toBe(2)
})

test('a failed refresh clears old data and pane capture toggles locally', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', () => {
    runs++
    return { value: { exitCode: runs === 1 ? 0 : 1, stdout: '{"scope":"claude-mod-status","sampled_at":"previous"}', stderr: '' } }
  })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'refresh' })
  expect(await ui.find({ type: 'Text', text: /sampled previous/ })).toBeDefined()
  await ui.press({ key: 'refresh' })
  expect(await ui.find({ type: 'Text', text: /sampled unknown/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /Snapshot unavailable/ })).toBeDefined()
  await ui.press({ key: 'capture' })
  expect(await ui.find({ type: 'Text', text: /Local mod capture: ON/ })).toBeDefined()
  await ui.press({ key: 'capture' })
  expect(await ui.find({ type: 'Text', text: /Local mod capture: OFF/ })).toBeDefined()
  expect(runs).toBe(2)
})

test('invalid literal configuration fails without a process', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.1:19090' }) }))
  on('process.run', () => { runs++; return { value: { exitCode: 0, stdout: 'null', stderr: '' } } })
  const answer = await $.command.run({ command: 'sentinel', args: 'status' })
  expect(answer.text).toContain('rejected or unavailable')
  expect(runs).toBe(0)
})

test('malformed controller output never claims a successful mutation', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', () => {
    const values = ['null', '[]', '{} trailing', 'not-json']
    return { value: { exitCode: 0, stdout: values[runs++], stderr: '' } }
  })
  for (let n = 0; n < 4; n++) expect((await $.command.run({ command: 'sentinel', args: 'training on' })).text).toContain('rejected or unavailable')
  expect(runs).toBe(4)
})

test('native command registers in a real session event and keeps other panes intact', async ($, on) => {
  let registered = ''
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('command.register', ($, e) => { registered = e.name; return { value: undefined } })
  on('command.list', () => ({ value: [{ name: 'sentinel' }] }))
  on('ui.render', () => ({ type: 'Text', props: {}, children: ['core pane'] }))
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  expect(registered).toBe('sentinel')
  const ui = await $.ui.mount({ ...PANE, requestId: 'other-pane', surface: 'terminal' })
  expect(await ui.find({ type: 'Text', text: 'core pane' })).toBeDefined()
})

test('startup waits for the host command list before the first prompt', async ($, on) => {
  let polls = 0
  let sleeps = 0
  on('command.register', () => ({ value: undefined }))
  on('command.list', () => ({ value: ++polls < 3 ? [] : [{ name: 'sentinel' }] }))
  on('clock.sleep', ($, e) => { expect(e.ms).toBe(25); sleeps++; return { value: undefined } })
  on('session.start', () => ({ cwd: '/work' }))
  await $.session.start({ surface: 'none', isInteractive: false, cwd: '/work' })
  expect(polls).toBe(3)
  expect(sleeps).toBe(2)
})

test('unavailable native commands cannot fall through to a model prompt', async ($, on) => {
  let polls = 0
  let prompts = 0
  on('command.register', () => ({ value: undefined }))
  on('command.list', () => { polls++; return { value: [] } })
  on('clock.sleep', () => ({ value: undefined }))
  on('session.start', () => ({ cwd: '/work' }))
  on('prompt.submit', ($, e) => { prompts++; return { text: e.text } })
  await $.session.start({ surface: 'none', isInteractive: false, cwd: '/work' })
  expect(polls).toBe(20)
  const result = await $.prompt.submit({ text: '/sentinel status' })
  expect(result).toMatchObject({ drop: 'Sentinel native command unavailable. Reload the plugin or use sentinel-tools control directly.' })
  expect(prompts).toBe(0)
  await $.prompt.submit({ text: 'ordinary user prompt' })
  expect(prompts).toBe(1)
})

test('headless panel uses text when the host cannot place a pane', async ($, on) => {
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', () => ({ value: { exitCode: 0, stdout: '{"scope":"claude-mod-status","sampled_at":"observed"}', stderr: '' } }))
  on('ui.open', () => ({ value: { isPlaced: false } }))
  const answer = await $.command.run({ command: 'sentinel', args: 'panel' })
  expect(answer.text).toContain('sampled observed')
  expect(answer.text).not.toContain('panel opened')
})

test('command refresh invalidates an open pane and clears old observations', async ($, on) => {
  let runs = 0
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('process.run', () => {
    runs++
    return { value: { exitCode: runs === 1 ? 0 : 1, stdout: '{"scope":"claude-mod-status","sampled_at":"previous"}', stderr: '' } }
  })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'refresh' })
  expect(await ui.find({ type: 'Text', text: /sampled previous/ })).toBeDefined()
  await $.command.run({ command: 'sentinel', args: 'failures' })
  expect(await ui.find({ type: 'Text', text: /sampled unknown/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /Snapshot unavailable/ })).toBeDefined()
})

test('turn usage stays reported, missing usage stays unknown, and clear disables capture', async ($, on) => {
  const records: any[] = []
  on('fs.read', () => ({ value: JSON.stringify({ executable: '/private/tmp/sentinel-tools', root: '/private/tmp/lab', endpoint: 'http://127.0.0.1:19090' }) }))
  on('session.id', () => ({ value: 'session-1' }))
  on('session.model', () => ({ value: 'sonnet' }))
  on('process.run', ($, e) => { records.push(JSON.parse(e.init.stdin)); return { value: { exitCode: 0, stdout: '', stderr: '' } } })
  on('turn.complete', () => ({ text: '' }))
  on('classic.SessionStart', () => ({}))
  await $.command.run({ command: 'sentinel', args: 'capture on' })
  await $.turn.complete({ turnId: 'turn-1', answer: 'answer', durationMs: 20, isAborted: false, usage: { model: 'local-artifact', input_tokens: 10, output_tokens: 5, cache_read_input_tokens: 4, cache_creation_input_tokens: 0 } })
  await $.turn.complete({ turnId: 'turn-2', answer: '', durationMs: 0, isAborted: true, usage: null })
  expect(records[0].usage).toEqual({ input_tokens: 10, output_tokens: 5, cache_read_input_tokens: 4, cache_creation_input_tokens: 0 })
  expect(records[0].model).toBe('local-artifact')
  expect(records[1].usage).toBeUndefined()
  await $.classic.SessionStart({ source: 'clear' })
  await $.turn.complete({ turnId: 'turn-3', answer: 'answer', durationMs: 1, isAborted: false, usage: null })
  expect(records.length).toBe(2)
})
