interface FileTreeRow<T> {
  path: string
  name: string
  depth: number
  file?: T
}

const nameOrder = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function compareNames(a: string, b: string): number {
  return nameOrder.compare(a, b) || (a < b ? -1 : a > b ? 1 : 0)
}

export function fileTreeRows<T extends { path: string }>(changes: readonly T[], collapsed: ReadonlySet<string>): FileTreeRow<T>[] {
  const sorted = changes.map(file => ({ file, segments: file.path.split('/') }))
  sorted.sort((a, b) => {
    for (let depth = 0; depth < Math.min(a.segments.length, b.segments.length); depth++) {
      const aFolder = depth < a.segments.length - 1
      const bFolder = depth < b.segments.length - 1
      if (aFolder !== bFolder) return aFolder ? -1 : 1
      const order = compareNames(a.segments[depth], b.segments[depth])
      if (order) return order
    }
    return 0
  })
  const rows: FileTreeRow<T>[] = []
  const seen = new Set<string>()
  let hiddenPrefix = ''
  for (const { file, segments } of sorted) {
    if (hiddenPrefix && file.path.startsWith(hiddenPrefix)) continue
    hiddenPrefix = ''
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

/** Removes the collapsed state from every ancestor of a path so keyboard
 *  navigation can reveal a file that is currently hidden by its folders. */
export function expandAncestors(collapsed: Set<string>, path: string): void {
  const segments = path.split('/')
  let folder = ''
  for (let depth = 0; depth < segments.length - 1; depth++) {
    folder += segments[depth]
    collapsed.delete(folder)
    folder += '/'
  }
}
