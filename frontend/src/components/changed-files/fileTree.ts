import type { Change } from '../../../bindings/gitvela/internal/git'

interface FileTreeRow {
  path: string
  name: string
  depth: number
  file?: Change
}

export function fileTreeRows(changes: Change[], collapsed: ReadonlySet<string>): FileTreeRow[] {
  const sorted = [...changes].sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : 0)
  const rows: FileTreeRow[] = []
  const seen = new Set<string>()
  let hiddenPrefix = ''
  for (const file of sorted) {
    if (hiddenPrefix && file.path.startsWith(hiddenPrefix)) continue
    hiddenPrefix = ''
    const segments = file.path.split('/')
    let folder = ''
    for (let depth = 0; depth < segments.length - 1; depth++) {
      const name = segments[depth]
      folder += name
      if (!seen.has(folder)) {
        seen.add(folder)
        rows.push({ path: folder, name, depth })
      }
      folder += '/'
      if (collapsed.has(folder.slice(0, -1))) {
        hiddenPrefix = folder
        break
      }
    }
    if (!hiddenPrefix) {
      rows.push({ path: file.path, name: segments[segments.length - 1], depth: segments.length - 1, file })
    }
  }
  return rows
}
