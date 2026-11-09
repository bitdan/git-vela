// Pure shortcut helpers. Kept free of Vue and the DOM so they can be tested
// without mounting a component. Bindings are stored as canonical strings such
// as "Ctrl+K", "Alt+Up" or "F11". An empty string means "unbound".

export type ShortcutId =
  | 'palette' | 'openRepository' | 'refresh' | 'searchHistory' | 'commit'
  | 'previousCommit' | 'nextCommit' | 'previousChange' | 'nextChange'
  | 'previousHunk' | 'nextHunk' | 'toggleHistory' | 'maximizeDiff'
  | 'toggleTheme' | 'toggleDensity' | 'toggleFullscreen'

export interface ShortcutDefinition {
  id: ShortcutId
  label: string
  group: string
  binding: string
}

export const shortcutDefinitions: ShortcutDefinition[] = [
  { id: 'palette', label: 'Command palette', group: 'General', binding: 'Ctrl+K' },
  { id: 'openRepository', label: 'Browse for a repository', group: 'Repository', binding: 'Ctrl+O' },
  { id: 'refresh', label: 'Refresh repository', group: 'Repository', binding: 'Alt+R' },
  { id: 'searchHistory', label: 'Search Git history', group: 'Repository', binding: 'Ctrl+F' },
  { id: 'commit', label: 'Focus commit message', group: 'Repository', binding: 'Ctrl+Enter' },
  { id: 'previousCommit', label: 'Previous commit', group: 'Navigation', binding: 'Alt+PageUp' },
  { id: 'nextCommit', label: 'Next commit', group: 'Navigation', binding: 'Alt+PageDown' },
  { id: 'previousChange', label: 'Previous changed file', group: 'Navigation', binding: 'Alt+Up' },
  { id: 'nextChange', label: 'Next changed file', group: 'Navigation', binding: 'Alt+Down' },
  { id: 'previousHunk', label: 'Previous diff hunk', group: 'Navigation', binding: 'Alt+Shift+Up' },
  { id: 'nextHunk', label: 'Next diff hunk', group: 'Navigation', binding: 'Alt+Shift+Down' },
  { id: 'toggleHistory', label: 'Show or hide history panels', group: 'View', binding: 'Ctrl+H' },
  { id: 'maximizeDiff', label: 'Maximize or restore diff', group: 'View', binding: 'Ctrl+M' },
  { id: 'toggleTheme', label: 'Toggle dark theme', group: 'View', binding: 'Ctrl+Shift+T' },
  { id: 'toggleDensity', label: 'Toggle compact rows', group: 'View', binding: 'Ctrl+Shift+D' },
  { id: 'toggleFullscreen', label: 'Toggle fullscreen', group: 'View', binding: 'F11' },
]

export type Bindings = Record<ShortcutId, string>

export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem?(key: string): void
}

export const keymapKey = 'gitvela.keymap'

export function safeStorage(): StorageLike | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

const keyAliases: Record<string, string> = {
  ' ': 'Space', space: 'Space', spacebar: 'Space',
  esc: 'Esc', escape: 'Esc',
  up: 'Up', down: 'Down', left: 'Left', right: 'Right',
  arrowup: 'Up', arrowdown: 'Down', arrowleft: 'Left', arrowright: 'Right',
  pageup: 'PageUp', pagedown: 'PageDown',
  del: 'Delete', delete: 'Delete',
  return: 'Enter', enter: 'Enter',
  ctrlbreak: 'Pause', pause: 'Pause',
}

function modifier(name: string): string | null {
  switch (name.toLowerCase()) {
    case 'ctrl': case 'control': case 'mod': case 'cmd': case 'command': case 'meta':
      return 'Ctrl'
    case 'alt': case 'option':
      return 'Alt'
    case 'shift':
      return 'Shift'
    default:
      return null
  }
}

function normalizeKey(raw: string): string | null {
  const value = raw.trim()
  if (!value) return null
  if (modifier(value)) return null
  const alias = keyAliases[value.toLowerCase()]
  if (alias) return alias
  if (value.length === 1) return value.toUpperCase()
  if (/^f([1-9]|1[0-2])$/i.test(value)) return value.toUpperCase()
  return value.charAt(0).toUpperCase() + value.slice(1)
}

/** Normalises user-entered shortcuts such as "alt + arrowup" to "Alt+Up". */
export function parseShortcut(input: string): string | null {
  const parts = input.split('+').map(part => part.trim()).filter(Boolean)
  if (!parts.length) return null
  const key = normalizeKey(parts[parts.length - 1])
  if (!key) return null
  const mods = new Set<string>()
  for (const part of parts.slice(0, -1)) {
    const name = modifier(part)
    if (!name || mods.has(name)) return null
    mods.add(name)
  }
  return [...['Ctrl', 'Alt', 'Shift'].filter(name => mods.has(name)), key].join('+')
}

