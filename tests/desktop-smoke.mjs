import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, unlink, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const repo = process.argv[2] ?? process.cwd()
const screenshot = process.argv[3]
const workingFile = process.argv[4]
const testMvp = process.env.GITVELA_SMOKE_MVP === '1'
const dailyRemote = process.env.GITVELA_SMOKE_M2_REMOTE
const worktreeTarget = process.env.GITVELA_SMOKE_WORKTREE
const testIndex = process.argv[5] === '--index'
const testCommit = process.argv[5] === '--commit'
const testBranch = process.argv[5] === '--branch'

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

async function evaluate(expression) {
  const result = await cdp('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text)
  return result.result.value
}

try {
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
    const branches = rect('.branch-panel')
    const history = rect('.log-panel')
    const changes = rect('.files-panel')
    return working.y === preview.y && working.x < preview.x && working.bottom <= history.y &&
      branches.y === history.y && history.y === changes.y && branches.x < history.x && history.x < changes.x
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
    for (const [tab, line] of [['Staged diff', '+staged version'], ['Unstaged diff', '+working version']]) {
      await evaluate(`[...document.querySelectorAll('.preview-tabs button')].find(button => button.innerText === ${JSON.stringify(tab)}).click()`)
      await until(() => evaluate(`document.querySelector('.patch')?.textContent.includes(${JSON.stringify(line)})`), tab)
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
    await evaluate("document.querySelector('.branch-panel .panel-header button').click()")
    await evaluate(`(() => {
      const input = document.querySelector('[aria-label="New branch name"]')
      input.value = 'feature/desktop'
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.form.requestSubmit()
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"Switch to feature/desktop\"]')"), 'new local branch')
    assert.equal(await evaluate("document.querySelector('.branch-panel .active')?.textContent.trim()"), 'main')
    await evaluate("document.querySelector('[title=\"Switch to feature/desktop\"]').click()")
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.textContent.trim() === 'feature/desktop'"), 'new active branch')
  }
  if (testBranch) {
    const click = (selector) => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`)
    const action = (label) => evaluate(`[...document.querySelectorAll('.branch-panel .new-branch button')].find(button => button.innerText === ${JSON.stringify(label)}).click()`)
    await click('[title="Switch to main"]')
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.textContent.trim() === 'main'"), 'main active again')
    await click('[aria-label="Manage feature/desktop"]')
    await action('Rename')
    await evaluate(`(() => {
      const input = document.querySelector('[aria-label="New branch name"]')
      input.value = 'feature/renamed'
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.form.requestSubmit()
    })()`)
    await until(() => evaluate("!!document.querySelector('[aria-label=\"Manage feature/renamed\"]')"), 'renamed branch')
    await click('[aria-label="Manage feature/renamed"]')
    await action('Delete…')
    await action('Cancel')
    assert.equal(await evaluate("!!document.querySelector('[aria-label=\"Manage feature/renamed\"]')"), true)
    await click('[aria-label="Manage feature/renamed"]')
    await action('Delete…')
    await action('Delete branch')
    await until(() => evaluate("!document.querySelector('[aria-label=\"Manage feature/renamed\"]')"), 'confirmed deletion')
    await evaluate(`(() => {
      const picker = document.querySelector('[aria-label="Reference type"]')
      picker.value = 'tags'
      picker.dispatchEvent(new Event('change', { bubbles: true }))
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"refs/tags/v1.0\"]')"), 'repository tag listed')
    await click('[title="refs/tags/v1.0"]')
    await until(() => evaluate("document.querySelector('.commit-row.active')?.innerText.includes('Initialize GitVela')"), 'tag commit selected')
  }
  if (worktreeTarget) {
    const click = (selector) => until(() => evaluate(`(() => {
      const button = document.querySelector(${JSON.stringify(selector)})
      if (!button || button.disabled) return false
      button.click()
      return true
    })()`), 'enabled worktree control ' + selector)
    await click('.topbar button[aria-pressed="false"]')
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
    await click('.worktree-form .danger-button')
    await until(() => evaluate("document.querySelectorAll('.worktree-row').length === 1"), 'confirmed removal')
  }
  const alerts = await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)")
  assert.deepEqual(alerts, [])

  await until(() => evaluate(`(() => {
    const row = [...document.querySelectorAll('.commit-row')].find(row => row.innerText.includes('Initialize GitVela'))
    if (!row) return false
    row.click()
    return true
  })()`), 'the initial commit')
  await until(() => evaluate(`(() => {
    const files = [...document.querySelectorAll('.file-row')]
    const file = files.find(row => row.title === 'GitVela.md')
    if (!file) return false
    files.slice(0, 3).forEach(row => row.click())
    file.click()
    return true
  })()`), 'GitVela.md in Changed Files')
  await until(() => evaluate(`(() => {
    const editors = document.querySelectorAll('.cm-mergeView .cm-content')
    return editors.length === 2 && editors[0].textContent === '' &&
      editors[1].textContent.includes('GitVela') && editors[1].getAttribute('contenteditable') !== 'true'
  })()`), 'read-only side-by-side Diff with the real root blob')

  if (testMvp) {
    if (worktreeTarget) {
      await evaluate("[...document.querySelectorAll('.topbar button')].find(button => button.innerText === 'Worktrees' && button.getAttribute('aria-pressed') === 'true')?.click()")
      execFileSync('git', ['-C', repo, 'branch', 'smoke/external-metadata'])
      await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/smoke/external-metadata\"]')"), 'external branch refresh')
    }
    const click = (selector) => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`)
    await click('.log-tools button[aria-expanded="false"]')
    await evaluate(`(() => {
      const form = document.querySelector('.history-search')
      const message = [...form.querySelectorAll('label')].find(label => label.textContent.trim() === 'Message').querySelector('input')
      message.value = 'Initialize GitVela'
      message.dispatchEvent(new Event('input', { bubbles: true }))
      form.requestSubmit()
    })()`)
    await until(() => evaluate("document.querySelector('.search-results .commit-row')?.innerText.includes('Initialize GitVela')"), 'history search result')
    await evaluate("document.querySelector('.search-results .commit-row').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))")
    await until(() => evaluate("document.querySelector('[aria-label=\"Commit actions\"]')?.innerText.includes('Copy message')"), 'commit context actions')
    await evaluate("[...document.querySelectorAll('[aria-label=\"Commit actions\"] button')].find(button => button.innerText === 'Copy hash').click()")
    await until(() => evaluate("document.querySelector('.log-panel [role=status]')?.innerText === 'Hash copied.'"), 'copy hash')
    await click('.log-tools button[aria-expanded="true"]')
    await until(() => evaluate("!!document.querySelector('.commit-row canvas')"), 'return to history graph')
    await click('.preferences summary')
    await click('.preferences-panel label:first-child input')
    await until(() => evaluate("document.querySelector('.shell').dataset.theme === 'dark'"), 'dark theme from Settings')
    await click('.preferences-panel label:first-child input')
    await click('.preferences-panel label:nth-child(2) input')
    await until(() => evaluate("!!document.querySelector('.files-panel .folder-row')"), 'file tree from Settings')
    await click('.preferences-panel label:nth-child(2) input')
    await click('.preferences summary')
    await until(() => evaluate("!!document.querySelector('.diff-host .cm-editor')"), 'commit diff available')
    await click('.preview-tabs .diff-size')
    await until(() => evaluate("document.querySelector('.upper').classList.contains('diff-expanded') && getComputedStyle(document.querySelector('.lower')).display === 'none'"), 'maximized diff')
    await evaluate("document.querySelector('.preview-tabs .diff-size').dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))")
    await until(() => evaluate("!document.querySelector('.upper').classList.contains('diff-expanded')"), 'restored workspace')
  }
  await evaluate("Array.from(document.querySelectorAll('.diff-actions button')).find(button => button.innerText === 'Unified').click()")
  await until(() => evaluate("document.querySelectorAll('.diff-host .cm-editor').length === 1"), 'unified Diff')
  await evaluate("Array.from(document.querySelectorAll('.diff-actions button')).find(button => button.innerText === 'Side by side').click()")
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
    await click('.commit-form', 'Amend…')
    await text('.amend-warning', 'Rewrites HEAD')
    assert.ok(await evaluate("document.querySelector('#commit-message').value.trim().length"))
    await set('#commit-message', 'Amended through GitVela')
    await click('.commit-form', 'Amend commit')
    await text('.commit-form [role=status]', 'Commit amended')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Amended through GitVela')

    const untracked = join(repo, 'gitvela-smoke-untracked.txt')
    await writeFile(untracked, 'saved by GitVela', { flag: 'wx' })
    await click('.topbar', 'Stash…')
    await evaluate("document.querySelector('[aria-label=Stashes] input[type=checkbox]').click()")
    await set('[aria-label="Stash message"]', 'desktop stash')
    await click('[aria-label=Stashes]', 'Save stash')
    await text('[aria-label=Stashes] [role=status]', 'Stash saved.')
    assert.equal(git(repo, 'status', '--porcelain'), '')
    await until(() => evaluate("document.querySelector('[aria-label=Stashes] select')?.options.length === 2"), 'saved stash visible')
    await evaluate(`(() => {
      const select = document.querySelector('[aria-label=Stashes] select')
      select.selectedIndex = 1
      select.dispatchEvent(new Event('change', { bubbles: true }))
    })()`)
    await click('[aria-label=Stashes]', 'Pop')
    await text('[aria-label=Stashes] [role=status]', 'Stash applied and removed.')
    assert.match(git(repo, 'status', '--porcelain'), /gitvela-smoke-untracked/)
    assert.equal(git(repo, 'stash', 'list'), '')
    await unlink(untracked)

    git(repo, 'init', '--bare', '-b', dailyBranch, dailyRemote)
    git(repo, 'remote', 'add', 'm2-smoke', dailyRemote)
    await click('.topbar', 'Sync…')
    await until(() => evaluate("[...document.querySelectorAll('[aria-label=\"Remote synchronization\"] select option')].some(option => option.value === 'm2-smoke')"), 'local test remote listed')
    await set('[aria-label="Remote synchronization"] select', 'm2-smoke')
    await click('[aria-label="Remote synchronization"]', 'Push…')
    await text('[aria-label="Confirm push"]', 'm2-smoke/' + dailyBranch)
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
    await click('[aria-label="Remote synchronization"]', 'Fetch')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Fetched m2-smoke.')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Amended through GitVela')
    await click('[aria-label="Remote synchronization"]', 'Pull (fast-forward)')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Pull complete')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Commit from smoke peer')
    await click('.topbar', 'Sync…')
  }
  assert.deepEqual(await evaluate("window.smokeRejections"), [])
  if (screenshot) {
    const image = await cdp('Page.captureScreenshot', { format: 'png' })
    await writeFile(screenshot, Buffer.from(image.data, 'base64'))
  }
  console.log('Desktop smoke passed: Go bindings, repository, history canvas, changed files, and read-only Diff.')
  if (dailyRemote) console.log('Amend, stash save and pop, and remote push, fetch, and pull passed.')
  if (worktreeTarget) console.log('Worktree creation, switching, and removal passed.')
  if (testMvp) console.log('History search, commit actions, settings, file tree, and diff maximization passed.')
} finally {
  socket.close()
}
