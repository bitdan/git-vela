import { describe, expect, it } from 'vitest'
import { navigateDiff, registerDiffNavigation } from './diffNavigation'

describe('diff navigation channel', () => {
  it('forwards to the active diff and detaches on cleanup', () => {
    const seen: number[] = []
    const release = registerDiffNavigation(direction => seen.push(direction))
    navigateDiff(-1)
    navigateDiff(1)
    release()
    navigateDiff(1)
    expect(seen).toEqual([-1, 1])
  })

  it('only the latest registration receives commands', () => {
    const first: number[] = []
    const second: number[] = []
    const releaseFirst = registerDiffNavigation(direction => first.push(direction))
    const releaseSecond = registerDiffNavigation(direction => second.push(direction))
    navigateDiff(1)
    releaseSecond()
    releaseFirst()
    expect(first).toEqual([])
    expect(second).toEqual([1])
  })
})