/** Builds the same canonical string from a keyboard event. */
export function eventShortcut(event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>): string | null {
  const key = normalizeKey(event.key)
  if (!key) return null
  const mods: string[] = []
  if (event.ctrlKey || event.metaKey) mods.push('Ctrl')
  if (event.altKey) mods.push('Alt')
  if (event.shiftKey) mods.push('Shift')
  return [...mods, key].join('+')
}

export function defaultBindings(): Bindings {
  return Object.fromEntries(shortcutDefinitions.map(item => [item.id, item.binding])) as Bindings
}

export function readBindings(storage: StorageLike | null = safeStorage()): Bindings {
  const bindings = defaultBindings()
  if (!storage) return bindings
  try {
    const raw: unknown = JSON.parse(storage.getItem(keymapKey) ?? '{}')
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return bindings
    for (const [id, value] of Object.entries(raw as Record<string, unknown>)) {
      if (!(id in bindings) || typeof value !== 'string') continue
      const combo = value === '' ? '' : parseShortcut(value)
      if (combo !== null) bindings[id as ShortcutId] = combo
    }
  } catch {
    // A corrupt preference must never stop the application from starting.
  }
  return bindings
}

export function persistBindings(bindings: Bindings, storage: StorageLike | null = safeStorage()): void {
  try {
    storage?.setItem(keymapKey, JSON.stringify(bindings))
  } catch {
    // Storage may be unavailable in a restricted WebView.
  }
}

export function shortcutLabel(id: ShortcutId): string {
  return shortcutDefinitions.find(item => item.id === id)?.label ?? id
}

/** Returns a new binding map, or an error when the combo is invalid or taken. */
export function assignBinding(bindings: Bindings, id: ShortcutId, combo: string): { bindings?: Bindings; error?: string } {
  const value = combo === '' ? '' : parseShortcut(combo)
  if (value === null) return { error: 'Use a key with optional Ctrl, Alt or Shift modifiers.' }
  if (value) {
    const owner = (Object.keys(bindings) as ShortcutId[]).find(other => other !== id && bindings[other] === value)
    if (owner) return { error: `Already used by “${shortcutLabel(owner)}”.` }
  }
  return { bindings: { ...bindings, [id]: value } }
}

export function resolveShortcut(event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>, bindings: Bindings): ShortcutId | undefined {
  const combo = eventShortcut(event)
  if (!combo) return undefined
  return (Object.keys(bindings) as ShortcutId[]).find(id => bindings[id] && bindings[id] === combo)
}

/** What the shortcut settings should do with a key pressed on its button. */
export type RecordingDecision =
  | { action: 'none' }                      // Not recording: let the browser activate the button.
  | { action: 'ignore' }                    // Recording, but the key maps to no shortcut.
  | { action: 'cancel'; moveFocus: boolean } // Esc (or Tab) leaves recording mode.
  | { action: 'clear' }                     // Backspace or Delete unbinds the command.
  | { action: 'assign'; combo: string }     // Bind the pressed key combination.

/** Pure recording rules so the settings panel stays keyboard accessible:
 *  a focused-but-not-recording button keeps Enter, Space and Tab behaviour,
 *  and Tab during recording cancels and moves focus instead of binding. */
export function recordingDecision(
  recording: ShortcutId | undefined, id: ShortcutId,
  event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>,
): RecordingDecision {
  if (recording !== id) return { action: 'none' }
  const plain = !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey
  if (plain && event.key === 'Escape') return { action: 'cancel', moveFocus: false }
  if (event.key === 'Tab') return { action: 'cancel', moveFocus: true }
  if (plain && (event.key === 'Backspace' || event.key === 'Delete')) return { action: 'clear' }
  const combo = eventShortcut(event)
  if (!combo) return { action: 'ignore' }
  return { action: 'assign', combo }
}

/** Commands that move through lists; plain text fields keep the arrow-style
 *  keys for caret movement, while modifier combos still dispatch globally. */
export const navigationShortcuts: ReadonlySet<ShortcutId> = new Set([
  'previousCommit', 'nextCommit', 'previousChange', 'nextChange', 'previousHunk', 'nextHunk',
])

/** Commands owned by the CodeMirror panes (the text editor and the diff
 *  viewer). Inside those panes the global dispatcher must not run them as
 *  well, or one keypress would trigger the editor keymap and the workspace
 *  command at the same time — for example Ctrl+F would open both the editor
 *  search panel and the Git history search. */
export const editorScopeShortcuts: ReadonlySet<ShortcutId> = new Set(['searchHistory', 'commit'])

/** Text fields keep their own navigation keys; CodeMirror panes do not. */
export function isTextEntry(target: EventTarget | null): boolean {
  if (typeof HTMLElement === 'undefined' || !(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return target.isContentEditable && !target.closest('.cm-editor')
}
