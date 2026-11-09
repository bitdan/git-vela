<script setup lang="ts">
import { computed, nextTick, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { storeToRefs } from 'pinia'
import { usePreferences } from '../../stores/preferences'
import { useWorkspace } from '../../stores/workspace'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import { expandAncestors, fileTreeRows } from './fileTree'
import type { Change } from '../../../bindings/gitvela/internal/git'

const store = useWorkspace()
const emit = defineEmits<{ select: []; openDiff: [file: Change]; maximize: []; edit: [path: string] }>()
const list = useTemplateRef<HTMLElement>('list')
const preferences = usePreferences()
const { fileTree: tree } = storeToRefs(preferences)
const collapsed = ref(new Set<string>())
const menu = shallowRef<{ file: Change; anchor: ContextAnchor }>()

function showActions(file: Change, event: MouseEvent | KeyboardEvent) {
  menu.value = { file, anchor: contextAnchor(event) }
}

function fileAction(action: 'diff' | 'maximize' | 'edit') {
  const file = menu.value?.file
  menu.value = undefined
  if (!file) return
  if (action === 'edit') emit('edit', file.path)
  else {
    select(file)
    if (action === 'maximize') emit('maximize')
  }
}
const rows = computed(() => tree.value
  ? fileTreeRows(store.files, collapsed.value)
  : store.files.map(file => ({ path: file.path, name: file.path, depth: 0, file })))

function toggleFolder(path: string) {
  if (collapsed.value.has(path)) collapsed.value.delete(path)
  else collapsed.value.add(path)
}

function select(file: Change) {
  emit('select')
  if (store.file?.index !== file.index || store.diffError) void store.selectFile(file)
}

function openDiff(file: Change) {
  select(file)
  emit('openDiff', file)
}

/** Alt+Up/Down navigation over changed files, independent of tree folders. */
function step(direction: number) {
  const files = store.files
  if (!files.length) return
  const index = files.findIndex(file => file.index === store.file?.index)
  const base = index < 0 ? (direction > 0 ? -1 : files.length) : index
  const next = files[Math.max(0, Math.min(files.length - 1, base + direction))]
  if (!next) return
  select(next)
  // A collapsed folder hides the target row; expand its ancestors first so
  // the file is visible and can be scrolled into view.
  expandAncestors(collapsed.value, next.path)
  const row = rows.value.findIndex(item => item.file?.index === next.index)
  if (row >= 0) void nextTick(() => virtual.value.scrollToIndex(row, { align: 'auto' }))
}

defineExpose({ step })

const virtual = useVirtualizer(computed(() => ({
  count: rows.value.length,
  getScrollElement: () => list.value,
  estimateSize: () => preferences.rowHeight,
  getItemKey: index => rows.value[index]?.file ? 'file:' + rows.value[index]?.file?.index : 'folder:' + rows.value[index]?.path,
  overscan: 8,
})))
const visibleRows = computed(() => virtual.value.getVirtualItems().flatMap(item => {
  const row = rows.value[item.index]
  return row ? [{ item, row }] : []
}))
watch([() => store.selected?.hash, () => store.parent, () => store.comparisonRange], () => { collapsed.value.clear(); menu.value = undefined })
watch([() => store.selected?.hash, () => store.parent, () => store.comparisonRange, tree], () => {
  if (list.value) list.value.scrollTop = 0
})
</script>

<template>
  <section class="panel files-panel" aria-label="Changed Files">
    <header class="panel-header">
      <div><span class="eyebrow">{{ store.comparisonRange ? 'COMPARE REVISIONS' : 'HISTORY' }}</span><h2>Changed Files <small>{{ store.files.length }}</small></h2></div>
      <button :aria-pressed="tree" @click="tree = !tree">{{ tree ? 'Tree' : 'Flat' }}</button>
    </header>
    <p v-if="store.loadingFiles" class="notice" role="status">Loading changed files…</p>
    <p v-else-if="store.filesError" class="notice error" role="alert">{{ store.filesError }}</p>
    <p v-else-if="!store.files.length" class="notice">{{ (store.selected || store.comparisonRange) ? 'No changed files.' : 'Select a commit.' }}</p>
    <div ref="list" class="file-scroll" @scroll.passive="menu = undefined">
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <template v-for="{ item, row } in visibleRows" :key="String(item.key)">
          <button v-if="row.file" class="file-row"
                  :class="{ active: store.file?.index === row.file.index }" :aria-pressed="store.file?.index === row.file.index"
                  :style="{ transform: 'translateY(' + item.start + 'px)', paddingInlineStart: 8 + row.depth * 12 + 'px' }"
                  :title="row.file.oldPath ? row.file.oldPath + ' → ' + row.path : row.path"
                  @click="select(row.file)" @dblclick="openDiff(row.file)"
                  @keydown.f4.exact.prevent.stop="emit('edit', row.file.path)"
                  @contextmenu.prevent="showActions(row.file, $event)" @keydown.shift.f10.prevent="showActions(row.file, $event)">
            <code class="file-status" :data-status="row.file.status">{{ row.file.status }}</code>
            <span class="ellipsis">{{ row.file.oldPath ? row.file.oldPath + ' → ' : '' }}{{ row.name }}</span>
          </button>
          <button v-else class="file-row folder-row" :aria-expanded="!collapsed.has(row.path)" :title="row.path"
                  :style="{ transform: 'translateY(' + item.start + 'px)', paddingInlineStart: 8 + row.depth * 12 + 'px' }"
                  @click="toggleFolder(row.path)">
            <span aria-hidden="true">{{ collapsed.has(row.path) ? '▸' : '▾' }}</span>
            <span class="ellipsis">{{ row.name }}</span>
          </button>
        </template>
      </div>
    </div>
    <ContextActions v-if="menu" :anchor="menu.anchor" label="Changed file actions" @close="menu = undefined">
      <button @click="fileAction('diff')">Show Diff</button>
      <button @click="fileAction('maximize')">Maximize Diff</button>
      <hr />
      <button @click="fileAction('edit')" aria-keyshortcuts="F4">Jump to Source</button>
    </ContextActions>
  </section>
</template>
