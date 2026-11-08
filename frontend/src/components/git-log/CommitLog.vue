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
const emit = defineEmits<{ select: []; createBranch: []; operation: [action: 'cherry-pick' | 'revert' | 'reset', commit: Commit]; pin: [commit: Commit]; compare: [commit: Commit]; refresh: [] }>()
const scroll = useTemplateRef<HTMLElement>('scroll')
const emptyFilter = (): HistoryFilter => ({ message: '', author: '', branch: '', from: '', through: '', hash: '' })
const draft = ref(emptyFilter())
const queryInput = useTemplateRef<HTMLInputElement>('query-input')
const tool = shallowRef<{ kind: 'user' | 'date' | 'paths' | 'sort' | 'view'; anchor: ContextAnchor }>()
const datePreset = ref(0)
const popupDraft = ref({ author: '', path: '', from: '', through: '' })
const recentAuthors = ref<string[]>([])
const authors = computed(() => [...new Set([...recentAuthors.value, ...store.commits.map(commit => commit.author)])].filter(Boolean).slice(0, 12))
const showTags = ref(true)
const compactRefs = ref(false)
const highlightMerges = ref(false)
const showHash = ref(true)
const showAuthor = ref(true)
const showDate = ref(true)
const refsOnLeft = ref(true)
const filteredGraphWidth = ref(0)
const branchLabel = (name: string) => name.replace(/^refs\/(heads|remotes|tags)\//, '')

function openTool(kind: NonNullable<typeof tool.value>['kind'], event: MouseEvent) {
  menu.value = undefined
  popupDraft.value = { author: draft.value.author, path: draft.value.path ?? '', from: draft.value.from, through: draft.value.through }
  if (tool.value?.kind === kind) tool.value = undefined
  else tool.value = { kind, anchor: contextAnchor(event, false) }
}

function applyTool() {
  tool.value = undefined
  const author = draft.value.author.trim()
  if (author) recentAuthors.value = [author, ...recentAuthors.value.filter(item => item !== author)].slice(0, 12)
  startSearch()
}

function setDatePreset(days: number) {
  datePreset.value = days
  draft.value.from = ''
  draft.value.through = ''
  applyTool()
}

function visibleRefs(commit: Commit) {
  const refs = showTags.value ? commit.refs : commit.refs?.filter(ref => !ref.startsWith('refs/tags/'))
  return compactRefs.value ? refs?.slice(0, 1) : refs
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
function queueSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(startSearch, 250)
}
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
const graphVisible = computed(() => !filter.value || filteredGraphWidth.value > 0)
const graphPixels = computed(() => graphVisible.value
  ? Math.max(52, (filter.value ? filteredGraphWidth.value : store.graphWidth) * 13 + 14) : 8)
const columns = computed(() => ({
  '--log-columns': [graphPixels.value + 'px', 'minmax(155px, 1fr)',
    ...(showAuthor.value ? ['110px'] : []), ...(showDate.value ? ['124px'] : []), ...(showHash.value ? ['72px'] : []),
  ].join(' '),
  '--log-min-width': (graphPixels.value + 175 + (showAuthor.value ? 116 : 0) +
    (showDate.value ? 130 : 0) + (showHash.value ? 78 : 0)) + 'px',
}))
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
/** Compact "2026/11/8 17:03" cell format keeps the Date column readable
 *  while showing the commit time; the tooltip carries the full locale form. */
function formatCommitDate(seconds: number) {
  const value = new Date(seconds * 1000)
  const pad = (input: number) => String(input).padStart(2, '0')
  return value.getFullYear() + '/' + (value.getMonth() + 1) + '/' + value.getDate() + ' ' + pad(value.getHours()) + ':' + pad(value.getMinutes())
}
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
  void nextTick(() => queryInput.value?.focus())
}

