<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import { expandAncestors, fileTreeRows } from './fileTree'
import type { WorkingFile } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ files: WorkingFile[]; active?: string; error: string; loading: boolean; canStage?: boolean; canUnstage?: boolean; indexBusy: boolean; notice: string; canAmend: boolean; canResolve?: boolean; amendHash?: string }>()
const emit = defineEmits<{ select: [file: WorkingFile, view?: 'working' | 'staged' | 'unstaged']; edit: [path: string]; updateIndex: [staged: boolean]; include: [paths: string[], staged: boolean]; commit: []; startAmend: []; cancelAmend: []; stageAll: []; resolve: [] }>()
const message = defineModel<string>('message', { required: true })
const canStageAll = computed(() => props.files.some(file => file.canStage))
const stagedCount = computed(() => props.files.filter(file => file.canUnstage).length)
const canCommit = computed(() => !!message.value.trim() && (stagedCount.value > 0 || !!props.amendHash) && !props.indexBusy)
const collapsed = ref(new Set<string>())
const menu = shallowRef<{ path: string; anchor: ContextAnchor }>()
const menuFile = computed(() => props.files.find(file => file.path === menu.value?.path))

function showActions(file: WorkingFile, event: MouseEvent | KeyboardEvent) {
  menu.value = { path: file.path, anchor: contextAnchor(event) }
}

function fileAction(action: 'working' | 'staged' | 'unstaged' | 'edit' | 'stage' | 'unstage') {
  const file = menuFile.value
  menu.value = undefined
  if (!file) return
  if (action === 'edit') emit('edit', file.path)
  else if (action === 'stage' || action === 'unstage') emit('include', [file.path], action === 'stage')
  else emit('select', file, action)
}

const rows = computed(() => fileTreeRows(props.files, collapsed.value))

const groups = computed(() => {
  const filesByFolder = new Map<string, WorkingFile[]>([['', []]])
  for (const file of props.files) {
    if (!file.canStage && !file.canUnstage) continue
    filesByFolder.get('')!.push(file)
    let folder = file.path
    while (folder.includes('/')) {
      folder = folder.slice(0, folder.lastIndexOf('/'))
      const files = filesByFolder.get(folder)
      if (files) files.push(file)
      else filesByFolder.set(folder, [file])
    }
  }
  return new Map([...filesByFolder].map(([path, files]) => [path, {
    checked: files.length > 0 && files.every(file => file.canUnstage),
    mixed: files.some(file => file.canUnstage) && files.some(file => !file.canUnstage),
    files,
  }]))
})

function toggleGroup(path: string) {
  const group = groups.value.get(path)
  if (!group) return
  // An included file may be partially staged. Keep its staged hunks as-is.
  const files = group.checked ? group.files : group.files.filter(file => !file.canUnstage)
  emit('include', files.map(file => file.path), !group.checked)
}

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
      <h2><label class="changes-heading">
        <input type="checkbox" aria-label="Include all changes in commit"
               :checked="groups.get('')?.checked" :indeterminate="groups.get('')?.mixed"
               :disabled="indexBusy || loading || !groups.get('')?.files.length" @click.prevent="toggleGroup('')" />
        <span>Working Changes <small>{{ files.length }}</small></span>
      </label></h2>
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
    <div class="file-scroll" @scroll.passive="menu = undefined">
      <div v-for="row in rows" :key="(row.file ? 'file:' : 'folder:') + row.path" class="working-tree-row"
           :style="{ paddingInlineStart: 10 + row.depth * 13 + 'px' }">
        <template v-if="row.file">
          <input type="checkbox" :aria-label="'Include ' + row.path + ' in commit'" :checked="row.file.canUnstage"
                 :title="row.file.canStage && row.file.canUnstage ? 'Partially staged' : undefined"
                 :disabled="indexBusy || loading || (!row.file.canStage && !row.file.canUnstage)"
                 @click.prevent="emit('include', [row.path], !row.file.canUnstage)" />
          <button class="working-row" :class="{ active: active === row.path }"
                  :aria-pressed="active === row.path" :title="row.file.oldPath ? row.file.oldPath + ' → ' + row.path : row.path"
                  @click="emit('select', row.file)"
                  @contextmenu.prevent="showActions(row.file, $event)" @keydown.shift.f10.prevent="showActions(row.file, $event)">
            <code class="file-status" :data-status="row.file.status.trim().at(-1)" :title="state(row.file.status)">{{ row.file.status.trim() }}</code>
            <span class="ellipsis">{{ row.name }}</span>
          </button>
        </template>
        <template v-else>
          <input type="checkbox" :aria-label="'Include folder ' + row.path + ' in commit'"
                 :checked="groups.get(row.path)?.checked" :indeterminate="groups.get(row.path)?.mixed"
                 :disabled="indexBusy || loading || !groups.get(row.path)?.files.length"
                 @click.prevent="toggleGroup(row.path)" />
          <button class="working-row folder-row ellipsis" :title="row.path"
                  :aria-expanded="!collapsed.has(row.path)" @click="toggleFolder(row.path)">
            <span aria-hidden="true">{{ collapsed.has(row.path) ? '▸' : '▾' }}</span>
            <span class="ellipsis">{{ row.name }}</span>
          </button>
        </template>
      </div>
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
    <ContextActions v-if="menu && menuFile" :anchor="menu.anchor" label="Working file actions" @close="menu = undefined">
      <button @click="fileAction('working')">Show Working File</button>
      <button :disabled="!menuFile.canStage" @click="fileAction('unstaged')">Show Unstaged Diff</button>
      <button :disabled="!menuFile.canUnstage" @click="fileAction('staged')">Show Staged Diff</button>
      <button @click="fileAction('edit')">Edit Working File</button>
      <hr />
      <button :disabled="indexBusy || loading || !menuFile.canStage" @click="fileAction('stage')">Stage</button>
      <button :disabled="indexBusy || loading || !menuFile.canUnstage" @click="fileAction('unstage')">Unstage</button>
    </ContextActions>
  </section>
</template>
