// actionlint 1.7.12 predates GitHub's bounded queue setting. Validate that exact
// policy ourselves and exempt only its known diagnostic in this one workflow.
const fs = require('node:fs')
const path = require('node:path')
const { spawnSync } = require('node:child_process')

const PUBLISHER = 'ci-proof-comments.yml'
const POLICY = 'concurrency:\n  group: sentinel-ci-proof-publisher\n  cancel-in-progress: false\n  queue: max'
const IGNORE = '^unexpected key "queue" for "concurrency" section\\. expected one of "cancel-in-progress", "group"$'

function validatePublisher(source) {
  if (typeof source !== 'string') throw new Error('Invalid publisher concurrency policy')
  const text = source.replace(/\r\n/g, '\n')
  const blocks = text.match(/^concurrency:\n  group: sentinel-ci-proof-publisher\n  cancel-in-progress: false\n  queue: max(?=\n|$)/gm) || []
  // Include flow mappings and job-scoped keys: the publisher exemption must
  // never hide a second queue policy. This intentionally accepts only the
  // single literal policy and rejects extra key-shaped mentions in comments.
  const concurrency = text.match(/(?:^|[\s{,])(?:concurrency|"concurrency"|'concurrency')\s*:/gm) || []
  const queue = text.match(/(?:^|[\s{,])(?:queue|"queue"|'queue')\s*:/gm) || []
  if (blocks.length !== 1 || blocks[0] !== POLICY || concurrency.length !== 1 || queue.length !== 1) {
    throw new Error('Invalid publisher concurrency policy: require the single literal global group, cancel-in-progress: false and queue: max block')
  }
}

function lint({ workflowDir = '.github/workflows', command, run = spawnSync, stdout = process.stdout, stderr = process.stderr }) {
  try {
    if (!Array.isArray(command) || command.length === 0 || command.some(arg => typeof arg !== 'string' || !arg || arg.includes('\0'))) throw new Error('Provide the actionlint executable and arguments')
    // Callers cannot broaden the exemption or suppress ordinary workflows.
    if (command.some(arg => /^-{1,2}ignore(?:=|$)/.test(arg))) throw new Error('Caller-provided actionlint ignores are prohibited')
    const publisher = path.join(workflowDir, PUBLISHER)
    validatePublisher(fs.readFileSync(publisher, 'utf8'))
    const other = fs.readdirSync(workflowDir).filter(name => /\.ya?ml$/.test(name) && name !== PUBLISHER).sort().map(name => path.join(workflowDir, name))
    const groups = other.length ? [other, [`-ignore=${IGNORE}`, publisher]] : [[`-ignore=${IGNORE}`, publisher]]
    let failed = false
    for (const files of groups) {
      const result = run(command[0], [...command.slice(1), '-shellcheck=', '-pyflakes=', ...files], {
        encoding: 'utf8', shell: false, timeout: 300000, maxBuffer: 8 * 1024 * 1024
      })
      if (result.stdout) stdout.write(result.stdout)
      if (result.stderr) stderr.write(result.stderr)
      if (result.error) stderr.write(`Workflow linter could not complete: ${result.error.message}\n`)
      if (result.status !== 0 || result.error || result.signal) {
        failed = true
        if (result.status == null && !result.error) stderr.write('Workflow linter terminated before completion\n')
      }
    }
    return failed ? 1 : 0
  } catch (error) {
    stderr.write(`${error.message}\n`)
    return 1
  }
}

module.exports = { validatePublisher, lint }
if (require.main === module) process.exitCode = lint({ command: process.argv.slice(2) })
