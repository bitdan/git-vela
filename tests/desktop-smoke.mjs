import assert from 'node:assert/strict'
import { writeFile } from 'node:fs/promises'
import { setTimeout as delay } from 'node:timers/promises'

const endpoint = process.env.GITVELA_CDP_URL ?? 'http://127.0.0.1:9331'
const repo = process.argv[2] ?? process.cwd()
const screenshot = process.argv[3]

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

  await evaluate("Array.from(document.querySelectorAll('.diff-actions button')).find(button => button.innerText === 'Unified').click()")
  await until(() => evaluate("document.querySelectorAll('.diff-host .cm-editor').length === 1"), 'unified Diff')
  await evaluate("Array.from(document.querySelectorAll('.diff-actions button')).find(button => button.innerText === 'Side by side').click()")
  await until(() => evaluate("document.querySelectorAll('.cm-mergeView .cm-editor').length === 2"), 'side-by-side Diff restored')
  assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('[role=alert]'), node => node.innerText)"), [])
  assert.deepEqual(await evaluate("window.smokeRejections"), [])
  if (screenshot) {
    const image = await cdp('Page.captureScreenshot', { format: 'png' })
    await writeFile(screenshot, Buffer.from(image.data, 'base64'))
  }
  console.log('Desktop smoke passed: Go bindings, repository, history canvas, changed files, and read-only Diff.')
} finally {
  socket.close()
}
