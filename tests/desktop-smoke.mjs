import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, unlink, writeFile } from 'node:fs/promises'
import { dirname, join, resolve, sep } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const repo = process.argv[2] ?? process.cwd()
const screenshot = process.argv[3]
const workingFile = process.argv[4]
const testMvp = process.env.GITVELA_SMOKE_MVP === '1'
const testM4 = process.env.GITVELA_SMOKE_M4 === '1'
const dailyRemote = process.env.GITVELA_SMOKE_M2_REMOTE
const worktreeTarget = process.env.GITVELA_SMOKE_WORKTREE
const testIndex = process.argv[5] === '--index'
const testCommit = process.argv[5] === '--commit'
const testAdvanced = process.env.GITVELA_SMOKE_M3 === '1'
const testEditor = process.env.GITVELA_SMOKE_EDITOR === '1'
const testProject = process.env.GITVELA_SMOKE_PROJECT === '1'
const githubLogin = process.env.GITVELA_SMOKE_GITHUB_LOGIN
const testBranch = process.argv[5] === '--branch' || process.env.GITVELA_SMOKE_BRANCH === '1'

async function until(action, description) {
  const deadline = Date.now() + 30000
  while (Date.now() < deadline) {
    const result = await action()
    if (result) return result
    await delay(100)
  }
  throw new Error('Timed out waiting for ' + description)
}

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

// Editor typing relies on the WebView having keyboard focus; a freshly
// launched window may start unfocused, which silently swallows Input events.
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

/** Show a working change's staged/unstaged diff through the row context menu;
 *  the fixed Working file / Unstaged / Staged buttons became per-file tabs. */
const showWorkingDiff = async (mode, title) => {
  await until(() => evaluate(`(() => {
    const row = ${title
      ? `document.querySelector('.working-row[title="' + ${JSON.stringify(title)} + '"]')`
      : `document.querySelector('.working-row.active') ?? [...document.querySelectorAll('.working-row')].find(item => !item.classList.contains('folder-row'))`}
    if (!row) return false
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
    return true
  })()`), 'working row for ' + (title ?? 'the selected change'))
  await clickButton('[aria-label="Working file actions"]', mode === 'staged' ? 'Show Staged Diff' : 'Show Unstaged Diff')
}

async function gitMenuAction(label) {
  await evaluate("if (!document.querySelector('.git-menu').open) document.querySelector('.git-menu summary').click()")
  await clickButton('.git-menu', label)
}

/** Settings groups live behind tabs, so a test must open the page it needs. */
async function settingsTab(label) {
  await until(() => evaluate(`(() => {
    const button = [...document.querySelectorAll('.settings-tabs [role=tab]')]
      .find(item => item.innerText === ${JSON.stringify(label)})
    if (!button) return false
    if (button.getAttribute('aria-selected') !== 'true') button.click()
    return button.getAttribute('aria-selected') === 'true'
  })()`), 'settings tab ' + label)
}

async function branchMenuAction(branch, label) {
  const selector = '[title="refs/heads/' + branch + '"]'
  await until(() => evaluate(`(() => {
    const row = document.querySelector(${JSON.stringify(selector)})
    if (!row) return false
    // Single click navigates Git Log; branch actions are on the context menu (IDEA).
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
    return true
  })()`), 'branch ' + branch)
  await clickButton('[aria-label="Branch actions"]', label)
}

const inputValue = (selector, value) => evaluate(`(() => {
  const field = document.querySelector(${JSON.stringify(selector)})
  field.value = ${JSON.stringify(value)}
  field.dispatchEvent(new Event('input', { bubbles: true }))
  field.dispatchEvent(new Event('change', { bubbles: true }))
})()`)

