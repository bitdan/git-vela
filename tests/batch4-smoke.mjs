// Batch-4 feature smoke (G7 multi-commit cherry-pick, G10 fetch options)
// against a running `wails3 dev -tags desktop_smoke` instance. Usage:
//   node tests/batch4-smoke.mjs
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, mkdir, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const keepRepo = process.env.GITVELA_BATCH4_KEEP === '1'

async function until(action, description) {
  const deadline = Date.now() + 30000
  let last
  while (Date.now() < deadline) {
    const result = await Promise.resolve().then(action).catch(error => { last = error; return undefined })
    if (result) return result
    await delay(120)
  }
  throw new Error('Timed out waiting for ' + description + (last ? ': ' + last.message : ''))
}

/* ---- Fixture: origin with a side branch, cloned and opened in the app ---- */

const base = await mkdtemp(join(tmpdir(), 'gitvela-batch4-'))
const origin = join(base, 'origin')
const clone = join(base, 'clone')
await mkdir(origin)
const git = (repo, ...args) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' }).toString()

git(origin, 'init', '-b', 'main')
git(origin, 'config', 'user.email', 'smoke@example.com')
git(origin, 'config', 'user.name', 'Smoke')
execFileSync('node', ['-e', `require('fs').writeFileSync(${JSON.stringify(join(origin, 'base.txt'))}, 'base\\n')`], { stdio: 'pipe' })
git(origin, 'add', '-A')
git(origin, 'commit', '-m', 'base commit')
git(origin, 'switch', '-c', 'side')
for (const name of ['side one', 'side two']) {
  execFileSync('node', ['-e', `require('fs').writeFileSync(${JSON.stringify(join(origin, name.replace(' ', '-') + '.txt'))}, ${JSON.stringify(name + '\n')})`], { stdio: 'pipe' })
  git(origin, 'add', '-A')
  git(origin, 'commit', '-m', name)
}
git(origin, 'switch', 'main')
execFileSync('git', ['-C', base, 'clone', '--quiet', origin, 'clone'], { stdio: 'pipe' })
git(clone, 'config', 'user.email', 'smoke@example.com')
git(clone, 'config', 'user.name', 'Smoke')

/* ---- CDP plumbing (same pattern as batch3-smoke.mjs) ---- */

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

const commitRow = subject => `[...document.querySelectorAll('.commit-row')]
  .find(row => row.innerText.includes(${JSON.stringify(subject)}))`

// Rebuild the two-row selection when a background refresh collapses it, and
// only open the menu once both rows are still selected (a row click closes it).
const selectPair = (first, second) => until(() => evaluate(`(() => {
  const active = () => [...document.querySelectorAll('.commit-row.active')]
  if (active().length !== 2) {
    const rows = [...document.querySelectorAll('.commit-row')]
    const one = rows.find(row => row.innerText.includes(${JSON.stringify(first)}))
    const two = rows.find(row => row.innerText.includes(${JSON.stringify(second)}))
    if (!one || !two) return false
    one.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    two.dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }))
    return false
  }
  if (!document.querySelector('[aria-label="Commit actions"]')) {
    active()[1].dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  }
  return true
})()`), 'two selected commits with the action menu')

const gitMenuAction = label => until(() => evaluate(`(() => {
  const menu = document.querySelector('.git-menu')
  if (!menu.open) menu.querySelector('summary').click()
  const button = [...menu.querySelectorAll('button')].find(item => item.innerText === ${JSON.stringify(label)})
  if (!button || button.disabled) return false
  button.click()
  return true
})()`), 'Git menu action ' + label)

/* ---- G10 helper: set the Fetch options and wait for the run to finish ---- */

async function fetchWith({ prune, tags }) {
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Remote synchronization"]')
    if (!panel) return false
    const checkbox = panel.querySelector('fieldset input[type=checkbox]')
    const select = panel.querySelector('fieldset select')
    if (!checkbox || !select || checkbox.disabled || select.disabled) return false
    if (checkbox.checked !== ${prune}) {
      checkbox.checked = ${prune}
      checkbox.dispatchEvent(new Event('change', { bubbles: true }))
    }
    if (select.value !== ${JSON.stringify(tags)}) {
      select.value = ${JSON.stringify(tags)}
      select.dispatchEvent(new Event('change', { bubbles: true }))
    }
    return true
  })()`), 'Fetch options')
  await clickButton('[aria-label="Remote synchronization"]', 'Fetch')
  await delay(150)
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Remote synchronization"]')
    const button = [...panel.querySelectorAll('button')].find(item => item.innerText === 'Fetch')
    return !!button && !button.disabled && panel.innerText.includes('Fetched origin.')
  })()`), 'the fetch to finish')
}

const remoteRefs = () => git(clone, 'for-each-ref', '--format=%(refname)', 'refs/remotes/origin')
const localTags = () => git(clone, 'tag', '-l').trim()

