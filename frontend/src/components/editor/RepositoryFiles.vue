<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Clipboard, Events } from '@wailsio/runtime'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { ProjectEntry } from '../../../bindings/gitvela/internal/git'
import { compareNames, fileTreeRows } from '../changed-files/fileTree'
import AppIcon from '../layout/AppIcon.vue'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import { LatestRequest } from '../../stores/latestRequest'
import { usePreferences } from '../../stores/preferences'
import ProjectNodeIcon from './ProjectNodeIcon.vue'
import ProjectSearchDialog from './ProjectSearchDialog.vue'

type TreeRow = ProjectEntry & { name: string; depth: number }
type Mutation = { action: 'file' | 'folder' | 'copy' | 'move' | 'delete'; folder: string; source?: ProjectEntry; rename?: boolean }

const props = defineProps<{ path: string; active: boolean; selected?: string; dirtyPaths: string[]; blocked: boolean }>()
const busy = defineModel<boolean>('busy', { required: true })
const emit = defineEmits<{ open: [path: string, line?: number]; gitMenu: [path: string, anchor: ContextAnchor]; changed: [source?: string, destination?: string] }>()
const preferences = usePreferences()
const directories = shallowRef<Record<string, ProjectEntry[]>>({})
const openFolders = ref(new Set(['.']))
const highlighted = ref('.')
const query = ref('')
const showFilter = ref(false)
const filteredPaths = shallowRef<string[]>([])
const loading = ref(false)
const error = ref('')
const menu = shallowRef<{ entry: ProjectEntry; anchor: ContextAnchor }>()
const clipboard = shallowRef<{ entry: ProjectEntry; cut: boolean }>()
const mutation = shallowRef<Mutation>()
const name = ref('')
const mutationError = ref('')
const searchFolder = ref<string>()
const list = useTemplateRef<HTMLElement>('list')
const filterInput = useTemplateRef<HTMLInputElement>('filter')
const pathSearch = new LatestRequest()
const requests = new Map<string, LatestRequest>()
const rootEntry: ProjectEntry = { path: '.', folder: true, revision: '' }
let unsubscribe: (() => void) | undefined

const basename = (path: string) => path.split(/[\\/]/).at(-1) || path
const parentOf = (path: string) => path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '.'
const inside = (path: string, folder: string) => path === folder || path.startsWith(folder + '/')
const menuFolder = computed(() => menu.value?.entry.folder ? menu.value.entry.path : parentOf(menu.value?.entry.path ?? '.'))
const selectedEntry = computed(() => rows.value.find(row => row.path === highlighted.value) ?? rootEntry)
const unavailable = computed(() => props.blocked || busy.value)
const rowHeight = computed(() => Math.min(24, preferences.repoRowHeight))

const rows = computed<TreeRow[]>(() => {
  const root = { ...rootEntry, name: basename(props.path), depth: 0 }
  const term = query.value.trim().toLocaleLowerCase()
  if (term) {
    const entries = new Map(Object.values(directories.value).flat().map(entry => [entry.path, entry]))
    const paths = [...new Set([...filteredPaths.value, ...entries.keys()])]
      .filter(path => path.toLocaleLowerCase().includes(term) && !entries.get(path)?.folder)
    return [root, ...fileTreeRows(paths.map(path => ({ path })), new Set()).map(row => ({
      ...row, depth: row.depth + 1, folder: !row.file, revision: entries.get(row.path)?.revision ?? '',
    }))]
  }
  const result = [root]
  function append(folder: string, depth: number) {
    if (!openFolders.value.has(folder)) return
    for (const entry of directories.value[folder] ?? []) {
      result.push({ ...entry, name: basename(entry.path), depth })
      if (entry.folder) append(entry.path, depth + 1)
    }
  }
  append('.', 1)
  return result
})

const virtual = useVirtualizer(computed(() => ({
  count: rows.value.length,
  getScrollElement: () => props.active ? list.value : null,
  estimateSize: () => rowHeight.value,
  getItemKey: index => rows.value[index]?.path ?? index,
  overscan: 12,
})))

