// The expanded diff limit in the Go backend (8 MiB / 120k lines per side) is
// far beyond what the CodeMirror merge view can render smoothly in the
// WebView. Keep the renderer inside a safe budget and show a notice instead
// of freezing the whole window.

/** Maximum rendered lines per side of a diff. */
export const maxRenderLines = 20000
/** Maximum rendered characters per side (a conservative byte estimate). */
export const maxRenderBytes = 2 << 20

function sideLines(side: string): number {
  return side.length ? side.replace(/\n$/, '').split('\n').length : 0
}

/** True when either side of a comparison exceeds the safe render budget. */
export function exceedsRenderLimit(before: string, after: string): boolean {
  for (const side of [before, after]) {
    if (side.length > maxRenderBytes) return true
    if (sideLines(side) > maxRenderLines) return true
  }
  return false
}