function dateBoundary(day: string, end: boolean) {
  if (!day) return ''
  const date = new Date(day + (end ? 'T23:59:59' : 'T00:00:00'))
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

function startSearch() {
  clearTimeout(searchTimer)
  const input = draft.value
  const query: HistoryFilter = {
    ...input,
    message: input.message.trim(), author: input.author.trim(), path: input.path?.trim(),
    from: datePreset.value ? new Date(Date.now() - datePreset.value * 86400000).toISOString() : dateBoundary(input.from, false),
    through: dateBoundary(input.through, true),
  }
  if (!query.regex && /^[a-f0-9]{7,64}$/i.test(query.message) &&
      !(query.author || query.branch || query.from || query.through || query.path || query.firstParent || query.noMerges)) {
    query.hash = query.message
    query.message = ''
  }
  if (!(query.hash || query.message || query.author || query.branch || query.path || query.from || query.through ||
      query.sort || query.firstParent || query.noMerges)) {
    const { regex, matchCase } = input
    clearSearch()
    Object.assign(draft.value, { regex, matchCase })
    return
  }
  if (!filter.value) historyScroll = scroll.value?.scrollTop ?? 0
  filter.value = query
  restartSearch()
}

function restartSearch() {
  requests.clear()
  matches.value = []
  filteredGraphWidth.value = 0
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
    filteredGraphWidth.value = Math.max(filteredGraphWidth.value, ...(page.commits ?? []).map(commit => commit.graph.width))
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
  clearTimeout(searchTimer)
  const hadSearch = !!filter.value
  draft.value = emptyFilter()
  datePreset.value = 0
  tool.value = undefined
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

watch(() => store.repository, (repo, previous) => {
  menu.value = undefined
  copyStatus.value = ''
  if (repo?.path !== previous?.path) {
    clearSearch(false)
    recentAuthors.value = []
    if (scroll.value) scroll.value.scrollTop = 0
  } else if (filter.value) {
    restartSearch()
  }
})
onUnmounted(() => {
  clearTimeout(searchTimer)
  requests.clear()
})

function showCommitActions(index: number, event: MouseEvent | KeyboardEvent) {
  const commit = commits.value[index]
  if (!commit) return
  tool.value = undefined
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
    <header class="log-toolbar" aria-label="Git Log filters">
      <form class="log-query" role="search" @submit.prevent="startSearch">
        <input ref="query-input" v-model="draft.message" aria-label="Text or hash" placeholder="Text or hash"
               spellcheck="false" :title="'Search history (' + (keymap.bindings.searchHistory || 'unbound') + ')'"
               @input="queueSearch" />
        <button type="button" title="Regular expression" aria-label="Regular expression" :aria-pressed="!!draft.regex"
                @click="draft.regex = !draft.regex; startSearch()">.*</button>
        <button type="button" title="Match case" aria-label="Match case" :aria-pressed="!!draft.matchCase"
                @click="draft.matchCase = !draft.matchCase; startSearch()">Cc</button>
      </form>
      <select v-model="draft.branch" aria-label="Branch" :title="draft.branch ? branchLabel(draft.branch) : 'Branch'"
              :class="{ 'filter-active': draft.branch }" @change="startSearch">
        <option value="">Branch</option>
        <option v-if="store.headHash" value="HEAD">HEAD</option>
        <optgroup label="Local">
          <option v-for="branch in store.branchRefs.filter(ref => ref.name.startsWith('refs/heads/'))"
                  :key="branch.name" :value="branch.name">{{ branchLabel(branch.name) }}</option>
        </optgroup>
        <optgroup label="Remote">
          <option v-for="branch in store.branchRefs.filter(ref => ref.name.startsWith('refs/remotes/'))"
                  :key="branch.name" :value="branch.name">{{ branchLabel(branch.name) }}</option>
        </optgroup>
        <optgroup v-if="store.tagRefs.length" label="Tags">
          <option v-for="tag in store.tagRefs" :key="tag.name" :value="tag.name">{{ branchLabel(tag.name) }}</option>
        </optgroup>
      </select>
      <button :class="{ 'filter-active': filter?.author }" :title="filter?.author || 'User'"
              :aria-expanded="tool?.kind === 'user'" @click="openTool('user', $event)">User ⌄</button>
      <button :class="{ 'filter-active': filter?.from || filter?.through }" :aria-expanded="tool?.kind === 'date'"
              @click="openTool('date', $event)">Date ⌄</button>
      <button :class="{ 'filter-active': filter?.path }" :title="filter?.path || 'Paths'"
              :aria-expanded="tool?.kind === 'paths'" @click="openTool('paths', $event)">Paths ⌄</button>
      <button aria-label="Sort and history options" title="Sort and history options"
              :class="{ 'filter-active': filter?.sort || filter?.firstParent || filter?.noMerges }"
              :aria-expanded="tool?.kind === 'sort'" @click="openTool('sort', $event)">⇅</button>
      <div class="log-toolbar-end">
        <button v-if="filter" title="Clear filters" aria-label="Clear filters" @click="clearSearch()">×</button>
        <button aria-label="Refresh history" title="Refresh history" :disabled="store.opening" @click="emit('refresh')">↻</button>
        <button aria-label="History display options" title="History display options"
                :aria-expanded="tool?.kind === 'view'" @click="openTool('view', $event)">◉</button>
      </div>
    </header>
    <ContextActions v-if="tool" :anchor="tool.anchor" :label="tool.kind + ' options'" @close="tool = undefined">
      <template v-if="tool.kind === 'user'">
        <form class="log-filter-form" @submit.prevent="draft.author = popupDraft.author; applyTool()">
          <label>Name or email<input v-model="popupDraft.author" data-initial-focus /></label>
          <button type="submit">Apply</button>
        </form>
        <button @click="draft.author = ''; applyTool()">All users</button>
        <hr v-if="authors.length" />
        <button v-for="author in authors" :key="author" @click="draft.author = author; applyTool()">{{ author }}</button>
      </template>
      <template v-else-if="tool.kind === 'date'">
        <button @click="setDatePreset(0)">Any date</button>
        <button @click="setDatePreset(1)">Last 24 hours</button>
        <button @click="setDatePreset(7)">Last 7 days</button>
        <hr />
        <form class="log-filter-form" @submit.prevent="datePreset = 0; draft.from = popupDraft.from; draft.through = popupDraft.through; applyTool()">
          <label>Committed from<input v-model="popupDraft.from" type="date" /></label>
          <label>Committed through<input v-model="popupDraft.through" type="date" /></label>
          <button type="submit">Apply date range</button>
        </form>
      </template>
      <template v-else-if="tool.kind === 'paths'">
        <form class="log-filter-form" @submit.prevent="draft.path = popupDraft.path; applyTool()">
          <label>Repository-relative path<input v-model="popupDraft.path" data-initial-focus placeholder="File or folder" /></label>
          <button type="submit">Apply</button>
        </form>
        <button @click="draft.path = ''; applyTool()">All paths</button>
      </template>
      <template v-else-if="tool.kind === 'sort'">
        <strong>Sort</strong>
        <button :aria-pressed="draft.sort === 'date'" @click="draft.sort = 'date'; applyTool()">By Commit Date</button>
        <button :aria-pressed="!draft.sort" @click="draft.sort = ''; applyTool()">Topologically</button>
        <hr />
        <strong>Options</strong>
        <button :aria-pressed="!!draft.firstParent" @click="draft.firstParent = !draft.firstParent; applyTool()">First Parent</button>
        <button :aria-pressed="!!draft.noMerges" @click="draft.noMerges = !draft.noMerges; applyTool()">No Merges</button>
      </template>
      <template v-else>
        <strong>Show</strong>
        <button :aria-pressed="compactRefs" @click="compactRefs = !compactRefs">Compact References View</button>
        <button :aria-pressed="showTags" @click="showTags = !showTags">Tag Names</button>
        <button :aria-pressed="refsOnLeft" @click="refsOnLeft = !refsOnLeft">References on the Left</button>
        <hr />
        <strong>Columns</strong>
        <button :aria-pressed="showAuthor" @click="showAuthor = !showAuthor">Author</button>
        <button :aria-pressed="showDate" @click="showDate = !showDate">Date</button>
        <button :aria-pressed="showHash" @click="showHash = !showHash">Hash</button>
        <hr />
        <button :aria-pressed="highlightMerges" @click="highlightMerges = !highlightMerges">Highlight Merge Commits</button>
      </template>
    </ContextActions>
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
    <div ref="scroll" class="log-scroll" :class="{ 'search-results': !graphVisible }" :style="columns" tabindex="0" aria-label="Commit history"
         @scroll.passive="menu = undefined" @keydown.up="navigate($event, -1)" @keydown.down="navigate($event, 1)">
      <div class="log-grid log-heading"><span>{{ graphVisible ? 'Graph' : '' }}</span><span>Commit message</span><span v-if="showAuthor">Author</span><span v-if="showDate">{{ filter ? 'Commit date' : 'Date' }}</span><span v-if="showHash">Hash</span></div>
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <button v-for="{ item, commit } in rows" :key="commit.hash"
                class="log-grid commit-row" :class="{ active: store.selected?.hash === commit.hash, 'highlight-merge': highlightMerges && (commit.parents?.length ?? 0) > 1 }"
                :aria-pressed="store.selected?.hash === commit.hash"
                :style="{ transform: 'translateY(' + item.start + 'px)' }" @click="select(item.index)"
                @contextmenu.prevent="showCommitActions(item.index, $event)"
                @keydown.shift.f10.prevent="showCommitActions(item.index, $event)">
          <span v-if="!graphVisible" aria-hidden="true"></span>
          <GraphCell v-else :row="commit.graph" :width="graphPixels" :height="preferences.rowHeight" />
          <span class="commit-title" :class="{ 'refs-right': !refsOnLeft }" :title="commit.subject">
            <span v-for="label in visibleRefs(commit)" :key="label" class="chip" :title="label">
              {{ label.replace(/^refs\/(heads|remotes|tags)\//, '') }}
            </span>
            <span class="ellipsis commit-subject">{{ commit.subject || '(no subject)' }}</span>
          </span>
          <span v-if="showAuthor" class="ellipsis muted">{{ commit.author }}</span>
          <time v-if="showDate" class="muted" :title="tooltip.format(commit.time * 1000)">{{ formatCommitDate(commit.time) }}</time>
          <code v-if="showHash" class="muted" :title="commit.hash">{{ commit.hash.slice(0, 8) }}</code>
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
