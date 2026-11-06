import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, readFile, unlink, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
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

async function gitMenuAction(label) {
  await evaluate("if (!document.querySelector('.git-menu').open) document.querySelector('.git-menu summary').click()")
  await clickButton('.git-menu', label)
}

async function branchMenuAction(branch, label) {
  const selector = '[title="refs/heads/' + branch + '"]'
  await until(() => evaluate(`(() => {
    const row = document.querySelector(${JSON.stringify(selector)})
    if (!row) return false
    row.click()
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
    const sidebar = rect('.workspace-sidebar')
    const history = rect('.log-panel')
    const changes = rect('.files-panel')
    return working.left >= sidebar.left && working.right <= sidebar.right &&
      sidebar.right < preview.x && sidebar.y === preview.y &&
      preview.bottom < history.y && history.x === preview.x && history.y === changes.y &&
      history.right < changes.x && sidebar.bottom === history.bottom
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
    await clickButton('.sidebar-segments', 'Branches')
    await evaluate("document.querySelector('.branch-panel .panel-header button').click()")
    await evaluate(`(() => {
      const input = document.querySelector('[aria-label="New branch name"]')
      input.value = 'feature/desktop'
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.form.requestSubmit()
    })()`)
    await until(() => evaluate("!!document.querySelector('[title=\"refs/heads/feature/desktop\"]')"), 'new local branch')
    assert.equal(await evaluate("document.querySelector('.branch-panel .active')?.textContent.trim()"), 'main')
    await branchMenuAction('feature/desktop', 'Checkout')
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.textContent.trim() === 'feature/desktop'"), 'new active branch')
  }
  if (testBranch) {
    const click = (selector) => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`)
    const action = (label) => evaluate(`[...document.querySelectorAll('.branch-panel .new-branch button')].find(button => button.innerText === ${JSON.stringify(label)}).click()`)
    await branchMenuAction('main', 'Checkout')
    await until(() => evaluate("document.querySelector('.branch-panel .active')?.textContent.trim() === 'main'"), 'main active again')
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
    await gitMenuAction('Branches')
    const click = (selector) => until(() => evaluate(`(() => {
      const button = document.querySelector(${JSON.stringify(selector)})
      if (!button || button.disabled) return false
      button.click()
      return true
    })()`), 'enabled worktree control ' + selector)
    await clickButton('.sidebar-segments', 'Worktrees')
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
      await gitMenuAction('Branches')
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
    const treeWasOn = await evaluate("document.querySelector('.preferences-panel label:nth-child(2) input').checked")
    if (!treeWasOn) await click('.preferences-panel label:nth-child(2) input')
    await until(() => evaluate("!!document.querySelector('.files-panel .folder-row')"), 'file tree from Settings')
    if (!treeWasOn) await click('.preferences-panel label:nth-child(2) input')
    await click('.preferences summary')
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
    await clickButton('.sidebar-segments', 'Changes')
    await click('.commit-form', 'Amend…')
    await text('.amend-warning', 'Rewrites HEAD')
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
    await gitMenuAction('Push…')
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
    await gitMenuAction('Fetch…')
    await click('[aria-label="Remote synchronization"]', 'Fetch')
    await text('[aria-label="Remote synchronization"] [role=status]', 'Fetched m2-smoke.')
    assert.equal(git(repo, 'log', '-1', '--format=%s'), 'Amended through GitVela')
    await gitMenuAction('Pull…')
    await click('[aria-label="Remote synchronization"]', 'Pull (fast-forward)')
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
    const editorStatus = (text) =>
      until(() => evaluate(`document.querySelector('.editor-tools [role=status]')?.innerText.includes(${JSON.stringify(text)})`), text)
    const enterText = async (text) => {
      await evaluate("document.querySelector('.editor-host .cm-content').focus()")
      await keyboard('a', 'KeyA', 65, 2)
      await cdp('Input.insertText', { text })
    }
    const selectFile = async (path) => {
      await inputValue('[aria-label="Filter repository files"]', path)
      await clickButton('.repository-files .file-scroll', path)
    }

    await clickButton('.sidebar-segments', 'Files')
    await selectFile(file)
    await editorStatus('Saved')
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

    await clickButton('.sidebar-segments', 'Changes')
    await until(() => evaluate(`(() => {
      const row = document.querySelector('.working-row[title="greetservice.go"]')
      if (!row) return false
      row.click()
      return true
    })()`), 'edited file in Working Changes')
    await clickButton('.preview-tabs', 'Unstaged diff')
    await until(() => evaluate("document.querySelector('.patch')?.textContent.includes('Edited in GitVela')"), 'saved edit in Git diff')
    await clickButton('.index-actions', 'Stage')
    await until(() => git('diff', '--cached', '--name-only', '--', file) === file, 'edited file staged')

    await clickButton('.preview-tabs', 'Editor')
    await enterText('unsaved draft')
    await editorStatus('Unsaved changes')
    await writeFile(target, 'external update')
    await clickButton('.editor-tools', 'Save')
    await until(() => evaluate("document.querySelector('.preview-panel [role=alert]')?.innerText.includes('changed on disk')"), 'external change blocked save')
    assert.equal(await readFile(target, 'utf8'), 'external update')

    await clickButton('.sidebar-segments', 'Files')
    await selectFile('README.md')
    await clickButton('[aria-label="Unsaved file changes"]', 'Keep editing')
    assert.equal(await evaluate("document.querySelector('.editor-host .cm-content')?.textContent"), 'unsaved draft')
    await clickButton('.editor-tools', 'Close')
    await clickButton('[aria-label="Unsaved file changes"]', 'Discard changes')
    await until(() => evaluate("!document.querySelector('.workspace-editor')"), 'editor closed after discard confirmation')
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
    await clickButton('.sidebar-segments', 'Branches')
    await branchMenuAction('smoke-m3-base', 'Interactive rebase onto this…')
    await text('.rebase-plan', 'M3 second')
    await evaluate("document.querySelector('[aria-label=\"Move M3 second earlier\"]').click()")
    await clickButton('.advanced-panel', 'Confirm Rebase')
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
    await until(() => evaluate("[...document.querySelectorAll('.commit-row')].some(row => row.innerText.includes('M3 side'))"), 'side commit visible')
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
      await clickButton('.advanced-panel', 'Confirm Cherry-pick')
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
    await clickButton('.editor-tools', 'Reload')
    await until(() => evaluate("document.querySelector('.editor-host .cm-content')?.textContent.includes('resolved')"), 'resolution reloaded')
    await clickButton('.conflict-panel', 'Mark resolved')
    await text('.advanced-panel', '0 unmerged')
    await clickButton('.advanced-panel', 'Continue')
    await text('.advanced-panel [role=status]', 'Git operation continued.')
    assert.equal(git('show', 'HEAD:m3.txt'), 'resolved')

    await writeFile(target, 'pending change\n')
    await clickButton('.sidebar-segments', 'Files')
    await until(() => evaluate("!!document.querySelector('.repository-files .folder-row[aria-expanded]')"), 'default repository tree')
    await inputValue('[aria-label="Filter repository files"]', 'm3.txt')
    await clickButton('.repository-files .file-scroll', 'm3.txt')
    await clickButton('.preview-tabs', 'File History')
    await text('.file-history-row', 'M3 side')
    await clickButton('.file-inspection', 'Blame')
    await text('.blame-row', 'Uncommitted')
    await clickButton('.file-inspection', 'Close')

    await clickButton('.sidebar-segments', 'Changes')
    await until(() => evaluate(`(() => {
      const row = document.querySelector('.working-row[title="m3.txt"]')
      if (!row) return false
      row.click()
      return true
    })()`), 'edited file in Changes')
    await clickButton('.preview-tabs', 'Unstaged diff')
    await clickButton('.preview-tabs', 'Hunks')
    await clickButton('.hunk-panel', 'Stage hunk')
    await text('.hunk-panel [role=status]', 'Selection staged.')
    assert.equal(git('show', ':m3.txt'), 'pending change')
    await clickButton('.preview-tabs', 'Staged diff')
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
    // The configured Ctrl+K shortcut opens the command palette.
    await evaluate("document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))")
    await until(() => evaluate("!!document.querySelector('.palette')"), 'command palette')
    await inputValue('[aria-label="Search commands"]', 'compact')
    await until(() => evaluate("document.querySelector('.palette-item.active')?.innerText.includes('compact')"), 'ranked compact-row command')
    await evaluate("document.querySelector('.palette').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))")
    await until(() => evaluate("document.querySelector('.shell').dataset.density === 'compact'"), 'compact row density')
    assert.equal(await evaluate("getComputedStyle(document.querySelector('.commit-row')).height"), '27px')

    // Restore the comfortable layout, then rebind a shortcut through Settings.
    await evaluate("document.querySelector('.preferences summary').click()")
    await evaluate("[...document.querySelectorAll('.preferences-panel label')].find(label => label.innerText.includes('Compact rows')).querySelector('input').click()")
    await until(() => evaluate("document.querySelector('.shell').dataset.density === 'comfortable'"), 'comfortable row density')
    await evaluate("document.querySelector('.shortcut-details summary').click()")
    await until(() => evaluate("document.querySelectorAll('.shortcut-group button').length >= 16"), 'editable shortcut list')
    await evaluate(`(() => {
      const button = document.querySelector('[aria-label="Change shortcut for Command palette"]')
      button.click()
      button.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', ctrlKey: true, shiftKey: true, bubbles: true }))
    })()`)
    await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.keymap') || '{}').palette === 'Ctrl+Shift+P'"), 'persisted custom shortcut')
    await clickButton('.shortcut-settings', 'Reset to defaults')
    await until(() => evaluate("JSON.parse(localStorage.getItem('gitvela.keymap') || '{}').palette === 'Ctrl+K'"), 'default shortcut restored')
    await evaluate("document.querySelector('.preferences').open = false")

    // Session state is persisted after opening a repository.
    await until(() => evaluate("(() => { const saved = JSON.parse(localStorage.getItem('gitvela.session') || 'null'); return !!saved && typeof saved.path === 'string' && saved.path.length > 0 })()"), 'saved session')
    assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"), [])
  }
  assert.deepEqual(await evaluate("window.smokeRejections"), [])
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
  socket.close()
}
