// Batch-2 feature smoke (G1 tags, G2 push options, G3 branch Update and Pull semantics,
// G6 ahead/behind badges) against a running `wails3 dev -tags desktop_smoke`
// instance. Usage:
//   node tests/batch2-smoke.mjs
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const keepRepo = process.env.GITVELA_BATCH2_KEEP === '1'

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

/* ---- Fixture: bare remote + clone (app target) + peer clone ---- */

const base = await mkdtemp(join(tmpdir(), 'gitvela-batch2-'))
const origin = join(base, 'origin')
const clone = join(base, 'clone')
const peer = join(base, 'peer')
await mkdir(origin)
const git = (repo, ...args) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' }).toString()

git(origin, 'init', '-b', 'main')
git(origin, 'config', 'user.email', 'smoke@example.com')
git(origin, 'config', 'user.name', 'Smoke')
execFileSync('git', ['-C', origin, 'commit', '--allow-empty', '-m', 'base commit'], { stdio: 'pipe' })
git(origin, 'branch', 'feature')
execFileSync('git', ['-C', base, 'clone', '--bare', '--quiet', origin, 'remote.git'])
const bare = join(base, 'remote.git')

execFileSync('git', ['-C', base, 'clone', '--quiet', 'remote.git', 'clone'], { stdio: 'pipe' })
git(clone, 'config', 'user.email', 'smoke@example.com')
git(clone, 'config', 'user.name', 'Smoke')
git(clone, 'branch', '--quiet', '--track', 'feature', 'origin/feature')

execFileSync('git', ['-C', base, 'clone', '--quiet', 'remote.git', 'peer'], { stdio: 'pipe' })
git(peer, 'config', 'user.email', 'peer@example.com')
git(peer, 'config', 'user.name', 'Peer')
execFileSync('git', ['-C', peer, 'commit', '--allow-empty', '-m', 'peer commit'], { stdio: 'pipe' })
git(peer, 'push', '--quiet', 'origin', 'main')
git(peer, 'checkout', '--quiet', '-b', 'feature', 'origin/feature')
execFileSync('git', ['-C', peer, 'commit', '--allow-empty', '-m', 'peer feature commit'], { stdio: 'pipe' })
git(peer, 'push', '--quiet', 'origin', 'feature')
const rev = (repo, ref) => git(repo, 'rev-parse', ref).trim()

/* ---- CDP plumbing (same pattern as batch1-smoke.mjs) ---- */

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

const openContextMenu = (selector, label) => until(() => evaluate(`(() => {
  const row = ${selector}
  if (!row) return false
  row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  return true
})()`), 'context menu on ' + label)
const menuLabels = menuLabel => evaluate(`[...document.querySelectorAll(${JSON.stringify(`[aria-label="${menuLabel}"] button`)})].map(button => button.innerText)`)

