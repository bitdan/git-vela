import { describe, expect, it } from 'vitest'
import { clearSession, normalizeSession, readSession, saveSession, sessionKey, type StorageLike } from './session'

function memory(initial: Record<string, string> = {}): StorageLike & { data: Record<string, string> } {
  const data = { ...initial }
  return {
    data,
    getItem: key => data[key] ?? null,
    setItem: (key, value) => { data[key] = value },
    removeItem: key => { delete data[key] },
  }
}

const hash = 'a'.repeat(40)
const before = 'b'.repeat(40)

describe('session validation', () => {
  it('requires a repository path', () => {
    expect(normalizeSession(null)).toBeUndefined()
    expect(normalizeSession([])).toBeUndefined()
    expect(normalizeSession({})).toBeUndefined()
    expect(normalizeSession({ path: '' })).toBeUndefined()
  })

  it('keeps valid fields and discards malformed ones', () => {
    const result = normalizeSession({
      path: 'F:/work/repo', hash, parent: 2, file: 'src/app.ts', workingFile: 'src/main.go',
      range: { before, after: hash, label: 'main → HEAD' },
      sidebar: 'branches', view: 'commit', historyVisible: false,
      extra: 'ignored',
    })
    expect(result).toEqual({
      path: 'F:/work/repo', hash, parent: 2, file: 'src/app.ts', workingFile: 'src/main.go',
      range: { before, after: hash, label: 'main → HEAD' },
      sidebar: 'branches', view: 'commit', historyVisible: false,
    })
  })

  it('drops invalid revisions, parent indexes, sidebars and views', () => {
    const result = normalizeSession({
      path: 'repo', hash: 'not-a-hash', parent: -1, workingFile: '',
      range: { before: 'short', after: hash, label: 'range' },
      sidebar: 'unknown', view: 'unknown', historyVisible: 'no',
    })
    expect(result).toEqual({ path: 'repo' })
  })
})

describe('session storage', () => {
  it('round-trips a saved session', () => {
    const storage = memory()
    saveSession({ path: 'F:/work/repo', hash, parent: 1, sidebar: 'files', view: 'working', workingFile: 'src/app.ts' }, storage)
    expect(readSession(storage)).toEqual({ path: 'F:/work/repo', hash, parent: 1, sidebar: 'files', view: 'working', workingFile: 'src/app.ts' })
  })

  it('ignores corrupt JSON and missing storage', () => {
    expect(readSession(memory({ [sessionKey]: '{broken' }))).toBeUndefined()
    expect(readSession(null)).toBeUndefined()
    expect(readSession(memory())).toBeUndefined()
  })

  it('clears the session', () => {
    const storage = memory({ [sessionKey]: JSON.stringify({ path: 'repo' }) })
    clearSession(storage)
    expect(readSession(storage)).toBeUndefined()
  })
})
