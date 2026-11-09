import { describe, expect, it } from 'vitest'
import { branchSyncActions, describeUpdate, parseRemoteRef, type BranchActionContext } from './branchActions'

const base: BranchActionContext = { ref: 'refs/heads/main', head: 'main', hasRemotes: true, upstream: 'origin/main', updateMethod: 'merge' }
const ids = (context: BranchActionContext) => branchSyncActions(context).map(action => action.id)

describe('branch sync actions', () => {
  it('offers Update and Push, never Pull, for the current branch', () => {
    const actions = branchSyncActions(base)
    expect(actions.map(action => action.id)).toEqual(['update', 'push'])
    expect(actions.every(action => action.enabled)).toBe(true)
    expect(actions.some(action => /pull/i.test(action.label))).toBe(false)
  })

  it('offers Update for a non-current branch as a fast-forward', () => {
    const [update] = branchSyncActions({ ...base, ref: 'refs/heads/feature', upstream: 'origin/feature' })
    expect(update).toMatchObject({ id: 'update', enabled: true })
    expect(update.description).toContain('without checking it out')
  })

  it('disables Update without a tracked branch and hides it without remotes', () => {
    const [update] = branchSyncActions({ ...base, upstream: '' })
    expect(update).toMatchObject({ id: 'update', enabled: false })
    expect(update.description).toContain("isn't configured")
    expect(ids({ ...base, hasRemotes: false })).toEqual(['push'])
  })

  it('disables Update for a branch checked out in another worktree', () => {
    const [update] = branchSyncActions({ ...base, ref: 'refs/heads/feature', upstream: 'origin/feature', otherWorktreeBranches: ['feature'] })
    expect(update.enabled).toBe(false)
  })

  it('offers Pull into current for remote branches and disables it on detached HEAD', () => {
    const actions = branchSyncActions({ ...base, ref: 'refs/remotes/origin/feature' })
    expect(actions.map(action => action.label)).toEqual(["Pull into 'main' Using Rebase", "Pull into 'main' Using Merge"])
    expect(branchSyncActions({ ...base, ref: 'refs/remotes/origin/feature', head: 'HEAD' }).every(action => !action.enabled)).toBe(true)
    expect(ids({ ...base, ref: 'refs/remotes/origin/HEAD' })).toEqual([])
  })

  it('parses remote refs using the longest configured remote', () => {
    expect(parseRemoteRef('refs/remotes/team/a/feature/x', ['team', 'team/a'])).toEqual({ remote: 'team/a', branch: 'feature/x' })
    expect(parseRemoteRef('refs/remotes/unknown/x', ['origin'])).toBeUndefined()
    expect(parseRemoteRef('refs/heads/main', ['origin'])).toBeUndefined()
  })

  it('describes update outcomes', () => {
    expect(describeUpdate({ branch: 'main', upstream: 'origin/main', current: true, outcome: 'up-to-date' })).toContain('up to date')
    expect(describeUpdate({ branch: 'f', upstream: 'origin/f', current: false, outcome: 'fast-forward' })).toContain('fast-forwarded')
  })
})