const contextMenuRow = (selector, label, menuLabel) => until(() => evaluate(`(() => {
  const row = ${selector}
  if (!row) return false
  row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  return true
})()`), 'context menu on ' + label).then(() => clickButton(`[aria-label=${JSON.stringify(menuLabel)}]`, label))

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
  await until(() => evaluate("document.querySelector('.commit-row canvas')?.width > 0"), 'repository history')

  /* ---- G6: ahead/behind badge after an external fetch ---- */
  const branchRow = "document.querySelector('.branch-row button.side-item[title=\"refs/heads/main\"]')"
  assert.ok(await evaluate(`!!${branchRow}`), 'main branch row listed')
  assert.equal(await evaluate(branchRow + '?.querySelector(".sync-badge")?.innerText'), undefined,
    'no badge while the tracking ref is up to date')
  execFileSync('git', ['-C', clone, 'fetch', '--quiet', 'origin'], { stdio: 'pipe' })
  await until(async () => (await evaluate(branchRow + '?.querySelector(".sync-badge")?.innerText')) === '↓1',
    'behind badge after fetching a peer commit')

  /* ---- G3: branch Update is not Pull (IDEA GitUpdateSelectedBranchAction) ---- */
  const featureRow = "document.querySelector('.branch-row button.side-item[title=\"refs/heads/feature\"]')"
  const mainBefore = rev(clone, 'HEAD')
  await openContextMenu(featureRow, 'feature')
  const featureMenu = await menuLabels('Branch actions')
  assert.ok(featureMenu.includes('Update') && featureMenu.includes('Push…'), 'non-current branch offers Update and Push: ' + featureMenu)
  assert.ok(!featureMenu.some(label => /^Pull/.test(label)), 'branch menu does not offer Pull: ' + featureMenu)
  await clickButton('[aria-label="Branch actions"]', 'Update')
  await until(async () => rev(clone, 'refs/heads/feature') === rev(clone, 'refs/remotes/origin/feature'), 'feature fast-forwarded')
  assert.equal(rev(clone, 'HEAD'), mainBefore, 'updating a non-current branch kept HEAD')
  assert.equal(git(clone, 'symbolic-ref', 'HEAD').trim(), 'refs/heads/main', 'HEAD stays on main')

  await openContextMenu(branchRow, 'main')
  const mainMenu = await menuLabels('Branch actions')
  assert.ok(mainMenu.includes('Update') && !mainMenu.some(label => /^Pull/.test(label)), 'current branch offers Update, not Pull: ' + mainMenu)
  await clickButton('[aria-label="Branch actions"]', 'Update')
  await until(async () => rev(clone, 'HEAD') === rev(clone, 'refs/remotes/origin/main'), 'main fast-forwarded by Update')
  await until(() => evaluate("[...document.querySelectorAll('.notification-balloon')].some(node => node.innerText.includes('fast-forwarded'))"),
    'update notification')

  /* ---- G3: Pull chooses its source explicitly (IDEA GitPullDialog) ---- */
  await evaluate("document.querySelector('.git-menu summary').click()")
  await clickButton('.git-menu', 'Pull…')
  await until(() => evaluate("!!document.querySelector('[aria-label=\"Remote branch to pull\"] option[value=feature]')"), 'pull branch list')
  assert.equal(await evaluate("document.querySelector('[aria-label=\"Remote branch to pull\"]').value"), 'main', 'upstream preselected')
  assert.equal(await evaluate("[...document.querySelectorAll('.pull-options input[type=radio]')].find(input => input.checked)?.value"), 'merge',
    'merge is the default pull mode')
  await evaluate(`(() => {
    const select = document.querySelector('[aria-label="Remote branch to pull"]')
    select.value = 'feature'
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })()`)
  await clickButton('.daily-panel', 'Pull')
  await until(() => evaluate("document.querySelector('.daily-panel')?.innerText.includes('Pull complete')"), 'pull notice')
  assert.equal(git(clone, 'log', '-1', '--format=%P').trim().split(' ').length, 2, 'pulling feature merged into main')
  assert.equal(git(clone, 'rev-parse', '--abbrev-ref', 'main@{upstream}').trim(), 'origin/main', 'pull kept the upstream')

  /* ---- G2: push options dialog ---- */
  await contextMenuRow(branchRow, 'Push…', 'Branch actions')
  await clickButton('.daily-panel', 'Push…')
  await until(() => evaluate("!!document.querySelector('.push-option')"), 'push options in the dialog')
  assert.ok(await evaluate("[...document.querySelectorAll('.push-option')].some(label => label.innerText.includes('Push annotated tags'))"),
    'push tags option listed')
  await evaluate(`(() => {
    const box = [...document.querySelectorAll('.push-option input[type=checkbox]')]
      .find(input => input.closest('.push-option').innerText.includes('Force overwrite'))
    box.click()
  })()`)
  await until(async () => (await evaluate("[...document.querySelectorAll('.daily-panel button')].some(button => button.innerText === 'Force push (lease)')")),
    'force confirmation label')
  await clickButton('.daily-panel', 'Force push (lease)')
  await until(() => evaluate("document.querySelector('.daily-panel')?.innerText.includes('Push complete')"), 'push notice')

  /* ---- G1: tag create / push / delete through the Tags panel ---- */
  await evaluate(`(() => {
    const select = document.querySelector('[aria-label="Reference type"]')
    select.value = 'tags'
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })()`)
  await clickButton('.branch-panel', 'New…')
  await evaluate(`(() => {
    const name = document.querySelector('[aria-label="New tag name"]')
    name.value = 'v1.0'
    name.dispatchEvent(new Event('input', { bubbles: true }))
    const message = document.querySelector('[aria-label="Optional tag message"]')
    message.value = 'batch2 smoke tag'
    message.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  await evaluate("document.querySelector('.branch-panel form.new-branch').requestSubmit()")
  const tagRow = "document.querySelector('.branch-row button.side-item[title=\"refs/tags/v1.0\"]')"
  await until(() => evaluate(`!!${tagRow}`), 'created tag listed')

  await contextMenuRow(tagRow, 'Push Tag…', 'Tag actions')
  await until(() => evaluate("!!document.querySelector('.branch-panel .confirmation-dialog select')"), 'push tag dialog')
  await clickButton('.branch-panel', 'Push tag')
  await until(() => evaluate("document.querySelector('.branch-panel')?.innerText.includes('Pushed tag v1.0')"), 'push tag notice')
  const tags = execFileSync('git', ['-C', bare, 'tag', '--list'], { stdio: 'pipe' }).toString()
  assert.ok(tags.includes('v1.0'), 'tag reached the remote')

  await contextMenuRow(tagRow, 'Delete…', 'Tag actions')
  await clickButton('.branch-panel', 'Delete tag')
  await until(() => evaluate(`!${tagRow}`), 'deleted tag removed from the list')

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])

  /* ---- G3: a conflicting Update stops in the console instead of failing silently ---- */
  git(peer, 'checkout', '--quiet', 'main')
  git(peer, 'pull', '--quiet', '--no-rebase', 'origin', 'main')
  await writeFile(join(peer, 'conflict.txt'), 'peer\n')
  git(peer, 'add', 'conflict.txt')
  execFileSync('git', ['-C', peer, 'commit', '-m', 'peer conflict'], { stdio: 'pipe' })
  git(peer, 'push', '--quiet', 'origin', 'main')
  await writeFile(join(clone, 'conflict.txt'), 'local\n')
  git(clone, 'add', 'conflict.txt')
  execFileSync('git', ['-C', clone, 'commit', '-m', 'local conflict'], { stdio: 'pipe' })
  await until(() => evaluate("[...document.querySelectorAll('.commit-row')].some(row => row.innerText.includes('local conflict'))"), 'local commit listed')
  await evaluate(`(() => {
    const select = document.querySelector('[aria-label="Reference type"]')
    select.value = 'branches'
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })()`)
  await openContextMenu(branchRow, 'main')
  await clickButton('[aria-label="Branch actions"]', 'Update')
  await until(() => evaluate("document.querySelector('[role=tab][aria-selected=true]')?.innerText === 'Console'"), 'console opened after a conflict')
  await until(() => evaluate("document.querySelector('.console-panel')?.innerText.includes('in progress')"), 'stopped merge shown')
  assert.ok(await evaluate("[...document.querySelectorAll('.notification-balloon.error')].some(node => node.innerText.includes('conflicts'))"),
    'conflict notification')
  git(clone, 'merge', '--abort')
  console.log('Batch 2 smoke passed: sync badges, branch update, pull source, push options, tag lifecycle.')
} finally {
  socket.close()
  if (!keepRepo) {
    // The app may still hold the clone open; cleanup is best-effort.
    await rm(base, { recursive: true, force: true }).catch(() => console.log('Left fixture in place: ' + base))
  } else console.log('Kept fixture directory: ' + base)
}
