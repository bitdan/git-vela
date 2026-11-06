<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { eventShortcut, shortcutDefinitions, type ShortcutId } from '../../stores/shortcuts'
import { useKeymap } from '../../stores/keymap'

const keymap = useKeymap()
const groups = computed(() => {
  const ordered: { name: string; items: typeof shortcutDefinitions }[] = []
  for (const item of shortcutDefinitions) {
    let group = ordered.find(entry => entry.name === item.group)
    if (!group) {
      group = { name: item.group, items: [] }
      ordered.push(group)
    }
    group.items.push(item)
  }
  return ordered
})

function start(id: ShortcutId) {
  keymap.error = ''
  keymap.recording = id
}

function cancel(id: ShortcutId) {
  setTimeout(() => {
    if (keymap.recording === id) keymap.recording = undefined
  }, 0)
}

function record(id: ShortcutId, event: KeyboardEvent) {
  const plain = !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey
  if (plain && event.key === 'Escape') {
    keymap.recording = undefined
    keymap.error = ''
    return
  }
  if (plain && (event.key === 'Backspace' || event.key === 'Delete')) {
    keymap.clear(id)
    keymap.recording = undefined
    return
  }
  const combo = eventShortcut(event)
  if (!combo) return
  if (keymap.set(id, combo)) {
    keymap.recording = undefined
    void nextTick(() => document.activeElement instanceof HTMLElement && document.activeElement.blur())
  }
}
</script>

<template>
  <div class="shortcut-settings">
    <p class="muted">Click a shortcut, then press the new keys. Backspace unbinds; Esc cancels.</p>
    <div v-for="group in groups" :key="group.name" class="shortcut-group">
      <strong>{{ group.name }}</strong>
      <label v-for="item in group.items" :key="item.id">
        <span>{{ item.label }}</span>
        <button :class="{ recording: keymap.recording === item.id }" :aria-label="'Change shortcut for ' + item.label"
                @click="start(item.id)" @blur="cancel(item.id)"
                @keydown.stop.prevent="record(item.id, $event)">
          {{ keymap.recording === item.id ? 'Press keys…' : (keymap.bindings[item.id] || 'Unbound') }}
        </button>
      </label>
    </div>
    <p v-if="keymap.error" class="notice error" role="alert">{{ keymap.error }}</p>
    <button @click="keymap.reset(); keymap.recording = undefined">Reset to defaults</button>
  </div>
</template>
