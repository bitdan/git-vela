import { ref, watch } from 'vue'
import { defineStore } from 'pinia'
import {
  assignBinding, defaultBindings, persistBindings, readBindings, resolveShortcut,
  type Bindings, type ShortcutId,
} from './shortcuts'

export const useKeymap = defineStore('keymap', () => {
  const bindings = ref<Bindings>(readBindings())
  const error = ref('')
  // While recording, the application-level listener must not steal the key.
  const recording = ref<ShortcutId>()

  watch(bindings, value => persistBindings(value), { deep: true })

  function set(id: ShortcutId, combo: string) {
    const result = assignBinding(bindings.value, id, combo)
    if (result.bindings) {
      bindings.value = result.bindings
      error.value = ''
      return true
    }
    error.value = result.error ?? 'Invalid shortcut.'
    return false
  }

  function clear(id: ShortcutId) {
    bindings.value = { ...bindings.value, [id]: '' }
    error.value = ''
  }

  function reset() {
    bindings.value = defaultBindings()
    error.value = ''
  }

  function resolve(event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>) {
    return resolveShortcut(event, bindings.value)
  }

  return { bindings, error, recording, set, clear, reset, resolve }
})