async function load(folder: string, report = true) {
  const request = requests.get(folder) ?? new LatestRequest()
  requests.set(folder, request)
  const call = api.ProjectEntries(props.path, folder)
  const current = request.replace(call)
  try {
    const entries = await call
    if (current()) {
      directories.value = { ...directories.value, [folder]: (entries ?? [])
        .sort((a, b) => Number(b.folder) - Number(a.folder) || compareNames(basename(a.path), basename(b.path))) }
    }
  } catch (cause) {
    if (current() && report) error.value = String(cause)
  } finally {
    request.finish(call)
  }
}

async function refresh() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    await Promise.all([...openFolders.value].map(folder => load(folder, folder === '.')))
  } finally {
    loading.value = false
  }
}

async function toggle(folder: string) {
  if (query.value.trim()) return
  if (openFolders.value.delete(folder)) return
  openFolders.value.add(folder)
  await load(folder)
}

async function reveal() {
  const file = props.selected
  if (!file) return
  query.value = ''
  openFolders.value.add('.')
  await load('.')
  const segments = file.split('/')
  for (let index = 1; index < segments.length; index++) {
    const folder = segments.slice(0, index).join('/')
    openFolders.value.add(folder)
    await load(folder, false)
  }
  highlighted.value = file
  await nextTick()
  const index = rows.value.findIndex(row => row.path === file)
  if (index >= 0) virtual.value.scrollToIndex(index, { align: 'auto' })
}

function filterFiles() {
  showFilter.value = !showFilter.value
  if (showFilter.value) void nextTick(() => filterInput.value?.focus())
  else query.value = ''
}

async function loadFilteredPaths(value: string) {
  if (!value.trim()) {
    pathSearch.clear()
    return
  }
  showFilter.value = true
  const call = api.EditableFiles(props.path)
  const current = pathSearch.replace(call)
  try {
    const paths = await call
    if (current()) filteredPaths.value = paths ?? []
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    pathSearch.finish(call)
  }
}

watch(query, value => {
  if (list.value) list.value.scrollTop = 0
  void loadFilteredPaths(value)
})

watch(() => props.active, active => {
  if (active) void refresh()
  else menu.value = undefined
}, { immediate: true })
onMounted(() => {
  unsubscribe = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === props.path && props.active && !unavailable.value) void refresh()
  })
})
onUnmounted(() => {
  unsubscribe?.()
  pathSearch.clear()
  requests.forEach(request => request.clear())
})

function showMenu(entry: ProjectEntry, event: MouseEvent | KeyboardEvent) {
  highlighted.value = entry.path
  const context = { entry, anchor: contextAnchor(event) }
  menu.value = context
  if (entry.path !== '.' && !entry.revision) {
    const parent = parentOf(entry.path)
    void load(parent).then(() => {
      const item = directories.value[parent]?.find(candidate => candidate.path === entry.path)
      if (menu.value === context && item) menu.value = { ...context, entry: item }
    })
  }
}

function openRow(row: TreeRow) {
  highlighted.value = row.path
  if (row.folder) void toggle(row.path)
  else emit('open', row.path)
}

function isDirty(entry?: ProjectEntry) {
  return !!entry && props.dirtyPaths.some(path => inside(path, entry.path))
}

function edit(action: Mutation['action'], folder: string, source?: ProjectEntry, rename = false) {
  menu.value = undefined
  if (unavailable.value || ((action === 'delete' || action === 'move') && isDirty(source))) return
  mutation.value = { action, folder, source, rename }
  mutationError.value = ''
  name.value = source ? basename(source.path) : ''
}

function paste(folder: string) {
  const item = clipboard.value
  if (item) edit(item.cut ? 'move' : 'copy', folder, item.entry)
}

function remember(entry: ProjectEntry, cut: boolean) {
  if (!entry.revision || (cut && isDirty(entry))) return
  clipboard.value = { entry, cut }
  menu.value = undefined
}

const mutationTitle = computed(() => {
  const request = mutation.value
  if (request?.rename) return 'Rename'
  return request ? { file: 'New File', folder: 'New Directory', copy: 'Copy to Directory', move: 'Move to Directory', delete: 'Delete from Disk' }[request.action] : ''
})

