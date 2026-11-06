<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; active: boolean; selected?: string }>()
const emit = defineEmits<{ open: [path: string] }>()
const files = shallowRef<string[]>([])
const query = ref('')
const loading = ref(false)
const error = ref('')
const list = useTemplateRef<HTMLElement>('list')
const requests = new LatestRequest()
let unsubscribe: (() => void) | undefined
const filtered = computed(() => {
  const text = query.value.trim().toLocaleLowerCase()
  return text ? files.value.filter(file => file.toLocaleLowerCase().includes(text)) : files.value
})
const virtual = useVirtualizer(computed(() => ({
  count: filtered.value.length,
  getScrollElement: () => props.active ? list.value : null,
  estimateSize: () => 28,
  overscan: 10,
})))

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
    <div class="ref-filter">
      <input v-model="query" aria-label="Filter repository files" placeholder="Filter by file path" spellcheck="false" />
    </div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-else-if="loading && !files.length" class="notice" role="status">Reading files…</p>
    <p v-else-if="!filtered.length" class="notice">{{ query ? 'No matching files.' : 'No files in this repository.' }}</p>
    <div ref="list" class="file-scroll">
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <button v-for="item in virtual.getVirtualItems()" :key="item.index" class="repository-file-row ellipsis"
                :class="{ active: selected === filtered[item.index] }" :title="filtered[item.index]"
                :style="{ transform: 'translateY(' + item.start + 'px)' }" @click="emit('open', filtered[item.index])">
          {{ filtered[item.index] }}
        </button>
      </div>
    </div>
  </section>
</template>
