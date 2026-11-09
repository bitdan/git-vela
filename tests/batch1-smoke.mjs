// Batch-1 feature smoke (G5 .gitignore, E1 line endings, G14 message history,
// issue navigation, E2 diff-tab session persistence) against a running
// `wails3 dev -tags desktop_smoke` instance. Usage:
//   node tests/batch1-smoke.mjs <absolute clone path>
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const keepRepo = process.env.GITVELA_BATCH1_KEEP === '1'

async function until(action, description) {
  const deadline = Date.now() + 30000
  let last
  while (Date.now() < deadline) {
    const result = await action().catch(error => { last = error; return undefined })
    if (result) return result
    await delay(120)
  }
  throw new Error('Timed out waiting for ' + description + (last ? ': ' + last.message : ''))
}

/* ---- Temporary repository: one issue-link commit, one untracked file ---- */

const repo = await mkdtemp(join(tmpdir(), 'gitvela-batch1-'))
const git = (...args) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' }).toString()
git('init', '-b', 'main')
git('config', 'user.email', 'smoke@example.com')
git('config', 'user.name', 'Smoke')
git('config', 'core.autocrlf', 'false')
await (await import('node:fs/promises')).writeFile(join(repo, 'tracked.txt'), 'first\nsecond\n')
git('add', 'tracked.txt')
git('commit', '-m', 'Fix PROJ-1 issue')
await (await import('node:fs/promises')).writeFile(join(repo, 'untracked.txt'), 'temp\n')

const readRepo = async path => readFile(join(repo, path), 'utf8')

/* ---- CDP plumbing (same pattern as desktop-smoke.mjs) ---- */

const page = await until(async () => {
  try {
    const targets = await (await fetch(endpoint + '/json/list')).json()
    return targets.find(target => target.type === 'page' && target.webSocketDebuggerUrl)
  } catch {
    return undefined
  }
}, 'the GitVela WebView2 debugging endpoint')

const socket = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((resolve, reject) => {
  socket.addEventListener('open', resolve, { once: true })
  socket.addEventListener('error', reject, { once: true })
})
let sequence = 0
const pending = new Map()
socket.addEventListener('message', ({ data }) => {
  const reply = JSON.parse(data)
  const request = pending.get(reply.id)
  if (!request) return
  pending.delete(reply.id)
  clearTimeout(request.timer)
  if (reply.error) request.reject(new Error(reply.error.message))
  else request.resolve(reply.result)
})

function cdp(method, params) {
  return new Promise((resolve, reject) => {
    const id = ++sequence
    const timer = setTimeout(() => {
      pending.delete(id)
      reject(new Error('CDP command timed out: ' + method))
    }, 5000)
    pending.set(id, { resolve, reject, timer })
    socket.send(JSON.stringify({ id, method, params }))
  })
}

await cdp('Page.bringToFront', {})

