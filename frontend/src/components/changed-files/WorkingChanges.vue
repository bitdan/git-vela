<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
import ProjectNodeIcon from '../editor/ProjectNodeIcon.vue'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import { expandAncestors, fileTreeRows } from './fileTree'
import type { WorkingFile } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ files: WorkingFile[]; active?: string; error: string; loading: boolean; canStage?: boolean; canUnstage?: boolean; indexBusy: boolean; notice: string; canAmend: boolean; canResolve?: boolean; amendHash?: string; amendAuthor?: string; amendAuthorEmail?: string; amendDate?: number; history: string[] }>()
const emit = defineEmits<{ select: [file: WorkingFile, view?: 'working' | 'staged' | 'unstaged']; edit: [path: string]; gitActions: [path: string, anchor: ContextAnchor]; updateIndex: [staged: boolean]; include: [paths: string[], staged: boolean]; commit: []; commitAndPush: []; refresh: []; startAmend: []; cancelAmend: []; stageAll: []; resolve: []; ignore: [path: string]; stash: [paths: string[]] }>()
const message = defineModel<string>('message', { required: true })
/** Amend options (G12): both default to off, which matches plain `git commit
 *  --amend` (keeps the author, adds no trailer). */
const resetAuthor = defineModel<boolean>('amendResetAuthor', { default: false })
const signOff = defineModel<boolean>('amendSignOff', { default: false })
const amendDateLabel = computed(() => props.amendDate
  ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(props.amendDate * 1000))
  : '')
const canStageAll = computed(() => props.files.some(file => file.canStage))
const stagedCount = computed(() => props.files.filter(file => file.canUnstage).length)
const canCommit = computed(() => !!message.value.trim() && (stagedCount.value > 0 || !!props.amendHash) && !props.indexBusy)
const collapsed = ref(new Set<string>())
const changesFolded = ref(false)
const activeFile = computed(() => props.files.find(file => file.path === props.active))
const toolbar = computed(() => [
  { label: 'Refresh changes', icon: 'M13 6a5 5 0 1 0 0 4m0-8v4H9', disabled: props.indexBusy || props.loading, run: () => emit('refresh') },
  { label: 'Stage', title: 'Stage the selected file', icon: 'M2.5 11.5v2h11v-2M8 10V2.5m-3 3 3-3 3 3', disabled: props.indexBusy || !props.canStage, run: () => emit('updateIndex', true) },
  { label: 'Unstage', title: 'Unstage the selected file', icon: 'M2.5 4.5v-2h11v2M8 6v7.5m-3-3 3 3 3-3', disabled: props.indexBusy || !props.canUnstage, run: () => emit('updateIndex', false) },
  { label: 'Stage all', title: 'Stage all working changes', icon: 'm2 8 3 3 6-6m-3 6 6-6', disabled: props.indexBusy || !canStageAll.value, run: () => emit('stageAll') },
  { label: 'Show diff', icon: 'M1.5 8s2.5-4 6.5-4 6.5 4 6.5 4-2.5 4-6.5 4-6.5-4-6.5-4zM8 6v4m-2-2h4', disabled: !activeFile.value, run: showSelectedDiff },
  { label: 'Expand all', icon: 'm5 6 3-3 3 3m-6 4 3 3 3-3', disabled: !props.files.length, run: () => { changesFolded.value = false; collapsed.value.clear() } },
  { label: 'Collapse all', icon: 'm5 3 3 3 3-3m-6 10 3-3 3 3', disabled: !props.files.length, run: () => { changesFolded.value = true } },
])

function showSelectedDiff() {
  const file = activeFile.value
  if (file) emit('select', file, file.status === '??' ? 'working' : file.canStage ? 'unstaged' : 'staged')
}
const menu = shallowRef<{ path: string; anchor: ContextAnchor }>()
const menuFile = computed(() => props.files.find(file => file.path === menu.value?.path))
const historyMenu = shallowRef<ContextAnchor>()

function showActions(file: WorkingFile, event: MouseEvent | KeyboardEvent) {
  menu.value = { path: file.path, anchor: contextAnchor(event) }
}

function fileAction(action: 'working' | 'staged' | 'unstaged' | 'edit' | 'stage' | 'unstage' | 'ignore' | 'stash') {
  const file = menuFile.value
  menu.value = undefined
  if (!file) return
  if (action === 'edit') emit('edit', file.path)
  else if (action === 'stage' || action === 'unstage') emit('include', [file.path], action === 'stage')
  else if (action === 'ignore') emit('ignore', file.path)
  else if (action === 'stash') emit('stash', [file.path])
  else emit('select', file, action)
}

