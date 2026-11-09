/** Protected branch patterns (IDEA "Protected branches"): exact names or
 *  shell-style wildcards. The backend repeats this check, so the UI copy only
 *  explains why a force push is unavailable. */
export function matchesProtectedBranch(patterns: readonly string[], branch: string): boolean {
  return patterns.some(pattern => {
    const value = pattern.trim()
    if (!value) return false
    if (value === branch) return true
    const escaped = value.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.')
    return new RegExp('^' + escaped + '$').test(branch)
  })
}
