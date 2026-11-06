// Session state is stored locally so the next launch reopens the same
// repository, commit and file. Draft editor content is deliberately excluded.

import { safeStorage, type StorageLike } from './shortcuts'

export type { StorageLike }
export const sessionKey = 'gitvela.session'

export type SessionSidebar = 'files' | 'changes' | 'branches' | 'worktrees'
export type SessionView = 'working' | 'unstaged' | 'staged' | 'commit' | 'editor' | 'inspection' | 'conflict'

export interface SessionRange {
  before: string
  after: string
  label: string
}

export interface SessionState {
  path: string
  hash?: string
  parent?: number
  file?: string
  range?: SessionRange
  sidebar?: SessionSidebar
  view?: SessionView
  historyVisible?: boolean
}

const sidebars: SessionSidebar[] = ['files', 'changes', 'branches', 'worktrees']
const views: SessionView[] = ['working', 'unstaged', 'staged', 'commit', 'editor', 'inspection', 'conflict']

function text(value: unknown, max = 4096): string | undefined {
  return typeof value === 'string' && value.length > 0 && value.length <= max ? value : undefined
}

function revision(value: unknown): string | undefined {
  const result = text(value, 64)
  return result && /^[0-9a-fA-F]{7,64}$/.test(result) ? result : undefined
}

export function normalizeSession(raw: unknown): SessionState | undefined {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined
  const value = raw as Record<string, unknown>
  const path = text(value.path)
  if (!path) return undefined
  const session: SessionState = { path }
  const hash = revision(value.hash)
  if (hash) session.hash = hash
  if (typeof value.parent === 'number' && Number.isInteger(value.parent) && value.parent >= 0 && value.parent <= 64) {
    session.parent = value.parent
  }
  const file = text(value.file)
  if (file) session.file = file
  if (value.range && typeof value.range === 'object' && !Array.isArray(value.range)) {
    const range = value.range as Record<string, unknown>
    const before = revision(range.before)
    const after = revision(range.after)
    const label = text(range.label)
    if (before && after && label) session.range = { before, after, label }
  }
  if (typeof value.sidebar === 'string' && sidebars.includes(value.sidebar as SessionSidebar)) {
    session.sidebar = value.sidebar as SessionSidebar
  }
  if (typeof value.view === 'string' && views.includes(value.view as SessionView)) {
    session.view = value.view as SessionView
  }
  if (typeof value.historyVisible === 'boolean') session.historyVisible = value.historyVisible
  return session
}

export function readSession(storage: StorageLike | null = safeStorage()): SessionState | undefined {
  if (!storage) return undefined
  try {
    return normalizeSession(JSON.parse(storage.getItem(sessionKey) ?? 'null'))
  } catch {
    return undefined
  }
}

export function saveSession(state: SessionState, storage: StorageLike | null = safeStorage()): void {
  try {
    storage?.setItem(sessionKey, JSON.stringify(normalizeSession(state) ?? { path: state.path }))
  } catch {
    // A restricted WebView may deny storage; the session is a convenience only.
  }
}

export function clearSession(storage: StorageLike | null = safeStorage()): void {
  try {
    if (storage?.removeItem) storage.removeItem(sessionKey)
    else storage?.setItem(sessionKey, '')
  } catch {
    // Ignore storage failures.
  }
}
