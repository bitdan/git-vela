<script setup lang="ts">
import { onUnmounted, ref, shallowRef, watch } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { ConflictVersion } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; file: string; busy: boolean; unmerged: boolean }>()
const emit = defineEmits<{ close: []; resolve: [] }>()
const versions = shallowRef<ConflictVersion[]>([])
const requests = new LatestRequest()
const loading = ref(false)
const error = ref('')
const labels = ['Base', 'Ours (stage 2)', 'Theirs (stage 3)']

async function load() {
  const call = api.ConflictVersions(props.path, props.file)
  const current = requests.replace(call)
  loading.value = true
  error.value = ''
  versions.value = []
  try {
    const result = await call
    if (current()) versions.value = result ?? []
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}
watch([() => props.path, () => props.file], load, { immediate: true })
onUnmounted(() => requests.clear())
</script>

<template>
  <section class="conflict-panel" aria-label="Conflict versions">
    <div class="editor-tools">
      <strong>{{ unmerged ? 'Conflict versions' : 'Resolution staged' }}</strong>
      <div>
        <button :disabled="busy || loading || !unmerged" @click="load">Refresh</button>
        <button :disabled="busy || !unmerged" @click="emit('resolve')">Mark resolved</button>
        <button @click="emit('close')">Close comparison</button>
      </div>
    </div>
    <p v-if="loading" class="notice" role="status">Loading conflict versions…</p>
    <p v-else-if="error" class="notice error" role="alert">{{ error }}</p>
    <div v-else class="conflict-grid">
      <div v-for="version in versions" :key="version.stage" class="conflict-version">
        <strong>{{ labels[version.stage - 1] }}</strong>
        <pre v-if="version.kind === 'text' || version.kind === 'submodule'" tabindex="0"><code>{{ version.content }}</code></pre>
        <p v-else class="notice">{{ { missing: 'File absent', binary: 'Binary file', large: 'File exceeds preview limits' }[version.kind] ?? version.kind }}</p>
      </div>
    </div>
  </section>
</template>
