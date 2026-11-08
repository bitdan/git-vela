<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import { fileTreeRows } from '../changed-files/fileTree'
import { LatestRequest } from '../../stores/latestRequest'
import { usePreferences } from '../../stores/preferences'

const props = defineProps<{ path: string; active: boolean; selected?: string }>()
const emit = defineEmits<{ open: [path: string] }>()
const preferences = usePreferences()
const files = shallowRef<string[]>([])
const query = ref('')
const collapsed = ref(new Set<string>())
const expanded = new Set<string>()
const loading = ref(false)
const error = ref('')
const list = useTemplateRef<HTMLElement>('list')
const requests = new LatestRequest()
let unsubscribe: (() => void) | undefined
const filtered = computed(() => {
  const text = query.value.trim().toLocaleLowerCase()
  return text ? files.value.filter(file => file.toLocaleLowerCase().includes(text)) : files.value
})
const rows = computed(() => fileTreeRows(filtered.value.map(path => ({ path })), query.value.trim() ? expanded : collapsed.value))
function toggleFolder(path: string) {
  if (collapsed.value.has(path)) collapsed.value.delete(path)
  else collapsed.value.add(path)
}

const virtual = useVirtualizer(computed(() => ({
  count: rows.value.length,
  getScrollElement: () => props.active ? list.value : null,
  estimateSize: () => preferences.repoRowHeight,
  overscan: 10,
  getItemKey: index => {
    const row = rows.value[index]
    return row ? (row.file ? 'file:' : 'folder:') + row.path : index
  },
})))

const visibleRows = computed(() => virtual.value.getVirtualItems().flatMap(item => {
  const row = rows.value[item.index]
  return row ? [{ item, row }] : []
}))

async function load() {
  const call = api.EditableFiles(props.path)
  const current = requests.replace(call)
  loading.value = true
  error.value = ''
  try {
    const result = await call
    if (current()) files.value = result ?? []
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}
watch(() => props.active, active => { if (active) void load() }, { immediate: true })
watch(query, () => { if (list.value) list.value.scrollTop = 0 })
onMounted(() => {
  unsubscribe = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === props.path && props.active && !loading.value) void load()
  })
})
onUnmounted(() => { unsubscribe?.(); requests.clear() })
</script>

<template>
  <section class="panel repository-files" aria-label="Repository files">
    <header class="panel-header">
      <h2>Files <small>{{ filtered.length }}</small></h2>
      <button :disabled="loading" @click="load">Refresh</button>
    </header>
    <p class="files-root" :title="path">
      <strong class="ellipsis">{{ path.split(/[\\/]/).at(-1) || path }}</strong>
      <span class="ellipsis muted">{{ path }}</span>
    </p>
    <div class="ref-filter">
      <input v-model="query" aria-label="Filter repository files" placeholder="Filter by file path" spellcheck="false" />
    </div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-else-if="loading && !files.length" class="notice" role="status">Reading files…</p>
    <p v-else-if="!filtered.length" class="notice">{{ query ? 'No matching files.' : 'No files in this repository.' }}</p>
    <div ref="list" class="file-scroll">
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <template v-for="{ item, row } in visibleRows" :key="String(item.key)">
          <button v-if="row.file" class="repository-file-row ellipsis" :class="{ active: selected === row.path }"
                  :title="row.path" :aria-pressed="selected === row.path"
                  :style="{ transform: 'translateY(' + item.start + 'px)', paddingInlineStart: 10 + row.depth * 13 + 'px' }"
                  @click="emit('open', row.path)">{{ row.name }}</button>
          <button v-else class="repository-file-row folder-row ellipsis" :title="row.path" :disabled="!!query.trim()"
                  :aria-expanded="!!query.trim() || !collapsed.has(row.path)"
                  :style="{ transform: 'translateY(' + item.start + 'px)', paddingInlineStart: 10 + row.depth * 13 + 'px' }"
                  @click="toggleFolder(row.path)">
            <span aria-hidden="true">{{ !query.trim() && collapsed.has(row.path) ? '▸' : '▾' }}</span>
            {{ row.name }}
          </button>
        </template>
      </div>
    </div>
  </section>
</template>
