import { describe, expect, it } from 'vitest'
import { canConfirmPush, outgoingSummary, shortHash } from './pushPreview'

const dest = { remote: 'origin', target: 'main' }
const preview = (count: number, newBranch = false, more = false) => ({
  tip: 'a'.repeat(40), newBranch, more,
  commits: Array.from({ length: count }, (_, i) => ({ hash: String(i).repeat(40) })),
})

describe('push preview', () => {
  it('needs a loaded preview before confirming', () => {
    expect(canConfirmPush(undefined, false)).toBe(false)
    expect(canConfirmPush(undefined, true)).toBe(false)
  })

  it('disables an empty push unless it creates the branch or forces', () => {
    expect(canConfirmPush(preview(0), false)).toBe(false)
    expect(canConfirmPush(preview(0), true)).toBe(true)
    expect(canConfirmPush(preview(0, true), false)).toBe(true)
    expect(canConfirmPush(preview(2), false)).toBe(true)
    expect(canConfirmPush({ ...preview(0), commits: null }, false)).toBe(false)
  })

  it('names the commits and the destination', () => {
    expect(outgoingSummary(preview(1), dest)).toBe('1 commit to push to origin/main.')
    expect(outgoingSummary(preview(3), dest)).toBe('3 commits to push to origin/main.')
    expect(outgoingSummary(preview(0), dest)).toBe('origin/main already contains this branch; nothing to push.')
    expect(outgoingSummary(preview(2, true), dest)).toBe('2 commits not on origin yet; this creates origin/main.')
    expect(outgoingSummary(preview(0, true), dest)).toBe('Creates origin/main; every commit is already on origin.')
    expect(outgoingSummary(preview(1000, false, true), dest)).toBe('More than 1000 commits to push to origin/main.')
  })

  it('shortens hashes', () => {
    expect(shortHash('0123456789abcdef')).toBe('01234567')
  })
})
