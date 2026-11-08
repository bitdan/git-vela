import type { Commit } from '../../../bindings/gitvela/internal/git'

/** IDEA-style multiple interval selection for the commit list (spec T06, gap I04).
 *  Plain clicks reset to a single row, Ctrl/Cmd toggles rows, Shift extends the
 *  range from an anchor row. The workspace store's `selected` commit stays the
 *  focused row that drives Changed Files and Commit Details; this class only
 *  tracks the visual multi-selection and the legal comparison targets. */
export class CommitSelection {
  /** Selected hashes in click order; mutated only through the methods below. */
  readonly hashes: string[] = []
  private anchor = -1

  get size() {
    return this.hashes.length
  }

  has(hash: string) {
    return this.hashes.includes(hash)
  }

  /** Plain click: replace the whole selection with one row and move the anchor. */
  reset(commits: Commit[], index: number) {
    this.hashes.length = 0
    this.anchor = -1
    const commit = commits[index]
    if (commit) {
      this.hashes.push(commit.hash)
      this.anchor = index
    }
  }

  /** Ctrl/Cmd click: toggle one row in or out. Returns whether the row joined
   *  the selection, so callers can move the focused commit accordingly. */
  toggle(commits: Commit[], index: number): boolean {
    const commit = commits[index]
    if (!commit) return false
    this.anchor = index
    const at = this.hashes.indexOf(commit.hash)
    if (at >= 0) {
      this.hashes.splice(at, 1)
      return false
    }
    this.hashes.push(commit.hash)
    return true
  }

  /** Shift click or Shift+Arrow: select the range between the anchor and the
   *  target row, in list order. Clicking without Ctrl afterwards re-anchors. */
  extend(commits: Commit[], index: number) {
    if (!commits[index]) return
    if (this.anchor < 0 || !commits[this.anchor]) this.anchor = index
    const [from, to] = this.anchor <= index ? [this.anchor, index] : [index, this.anchor]
    this.hashes.length = 0
    for (let i = from; i <= to; i++) this.hashes.push(commits[i]!.hash)
  }

  clear() {
    this.hashes.length = 0
    this.anchor = -1
  }
}

/** The ordered comparison pair for exactly two selected rows: the older commit
 *  (deeper in the newest-first log) is the "before" side, like pin+compare. */
export function comparePair(commits: Commit[], hashes: readonly string[]): { before: Commit; after: Commit } | undefined {
  if (hashes.length !== 2) return undefined
  const first = commits.findIndex(commit => commit.hash === hashes[0])
  const second = commits.findIndex(commit => commit.hash === hashes[1])
  if (first < 0 || second < 0) return undefined
  return first > second
    ? { before: commits[first]!, after: commits[second]! }
    : { before: commits[second]!, after: commits[first]! }
}
