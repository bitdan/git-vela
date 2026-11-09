// Pull integration modes, after IDEA GitPullOption. REBASE, FF_ONLY, NO_FF and
// SQUASH are mutually exclusive there, so they are a single choice here; plain
// merge (no option) is the IDEA default. NO_COMMIT is unavailable with rebase.
export type PullMode = 'merge' | 'ff' | 'no-ff' | 'squash' | 'rebase'

export const pullModes: { value: PullMode; label: string; option: string }[] = [
  { value: 'merge', label: 'Merge', option: 'git pull' },
  { value: 'rebase', label: 'Rebase', option: '--rebase' },
  { value: 'ff', label: 'Fast-forward only', option: '--ff-only' },
  { value: 'no-ff', label: 'Create a merge commit (no fast-forward)', option: '--no-ff' },
  { value: 'squash', label: 'Squash commit', option: '--squash' },
]