let shortcutRepo
try {
  // Previous runs persist their selection, tool-window state and compare
  // ranges in localStorage; the session restore then races this script's
  // first clicks and can re-select a stale commit (seen 2026-11-08). Always
  // start from a clean slate and navigate before touching the UI. Navigation
  // goes through Page.navigate because calling location.reload() inside a
  // Runtime.evaluate can deadlock the WebView2 renderer.
  await evaluate("localStorage.clear()")
  await cdp('Page.navigate', { url: (await evaluate('location.origin')) + '/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload')
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('git version')"), 'the Go Git version binding')
  await evaluate(`(() => {
    window.smokeRejections = []
    window.addEventListener('unhandledrejection', event => window.smokeRejections.push(String(event.reason)), { signal: AbortSignal.timeout(30000) })
  })()`)
  await evaluate(`(() => {
    const input = document.querySelector('[aria-label="Repository path"]')
    input.value = ${JSON.stringify(repo)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form.requestSubmit()
  })()`)
  await until(() => evaluate("document.querySelector('.commit-row canvas')?.width > 0"), 'real commit history and a rendered graph')
  assert.equal(await evaluate(`(() => {
    const rect = selector => document.querySelector(selector).getBoundingClientRect()
    const working = rect('.working-panel')
    const preview = rect('.preview-panel')
    const sidebar = rect('.workspace-sidebar')
    const branches = rect('.branch-panel')
    const history = rect('.log-panel')
    const changes = rect('.files-panel')
    return working.left >= sidebar.left && working.right <= sidebar.right &&
      sidebar.right < preview.x && sidebar.y === preview.y &&
      preview.bottom < history.y && branches.y === history.y && history.y === changes.y &&
      branches.right < history.x && history.right < changes.x && sidebar.bottom === preview.bottom
  })()`), true)
  assert.equal(await evaluate("document.querySelector('.shell').dataset.theme"), 'light')
  if (workingFile) {
    await until(() => evaluate(`(() => {
      const row = [...document.querySelectorAll('.working-row')].find(row => row.title === ${JSON.stringify(workingFile)})
      if (!row) return false
      row.click()
      return true
    })()`), 'working file in the upper left')
    await until(() => evaluate("document.querySelector('.preview-panel .working-preview')?.textContent.length > 0"), 'working file content in the upper right')
  }
  if (testIndex) {
    const buttons = "Array.from(document.querySelectorAll('.index-actions button'))"
    await evaluate(buttons + ".find(button => button.innerText === 'Stage').click()")
    await until(() => evaluate(buttons + ".some(button => button.innerText === 'Unstage' && !button.disabled)"), 'the selected file staged')
    await evaluate(buttons + ".find(button => button.innerText === 'Unstage').click()")
    await until(() => evaluate(buttons + ".some(button => button.innerText === 'Stage' && !button.disabled)"), 'the selected file unstaged')
  }
  if (testCommit) {
    for (const [mode, line] of [['staged', '+staged version'], ['unstaged', '+working version']]) {
      await showWorkingDiff(mode)
      await until(() => evaluate(`document.querySelector('.patch')?.textContent.includes(${JSON.stringify(line)})`), mode + ' diff')
    }
    await evaluate(`(() => {
      const input = document.querySelector('#commit-message')
      input.value = 'Commit from desktop'
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })()`)
    await until(() => evaluate("!document.querySelector('.commit-actions button').disabled"), 'commit enabled')
    await evaluate("document.querySelector('.commit-actions button').click()")
    await until(() => evaluate("[...document.querySelectorAll('.commit-row')].some(row => row.innerText.includes('Commit from desktop'))"), 'new commit in history')
  }
  if (testBranch) {
    await gitMenuAction('Branches')
    await evaluate("document.querySelector('.branch-panel .panel-header button').click()")
    await evaluate(`(() => {
      const input = document.querySelector('[aria-label="New branch name"]')
      input.value = 'feature/desktop'
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.form.requestSubmit()
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/feature/desktop\"]')"), 'new local branch')
    assert.equal(await evaluate("document.querySelector('.branch-panel .active')?.title"), 'refs/heads/main')
    await branchMenuAction('feature/desktop', 'Checkout')
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.title === 'refs/heads/feature/desktop'"), 'new active branch')
  }
  if (testBranch) {
    const click = (selector) => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`)
    const action = (label) => until(() => evaluate(`(() => {
      const button = [...document.querySelectorAll('[aria-label="Confirm branch deletion"] button')].find(item => item.innerText === ${JSON.stringify(label)})
      if (!button || button.disabled) return false
      button.click()
      return true
    })()`), 'deletion dialog button ' + label)
    await branchMenuAction('main', 'Checkout')
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.title === 'refs/heads/main'"), 'main active again')
    await branchMenuAction('feature/desktop', 'Rename…')
    await evaluate(`(() => {
      const input = document.querySelector('[aria-label="New branch name"]')
      input.value = 'feature/renamed'
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.form.requestSubmit()
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/feature/renamed\"]')"), 'renamed branch')
    await branchMenuAction('feature/renamed', 'Delete…')
    await action('Cancel')
    assert.equal(await evaluate("!!document.querySelector('[title=\"refs/heads/feature/renamed\"]')"), true)
    await branchMenuAction('feature/renamed', 'Delete…')
    await action('Delete branch')
    await until(() => evaluate("!document.querySelector('[title=\"refs/heads/feature/renamed\"]')"), 'confirmed deletion')
    // An unmerged branch names what it discards, needs "Delete anyway", and can be restored.
    const mainTip = execFileSync('git', ['-C', repo, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
    execFileSync('git', ['-C', repo, 'branch', 'smoke/unmerged'])
    const unmergedTip = execFileSync('git', ['-C', repo, 'commit-tree', '-p', mainTip, '-m', 'unmerged smoke commit', mainTip + '^{tree}'], { encoding: 'utf8' }).trim()
    execFileSync('git', ['-C', repo, 'update-ref', 'refs/heads/smoke/unmerged', unmergedTip])
    await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/smoke/unmerged\"]')"), 'unmerged branch listed')
    await branchMenuAction('smoke/unmerged', 'Delete…')
    await until(() => evaluate("document.querySelector('[aria-label=\"Confirm branch deletion\"]')?.innerText.includes('not merged into main')"), 'unmerged warning')
    assert.equal(await evaluate("document.activeElement?.innerText"), 'Cancel', 'Cancel is focused by default')
    await action('Delete anyway')
    await until(() => evaluate("!document.querySelector('[title=\"refs/heads/smoke/unmerged\"]')"), 'unmerged branch deleted')
    await until(() => evaluate(`(() => {
      const balloon = [...document.querySelectorAll('.notification-balloon')].find(item => item.innerText.includes('smoke/unmerged'))
      const button = [...(balloon?.querySelectorAll('button') ?? [])].find(item => item.innerText === 'Restore')
      if (!button) return false
      button.click()
      return true
    })()`), 'Restore action')
    await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/smoke/unmerged\"]')"), 'branch restored')
    assert.equal(execFileSync('git', ['-C', repo, 'rev-parse', 'refs/heads/smoke/unmerged'], { encoding: 'utf8' }).trim(), unmergedTip)
    // The smoke repository is a disposable clone, so it can carry its own tag.
    const tagged = execFileSync('git', ['-C', repo, 'log', '-1', '--format=%s', 'HEAD~1'], { encoding: 'utf8' }).trim()
    execFileSync('git', ['-C', repo, 'tag', 'gitvela-smoke-tag', 'HEAD~1'])
    await evaluate(`(() => {
      const picker = document.querySelector('[aria-label="Reference type"]')
      picker.value = 'tags'
      picker.dispatchEvent(new Event('change', { bubbles: true }))
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"refs/tags/gitvela-smoke-tag\"]')"), 'repository tag listed')
    await click('[title="refs/tags/gitvela-smoke-tag"]')
    await until(() => evaluate(`document.querySelector('.commit-row.active')?.innerText.includes(${JSON.stringify(tagged.slice(0, 40))})`), 'tag commit selected')
  }
  if (worktreeTarget) {
    const click = (selector) => until(() => evaluate(`(() => {
      const button = document.querySelector(${JSON.stringify(selector)})
      if (!button || button.disabled) return false
      button.click()
      return true
    })()`), 'enabled worktree control ' + selector)
    await gitMenuAction('Worktrees')
    await until(() => evaluate("document.querySelectorAll('.worktree-row').length === 1"), 'main worktree listed')
    await click('[aria-label="Worktrees"] .panel-header button[aria-expanded="false"]')
    await evaluate(`(() => {
      const form = document.querySelector('.worktree-form')
      const [directory, branch] = form.querySelectorAll('input')
      for (const [input, value] of [[directory, ${JSON.stringify(worktreeTarget)}], [branch, 'feature/desktop-worktree']]) {
        input.value = value
        input.dispatchEvent(new Event('input', { bubbles: true }))
      }
    })()`)
    await until(() => evaluate("!document.querySelector('.worktree-form button[type=submit]')?.disabled"), 'worktree creation enabled')
    await click('.worktree-form button[type=submit]')
    await until(() => evaluate("document.querySelectorAll('.worktree-row').length === 2"), 'linked worktree created')
    await click('.worktree-row:nth-last-child(1) .side-item')
    await until(() => evaluate("document.querySelector('.statusbar').innerText.includes('feature/desktop-worktree')"), 'linked worktree opened')
    await until(() => evaluate("document.querySelectorAll('.worktree-row').length === 2"), 'worktrees listed from linked checkout')
    await click('.worktree-row:nth-last-child(2) .side-item')
    await until(() => evaluate("document.querySelector('.statusbar').innerText.includes(' · main')"), 'main worktree opened again')
    await until(() => evaluate("!!document.querySelector('.worktree-row .switch-branch')"), 'remove action available')
    await click('.worktree-row .switch-branch')
    await click('[aria-label="Confirm worktree removal"] .danger-button')
    await until(() => evaluate("document.querySelectorAll('.worktree-row').length === 1"), 'confirmed removal')
  }
  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])

  await until(() => evaluate(`(() => {
    const row = [...document.querySelectorAll('.commit-row')].find(row => row.innerText.includes('Initialize GitVela'))
    if (row) { row.click(); return true }
    // The log is virtualized: the first commit scrolls out of the rendered
    // window once the repository grows past a viewport of history.
    const scroll = document.querySelector('.log-scroll')
    if (scroll) scroll.scrollTop = scroll.scrollHeight
    return false
  })()`), 'the initial commit')
  await until(() => evaluate(`(() => {
    const list = document.querySelector('.files-panel .file-scroll')
    const file = list?.querySelector('.file-row[title="GitVela.md"]')
    if (!file) {
      if (list) list.scrollTop += list.clientHeight
      return false
    }
    file.click()
    return true
  })()`), 'GitVela.md in Changed Files')
  await until(() => evaluate(`(() => {
    const editors = document.querySelectorAll('.cm-mergeView .cm-content')
    return editors.length === 2 && editors[0].textContent === '' &&
      editors[1].textContent.includes('GitVela') && editors[1].getAttribute('contenteditable') !== 'true'
  })()`), 'read-only side-by-side Diff with the real root blob')

  // Pinned per-file diff tabs (T03/I03): double-clicking a history file opens
  // a dedicated diff tab instead of stealing the shared preview.
  await evaluate(`(() => {
    const row = document.querySelector('.files-panel .file-row[title="GitVela.md"]')
    row.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }))
  })()`)
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].some(tab => tab.title === 'GitVela.md' && tab.classList.contains('active'))"), 'pinned diff tab opens and activates')
  await evaluate("[...document.querySelectorAll('.preview-tabs button')].find(button => button.innerText === 'Commit diff').click()")
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].every(tab => !tab.classList.contains('active'))"), 'selection-driven tab takes over')
  await evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].find(tab => tab.title === 'GitVela.md').click()")
  await until(() => evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].some(tab => tab.title === 'GitVela.md' && tab.classList.contains('active'))"), 'pinned diff tab refocuses')
  await evaluate("[...document.querySelectorAll('.preview-tabs .editor-tab')].find(tab => tab.title === 'GitVela.md').querySelector('.editor-close').click()")
  await until(() => evaluate("document.querySelectorAll('.preview-tabs .editor-tab').length === 0"), 'pinned diff tab closes')

  // Ctrl/Shift multi-selection (T06/I04): compare selected, write ops guarded.
  await evaluate("document.querySelectorAll('.commit-row')[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))")
  await until(() => evaluate("document.querySelectorAll('.commit-row.active').length === 1"), 'plain click resets selection')
  await evaluate("document.querySelectorAll('.commit-row')[1].dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }))")
  await until(() => evaluate("document.querySelectorAll('.commit-row.active').length === 2"), 'ctrl-click extends selection')
  // A background reload (watcher snapshot change, five-second poll) collapses
  // the multi-selection back to the single restored commit and closes the
  // menu. Rebuild the selection and the menu inside the polled action so a
  // reload in the middle of the step cannot strand it.
  const guard = await until(() => evaluate(`(() => {
    const buttons = [...document.querySelectorAll('[aria-label="Commit actions"] button')]
    const compare = buttons.find(button => button.innerText.startsWith('Compare selected'))
    if (compare && !compare.disabled) {
      // Cherry-pick follows a multi-selection; the other rewrites must not.
      const guarded = buttons.find(button => button.innerText === 'Revert…')?.disabled === true &&
        buttons.find(button => button.innerText === 'Reset to here…')?.disabled === true &&
        buttons.some(button => button.innerText.startsWith('Cherry-pick ' + document.querySelectorAll('.commit-row.active').length + ' selected'))
      compare.click()
      return guarded ? 'guarded' : 'unguarded'
    }
    // Rebuild a collapsed selection, but never re-click while two rows are
    // still selected: a row click closes the menu, so clicking on every poll
    // would keep it from ever rendering.
    if (document.querySelectorAll('.commit-row.active').length !== 2) {
      const rows = document.querySelectorAll('.commit-row')
      if (rows.length < 2) return false
      rows[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
      rows[1].dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }))
      return false
    }
    document.querySelectorAll('.commit-row.active')[1]
      ?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
    return false
  })()`), 'compare selected available')
  assert.equal(guard, 'guarded', 'write ops disabled on multi-selection')
  await until(() => evaluate("document.querySelector('.files-panel .eyebrow')?.innerText === 'COMPARE REVISIONS'"), 'comparison opens from multi-selection')

  if (testMvp) {
    if (worktreeTarget) {
      await gitMenuAction('Branches')
      execFileSync('git', ['-C', repo, 'branch', 'smoke/external-metadata'])
      await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/smoke/external-metadata\"]')"), 'external branch refresh')
    }
    const click = (selector) => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`)
    const logSearch = '[aria-label="Text or hash"]'
    await inputValue(logSearch, 'Initialize GitVela')
    await evaluate(`document.querySelector(${JSON.stringify(logSearch)}).form.requestSubmit()`)
    await until(() => evaluate("document.querySelector('.search-results .commit-row')?.innerText.includes('Initialize GitVela')"), 'history search result')
    await evaluate("document.querySelector('.search-results .commit-row').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))")
    await until(() => evaluate("document.querySelector('[aria-label=\"Commit actions\"]')?.innerText.includes('Copy message')"), 'commit context actions')
    await evaluate("[...document.querySelectorAll('[aria-label=\"Commit actions\"] button')].find(button => button.innerText === 'Copy hash').click()")
    await until(() => evaluate("document.querySelector('.log-panel [role=status]')?.innerText === 'Hash copied.'"), 'copy hash')
    await click('[aria-label="Clear filters"]')
    await click('[aria-label="Sort and history options"]')
    await clickButton('[aria-label="sort options"]', 'By Commit Date')
    await until(() => evaluate(`!!document.querySelector('.commit-row canvas') && !!document.querySelector('[aria-label="Clear filters"]')`), 'ordered graph')
    await click('[aria-label="Clear filters"]')
    await clickButton('.log-toolbar', 'Paths ⌄')
    await inputValue('[aria-label="paths options"] input', 'GitVela.md')
    await clickButton('[aria-label="paths options"]', 'Apply')
    await until(() => evaluate("!!document.querySelector('.search-results .commit-row')"), 'repository path filter')
    await click('[aria-label="Clear filters"]')
    await click('[aria-label="History display options"]')
    await clickButton('[aria-label="view options"]', 'Hash')
    assert.equal(await evaluate("document.querySelector('.log-heading').innerText.includes('Hash')"), false)
    await clickButton('[aria-label="view options"]', 'Hash')
    await evaluate("document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))")
    await until(() => evaluate("!!document.querySelector('.commit-row canvas')"), 'return to history graph')
    // Settings opens as a modal dialog; toggles apply live.
    await click('.topbar button[aria-label="Settings"]')
    await until(() => evaluate("!!document.querySelector('.settings-dialog[open]')"), 'settings dialog opens')
    await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('Dark theme')).querySelector('input').click()")
    await until(() => evaluate("document.querySelector('.shell').dataset.theme === 'dark'"), 'dark theme from Settings')
    await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('Dark theme')).querySelector('input').click()")
    const treeWasOn = await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('as a tree')).querySelector('input').checked")
    if (!treeWasOn) await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('as a tree')).querySelector('input').click()")
    await until(() => evaluate("!!document.querySelector('.files-panel .folder-row')"), 'file tree from Settings')
    if (!treeWasOn) await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('as a tree')).querySelector('input').click()")
    await click('.settings-dialog .dialog-actions button')
    await until(() => evaluate("!document.querySelector('.settings-dialog')"), 'settings dialog closed')
    await until(() => evaluate("!!document.querySelector('.diff-host .cm-editor')"), 'commit diff available')
    await click('.preview-tabs .history-toggle')
    await until(() => evaluate("getComputedStyle(document.querySelector('#history-dock')).display === 'none' && getComputedStyle(document.querySelector('.workspace-sidebar')).display !== 'none'"), 'history hidden with sidebar open')
    await click('.preview-tabs .history-toggle')
    await until(() => evaluate("getComputedStyle(document.querySelector('#history-dock')).display !== 'none'"), 'history visible again')
    await click('.preview-tabs .diff-size')
    await until(() => evaluate("document.querySelector('.upper').classList.contains('diff-expanded') && getComputedStyle(document.querySelector('.lower')).display === 'none'"), 'maximized diff')
    await evaluate("document.querySelector('.preview-tabs .diff-size').dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))")
    await until(() => evaluate("!document.querySelector('.upper').classList.contains('diff-expanded')"), 'restored workspace')
  }
  await clickButton('.diff-actions', 'Unified')
  await until(() => evaluate("document.querySelectorAll('.diff-host .cm-editor').length === 1"), 'unified Diff')
  await clickButton('.diff-actions', 'Side by side')
  await until(() => evaluate("document.querySelectorAll('.cm-mergeView .cm-editor').length === 2"), 'side-by-side Diff restored')
  assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"), [])
  if (dailyRemote) {
    const dailyBranch = 'smoke/daily'
    const git = (dir, ...args) => execFileSync('git', ['-C', dir, ...args], { encoding: 'utf8' }).trim()
    const click = (container, label) => until(() => evaluate(`(() => {
      const button = [...document.querySelectorAll(${JSON.stringify(container + ' button')})]
        .find(item => item.innerText === ${JSON.stringify(label)})
      if (!button || button.disabled) return false
      button.click()
      return true
    })()`), 'enabled ' + label)
    const set = (selector, value) => evaluate(`(() => {
      const field = document.querySelector(${JSON.stringify(selector)})
      field.value = ${JSON.stringify(value)}
      field.dispatchEvent(new Event('input', { bubbles: true }))
      field.dispatchEvent(new Event('change', { bubbles: true }))
    })()`)
    const config = dir => {
      git(dir, 'config', 'user.name', 'GitVela Desktop Smoke')
      git(dir, 'config', 'user.email', 'smoke@example.invalid')
      git(dir, 'config', 'commit.gpgsign', 'false')
    }
    const text = (selector, expected) =>
      until(() => evaluate(`[...document.querySelectorAll(${JSON.stringify(selector)})].some(node => node.innerText.includes(${JSON.stringify(expected)}))`), expected)

    config(repo)
    git(repo, 'switch', '-c', dailyBranch)
    await text('.statusbar', dailyBranch)
    await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")
    // Amend is an IDEA-style checkbox above the message field now. The 5s
    // external-change poll can re-render the commit form right after the
    // branch switch, so retry until the checkbox click sticks (2026-11-08).
    await until(() => evaluate(`(() => {
      const box = document.querySelector('.amend-row input')
      if (!box || box.disabled) return false
      if (!box.checked) box.click()
      return !!document.querySelector('.amend-warning')
    })()`), 'amend armed with warning')
    assert.ok(await evaluate("document.querySelector('#commit-message').value.trim().length"))
    await set('#commit-message', 'Amended through GitVela')
    await click('.commit-form', 'Amend commit')
    await text('.commit-form [role=status]', 'Commit amended')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Amended through GitVela')

    const untracked = join(repo, 'gitvela-smoke-untracked.txt')
    await writeFile(untracked, 'saved by GitVela', { flag: 'wx' })
    await gitMenuAction('Stash…')
    await evaluate("document.querySelector('[aria-label=Stashes] input[type=checkbox]').click()")
    await set('[aria-label="Stash message"]', 'desktop stash')
    await click('[aria-label=Stashes]', 'Save stash')
    await text('[aria-label=Stashes] [role=status]', 'Stash saved.')
    assert.equal(git(repo, 'status', '--porcelain'), '')
    await until(() => evaluate("!!document.querySelector('.shelf-entries button')"), 'saved stash visible')
    await evaluate("document.querySelector('.shelf-entries button').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))")
    await click('[aria-label="Shelf actions"]', 'Pop')
    await text('[aria-label=Stashes] [role=status]', 'Stash applied and removed.')
    assert.match(git(repo, 'status', '--porcelain'), /gitvela-smoke-untracked/)
    assert.equal(git(repo, 'stash', 'list'), '')
    await unlink(untracked)

    git(repo, 'init', '--bare', '-b', dailyBranch, dailyRemote)
    git(repo, 'remote', 'add', 'm2-smoke', dailyRemote)
    await writeFile(join(repo, 'commit-and-push-smoke.txt'), 'local commit before push review')
    git(repo, 'add', '--', 'commit-and-push-smoke.txt')
    await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")
    await set('#commit-message', 'Commit and push review')
    await click('.commit-form', 'Commit and Push…')
    await until(() => git(repo, 'log', '-1', '--format=%s') === 'Commit and push review', 'local commit created')
    assert.equal(git(dailyRemote, 'for-each-ref', '--format=%(refname)', 'refs/heads'), '')
    await until(() => evaluate("[...document.querySelectorAll('[aria-label=\"Remote synchronization\"] select option')].some(option => option.value === 'm2-smoke')"), 'local test remote listed')
    await set('[aria-label="Remote synchronization"] select', 'm2-smoke')
    await click('[aria-label="Remote synchronization"]', 'Push…')
    await text('[aria-label="Confirm push"]', 'm2-smoke/' + dailyBranch)
    await until(() => evaluate(`document.querySelector('dialog[aria-label="Confirm push"]:modal')?.contains(document.activeElement) &&
      document.activeElement.textContent === 'Cancel'`), 'modal push review focuses Cancel')
    await cdp('Input.dispatchKeyEvent', { type: 'rawKeyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
    await cdp('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
    await until(() => evaluate(`!document.querySelector('dialog[aria-label="Confirm push"]')`), 'Escape cancels the modal')
    assert.equal(git(dailyRemote, 'for-each-ref', '--format=%(refname)', 'refs/heads'), '')
    await click('[aria-label="Remote synchronization"]', 'Push…')
    // The review lists the outgoing commits (IDEA push dialog) before Confirm is enabled.
    await until(() => evaluate(`[...document.querySelectorAll('[aria-label="Outgoing commits"] li')]
      .some(item => item.innerText.includes('Commit and push review'))`), 'push review lists the outgoing commit')
    await text('[data-outgoing-summary]', 'this creates m2-smoke/' + dailyBranch)
    await click('[aria-label="Confirm push"]', 'Confirm push')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Push complete')
    assert.equal(git(repo, 'rev-parse', '--abbrev-ref', '@{upstream}'), 'm2-smoke/' + dailyBranch)

    const peer = await mkdtemp(join(dirname(dailyRemote), 'm2-peer-'))
    git(repo, 'clone', dailyRemote, peer)
    config(peer)
    await writeFile(join(peer, 'gitvela-smoke-peer.txt'), 'peer update')
    git(peer, 'add', 'gitvela-smoke-peer.txt')
    git(peer, 'commit', '-m', 'Commit from smoke peer')
    git(peer, 'push', 'origin', dailyBranch)
    await gitMenuAction('Fetch…')
    await click('[aria-label="Remote synchronization"]', 'Fetch')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Fetched m2-smoke.')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Commit and push review')
    await gitMenuAction('Pull…')
    // Pull preselects the upstream; the default mode is a plain merge (IDEA), which fast-forwards here.
    await click('[aria-label="Remote synchronization"]', 'Pull')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Pull complete')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Commit from smoke peer')
  }
  if (testEditor) {
    const file = 'greetservice.go'
    const target = join(repo, file)
    const git = (...args) => execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8' }).trim()
    const editContent = "package main\n\n// Edited in GitVela\n"
    const original = await readFile(target, 'utf8')
    const keyboard = async (key, code, windowsVirtualKeyCode, modifiers) => {
      for (const type of ['keyDown', 'keyUp']) {
        await cdp('Input.dispatchKeyEvent', { type, key, code, windowsVirtualKeyCode, modifiers })
      }
    }
    const editorStatus = (text) => {
      // Report the last observed status so a hang shows whether the editor
      // stayed in another state or the whole page went unresponsive.
      let last = 'unavailable'
      return until(async () => {
        last = await evaluate(`document.querySelector('.statusbar .editor-meta [role=status]')?.innerText ?? 'missing'`)
        return typeof last === 'string' && last.includes(text)
      }, 'editor status ' + JSON.stringify(text))
        .catch(error => { throw new Error(error.message + ' (last observed: ' + last + ')') })
    }
    const enterText = async (text) => {
      await evaluate("document.querySelector('.editor-host .cm-content').focus()")
      await keyboard('a', 'KeyA', 65, 2)
      await cdp('Input.insertText', { text })
    }
    const selectFile = async (path) => {
      await inputValue('[aria-label="Filter repository files"]', path)
      await until(() => evaluate(`(() => {
        const file = [...document.querySelectorAll('.repository-files .repository-file-row')]
          .find(row => row.title === ${JSON.stringify(path)})
        if (!file) return false
        file.click()
        return true
      })()`), 'repository file ' + path)
    }

    await evaluate("document.querySelector('.activity-bar button[aria-label=Files]').click()")
    await selectFile(file)
    await editorStatus('Saved')
    await until(() => evaluate(`(() => {
      const trigger = document.querySelector('.source-actions-trigger')
      if (!trigger) return false
      trigger.click()
      return true
    })()`), 'editor actions trigger')
    await clickButton('.editor-tools', 'Find / Replace')
    await until(() => evaluate("!!document.querySelector('.editor-host .cm-search')"), 'find and replace panel')
    await keyboard('Escape', 'Escape', 27, 0)
    await enterText(editContent)
    await editorStatus('Unsaved changes')
    await keyboard('z', 'KeyZ', 90, 2)
    await editorStatus('Saved')
    await enterText(editContent)
    await keyboard('s', 'KeyS', 83, 2)
    await editorStatus('Saved')
    const savedContent = original.includes('\r\n') ? editContent.replaceAll('\n', '\r\n') : editContent
    assert.equal(await readFile(target, 'utf8'), savedContent)
    assert.equal(git('diff', '--cached', '--name-only', '--', file), '')

    await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")
    await until(() => evaluate(`(() => {
      const row = document.querySelector('.working-row[title="greetservice.go"]')
      if (!row) return false
      row.click()
      return true
    })()`), 'edited file in Working Changes')
    await showWorkingDiff('unstaged', 'greetservice.go')
    await until(() => evaluate("document.querySelector('.patch')?.textContent.includes('Edited in GitVela')"), 'saved edit in Git diff')
    await clickButton('.index-actions', 'Stage')
    await until(() => git('diff', '--cached', '--name-only', '--', file) === file, 'edited file staged')

    // Re-activate the open editor tab and verify unsaved-draft protection.
    await until(() => evaluate(`(() => {
      const tab = [...document.querySelectorAll('.preview-tabs .editor-tab')]
        .find(item => item.title.endsWith('greetservice.go'))
      if (!tab) return false
      tab.click()
      return true
    })()`), 'the open editor tab')
    await enterText('unsaved draft')
    await editorStatus('Unsaved changes')
    await writeFile(target, 'external update')
    await until(() => evaluate(`(() => {
      const trigger = document.querySelector('.source-actions-trigger')
      if (!trigger) return false
      trigger.click()
      return true
    })()`), 'editor actions trigger')
    await clickButton('.editor-tools', 'Save')
    await until(() => evaluate("document.querySelector('.preview-panel [role=alert]')?.innerText.includes('changed on disk')"), 'external change blocked save')
    assert.equal(await readFile(target, 'utf8'), 'external update')

    await evaluate("document.querySelector('.activity-bar button[aria-label=Files]').click()")
    await selectFile('README.md')
    await until(() => evaluate("document.querySelector('.source-preview-tabs .editor-tab.active')?.title === 'README.md'"), 'second source tab opens')
    await evaluate("document.querySelector('.editor-host .cm-content[aria-label=\"Edit README.md\"]').focus()")
    await keyboard('a', 'KeyA', 65, 2)
    await cdp('Input.insertText', { text: 'second unsaved draft' })
    await evaluate("document.querySelector('.source-preview-tabs [aria-label=\"Close greetservice.go\"]').click()")
    await until(() => evaluate("document.querySelector('dialog[open] li')?.innerText === 'greetservice.go'"), 'confirmation names the inactive tab')
    await clickButton('[aria-label="Unsaved file changes"]', 'Keep editing')
    assert.ok(await evaluate("!!document.querySelector('.source-preview-tabs [aria-label=\"Close greetservice.go\"]')"))
    await evaluate("document.querySelector('.source-preview-tabs [aria-label=\"Close greetservice.go\"]').click()")
    await clickButton('[aria-label="Unsaved file changes"]', 'Discard changes')
    assert.equal(await evaluate("document.querySelector('.editor-host .cm-content')?.textContent"), 'second unsaved draft')
    await evaluate("document.querySelector('.source-preview-tabs .editor-tab.active .editor-close').click()")
    await clickButton('[aria-label="Unsaved file changes"]', 'Discard changes')
    await until(() => evaluate("!document.querySelector('.workspace-editor')"), 'both editor tabs closed after confirmation')
    git('restore', '--source=HEAD', '--staged', '--worktree', '--', file)
    assert.equal(await readFile(target, 'utf8'), original)
  }
  if (testAdvanced) {
    const git = (...args) => execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8' }).trim()
    const text = (selector, value) => until(() => evaluate(
      `document.querySelector(${JSON.stringify(selector)})?.innerText.includes(${JSON.stringify(value)})`), value)
    const target = join(repo, 'm3.txt')
    git('config', 'user.name', 'GitVela Smoke')
    git('config', 'user.email', 'smoke@example.invalid')
    git('config', 'commit.gpgsign', 'false')
    git('config', 'core.autocrlf', 'false')
    git('switch', '-c', 'smoke-m3')
    await writeFile(target, 'base\n')
    git('add', 'm3.txt')
    git('commit', '-m', 'M3 base')
    git('branch', 'smoke-m3-base')
    await writeFile(join(repo, 'm3-one.txt'), 'one\n')
    git('add', 'm3-one.txt')
    git('commit', '-m', 'M3 first')
    await writeFile(join(repo, 'm3-two.txt'), 'two\n')
    git('add', 'm3-two.txt')
    git('commit', '-m', 'M3 second')
    await text('.statusbar', ' · smoke-m3')
    await gitMenuAction('Branches')
    await branchMenuAction('smoke-m3-base', 'Interactive rebase onto this…')
    await text('.rebase-plan', 'M3 second')
    await evaluate("document.querySelector('[aria-label=\"Move M3 second earlier\"]').click()")
    await clickButton('[aria-label="Confirm Rebase"]', 'Confirm Rebase')
    await text('.advanced-panel [role=status]', 'Rebase complete.')
    assert.equal(git('log', '-2', '--format=%s'), 'M3 first\nM3 second')

    git('switch', '-c', 'smoke-m3-side', 'smoke-m3-base')
    await writeFile(target, 'side\n')
    git('add', 'm3.txt')
    git('commit', '-m', 'M3 side')
    git('switch', 'smoke-m3')
    await writeFile(target, 'main\n')
    git('add', 'm3.txt')
    git('commit', '-m', 'M3 main')
    const sideHash = git('rev-parse', 'smoke-m3-side')
    const mainHash = git('rev-parse', 'HEAD')
    await until(() => evaluate(`(() => {
      const rows = [...document.querySelectorAll('.commit-row')]
      const hasRef = (hash, ref) => rows.some(row => row.querySelector('code')?.title === hash &&
        [...row.querySelectorAll('.chip')].some(chip => chip.title === ref))
      return hasRef(${JSON.stringify(mainHash)}, 'HEAD') && hasRef(${JSON.stringify(sideHash)}, 'refs/heads/smoke-m3-side')
    })()`), 'both comparison refs reflect the final fixture commits')
    await branchMenuAction('smoke-m3-side', 'Compare with HEAD')
    await text('.files-panel .commit-summary', 'HEAD → smoke-m3-side')
    await until(() => evaluate(`(() => {
      const file = document.querySelector('.files-panel .file-row[title="m3.txt"]')
      if (!file) return false
      file.click()
      return true
    })()`), 'file in branch comparison')
    try {
      await until(() => evaluate("document.querySelector('.cm-merge-b .cm-content')?.textContent.includes('side')"), 'compared branch contents')
    } catch (error) {
      console.error(await evaluate("document.querySelector('.preview-panel')?.innerText"))
      throw error
    }
    await evaluate(`[...document.querySelectorAll('.commit-row')].find(row => row.innerText.includes('M3 side'))
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))`)
    await clickButton('[aria-label="Commit actions"]', 'Cherry-pick…')
    try {
      await clickButton('[aria-label="Confirm Cherry-pick"]', 'Confirm Cherry-pick')
    } catch (error) {
      console.error(await evaluate("document.querySelector('.advanced-panel')?.innerText || document.querySelector('.statusbar')?.innerText"))
      throw error
    }
    await text('.advanced-panel', '1 unmerged')
    await until(() => evaluate(`(() => {
      const row = document.querySelector('.working-row[title="m3.txt"]')
      if (!row) return false
      row.click()
      return true
    })()`), 'conflicted file in Changes')
    await clickButton('.preview-tabs', 'Resolve conflict…')
    await text('.conflict-grid', 'Theirs (stage 3)')
    await until(() => evaluate("!!document.querySelector('.editor-host .cm-content')"), 'resolution editor')
    await writeFile(target, 'resolved\n')
    await until(() => evaluate(`(() => {
      const trigger = document.querySelector('.source-actions-trigger')
      if (!trigger) return false
      trigger.click()
      return true
    })()`), 'editor actions trigger')
    await clickButton('.editor-tools', 'Reload')
    await until(() => evaluate("document.querySelector('.editor-host .cm-content')?.textContent.includes('resolved')"), 'resolution reloaded')
    await clickButton('.conflict-panel', 'Mark resolved')
    await text('.advanced-panel', '0 unmerged')
    await clickButton('.lower-console', 'Continue')
    await clickButton('dialog[aria-label="Confirm continue"]', 'Confirm continue')
    await text('.advanced-panel [role=status]', 'Git operation continued.')
    assert.equal(git('show', 'HEAD:m3.txt'), 'resolved')

    await writeFile(target, 'pending change\n')
    await evaluate("document.querySelector('.activity-bar button[aria-label=Files]').click()")
    await until(() => evaluate("!!document.querySelector('.repository-files .folder-row[aria-expanded]')"), 'default repository tree')
    await inputValue('[aria-label="Filter repository files"]', 'm3.txt')
    await clickButton('.repository-files .file-scroll', 'm3.txt')
    await clickButton('.preview-tabs', 'File History')
    await text('.file-history-row', 'M3 side')
    await clickButton('.file-inspection', 'Annotate with Git Blame')
    await text('.cm-git-blame', 'Uncommitted')
    await clickButton('.file-inspection', 'Close')

    await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")
    await until(() => evaluate(`(() => {
      const row = document.querySelector('.working-row[title="m3.txt"]')
      if (!row) return false
      row.click()
      return true
    })()`), 'edited file in Changes')
    await showWorkingDiff('unstaged', 'm3.txt')
    await clickButton('.preview-tabs', 'Hunks')
    await clickButton('.hunk-panel', 'Stage hunk')
    await text('.hunk-panel [role=status]', 'Selection staged.')
    assert.equal(git('show', ':m3.txt'), 'pending change')
    await showWorkingDiff('staged', 'm3.txt')
    await clickButton('.preview-tabs', 'Hunks')
    await clickButton('.hunk-panel', 'Unstage hunk')
    await text('.hunk-panel [role=status]', 'Index selection unstaged.')
    assert.equal(git('show', ':m3.txt'), 'resolved')
    git('restore', '--', 'm3.txt')
  }
  if (testProject) {
    const project = await mkdtemp(join(tmpdir(), 'gitvela-project-'))
    const git = (...args) => execFileSync('git', ['-C', project, ...args], { encoding: 'utf8' }).trim()
    await writeFile(join(project, 'project.txt'), 'first project file')
    await gitMenuAction('Initialize Project…')
    await inputValue('[aria-label="Initialize project"] input', project)
    await clickButton('[aria-label="Initialize project"]', 'Initialize Git')
    await until(() => evaluate("document.querySelector('.working-row[title=\"project.txt\"]') !== null"), 'new project working file')
    assert.equal(git('branch', '--show-current'), 'main')
    git('config', 'user.name', 'GitVela Smoke')
    git('config', 'user.email', 'smoke@example.invalid')
    git('config', 'commit.gpgsign', 'false')
    git('config', 'core.autocrlf', 'false')
    await clickButton('.index-actions', 'Stage all')
    await inputValue('#commit-message', 'Initial project commit')
    await gitMenuAction('Commit…')
    await clickButton('.commit-form', 'Commit')
    await until(() => evaluate("document.querySelector('.commit-row')?.innerText.includes('Initial project commit')"), 'first project commit')

    await gitMenuAction('Manage Remotes…')
    await clickButton('[aria-label="Manage remotes"]', 'Add remote…')
    await inputValue('[aria-label="Manage remotes"] input[placeholder=origin]', 'example')
    await inputValue('[aria-label="Manage remotes"] input[placeholder^=https]', 'https://example.invalid/project.git')
    await clickButton('[aria-label="Manage remotes"]', 'Add')
    await until(() => evaluate("document.querySelector('.remote-row')?.innerText.includes('example.invalid/project.git')"), 'remote added')
    await clickButton('.remote-row', 'Edit URL…')
    await inputValue('[aria-label="Manage remotes"] input[placeholder^=https]', 'https://example.invalid/updated.git')
    await clickButton('[aria-label="Manage remotes"]', 'Save URL')
    await until(() => evaluate("document.querySelector('.remote-row')?.innerText.includes('example.invalid/updated.git')"), 'remote edited')
    await clickButton('.remote-row', 'Remove…')
    await clickButton('[aria-label="Confirm remote removal"]', 'Remove remote')
    await until(() => evaluate("document.querySelector('[aria-label=\"Manage remotes\"]').innerText.includes('No configured remotes.')"), 'remote removed')
    assert.equal(git('remote'), '')
    await clickButton('[aria-label="Git Remotes"]', 'Done')

    await gitMenuAction('Publish to GitHub…')
    assert.equal(await evaluate("document.querySelector('[aria-label=\"Publish to GitHub\"] select').selectedIndex"), 0)
    if (githubLogin) {
      await clickButton('[aria-label="Publish to GitHub"]', 'Check GitHub account')
      await until(() => evaluate(`document.querySelector('[aria-label="Publish to GitHub"]')?.innerText.includes(${JSON.stringify('Signed in as ' + githubLogin)})`), 'authenticated GitHub account')
      await clickButton('[aria-label="Publish to GitHub"]', 'Review publication')
      assert.match(await evaluate("document.querySelector('[aria-label=\"Confirm GitHub publication\"]').innerText"), /as private and push.*main/)
      await clickButton('[aria-label="Confirm GitHub publication"]', 'Cancel')
    }
    assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"), [])
  }
  if (testM4) {
    const openContext = async (selector, label) => {
      await until(() => evaluate(`(() => {
        const trigger = document.querySelector(${JSON.stringify(selector)})
        if (!trigger) return false
        trigger.dispatchEvent(new MouseEvent('contextmenu', {
          bubbles: true, cancelable: true, clientX: innerWidth - 2, clientY: innerHeight - 2,
        }))
        return true
      })()`), label + ' trigger')
      const menu = '[aria-label="' + label + '"]'
      await until(() => evaluate(`(() => {
        const popup = document.querySelector(${JSON.stringify(menu)})
        if (!popup?.classList.contains('context-actions')) return false
        const rect = popup.getBoundingClientRect()
        return rect.right <= innerWidth && rect.bottom <= innerHeight && popup.contains(document.activeElement)
      })()`), label + ' popup inside the viewport')
      return menu
    }
    const dismissContext = async menu => {
      await evaluate("document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))")
      await until(() => evaluate(`!document.querySelector(${JSON.stringify(menu)})`), 'context menu dismissed by Escape')
    }
    await dismissContext(await openContext('.commit-row.active', 'Commit actions'))
    const changedMenu = await openContext('.files-panel .file-row:not(.folder-row)', 'Changed file actions')
    await clickButton(changedMenu, 'Show Diff')
    await gitMenuAction('Branches')
    await dismissContext(await openContext('.branch-row .side-item.active', 'Branch actions'))
    await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")

    await clickButton('.commit-tabs', 'Shelf')
    await until(() => evaluate("document.querySelector('.workspace-sidebar [aria-label=Stashes]')?.innerText.includes('Save stash')"), 'Shelf in the Commit workspace')
    await clickButton('.commit-tabs', 'Commit')

    // The configured Ctrl+K shortcut opens the command palette.
    await evaluate("document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))")
    await until(() => evaluate("!!document.querySelector('.palette')"), 'command palette')
    await inputValue('[aria-label="Search commands"]', 'compact')
    await until(() => evaluate("document.querySelector('.palette-item.active')?.innerText.includes('compact')"), 'ranked compact-row command')
    await evaluate("document.querySelector('.palette').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))")
    await until(() => evaluate("document.querySelector('.shell').dataset.density === 'compact'"), 'compact row density')
    assert.equal(await evaluate("getComputedStyle(document.querySelector('.commit-row')).height"), '27px')

    // Restore the comfortable layout, then rebind a shortcut through Settings.
    await evaluate("document.querySelector('.topbar button[aria-label=\"Settings\"]').click()")
    await evaluate("[...document.querySelectorAll('.settings-dialog label')].find(label => label.innerText.includes('Compact rows')).querySelector('input').click()")
    await until(() => evaluate("document.querySelector('.shell').dataset.density === 'comfortable'"), 'comfortable row density')
    await settingsTab('Keymap')
    await until(() => evaluate("document.querySelectorAll('.shortcut-group button').length >= 16"), 'editable shortcut list')
    await evaluate(`(() => {
      const button = document.querySelector('[aria-label="Change shortcut for Command palette"]')
      button.click()
      button.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', ctrlKey: true, shiftKey: true, bubbles: true }))
    })()`)
    await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.keymap') || '{}').palette === 'Ctrl+Shift+P'"), 'persisted custom shortcut')
    await clickButton('.shortcut-settings', 'Reset to defaults')
    await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.keymap') || '{}').palette === 'Ctrl+K'"), 'default shortcut restored')

    // Tab on a shortcut button moves focus instead of binding the Tab key.
    await evaluate(`(() => {
      const button = document.querySelector('[aria-label="Change shortcut for Command palette"]')
      button.focus()
      button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    })()`)
    await until(() => evaluate("(() => { const map = JSON.parse(localStorage.getItem('gitvela.keymap') || '{}'); return !Object.values(map).includes('Tab') && !document.querySelector('.shortcut-settings .recording') })()"), 'Tab moves focus without becoming a shortcut')
    await clickButton('.settings-dialog .dialog-actions', 'Close')
    await until(() => evaluate("!document.querySelector('.settings-dialog')"), 'settings dialog closed for repository interactions')

    // Use a fresh repository with a genuinely committable index and message.
    const tempRoot = resolve(tmpdir())
    shortcutRepo = await mkdtemp(join(tempRoot, 'gitvela-shortcuts-'))
    const git = (...args) => execFileSync('git', ['-C', shortcutRepo, ...args], { encoding: 'utf8' }).trim()
    git('init', '-b', 'main')
    git('config', 'user.name', 'GitVela Smoke')
    git('config', 'user.email', 'smoke@example.invalid')
    git('config', 'commit.gpgsign', 'false')
    git('config', 'core.autocrlf', 'false')
    git('config', 'core.hooksPath', join(shortcutRepo, '.git', 'no-hooks'))
    const file = 'shortcut-test.txt'
    await writeFile(join(shortcutRepo, file), 'original\n')
    git('add', file)
    git('commit', '-m', 'Shortcut fixture base')
    const base = git('rev-parse', 'HEAD')
    await writeFile(join(shortcutRepo, 'history.txt'), 'history fixture\n')
    git('add', 'history.txt')
    git('commit', '-m', 'Shortcut fixture latest')
    const head = git('rev-parse', 'HEAD')
    await writeFile(join(shortcutRepo, file), 'staged shortcut fixture\n')
    git('add', file)
    await writeFile(join(shortcutRepo, file), 'large working change\n'.repeat(22000))
    assert.equal(git('diff', '--cached', '--name-only'), file)
    await inputValue('[aria-label="Repository path"]', shortcutRepo)
    await evaluate(`document.querySelector('[aria-label="Repository path"]').form.requestSubmit()`)
    await until(() => evaluate(`document.querySelector('.working-row[title="shortcut-test.txt"]')`), 'temporary staged file')
    const workingMenu = await openContext('.working-row[title="shortcut-test.txt"]', 'Working file actions')
    await clickButton(workingMenu, 'Show Staged Diff')
    await until(() => evaluate("document.querySelector('.patch')?.textContent.includes('+staged shortcut fixture')"), 'staged diff from working file context menu')
    await mkdir(join(shortcutRepo, 'unchecked'))
    await writeFile(join(shortcutRepo, 'unchecked', 'keep.txt'), 'leave this out\n')
    const toggleCheck = async (label, checked) => {
      await until(() => evaluate(`(() => {
        const box = document.querySelector(${JSON.stringify('input[aria-label="' + label + '"]')})
        if (!box || box.disabled || box.checked === ${checked}) return false
        box.click()
        return true
      })()`), label + ' checkbox')
    }
    await toggleCheck('Include folder unchecked in commit', true)
    await until(() => git('diff', '--cached', '--name-only').includes('unchecked/keep.txt'), 'folder contents staged')
    await toggleCheck('Include unchecked/keep.txt in commit', false)
    await until(() => git('diff', '--cached', '--name-only') === file, 'unchecked file removed from index')
    await until(() => evaluate(`document.querySelector('input[aria-label="Include all changes in commit"]').indeterminate`), 'mixed root checkbox')
    await toggleCheck('Include all changes in commit', true)
    await until(() => git('diff', '--cached', '--name-only').includes('unchecked/keep.txt'), 'remaining file staged by root checkbox')
    assert.equal(git('show', ':' + file), 'staged shortcut fixture', 'root selection preserves partial staging')
    await toggleCheck('Include unchecked/keep.txt in commit', false)
    await until(() => git('diff', '--cached', '--name-only') === file, 'only checked file left staged')
    const bindCommit = async (binding, key, modifiers = {}) => {
      await evaluate(`(() => {
        const button = document.querySelector('[aria-label="Change shortcut for Focus commit message"]')
        button.click()
        button.dispatchEvent(new KeyboardEvent('keydown', ${JSON.stringify({ key, ...modifiers, bubbles: true })}))
      })()`)
      await until(() => evaluate(`JSON.parse(localStorage.getItem('gitvela.keymap')).commit === ${JSON.stringify(binding)}`), 'commit binding ' + binding)
    }
    const pressCommitField = (key, modifiers = {}) => evaluate(`(() => {
      const field = document.getElementById('commit-message')
      field.focus()
      return field.dispatchEvent(new KeyboardEvent('keydown', ${JSON.stringify({ key, ...modifiers, bubbles: true, cancelable: true })}))
    })()`)
    // Bind every chord in one settings session, then close the dialog: the
    // modal traps focus, so the commit field interactions below need it gone.
    await evaluate("document.querySelector('.topbar button[aria-label=Settings]').click()")
    await until(() => evaluate("!!document.querySelector('.settings-dialog[open]')"), 'settings dialog reopened for shortcuts')
    await settingsTab('Keymap')
    await bindCommit('A', 'a')
    await bindCommit('Enter', 'Enter')
    await bindCommit('Shift+A', 'A', { shiftKey: true })
    await bindCommit('Ctrl+Shift+S', 's', { ctrlKey: true, shiftKey: true })
    await clickButton('.settings-dialog .dialog-actions', 'Close')
    await until(() => evaluate("!document.querySelector('.settings-dialog')"), 'settings dialog closed for page interactions')
    const message = 'Shortcut integration commit'
    await inputValue('#commit-message', message)
    await until(() => evaluate("!!document.querySelector('#commit-message')?.form.querySelector('button[type=submit]:enabled')"), 'enabled commit')
    for (const [binding, key, code, typed, modifiers] of [
      ['A', 'a', 'KeyA', 'a', 0],
      ['Enter', 'Enter', 'Enter', '\n', 0],
      ['Shift+A', 'A', 'KeyA', 'A', 8],
    ]) {
      assert.equal(await pressCommitField(key, { shiftKey: modifiers === 8 }), true, binding + ' must keep normal text input behavior')
      await evaluate("document.getElementById('commit-message').setSelectionRange(1000, 1000)")
      await cdp('Input.dispatchKeyEvent', { type: 'keyDown', key, code, text: key === 'Enter' ? '\r' : typed, modifiers })
      await cdp('Input.dispatchKeyEvent', { type: 'keyUp', key, code, modifiers })
      await until(() => evaluate(`document.getElementById('commit-message').value === ${JSON.stringify(message + typed)}`), binding + ' inserts text')
      assert.equal(git('rev-parse', 'HEAD'), head)
      assert.equal(git('diff', '--cached', '--name-only'), file)
      await inputValue('#commit-message', message)
    }
    assert.equal(await pressCommitField('Enter', { ctrlKey: true }), true, 'old binding must not submit')
    assert.equal(git('rev-parse', 'HEAD'), head)
    assert.equal(await pressCommitField('s', { ctrlKey: true, shiftKey: true }), false, 'configured chord must dispatch')
    await until(() => git('rev-parse', 'HEAD') !== head, 'real commit from configured chord')
    assert.equal(git('log', '-1', '--format=%s'), 'Shortcut integration commit')
    assert.equal(git('diff-tree', '--no-commit-id', '--name-only', '-r', 'HEAD'), file)
    assert.equal(git('diff', '--cached', '--name-only'), '')
    // Reopen Settings to restore the default shortcuts.
    await evaluate("document.querySelector('.topbar button[aria-label=Settings]').click()")
    await until(() => evaluate("!!document.querySelector('.settings-dialog[open]')"), 'settings dialog reopened')
    await settingsTab('Keymap')
    await clickButton('.shortcut-settings', 'Reset to defaults')
    await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.keymap') || '{}').palette === 'Ctrl+K'"), 'default shortcut restored')
    await evaluate("document.querySelector('.settings-dialog .dialog-actions button').click()")
    await until(() => evaluate("!document.querySelector('.settings-dialog')"), 'settings dialog closed after shortcut reset')

    // Reload the WebView and inspect actual session writes, including any
    // intermediate state that might otherwise be overwritten by the last save.
    const restoreWorkingDiff = async (view, tab, historyFile, range) => {
      const session = {
        path: shortcutRepo, hash: base, file: historyFile, workingFile: file,
        sidebar: 'changes', view, historyVisible: false, range, smokeMarker: true,
      }
      const writes = []
      // Sessions are stored per workspace root; the injected legacy key is
      // migrated into that map by the next save.
      const monitor = ({ data }) => {
        const { method, params } = JSON.parse(data)
        if (!/^DOMStorage\.domStorageItem(Updated|Added)$/.test(method)) return
        if (params.key !== 'gitvela.sessions' && params.key !== 'gitvela.session') return
        const value = JSON.parse(params.newValue ?? 'null')
        const saved = params.key === 'gitvela.sessions' ? value?.byPath?.[session.path] : value
        if (saved && !saved.smokeMarker) writes.push(saved)
      }
      socket.addEventListener('message', monitor)
      try {
        await evaluate(`(() => {
          localStorage.setItem('gitvela.session', ${JSON.stringify(JSON.stringify(session))})
          location.reload()
        })()`)
        await until(() => evaluate(`(() => {
          const tab = document.querySelector('.preview-tabs .editor-tab.active')
          return tab?.title === 'shortcut-test.txt' && tab.innerText.includes(${JSON.stringify(view)})
        })()`), 'restored working file and ' + view + ' tab')
        await until(() => writes.length > 0, 'restored session saved')
        for (const saved of writes) {
          assert.equal(saved.hash, base)
          assert.equal(saved.file, historyFile)
          assert.equal(saved.workingFile, file)
          assert.equal(saved.view, view)
          assert.equal(saved.historyVisible, false)
          assert.deepEqual(saved.range, range)
        }
      } finally {
        socket.removeEventListener('message', monitor)
      }
    }

    assert.deepEqual(await evaluate('window.smokeRejections'), [])
    await cdp('DOMStorage.enable', {})
    await restoreWorkingDiff('unstaged', 'Unstaged diff', file)
    await clickButton('.preview-panel', 'Load full preview (up to 8 MiB)')
    await until(() => evaluate("document.querySelector('.preview-panel').innerText.includes('too large to render line by line')"), 'large working diff fallback')
    assert.equal(await evaluate("document.querySelectorAll('.patch span').length"), 0)

    git('add', file)
    await restoreWorkingDiff('staged', 'Staged diff', 'history.txt', {
      before: base, after: head, label: 'Saved comparison',
    })

    // Session state is persisted after opening a repository.
    await until(() => evaluate("(() => { const index = JSON.parse(localStorage.getItem('gitvela.sessions') || 'null'); const saved = index?.byPath?.[index.last]; return !!saved && typeof saved.path === 'string' && saved.path.length > 0 && localStorage.getItem('gitvela.session') === null })()"), 'saved session')
    assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"), [])
  }
  if (!testM4) assert.deepEqual(await evaluate("window.smokeRejections"), [])
  if (screenshot) {
    const image = await cdp('Page.captureScreenshot', { format: 'png' })
    await writeFile(screenshot, Buffer.from(image.data, 'base64'))
  }
  console.log('Desktop smoke passed: Go bindings, repository, history canvas, changed files, and read-only Diff.')
  if (testAdvanced) console.log('Tree, history, blame, comparison, interactive rebase, conflict resolution, and hunk staging passed.')
  if (testEditor) console.log('File editing, undo, search, save, staging, and external-change protection passed.')
  if (testProject) console.log('Project initialization, first commit, remote management, and GitHub publication controls passed.')
  if (dailyRemote) console.log('Amend, stash save and pop, and remote push, fetch, and pull passed.')
  if (worktreeTarget) console.log('Worktree creation, switching, and removal passed.')
  if (testMvp) console.log('History search, commit actions, settings, file tree, and diff maximization passed.')
  if (testM4) console.log('Command palette, configurable shortcuts, compact rows, and session persistence passed.')
} finally {
  const tempRoot = resolve(tmpdir())
  try {
    if (shortcutRepo && resolve(shortcutRepo).startsWith(tempRoot + sep + 'gitvela-shortcuts-')) {
      // Switch away so the Windows repository watcher releases its handles.
      await inputValue('[aria-label="Repository path"]', repo)
      await evaluate(`document.querySelector('[aria-label="Repository path"]').form.requestSubmit()`)
      await rm(shortcutRepo, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 })
    }
  } catch (error) {
    console.warn('Could not remove temporary shortcut fixture:', error.message)
  } finally {
    socket.close()
  }
}