async function evaluate(expression) {
  const result = await cdp('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text)
  return result.result.value
}

const clickButton = (scope, label) => until(() => evaluate(`(() => {
  const button = [...document.querySelectorAll(${JSON.stringify(scope + ' button')})]
    .find(item => item.innerText === ${JSON.stringify(label)})
  if (!button || button.disabled) return false
  button.click()
  return true
})()`), 'enabled button ' + label)

const contextMenuRow = (selector, label, menuLabel) => until(() => evaluate(`(() => {
  const row = ${selector}
  if (!row) return false
  row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  return true
})()`), 'context menu on ' + label).then(() => clickButton(`[aria-label=${JSON.stringify(menuLabel)}]`, label))

try {
  // Clean state, then configure the issue URL before the reload so the
  // preferences store picks it up on startup.
  await evaluate("localStorage.clear()")
  await evaluate("localStorage.setItem('gitvela.issueURL', 'https://example.com/issue/$1')")
  await cdp('Page.navigate', { url: 'http://wails.localhost:9245/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload')
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('git version')"), 'the Go Git version binding')
  await evaluate(`(() => {
    const input = document.querySelector('[aria-label="Repository path"]')
    input.value = ${JSON.stringify(repo)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form.requestSubmit()
  })()`)
  await until(() => evaluate("document.querySelector('.commit-row canvas')?.width > 0"), 'repository history')

  /* ---- G5: Add to .gitignore on an untracked file ---- */
  assert.ok(await evaluate("!!document.querySelector('.working-row[title=\"untracked.txt\"]')"), 'untracked file listed')
  await contextMenuRow("document.querySelector('.working-row[title=\"untracked.txt\"]')", 'Add to .gitignore', 'Working file actions')
  await until(() => evaluate("!document.querySelector('.working-row[title=\"untracked.txt\"]')"), 'untracked file dropped from the working list')
  assert.equal(await readRepo('.gitignore'), '/untracked.txt\n', '.gitignore gains the anchored pattern')
  await until(() => evaluate("document.querySelector('.commit-actions span')?.innerText.includes('Added untracked.txt to .gitignore')"), 'ignore notice')

  /* ---- E1: line ending display + conversion ----
   * The conversion stays CRLF so the later commit has a real staged change
   * (an index identical to HEAD would make git commit refuse). */
  await contextMenuRow("document.querySelector('.working-row[title=\"tracked.txt\"]')", 'Edit Working File', 'Working file actions')
  await until(() => evaluate("!!document.querySelector('.workspace-editor')"), 'editor opens')
  const widget = "document.querySelector('.workspace-editor .line-ending-widget')"
  assert.equal(await evaluate(widget + '?.innerText'), 'LF', 'editor widget shows LF')
  assert.ok(await evaluate("document.querySelector('.statusbar .editor-meta')?.innerText.includes('UTF-8 ·')"), 'status bar shows encoding')
  assert.equal(await evaluate("document.querySelector('.statusbar .editor-meta .line-ending-widget')?.innerText"), 'LF', 'status bar widget shows LF')
  await evaluate("document.querySelector('.statusbar .editor-meta .line-ending-widget').click()")
  await until(() => evaluate(widget + "?.innerText === 'CRLF'"), 'widget switches to CRLF')
  await until(() => evaluate("document.querySelector('.workspace-editor')?.innerText.includes('Saved')"), 'conversion saved')
  assert.ok((await readRepo('tracked.txt')).includes('\r\n'), 'stored file converted to CRLF')
  assert.ok(await evaluate("document.querySelector('.workspace-editor')?.innerText.includes('UTF-8')"), 'UTF-8 label kept')

  /* ---- G14: commit message history ---- */
  await contextMenuRow("document.querySelector('.working-row[title=\"tracked.txt\"]')", 'Stage', 'Working file actions')
  await until(() => evaluate("document.querySelector('.index-actions button[title=\"Stage the selected file\"]')?.disabled"), 'selected file staged')
  await evaluate(`(() => {
    const input = document.querySelector('#commit-message')
    input.value = 'Batch1 smoke PROJ-2 fix'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  await evaluate("document.querySelector('.commit-actions .primary').click()")
  await until(() => evaluate("[...document.querySelectorAll('.commit-row')].some(row => row.innerText.includes('PROJ-2'))"), 'PROJ-2 commit in history')
  await evaluate("document.querySelector('.commit-history button').click()")
  const item = await until(() => evaluate("[...document.querySelectorAll('.commit-history-item')].map(item => item.innerText)[0]"), 'history dropdown entries')
  assert.equal(item, 'Batch1 smoke PROJ-2 fix')
  await evaluate("document.querySelector('.commit-history-item').click()")
  assert.equal(await evaluate("document.querySelector('#commit-message').value"), 'Batch1 smoke PROJ-2 fix', 'picked history fills the message field')

  /* ---- Issue navigation in Commit Details ---- */
  await evaluate("[...document.querySelectorAll('.commit-row')].find(row => row.innerText.includes('PROJ-1')).click()")
  await until(() => evaluate("!!document.querySelector('.details-message a[href=\"https://example.com/issue/PROJ-1\"]')"), 'issue link in the full message')
  assert.ok(await evaluate("!!document.querySelector('.details-subject a[href=\"https://example.com/issue/PROJ-1\"]')"), 'issue link in the subject')

  /* ---- E2: pinned diff tab survives a reload ---- */
  await evaluate("[...document.querySelectorAll('.commit-row')].find(row => row.innerText.includes('PROJ-2')).click()")
  await until(() => evaluate("!!document.querySelector('.files-panel .file-row[title=\"tracked.txt\"]')"), 'changed file listed for PROJ-2')
  await evaluate("document.querySelector('.files-panel .file-row[title=\"tracked.txt\"]').dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }))")
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].some(tab => tab.title === 'tracked.txt' && tab.classList.contains('active'))"), 'pinned diff tab opens')
  await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.session') ?? '{}').diffTabs?.length === 1"), 'diff tab persisted to the session')
  await cdp('Page.navigate', { url: 'http://wails.localhost:9245/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload for restore')
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].some(tab => tab.title === 'tracked.txt' && tab.classList.contains('active'))"), 'pinned diff tab restored and active')
  await until(() => evaluate("!!document.querySelector('.diff-host, .patch, .cm-mergeView')"), 'restored diff tab content loaded lazily')

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])
  console.log('Batch 1 smoke passed: .gitignore, line endings, message history, issue links, diff tab restore.')
} finally {
  socket.close()
  if (!keepRepo) await rm(repo, { recursive: true, force: true })
  else console.log('Kept temp repository: ' + repo)
}
