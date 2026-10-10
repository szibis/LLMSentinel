const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { test } = require('node:test')
let validatePublisher, lint
try { ({ validatePublisher, lint } = require('./lint-workflows.cjs')) } catch (error) { if (error.code !== 'MODULE_NOT_FOUND') throw error }

const block = 'concurrency:\n  group: sentinel-ci-proof-publisher\n  cancel-in-progress: false\n  queue: max\n'
const publisher = `name: Proofs\non: workflow_run\n${block}\njobs:\n  publish:\n    runs-on: ubuntu-latest\n    steps: []\n`
function fixture(t, content = publisher) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'workflow-lint-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  fs.writeFileSync(path.join(root, 'ci-proof-comments.yml'), content)
  fs.writeFileSync(path.join(root, 'build.yml'), 'name: Build\n')
  fs.writeFileSync(path.join(root, 'security.yaml'), 'name: Security\n')
  let output = ''
  const sink = { write: value => { output += value } }
  return { root, sink, output: () => output }
}

test('validates the actual literal bounded queue policy before linting', () => {
  assert.equal(typeof validatePublisher, 'function', 'literal publisher validator is missing')
  assert.doesNotThrow(() => validatePublisher(publisher))
})

test('rejects policy removal, cancellation, duplicate or misplaced queue keys and commented blocks', () => {
  for (const text of [
    publisher.replace('  queue: max\n', ''), publisher.replace('queue: max', 'queue: single'),
    publisher.replace('cancel-in-progress: false', 'cancel-in-progress: true'),
    publisher.replace('sentinel-ci-proof-publisher', 'per-run-${{ github.run_id }}'),
    publisher + '\nother:\n  queue: max\n', publisher + '\nqueue: max\n',
    publisher + '\nother:\n  "queue": max\n', publisher + '\n"concurrency": {}\n',
    publisher.replace('    steps: []', '    concurrency: { group: other, queue: invalid }\n    steps: []'),
    publisher.replace('    steps: []', '    concurrency: { group: other, "queue": invalid }\n    steps: []'),
    publisher.replace('    steps: []', "    concurrency: { group: other, 'queue': invalid }\n    steps: []"),
    publisher.replace(block, block.split('\n').map(line => `# ${line}`).join('\n')),
    publisher.replace('  queue: max\n', '').replace('    steps: []', '    queue: max\n    steps: []'),
    publisher.replace(block, block.replace(/^/gm, '  ')), publisher + '\nconcurrency:\n  group: other\n'
  ]) assert.throws(() => validatePublisher(text), /publisher concurrency policy/i)
})

test('runs ordinary workflows without ignores and scopes the exact diagnostic ignore to the publisher', t => {
  const f = fixture(t), calls = []
  const command = ['go', 'run', 'github.com/rhysd/actionlint/cmd/actionlint@v1.7.12']
  const code = lint({ workflowDir: f.root, command, stdout: f.sink, stderr: f.sink, run: (executable, args, options) => {
    calls.push({ executable, args, options }); return { status: 0, stdout: '', stderr: '' }
  } })
  assert.equal(code, 0)
  assert.equal(calls.length, 2)
  assert.equal(calls[0].executable, 'go')
  assert.deepEqual(calls[0].args.slice(0, 2), command.slice(1))
  assert.ok(calls[0].args.includes(path.join(f.root, 'build.yml')))
  assert.ok(calls[0].args.includes(path.join(f.root, 'security.yaml')))
  assert.equal(calls[0].args.some(arg => arg.startsWith('-ignore')), false)
  const flag = calls[1].args.find(arg => arg.startsWith('-ignore='))
  assert.equal(flag, '-ignore=^unexpected key "queue" for "concurrency" section\\. expected one of "cancel-in-progress", "group"$')
  assert.deepEqual(calls[1].args.filter(arg => /\.(yml|yaml)$/.test(arg)), [path.join(f.root, 'ci-proof-comments.yml')])
  assert.equal(calls[1].options.shell, false)
  assert.ok(calls[1].options.timeout > 0 && calls[1].options.timeout <= 300000)
})

test('an invalid policy never invokes a linter despite the unsupported queue diagnostic', t => {
  const f = fixture(t, publisher.replace('queue: max', 'queue: bad'))
  let calls = 0
  assert.equal(lint({ workflowDir: f.root, command: ['actionlint'], stdout: f.sink, stderr: f.sink, run: () => { calls++; return { status: 0 } } }), 1)
  assert.equal(calls, 0)
  assert.match(f.output(), /publisher concurrency policy/i)
})

test('ordinary workflow errors and different publisher errors remain failures with original diagnostics', t => {
  for (const failureAt of [0, 1]) {
    const f = fixture(t), error = failureAt === 0 ? 'build.yml: unsupported queue value' : 'ci-proof-comments.yml: unknown action'
    let calls = 0
    const code = lint({ workflowDir: f.root, command: ['actionlint'], stdout: f.sink, stderr: f.sink, run: () => ({ status: calls++ === failureAt ? 1 : 0, stdout: error, stderr: '' }) })
    assert.equal(code, 1)
    assert.equal(calls, 2)
    assert.ok(f.output().includes(error))
  }
})

test('execution failures and timeouts cannot be mistaken for successful linting', t => {
  for (const result of [{ status: null, error: new Error('tool unavailable') }, { status: null, signal: 'SIGTERM' }, { status: 2, stderr: 'invalid arguments' }]) {
    const f = fixture(t)
    assert.equal(lint({ workflowDir: f.root, command: ['actionlint'], stdout: f.sink, stderr: f.sink, run: () => result }), 1)
  }
})

test('caller arguments cannot broaden the scoped lint exemption', t => {
  const f = fixture(t)
  let calls = 0
  const code = lint({ workflowDir: f.root, command: ['actionlint', '-ignore=.*'], stdout: f.sink, stderr: f.sink, run: () => { calls++; return { status: 0 } } })
  assert.equal(code, 1)
  assert.equal(calls, 0)
  assert.match(f.output(), /ignores are prohibited/)
})
