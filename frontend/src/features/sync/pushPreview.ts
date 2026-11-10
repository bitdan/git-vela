/** Push dialog rules for the outgoing-commit list (IDEA push dialog,
 *  GitOutgoingCommitsProvider). Kept pure so they are unit-tested. */

export interface OutgoingPreview {
  tip: string
  newBranch: boolean
  commits: { hash: string }[] | null
  more: boolean
}

export interface PushTarget {
  remote: string
  target: string
}

/** IDEA disables Push when the remote branch already contains everything,
 *  unless the push creates the branch or deliberately rewrites it. */
export function canConfirmPush(preview: OutgoingPreview | undefined, force: boolean): boolean {
  if (!preview) return false
  return preview.newBranch || force || (preview.commits?.length ?? 0) > 0
}

export function outgoingSummary(preview: OutgoingPreview, destination: PushTarget): string {
  const count = preview.commits?.length ?? 0
  const where = destination.remote + '/' + destination.target
  const commits = (preview.more ? 'More than ' + count : String(count)) + (count === 1 && !preview.more ? ' commit' : ' commits')
  if (preview.newBranch) {
    return count ? commits + ' not on ' + destination.remote + ' yet; this creates ' + where + '.'
      : 'Creates ' + where + '; every commit is already on ' + destination.remote + '.'
  }
  return count ? commits + ' to push to ' + where + '.' : where + ' already contains this branch; nothing to push.'
}

export function shortHash(hash: string): string {
  return hash.slice(0, 8)
}
