<script setup lang="ts">
import { computed } from 'vue'
import type { WorkingFile } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ files: WorkingFile[]; active?: string; error: string; loading: boolean; canStage?: boolean; canUnstage?: boolean; indexBusy: boolean; notice: string }>()
const emit = defineEmits<{ select: [file: WorkingFile]; updateIndex: [staged: boolean]; commit: [] }>()
const message = defineModel<string>('message', { required: true })
const stagedCount = computed(() => props.files.filter(file => file.canUnstage).length)
const canCommit = computed(() => !!message.value.trim() && stagedCount.value > 0 && !props.indexBusy)
const groups = computed(() => {
  const byDirectory = new Map<string, WorkingFile[]>()
  for (const file of props.files) {
    const folder = file.path.slice(0, Math.max(0, file.path.lastIndexOf('/')))
    const entries = byDirectory.get(folder)
    if (entries) entries.push(file)
    else byDirectory.set(folder, [file])
  }
  return [...byDirectory].sort(([left], [right]) => left.localeCompare(right))
})

function state(status: string) {
  if (status === '??') return 'Untracked'
  const descriptions = [status[0] !== ' ' ? 'Index: ' + status[0] : '', status[1] !== ' ' ? 'Working tree: ' + status[1] : '']
  return descriptions.filter(Boolean).join(' · ')
}
</script>

<template>
  <section class="panel working-panel" aria-label="Working Changes">
    <header class="panel-header">
      <h2>Working Changes <small>{{ files.length }}</small></h2>
      <div class="index-actions">
        <button :disabled="indexBusy || !canStage" title="Stage the selected file" @click="emit('updateIndex', true)">Stage</button>
        <button :disabled="indexBusy || !canUnstage" title="Unstage the selected file" @click="emit('updateIndex', false)">Unstage</button>
      </div>
    </header>
    <p v-if="indexBusy" class="notice" role="status">Running Git…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-else-if="!files.length" class="notice" role="status">{{ loading ? 'Reading working tree…' : 'Working tree is clean.' }}</p>
    <div class="file-scroll">
      <details v-for="[folder, entries] in groups" :key="folder" open>
        <summary :title="folder" class="ellipsis">{{ folder || 'Repository root' }} <small>{{ entries.length }}</small></summary>
        <button v-for="file in entries" :key="file.path" class="working-row" :class="{ active: active === file.path }"
                :aria-pressed="active === file.path" :title="file.oldPath ? file.oldPath + ' → ' + file.path : file.path"
                @click="emit('select', file)">
          <code class="file-status" :data-status="file.status.trim().at(-1)" :title="state(file.status)">{{ file.status.trim() }}</code>
          <span class="ellipsis">{{ file.path.split('/').at(-1) }}</span>
        </button>
      </details>
    </div>
    <form class="commit-form" @submit.prevent="canCommit && emit('commit')">
      <label for="commit-message">Commit message</label>
      <textarea id="commit-message" v-model="message" placeholder="Describe the staged changes" rows="2"
                :disabled="indexBusy" @keydown.ctrl.enter.prevent="canCommit && emit('commit')" />
      <div class="commit-actions">
        <span role="status">{{ stagedCount }} staged<template v-if="notice"> · {{ notice }}</template></span>
        <button class="primary" type="submit" :disabled="!canCommit">Commit</button>
      </div>
    </form>
  </section>
</template>
