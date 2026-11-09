import { GutterMarker, gutter, type EditorView } from '@codemirror/view'
import type { BlameLine } from '../../../bindings/gitvela/internal/git'

const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'short' })

function blameDate(seconds: number) {
  const time = new Date(seconds * 1000)
  if (!seconds || Number.isNaN(time.getTime())) return ''
  const today = new Date()
  if (time.toDateString() === today.toDateString()) return 'Today'
  today.setDate(today.getDate() - 1)
  return time.toDateString() === today.toDateString() ? 'Yesterday' : date.format(time)
}

class BlameMarker extends GutterMarker {
  constructor(readonly line: BlameLine, readonly group: number, readonly jump: (hash: string) => void) { super() }

  toDOM() {
    const item = document.createElement(this.line.uncommitted ? 'span' : 'button')
    item.className = 'blame-annotation'
    item.dataset.group = String(this.group)
    const day = document.createElement('span')
    day.className = 'blame-date'
    day.textContent = blameDate(this.line.time)
    const author = document.createElement('span')
    author.className = 'ellipsis'
    author.textContent = this.line.uncommitted ? 'Uncommitted' : this.line.author
    item.append(day, author)
    if (item instanceof HTMLButtonElement) {
      item.type = 'button'
      item.title = [this.line.author, blameDate(this.line.time), this.line.hash.slice(0, 8), this.line.subject].filter(Boolean).join('\n')
      item.addEventListener('click', () => this.jump(this.line.hash))
    }
    return item
  }
}

export function blameGutter(lines: BlameLine[], jump: (hash: string) => void) {
  let group = 0
  const markers = lines.map((line, index) => {
    if (index && lines[index - 1]?.hash !== line.hash) group = (group + 1) % 3
    return new BlameMarker(line, group, jump)
  })
  return gutter({
    class: 'cm-git-blame',
    lineMarker: (view: EditorView, block) => markers[view.state.doc.lineAt(block.from).number - 1] ?? null,
  })
}
