import { describe, expect, it } from 'vitest'
import { CommitSelection, comparePair } from './commitSelection'
import type { Commit } from '../../../bindings/gitvela/internal/git'

const commit = (hash: string): Commit => ({
  hash,
  subject: 'commit ' + hash.slice(0, 6),
  author: 'tester',
  time: 1700000000,
  parents: [],
  refs: [],
  graph: { node: 0, color: 0, incoming: false, through: null, out: null, width: 1 },
})

// Newest-first list, like the Git log.
const commits = [commit('a'.repeat(40)), commit('b'.repeat(40)), commit('c'.repeat(40)), commit('d'.repeat(40))]

describe('CommitSelection', () => {
  it('resets to a single row and anchors it', () => {
    const selection = new CommitSelection()
    selection.reset(commits, 1)
    expect(selection.hashes).toEqual([commits[1]!.hash])
    selection.extend(commits, 3)
    expect(selection.hashes).toEqual([commits[1]!.hash, commits[2]!.hash, commits[3]!.hash])
  })

  it('toggles rows in click order and reports the direction', () => {
    const selection = new CommitSelection()
    selection.reset(commits, 0)
    expect(selection.toggle(commits, 2)).toBe(true)
    expect(selection.hashes).toEqual([commits[0]!.hash, commits[2]!.hash])
    expect(selection.toggle(commits, 0)).toBe(false)
    expect(selection.hashes).toEqual([commits[2]!.hash])
  })

  it('extends the range from the anchor in both directions', () => {
    const selection = new CommitSelection()
    selection.reset(commits, 3)
    selection.extend(commits, 1)
    expect(selection.hashes).toEqual([commits[1]!.hash, commits[2]!.hash, commits[3]!.hash])
  })

  it('clears on reset with an out-of-range row', () => {
    const selection = new CommitSelection()
    selection.reset(commits, 0)
    selection.reset(commits, 99)
    expect(selection.size).toBe(0)
  })

  it('clears the whole selection', () => {
    const selection = new CommitSelection()
    selection.reset(commits, 1)
    selection.toggle(commits, 2)
    selection.clear()
    expect(selection.size).toBe(0)
    expect(selection.has(commits[1]!.hash)).toBe(false)
  })
})

describe('comparePair', () => {
  it('orders exactly two rows older → newer', () => {
    const pair = comparePair(commits, [commits[0]!.hash, commits[2]!.hash])
    expect(pair?.before).toBe(commits[2]!)
    expect(pair?.after).toBe(commits[0]!)
  })

  it('rejects selections without a legal comparison target', () => {
    expect(comparePair(commits, [commits[0]!.hash])).toBeUndefined()
    expect(comparePair(commits, [commits[0]!.hash, 'not-loaded'])).toBeUndefined()
    expect(comparePair(commits, [commits[0]!.hash, commits[1]!.hash, commits[2]!.hash])).toBeUndefined()
  })
})
