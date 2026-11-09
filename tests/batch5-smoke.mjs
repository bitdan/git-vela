// Batch-5 feature smoke (G12 amend options, G13 bisect, cleanup, partial
// stash, editor preferences, fetch --all, workspace roots) against a running
// `wails3 dev -tags desktop_smoke` instance. Usage:
//   node tests/batch5-smoke.mjs
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const keepRepo = process.env.GITVELA_BATCH5_KEEP === '1'

async function until(action, description) {
  const deadline = Date.now() + 45000
  let last
  while (Date.now() < deadline) {
    const result = await Promise.resolve().then(action).catch(error => { last = error; return undefined })
    if (result) return result
    await delay(120)
  }
  throw new Error('Timed out waiting for ' + description + (last ? ': ' + last.message : ''))
}

/* ---- Fixture: a repository whose history contains one broken commit ---- */

const base = await mkdtemp(join(tmpdir(), 'gitvela-batch5-'))
const repo = join(base, 'repo')
const other = join(base, 'other')
const backup = join(base, 'backup.git')
const git = (dir, ...args) => execFileSync('git', ['-C', dir, ...args], { stdio: 'pipe' }).toString()

await mkdir(repo)
git(repo, 'init', '-b', 'main')
git(repo, 'config', 'user.email', 'smoke@example.com')
git(repo, 'config', 'user.name', 'Smoke')

async function commitFile(name, content, message) {
  await writeFile(join(repo, name), content)
  git(repo, 'add', '-A')
  git(repo, 'commit', '-m', message)
}
await commitFile('base.txt', 'alpha\nbeta\ngamma\n', 'base commit')
const rootCommit = git(repo, 'rev-parse', 'HEAD').trim()
await commitFile('second.txt', 'second\n', 'second commit')
await commitFile('bug.txt', 'broken\n', 'break it')
const brokenCommit = git(repo, 'rev-parse', 'HEAD').trim()
git(repo, 'commit', '--allow-empty', '-m', 'after break')
await commitFile('tip.txt', 'tip\n', 'tip commit')
const tipCommit = git(repo, 'rev-parse', 'HEAD').trim()

// A second repository: switching workspace roots reopens it.
await mkdir(other)
git(other, 'init', '-b', 'main')
git(other, 'config', 'user.email', 'smoke@example.com')
git(other, 'config', 'user.name', 'Smoke')
await writeFile(join(other, 'other.txt'), 'other\n')
git(other, 'add', '-A')
git(other, 'commit', '-m', 'other root commit')