try {
  await evaluate('localStorage.clear()')
  await cdp('Page.navigate', { url: 'http://wails.localhost:9245/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload')
  await evaluate(`(() => {
    const input = document.querySelector('[aria-label="Repository path"]')
    input.value = ${JSON.stringify(clone)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form.requestSubmit()
  })()`)
  await until(() => evaluate('!!' + commitRow('side two')), 'repository history with the side branch')

  /* ---- G7: cherry-pick two selected commits in history order ---- */
  await selectPair('side one', 'side two')
  const menu = await evaluate(`[...document.querySelectorAll('[aria-label="Commit actions"] button')]
    .map(button => ({ label: button.innerText, disabled: button.disabled }))`)
  const revert = menu.find(item => item.label === 'Revert…')
  const reset = menu.find(item => item.label === 'Reset to here…')
  assert.equal(revert?.disabled, true, 'revert stays disabled on a multi-selection')
  assert.equal(reset?.disabled, true, 'reset stays disabled on a multi-selection')
  const pick = menu.find(item => item.label.startsWith('Cherry-pick 2 selected'))
  assert.ok(pick && !pick.disabled, 'cherry-pick accepts the two selected commits')
  await until(() => evaluate(`(() => {
    const button = [...document.querySelectorAll('[aria-label="Commit actions"] button')]
      .find(item => item.innerText.startsWith('Cherry-pick 2 selected'))
    if (!button || button.disabled) return false
    button.click()
    return true
  })()`), 'the multi-commit cherry-pick dialog')

  const listed = await until(() => evaluate(`(() => {
    const rows = [...document.querySelectorAll('.cherry-targets li')]
    return rows.length === 2 ? rows.map(row => row.innerText) : false
  })()`), 'both commits listed in the confirmation')
  assert.ok(listed[0].includes('side one') && listed[1].includes('side two'),
    'the dialog lists the commits oldest first')
  await clickButton('.confirmation-dialog', 'Confirm Cherry-pick')
  await until(() => {
    const log = git(clone, 'log', '--format=%s', '-3')
    return log.startsWith('side two\nside one\nbase commit')
  }, 'both commits replayed in history order')
  assert.ok(git(clone, 'show', 'HEAD:side-two.txt').includes('side two'), 'the second pick is the new tip')
  assert.ok(git(clone, 'show', 'HEAD:side-one.txt').includes('side one'), 'the first pick is below it')

  /* ---- G10: fetch options (prune and tag mode) ---- */
  // A branch and an unreachable tag created on the remote after the clone.
  git(origin, 'switch', '-c', 'feature')
  git(origin, 'commit', '--allow-empty', '-m', 'remote feature')
  git(origin, 'switch', 'main')
  git(origin, 'checkout', '--detach')
  git(origin, 'commit', '--allow-empty', '-m', 'orphan commit')
  git(origin, 'tag', 'v-orphan')
  git(origin, 'switch', 'main')

  await gitMenuAction('Fetch…')
  await until(() => evaluate("!!document.querySelector('[aria-label=\"Remote synchronization\"] fieldset')"), 'the fetch options')

  // Default fetch: branches arrive, a tag outside the fetched history does not.
  await fetchWith({ prune: false, tags: '' })
  assert.ok(remoteRefs().includes('refs/remotes/origin/feature'), 'the new remote branch was fetched')
  assert.equal(localTags(), '', 'reachable-tags mode skipped the unreachable tag')

  // Without prune, a branch deleted on the remote keeps its remote-tracking ref.
  git(origin, 'branch', '-D', 'feature')
  await fetchWith({ prune: false, tags: '' })
  assert.ok(remoteRefs().includes('refs/remotes/origin/feature'), 'the stale ref survived a non-pruning fetch')

  // Prune removes it; "All tags" also downloads the unreachable tag.
  await fetchWith({ prune: true, tags: 'all' })
  assert.ok(!remoteRefs().includes('refs/remotes/origin/feature'), 'prune removed the deleted branch')
  assert.equal(localTags(), 'v-orphan', 'all-tags mode fetched the unreachable tag')

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])

  /* ---- Missing folder: polling stops instead of repeating OS errors ---- */
  // A Git process may hold the working directory for a moment, which makes the
  // first rmdir fail on Windows; retry until the folder is actually gone.
  await rm(clone, { recursive: true, force: true, maxRetries: 20, retryDelay: 200 })
  const notice = await until(() => evaluate("document.querySelector('.top-error')?.innerText ?? ''"),
    'the missing-folder notice')
  assert.ok(notice.includes('no longer'), 'the missing folder is reported once: ' + notice)
  const snapshot = () => evaluate(`({
    alerts: Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText),
    status: Array.from(document.querySelectorAll('[role=status]'), node => node.innerText),
  })`)
  const before = await snapshot()
  await delay(7000)
  assert.deepEqual(await snapshot(), before, 'no repeated Git work after the folder disappears')
  assert.equal(await evaluate("document.querySelector('.top-error')?.innerText ?? ''"), notice,
    'the repository notice stays stable')

  console.log('Batch 4 smoke passed: multi-commit cherry-pick, fetch prune and tag modes, missing-folder polling.')
} finally {
  socket.close()
  if (!keepRepo) {
    await rm(base, { recursive: true, force: true, maxRetries: 20, retryDelay: 200 })
      .catch(() => console.log('Left fixture in place: ' + base))
  } else console.log('Kept fixture directory: ' + base)
}
