// TextDiff is loaded asynchronously, so a template ref on it may not be
// available yet. The active diff registers its hunk navigation here instead.

let handler: ((direction: number) => void) | undefined

export function registerDiffNavigation(fn: (direction: number) => void): () => void {
  handler = fn
  return () => {
    if (handler === fn) handler = undefined
  }
}

export function navigateDiff(direction: number): void {
  handler?.(direction)
}
