import { describe, expect, it } from 'vitest'
import { matchesProtectedBranch } from './protectedBranches'

describe('matchesProtectedBranch', () => {
  it('matches exact names and wildcards', () => {
    expect(matchesProtectedBranch(['main'], 'main')).toBe(true)
    expect(matchesProtectedBranch(['release/*'], 'release/1.0')).toBe(true)
    expect(matchesProtectedBranch(['release/*'], 'release/1.0/hotfix')).toBe(true)
    expect(matchesProtectedBranch(['v?'], 'v1')).toBe(true)
  })

  it('ignores unrelated, empty and malformed patterns', () => {
    expect(matchesProtectedBranch(['main'], 'mainline')).toBe(false)
    expect(matchesProtectedBranch(['release/*'], 'releases/1.0')).toBe(false)
    expect(matchesProtectedBranch(['  ', ''], 'main')).toBe(false)
    expect(matchesProtectedBranch(['['], 'main')).toBe(false)
    expect(matchesProtectedBranch([], 'main')).toBe(false)
  })

  it('does not treat dots as wildcards', () => {
    expect(matchesProtectedBranch(['v1.0'], 'v1x0')).toBe(false)
  })
})
