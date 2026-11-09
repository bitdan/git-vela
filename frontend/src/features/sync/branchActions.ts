// Branch-context synchronization actions, kept free of Vue so availability can
// be unit tested. The rules follow Rebased 1.1.23 (git4idea):
//  - GitUpdateSelectedBranchAction: local branches only, hidden without remotes,
//    disabled without a tracked branch or while it is checked out elsewhere.
//  - GitPushBranchAction: any local branch, current or not.
//  - GitPullBranchAction.WithRebase/WithMerge: remote branches only, integrates
//    into the checked-out branch.

export type UpdateMethod = 'merge' | 'rebase'

export type BranchSyncActionId = 'update' | 'push' | 'pull-rebase' | 'pull-merge'

export interface BranchSyncAction {
  id: BranchSyncActionId
  label: string
  enabled: boolean
  /** Tooltip; explains why a disabled action is unavailable. */
  description: string
}

export interface BranchActionContext {
  /** Full ref name, e.g. refs/heads/main or refs/remotes/origin/main. */
  ref: string
  /** Checked-out branch name, or '' / 'HEAD' when detached. */
  head: string
  hasRemotes: boolean
  /** Upstream short name (origin/main) from BranchSync, '' when untracked. */
  upstream: string
  /** Local branches checked out in other worktrees of this repository. */
  otherWorktreeBranches?: readonly string[]
  updateMethod: UpdateMethod
}

export function localBranchName(ref: string) {
  return ref.startsWith('refs/heads/') ? ref.slice('refs/heads/'.length) : undefined
}

/** Splits refs/remotes/<remote>/<branch>, choosing the longest configured
 *  remote name so remotes containing "/" resolve correctly. */
export function parseRemoteRef(ref: string, remotes: readonly string[]) {
  if (!ref.startsWith('refs/remotes/')) return undefined
  const rest = ref.slice('refs/remotes/'.length)
  const remote = [...remotes].sort((a, b) => b.length - a.length).find(name => rest.startsWith(name + '/'))
  if (!remote) return undefined
  const branch = rest.slice(remote.length + 1)
  return branch && branch !== 'HEAD' ? { remote, branch } : undefined
}

export function currentBranch(head: string) {
  return head && head !== 'HEAD' ? head : undefined
}

export function branchSyncActions(context: BranchActionContext): BranchSyncAction[] {
  const local = localBranchName(context.ref)
  const current = currentBranch(context.head)
  if (local) {
    const actions: BranchSyncAction[] = []
    if (context.hasRemotes) {
      let enabled = true
      let description = `Update selected branch using ${context.updateMethod}`
      if (context.otherWorktreeBranches?.includes(local)) {
        enabled = false
        description = `${local} is checked out in another worktree; update it there`
      } else if (!context.upstream) {
        enabled = false
        description = `Tracking branch isn't configured for ${local}`
      } else if (local !== current) {
        description = `Fast-forward ${local} to ${context.upstream} without checking it out`
      }
      actions.push({ id: 'update', label: 'Update', enabled, description })
    }
    actions.push({ id: 'push', label: 'Push…', enabled: true, description: `Push ${local}` })
    return actions
  }
  if (context.ref.startsWith('refs/remotes/') && !context.ref.endsWith('/HEAD')) {
    const target = current ? `'${current}'` : 'current branch'
    const description = current ? '' : 'Check out a local branch first; HEAD is detached'
    return [
      { id: 'pull-rebase', label: `Pull into ${target} Using Rebase`, enabled: !!current, description: description || `Rebase ${current} onto the selected branch` },
      { id: 'pull-merge', label: `Pull into ${target} Using Merge`, enabled: !!current, description: description || `Merge the selected branch into ${current}` },
    ]
  }
  return []
}

export interface UpdateOutcome {
  branch: string
  upstream: string
  current: boolean
  outcome: string
}

export function describeUpdate(result: UpdateOutcome) {
  switch (result.outcome) {
    case 'up-to-date': return `${result.branch} is up to date with ${result.upstream}.`
    case 'fast-forward': return `${result.branch} fast-forwarded to ${result.upstream}.`
    case 'merged': return `Merged ${result.upstream} into ${result.branch}.`
    case 'rebased': return `Rebased ${result.branch} onto ${result.upstream}.`
    default: return `${result.branch} updated from ${result.upstream}.`
  }
}
