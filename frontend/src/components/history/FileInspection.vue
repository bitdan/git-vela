<script setup lang="ts">
import { onUnmounted, ref, shallowRef, watch } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { FileHistoryPage, Commit } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; file: string; lines?: { start: number; end: number } }>()
const emit = defineEmits<{ close: []; blame: []; commit: [commit: Commit] }>()
const history = shallowRef<FileHistoryPage>()
const loading = ref(false)
const error = ref('')
const requests = new LatestRequest()
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })

async function load() {
  loading.value = true
  error.value = ''
  history.value = undefined
  const call = props.lines
    ? api.FileSelectionHistory(props.path, props.file, props.lines.start, props.lines.end)
    : api.FileHistory(props.path, props.file)
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
}

watch([() => props.path, () => props.file, () => props.lines], load, { immediate: true })
onUnmounted(() => requests.clear())
</script>

<template>
  <div class="file-inspection">
    <div class="file-history-tools">
      <strong>{{ lines ? 'History for lines ' + lines.start + '–' + lines.end : 'File History' }} · {{ file }}</strong>
      <button @click="emit('blame')">Annotate with Git Blame</button>
      <button :disabled="loading" @click="load">Refresh</button>
      <button @click="emit('close')">Close</button>
    </div>
    <p v-if="loading" class="notice" role="status">Loading file history…</p>
    <p v-else-if="error" class="notice error" role="alert">{{ error }}</p>
    <div v-else class="inspection-scroll">
      <p v-if="!history?.commits?.length" class="notice">No committed history for this selection on HEAD.</p>
      <button v-for="commit in history?.commits" :key="commit.hash" class="file-history-row"
              :title="commit.hash" @click="emit('commit', commit)">
        <code>{{ commit.hash.slice(0, 8) }}</code>
        <span class="ellipsis">{{ commit.subject }}</span>
        <span class="ellipsis muted">{{ commit.author }}</span>
        <time class="muted">{{ date.format(commit.time * 1000) }}</time>
      </button>
      <p v-if="history?.more" class="notice">Showing the most recent 200 file commits.</p>
    </div>
  </div>
</template>