async function apply() {
  const request = mutation.value
  if (!request || unavailable.value || (request.action !== 'delete' && !name.value.trim())) return
  if ((request.action === 'move' || request.action === 'delete') && isDirty(request.source)) return
  busy.value = true
  mutationError.value = ''
  const leaf = name.value.trim()
  const destination = request.folder === '.' ? leaf : request.folder + '/' + leaf
  try {
    await api.ChangeProjectEntry(props.path, request.action, request.source?.path ?? '', request.source?.revision ?? '', request.folder, leaf)
    mutation.value = undefined
    if (request.source && (request.action === 'move' || request.action === 'delete')) {
      const source = request.source.path
      if (clipboard.value?.entry.path === source) clipboard.value = undefined
      for (const folder of openFolders.value) if (inside(folder, source)) openFolders.value.delete(folder)
      emit('changed', source, request.action === 'move' ? destination : undefined)
    } else {
      emit('changed')
    }
    if (request.action !== 'delete') {
      openFolders.value.add(request.folder)
      highlighted.value = destination
    } else {
      highlighted.value = parentOf(request.source?.path ?? '.')
    }
    await Promise.all([refresh(), loadFilteredPaths(query.value)])
    if (request.action === 'file') emit('open', destination)
  } catch (cause) {
    mutationError.value = String(cause)
    await refresh()
  } finally {
    busy.value = false
  }
}

async function location(entry: ProjectEntry, action: 'explorer' | 'terminal' | 'path' | 'relative') {
  menu.value = undefined
  error.value = ''
  try {
    if (action === 'path') await Clipboard.SetText(await api.ProjectLocation(props.path, entry.path))
    else if (action === 'relative') await Clipboard.SetText(entry.path)
    else await api.OpenProjectLocation(props.path, entry.path, action)
  } catch (cause) {
    error.value = String(cause)
  }
}

function rowKey(row: TreeRow, event: KeyboardEvent) {
  if (event.altKey || event.metaKey || event.shiftKey) return
  if (event.ctrlKey) {
    if (event.key.toLowerCase() === 'v' && clipboard.value) paste(row.folder ? row.path : parentOf(row.path))
    else if (event.key.toLowerCase() === 'c' && row.revision) remember(row, false)
    else if (event.key.toLowerCase() === 'x' && row.revision) remember(row, true)
    else return
  } else if (event.key === 'F2' && row.revision) edit('move', parentOf(row.path), row, true)
  else if (event.key === 'Delete' && row.revision) edit('delete', parentOf(row.path), row)
  else if (event.key === 'F4' && !row.folder) emit('open', row.path)
  else return
  event.preventDefault()
  event.stopPropagation()
}
</script>

