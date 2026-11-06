import { describe, expect, it } from 'vitest'
import type { Change } from '../../../bindings/gitvela/internal/git'
import { fileTreeRows } from './fileTree'

describe('Changed Files tree', () => {
  it('groups by destination and retains Git file identifiers across collapsed folders', () => {
    const changes = [
      { index: 6, path: 'docs/guides/intro.md', oldPath: 'README.md', status: 'R' },
      { index: 9, path: 'docs.txt', oldPath: '', status: 'A' },
      { index: 2, path: 'docs/setup.md', oldPath: '', status: 'M' },
      { index: 4, path: 'src/app.go', oldPath: '', status: 'D' },
    ] as Change[]
    const rows = fileTreeRows(changes, new Set(['docs/guides']))
    expect(rows.map(row => [row.path, row.depth, row.file?.index])).toEqual([
      ['docs.txt', 0, 9], ['docs', 0, undefined], ['docs/guides', 1, undefined],
      ['docs/setup.md', 1, 2], ['src', 0, undefined], ['src/app.go', 1, 4],
    ])
    expect(fileTreeRows(changes, new Set()).find(row => row.file?.index === 6)?.file).toBe(changes[0])
  })
})
