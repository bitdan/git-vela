// Batch-1 feature smoke (G5 .gitignore, E1 line endings, G14 message history,
// issue navigation, E2 diff-tab session persistence) against a running
// `wails3 dev -tags desktop_smoke` instance. Usage:
//   node tests/batch1-smoke.mjs <absolute clone path>
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
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
await writeFile(join(repo, 'tracked.txt'), 'first\nsecond\n')
git('add', 'tracked.txt')
git('commit', '-m', 'Fix PROJ-1 issue')
await writeFile(join(repo, 'tracked.txt'), 'first\nworking copy\n')
await writeFile(join(repo, 'untracked.txt'), 'temp\n')

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
  await cdp('Page.navigate', { url: (await evaluate('location.origin')) + '/' })
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
  await contextMenuRow("document.querySelector('.working-row[title=\"tracked.txt\"]')", 'Jump to Source', 'Working file actions')
  await until(() => evaluate("!!document.querySelector('.workspace-editor')"), 'editor opens')
  const widget = "document.querySelector('.statusbar .line-ending-widget')"
  assert.equal(await evaluate("document.querySelector('.topbar > .repo-form')"), null, 'path input stays in the project menu')
  assert.equal(await evaluate("document.querySelector('.source-preview-tabs')?.innerText.includes('Commit diff')"), false)
  assert.equal(await evaluate("document.querySelector('.workspace-editor .editor-tools')"), null, 'editor has no second toolbar')
  await evaluate(`(() => {
    const editor = document.querySelector('.workspace-editor .cm-content')
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    editor.dispatchEvent(event)
    if (!event.defaultPrevented) throw new Error('WebView context menu was not suppressed')
  })()`)
  const gitActions = await until(() => evaluate(`[...document.querySelectorAll('[aria-label="File Git actions"] button')]
    .map(button => button.innerText)`).then(items => items.length && items), 'the source Git menu')
  assert.ok(gitActions.includes('Annotate with Git Blame') && gitActions.includes('Show History'))
  for (const action of ['Commit File…', 'Compare with Revision…', 'Compare with Branch or Tag…',
    'Show History for Selection…', 'Show Current Revision', 'Rollback…', 'Push…', 'Merge…', 'Manage Remotes…']) {
    assert.ok(gitActions.includes(action), action + ' is in the Git menu')
  }
  assert.ok(!gitActions.includes('Find / Replace'))
  assert.ok(await evaluate("!!document.querySelector('.topbar .git-menu') && !document.querySelector('.statusbar .git-menu')"))

  const openSourceMenu = () => evaluate(`document.querySelector('.workspace-editor .cm-content')
    .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))`)
  await clickButton('[aria-label="File Git actions"]', 'Annotate with Git Blame')
  await until(() => evaluate(`(() => {
    const gutter = document.querySelector('.workspace-editor .cm-git-blame')
    return gutter?.textContent.includes('Smoke') && gutter.textContent.includes('Uncommitted')
  })()`), 'author and working changes in the editor gutter')
  assert.ok(await evaluate("!!document.querySelector('.cm-git-blame button[title]')"))
  assert.equal(await evaluate("document.querySelector('.file-inspection') === null"), true, 'annotating keeps the source editor visible')
  await evaluate("document.querySelector('.cm-git-blame button').click()")
  await until(() => evaluate("document.querySelector('.commit-row.active')?.innerText.includes('PROJ-1')"), 'the annotation commit in the Git log')
  await openSourceMenu()
  await clickButton('[aria-label="File Git actions"]', 'Hide Git Blame')
  await until(() => evaluate("!document.querySelector('.cm-git-blame')"), 'the gutter to close')
  await openSourceMenu()
  await clickButton('[aria-label="File Git actions"]', 'Show Current Revision')
  await until(() => evaluate("document.querySelector('dialog .file-revision-content')?.textContent.includes('second')"), 'HEAD contents')
  await clickButton('dialog', 'Close')
  await openSourceMenu()
  await clickButton('[aria-label="File Git actions"]', 'Compare with Revision…')
  await clickButton('dialog', 'Compare')
  await until(() => evaluate("document.querySelector('dialog .file-revision-content')?.textContent.includes('+working copy')"), 'working-copy comparison')
  await clickButton('dialog', 'Close')
  await openSourceMenu()
  await clickButton('[aria-label="File Git actions"]', 'Rollback…')
  await until(() => evaluate("document.querySelector('dialog[aria-label=\"Rollback File\"]')?.open"), 'the rollback confirmation')
  assert.equal(await evaluate("document.activeElement.textContent.trim()"), 'Cancel')
  await clickButton('dialog', 'Cancel')
  assert.ok((await readRepo('tracked.txt')).includes('working copy'), 'cancel preserves the file')

  await openSourceMenu()
  await clickButton('[aria-label="File Git actions"]', 'Show History')
  await until(() => evaluate("document.querySelector('[aria-label=\"File inspection\"]') || document.querySelector('.inspection-scroll')"), 'the history of the source file')
  await evaluate(`document.querySelector('.working-row[title="tracked.txt"]')
    .dispatchEvent(new KeyboardEvent('keydown', { key: 'F4', bubbles: true, cancelable: true }))`)
  await until(() => evaluate("document.querySelector('.source-preview-tabs .editor-tab.active')?.title === 'tracked.txt'"), 'F4 to return to the source tab')
  assert.ok(await evaluate("document.querySelector('.statusbar .editor-meta')?.innerText.includes('UTF-8 ·')"), 'status bar shows encoding')
  assert.equal(await evaluate("document.querySelector('.statusbar .editor-meta .line-ending-widget')?.innerText"), 'LF', 'status bar widget shows LF')
  await evaluate("document.querySelector('.statusbar .editor-meta .line-ending-widget').click()")
  await until(() => evaluate(widget + "?.innerText === 'CRLF'"), 'widget switches to CRLF')
  await until(() => evaluate("document.querySelector('.statusbar .editor-meta')?.innerText.includes('Saved')"), 'conversion saved')
  assert.ok((await readRepo('tracked.txt')).includes('\r\n'), 'stored file converted to CRLF')
  assert.ok(await evaluate("document.querySelector('.statusbar .editor-meta')?.innerText.includes('UTF-8')"), 'UTF-8 label kept')

  /* ---- G14: commit message history ---- */
  await contextMenuRow("document.querySelector('.working-row[title=\"tracked.txt\"]')", 'Stage', 'Working file actions')
  await until(() => Promise.resolve(git('diff', '--cached', '--name-only').includes('tracked.txt')), 'selected file staged')
  await evaluate(`(() => {
    const input = document.querySelector('#commit-message')
    input.value = 'Batch1 smoke PROJ-2 fix'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  await clickButton('.commit-actions', 'Commit')
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
  await until(() => evaluate(`(() => {
    const sessions = JSON.parse(localStorage.getItem('gitvela.sessions') ?? '{}')
    return sessions.byPath?.[sessions.last]?.diffTabs?.length === 1
  })()`), 'diff tab persisted to the session')
  await cdp('Page.navigate', { url: (await evaluate('location.origin')) + '/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload for restore')
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].some(tab => tab.title === 'tracked.txt' && tab.classList.contains('active'))"), 'pinned diff tab restored and active')
  await until(() => evaluate("!!document.querySelector('.diff-host, .patch, .cm-mergeView')"), 'restored diff tab content loaded lazily')

  // Rollback is explicit and scoped even when another path is staged.
  git('add', '.gitignore')
  await writeFile(join(repo, 'tracked.txt'), 'discard only this edit\n')
  await evaluate("document.querySelector('.git-menu summary').click()")
  await clickButton('.git-menu', 'Refresh')
  await until(() => evaluate("!!document.querySelector('.working-row[title=\"tracked.txt\"]')"), 'the edited file to appear')
  await contextMenuRow("document.querySelector('.working-row[title=\"tracked.txt\"]')", 'More Git Actions…', 'Working file actions')
  await clickButton('[aria-label="File Git actions"]', 'Rollback…')
  await clickButton('dialog[aria-label="Rollback File"]', 'Rollback File')
  await until(async () => (await readRepo('tracked.txt')).includes('working copy'), 'the selected file to be restored')
  assert.equal(git('diff', '--cached', '--name-only').trim(), '.gitignore', 'rollback keeps other staged files')
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('Ready')"), 'rollback refresh to finish')

  // Project view has one header, a root node and lazily expanded directories.
  await evaluate("document.querySelector('.activity-bar [aria-label=Files]').click()")
  await until(() => evaluate("!!document.querySelector('.repository-file-row.project-root')"), 'the Project root row')
  assert.equal(await evaluate("document.querySelector('.workspace-sidebar .sidebar-project')"), null)
  const rootRow = "document.querySelector('.repository-file-row.project-root')"
  const treeMenu = 'Project file actions'
  const setName = value => evaluate(`(() => {
    const input = document.querySelector('#project-entry-name')
    input.value = ${JSON.stringify(value)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  const openTreeMenu = row => evaluate(`${row}.dispatchEvent(new MouseEvent('contextmenu', {
    bubbles: true, cancelable: true,
  }))`)
  const projectMenu = "document.querySelector('[aria-label=\"Project file actions\"]')"
  await openTreeMenu(rootRow)
  await until(() => evaluate('!!' + projectMenu), 'the project context menu')
  await evaluate('document.activeElement.blur()')
  await cdp('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await cdp('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await until(() => evaluate('!' + projectMenu), 'Escape dismisses the project context menu without focus')
  assert.equal(await evaluate('document.activeElement === ' + rootRow), true, 'Escape restores focus to the tree row')
  await openTreeMenu(rootRow)
  await until(() => evaluate('!!' + projectMenu), 'the context menu reopens')
  await evaluate(rootRow + ".dispatchEvent(new PointerEvent('pointerdown', {bubbles:true,button:0}))")
  await until(() => evaluate('!' + projectMenu), 'clicking the original row outside the menu dismisses it')
  await contextMenuRow(rootRow, 'New Directory…', treeMenu)
  await setName('empty')
  await clickButton('dialog', 'New Directory')
  const folderRow = "document.querySelector('.repository-file-row[title=\"empty\"]')"
  await until(() => evaluate('!!' + folderRow), 'empty directory shown in Project')
  await contextMenuRow(folderRow, 'New File…', treeMenu)
  await setName('sample.txt')
  await clickButton('dialog', 'New File')
  const sample = "document.querySelector('.repository-file-row[title=\"empty/sample.txt\"]')"
  await until(() => evaluate('!!' + sample), 'file shown below the expanded directory')
  await writeFile(join(repo, 'empty', 'sample.txt'), 'first line\nsearch target\n')
  await contextMenuRow(folderRow, 'Find in Directory…', treeMenu)
  await evaluate(`(() => {
    const field = document.querySelector('[aria-label="Search text"]')
    field.value = 'search target'
    field.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  await clickButton('dialog', 'Find')
  await until(() => evaluate("document.querySelector('.project-match strong')?.innerText === 'empty/sample.txt:2'"), 'directory search match')
  await evaluate("document.querySelector('.project-match').click()")
  await until(() => evaluate("document.querySelector('.editor-cursor')?.textContent.includes('Ln 2')"), 'result opens its source line')

  await evaluate("document.querySelector('[aria-label=\"Collapse project tree\"]').click()")
  await until(() => evaluate('!' + sample), 'the file is hidden in its collapsed directory')
  await evaluate("document.querySelector('[aria-label=\"Locate current file\"]').click()")
  await until(() => evaluate(sample + "?.classList.contains('active')"), 'locate expands and selects the current file')

  // Refresh the entry fingerprint after an external edit, then exercise paste and rename.
  await evaluate("document.querySelector('[aria-label=\"Refresh project\"]').click()")
  await until(() => evaluate("!document.querySelector('[aria-label=\"Refresh project\"]').disabled"), 'Project refresh')
  const key = (row, key, ctrlKey = false) => evaluate(`${row}.dispatchEvent(new KeyboardEvent('keydown',
    { key: ${JSON.stringify(key)}, ctrlKey: ${ctrlKey}, bubbles: true, cancelable: true }))`)
  await key(sample, 'c', true)
  await key(rootRow, 'v', true)
  await clickButton('dialog', 'Copy to Directory')
  await until(() => evaluate("!!document.querySelector('.repository-file-row[title=\"sample.txt\"]')"), 'copied file at the project root')
  const copy = "document.querySelector('.repository-file-row[title=\"sample.txt\"]')"
  await key(copy, 'F2')
  await setName('renamed.txt')
  await clickButton('dialog', 'Rename')
  const renamed = "document.querySelector('.repository-file-row[title=\"renamed.txt\"]')"
  await until(() => evaluate('!!' + renamed), 'renamed file visible')
  await key(renamed, 'Delete')
  assert.equal(await evaluate("document.activeElement.textContent.trim()"), 'Cancel')
  await clickButton('dialog', 'Cancel')
  assert.ok((await readRepo('renamed.txt')).includes('search target'), 'cancel keeps the file')
  await key(renamed, 'Delete')
  await clickButton('dialog', 'Delete from Disk')
  await until(() => evaluate('!' + renamed), 'deleted file removed from Project')
  assert.ok((await readRepo('empty/sample.txt')).includes('search target'), 'source copy remains')
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('Ready')"), 'Project mutation refresh')

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])
  console.log('Batch 1 smoke passed: Project tree and file operations, source Git menu, inline blame, revision comparison, rollback cancellation, .gitignore, line endings, message history, issue links, diff tab restore.')
} finally {
  socket.close()
  if (!keepRepo) {
    await rm(repo, { recursive: true, force: true, maxRetries: 20, retryDelay: 200 })
      .catch(() => console.log('Left fixture in place: ' + repo))
  } else console.log('Kept temp repository: ' + repo)
}
