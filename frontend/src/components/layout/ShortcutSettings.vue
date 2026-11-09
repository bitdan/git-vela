<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { recordingDecision, shortcutDefinitions, type ShortcutId } from '../../stores/shortcuts'
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

function onShortcutKey(id: ShortcutId, event: KeyboardEvent) {
  const decision = recordingDecision(keymap.recording, id, event)
  switch (decision.action) {
    case 'none':
    case 'ignore':
      // A focused but not-recording button keeps Enter, Space and Tab so the
      // list stays reachable with the keyboard alone.
      return
    case 'cancel':
      event.preventDefault()
      if (!decision.moveFocus) event.stopPropagation()
      keymap.recording = undefined
      keymap.error = ''
      return
    case 'clear':
      event.preventDefault()
      event.stopPropagation()
      keymap.clear(id)
      keymap.recording = undefined
      return
    case 'assign':
      event.preventDefault()
      event.stopPropagation()
      if (keymap.set(id, decision.combo)) {
        keymap.recording = undefined
        void nextTick(() => document.activeElement instanceof HTMLElement && document.activeElement.blur())
      }
      return
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
                @keydown="onShortcutKey(item.id, $event)">
          {{ keymap.recording === item.id ? 'Press keys…' : (keymap.bindings[item.id] || 'Unbound') }}
        </button>
      </label>
    </div>
    <p v-if="keymap.error" class="notice error" role="alert">{{ keymap.error }}</p>
    <button @click="keymap.reset(); keymap.recording = undefined">Reset to defaults</button>
  </div>
</template>
