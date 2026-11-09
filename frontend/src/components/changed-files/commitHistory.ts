// Commit message history (IDEA CommitMessageHistory counterpart): the last
// messages the user submitted are remembered per repository so they can be
// picked again from the commit form. Only the message text is stored.

import { safeStorage, type StorageLike } from '../../stores/shortcuts'

export const commitHistoryKey = 'gitvela.commitHistory'
export const commitHistoryLimit = 20

type CommitHistoryMap = Record<string, string[]>

/** Pure core: move `message` to the front of `history`, deduplicated,
 *  trimmed and capped at `max` entries. Empty messages are dropped. */
export function pushCommitMessage(history: string[], message: string, max = commitHistoryLimit): string[] {
  const text = message.trim()
  if (!text) return history
  const rest = history.filter(item => item !== text)
  return [text, ...rest].slice(0, Math.max(1, max))
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every(item => typeof item === 'string')
}

function readMap(storage: StorageLike | null): CommitHistoryMap {
  if (!storage?.getItem) return {}
  try {
    const raw = JSON.parse(storage.getItem(commitHistoryKey) ?? '{}') as unknown
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {}
    const map: CommitHistoryMap = {}
    for (const [repo, list] of Object.entries(raw as Record<string, unknown>)) {
      if (!repo || !isStringArray(list)) continue
      const clean = list.filter(item => item.trim()).slice(0, commitHistoryLimit)
      if (clean.length) map[repo] = clean
    }
    return map
  } catch {
    return {}
  }
}

function writeMap(storage: StorageLike | null, map: CommitHistoryMap) {
  if (!storage?.setItem) return
  try {
    storage.setItem(commitHistoryKey, JSON.stringify(map))
  } catch {
    // A restricted WebView may deny storage; the history is a convenience.
  }
}

export function readCommitHistory(storage: StorageLike | null | undefined, repo: string): string[] {
  if (!repo) return []
  return readMap(storage ?? safeStorage())[repo] ?? []
}

/** Persists the submitted message and returns the updated history so callers
 *  can keep a reactive copy in sync. */
export function rememberCommitMessage(
  storage: StorageLike | null | undefined,
  repo: string,
  message: string,
  history: string[] = [],
  max = commitHistoryLimit,
): string[] {
  if (!repo) return history
  const store = storage ?? safeStorage()
  const map = readMap(store)
  // Prefer the caller's reactive history; the stored copy is a fallback.
  const next = pushCommitMessage(history.length ? history : map[repo] ?? [], message, max)
  map[repo] = next
  writeMap(store, map)
  return next
}

export function clearCommitHistory(storage: StorageLike | null = safeStorage(), repo?: string) {
  if (!repo) {
    try {
      storage?.removeItem?.(commitHistoryKey)
    } catch {
      // Ignore storage failures.
    }
    return
  }
  const map = readMap(storage)
  if (!map[repo]) return
  delete map[repo]
  writeMap(storage, map)
}