/** Recent commit messages (IDEA commit message history): picking one fills
 *  the message field so it can be edited before submitting. */
function showHistory(event: MouseEvent | KeyboardEvent) {
  if (props.history.length) historyMenu.value = contextAnchor(event)
}

function pickHistory(text: string) {
  historyMenu.value = undefined
  message.value = text
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

watch(() => props.active, path => {
  if (!path) return
  changesFolded.value = false
  expandAncestors(collapsed.value, path)
})

function toggleFolder(path: string) {
  if (collapsed.value.has(path)) collapsed.value.delete(path)
  else collapsed.value.add(path)
}

function state(status: string) {
  if (status === '??') return 'Untracked'
  const descriptions = [status[0] !== ' ' ? 'Index: ' + status[0] : '', status[1] !== ' ' ? 'Working tree: ' + status[1] : '']
  return descriptions.filter(Boolean).join(' · ')
}

/** Amend is presented as an IDEA-style checkbox; toggling it enters or leaves
 *  the amend flow driven by the parent. */
function onAmendToggle(on: boolean) {
  if (on) emit('startAmend')
  else emit('cancelAmend')
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
    <header class="commit-toolbar index-actions" role="toolbar" aria-label="Changes tools">
      <button v-for="tool in toolbar" :key="tool.label" type="button" :disabled="tool.disabled"
              :aria-label="tool.label" :title="tool.title ?? tool.label" @click="tool.run">
        <svg class="tool-glyph" viewBox="0 0 16 16" aria-hidden="true"><path :d="tool.icon" /></svg>
        <span class="visually-hidden">{{ tool.label }}</span>
      </button>
      <button v-if="canResolve" class="resolve-tool" :disabled="indexBusy"
              title="Stage the selected conflict resolution" @click="emit('resolve')">Mark resolved</button>
    </header>
    <p v-if="indexBusy" class="notice" role="status">Running Git…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-else-if="!files.length" class="notice" role="status">{{ loading ? 'Reading working tree…' : 'Working tree is clean.' }}</p>
    <div class="file-scroll commit-changes-scroll" @scroll.passive="menu = undefined">
      <div v-if="files.length" class="changes-root">
        <button class="changes-toggle" type="button" :aria-expanded="!changesFolded" aria-label="Toggle changes tree"
                @click="changesFolded = !changesFolded">{{ changesFolded ? '›' : '⌄' }}</button>
        <label class="changes-heading">
          <input type="checkbox" aria-label="Include all changes in commit"
                 :checked="groups.get('')?.checked" :indeterminate="groups.get('')?.mixed"
                 :disabled="indexBusy || loading || !groups.get('')?.files.length" @click.prevent="toggleGroup('')" />
          <span>Changes <small>{{ files.length }} {{ files.length === 1 ? 'file' : 'files' }}</small></span>
        </label>
      </div>
      <template v-if="!changesFolded">
        <div v-for="row in rows" :key="(row.file ? 'file:' : 'folder:') + row.path" class="working-tree-row"
             :class="{ 'selected-change': active === row.path }"
             :style="{ paddingInlineStart: 21 + row.depth * 13 + 'px' }">
          <template v-if="row.file">
            <input type="checkbox" :aria-label="'Include ' + row.path + ' in commit'" :checked="row.file.canUnstage"
                   :title="row.file.canStage && row.file.canUnstage ? 'Partially staged' : undefined"
                   :disabled="indexBusy || loading || (!row.file.canStage && !row.file.canUnstage)"
                   @click.prevent="emit('include', [row.path], !row.file.canUnstage)" />
            <button class="working-row" :class="{ active: active === row.path }"
                    :aria-pressed="active === row.path" :title="row.file.oldPath ? row.file.oldPath + ' → ' + row.path : row.path"
                    @click="emit('select', row.file)"
                    @dblclick="emit('select', row.file, row.file.canStage ? 'unstaged' : 'staged')"
                    @keydown.f4.exact.prevent.stop="emit('edit', row.file.path)"
                    @contextmenu.prevent="showActions(row.file, $event)" @keydown.shift.f10.prevent="showActions(row.file, $event)">
              <code class="file-status" :data-status="row.file.status.trim()" :title="state(row.file.status)">{{ row.file.status.trim() }}</code>
              <ProjectNodeIcon :name="row.name" :folder="false" />
              <span class="ellipsis change-name">{{ row.name }}</span>
              <span v-if="row.path.includes('/')" class="ellipsis muted change-parent">{{ row.path.slice(0, row.path.lastIndexOf('/')) }}</span>
            </button>
          </template>
          <template v-else>
            <input type="checkbox" :aria-label="'Include folder ' + row.path + ' in commit'"
                   :checked="groups.get(row.path)?.checked" :indeterminate="groups.get(row.path)?.mixed"
                   :disabled="indexBusy || loading || !groups.get(row.path)?.files.length"
                   @click.prevent="toggleGroup(row.path)" />
            <button class="working-row folder-row ellipsis" :title="row.path"
                    :aria-expanded="!collapsed.has(row.path)" @click="toggleFolder(row.path)">
              <span class="changes-chevron" aria-hidden="true">{{ collapsed.has(row.path) ? '›' : '⌄' }}</span>
              <ProjectNodeIcon :name="row.name" :folder="true" />
              <span class="ellipsis">{{ row.name }}</span>
            </button>
          </template>
        </div>
      </template>
    </div>
    <form class="commit-form" @submit.prevent="canCommit && emit('commit')">
      <label class="amend-row">
        <input type="checkbox" :checked="!!amendHash" :disabled="indexBusy || (!amendHash && !canAmend)"
               aria-label="Amend last commit"
               :data-amend-action="amendHash ? 'cancelAmend' : 'startAmend'"
               @change="onAmendToggle(($event.target as HTMLInputElement).checked)" />
        <span>Amend last commit</span>
      </label>
      <p v-if="amendHash" class="amend-warning" role="status">
        Rewrites HEAD {{ amendHash.slice(0, 8) }} and includes staged changes. Check whether the commit has already been shared.
      </p>
      <div v-if="amendHash && amendAuthor" class="amend-meta">
        <span class="muted">Author {{ amendAuthor }}<template v-if="amendAuthorEmail"> &lt;{{ amendAuthorEmail }}&gt;</template><template v-if="amendDateLabel"> · {{ amendDateLabel }}</template></span>
        <label title="Set the author to your configured Git identity and the author date to now">
          <input v-model="resetAuthor" type="checkbox" /> Reset author
        </label>
        <label title="Append a Signed-off-by trailer from your configured Git identity">
          <input v-model="signOff" type="checkbox" /> Sign off
        </label>
      </div>
      <textarea id="commit-message" v-model="message" :placeholder="amendHash ? 'Describe the amended changes' : 'Describe the staged changes'" rows="5"
                :disabled="indexBusy" />
      <div v-if="history.length" class="commit-history">
        <button type="button" :aria-label="history.length + ' recent commit messages'" title="Recent commit messages"
                @click="showHistory($event)" @keydown.shift.f10.prevent="showHistory($event)">History</button>
      </div>
      <ContextActions v-if="historyMenu" :anchor="historyMenu" label="Recent commit messages" @close="historyMenu = undefined">
        <button v-for="(item, index) in history" :key="index" class="commit-history-item" :title="item"
                @click="pickHistory(item)">{{ item.split('\n')[0] }}</button>
      </ContextActions>
      <div class="commit-actions">
        <span role="status">{{ stagedCount }} staged<template v-if="notice"> · {{ notice }}</template></span>
        <div class="commit-buttons">
          <button class="primary" type="submit" :disabled="!canCommit">{{ amendHash ? 'Amend commit' : 'Commit' }}</button>
          <button type="button" :disabled="!canCommit" @click="emit('commitAndPush')">Commit and Push…</button>
        </div>
      </div>
    </form>
    <ContextActions v-if="menu && menuFile" :anchor="menu.anchor" label="Working file actions" @close="menu = undefined">
      <button @click="fileAction('working')">Show Working File</button>
      <button :disabled="!menuFile.canStage" @click="fileAction('unstaged')">Show Unstaged Diff</button>
      <button :disabled="!menuFile.canUnstage" @click="fileAction('staged')">Show Staged Diff</button>
      <button @click="fileAction('edit')" aria-keyshortcuts="F4">Jump to Source</button>
      <button @click="emit('gitActions', menuFile.path, menu.anchor); menu = undefined">More Git Actions…</button>
      <hr />
      <button :disabled="indexBusy || loading || !menuFile.canStage" @click="fileAction('stage')">Stage</button>
      <button :disabled="indexBusy || loading || !menuFile.canUnstage" @click="fileAction('unstage')">Unstage</button>
      <button :disabled="indexBusy || loading || (!menuFile.canStage && !menuFile.canUnstage)"
              title="Stash only this file's changes" @click="fileAction('stash')">Stash file changes…</button>
      <button v-if="menuFile.status === '??'" :disabled="indexBusy || loading"
              title="Append this path to the repository .gitignore" @click="fileAction('ignore')">Add to .gitignore</button>
    </ContextActions>
  </section>
</template>
