// Batch-3 feature smoke (G4 rebase reword/edit, G8 squash into parent +
// squash merge plumbing, G9 clone from URL, G11 stash diff) against a running
// `wails3 dev -tags desktop_smoke` instance. Usage:
//   node tests/batch3-smoke.mjs
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, mkdir, rm } from 'node:fs/promises'
import { dirname, basename, join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const keepRepo = process.env.GITVELA_BATCH3_KEEP === '1'

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

/* ---- Fixture: origin + clone with two throwaway commits ---- */

const base = await mkdtemp(join(tmpdir(), 'gitvela-batch3-'))
const origin = join(base, 'origin')
const clone = join(base, 'clone')
await mkdir(origin)
const git = (repo, ...args) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' }).toString()

git(origin, 'init', '-b', 'main')
git(origin, 'config', 'user.email', 'smoke@example.com')
git(origin, 'config', 'user.name', 'Smoke')
execFileSync('git', ['-C', origin, 'commit', '--allow-empty', '-m', 'base commit'], { stdio: 'pipe' })

execFileSync('git', ['-C', base, 'clone', '--quiet', origin, 'clone'], { stdio: 'pipe' })
git(clone, 'config', 'user.email', 'smoke@example.com')
git(clone, 'config', 'user.name', 'Smoke')
execFileSync('git', ['-C', clone, 'commit', '--allow-empty', '-m', 'first change'], { stdio: 'pipe' })
execFileSync('git', ['-C', clone, 'commit', '--allow-empty', '-m', 'second change'], { stdio: 'pipe' })

/* ---- CDP plumbing (same pattern as batch2-smoke.mjs) ---- */

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

// A commit row is found by subject inside the virtualized log.
const commitRow = subject => `[...document.querySelectorAll('.commit-row')]
  .find(row => row.innerText.includes(${JSON.stringify(subject)}))`

const setInput = (selector, value) => until(() => evaluate(`(() => {
  const field = ${selector}
  if (!field || field.disabled) return false
  field.value = ${JSON.stringify(value)}
  field.dispatchEvent(new Event('input', { bubbles: true }))
  field.dispatchEvent(new Event('change', { bubbles: true }))
  return true
})()`), 'setting field ' + selector)

const choosePlanAction = (subject, action) => until(() => evaluate(`(() => {
  const row = [...document.querySelectorAll('.rebase-plan .rebase-step')]
    .find(item => item.innerText.includes(${JSON.stringify(subject)}))
  if (!row) return false
  const select = row.querySelector('select')
  if (!select || select.disabled) return false
  select.value = ${JSON.stringify(action)}
  select.dispatchEvent(new Event('change', { bubbles: true }))
  return true
})()`), 'plan action ' + action + ' for ' + subject)

// Only the message editors of reword/squash steps are visible; the others are
// hidden with v-show and must not receive the message.
const visibleTextarea = "[...document.querySelectorAll('.rebase-plan textarea')].find(item => item.offsetParent !== null)"

try {
  await evaluate("localStorage.clear()")
  await cdp('Page.navigate', { url: (await evaluate('location.origin')) + '/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload')
  await evaluate(`(() => {
    const input = document.querySelector('[aria-label="Repository path"]')
    input.value = ${JSON.stringify(clone)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form.requestSubmit()
  })()`)
  await until(() => evaluate("!!" + commitRow('second change')), 'repository history')

  /* ---- G8: squash a commit into its parent via the rebase plan ---- */
  await contextMenuRow(commitRow('second change'), 'Squash into parent…', 'Commit actions')
  await until(() => evaluate("!!document.querySelector('.rebase-plan')"), 'the rebase plan dialog')
  assert.equal(await evaluate("document.querySelectorAll('.rebase-plan .rebase-step').length"), 2,
    'the plan holds the parent and the target commit')
  assert.equal(await evaluate(`[...document.querySelectorAll('.rebase-plan .rebase-step')]
    .find(item => item.innerText.includes('second change'))?.querySelector('select')?.value`), 'squash',
    'the target commit is preset to squash')
  assert.ok(await evaluate("!!document.querySelector('.rebase-plan textarea')"),
    'a message editor is offered for the squashed commit')
  await setInput(visibleTextarea, 'first and second combined')
  await clickButton('.confirmation-dialog', 'Confirm Rebase')
  await until(() => {
    const log = git(clone, 'log', '--format=%B', '-1')
    return log.includes('first and second combined') && !git(clone, 'log', '--format=%s', '-3').includes('second change')
  }, 'the two commits to merge into one')
  await until(() => evaluate("!!" + commitRow('first and second combined')), 'the rewritten history to render')

  /* ---- G4: reword through the same plan dialog ---- */
  execFileSync('git', ['-C', clone, 'commit', '--allow-empty', '-m', 'third change'], { stdio: 'pipe' })
  execFileSync('git', ['-C', clone, 'branch', 'bookmark'], { stdio: 'pipe' })
  execFileSync('git', ['-C', clone, 'commit', '--allow-empty', '-m', 'fourth change'], { stdio: 'pipe' })
  const bookmarkRow = "document.querySelector('.branch-row button.side-item[title=\"refs/heads/bookmark\"]')"
  await until(() => evaluate("!!" + commitRow('fourth change')), 'the CLI commit to appear')
  await contextMenuRow(bookmarkRow, 'Interactive rebase onto this…', 'Branch actions')
  await until(() => evaluate("!!document.querySelector('.rebase-plan')"), 'the rebase plan dialog')
  await choosePlanAction('fourth change', 'reword')
  await setInput(visibleTextarea, 'fourth renamed')
  await clickButton('.confirmation-dialog', 'Confirm Rebase')
  await until(() => git(clone, 'log', '-1', '--format=%s').includes('fourth renamed'),
    'the commit message to be rewritten')
  await until(() => evaluate("!!" + commitRow('fourth renamed')), 'the reworded commit to render')

  /* ---- G4: edit stops the rebase and the console continues it ---- */
  await contextMenuRow(bookmarkRow, 'Interactive rebase onto this…', 'Branch actions')
  await until(() => evaluate("!!document.querySelector('.rebase-plan')"), 'the rebase plan dialog')
  await choosePlanAction('fourth renamed', 'edit')
  await clickButton('.confirmation-dialog', 'Confirm Rebase')
  // Poll for the rebase state directory instead of `git status`: a status call
  // refreshes and rewrites the index, and its transient index.lock makes the
  // app's rebase fail with "another Git process seems to be running".
  const rebaseDir = join(clone, '.git', 'rebase-merge')
  await until(() => existsSync(rebaseDir), 'the rebase to stop for the edit step')
  await clickButton('.lower-console', 'Continue')
  const continuation = 'dialog[aria-label="Confirm continue"]'
  await until(() => evaluate('document.querySelector(' + JSON.stringify(continuation) + ')?.open'), 'the Continue modal')
  assert.equal(await evaluate('document.activeElement.textContent.trim()'), 'Cancel')
  await cdp('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await cdp('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await until(() => evaluate('!document.querySelector(' + JSON.stringify(continuation) + ')'), 'Escape to close the Continue modal')
  assert.ok(existsSync(rebaseDir), 'Escape leaves the rebase in progress')

  await clickButton('.lower-console', 'Abort…')
  const abort = 'dialog[aria-label="Confirm abort"]'
  await until(() => evaluate('document.querySelector(' + JSON.stringify(abort) + ')?.open'), 'the Abort modal')
  assert.equal(await evaluate('document.activeElement.textContent.trim()'), 'Cancel')
  assert.ok((await evaluate('document.querySelector(' + JSON.stringify(abort) + ').innerText')).includes(join(basename(dirname(clone)), basename(clone))))
  await evaluate('document.querySelector(' + JSON.stringify(abort) + ').querySelector("[aria-label=\\\"Close dialog\\\"]").click()')
  await until(() => evaluate('!document.querySelector(' + JSON.stringify(abort) + ')'), 'close button to dismiss Abort')
  assert.ok(existsSync(rebaseDir), 'closing Abort leaves the rebase in progress')

  await clickButton('.lower-console', 'Continue')
  await clickButton(continuation, 'Confirm continue')
  await until(() => !existsSync(rebaseDir), 'the rebase to finish after continue')
  assert.equal(git(clone, 'log', '-2', '--format=%s'), 'fourth renamed\nthird change\n',
    'history is unchanged across an edit stop')

  /* ---- G9: clone from URL, started from the Git menu ---- */
  await until(() => evaluate(`(() => {
    const menu = document.querySelector('.git-menu')
    if (!menu) return false
    if (!menu.open) menu.querySelector('summary').click()
    const button = [...menu.querySelectorAll('button')].find(item => item.innerText === 'Clone from URL…')
    if (!button || button.disabled) return false
    button.click()
    return true
  })()`), 'the Git menu clone action')
  await until(() => evaluate('document.querySelector(\'dialog[aria-label="Clone repository"]\')?.open'), 'the clone modal')
  assert.equal(await evaluate('document.activeElement.textContent.trim()'), 'Cancel')
  assert.deepEqual(await evaluate("[...document.querySelectorAll('.activity-bar button')].map(button => button.ariaLabel)"),
    ['Files', 'Commit', 'Git Log'])
  assert.ok(await evaluate("!!document.querySelector('.topbar .git-menu') && !!document.querySelector('.topbar [aria-label=Settings]')"))
  const cloneField = name => `document.querySelector('[aria-label="Clone repository"] [aria-label="${name}"]')`
  await setInput(cloneField('Repository URL or path'), origin)
  await setInput(cloneField('Clone parent folder'), base)
  await setInput(cloneField('Clone folder name'), 'cloned-repo')
  await clickButton('[aria-label="Clone repository"]', 'Clone')
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('cloned-repo')"), 'the cloned repository to open')
  assert.equal(git(join(base, 'cloned-repo'), 'log', '-1', '--format=%s').trim(), 'base commit',
    'the clone holds the origin history')

  /* ---- G11: stash diff from the shelf panel ---- */
  const cloned = join(base, 'cloned-repo')
  git(cloned, 'config', 'user.email', 'smoke@example.com')
  git(cloned, 'config', 'user.name', 'Smoke')
  execFileSync('git', ['-C', cloned, 'commit', '--allow-empty', '--allow-empty-message', '-m', 'seed'], { stdio: 'pipe' })
  const stashSeed = join(cloned, 'seed.txt')
  execFileSync('node', ['-e', `require('fs').writeFileSync(${JSON.stringify(stashSeed)}, 'seed v1\\n')`], { stdio: 'pipe' })
  execFileSync('git', ['-C', cloned, 'add', '--', 'seed.txt'], { stdio: 'pipe' })
  execFileSync('git', ['-C', cloned, 'commit', '-m', 'add seed'], { stdio: 'pipe' })
  execFileSync('node', ['-e', `require('fs').writeFileSync(${JSON.stringify(stashSeed)}, 'seed v2\\n')`], { stdio: 'pipe' })
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('1') || document.body.innerText.includes('seed.txt')"),
    'the working change to be picked up', )
  // Opening the shelf is a no-op while the repository is still refreshing, so
  // retry the menu click until the panel actually mounts instead of clicking
  // once and waiting for a panel that will never appear.
  await until(() => evaluate(`(() => {
    if (document.querySelector('[aria-label="Stashes"]')) return true
    const menu = document.querySelector('.git-menu')
    if (!menu.open) menu.querySelector('summary').click()
    const button = [...menu.querySelectorAll('button')].find(item => item.innerText === 'Stash…')
    if (!button || button.disabled) return false
    button.click()
    return false
  })()`), 'the shelf panel')
  await setInput(`document.querySelector('[aria-label="Stashes"] [aria-label="Stash message"]')`, 'wip change')
  await clickButton('[aria-label=\"Stashes\"]', 'Save stash')
  const stashRow = "[...document.querySelectorAll('[aria-label=\"Saved stashes\"] button')].find(item => item.innerText.includes('wip change'))"
  await until(() => evaluate(`!!${stashRow}`), 'the stash entry to be listed')
  await contextMenuRow(stashRow, 'View Diff', 'Shelf actions')
  // Read the details panel's text content: the history tool window can be
  // collapsed (display: none), and body.innerText skips hidden subtrees.
  const detailsText = `document.querySelector('[aria-label="Commit Details"]')?.textContent ?? ''`
  await until(() => evaluate(`${detailsText}.includes('Stash stash@{0}')`), 'the stash diff comparison')

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])
  console.log('Batch 3 smoke passed: squash into parent, reword, edit+continue, clone from URL, stash diff.')
} finally {
  socket.close()
  if (!keepRepo) {
    await rm(base, { recursive: true, force: true }).catch(() => console.log('Left fixture in place: ' + base))
  } else console.log('Kept fixture directory: ' + base)
}
