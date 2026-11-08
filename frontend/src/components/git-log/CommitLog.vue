<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Clipboard } from '@wailsio/runtime'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { Commit, Cursor, HistoryFilter } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useWorkspace } from '../../stores/workspace'
import { usePreferences } from '../../stores/preferences'
import { useKeymap } from '../../stores/keymap'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import GraphCell from './GraphCell.vue'

const store = useWorkspace()
const preferences = usePreferences()
const keymap = useKeymap()
const props = defineProps<{ pinned?: string; busy?: boolean; restoring?: boolean }>()
const emit = defineEmits<{ select: []; createBranch: []; operation: [action: 'cherry-pick' | 'revert' | 'reset', commit: Commit]; pin: [commit: Commit]; compare: [commit: Commit] }>()
const scroll = useTemplateRef<HTMLElement>('scroll')
const emptyFilter = (): HistoryFilter => ({ message: '', author: '', branch: '', from: '', through: '', hash: '' })
const draft = ref(emptyFilter())
const hashMode = ref(false)
const showSearch = ref(false)
const filter = shallowRef<HistoryFilter>()
const matches = shallowRef<Commit[]>([])
const cursor = shallowRef<Cursor>()
const searchMore = ref(false)
const searching = ref(false)
const searchError = ref('')
const requests = new LatestRequest()
let historyScroll = 0
const menu = ref<Commit>()
const menuAnchor = shallowRef<ContextAnchor>()
const copyStatus = ref('')
const commits = computed(() => filter.value ? matches.value : store.commits)
const graphPixels = computed(() => filter.value ? 8 : Math.max(52, store.graphWidth * 13 + 14))
const columns = computed(() => ({ '--graph-width': graphPixels.value + 'px' }))
const virtual = useVirtualizer(computed(() => ({
  count: commits.value.length,
  getScrollElement: () => scroll.value,
  estimateSize: () => preferences.rowHeight,
  overscan: 12,
})))
const rows = computed(() => virtual.value.getVirtualItems().flatMap(item => {
  const commit = commits.value[item.index]
  return commit ? [{ item, commit }] : []
}))
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })
const tooltip = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

watch([() => rows.value.at(-1)?.item.index, () => props.restoring], ([index, restoring]) => {
  if (!restoring && !filter.value && index !== undefined && index >= store.commits.length - 20 && !store.logError) void store.loadPage()
})

function select(index: number, reveal = true) {
  const commit = commits.value[index]
  if (!commit) return
  menu.value = undefined
  if (reveal) void nextTick(() => virtual.value.scrollToIndex(index, { align: 'auto' }))
  emit('select')
  void store.selectCommit(commit)
}

function jump(hash: string) {
  if (filter.value) clearSearch()
  const index = store.commits.findIndex(item => item.hash === hash)
  if (index >= 0) {
    select(index)
    return true
  }
  return false
}

function step(direction: number) {
  if (!commits.value.length) return
  const index = commits.value.findIndex(item => item.hash === store.selected?.hash)
  const base = index < 0 ? (direction > 0 ? -1 : commits.value.length) : index
  select(Math.max(0, Math.min(commits.value.length - 1, base + direction)))
}

function navigate(event: KeyboardEvent, direction: number) {
  // Modifier combos such as Alt+Up/Down belong to the global dispatcher;
  // handling them here too would dispatch one keypress twice.
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (!commits.value.length) return
  event.preventDefault()
  step(direction)
}

function openSearch() {
  showSearch.value = true
  void nextTick(() => document.querySelector<HTMLInputElement>('.history-search input')?.focus())
}

