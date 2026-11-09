<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useTemplateRef, watch } from 'vue'
import { filterCommands, type PaletteCommand } from './commandFilter'

const props = defineProps<{ commands: PaletteCommand[] }>()
const emit = defineEmits<{ close: []; run: [id: string] }>()
const input = useTemplateRef<HTMLInputElement>('input')
const results = useTemplateRef<HTMLElement>('results')
const query = ref('')
const active = ref(0)
const matches = computed(() => filterCommands(props.commands, query.value))
const groups = computed(() => {
  const ordered: { name: string; items: { command: PaletteCommand; index: number }[] }[] = []
  const byName = new Map<string, { command: PaletteCommand; index: number }[]>()
  matches.value.forEach((command, index) => {
    let items = byName.get(command.group)
    if (!items) {
      items = []
      byName.set(command.group, items)
      ordered.push({ name: command.group, items })
    }
    items.push({ command, index })
  })
  return ordered
})

watch(matches, () => {
  active.value = 0
  void nextTick(scrollActive)
})
watch(active, () => void nextTick(scrollActive))

function scrollActive() {
  results.value?.querySelector<HTMLElement>('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
}

function choose(index: number) {
  const command = matches.value[index]
  if (!command || command.disabled) return
  emit('run', command.id)
}

function onKeydown(event: KeyboardEvent) {
  // The palette owns the keyboard while it is open.
  event.stopPropagation()
  switch (event.key) {
    case 'Escape':
      event.preventDefault()
      emit('close')
      return
    case 'ArrowDown':
      event.preventDefault()
      if (matches.value.length) active.value = (active.value + 1) % matches.value.length
      return
    case 'ArrowUp':
      event.preventDefault()
      if (matches.value.length) active.value = (active.value - 1 + matches.value.length) % matches.value.length
      return
    case 'Enter':
      event.preventDefault()
      choose(active.value)
      return
    case 'Tab':
      event.preventDefault()
      return
  }
}

onMounted(() => void nextTick(() => input.value?.focus()))
</script>

<template>
  <div class="palette-backdrop" @pointerdown.self="emit('close')">
    <section class="palette" role="dialog" aria-modal="true" aria-label="Command palette" @keydown="onKeydown">
      <div class="palette-search">
        <span aria-hidden="true">⌕</span>
        <input ref="input" v-model="query" aria-label="Search commands" placeholder="Commands, repositories, branches…" spellcheck="false" />
        <button aria-label="Close command palette" @click="emit('close')">Esc</button>
      </div>
      <div ref="results" class="palette-results" role="listbox" aria-label="Matching commands">
        <template v-for="group in groups" :key="group.name">
          <strong class="palette-group">{{ group.name }}</strong>
          <button v-for="entry in group.items" :key="entry.command.id" class="palette-item"
                  :class="{ active: active === entry.index }" :data-active="active === entry.index"
                  :disabled="entry.command.disabled" role="option" :aria-selected="active === entry.index"
                  @pointermove="active = entry.index" @click="choose(entry.index)">
            <span class="ellipsis">{{ entry.command.label }}</span>
            <kbd v-if="entry.command.hint">{{ entry.command.hint }}</kbd>
          </button>
        </template>
        <p v-if="!matches.length" class="notice">No matching commands.</p>
      </div>
      <footer class="palette-footer">↑↓ Navigate · Enter Run · Esc Close</footer>
    </section>
  </div>
</template>
