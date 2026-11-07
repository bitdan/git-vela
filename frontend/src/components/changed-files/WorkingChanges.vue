<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { expandAncestors, fileTreeRows } from './fileTree'
import type { WorkingFile } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ files: WorkingFile[]; active?: string; error: string; loading: boolean; canStage?: boolean; canUnstage?: boolean; indexBusy: boolean; notice: string; canAmend: boolean; canResolve?: boolean; amendHash?: string }>()
const emit = defineEmits<{ select: [file: WorkingFile]; updateIndex: [staged: boolean]; commit: []; startAmend: []; cancelAmend: []; stageAll: []; resolve: [] }>()
const message = defineModel<string>('message', { required: true })
const canStageAll = computed(() => props.files.some(file => file.canStage))
const stagedCount = computed(() => props.files.filter(file => file.canUnstage).length)
const canCommit = computed(() => !!message.value.trim() && (stagedCount.value > 0 || !!props.amendHash) && !props.indexBusy)
const collapsed = ref(new Set<string>())
const rows = computed(() => fileTreeRows(props.files, collapsed.value))

watch(() => props.active, path => { if (path) expandAncestors(collapsed.value, path) })

function toggleFolder(path: string) {
  if (collapsed.value.has(path)) collapsed.value.delete(path)
  else collapsed.value.add(path)
}

function state(status: string) {
  if (status === '??') return 'Untracked'
  const descriptions = [status[0] !== ' ' ? 'Index: ' + status[0] : '', status[1] !== ' ' ? 'Working tree: ' + status[1] : '']
  return descriptions.filter(Boolean).join(' · ')
}

/** Called by the global shortcut dispatcher when the commit shortcut fires
 *  while the message field has focus. The keymap stays the single source of
 *  truth for the binding, so a rebound or unbound shortcut can never create
 *  an accidental commit here. */
function submit() {
  if (canCommit.value) emit('commit')
}

defineExpose({ submit })
</script>

<template>
  <section class="panel working-panel" aria-label="Working Changes">
    <header class="panel-header">
      <h2>Working Changes <small>{{ files.length }}</small></h2>
      <div class="index-actions">
        <button :disabled="indexBusy || !canStage" title="Stage the selected file" @click="emit('updateIndex', true)">Stage</button>
        <button :disabled="indexBusy || !canUnstage" title="Unstage the selected file" @click="emit('updateIndex', false)">Unstage</button>
        <button v-if="canResolve" :disabled="indexBusy" title="Stage the selected conflict resolution" @click="emit('resolve')">Mark resolved</button>
        <button :disabled="indexBusy || !canStageAll" title="Stage all working changes" @click="emit('stageAll')">Stage all</button>
      </div>
    </header>
    <p v-if="indexBusy" class="notice" role="status">Running Git…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-else-if="!files.length" class="notice" role="status">{{ loading ? 'Reading working tree…' : 'Working tree is clean.' }}</p>
    <div class="file-scroll">
      <template v-for="row in rows" :key="(row.file ? 'file:' : 'folder:') + row.path">
        <button v-if="row.file" class="working-row" :class="{ active: active === row.path }"
                :style="{ paddingInlineStart: 10 + row.depth * 13 + 'px' }"
                :aria-pressed="active === row.path" :title="row.file.oldPath ? row.file.oldPath + ' → ' + row.path : row.path"
                @click="emit('select', row.file)">
          <code class="file-status" :data-status="row.file.status.trim().at(-1)" :title="state(row.file.status)">{{ row.file.status.trim() }}</code>
          <span class="ellipsis">{{ row.name }}</span>
        </button>
        <button v-else class="working-row folder-row ellipsis" :title="row.path"
                :style="{ paddingInlineStart: 10 + row.depth * 13 + 'px' }"
                :aria-expanded="!collapsed.has(row.path)" @click="toggleFolder(row.path)">
          <span aria-hidden="true">{{ collapsed.has(row.path) ? '▸' : '▾' }}</span>
          <span class="ellipsis">{{ row.name }}</span>
        </button>
      </template>
    </div>
    <form class="commit-form" @submit.prevent="canCommit && emit('commit')">
      <label for="commit-message">{{ amendHash ? 'Amend commit message' : 'Commit message' }}</label>
      <p v-if="amendHash" class="amend-warning" role="status">
        Rewrites HEAD {{ amendHash.slice(0, 8) }} and includes staged changes. Check whether the commit has already been shared.
      </p>
      <textarea id="commit-message" v-model="message" placeholder="Describe the staged changes" rows="2"
                :disabled="indexBusy" />
      <div class="commit-actions">
        <span role="status">{{ stagedCount }} staged<template v-if="notice"> · {{ notice }}</template></span>
        <button class="primary" type="submit" :disabled="!canCommit">{{ amendHash ? 'Amend commit' : 'Commit' }}</button>
        <button v-if="amendHash" type="button" :disabled="indexBusy" @click="emit('cancelAmend')">Cancel amend</button>
        <button v-else type="button" :disabled="indexBusy || !canAmend" @click="emit('startAmend')">Amend…</button>
      </div>
    </form>
  </section>
</template>
