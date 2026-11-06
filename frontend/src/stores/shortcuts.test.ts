import { describe, expect, it } from 'vitest'
import {
  assignBinding, defaultBindings, editorScopeShortcuts, eventShortcut, navigationShortcuts,
  parseShortcut, persistBindings, readBindings, recordingDecision, resolveShortcut,
  shortcutDefinitions, type StorageLike,
} from './shortcuts'

function memory(initial: Record<string, string> = {}): StorageLike & { data: Record<string, string> } {
  const data = { ...initial }
  return {
    data,
    getItem: key => data[key] ?? null,
    setItem: (key, value) => { data[key] = value },
    removeItem: key => { delete data[key] },
  }
}

function keyEvent(key: string, modifiers: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean }> = {}) {
  return { key, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...modifiers }
}

describe('shortcut normalisation', () => {
  it('normalises aliases, order and case', () => {
    expect(parseShortcut('alt + arrowup')).toBe('Alt+Up')
    expect(parseShortcut('SHIFT+ctrl+K')).toBe('Ctrl+Shift+K')
    expect(parseShortcut('cmd+f')).toBe('Ctrl+F')
    expect(parseShortcut('F11')).toBe('F11')
    expect(parseShortcut('ctrl+pageDown')).toBe('Ctrl+PageDown')
  })

  it('rejects modifier-only, duplicate and empty shortcuts', () => {
    expect(parseShortcut('')).toBeNull()
    expect(parseShortcut('Ctrl')).toBeNull()
    expect(parseShortcut('Ctrl+Shift')).toBeNull()
    expect(parseShortcut('Ctrl+Alt+Ctrl+K')).toBeNull()
  })

  it('matches canonical keyboard events, including Cmd as Ctrl', () => {
    expect(eventShortcut(keyEvent('k', { ctrlKey: true }))).toBe('Ctrl+K')
    expect(eventShortcut(keyEvent('K', { metaKey: true }))).toBe('Ctrl+K')
    expect(eventShortcut(keyEvent('ArrowDown', { altKey: true, shiftKey: true }))).toBe('Alt+Shift+Down')
    expect(eventShortcut(keyEvent('Control'))).toBeNull()
  })
})

describe('stored bindings', () => {
  it('uses defaults when storage is empty or corrupt', () => {
    expect(readBindings(memory())).toEqual(defaultBindings())
    expect(readBindings(memory({ 'gitvela.keymap': 'not json' }))).toEqual(defaultBindings())
    expect(readBindings(null)).toEqual(defaultBindings())
  })

  it('merges valid overrides and ignores unknown ids or invalid values', () => {
    const bindings = readBindings(memory({
      'gitvela.keymap': JSON.stringify({ palette: 'ctrl+shift+p', nextChange: '', bogus: 'Ctrl+Q', refresh: 'Ctrl' }),
    }))
    expect(bindings.palette).toBe('Ctrl+Shift+P')
    expect(bindings.nextChange).toBe('')
    expect(bindings.refresh).toBe(defaultBindings().refresh)
    expect(Object.keys(bindings)).toHaveLength(shortcutDefinitions.length)
  })

  it('persists and reads back a custom keymap', () => {
    const storage = memory()
    const bindings = { ...defaultBindings(), palette: 'Ctrl+Shift+P' }
    persistBindings(bindings, storage)
    expect(readBindings(storage).palette).toBe('Ctrl+Shift+P')
  })
})

describe('assignment and resolution', () => {
  it('refuses to steal a shortcut from another command', () => {
    const result = assignBinding(defaultBindings(), 'palette', 'Alt+Up')
    expect(result.bindings).toBeUndefined()
    expect(result.error).toContain('Previous changed file')
  })

  it('allows rebinding and unbinding, and resolves the current map', () => {
    const assigned = assignBinding(defaultBindings(), 'palette', 'Ctrl+Shift+P')
    expect(assigned.bindings?.palette).toBe('Ctrl+Shift+P')
    const bindings = assigned.bindings!
    expect(resolveShortcut(keyEvent('p', { ctrlKey: true, shiftKey: true }), bindings)).toBe('palette')
    expect(resolveShortcut(keyEvent('k', { ctrlKey: true }), bindings)).toBeUndefined()
    expect(assignBinding(bindings, 'palette', '').bindings?.palette).toBe('')
  })
})

describe('shortcut scopes', () => {
  it('keeps list navigation out of plain text fields', () => {
    for (const id of ['previousCommit', 'nextCommit', 'previousChange', 'nextChange', 'previousHunk', 'nextHunk'] as const) {
      expect(navigationShortcuts.has(id)).toBe(true)
    }
    expect(navigationShortcuts.has('refresh')).toBe(false)
    expect(navigationShortcuts.has('toggleTheme')).toBe(false)
  })

  it('marks editor-owned commands so the global dispatcher skips them', () => {
    expect(editorScopeShortcuts.has('searchHistory')).toBe(true)
    expect(editorScopeShortcuts.has('commit')).toBe(true)
    expect(editorScopeShortcuts.has('palette')).toBe(false)
    expect(editorScopeShortcuts.has('openRepository')).toBe(false)
    for (const id of editorScopeShortcuts) {
      expect(shortcutDefinitions.some(item => item.id === id)).toBe(true)
    }
  })
})

describe('recording decisions', () => {
  it('ignores keys when the command is not being recorded', () => {
    expect(recordingDecision('palette', 'refresh', keyEvent('Tab'))).toEqual({ action: 'none' })
    expect(recordingDecision(undefined, 'refresh', keyEvent('Enter'))).toEqual({ action: 'none' })
  })

  it('cancels on Escape and Tab, and clears on Backspace or Delete', () => {
    expect(recordingDecision('palette', 'palette', keyEvent('Escape'))).toEqual({ action: 'cancel', moveFocus: false })
    expect(recordingDecision('palette', 'palette', keyEvent('Tab'))).toEqual({ action: 'cancel', moveFocus: true })
    expect(recordingDecision('palette', 'palette', keyEvent('Tab', { shiftKey: true }))).toEqual({ action: 'cancel', moveFocus: true })
    expect(recordingDecision('palette', 'palette', keyEvent('Backspace'))).toEqual({ action: 'clear' })
    expect(recordingDecision('palette', 'palette', keyEvent('Delete'))).toEqual({ action: 'clear' })
  })

  it('assigns the pressed combo and ignores bare modifiers', () => {
    expect(recordingDecision('palette', 'palette', keyEvent('p', { ctrlKey: true, shiftKey: true })))
      .toEqual({ action: 'assign', combo: 'Ctrl+Shift+P' })
    expect(recordingDecision('palette', 'palette', keyEvent('Delete', { shiftKey: true })))
      .toEqual({ action: 'assign', combo: 'Shift+Delete' })
    expect(recordingDecision('palette', 'palette', keyEvent('Control'))).toEqual({ action: 'ignore' })
  })
})