function dateBoundary(day: string, end: boolean) {
  if (!day) return ''
  const date = new Date(day + (end ? 'T23:59:59' : 'T00:00:00'))
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

function startSearch() {
  const input = draft.value
  const query = hashMode.value
    ? { ...emptyFilter(), hash: input.hash.trim() }
    : { ...emptyFilter(), message: input.message.trim(), author: input.author.trim(),
        branch: input.branch, from: dateBoundary(input.from, false), through: dateBoundary(input.through, true) }
  if (!hashMode.value && ((input.from && !query.from) || (input.through && !query.through))) {
    searchError.value = 'Enter a valid date range.'
    return
  }
  if (!Object.values(query).some(Boolean)) {
    clearSearch()
    return
  }
  if (!filter.value) historyScroll = scroll.value?.scrollTop ?? 0
  filter.value = query
  restartSearch()
}

function restartSearch() {
  requests.clear()
  matches.value = []
  cursor.value = undefined
  searchMore.value = false
  menu.value = undefined
  if (scroll.value) scroll.value.scrollTop = 0
  void loadMatches()
}

async function loadMatches() {
  const path = store.repository?.path
  const query = filter.value
  if (!path || !query) return
  const call = api.SearchHistory(path, query, cursor.value ?? { offset: 0, snapshot: '', lanes: null })
  const current = requests.replace(call)
  searching.value = true
  searchError.value = ''
  try {
    const page = await call
    if (!current()) return
    matches.value = [...matches.value, ...(page.commits ?? [])]
    cursor.value = page.next
    searchMore.value = page.more
  } catch (error) {
    if (current()) searchError.value = String(error)
  } finally {
    if (current()) searching.value = false
    requests.finish(call)
  }
}

function clearSearch(restoreScroll = true) {
  const hadSearch = !!filter.value
  requests.clear()
  filter.value = undefined
  searching.value = false
  searchError.value = ''
  matches.value = []
  menu.value = undefined
  if (hadSearch && restoreScroll) {
    void nextTick(() => { if (scroll.value) scroll.value.scrollTop = historyScroll })
  }
}

function closeSearch() {
  clearSearch()
  showSearch.value = false
}

watch(() => store.repository, (repo, previous) => {
  menu.value = undefined
  copyStatus.value = ''
  if (repo?.path !== previous?.path) {
    clearSearch(false)
    draft.value = emptyFilter()
    hashMode.value = false
    if (scroll.value) scroll.value.scrollTop = 0
  } else if (filter.value) {
    restartSearch()
  }
})
onUnmounted(() => requests.clear())

function showCommitActions(index: number, event: MouseEvent | KeyboardEvent) {
  const commit = commits.value[index]
  if (!commit) return
  select(index, false)
  menu.value = commit
  copyStatus.value = ''
  menuAnchor.value = contextAnchor(event)
}

function closeCommitActions() {
  menu.value = undefined
  scroll.value?.focus()
}

async function copyCommit(commit: Commit, message: boolean) {
  const repo = store.repository
  if (!repo) return
  closeCommitActions()
  try {
    const value = message ? await api.CommitMessage(repo.path, commit.hash) : commit.hash
    if (store.repository?.path !== repo.path) return
    await Clipboard.SetText(value)
    copyStatus.value = message ? 'Commit message copied.' : 'Hash copied.'
  } catch (error) {
    if (store.repository?.path === repo.path) copyStatus.value = String(error)
  }
}

defineExpose({ jump, step, openSearch })
</script>

<template>
  <section class="panel log-panel" aria-label="Git Log">
    <header class="panel-header">
      <div><span class="eyebrow">HISTORY</span><h2>Git Log</h2></div>
      <div class="log-tools">
        <span class="muted">{{ commits.length.toLocaleString() }} {{ filter ? 'matches loaded' : 'loaded' }}</span>
        <button :aria-expanded="showSearch" :title="'Search history (' + (keymap.bindings.searchHistory || 'unbound') + ')'"
                @click="showSearch ? closeSearch() : showSearch = true">Search…</button>
      </div>
    </header>
    <form v-if="showSearch" class="history-search" @submit.prevent="startSearch">
      <label>Search by
        <select v-model="hashMode">
          <option :value="false">History filters</option>
          <option :value="true">Commit hash</option>
        </select>
      </label>
      <label v-if="hashMode">Commit hash <input v-model="draft.hash" spellcheck="false" placeholder="7 or more hex digits" /></label>
      <template v-else>
        <label>Message <input v-model="draft.message" placeholder="Text in commit message" /></label>
        <label>Author <input v-model="draft.author" placeholder="Name or email" /></label>
        <label>Branch
          <select v-model="draft.branch">
            <option value="">All branches and tags</option>
            <option v-for="branch in store.branchRefs" :key="branch.name" :value="branch.name">
              {{ branch.name.replace(/^refs\/(heads|remotes)\//, '') }}
            </option>
          </select>
        </label>
        <label>Committed from <input v-model="draft.from" type="date" /></label>
        <label>Committed through <input v-model="draft.through" type="date" /></label>
      </template>
      <div class="search">
        <button type="submit">Search</button>
        <button v-if="filter" type="button" @click="clearSearch()">Back to graph</button>
      </div>
    </form>
    <ContextActions v-if="menu && menuAnchor" :anchor="menuAnchor" label="Commit actions" @close="menu = undefined">
      <button @click="copyCommit(menu, false)">Copy hash</button>
      <button @click="copyCommit(menu, true)">Copy message</button>
      <hr />
      <button @click="menu = undefined; emit('createBranch')">Create branch…</button>
      <button @click="emit('pin', menu); closeCommitActions()">{{ props.pinned === menu.hash ? 'Clear comparison base' : 'Use as comparison base' }}</button>
      <button :disabled="store.opening || props.pinned === menu.hash" @click="emit('compare', menu); closeCommitActions()">
        {{ props.pinned ? 'Compare with base' : 'Compare with HEAD' }}
      </button>
      <hr />
      <button :disabled="props.busy || (menu.parents?.length ?? 0) > 1" @click="emit('operation', 'cherry-pick', menu); closeCommitActions()">Cherry-pick…</button>
      <button :disabled="props.busy || (menu.parents?.length ?? 0) > 1" @click="emit('operation', 'revert', menu); closeCommitActions()">Revert…</button>
      <button :disabled="props.busy" @click="emit('operation', 'reset', menu); closeCommitActions()">Reset to here…</button>
    </ContextActions>
    <p v-if="copyStatus" class="notice" role="status">{{ copyStatus }}</p>
    <p v-if="searchError" class="notice error" role="alert">{{ searchError }}</p>
    <p v-if="!filter && store.logError" class="notice error" role="alert">{{ store.logError }}</p>
    <div ref="scroll" class="log-scroll" :class="{ 'search-results': filter }" :style="columns" tabindex="0" aria-label="Commit history"
         @scroll.passive="menu = undefined" @keydown.up="navigate($event, -1)" @keydown.down="navigate($event, 1)">
      <div class="log-grid log-heading"><span>{{ filter ? '' : 'Graph' }}</span><span>Commit message</span><span>Author</span><span>{{ filter ? 'Commit date' : 'Date' }}</span><span>Hash</span></div>
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <button v-for="{ item, commit } in rows" :key="commit.hash"
                class="log-grid commit-row" :class="{ active: store.selected?.hash === commit.hash }"
                :aria-pressed="store.selected?.hash === commit.hash"
                :style="{ transform: 'translateY(' + item.start + 'px)' }" @click="select(item.index)"
                @contextmenu.prevent="showCommitActions(item.index, $event)"
                @keydown.shift.f10.prevent="showCommitActions(item.index, $event)">
          <span v-if="filter" aria-hidden="true"></span>
          <GraphCell v-else :row="commit.graph" :width="graphPixels" :height="preferences.rowHeight" />
          <span class="commit-title" :title="commit.subject">
            <span v-for="label in commit.refs" :key="label" class="chip" :title="label">
              {{ label.replace(/^refs\/(heads|remotes|tags)\//, '') }}
            </span>
            <span class="ellipsis">{{ commit.subject || '(no subject)' }}</span>
          </span>
          <span class="ellipsis muted">{{ commit.author }}</span>
          <time class="muted" :title="tooltip.format(commit.time * 1000)">{{ date.format(commit.time * 1000) }}</time>
          <code class="muted" :title="commit.hash">{{ commit.hash.slice(0, 8) }}</code>
        </button>
      </div>
      <template v-if="filter">
        <p v-if="searching" class="notice" role="status">Searching history…</p>
        <p v-else-if="!matches.length && !searchError" class="notice">No matching commits.</p>
        <button v-else-if="searchMore && !searchError" class="load-more" @click="loadMatches">Load more matches</button>
      </template>
      <template v-else>
        <p v-if="!store.commits.length && !store.loading && !store.logError" class="notice">This repository has no commits yet.</p>
        <p v-if="store.loading" class="notice" role="status">Loading commits…</p>
        <button v-else-if="store.more" class="load-more" @click="store.loadPage()">Load more commits</button>
      </template>
    </div>
  </section>
</template>