/* ---- CDP plumbing (same pattern as batch4-smoke.mjs) ---- */

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
    .find(item => item.innerText.trim() === ${JSON.stringify(label)})
  if (!button || button.disabled) return false
  button.click()
  return true
})()`), 'enabled button ' + label)

const inputValue = (selector, value) => until(() => evaluate(`(() => {
  const input = document.querySelector(${JSON.stringify(selector)})
  if (!input || input.disabled) return false
  input.value = ${JSON.stringify(value)}
  input.dispatchEvent(new Event('input', { bubbles: true }))
  return true
})()`), 'input ' + selector)

const gitMenuAction = label => until(() => evaluate(`(() => {
  const menu = document.querySelector('.git-menu')
  if (!menu) return false
  if (!menu.open) menu.querySelector('summary').click()
  const button = [...menu.querySelectorAll('button')].find(item => item.innerText === ${JSON.stringify(label)})
  if (!button || button.disabled) return false
  button.click()
  return true
})()`), 'Git menu action ' + label)

const workingRow = name => `[...document.querySelectorAll('.working-row')]
  .find(row => row.innerText.includes(${JSON.stringify(name)}))`

const openRepository = async path => {
  await evaluate(`(() => {
    const input = document.querySelector('[aria-label="Repository path"]')
    input.value = ${JSON.stringify(path)}
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form.requestSubmit()
  })()`)
}

try {
  await evaluate('localStorage.clear()')
  await cdp('Page.navigate', { url: 'http://wails.localhost:9245/' })
  await until(() => evaluate("!!document.querySelector('.statusbar')"), 'the app to reload')
  await openRepository(repo)
  await until(() => evaluate("!!document.querySelector('.commit-row')"), 'the history of the fixture')

  /* ---- Editor preferences: soft wrap, font size, tab width ---- */

  await evaluate("document.querySelector('.topbar button[aria-label=Settings]').click()")
  await until(() => evaluate("!!document.querySelector('dialog[open]')"), 'the settings dialog')
  // Sections are tabs now, so the editor options have their own page.
  await until(() => evaluate(`(() => {
    const button = [...document.querySelectorAll('.settings-tabs [role=tab]')]
      .find(item => item.innerText === 'Editor')
    if (!button) return false
    if (button.getAttribute('aria-selected') !== 'true') button.click()
    return !!document.querySelector('[aria-label="Editor font size in pixels"]')
  })()`), 'the editor settings tab')
  await until(() => evaluate(`(() => {
    const label = [...document.querySelectorAll('dialog[open] label')]
      .find(item => item.innerText.trim() === 'Soft wrap long lines')
    const box = label?.querySelector('input[type=checkbox]')
    if (!box) return false
    if (!box.checked) box.click()
    return box.checked
  })()`), 'the soft wrap option to be enabled')
  await inputValue('dialog[open] input[aria-label="Editor font size in pixels"]', '18')
  await until(() => evaluate(`(() => {
    const select = document.querySelector('dialog[open] select[aria-label="Editor tab size"]')
    if (!select || select.value === '2') return !!select
    select.value = '2'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    return true
  })()`), 'the tab size option')
  await clickButton('dialog[open] footer', 'Close')
  await until(() => evaluate("!document.querySelector('dialog[open]')"), 'the settings dialog to close')

  const stored = JSON.parse(await evaluate(`JSON.stringify({
    wrap: localStorage.getItem('gitvela.editorWordWrap'),
    font: localStorage.getItem('gitvela.editorFontSize'),
    tab: localStorage.getItem('gitvela.editorTabSize'),
  })`))
  assert.equal(stored.wrap, 'true', 'soft wrap is persisted')
  assert.equal(stored.font, '18', 'the font size is persisted')
  assert.equal(stored.tab, '2', 'the tab width is persisted')

  await evaluate("document.querySelector('.activity-bar button[aria-label=Files]').click()")
  await until(() => evaluate("!!document.querySelector('.repository-files .repository-file-row')"), 'the file list')
  await inputValue('[aria-label="Filter repository files"]', 'base.txt')
  await clickButton('.repository-files .file-scroll', 'base.txt')
  await until(() => evaluate("!!document.querySelector('.editor-host .cm-content')"), 'the file to open in the editor')
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.editor-host .cm-content')).fontSize"), '18px',
    'the editor uses the configured font size without reloading the file')
  await until(() => evaluate("!!document.querySelector('.editor-host .cm-content.cm-lineWrapping')"),
    'the editor to wrap long lines')
  assert.equal(await evaluate(`(() => {
    const view = document.querySelector('.editor-host .cm-content')
    return getComputedStyle(view).tabSize || view.style.tabSize
  })()`), '2', 'the editor renders tabs at the configured width')

  /* ---- Go to Line ---- */

  await evaluate("document.querySelector('.source-actions-trigger').click()")
  await clickButton('.editor-tools', 'Go to Line')
  await until(() => evaluate("!!document.querySelector('.go-to-line input')"), 'the go-to-line form')
  await inputValue('.go-to-line input', '3')
  await evaluate("document.querySelector('.go-to-line').requestSubmit()")
  await until(() => evaluate(`(() => {
    if (document.querySelector('.go-to-line')) return false
    const line = document.querySelector('.editor-host .cm-activeLine')
    return !!line && line.innerText.trim() === 'gamma'
  })()`), 'the cursor to jump to line 3')
  await evaluate("document.querySelector('.source-preview-tabs .editor-tab.active .editor-close').click()")

  /* ---- Bisect: find the commit that introduced bug.txt ---- */

  await gitMenuAction('Bisect…')
  await until(() => evaluate("!!document.querySelector('[aria-label=Bisect]')"), 'the bisect dialog')
  await inputValue('[aria-label="Known good commit"]', rootCommit)
  await clickButton('[aria-label=Bisect]', 'Start bisect')
  for (let step = 0; step < 12; step++) {
    const finished = await evaluate(
      "document.querySelector('[aria-label=Bisect]')?.innerText.includes('First bad commit') ?? false")
    if (finished) break
    const buggy = existsSync(join(repo, 'bug.txt'))
    await clickButton('[aria-label=Bisect]', buggy ? 'Mark bad' : 'Mark good')
    await until(() => evaluate(`(() => {
      const panel = document.querySelector('[aria-label=Bisect]')
      if (!panel) return false
      if (panel.innerText.includes('First bad commit')) return true
      const button = [...panel.querySelectorAll('button')].find(item => item.innerText === 'Mark good')
      return !!button && !button.disabled
    })()`), 'the bisect step to finish')
  }
  const verdict = await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label=Bisect]')
    const code = panel?.querySelector('code')
    return panel?.innerText.includes('First bad commit') && code ? code.innerText.trim() : false
  })()`), 'the first bad commit')
  assert.equal(verdict, brokenCommit.slice(0, 8), 'bisect names the commit that broke the build')
  await clickButton('[aria-label=Bisect]', 'Finish bisect')
  await until(() => evaluate("!!document.querySelector('[aria-label=Bisect] input[aria-label=\"Known bad commit\"]')"),
    'the bisect dialog to return to its start state')
  assert.equal(git(repo, 'rev-parse', '--abbrev-ref', 'HEAD').trim(), 'main', 'bisect returned to the branch')
  assert.equal(git(repo, 'rev-parse', 'HEAD').trim(), tipCommit, 'bisect restored the original tip')
  await clickButton('[aria-label=Bisect]', 'Close')
  await until(() => evaluate("!document.querySelector('[aria-label=Bisect]')"), 'the bisect panel to close')

  /* ---- Amend with sign-off ---- */

  await evaluate("document.querySelector('.activity-bar button[aria-label=Commit]').click()")
  await writeFile(join(repo, 'amend.txt'), 'amended\n')
  await gitMenuAction('Refresh')
  await until(() => evaluate(`!!${workingRow('amend.txt')}`), 'the new file in the working tree')
  await until(() => evaluate(`(() => {
    const box = document.querySelector('input[aria-label="Include amend.txt in commit"]')
    if (!box || box.disabled) return false
    box.click()
    return true
  })()`), 'the new file to be staged')
  await until(() => evaluate(`(() => {
    const box = document.querySelector('input[aria-label="Amend last commit"]')
    if (!box || box.disabled || box.checked) return false
    box.click()
    return true
  })()`), 'the amend mode to start')
  await until(() => evaluate("!!document.querySelector('.amend-meta')"), 'the amend author details')
  await until(() => evaluate(`(() => {
    const label = [...document.querySelectorAll('.amend-meta label')]
      .find(item => item.innerText.trim() === 'Sign off')
    const box = label?.querySelector('input[type=checkbox]')
    if (!box) return false
    if (!box.checked) box.click()
    return box.checked
  })()`), 'the sign-off option to be enabled')
  await clickButton('.commit-form', 'Amend commit')
  await until(() => git(repo, 'log', '-1', '--format=%B').includes('Signed-off-by:'), 'the amend to be written')
  const amended = git(repo, 'log', '-1', '--format=%B')
  assert.ok(amended.includes('Signed-off-by: Smoke <smoke@example.com>'),
    'the amended commit carries a Signed-off-by trailer: ' + amended)
  assert.ok(git(repo, 'show', '--stat', '--format=', 'HEAD').includes('amend.txt'), 'the staged file joined the amended commit')
  assert.equal(git(repo, 'log', '--format=%s', '-1').trim(), 'tip commit', 'the amended subject is unchanged')

  /* ---- Partial stash, then rename it from the shelf ---- */

  await writeFile(join(repo, 'base.txt'), 'alpha\nbeta changed\ngamma\n')
  await writeFile(join(repo, 'second.txt'), 'second changed\n')
  await gitMenuAction('Refresh')
  await until(() => evaluate(`(() => {
    const row = ${workingRow('base.txt')}
    if (!row) return false
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
    return !!document.querySelector('[aria-label="Working file actions"]')
  })()`), 'the working file actions menu').catch(async error => {
    console.log('rows:', await evaluate(`JSON.stringify([...document.querySelectorAll('.working-row')].map(r => r.innerText))`))
    console.log('sidebar:', await evaluate("document.querySelector('.workspace-sidebar .tool-title')?.innerText"))
    console.log('notices:', await evaluate("JSON.stringify(Array.from(document.querySelectorAll('.notice'), n => n.innerText))"))
    console.log('status:', JSON.stringify(git(repo, 'status', '--porcelain')))
    throw error
  })
  await clickButton('[aria-label="Working file actions"]', 'Stash file changes…')
  // `git stash push -- <path>` writes the index commit first and resets the
  // path afterwards, so wait for the working tree to settle before asserting.
  await until(() => {
    const porcelain = git(repo, 'status', '--porcelain')
    return git(repo, 'stash', 'list').trim() !== '' && !porcelain.includes('base.txt')
  }, 'the partial stash to leave the working tree').catch(async error => {
    console.log('status:', JSON.stringify(git(repo, 'status', '--porcelain')))
    console.log('cached:', JSON.stringify(git(repo, 'diff', '--cached', '--name-only')))
    console.log('stashes:', JSON.stringify(git(repo, 'stash', 'list')))
    console.log('notices:', await evaluate("Array.from(document.querySelectorAll('.notice'), node => node.innerText)"))
    throw error
  })
  const status = git(repo, 'status', '--porcelain')
  assert.ok(status.includes('second.txt'), 'the other change stayed in the working tree: ' + status)
  assert.equal(git(repo, 'stash', 'list').trim().split('\n').length, 1, 'one stash entry was created')
  const stashed = git(repo, 'stash', 'show', '--name-only', 'stash@{0}')
  assert.ok(stashed.includes('base.txt') && !stashed.includes('second.txt'),
    'the stash holds only the selected file: ' + stashed)

  await clickButton('.commit-tabs', 'Shelf')
  await until(() => evaluate("!!document.querySelector('.shelf-entries button')"), 'the shelf entries')
  await evaluate("document.querySelector('.shelf-entries button').click()")
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label=Stashes]')
    const buttons = [...panel.querySelectorAll('button')].filter(item => item.innerText.trim() === 'Rename…')
    const button = buttons.at(-1)
    if (!button || button.disabled) return false
    button.click()
    return true
  })()`), 'the rename stash dialog')
  await inputValue('.confirmation-dialog input[aria-label="New stash message"]', 'renamed stash')
  await clickButton('.confirmation-dialog', 'Rename')
  await until(() => git(repo, 'stash', 'list', '--format=%s').includes('renamed stash'),
    'the stash rename to be applied')

  /* ---- Cleanup: show diagnostics, then run maintenance on confirmation ---- */

  git(repo, 'switch', '-c', 'throwaway')
  git(repo, 'commit', '--allow-empty', '-m', 'unreachable commit')
  git(repo, 'switch', 'main')
  git(repo, 'branch', '-D', 'throwaway')
  await gitMenuAction('Cleanup (git gc)…')
  const cleanup = 'dialog[aria-label="Git cleanup"]'
  await until(() => evaluate('document.querySelector(' + JSON.stringify(cleanup) + ')?.open'), 'the cleanup modal')
  assert.equal(await evaluate('document.activeElement.textContent.trim()'), 'Cancel')
  await clickButton(cleanup, 'Cancel')
  await until(() => evaluate('!document.querySelector(' + JSON.stringify(cleanup) + ')'), 'the cleanup modal to close')
  await gitMenuAction('Cleanup (git gc)…')
  const count = await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Git cleanup"]')
    const match = panel?.innerText.match(/(\\d+) unreachable commit/)
    return match ? Number(match[1]) : false
  })()`), 'the unreachable commit count')
  assert.ok(count >= 1, 'the cleanup panel reports unreachable commits: ' + count)
  await clickButton('[aria-label="Git cleanup"]', 'Run git gc')
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Git cleanup"]')
    return !!panel && panel.innerText.includes('Git cleanup finished.')
  })()`), 'the cleanup to finish').catch(async error => {
    console.log('cleanup panel:', await evaluate("document.querySelector('[aria-label=\"Git cleanup\"]')?.innerText"))
    console.log('alerts:', await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"))
    throw error
  })
  await clickButton('[aria-label="Git cleanup"]', 'Close')
  await until(() => evaluate("!document.querySelector('[aria-label=\"Git cleanup\"]')"), 'the cleanup panel to close')

  /* ---- Fetch all remotes ---- */

  execFileSync('git', ['clone', '--bare', '--quiet', repo, backup], { stdio: 'pipe' })
  git(repo, 'remote', 'add', 'backup', backup)
  await gitMenuAction('Fetch…')
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Remote synchronization"]')
    if (!panel) return false
    const button = [...panel.querySelectorAll('button')].find(item => item.innerText === 'Fetch all remotes')
    return !!button && !button.disabled
  })()`), 'the fetch-all button').catch(async error => {
    console.log('panels:', await evaluate("JSON.stringify(Array.from(document.querySelectorAll('.daily-panel'), n => n.getAttribute('aria-label')))"))
    console.log('menu open:', await evaluate("document.querySelector('.git-menu')?.open"))
    console.log('alerts:', await evaluate("JSON.stringify(Array.from(document.querySelectorAll('[role=alert]'), n => n.innerText))"))
    throw error
  })
  await clickButton('[aria-label="Remote synchronization"]', 'Fetch all remotes')
  await until(() => evaluate(`(() => {
    const panel = document.querySelector('[aria-label="Remote synchronization"]')
    return panel.innerText.includes('Fetched all remotes.')
  })()`), 'the fetch of every remote')
  assert.ok(git(repo, 'for-each-ref', '--format=%(refname)', 'refs/remotes/backup').includes('refs/remotes/backup/main'),
    'the second remote is fetched')

  /* ---- Workspace roots: separate sessions per root ---- */

  // Fetch reports success before its follow-up history refresh finishes.
  await until(() => evaluate("document.querySelector('.statusbar')?.innerText.includes('Ready')"),
    'the fetch and repository refresh to finish')
  await until(() => evaluate(`(() => {
    const row = ${workingRow('second.txt')}
    if (!row) return false
    row.click()
    return true
  })()`), 'a working file selection')
  await until(() => evaluate(`(() => {
    const rows = [...document.querySelectorAll('.commit-row')]
    const row = rows.find(item => item.innerText.includes('second commit'))
    if (!row) return false
    row.click()
    return row.classList.contains('active')
  })()`), 'the second commit to be selected before switching roots')
  const selectedHash = git(repo, 'log', '-1', '--format=%H', '--grep=^second commit$').trim()
  assert.ok(selectedHash, 'the fixture has a second commit')
  await until(() => evaluate(`(() => {
    const sessions = JSON.parse(localStorage.getItem('gitvela.sessions') || '{}')
    return sessions.byPath?.[sessions.last]?.hash === ${JSON.stringify(selectedHash)}
  })()`), 'the selected commit to be saved before switching roots')

  await openRepository(other)
  await until(() => evaluate("document.querySelector('.sidebar-project strong')?.innerText.includes('other')"),
    'the second repository to open')

  const roots = await until(() => evaluate(`(() => {
    const menu = document.querySelector('.roots-menu')
    if (!menu) return false
    const buttons = [...menu.querySelectorAll('.command-menu button')]
      .filter(item => item.title)
    return buttons.length === 2 ? buttons.map(item => item.title) : false
  })()`), 'both workspace roots in the menu')
  // The app stores absolute Windows paths, while the fixture is addressed with
  // its POSIX equivalent, so the roots are compared by folder name.
  const names = roots.map(root => root.split(/[\\/]/).at(-1))
  assert.ok(names.includes('repo') && names.includes('other'), 'the menu lists both roots: ' + roots.join(', '))

  // Pick the fixture root in Node, so the click does not depend on how the
  // path separators are escaped inside the injected script.
  const target = roots.find(root => root.split(/[\\/]/).at(-1) === 'repo')
  assert.ok(target, 'the first root is listed')
  await until(() => evaluate(`(() => {
    const menu = document.querySelector('.roots-menu')
    if (!menu.open) menu.querySelector('summary').click()
    const button = [...menu.querySelectorAll('button')]
      .find(item => item.title === ${JSON.stringify(target)})
    if (!button || button.disabled) return false
    button.click()
    return true
  })()`), 'the first root to be reopened').catch(async error => {
    console.log('root buttons:', await evaluate(`JSON.stringify([...document.querySelectorAll('.roots-menu button')].map(b => ({ title: b.title, disabled: b.disabled, text: b.innerText })))`))
    console.log('menu open:', await evaluate("document.querySelector('.roots-menu')?.open"))
    throw error
  })
  await until(() => evaluate("document.querySelector('.sidebar-project strong')?.innerText.includes('repo')"),
    'the first repository to be active again')
  await until(() => evaluate(`(() => {
    const row = [...document.querySelectorAll('.commit-row.active')]
    return row.length > 0 && row[0].innerText.includes('second commit')
  })()`), 'the commit selected in this root before switching away').catch(async error => {
    console.log('sessions after switch:', await evaluate("localStorage.getItem('gitvela.sessions')"))
    console.log('active rows:', await evaluate("JSON.stringify(Array.from(document.querySelectorAll('.commit-row.active'), n => n.innerText))"))
    throw error
  })

  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])

  console.log('Batch 5 smoke passed: editor preferences, go-to-line, bisect, amend sign-off, partial stash, stash rename, cleanup, fetch all remotes, workspace roots with per-root sessions.')
} finally {
  socket.close()
  if (!keepRepo) {
    await rm(base, { recursive: true, force: true, maxRetries: 20, retryDelay: 200 })
      .catch(() => console.log('Left fixture in place: ' + base))
  } else console.log('Kept fixture directory: ' + base)
}
