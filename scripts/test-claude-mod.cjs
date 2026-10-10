// Real CLI startup proof. Every provider route is a rejecting local fixture.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const http = require('node:http')
const os = require('node:os')
const path = require('node:path')
const { spawn, spawnSync } = require('node:child_process')

async function main() {
  assert.ok(process.argv.length === 4 || process.argv.length === 5, 'usage: node test-claude-mod.cjs SENTINEL_TOOLS CLAUDE [REPORT]')
  const tools = path.resolve(process.argv[2])
  const claude = path.resolve(process.argv[3])
  const version = spawnSync(claude, ['--version'], { encoding: 'utf8', timeout: 10000 })
  assert.equal(version.status, 0)
  assert.match(version.stdout, /^2\.1\.291 \(Claude Code\)/)
  const root = fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()), 'sentinel-mod-'))
  const lab = path.join(root, 'lab')
  const requests = []
  const server = http.createServer((req, res) => {
    requests.push({ method: req.method, path: req.url })
    req.resume()
    res.setHeader('Content-Type', 'application/json')
    if (req.method === 'GET' && req.url === '/sentinel/control') {
      res.end(JSON.stringify({ mode: 'fixture-serving', policy: 'local-only', capture_enabled: false }))
    } else {
      res.statusCode = 400
      res.end(JSON.stringify({ type: 'error', error: { type: 'invalid_request_error', message: 'Provider requests forbidden in this fixture' } }))
    }
  })
  let child
  try {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
    const endpoint = `http://127.0.0.1:${server.address().port}`
    const env = {
      ...Object.fromEntries(['PATH', 'HOME', 'TERM', 'LANG', 'LC_ALL', 'TMPDIR', 'USER'].filter(key => process.env[key] !== undefined).map(key => [key, process.env[key]])),
      SENTINEL_LAB_ROOT: lab, SENTINEL_PROJECT_ROOT: path.dirname(tools),
      CLAUDE_CONFIG_DIR: path.join(root, 'claude'), DISABLE_TELEMETRY: '1',
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
      ANTHROPIC_BASE_URL: endpoint, ANTHROPIC_API_KEY: 'sentinel-local-fixture-key'
    }
    // Ambient provider settings/credentials are excluded by the allowlist.
    const prepared = spawnSync(tools, ['lab', 'prepare'], { env, encoding: 'utf8', timeout: 15000 })
    assert.equal(prepared.status, 0, 'isolated lab preparation failed')
    const plugin = path.join(lab, 'control-plugin')
    const configPath = path.join(plugin, 'sentinel-config.json')
    const config = JSON.parse(fs.readFileSync(configPath, 'utf8'))
    config.endpoint = endpoint
    fs.writeFileSync(configPath, JSON.stringify(config), { mode: 0o600 })
    for (const command of ['status', 'panel', 'failures']) {
      const result = await new Promise((resolve, reject) => {
        let stdout = ''
        let stderr = ''
        child = spawn(claude, ['-p', `/sentinel ${command}`, '--plugin-dir', plugin,
          '--setting-sources', 'user', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}', '--output-format', 'json'],
        { env, cwd: path.join(lab, 'workspace'), stdio: ['ignore', 'pipe', 'pipe'] })
        const timer = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('native command timed out')) }, 30000)
        child.stdout.on('data', chunk => { stdout += chunk; if (stdout.length > 1024 * 1024) child.kill('SIGKILL') })
        child.stderr.on('data', chunk => { stderr += chunk; if (stderr.length > 1024 * 1024) child.kill('SIGKILL') })
        child.on('error', error => { clearTimeout(timer); reject(error) })
        child.on('close', code => { clearTimeout(timer); resolve({ code, stdout, stderr }) })
      })
      assert.equal(result.code, 0, `native ${command} failed`)
      const output = JSON.parse(result.stdout)
      assert.equal(output.num_turns, 0, `${command} started a model turn`)
      assert.equal(output.total_cost_usd, 0)
      assert.equal(output.local_command, 'custom')
      assert.equal(output.is_error, false)
      if (command === 'status') assert.match(output.result, /fixture-serving/)
      if (command === 'panel') {
        assert.match(output.result, /sampled /)
        assert.match(output.result, /Gateway: unknown/)
      }
      if (command === 'failures') assert.match(output.result, /Quality evidence: unknown/)
    }
    assert.equal(requests.length, 3)
    assert.ok(requests.every(req => req.method === 'GET' && req.path === '/sentinel/control'), 'provider request attempted')
    const report = { client_version: '2.1.291', native_commands: 3, controller_gets: requests.length, provider_requests: 0, model_turns: 0 }
    if (process.argv[4]) fs.writeFileSync(process.argv[4], JSON.stringify(report) + '\n')
    console.log(JSON.stringify(report))
  } finally {
    if (child && child.exitCode === null) child.kill('SIGKILL')
    await new Promise(resolve => server.close(resolve))
    fs.rmSync(root, { recursive: true, force: true })
  }
}

main().catch(error => { console.error(error.message); process.exitCode = 1 })