<template>
  <section class="panel repository-files" aria-label="Repository files" @keydown.ctrl.f.stop.prevent="filterFiles">
    <header class="tool-title project-tools">
      <strong>Project</strong>
      <button class="project-tool" aria-label="New project item" title="New file or directory"
              @click="showMenu(selectedEntry, $event)">＋</button>
      <button class="project-tool" aria-label="Reveal active file" title="Reveal active file" :disabled="!selected" @click="reveal">⌖</button>
      <button class="project-tool" aria-label="Collapse project tree" title="Collapse project tree"
              @click="openFolders = new Set(['.']); query = ''">⌃</button>
      <button class="project-tool" aria-label="Filter project files" title="Filter project files"
              :aria-pressed="showFilter" @click="filterFiles">⌕</button>
      <button class="project-tool" aria-label="Refresh project" title="Refresh project" :disabled="loading" @click="refresh">↻</button>
      <slot name="tools" />
    </header>
    <div v-show="showFilter" class="ref-filter">
      <input ref="filter" v-model="query" aria-label="Filter repository files" placeholder="Filter by file path" spellcheck="false" />
    </div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <div ref="list" class="file-scroll project-scroll" :style="{ '--project-row-height': rowHeight + 'px' }">
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <template v-for="item in virtual.getVirtualItems()" :key="String(item.key)">
          <button v-if="rows[item.index]" class="repository-file-row"
                  :class="{ active: highlighted === rows[item.index].path, 'folder-row': rows[item.index].folder, 'project-root': rows[item.index].path === '.' }"
                  :title="rows[item.index].path === '.' ? path : rows[item.index].path"
                  :aria-pressed="rows[item.index].folder ? undefined : highlighted === rows[item.index].path"
                  :aria-expanded="rows[item.index].folder ? !!query.trim() || openFolders.has(rows[item.index].path) : undefined"
                  :style="{ transform: 'translateY(' + item.start + 'px)', paddingInlineStart: 4 + rows[item.index].depth * 15 + 'px' }"
                  @click="openRow(rows[item.index])" @keydown="rowKey(rows[item.index], $event)"
                  @contextmenu.prevent.stop="showMenu(rows[item.index], $event)"
                  @keydown.shift.f10.prevent.stop="showMenu(rows[item.index], $event)">
            <span class="project-chevron" aria-hidden="true">{{ rows[item.index].folder ? (openFolders.has(rows[item.index].path) || query.trim() ? '⌄' : '›') : '' }}</span>
            <ProjectNodeIcon :name="rows[item.index].name" :folder="rows[item.index].folder" :root="rows[item.index].path === '.'" />
            <span class="ellipsis">{{ rows[item.index].name }}</span>
            <span v-if="rows[item.index].path === '.'" class="ellipsis muted project-root-path">{{ path }}</span>
          </button>
        </template>
      </div>
    </div>
    <Teleport to=".shell" v-if="menu">
      <ContextActions :anchor="menu.anchor" label="Project file actions" class="project-context-menu" @close="menu = undefined">
        <button :disabled="unavailable" @click="edit('file', menuFolder)">New File…</button>
        <button :disabled="unavailable" @click="edit('folder', menuFolder)">New Directory…</button>
        <hr>
        <button @click="location(menu.entry, 'explorer')">Show in Explorer</button>
        <button @click="location(menu.entry, 'terminal')">Open in Terminal</button>
        <button @click="searchFolder = menuFolder; menu = undefined">Find in Directory…</button>
        <hr>
        <button :disabled="!menu.entry.revision || isDirty(menu.entry)" @click="remember(menu.entry, true)">Cut <kbd>Ctrl+X</kbd></button>
        <button :disabled="!menu.entry.revision" @click="remember(menu.entry, false)">Copy <kbd>Ctrl+C</kbd></button>
        <button :disabled="unavailable || !clipboard || (clipboard.cut && isDirty(clipboard.entry))" @click="paste(menuFolder)">Paste… <kbd>Ctrl+V</kbd></button>
        <hr>
        <button @click="location(menu.entry, 'path')">Copy Path</button>
        <button @click="location(menu.entry, 'relative')">Copy Relative Path</button>
        <hr>
        <button :disabled="unavailable || !menu.entry.revision || isDirty(menu.entry)"
                @click="edit('move', parentOf(menu.entry.path), menu.entry, true)">Rename… <kbd>F2</kbd></button>
        <button :disabled="unavailable || !menu.entry.revision || isDirty(menu.entry)"
                @click="edit('delete', parentOf(menu.entry.path), menu.entry)">Delete… <kbd>Del</kbd></button>
        <template v-if="!menu.entry.folder">
          <hr>
          <button @click="emit('gitMenu', menu.entry.path, menu.anchor); menu = undefined">Git Actions…</button>
        </template>
      </ContextActions>
    </Teleport>
    <ConfirmationDialog v-if="mutation" :title="mutationTitle" :busy="busy" @cancel="mutation = undefined">
      <p v-if="mutation.source"><strong>{{ mutation.source.path }}</strong></p>
      <p v-if="mutation.action === 'delete'">Permanently delete this {{ mutation.source?.folder ? 'directory and its contents' : 'file' }} from {{ path }}?</p>
      <form v-else id="project-entry-form" class="repository-dialog-form" @submit.prevent="apply">
        <p class="muted">{{ mutation.folder === '.' ? path : mutation.folder }}</p>
        <label for="project-entry-name">Name</label>
        <input id="project-entry-name" v-model="name" :disabled="busy" data-initial-focus />
      </form>
      <p v-if="mutationError" class="notice error" role="alert">{{ mutationError }}</p>
      <template #actions>
        <button data-cancel :disabled="busy" @click="mutation = undefined">Cancel</button>
        <button v-if="mutation.action === 'delete'" class="danger-button" :disabled="busy" @click="apply">Delete from Disk</button>
        <button v-else type="submit" form="project-entry-form" class="primary" :disabled="busy || !name.trim()">{{ mutationTitle }}</button>
      </template>
    </ConfirmationDialog>
    <ProjectSearchDialog v-if="searchFolder !== undefined" :repo="path" :folder="searchFolder"
                         @close="searchFolder = undefined" @open="(file, line) => { searchFolder = undefined; emit('open', file, line) }" />
  </section>
</template>
