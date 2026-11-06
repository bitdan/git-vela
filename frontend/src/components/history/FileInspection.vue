<script setup lang="ts">
import { computed, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { BlameLine, FileHistoryPage, Commit } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; file: string; mode: 'history' | 'blame' }>()
const emit = defineEmits<{ close: []; mode: [mode: 'history' | 'blame']; commit: [commit: Commit] }>()
const history = shallowRef<FileHistoryPage>()
const blame = shallowRef<BlameLine[]>([])
const loading = ref(false)
const error = ref('')
const requests = new LatestRequest()
const scroll = useTemplateRef<HTMLElement>('scroll')
const virtual = useVirtualizer(computed(() => ({
  count: props.mode === 'blame' ? blame.value.length : 0,
  getScrollElement: () => scroll.value,
  estimateSize: () => 27,
  overscan: 12,
})))
const blameRows = computed(() => virtual.value.getVirtualItems().flatMap(item => {
  const line = blame.value[item.index]
  return line ? [{ item, line }] : []
}))
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })

async function load() {
  loading.value = true
  error.value = ''
  history.value = undefined
  blame.value = []
  if (props.mode === 'history') {
    const call = api.FileHistory(props.path, props.file)
    const current = requests.replace(call)
    try {
      const result = await call
      if (current()) history.value = result
    } catch (cause) {
      if (current()) error.value = String(cause)
    } finally {
      if (current()) loading.value = false
      requests.finish(call)
    }
  } else {
    const call = api.FileBlame(props.path, props.file)
    const current = requests.replace(call)
    try {
      const result = await call
      if (current()) blame.value = result ?? []
    } catch (cause) {
      if (current()) error.value = String(cause)
    } finally {
      if (current()) loading.value = false
      requests.finish(call)
    }
  }
}

watch([() => props.path, () => props.file, () => props.mode], load, { immediate: true })
onUnmounted(() => requests.clear())
</script>

<template>
  <div class="file-inspection">
    <div class="editor-tools">
      <div>
        <button :aria-pressed="mode === 'history'" @click="emit('mode', 'history')">File History</button>
        <button :aria-pressed="mode === 'blame'" @click="emit('mode', 'blame')">Blame</button>
      </div>
      <div>
        <button :disabled="loading" @click="load">Refresh</button>
        <button @click="emit('close')">Close</button>
      </div>
    </div>
    <p v-if="loading" class="notice" role="status">Loading {{ mode === 'history' ? 'file history' : 'blame' }}…</p>
    <p v-else-if="error" class="notice error" role="alert">{{ error }}</p>
    <div v-else ref="scroll" class="inspection-scroll">
      <template v-if="mode === 'history'">
        <p v-if="!history?.commits?.length" class="notice">No committed history for this path on HEAD.</p>
        <button v-for="commit in history?.commits" :key="commit.hash" class="file-history-row"
                :title="commit.hash" @click="emit('commit', commit)">
          <code>{{ commit.hash.slice(0, 8) }}</code>
          <span class="ellipsis">{{ commit.subject }}</span>
          <span class="ellipsis muted">{{ commit.author }}</span>
          <time class="muted">{{ date.format(commit.time * 1000) }}</time>
        </button>
        <p v-if="history?.more" class="notice">Showing the most recent 200 file commits.</p>
      </template>
      <template v-else>
        <p v-if="!blame.length" class="notice">This file has no lines.</p>
        <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
          <div v-for="{ item, line } in blameRows" :key="line.number" class="blame-row"
               :class="{ uncommitted: line.uncommitted, alternate: line.number % 2 === 0 }"
               :style="{ transform: 'translateY(' + item.start + 'px)' }">
            <code class="muted">{{ line.number }}</code>
            <span class="ellipsis" :title="line.subject">{{ line.uncommitted ? 'Uncommitted' : line.author }}</span>
            <code class="muted" :title="line.hash">{{ line.uncommitted ? '—' : line.hash.slice(0, 8) }}</code>
            <code>{{ line.text }}</code>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
