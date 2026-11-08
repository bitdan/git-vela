<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onErrorCaptured, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { Events, Window } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../bindings/events'
import { storeToRefs } from 'pinia'
import { defaultPreviewHeight, layoutBounds, usePreferences } from './stores/preferences'
import { useKeymap } from './stores/keymap'
import { editorScopeShortcuts, isTextEntry, navigationShortcuts, type ShortcutId } from './stores/shortcuts'
import { clearSession, readSession, saveSession } from './stores/session'
import { useWorkspace } from './stores/workspace'
import ConfirmationDialog from './components/layout/ConfirmationDialog.vue'
import ContextActions from './components/layout/ContextActions.vue'
import ActivityBar, { type ActivityItem } from './components/layout/ActivityBar.vue'
import { contextAnchor, type ContextAnchor } from './components/layout/contextAnchor'
import CommandPalette from './components/layout/CommandPalette.vue'
import ShortcutSettings from './components/layout/ShortcutSettings.vue'
import type { PaletteCommand } from './components/layout/commandFilter'
import { exceedsRenderLimit } from './components/diff/renderGuard'
import { navigateDiff } from './components/diff/diffNavigation'
import CommitLog from './components/git-log/CommitLog.vue'
import CommitDetails from './components/git-log/CommitDetails.vue'
import GitConsole from './components/operations/GitConsole.vue'
import WorktreePanel from './components/worktrees/WorktreePanel.vue'
import ChangedFiles from './components/changed-files/ChangedFiles.vue'
import { compareNames, fileTreeRows } from './components/changed-files/fileTree'
import WorkingChanges from './components/changed-files/WorkingChanges.vue'
import RepositoryFiles from './components/editor/RepositoryFiles.vue'
import FileInspection from './components/history/FileInspection.vue'
import HunkPanel from './components/history/HunkPanel.vue'
import ConflictPanel from './components/history/ConflictPanel.vue'
import StashPanel from './components/operations/StashPanel.vue'
import SyncPanel from './components/operations/SyncPanel.vue'
import RemotesPanel from './components/operations/RemotesPanel.vue'
import { RepositoryService as api } from '../bindings/gitvela/internal/app'
import type { AmendTarget, Commit, FilePreview, HeadState, OperationState, Ref as GitRef, TextFile, WorkingFile } from '../bindings/gitvela/internal/git'
import { LatestRequest } from './stores/latestRequest'
import appIcon from './git-vela.svg'
import './styles.css'

const TextDiff = defineAsyncComponent(() => import('./components/diff/TextDiff.vue'))
const WorkspaceEditor = defineAsyncComponent(() => import('./components/editor/WorkspaceEditor.vue'))
const store = useWorkspace()
const keymap = useKeymap()
const preferences = usePreferences()
const log = useTemplateRef<InstanceType<typeof CommitLog>>('log')
const filesPanel = useTemplateRef<{ step: (direction: number) => void }>('filesPanel')
const changesPanel = useTemplateRef<InstanceType<typeof WorkingChanges>>('changes-panel')
const settings = useTemplateRef<HTMLDetailsElement>('settings')
const paletteTrigger = useTemplateRef<HTMLButtonElement>('paletteTrigger')
const content = useTemplateRef<HTMLElement>('content')
const paletteOpen = ref(false)
const windowError = ref('')
const restoreNotice = ref('')
const sessionNotice = ref('')
const expandedPreview = ref(false)
const expandedDiff = ref(false)
const path = ref('')
const navigation = ref('')
const branchError = ref('')
const branchNotice = ref('')
const branchName = ref('')
const branchFilter = ref('')
type BranchForm = 'create' | 'rename' | 'delete'
const branchForm = ref<BranchForm>()
const managedBranch = ref('')
const branchBase = ref('')
const createBase = computed(() => branchBase.value || store.selected?.hash)
const gitMenu = useTemplateRef<HTMLDetailsElement>('git-menu')
const branchMenu = shallowRef<{ item: GitRef; anchor: ContextAnchor }>()
type Sidebar = 'files' | 'changes' | 'branches' | 'worktrees'
const sidebar = ref<Exclude<Sidebar, 'branches'>>('changes')
const sidebarHidden = ref(false)
const commitTab = ref<'commit' | 'shelf'>('commit')
const historyMaximized = ref(false)
const gitTab = ref<'log' | 'console'>('log')
const activityActive = computed<ActivityItem | undefined>(() => {
  if (historyVisible.value) return 'git'
  if (sidebarHidden.value) return undefined
  return sidebar.value === 'changes' ? 'commit' : sidebar.value
})

function onActivitySelect(item: ActivityItem) {
  if (indexBusy.value) return
  closeBranchMenu()
  diffExpanded.value = false
  if (item === 'git') {
    historyVisible.value = true
    return
  }
  historyMaximized.value = false
  sidebarHidden.value = false
  sidebar.value = item === 'commit' ? 'changes' : item
  if (item === 'commit') commitTab.value = 'commit'
}
const historyVisible = ref(true)
const restoringSession = ref(false)
const sidebarTabs: { id: Sidebar; label: string }[] = [
  { id: 'files', label: 'Files' },
  { id: 'changes', label: 'Commit' },
  { id: 'branches', label: 'Branches' },
  { id: 'worktrees', label: 'Worktrees' },
]
const refKind = ref<'branches' | 'tags'>('branches')
const visibleRefs = computed(() => {
  const query = branchFilter.value.trim().toLowerCase()
  const refs = refKind.value === 'branches' ? store.branchRefs : store.tagRefs
  const current = 'refs/heads/' + store.repository?.head
  const rank = (name: string) => name === current ? 0 : name.startsWith('refs/remotes/') ? 2 : 1
  return refs.filter(item => refLabel(item.name).toLowerCase().includes(query))
    .sort((a, b) => rank(a.name) - rank(b.name) || compareNames(refLabel(a.name), refLabel(b.name)))
})

const collapsedBranches = ref(new Set<string>())
const branchRows = computed(() => fileTreeRows(visibleRefs.value.map(item => ({
  item,
  path: (item.name.startsWith('refs/remotes/') ? 'Remote/' : 'Local/') + refLabel(item.name),
})), branchFilter.value.trim() ? new Set() : collapsedBranches.value))

function toggleBranchFolder(path: string) {
  if (collapsedBranches.value.has(path)) collapsedBranches.value.delete(path)
  else collapsedBranches.value.add(path)
}

function refLabel(ref: string) {
  return ref.replace(/^refs\/(heads|remotes|tags)\//, '')
}

function openBranchForm(form?: BranchForm, branch = '', base = '') {
  branchBase.value = base
  branchForm.value = form
  managedBranch.value = branch
  branchName.value = form === 'rename' ? branch : ''
  branchError.value = ''
  branchNotice.value = ''
}
const picking = ref(false)
const { dark, previewHeight, fileTree, density, resumeOnStart } = storeToRefs(preferences)
const workspaceError = ref('')
onErrorCaptured((error, _component, info) => {
  console.error('GitVela workspace error', info, error)
  workspaceError.value = String(error)
  return false
})
const workingFiles = ref<WorkingFile[]>([])
const workingError = ref('')
const workingLoading = ref(false)
const workingSelection = ref<WorkingFile>()
const currentWorkingFile = computed(() => workingFiles.value.find(file => file.path === workingSelection.value?.path))
const indexBusy = ref(false)
const indexError = ref('')
const preview = ref<FilePreview>()
const previewError = ref('')
const previewLoading = ref(false)
type FileView = 'working' | 'unstaged' | 'staged'
const view = ref<FileView | 'commit' | 'editor' | 'inspection' | 'conflict'>('working')
type EditorTab = { path: string; file: TextFile; draft: string }
const editorTabs = ref<EditorTab[]>([])
const activeEditorPath = ref('')
const activeEditorTab = computed(() => editorTabs.value.find(tab => tab.path === activeEditorPath.value))
const editor = computed(() => activeEditorTab.value?.file)
const editorPath = computed(() => activeEditorPath.value)
const editorError = ref('')
const editorLoading = ref(false)
const editorSaving = ref(false)
const editorRequest = new LatestRequest()
const editorDirty = computed(() => !!activeEditorTab.value && activeEditorTab.value.draft !== activeEditorTab.value.file.content)

function editorTabFor(path: string) {
  return editorTabs.value.find(tab => tab.path === path)
}

function isTabDirty(tab: EditorTab) {
  return tab.draft !== tab.file.content
}

function fileName(path: string) {
  return path.split(/[\\/]/).at(-1) || path
}

function activateEditorTab(path: string) {
  if (!editorTabFor(path)) return
  activeEditorPath.value = path
  view.value = 'editor'
}

/** Closing keeps the unsaved-changes protection: a dirty tab goes through the
 *  confirmation dialog before it is dropped. */
function requestCloseEditorTab(path: string) {
  const tab = editorTabFor(path)
  if (!tab) return
  if (!isTabDirty(tab)) {
    closeEditorTab(path)
    return
  }
  pendingEditorLabel.value = 'closing ' + fileName(path)
  pendingEditorAction.value = () => closeEditorTab(path)
}

function closeEditorTab(path: string) {
  const index = editorTabs.value.findIndex(tab => tab.path === path)
  if (index < 0) return
  editorTabs.value.splice(index, 1)
  if (activeEditorPath.value !== path) return
  const next = editorTabs.value[index] ?? editorTabs.value[index - 1]
  if (next) {
    activeEditorPath.value = next.path
    if (view.value === 'editor') view.value = 'editor'
  } else {
    activeEditorPath.value = ''
    if (view.value === 'editor' || view.value === 'conflict') view.value = 'working'
  }
}
const pendingEditorAction = shallowRef<() => void>()
const pendingEditorLabel = ref('')
const showHunks = ref(false)
const conflictFile = ref('')
const submoduleError = ref('')
const inspection = shallowRef<{ file: string; mode: 'history' | 'blame'; previous: FileView | 'editor' }>()
const pinnedCommit = shallowRef<Commit>()
type AdvancedAction = 'cherry-pick' | 'revert' | 'reset' | 'merge' | 'rebase'
const advancedLabels: Record<AdvancedAction, string> = {
  'cherry-pick': 'Cherry-pick', revert: 'Revert', reset: 'Reset', merge: 'Merge', rebase: 'Rebase',
}
const advanced = shallowRef<{ action: AdvancedAction; hash: string; label: string; head: HeadState }>()
type RebaseAction = 'pick' | 'squash' | 'fixup' | 'drop'
const rebasePlan = ref<{ commit: Commit; action: RebaseAction }[]>([])
const resetMode = ref<'soft' | 'mixed' | 'hard'>('mixed')
const hardResetConfirmed = ref(false)
const preparingOperation = ref(false)
const operation = shallowRef<OperationState>()
const operationError = ref('')
const operationNotice = ref('')
const operationRequest = new LatestRequest()
const headRequest = new LatestRequest()
const inspectionPath = computed(() => view.value === 'editor' ? editorPath.value :
  (view.value === 'working' || view.value === 'staged' || view.value === 'unstaged') ? workingSelection.value?.path : undefined)
const previewPath = computed(() => view.value === 'conflict' ? conflictFile.value : view.value === 'inspection' ? inspection.value?.file : view.value === 'editor' ? editorPath.value : view.value === 'commit' ? store.file?.path : workingSelection.value?.path)
const workingView = computed(() => view.value === 'working' || view.value === 'staged' || view.value === 'unstaged' ? view.value : undefined)
const diffExpanded = ref(false)
const restoreDiff = useTemplateRef<HTMLButtonElement>('restore-diff')
const commitMessage = ref('')
const amendTarget = ref<AmendTarget>()
let draftBeforeAmend = ''
type DailyPanel = 'sync' | 'init' | 'remotes' | 'publish'
type SyncIntent = 'fetch' | 'pull' | 'push'
const dailyPanel = ref<DailyPanel>()
const syncIntent = ref<SyncIntent>('fetch')
const projectPath = ref('')
const projectError = ref('')
const hasLocalHead = computed(() => store.branchRefs.some(item => item.name === 'refs/heads/' + store.repository?.head))
const commitNotice = ref('')
const patchTooLarge = computed(() => preview.value?.kind === 'text' && exceedsRenderLimit('', preview.value.content))
const patchLines = computed(() => preview.value?.content.replace(/\n$/, '').split('\n') ?? [])
const workingTabs: { id: FileView; title: string }[] = [
  { id: 'working', title: 'Working file' },
  { id: 'unstaged', title: 'Unstaged diff' },
  { id: 'staged', title: 'Staged diff' },
]

function patchClass(line: string) {
  if (line.startsWith('@@')) return 'patch-hunk'
  if (line.startsWith('+') && !line.startsWith('+++')) return 'patch-added'
  if (line.startsWith('-') && !line.startsWith('---')) return 'patch-removed'
}
const statusRequest = new LatestRequest()
const watchRequest = new LatestRequest()
const watchError = ref('')
let unsubscribeChanges: (() => void) | undefined
const previewRequest = new LatestRequest()
let timer: ReturnType<typeof setInterval>

watch(view, value => {
  if (value !== 'commit') diffExpanded.value = false
  showHunks.value = false
  expandedPreview.value = false
})
watch(() => store.file?.path, () => { submoduleError.value = ''; expandedDiff.value = false })
watch([() => store.selected?.hash, () => store.parent, () => store.comparisonRange], () => { expandedDiff.value = false })
const imageSides = computed(() => [
  { label: 'Before', src: store.comparison?.before ?? '' },
  { label: 'After', src: store.comparison?.after ?? '' },
])
function isDataImage(src: string) {
  return /^data:image\/(png|jpeg|gif|webp|bmp);base64,[A-Za-z0-9+/=]+$/.test(src)
}
function loadFullDiff() {
  if (!store.file || store.loadingDiff) return
  expandedDiff.value = true
  void store.selectFile(store.file, true)
}
function loadFullWorkingPreview() {
  const next = workingView.value
  if (!next || previewLoading.value) return
  expandedPreview.value = true
  void selectView(next, false, true)
}
watch(editorDirty, dirty => { if (!dirty) pendingEditorAction.value = undefined })
watch(() => store.repository?.path, repo => {
  if (repo) {
    path.value = repo
    watchRepository(repo)
  }
  navigation.value = ''
  branchError.value = ''
  branchNotice.value = ''
  branchName.value = ''
  branchFilter.value = ''
  collapsedBranches.value.clear()
  branchForm.value = undefined
  managedBranch.value = ''
  branchBase.value = ''
  branchMenu.value = undefined
  refKind.value = 'branches'
  statusRequest.clear()
  previewRequest.clear()
  clearEditor()
  inspection.value = undefined
  conflictFile.value = ''
  rebasePlan.value = []
  pinnedCommit.value = undefined
  advanced.value = undefined
  operation.value = undefined
  operationError.value = ''
  operationNotice.value = ''
  headRequest.clear()
  preparingOperation.value = false
  operationRequest.clear()
  void loadOperationState()
  workingFiles.value = []
  workingSelection.value = undefined
  commitMessage.value = ''
  amendTarget.value = undefined
  draftBeforeAmend = ''
  dailyPanel.value = undefined
  commitNotice.value = ''
  indexError.value = ''
  preview.value = undefined
  previewError.value = ''
  previewLoading.value = false
  expandedPreview.value = false
  expandedDiff.value = false
  view.value = 'working'
  if (!restoringSession.value) void loadWorking()
})


function watchRepository(repo: string) {
  watchError.value = ''
  const call = api.WatchRepository(repo)
  const current = watchRequest.replace(call)
  void call.catch(error => {
    if (current()) watchError.value = String(error)
  }).finally(() => watchRequest.finish(call))
}

async function refreshExternalChanges() {
  if (document.hidden || indexBusy.value || restoringSession.value) return
  void store.checkForChanges()
  void loadOperationState()
  if (workingLoading.value) return
  const repo = store.repository?.path
  await loadWorking()
  if (repo === store.repository?.path && workingSelection.value && workingView.value && !previewLoading.value) {
    void selectView(workingView.value, true, expandedPreview.value)
  }
}

async function loadWorking() {
  const repo = store.repository?.path
  if (!repo) return
  const call = api.WorkingChanges(repo)
  const current = statusRequest.replace(call)
  workingLoading.value = true
  try {
    const files = await call
    if (current()) {
      workingFiles.value = files ?? []
      workingError.value = ''
    }
  } catch (error) {
    if (current()) workingError.value = String(error)
  } finally {
    if (current()) workingLoading.value = false
    statusRequest.finish(call)
  }
}

function selectWorking(file: WorkingFile, next: FileView = 'working') {
  workingSelection.value = file
  showHunks.value = false
  indexError.value = ''
  expandedPreview.value = false
  void selectView(next)
}

async function selectView(next: FileView | 'commit', background = false, expanded = false) {
  view.value = next
  if (next === 'commit') return
  const repo = store.repository?.path
  const file = workingSelection.value
  if (!repo || !file) return
  if (!background) {
    preview.value = undefined
    previewError.value = ''
  }
  if (next !== 'working' && !currentWorkingFile.value) {
    previewRequest.clear()
    previewLoading.value = false
    preview.value = { kind: 'text', content: '' }
    previewError.value = ''
    return
  }
  previewLoading.value = !background
  const call = next === 'working'
    ? expanded ? api.WorkingPreviewExpanded(repo, file.path) : api.WorkingPreview(repo, file.path)
    : expanded ? api.WorkingDiffExpanded(repo, file.path, next === 'staged') : api.WorkingDiff(repo, file.path, next === 'staged')
  const current = previewRequest.replace(call)
  try {
    const result = await call
    if (current()) {
      preview.value = result
      previewError.value = ''
    }
  } catch (error) {
    if (current()) previewError.value = String(error)
  } finally {
    if (current()) previewLoading.value = false
    previewRequest.finish(call)
  }
}

function clearEditor() {
  editorRequest.clear()
  editorTabs.value = []
  activeEditorPath.value = ''
  editorError.value = ''
  editorLoading.value = false
  pendingEditorAction.value = undefined
  if (view.value === 'editor' || view.value === 'conflict') view.value = 'working'
}

function withSavedEditor(label: string, action: () => void) {
  if (editorSaving.value) return
  if (!editorTabs.value.some(isTabDirty)) {
    action()
    return
  }
  pendingEditorLabel.value = label
  pendingEditorAction.value = action
}

function discardEditorChanges() {
  const action = pendingEditorAction.value
  clearEditor()
  action?.()
}

function openRepository(folder: string) {
  if (!folder || indexBusy.value || store.opening) return
  if (folder === store.repository?.path) {
    void store.open(folder, true)
    return
  }
  withSavedEditor('opening another repository', () => { void store.open(folder) })
}

function openEditorFile(file: string) {
  if (!file || store.opening) return
  if (editorTabFor(file)) {
    activateEditorTab(file)
    return
  }
  void loadEditor(file)
}

function reloadEditor() {
  const file = activeEditorPath.value
  if (!file) return
  const target = view.value === 'conflict' ? 'conflict' : 'editor'
  withSavedEditor('reloading the file', () => { void loadEditor(file, target) })
}

async function loadEditor(file: string, targetView: 'editor' | 'conflict' = 'editor') {
  const repo = store.repository?.path
  if (!repo || editorSaving.value) return
  editorLoading.value = true
  editorError.value = ''
  view.value = targetView
  const call = api.OpenTextFile(repo, file)
  const current = editorRequest.replace(call)
  try {
    const text = await call
    if (!current()) return
    const existing = editorTabFor(file)
    if (existing) {
      existing.file = text
      existing.draft = text.content
    } else {
      editorTabs.value.push({ path: file, file: text, draft: text.content })
    }
    activeEditorPath.value = file
  } catch (cause) {
    if (current()) editorError.value = String(cause)
  } finally {
    if (current()) editorLoading.value = false
    editorRequest.finish(call)
  }
}

async function saveEditor(path = activeEditorPath.value) {
  const repo = store.repository?.path
  const tab = editorTabFor(path)
  if (!repo || !tab || !isTabDirty(tab) || editorLoading.value || indexBusy.value || store.opening) return
  const content = tab.draft
  const text = tab.file
  editorSaving.value = true
  indexBusy.value = true
  editorError.value = ''
  try {
    const revision = await api.SaveTextFile(repo, path, text.revision, content)
    if (repo !== store.repository?.path) return
    tab.file = { ...text, revision, content }
    tab.draft = content
    await loadWorking()
  } catch (cause) {
    if (repo === store.repository?.path) editorError.value = String(cause)
  } finally {
    editorSaving.value = false
    indexBusy.value = false
  }
}

function preventDiscard(event: BeforeUnloadEvent) {
  if (editorDirty.value || editorSaving.value) event.preventDefault()
}

function openConflict(file: string) {
  conflictFile.value = file
  if (editorTabFor(file)) {
    activeEditorPath.value = file
    view.value = 'conflict'
  } else {
    void loadEditor(file, 'conflict')
  }
}

function closeConflict() {
  view.value = editorTabs.value.length ? 'editor' : 'working'
}

async function openSubmodule() {
  const repo = store.repository?.path
  const file = store.file?.path
  if (!repo || !file || indexBusy.value) return
  submoduleError.value = ''
  try {
    const path = await api.SubmodulePath(repo, file)
    if (repo === store.repository?.path && file === store.file?.path) openRepository(path)
  } catch (cause) {
    if (repo === store.repository?.path && file === store.file?.path) submoduleError.value = String(cause)
  }
}

function lfsPointer(content: string) {
  const oid = /^oid sha256:([a-f0-9]{64})$/m.exec(content)?.[1]
  const size = /^size (\d+)$/m.exec(content)?.[1]
  return oid && size ? { oid, size } : undefined
}

function inspectFile(mode: 'history' | 'blame') {
  const file = inspectionPath.value
  const previous = view.value === 'editor' ? 'editor' : workingView.value
  if (!file || !previous) return
  inspection.value = { file, mode, previous }
  view.value = 'inspection'
}

function closeInspection() {
  const previous = inspection.value?.previous
  inspection.value = undefined
  view.value = previous === 'editor' && !editor.value ? 'working' : previous ?? 'working'
}

function openHistoricalCommit(commit: Commit) {
  const path = inspection.value?.file
  historyVisible.value = true
  view.value = 'commit'
  void store.selectCommit(commit, 0, path)
}

function compareRevisions(before: string | undefined, after: string, label: string) {
  if (!before || store.opening) return
  historyVisible.value = true
  diffExpanded.value = false
  view.value = 'commit'
  void store.selectComparison(before, after, label)
}

function pinCommit(commit: Commit) {
  pinnedCommit.value = pinnedCommit.value?.hash === commit.hash ? undefined : commit
}

function compareCommit(commit: Commit) {
  const base = pinnedCommit.value
  compareRevisions(base?.hash ?? store.headHash, commit.hash,
    (base?.hash.slice(0, 8) ?? 'HEAD') + ' → ' + commit.hash.slice(0, 8))
}

async function loadOperationState() {
  const repo = store.repository?.path
  if (!repo) return
  const call = api.OperationState(repo)
  const current = operationRequest.replace(call)
  try {
    const state = await call
    if (!current()) return
    operation.value = state
  } catch (cause) {
    if (current()) operationError.value = String(cause)
  } finally {
    operationRequest.finish(call)
  }
}

// A starting or failing history operation surfaces itself in the Git tool
// window's Console tab, mirroring IDEA where the console tab activates.
watch(() => [operation.value?.kind, operation.value?.conflicts?.length ?? 0, operationError.value], ([kind, conflicts, error]) => {
  if (kind || conflicts || error) gitTab.value = 'console'
})

async function prepareOperation(action: AdvancedAction, hash: string, label: string) {
  const repo = store.repository?.path
  if (!repo || indexBusy.value || store.opening || preparingOperation.value) return
  if (operation.value?.kind || operation.value?.conflicts?.length) {
    operationError.value = 'Finish the current Git operation or resolve the unmerged files first.'
    return
  }
  advanced.value = undefined
  rebasePlan.value = []
  resetMode.value = 'mixed'
  hardResetConfirmed.value = false
  operationError.value = ''
  operationNotice.value = ''
  preparingOperation.value = true
  const call = api.ReadHead(repo)
  const current = headRequest.replace(call)
  try {
    const head = await call
    if (!current()) return
    if (action === 'rebase') {
      const commits = await api.RebasePlan(repo, hash, head)
      if (!current()) return
      rebasePlan.value = (commits ?? []).map(commit => ({ commit, action: 'pick' }))
    }
    advanced.value = { action, hash, label, head }
  } catch (cause) {
    if (current()) operationError.value = String(cause)
  } finally {
    if (current()) preparingOperation.value = false
    headRequest.finish(call)
  }
}

function moveRebaseStep(index: number, offset: number) {
  const next = index + offset
  if (next < 0 || next >= rebasePlan.value.length || indexBusy.value) return
  const [step] = rebasePlan.value.splice(index, 1)
  rebasePlan.value.splice(next, 0, step)
}

async function runAdvanced() {
  const repo = store.repository?.path
  const request = advanced.value
  if (!repo || !request || indexBusy.value || store.opening ||
      (request.action === 'reset' && resetMode.value === 'hard' && !hardResetConfirmed.value)) return
  const mode = resetMode.value
  advanced.value = undefined
  indexBusy.value = true
  operationError.value = ''
  operationNotice.value = ''
  try {
    if (request.action === 'rebase') {
      const steps = rebasePlan.value.map(step => ({ hash: step.commit.hash, action: step.action }))
      await api.RunInteractiveRebase(repo, request.hash, request.head, steps)
    } else {
      await api.ApplyHistoryOperation(repo, request.action, request.hash, request.head, mode)
    }
    operationNotice.value = advancedLabels[request.action] + ' complete.'
  } catch (cause) {
    operationError.value = String(cause)
  } finally {
    try {
      await Promise.all([refreshAfterDailyOperation(), loadOperationState()])
      if (operation.value?.conflicts?.length) {
        sidebar.value = 'changes'
        commitTab.value = 'commit'
      }
    } finally {
      indexBusy.value = false
    }
  }
}

async function finishOperation(abort: boolean) {
  const repo = store.repository?.path
  const kind = operation.value?.kind
  if (!repo || !kind || indexBusy.value || store.opening) return
  indexBusy.value = true
  operationError.value = ''
  operationNotice.value = ''
  try {
    await api.FinishHistoryOperation(repo, kind, abort)
    operationNotice.value = abort ? 'Git operation aborted.' : 'Git operation continued.'
  } catch (cause) {
    operationError.value = String(cause)
  } finally {
    try {
      await Promise.all([refreshAfterDailyOperation(), loadOperationState()])
    } finally {
      indexBusy.value = false
    }
  }
}

const selectedConflict = computed(() => currentWorkingFile.value?.path &&
  operation.value?.conflicts?.includes(currentWorkingFile.value.path) ? currentWorkingFile.value.path : undefined)

async function markResolved(file = selectedConflict.value) {
  const repo = store.repository?.path
  if (!repo || !file || indexBusy.value) return
  if (editorDirty.value && editorPath.value === file) {
    operationError.value = 'Save your edits before marking this file resolved.'
    return
  }
  indexBusy.value = true
  operationError.value = ''
  try {
    await api.ResolveConflict(repo, file)
  } catch (cause) {
    operationError.value = String(cause)
  } finally {
    try {
      await Promise.all([loadWorking(), loadOperationState()])
      if (workingSelection.value && workingView.value) await selectView(workingView.value, true)
    } finally {
      indexBusy.value = false
    }
  }
}

async function applyHunk(file: string, staged: boolean, revision: string, index: number, lines?: number[]) {
  const repo = store.repository?.path
  if (!repo || indexBusy.value || store.opening) return
  indexBusy.value = true
  try {
    if (lines) await api.SetHunkLinesStaged(repo, file, staged, revision, index, lines)
    else await api.SetHunkStaged(repo, file, staged, revision, index)
  } finally {
    try {
      await loadWorking()
    } finally {
      indexBusy.value = false
    }
  }
}

function updateIndex(staged: boolean) {
  if (currentWorkingFile.value) void updateIncludedFiles([currentWorkingFile.value.path], staged)
}

async function updateIncludedFiles(paths: string[], staged: boolean) {
  const repo = store.repository?.path
  if (!repo || indexBusy.value || store.opening || restoringSession.value) return
  const selected = new Set(paths)
  const files = workingFiles.value.filter(file => selected.has(file.path) &&
    (staged ? file.canStage : file.canUnstage))
  if (!files.length) return
  indexBusy.value = true
  indexError.value = ''
  try {
    for (const file of files) await api.SetStaged(repo, file.path, staged)
  } catch (error) {
    if (repo === store.repository?.path) indexError.value = String(error)
  } finally {
    if (repo === store.repository?.path) {
      await loadWorking()
      if (view.value === 'staged' || view.value === 'unstaged') void selectView(view.value)
    }
    indexBusy.value = false
  }
}

async function startAmend() {
  const repo = store.repository?.path
  if (!repo || indexBusy.value || store.opening || amendTarget.value) return
  indexBusy.value = true
  indexError.value = ''
  commitNotice.value = ''
  try {
    const target = await api.PrepareAmend(repo)
    if (repo !== store.repository?.path) return
    draftBeforeAmend = commitMessage.value
    amendTarget.value = target
    commitMessage.value = target.message
    void nextTick(() => document.getElementById('commit-message')?.focus())
  } catch (cause) {
    if (repo === store.repository?.path) indexError.value = String(cause)
  } finally {
    indexBusy.value = false
  }
}

function cancelAmend() {
  if (!amendTarget.value) return
  amendTarget.value = undefined
  commitMessage.value = draftBeforeAmend
  draftBeforeAmend = ''
  indexError.value = ''
}

async function refreshAfterDailyOperation() {
  const repo = store.repository?.path
  if (!repo) return
  await Promise.all([store.open(repo, true), loadWorking()])
  if (repo === store.repository?.path && workingSelection.value && workingView.value) {
    await selectView(workingView.value, true)
  }
}

function toggleDailyPanel(panel: DailyPanel) {
  if (indexBusy.value || store.opening) return
  dailyPanel.value = dailyPanel.value === panel ? undefined : panel
  if (dailyPanel.value === 'init') {
    projectPath.value = store.repository ? '' : path.value
    projectError.value = ''
  }
}

async function pickProject() {
  if (picking.value || indexBusy.value) return
  picking.value = true
  try {
    const folder = await api.ChooseDirectory()
    if (folder) projectPath.value = folder
  } catch (cause) {
    projectError.value = String(cause)
  } finally {
    picking.value = false
  }
}

async function initializeProject() {
  if (indexBusy.value || picking.value || !projectPath.value.trim()) return
  indexBusy.value = true
  projectError.value = ''
  try {
    const repo = await api.Initialize(projectPath.value.trim())
    await store.open(repo.path)
    sidebar.value = 'changes'
    commitTab.value = 'commit'
  } catch (cause) {
    projectError.value = String(cause)
  } finally {
    indexBusy.value = false
  }
}

async function stageAll() {
  const repo = store.repository?.path
  if (!repo || indexBusy.value || store.opening) return
  indexBusy.value = true
  indexError.value = ''
  try {
    await api.StageAll(repo)
  } catch (cause) {
    indexError.value = String(cause)
  } finally {
    try {
      await loadWorking()
      if (workingSelection.value && workingView.value) await selectView(workingView.value, true)
    } finally {
      indexBusy.value = false
    }
  }
}

async function commitChanges(reviewPush = false) {
  const repo = store.repository?.path
  const message = commitMessage.value
  const target = amendTarget.value
  if (!repo || indexBusy.value || store.opening || !message.trim() ||
      (!target && !workingFiles.value.some(file => file.canUnstage))) return
  indexBusy.value = true
  indexError.value = ''
  commitNotice.value = ''
  try {
    if (target) await api.AmendIndex(repo, target.hash, target.ref, message)
    else await api.CommitIndex(repo, message)
    if (repo !== store.repository?.path) return
    if (target) cancelAmend()
    else commitMessage.value = ''
    commitNotice.value = target ? 'Commit amended' : 'Commit created'
    await Promise.all([store.open(repo), loadWorking()])
    if (repo === store.repository?.path && (view.value === 'staged' || view.value === 'unstaged')) {
      void selectView(view.value)
    }
    if (reviewPush && repo === store.repository?.path) {
      syncIntent.value = 'push'
      dailyPanel.value = 'sync'
    }
  } catch (error) {
    if (repo === store.repository?.path) indexError.value = String(error)
  } finally {
    indexBusy.value = false
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', outsideMenus)
  window.addEventListener('beforeunload', preventDiscard)
  window.addEventListener('keydown', onGlobalKeydown)
  void store.detectGit()
  void restoreSession()
  unsubscribeChanges = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === store.repository?.path) void refreshExternalChanges()
  })
  timer = setInterval(() => { void refreshExternalChanges() }, 5000)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', outsideMenus)
  window.removeEventListener('beforeunload', preventDiscard)
  window.removeEventListener('keydown', onGlobalKeydown)
  if (sessionTimer) clearTimeout(sessionTimer)
  editorRequest.clear()
  headRequest.clear()
  operationRequest.clear()
  clearInterval(timer)
  unsubscribeChanges?.()
  watchRequest.clear()
  statusRequest.clear()
  previewRequest.clear()
})

async function choose() {
  if (picking.value) return
  picking.value = true
  try { await store.choose(openRepository) } finally { picking.value = false }
}

function refresh() {
  if (store.repository) void store.open(store.repository.path, true)
  void loadWorking()
  if (workingSelection.value && workingView.value) void selectView(workingView.value)
}

async function updateBranch(operation: 'create' | 'switch' | 'rename' | 'delete', name: string) {
  const repo = store.repository?.path
  const base = createBase.value
  const original = managedBranch.value
  if (!repo || !name || indexBusy.value || (operation === 'create' && !base)) return
  indexBusy.value = true
  branchError.value = ''
  branchNotice.value = ''
  navigation.value = ''
  try {
    switch (operation) {
      case 'create':
        if (!base) return
        await api.CreateBranch(repo, name, base)
        break
      case 'switch':
        await api.SwitchBranch(repo, name)
        break
      case 'rename':
        await api.RenameBranch(repo, original, name)
        break
      case 'delete':
        await api.DeleteBranch(repo, name)
        break
    }
    if (repo !== store.repository?.path) return
    if (operation === 'switch') {
      cancelAmend()
      branchNotice.value = 'Switched to ' + name
      previewRequest.clear()
      workingSelection.value = undefined
      preview.value = undefined
      previewError.value = ''
      previewLoading.value = false
      view.value = 'working'
      await Promise.all([store.open(repo), loadWorking()])
    } else {
      openBranchForm()
      branchFilter.value = ''
      branchNotice.value = { create: 'Created ', rename: 'Renamed to ', delete: 'Deleted ' }[operation] + name
      await store.open(repo, true)
    }
  } catch (error) {
    if (repo === store.repository?.path) branchError.value = String(error)
  } finally {
    indexBusy.value = false
  }
}

async function jump(hash: string) {
  historyVisible.value = true
  diffExpanded.value = false
  await nextTick()
  const repo = store.repository
  navigation.value = ''
  while (repo === store.repository && !log.value?.jump(hash)) {
    if (!store.more || store.loading || store.logError) {
      navigation.value = 'This ref is outside the currently loaded history.'
      return
    }
    await store.loadPage()
  }
}

function maximizeDiff() {
  view.value = 'commit'
  diffExpanded.value = true
  void nextTick(() => restoreDiff.value?.focus())
}

function restoreDiffLayout() {
  diffExpanded.value = false
  void nextTick(() => {
    const selector = historyVisible.value ? '.file-row.active' : '.history-toggle'
    const button = content.value?.querySelector<HTMLButtonElement>(selector)
    const fallback = content.value?.querySelector<HTMLButtonElement>('.history-toggle')
    const focusTarget = button ?? fallback
    focusTarget?.focus()
  })
}

function createBranchFromCommit() {
  if (indexBusy.value) return
  openSidebar('branches')
  refKind.value = 'branches'
  openBranchForm('create')
}

function closeGitMenu() {
  if (gitMenu.value) gitMenu.value.open = false
}

function gitAction(action: () => void) {
  closeGitMenu()
  action()
}

function openSync(intent: SyncIntent) {
  if (indexBusy.value || store.opening) return
  if (dailyPanel.value === 'sync' && syncIntent.value === intent) dailyPanel.value = undefined
  else {
    syncIntent.value = intent
    dailyPanel.value = 'sync'
  }
}

function openSidebar(panel: Sidebar) {
  if (indexBusy.value) return
  closeBranchMenu()
  if (panel === 'branches') {
    historyVisible.value = true
    void nextTick(() => content.value?.querySelector<HTMLInputElement>('[aria-label="Filter branches or tags"]')?.focus())
  } else {
    sidebar.value = panel
    if (panel === 'changes') commitTab.value = 'commit'
  }
  diffExpanded.value = false
}

function openShelf() {
  if (indexBusy.value || store.opening) return
  openSidebar('changes')
  commitTab.value = 'shelf'
}

function focusCommit() {
  openSidebar('changes')
  void nextTick(() => document.getElementById('commit-message')?.focus())
}

function closeBranchMenu() {
  branchMenu.value = undefined
}

function openBranchActions(item: GitRef, event: MouseEvent | KeyboardEvent) {
  branchMenu.value = { item, anchor: contextAnchor(event) }
}

function outsideMenus(event: PointerEvent) {
  if (!(event.target instanceof Node)) return
  if (!gitMenu.value?.contains(event.target)) closeGitMenu()
}

function branchAction(action: 'history' | 'create' | 'switch' | 'rename' | 'delete' | 'compare' | 'merge' | 'rebase' | SyncIntent) {
  const item = branchMenu.value?.item
  if (!item) return
  closeBranchMenu()
  const name = refLabel(item.name)
  switch (action) {
    case 'history': void jump(item.hash); break
    case 'compare': compareRevisions(store.headHash, item.hash, 'HEAD → ' + name); break
    case 'merge': case 'rebase': void prepareOperation(action, item.hash, name); break
    case 'create': openBranchForm('create', name, item.hash); break
    case 'switch': withSavedEditor('switching branches', () => { void updateBranch('switch', name) }); break
    case 'rename': case 'delete': openBranchForm(action, name); break
    default: openSync(action)
  }
}

function split(event: PointerEvent) {
  if (!content.value || !(event.currentTarget instanceof HTMLElement)) return
  event.currentTarget.setPointerCapture(event.pointerId)
}

function resize(event: PointerEvent) {
  if (!content.value || !(event.currentTarget instanceof HTMLElement) || !event.currentTarget.hasPointerCapture(event.pointerId)) return
  const bounds = content.value.getBoundingClientRect()
  previewHeight.value = Math.min(70, Math.max(28, Math.round((event.clientY - bounds.top) / bounds.height * 100)))
}

function resizeWithKey(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  previewHeight.value = Math.min(70, Math.max(28, previewHeight.value + (event.key === 'ArrowUp' ? -3 : 3)))
}

/* ---- Tool window splitters. Dragging persists through the preferences
 *  store; arrow keys give the same control from the keyboard. ---- */

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function trackDrag(event: PointerEvent) {
  if (!(event.currentTarget instanceof HTMLElement)) return undefined
  event.currentTarget.setPointerCapture(event.pointerId)
  return event.currentTarget
}

function dragging(event: PointerEvent) {
  return event.currentTarget instanceof HTMLElement && event.currentTarget.hasPointerCapture(event.pointerId)
}

function resizeSidebar(event: PointerEvent) {
  if (!content.value || !dragging(event)) return
  const bounds = content.value.getBoundingClientRect()
  preferences.sidebarWidth = Math.round(clamp(event.clientX - bounds.left - 38,
    layoutBounds.sidebar.min, layoutBounds.sidebar.max))
}

function resizeSidebarWithKey(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  preferences.sidebarWidth = Math.round(clamp(preferences.sidebarWidth + (event.key === 'ArrowLeft' ? -24 : 24),
    layoutBounds.sidebar.min, layoutBounds.sidebar.max))
}

function resizeBranches(event: PointerEvent) {
  if (!content.value || !dragging(event)) return
  const bounds = content.value.getBoundingClientRect()
  preferences.branchesWidth = Math.round(clamp(event.clientX - bounds.left,
    layoutBounds.branches.min, layoutBounds.branches.max))
}

function resizeBranchesWithKey(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  preferences.branchesWidth = Math.round(clamp(preferences.branchesWidth + (event.key === 'ArrowLeft' ? -24 : 24),
    layoutBounds.branches.min, layoutBounds.branches.max))
}

function resizeFiles(event: PointerEvent) {
  if (!content.value || !dragging(event)) return
  const bounds = content.value.getBoundingClientRect()
  preferences.filesWidth = Math.round(clamp(bounds.right - event.clientX,
    layoutBounds.files.min, layoutBounds.files.max))
}

function resizeFilesWithKey(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  preferences.filesWidth = Math.round(clamp(preferences.filesWidth + (event.key === 'ArrowLeft' ? 24 : -24),
    layoutBounds.files.min, layoutBounds.files.max))
}

function resizeDetails(event: PointerEvent) {
  if (!content.value) return
  const host = event.currentTarget instanceof HTMLElement ? event.currentTarget.parentElement : null
  if (!host || !(event.currentTarget instanceof HTMLElement) ||
      !event.currentTarget.hasPointerCapture(event.pointerId)) return
  const bounds = host.getBoundingClientRect()
  preferences.detailsHeight = Math.round(clamp((event.clientY - bounds.top) / bounds.height * 100,
    layoutBounds.details.min, layoutBounds.details.max))
}

function resizeDetailsWithKey(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  preferences.detailsHeight = Math.round(clamp(preferences.detailsHeight + (event.key === 'ArrowUp' ? -3 : 3),
    layoutBounds.details.min, layoutBounds.details.max))
}

/** A log selection updates the Changed Files and Commit Details panels and
 *  reveals the commit diff tab — but never while the user is editing a file
 *  or resolving a conflict, matching IDEA where an open editor is not stolen
 *  by log selection. Editor tabs keep their content and can be switched back
 *  to at any time. */
function handleHistorySelect() {
  if (view.value === 'editor' || view.value === 'conflict' || view.value === 'inspection') return
  view.value = 'commit'
}

/** Double-clicking a history file opens (or focuses) the commit diff tab in
 *  the editor workspace instead of maximizing the preview. */
function openHistoryDiff() {
  view.value = 'commit'
}

/** The workspace geometry: activity bar + Commit tool window + editor on
 *  top, the Git tool window across the full width below. */
const contentStyle = computed(() => ({
  gridTemplateColumns: diffExpanded.value
    ? 'minmax(0, 1fr)'
    : sidebarHidden.value
      ? '38px minmax(0, 1fr)'
      : '38px ' + preferences.sidebarWidth + 'px 6px minmax(0, 1fr)',
  gridTemplateRows: historyMaximized.value
    ? 'minmax(0, 0fr) auto minmax(0, 1fr)'
    : historyVisible.value && !diffExpanded.value
      ? 'minmax(0, ' + previewHeight.value + 'fr) auto minmax(0, ' + (100 - previewHeight.value) + 'fr)'
      : 'minmax(0, 1fr)',
}))

/* ---- Keyboard commands, command palette, session restore ---- */

function stepChange(direction: number) {
  if (!store.repository || !store.files.length) return
  historyVisible.value = true
  diffExpanded.value = false
  view.value = 'commit'
  filesPanel.value?.step(direction)
}

async function toggleFullscreen() {
  try {
    await Window.ToggleFullscreen()
    windowError.value = ''
  } catch (error) {
    windowError.value = 'Could not toggle fullscreen: ' + String(error)
  }
}

function runShortcut(id: ShortcutId) {
  switch (id) {
    case 'palette': paletteOpen.value = !paletteOpen.value; break
    case 'openRepository': void choose(); break
    case 'refresh': refresh(); break
    case 'searchHistory':
      if (store.repository) {
        historyVisible.value = true
        void nextTick(() => log.value?.openSearch())
      }
      break
    case 'commit': focusCommit(); break
    case 'previousCommit': log.value?.step(-1); break
    case 'nextCommit': log.value?.step(1); break
    case 'previousChange': stepChange(-1); break
    case 'nextChange': stepChange(1); break
    case 'previousHunk': navigateDiff(-1); break
    case 'nextHunk': navigateDiff(1); break
    case 'toggleHistory': historyVisible.value = !historyVisible.value; diffExpanded.value = false; break
    case 'maximizeDiff':
      if (diffExpanded.value) restoreDiffLayout()
      else if (view.value === 'commit' && store.file) maximizeDiff()
      break
    case 'toggleTheme': dark.value = !dark.value; break
    case 'toggleDensity': preferences.toggleDensity(); break
    case 'toggleFullscreen': void toggleFullscreen(); break
  }
}

function isCodeEditor(target: EventTarget | null): boolean {
  return target instanceof Element && !!target.closest('.cm-editor')
}

function isCommitField(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.id === 'commit-message'
}

function onGlobalKeydown(event: KeyboardEvent) {
  if (keymap.recording || paletteOpen.value || event.defaultPrevented || event.isComposing) return
  const id = keymap.resolve(event)
  if (!id) return
  // CodeMirror panes own keys like Ctrl+F and Ctrl+Enter; the global
  // dispatcher must not also run the workspace command for the same press.
  if (editorScopeShortcuts.has(id) && isCodeEditor(event.target)) return
  // Navigation keys belong to text fields, but modifier shortcuts still work.
  const textEntry = isTextEntry(event.target)
  if (navigationShortcuts.has(id) && textEntry) return
  // In text fields, only a deliberate Ctrl/Meta chord can submit. AltGr
  // also reports Ctrl on Windows, but is used to type ordinary characters.
  if (id === 'commit' && (event.repeat || (textEntry &&
      (!(event.ctrlKey || event.metaKey) || event.getModifierState('AltGraph'))))) return
  event.preventDefault()
  // Submitting happens only through the commit message field itself, so a
  // rebound or duplicated dispatch can never fire Git a second time.
  if (id === 'commit' && isCommitField(event.target)) changesPanel.value?.submit()
  else runShortcut(id)
}

function closePalette() {
  paletteOpen.value = false
  void nextTick(() => paletteTrigger.value?.focus())
}

function openSettings() {
  paletteOpen.value = false
  void nextTick(() => {
    if (settings.value) settings.value.open = true
    settings.value?.querySelector('summary')?.focus()
  })
}

function runPalette(id: string) {
  paletteOpen.value = false
  const command = paletteCommands.value.find(item => item.id === id)
  if (command && !command.disabled) command.run()
}

function forgetSession() {
  clearSession()
  sessionNotice.value = 'Saved session cleared.'
}

async function restoreSession() {
  const saved = readSession()
  if (!saved || !resumeOnStart.value) return
  restoringSession.value = true
  try {
    await store.open(saved.path)
    const repo = store.repository
    if (!repo) {
      clearSession()
      store.repoError = ''
      restoreNotice.value = 'The last repository is no longer available. Choose a repository to continue.'
      return
    }
    sidebar.value = !saved.sidebar || saved.sidebar === 'branches' ? 'changes' : saved.sidebar
    sidebarHidden.value = false
    historyVisible.value = saved.sidebar === 'branches' || (saved.historyVisible ?? true)
    historyMaximized.value = saved.historyMaximized ?? false
    // Sessions written without a view used the revision to choose the preview.
    const savedView = saved.view ?? (saved.hash || saved.range ? 'commit' : 'working')
    const nextView = savedView === 'commit' || savedView === 'staged' || savedView === 'unstaged'
      ? savedView : 'working'

    if (saved.hash) {
      let commit = store.commits.find(item => item.hash === saved.hash)
      while (!commit && repo === store.repository && store.more && !store.loading && !store.logError) {
        await store.loadPage()
        commit = store.commits.find(item => item.hash === saved.hash)
      }
      if (repo !== store.repository) return
      if (commit) await store.selectCommit(commit, saved.parent ?? 0, saved.range ? undefined : saved.file)
    }
    if (repo !== store.repository) return
    if (saved.range) {
      await store.selectComparison(saved.range.before, saved.range.after, saved.range.label, saved.file)
    }
    if (repo !== store.repository) return

    await loadWorking()
    if (repo !== store.repository) return
    if (saved.workingFile) {
      workingSelection.value = workingFiles.value.find(item => item.path === saved.workingFile)
        ?? workingFiles.value.find(item => item.oldPath === saved.workingFile)
    }
    // Reopen the editor tabs from the previous session (drafts are not saved).
    for (const file of saved.editorFiles ?? []) {
      if (repo.path !== store.repository?.path) break
      await loadEditor(file)
    }
    if (repo !== store.repository) return
    if (saved.editorActive && editorTabFor(saved.editorActive)) activeEditorPath.value = saved.editorActive
    await selectView(nextView)
    if (savedView === 'editor' && activeEditorPath.value) view.value = 'editor'
  } finally {
    restoringSession.value = false
    scheduleSessionSave()
  }
}

let sessionTimer: ReturnType<typeof setTimeout> | undefined
function scheduleSessionSave(attempt = 0) {
  if (sessionTimer) clearTimeout(sessionTimer)
  if (restoringSession.value) return
  sessionTimer = setTimeout(() => {
    const repo = store.repository?.path
    // Never persist the empty state produced while a refresh resets the store.
    if (!repo || store.opening || store.loading) {
      if (attempt < 5) scheduleSessionSave(attempt + 1)
      return
    }
    saveSession({
      path: repo,
      hash: store.selected?.hash,
      parent: store.parent,
      file: store.file?.path,
      workingFile: workingSelection.value?.path,
      range: store.comparisonRange ? { ...store.comparisonRange } : undefined,
      sidebar: sidebar.value,
      view: view.value,
      historyVisible: historyVisible.value,
      historyMaximized: historyMaximized.value,
      editorFiles: editorTabs.value.map(tab => tab.path),
      editorActive: activeEditorPath.value || undefined,
    })
  }, 350)
}
watch([
  () => store.repository?.path, () => store.selected?.hash, () => store.parent,
  () => store.file?.path, () => workingSelection.value?.path, () => store.comparisonRange,
  sidebar, view, historyVisible, historyMaximized, editorTabs.value.length, activeEditorPath,
], () => scheduleSessionSave())

const paletteCommands = computed<PaletteCommand[]>(() => {
  const items: PaletteCommand[] = []
  const hint = (id: ShortcutId) => keymap.bindings[id] || undefined
  const add = (id: string, label: string, group: string, run: () => void, options: Partial<PaletteCommand> = {}) => {
    items.push({ id, label, group, run, ...options })
  }
  const busy = indexBusy.value || store.opening
  add('open', 'Browse for a repository…', 'Repository', () => void choose(), { hint: hint('openRepository'), disabled: picking.value || busy })
  add('init', 'Initialize Project…', 'Repository', () => toggleDailyPanel('init'), { disabled: busy })
  for (const repo of store.recent) {
    add('repo:' + repo, 'Open ' + (repo.split(/[\\/]/).at(-1) || repo), 'Recent repositories', () => openRepository(repo), { keywords: repo, disabled: busy })
  }
  if (store.repository) {
    add('refresh', 'Refresh repository', 'Repository', refresh, { hint: hint('refresh'), disabled: busy })
    add('search', 'Search Git history…', 'Repository', () => {
      historyVisible.value = true
      void nextTick(() => log.value?.openSearch())
    }, { hint: hint('searchHistory') })
    add('commit', 'Commit changes…', 'Git operations', focusCommit, { hint: hint('commit'), disabled: busy })
    add('fetch', 'Fetch…', 'Git operations', () => openSync('fetch'), { disabled: busy })
    add('pull', 'Pull…', 'Git operations', () => openSync('pull'), { disabled: busy })
    add('push', 'Push…', 'Git operations', () => openSync('push'), { disabled: busy })
    add('stash', 'Stash…', 'Git operations', openShelf, { disabled: busy })
    add('remotes', 'Manage Remotes…', 'Git operations', () => toggleDailyPanel('remotes'), { disabled: busy })
    add('publish', 'Publish to GitHub…', 'Git operations', () => toggleDailyPanel('publish'), { disabled: busy })
    add('new-branch', 'New Branch…', 'Git operations', createBranchFromCommit, { disabled: busy || !store.selected })
  }
  for (const tab of sidebarTabs) {
    add('sidebar:' + tab.id, 'Show ' + tab.label, 'Workspace views', () => openSidebar(tab.id), { disabled: busy })
  }
  if (store.repository) {
    add('previousCommit', 'Previous commit', 'Navigation', () => log.value?.step(-1), { hint: hint('previousCommit') })
    add('nextCommit', 'Next commit', 'Navigation', () => log.value?.step(1), { hint: hint('nextCommit') })
    add('previousChange', 'Previous changed file', 'Navigation', () => stepChange(-1), { hint: hint('previousChange') })
    add('nextChange', 'Next changed file', 'Navigation', () => stepChange(1), { hint: hint('nextChange') })
    add('previousHunk', 'Previous diff hunk', 'Navigation', () => navigateDiff(-1), { hint: hint('previousHunk'), disabled: view.value !== 'commit' })
    add('nextHunk', 'Next diff hunk', 'Navigation', () => navigateDiff(1), { hint: hint('nextHunk'), disabled: view.value !== 'commit' })
  }
  add('theme', 'Toggle dark theme', 'View', () => { dark.value = !dark.value }, { hint: hint('toggleTheme') })
  add('density', density.value === 'compact' ? 'Use comfortable rows' : 'Use compact rows', 'View', () => preferences.toggleDensity(), { hint: hint('toggleDensity') })
  add('fullscreen', 'Toggle fullscreen', 'View', () => void toggleFullscreen(), { hint: hint('toggleFullscreen') })
  add('history', 'Show or hide history', 'View', () => { historyVisible.value = !historyVisible.value; diffExpanded.value = false }, { hint: hint('toggleHistory') })
  add('diff', diffExpanded.value ? 'Restore diff layout' : 'Maximize diff', 'View', () => diffExpanded.value ? restoreDiffLayout() : maximizeDiff(), { hint: hint('maximizeDiff'), disabled: !diffExpanded.value && (view.value !== 'commit' || !store.file) })
  add('file-tree', fileTree.value ? 'Show Changed Files flat' : 'Show Changed Files as a tree', 'View', () => { fileTree.value = !fileTree.value })
  add('reset-size', 'Reset panel sizes', 'View', () => { previewHeight.value = defaultPreviewHeight })
  add('shortcuts', 'Keyboard shortcuts…', 'Settings', openSettings)
  add('forget-session', 'Forget saved session', 'Settings', forgetSession)
  if (store.repository) {
    for (const ref of store.branchRefs.slice(0, 30)) {
      const name = refLabel(ref.name)
      add('ref:' + ref.name, 'Jump to ' + name, 'Branches & tags', () => void jump(ref.hash), { keywords: ref.name })
    }
    for (const tag of store.tagRefs.slice(0, 15)) {
      add('tag:' + tag.name, 'Jump to tag ' + refLabel(tag.name), 'Branches & tags', () => void jump(tag.hash), { keywords: tag.name })
    }
  }
  return items
})
</script>

<template>
  <div class="shell" :data-theme="dark ? 'dark' : 'light'" :data-density="density">
    <header class="topbar">
      <div class="brand">
        <span class="brand-icon">
          <img :src="appIcon" alt="" />
        </span>
        <strong>GitVela</strong>
      </div>
      <button v-if="store.repository" class="branch-chip" :disabled="indexBusy || store.opening"
              :title="'Current branch: ' + (store.repository.head === 'HEAD' ? 'detached HEAD' : store.repository.head)"
              @click="openSidebar('branches')">
        <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
          <circle cx="6" cy="4" r="1.7" /><circle cx="6" cy="15" r="1.7" /><circle cx="14" cy="5" r="1.7" />
          <path d="M6 5.7v7.6m8-6.6a6 6 0 0 1-6 5.8H6" />
        </svg>
        <span class="ellipsis">{{ store.repository.head === 'HEAD' ? 'detached HEAD' : store.repository.head }}</span>
      </button>
      <details ref="git-menu" class="git-menu" @keydown.esc.stop="closeGitMenu(); gitMenu?.querySelector('summary')?.focus()">
        <summary>
          <svg class="menu-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
            <circle cx="6" cy="4" r="1.7" /><circle cx="6" cy="15" r="1.7" /><circle cx="14" cy="5" r="1.7" />
            <path d="M6 5.7v7.6m8-6.6a6 6 0 0 1-6 5.8H6" />
          </svg>
          Git
        </summary>
        <div class="command-menu" role="group" aria-label="Git operations">
          <button :disabled="indexBusy || store.opening" @click="gitAction(() => toggleDailyPanel('init'))">Initialize Project…</button>
          <template v-if="store.repository">
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(focusCommit)">Commit…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('push'))">Push…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('pull'))">Pull…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('fetch'))">Fetch…</button>
            <button :disabled="indexBusy" @click="gitAction(openShelf)">Stash…</button>
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(() => { openSidebar('branches'); refKind = 'branches' })">Branches</button>
            <button :disabled="indexBusy || !store.selected" @click="gitAction(createBranchFromCommit)">New Branch…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSidebar('worktrees'))">Worktrees</button>
            <button :disabled="indexBusy || store.opening" @click="gitAction(refresh)">Refresh</button>
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(() => toggleDailyPanel('remotes'))">Manage Remotes…</button>
            <button :disabled="indexBusy" @click="gitAction(() => toggleDailyPanel('publish'))">Publish to GitHub…</button>
          </template>
        </div>
      </details>
      <form class="repo-form" @submit.prevent="openRepository(path.trim())">
        <input v-model="path" aria-label="Repository path" placeholder="Enter a local repository path" spellcheck="false" />
        <button type="submit" :disabled="!path.trim() || store.opening || indexBusy">Open path</button>
      </form>
      <select v-if="store.recent.length" class="recent-repos" aria-label="Recent repositories" :disabled="indexBusy || store.opening"
              value="" @change="openRepository(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''">
        <option value="" disabled>Recent</option>
        <option v-for="repo in store.recent" :key="repo" :value="repo">{{ repo.split(/[\\/]/).at(-1) || repo }}</option>
      </select>
      <button class="primary" :disabled="picking || store.opening || indexBusy" @click="choose">Browse…</button>
      <button ref="paletteTrigger" :title="'Command palette (' + (keymap.bindings.palette || 'unbound') + ')'"
              @click="paletteOpen = !paletteOpen">Commands…</button>
      <details ref="settings" class="preferences">
        <summary>Settings</summary>
        <div class="preferences-panel">
          <label><input v-model="dark" type="checkbox" /> Dark theme</label>
          <label><input type="checkbox" :checked="density === 'compact'" @change="preferences.toggleDensity()" /> Compact rows</label>
          <label><input v-model="fileTree" type="checkbox" /> Show Changed Files as a tree</label>
          <label><input v-model="resumeOnStart" type="checkbox" /> Reopen last repository on startup</label>
          <button @click="previewHeight = defaultPreviewHeight">Reset panel sizes</button>
          <button @click="toggleFullscreen">Toggle fullscreen</button>
          <button @click="forgetSession">Forget saved session</button>
          <p v-if="sessionNotice" class="notice" role="status">{{ sessionNotice }}</p>
          <details class="shortcut-details">
            <summary>Keyboard shortcuts</summary>
            <ShortcutSettings />
          </details>
        </div>
      </details>
    </header>
    <div v-if="workspaceError" class="notice error top-error" role="alert">
      {{ workspaceError }}
      <button @click="workspaceError = ''; refresh()">Reload workspace</button>
    </div>
    <p v-if="store.repoError" class="notice error top-error" role="alert">{{ store.repoError }}</p>
    <p v-if="windowError" class="notice error top-error" role="alert">{{ windowError }} <button @click="windowError = ''">Dismiss</button></p>
    <p v-if="restoreNotice" class="notice top-error" role="status">{{ restoreNotice }} <button @click="restoreNotice = ''">Dismiss</button></p>
    <p v-if="watchError" class="notice top-error" role="status" :title="watchError">File notifications unavailable. Checking for Git changes every 5 seconds.</p>
    <section v-if="dailyPanel === 'init'" class="daily-panel" aria-label="Initialize project">
      <form class="daily-controls" @submit.prevent="withSavedEditor('initializing another project', () => { void initializeProject() })">
        <h2>Initialize project</h2>
        <label>Project folder <input v-model="projectPath" placeholder="Absolute folder path" :disabled="indexBusy" /></label>
        <button type="button" :disabled="indexBusy || picking" @click="pickProject">Browse…</button>
        <button type="submit" class="primary" :disabled="indexBusy || picking || !projectPath.trim()">Initialize Git</button>
        <button type="button" :disabled="indexBusy || picking" @click="dailyPanel = undefined">Cancel</button>
      </form>
      <p v-if="projectError" class="notice error" role="alert">{{ projectError }}</p>
      <p v-else class="notice">{{ indexBusy ? 'Initializing…' : 'Create a Git repository in an existing project folder.' }}</p>
    </section>
    <ConfirmationDialog v-if="dailyPanel === 'remotes' && store.repository" title="Git Remotes" :busy="indexBusy"
                        @cancel="dailyPanel = undefined">
      <RemotesPanel :key="store.repository.path" :path="store.repository.path" :name="store.repository.name"
                    :branch="store.repository.head" :has-head="hasLocalHead" :publish="false"
                    :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
      <template #actions>
        <button data-cancel class="primary" :disabled="indexBusy" @click="dailyPanel = undefined">Done</button>
      </template>
    </ConfirmationDialog>
    <RemotesPanel v-if="dailyPanel === 'publish' && store.repository" :key="store.repository.path"
                  :path="store.repository.path" :name="store.repository.name" :branch="store.repository.head"
                  :has-head="hasLocalHead" publish :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
    <SyncPanel v-if="dailyPanel === 'sync' && store.repository" :key="store.repository.path"
               :path="store.repository.path" :head="store.repository.head" :intent="syncIntent" :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
    <ConfirmationDialog v-if="pendingEditorAction" title="Unsaved file changes" @cancel="pendingEditorAction = undefined">
      <p>Discard unsaved changes in <strong>{{ editorPath }}</strong> before {{ pendingEditorLabel }}?</p>
      <template #actions>
        <button data-cancel @click="pendingEditorAction = undefined">Keep editing</button>
        <button class="danger-button" @click="discardEditorChanges">Discard changes</button>
      </template>
    </ConfirmationDialog>
    <ConfirmationDialog v-if="advanced" :title="'Confirm ' + advancedLabels[advanced.action]"
                        :busy="indexBusy" @cancel="advanced = undefined">
      <div class="daily-controls">
        <strong>{{ advancedLabels[advanced.action] }} {{ advanced.label }} ({{ advanced.hash.slice(0, 8) }})</strong>
        <span class="muted">Current {{ refLabel(advanced.head.ref) || 'detached HEAD' }} · {{ advanced.head.hash.slice(0, 8) }}</span>
        <label v-if="advanced.action === 'reset'">Mode
          <select v-model="resetMode" @change="hardResetConfirmed = false">
            <option value="soft">Soft — keep index and files</option>
            <option value="mixed">Mixed — keep working files</option>
            <option value="hard">Hard — discard tracked changes</option>
          </select>
        </label>
        <label v-if="advanced.action === 'reset' && resetMode === 'hard'">
          <input v-model="hardResetConfirmed" type="checkbox" /> Discard tracked changes
        </label>
        <span v-if="advanced.action === 'rebase'" class="notice">Rewrites the current branch's commits.</span>
      </div>
      <div v-if="advanced?.action === 'rebase'" class="rebase-plan" aria-label="Interactive rebase plan">
        <div v-for="(step, index) in rebasePlan" :key="step.commit.hash" class="rebase-step">
          <code>{{ step.commit.hash.slice(0, 8) }}</code>
          <span class="ellipsis" :title="step.commit.subject">{{ step.commit.subject }}</span>
          <select v-model="step.action" :aria-label="'Action for ' + step.commit.hash.slice(0, 8)">
            <option value="pick">Pick</option>
            <option value="squash">Squash</option>
            <option value="fixup">Fixup</option>
            <option value="drop">Drop</option>
          </select>
          <button :disabled="index === 0" :aria-label="'Move ' + step.commit.subject + ' earlier'" @click="moveRebaseStep(index, -1)">↑</button>
          <button :disabled="index === rebasePlan.length - 1" :aria-label="'Move ' + step.commit.subject + ' later'" @click="moveRebaseStep(index, 1)">↓</button>
        </div>
      </div>
      <template #actions>
        <button data-cancel :disabled="indexBusy" @click="advanced = undefined">Cancel</button>
        <button :class="advanced.action === 'reset' && resetMode === 'hard' ? 'danger-button' : 'primary'"
                :disabled="indexBusy || store.opening || (advanced.action === 'reset' && resetMode === 'hard' && !hardResetConfirmed)"
                @click="withSavedEditor('changing Git history', () => { void runAdvanced() })">Confirm {{ advancedLabels[advanced.action] }}</button>
      </template>
    </ConfirmationDialog>
    <div class="workspace">
      <main v-if="store.repository" ref="content" class="content" :class="{ 'preview-only': diffExpanded }"
            :style="contentStyle">
        <ActivityBar v-show="!diffExpanded" :active="activityActive" :history-visible="historyVisible" :busy="indexBusy"
                     @select="onActivitySelect" @settings="openSettings" />
        <aside v-show="!diffExpanded && !sidebarHidden" class="panel workspace-sidebar" aria-label="Repository workspace">
          <div class="tool-title">
            <strong>{{ sidebar === 'changes' ? 'Commit' : sidebar === 'files' ? 'Files' : 'Worktrees' }}</strong>
            <button class="tool-hide" :disabled="indexBusy" title="Hide tool window" aria-label="Hide tool window"
                    @click="sidebarHidden = true">
              <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
                <path d="m6 6 8 8m0-8-8 8" />
              </svg>
            </button>
          </div>
          <div class="sidebar-project">
            <span class="eyebrow">Repository</span>
            <strong class="ellipsis" :title="store.repository.path">{{ store.repository.name }}</strong>
          </div>
          <nav v-show="sidebar === 'changes'" class="commit-tabs" aria-label="Commit workspace">
            <button :aria-pressed="commitTab === 'commit'" :disabled="indexBusy" @click="commitTab = 'commit'">Commit</button>
            <button :aria-pressed="commitTab === 'shelf'" :disabled="indexBusy" @click="openShelf">Shelf</button>
          </nav>
          <RepositoryFiles v-show="sidebar === 'files'" :key="store.repository.path" :path="store.repository.path"
                           :active="sidebar === 'files' && !diffExpanded" :selected="editorPath" @open="openEditorFile" />
          <WorkingChanges ref="changes-panel" v-show="sidebar === 'changes' && commitTab === 'commit'" v-model:message="commitMessage" :files="workingFiles" :active="workingView ? workingSelection?.path : undefined"
                          :error="indexError || workingError" :loading="workingLoading"
                          :can-stage="!!workingView && currentWorkingFile?.canStage" :can-unstage="!!workingView && currentWorkingFile?.canUnstage"
                          :index-busy="indexBusy || store.opening || restoringSession" :notice="commitNotice" :amend-hash="amendTarget?.hash" :can-amend="!!store.commits.length && !store.opening"
                          @select="selectWorking" @edit="openEditorFile" @update-index="updateIndex" @include="updateIncludedFiles" @commit="commitChanges" @commit-and-push="commitChanges(true)"
                          @start-amend="startAmend" @cancel-amend="cancelAmend" @stage-all="stageAll"
                          :can-resolve="!!selectedConflict" @resolve="markResolved()" />
          <StashPanel v-if="sidebar === 'changes' && commitTab === 'shelf'" :key="store.repository.path"
                      class="shelf-panel" :path="store.repository.path" :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
          <WorktreePanel v-if="sidebar === 'worktrees'" :key="store.repository.path" :path="store.repository.path" v-model:busy="indexBusy" @open="openRepository" />
        </aside>
          <div v-show="!diffExpanded && !sidebarHidden" class="splitter vsplitter" role="separator" aria-orientation="vertical"
               aria-label="Resize Commit tool window" tabindex="0"
               @pointerdown="trackDrag" @pointermove="resizeSidebar" @keydown="resizeSidebarWithKey"></div>
          <div v-show="!historyMaximized" class="upper" :class="{ 'diff-expanded': diffExpanded }">
            <section class="panel preview-panel" aria-label="File Preview" @keydown.esc="diffExpanded && restoreDiffLayout()">
              <header class="preview-tabs">
                <div v-for="tab in editorTabs" v-show="!diffExpanded" :key="tab.path" class="editor-tab" role="tab"
                     :class="{ active: view === 'editor' && activeEditorPath === tab.path }"
                     :aria-selected="view === 'editor' && activeEditorPath === tab.path" tabindex="0"
                     :title="tab.path" @click="activateEditorTab(tab.path)"
                     @keydown.enter.prevent="activateEditorTab(tab.path)">
                  <span class="ellipsis">{{ fileName(tab.path) }}</span>
                  <span v-if="isTabDirty(tab)" class="editor-dirty" aria-label="Unsaved changes">•</span>
                  <button class="editor-close" :aria-label="'Close ' + fileName(tab.path)"
                          @click.stop="requestCloseEditorTab(tab.path)">×</button>
                </div>
                <button v-for="tab in workingTabs" v-show="!diffExpanded" :key="tab.id" :aria-pressed="view === tab.id" @click="selectView(tab.id)">{{ tab.title }}</button>
                <button v-show="!diffExpanded" :aria-pressed="view === 'commit'" @click="selectView('commit')">Commit diff</button>
                <span class="ellipsis muted" :title="previewPath">{{ previewPath }}</span>
                <button v-if="view === 'working' && workingSelection && preview?.kind === 'text'"
                        @click="openEditorFile(workingSelection.path)">Edit file</button>
                <button v-if="(view === 'staged' || view === 'unstaged') && currentWorkingFile"
                        :aria-pressed="showHunks" :disabled="indexBusy" @click="showHunks = !showHunks">Hunks</button>
                <button v-if="inspectionPath" @click="inspectFile('history')">File History</button>
                <button v-if="inspectionPath" @click="inspectFile('blame')">Blame</button>
                <button v-if="inspectionPath && operation?.conflicts?.includes(inspectionPath)"
                        @click="openConflict(inspectionPath)">Resolve conflict…</button>
                <button v-if="!diffExpanded" class="history-toggle" aria-controls="history-dock" :aria-pressed="historyVisible"
                        @click="historyVisible = !historyVisible">History</button>
                <button v-if="diffExpanded" ref="restore-diff" class="diff-size" @click="restoreDiffLayout">Restore layout</button>
                <button v-else-if="view === 'commit' && store.file" class="diff-size" @click="maximizeDiff">Maximize diff</button>
              </header>
              <p v-if="(view === 'editor' || view === 'conflict') && editorError" class="notice error" role="alert">{{ editorError }}</p>
              <p v-if="(view === 'editor' || view === 'conflict') && editorLoading" class="notice" role="status">Opening text file…</p>
              <ConflictPanel v-if="view === 'conflict'" :path="store.repository.path" :file="conflictFile"
                             :unmerged="!!operation?.conflicts?.includes(conflictFile)" :busy="indexBusy"
                             @resolve="markResolved(conflictFile)" @close="closeConflict" />
              <template v-for="tab in editorTabs" :key="tab.path">
                <WorkspaceEditor v-show="(view === 'editor' && activeEditorPath === tab.path || view === 'conflict' && tab.path === conflictFile) && !editorLoading"
                                 :path="tab.path" :initial-content="tab.file.content" :dark="dark"
                                 :line-ending="tab.file.lineEnding" :bom="tab.file.bom"
                                 :dirty="isTabDirty(tab)" :saving="editorSaving"
                                 :loading="editorLoading && activeEditorPath === tab.path" :blocked="indexBusy || store.opening"
                                 @change="tab.draft = $event" @save="saveEditor(tab.path)" @reload="reloadEditor"
                                 @close="requestCloseEditorTab(tab.path)" />
              </template>
              <p v-if="view === 'editor' && !editorTabs.length && !editorLoading" class="notice">No open editor tabs. Open a file from the Changes tree or the Files panel.</p>
              <FileInspection v-if="view === 'inspection' && inspection" :path="store.repository.path" :file="inspection.file"
                              :mode="inspection.mode" @mode="inspection = { ...inspection, mode: $event }"
                              @close="closeInspection" @commit="openHistoricalCommit" />
              <HunkPanel v-if="showHunks && currentWorkingFile && (view === 'staged' || view === 'unstaged')"
                         :path="store.repository.path" :file="currentWorkingFile.path" :staged="view === 'staged'"
                         :apply-selection="applyHunk" :busy="indexBusy || store.opening" @close="showHunks = false; workingView && selectView(workingView)" />
              <template v-else-if="workingView">
                <p v-if="previewLoading" class="notice" role="status">Loading file preview…</p>
                <p v-else-if="previewError" class="notice error" role="alert">{{ previewError }}</p>
                <pre v-else-if="preview?.kind === 'text' && view === 'working'" class="working-preview" tabindex="0"><code>{{ preview.content }}</code></pre>
                <p v-else-if="patchTooLarge" class="notice" role="status">
                  This diff is too large to render line by line. Open Working file to view the current contents.
                </p>
                <pre v-else-if="preview?.kind === 'text' && preview.content" class="working-preview patch" tabindex="0"><code><span v-for="(line, index) in patchLines" :key="index" :class="patchClass(line)">{{ line + '\n' }}</span></code></pre>
                <p v-else-if="preview?.kind === 'text'" class="notice">{{ view === 'staged' ? 'No staged diff for this file.' : 'No tracked unstaged diff for this file. Use Working file to read untracked files.' }}</p>
                <p v-else-if="preview?.kind === 'missing'" class="notice">This file is absent from the working tree.</p>
                <p v-else-if="preview?.kind === 'large'" class="notice">
                  Preview limit reached (512 KiB or 12,000 lines).
                  <button v-if="!expandedPreview" :disabled="previewLoading" @click="loadFullWorkingPreview">Load full preview (up to 8 MiB)</button>
                  <span v-else>This file exceeds the expanded preview limit (8 MiB or 120,000 lines).</span>
                </p>
                <div v-else-if="preview?.kind === 'image' && isDataImage(preview.content)" class="image-preview">
                  <img :src="preview.content" :alt="'Working copy of ' + (workingSelection?.path ?? 'file')" />
                </div>
                <p v-else-if="preview?.kind === 'binary' || preview?.kind === 'image'" class="notice">Binary or non-UTF-8 file.</p>
                <p v-else-if="preview?.kind === 'special'" class="notice">This path is not a regular file.</p>
                <p v-else class="notice">Select a working change to preview its current contents.</p>
              </template>
              <template v-else-if="view === 'commit'">
                <p v-if="store.loadingDiff" class="notice" role="status">Loading file comparison…</p>
                <p v-else-if="store.diffError" class="notice error" role="alert">{{ store.diffError }}</p>
                <p v-else-if="!store.comparison" class="notice">Select a file from the commit changes below.</p>
                <template v-else-if="store.comparison.kind === 'text' || store.comparison.kind === 'lfs'">
                  <div v-if="store.comparison.kind === 'lfs'" class="lfs-details notice">
                    <strong>Git LFS pointer</strong>
                    <div v-for="side in (['before', 'after'] as const)" :key="side">
                      <template v-if="lfsPointer(store.comparison[side])">
                        {{ side === 'before' ? 'Before' : 'After' }}
                        <code>{{ lfsPointer(store.comparison[side])?.oid }}</code>
                        · {{ lfsPointer(store.comparison[side])?.size }} bytes
                      </template>
                    </div>
                  </div>
                  <TextDiff :comparison="store.comparison" :path="store.file?.path ?? ''" :dark="dark" />
                </template>
                <div v-else-if="store.comparison.kind === 'image'" class="image-diff">
                  <figure v-for="side in imageSides" :key="side.label">
                    <figcaption>{{ side.label }}</figcaption>
                    <div class="image-frame">
                      <img v-if="isDataImage(side.src)" :src="side.src" :alt="side.label + ' version of ' + (store.file?.path ?? 'file')" />
                      <p v-else class="notice">{{ side.src ? 'Unsupported image format.' : 'No image on this side.' }}</p>
                    </div>
                  </figure>
                </div>
                <p v-else-if="store.comparison.kind === 'large'" class="notice">
                  Preview limit reached (512 KiB or 12,000 lines per side).
                  <button v-if="!expandedDiff" :disabled="store.loadingDiff" @click="loadFullDiff">Load full preview (up to 8 MiB)</button>
                  <span v-else>This file exceeds the expanded preview limit (8 MiB or 120,000 lines per side).</span>
                </p>
                <p v-else-if="store.comparison.kind === 'binary'" class="notice">Binary or non-UTF-8 content has no text preview.</p>
                <div v-else-if="store.comparison.kind === 'submodule'" class="notice submodule">
                  <strong>Submodule update</strong>
                  <button :disabled="indexBusy" @click="openSubmodule">Open submodule</button>
                  <p v-if="submoduleError" class="error" role="alert">{{ submoduleError }}</p>
                  <div>Before <code>{{ store.comparison.before || '—' }}</code></div>
                  <div>After <code>{{ store.comparison.after || '—' }}</code></div>
                </div>
              </template>
            </section>
          </div>
          <div v-show="historyVisible && !diffExpanded && !historyMaximized" class="splitter row-splitter" role="separator" aria-orientation="horizontal" aria-label="Resize preview and Git history"
               :aria-valuemin="28" :aria-valuemax="70" :aria-valuenow="previewHeight" tabindex="0"
               @pointerdown="split" @pointermove="resize" @keydown="resizeWithKey"></div>
          <div v-show="historyVisible && !diffExpanded" id="history-dock" class="lower panel">
            <header class="tool-window-header">
              <div class="tool-tabs" role="tablist" aria-label="Git tool window views">
                <button role="tab" :aria-selected="gitTab === 'log'" @click="gitTab = 'log'">Log</button>
                <button role="tab" :aria-selected="gitTab === 'console'" @click="gitTab = 'console'">Console</button>
              </div>
              <div class="tool-window-actions">
                <button :disabled="indexBusy" :title="historyMaximized ? 'Restore tool window size' : 'Maximize tool window'"
                        :aria-pressed="historyMaximized" @click="historyMaximized = !historyMaximized">
                  <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
                    <path v-if="!historyMaximized" d="M4 8V4h4M16 12v4h-4M4 8h4M16 12h-4" />
                    <path v-else d="M8 4v4H4m8 8v-4h4M8 4h.01M16 12h.01" />
                  </svg>
                </button>
                <button :disabled="indexBusy" title="Hide Git tool window" aria-label="Hide Git tool window"
                        @click="historyVisible = false; historyMaximized = false">
                  <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
                    <path d="m6 6 8 8m0-8-8 8" />
                  </svg>
                </button>
              </div>
            </header>
            <div v-show="gitTab === 'log'" class="lower-body">
            <aside class="panel branch-panel" aria-label="Branches and tags" :style="{ width: preferences.branchesWidth + 'px' }">
              <header class="panel-header">
                <h2>{{ refKind === 'branches' ? 'Branches' : 'Tags' }}</h2>
                <button v-if="refKind === 'branches'" :disabled="!store.selected || indexBusy" :aria-expanded="branchForm === 'create'"
                        @click="openBranchForm(branchForm === 'create' ? undefined : 'create')">New…</button>
              </header>
              <form v-if="branchForm === 'create' || branchForm === 'rename'" class="new-branch"
                    @submit.prevent="updateBranch(branchForm, branchName.trim())">
                <input v-model="branchName" aria-label="New branch name" placeholder="Branch name" spellcheck="false" />
                <small v-if="branchForm === 'create' && createBase" class="muted">From {{ createBase.slice(0, 8) }}</small>
                <small v-if="branchForm === 'rename'" class="muted ellipsis" :title="managedBranch">Rename {{ managedBranch }}</small>
                <button type="button" :disabled="indexBusy" @click="openBranchForm()">Cancel</button>
                <button type="submit" :disabled="!branchName.trim() || indexBusy || (branchForm === 'create' && !createBase)">
                  {{ branchForm === 'create' ? 'Create' : 'Rename' }}
                </button>
              </form>
              <ConfirmationDialog v-else-if="branchForm === 'delete'" title="Confirm branch deletion"
                                  :busy="indexBusy" @cancel="openBranchForm()">
                <p>Delete <strong>{{ managedBranch }}</strong> if it is merged into the current HEAD?</p>
                <p v-if="branchError" class="notice error" role="alert">{{ branchError }}</p>
                <template #actions>
                  <button data-cancel :disabled="indexBusy" @click="openBranchForm()">Cancel</button>
                  <button class="danger-button" :disabled="indexBusy" @click="updateBranch('delete', managedBranch)">Delete branch</button>
                </template>
              </ConfirmationDialog>
              <div v-else class="ref-filter">
                <select v-model="refKind" aria-label="Reference type" @change="branchFilter = ''; navigation = ''">
                  <option value="branches">Branches</option>
                  <option value="tags">Tags</option>
                </select>
                <input v-model="branchFilter" aria-label="Filter branches or tags" placeholder="Filter" />
              </div>
              <div class="branch-scroll" @scroll.passive="closeBranchMenu()">
                <p v-if="branchError" class="notice error" role="alert">{{ branchError }}</p>
                <p v-if="branchNotice" class="notice" role="status">{{ branchNotice }}</p>
                <p v-if="navigation" class="notice" role="status">{{ navigation }}</p>
                <p v-if="!visibleRefs.length" class="notice">{{ branchFilter ? 'No matching references' : 'No ' + refKind + ' yet' }}</p>
                <template v-if="refKind === 'branches'">
                  <div v-if="store.headHash && !branchFilter.trim()" class="branch-row">
                    <button class="side-item ellipsis" @click="jump(store.headHash)">HEAD (Current Branch)</button>
                  </div>
                  <div v-for="row in branchRows" :key="(row.file ? 'ref:' : 'folder:') + row.path" class="branch-row"
                       :style="{ paddingInlineStart: row.depth * 12 + 'px' }">
                    <button v-if="row.file" class="side-item ellipsis"
                            :class="{ active: row.file.item.name === 'refs/heads/' + store.repository.head }"
                            :title="row.file.item.name" :aria-expanded="branchMenu?.item.name === row.file.item.name"
                            @click="jump(row.file.item.hash)"
                            @contextmenu.prevent="openBranchActions(row.file.item, $event)"
                            @keydown.shift.f10.prevent="openBranchActions(row.file.item, $event)">
                      <span v-if="row.file.item.name === 'refs/heads/' + store.repository.head" class="current-ref-marker" aria-hidden="true">●</span>{{ row.name }}
                    </button>
                    <button v-else class="side-item ellipsis branch-folder" :disabled="!!branchFilter.trim()"
                            :aria-expanded="!!branchFilter.trim() || !collapsedBranches.has(row.path)"
                            @click="toggleBranchFolder(row.path)">
                      <span aria-hidden="true">{{ !branchFilter.trim() && collapsedBranches.has(row.path) ? '▸' : '▾' }}</span> {{ row.name }}
                    </button>
                  </div>
                </template>
                <div v-else v-for="item in visibleRefs" :key="item.name" class="branch-row">
                  <button class="side-item ellipsis" :title="item.name" @click="jump(item.hash)">{{ refLabel(item.name) }}</button>
                </div>
              </div>
            </aside>
            <div class="splitter vsplitter" role="separator" aria-orientation="vertical" aria-label="Resize Branches panel" tabindex="0"
                 @pointerdown="trackDrag" @pointermove="resizeBranches" @keydown="resizeBranchesWithKey"></div>
            <CommitLog ref="log" @refresh="refresh" :restoring="restoringSession" :pinned="pinnedCommit?.hash" :busy="indexBusy || store.opening || preparingOperation"
                       @select="handleHistorySelect" @create-branch="createBranchFromCommit" @pin="pinCommit" @compare="compareCommit"
                       @operation="(action, commit) => prepareOperation(action, commit.hash, commit.subject)" />
            <div class="splitter vsplitter" role="separator" aria-orientation="vertical" aria-label="Resize Changed Files panel" tabindex="0"
                 @pointerdown="trackDrag" @pointermove="resizeFiles" @keydown="resizeFilesWithKey"></div>
            <div class="log-right" :style="{ width: preferences.filesWidth + 'px' }">
              <ChangedFiles ref="filesPanel" @edit="openEditorFile" @select="handleHistorySelect" @open-diff="openHistoryDiff" @maximize="maximizeDiff" />
              <div class="splitter details-splitter" role="separator" aria-orientation="horizontal" aria-label="Resize Commit Details panel" tabindex="0"
                   @pointerdown="trackDrag" @pointermove="resizeDetails" @keydown="resizeDetailsWithKey"></div>
              <CommitDetails :style="{ height: preferences.detailsHeight + '%' }" />
            </div>
            </div>
            <GitConsole v-show="gitTab === 'console'" class="lower-console" :preparing="preparingOperation" :operation="operation"
                        :error="operationError" :notice="operationNotice" :busy="indexBusy || store.opening"
                        @continue="withSavedEditor('continuing the Git operation', () => { void finishOperation(false) })"
                        @abort="withSavedEditor('aborting the Git operation', () => { void finishOperation(true) })"
                        @dismiss-error="operationError = ''" @dismiss-notice="operationNotice = ''" @show-changes="openSidebar('changes')" />
          </div>
      </main>
      <main v-else class="empty">
        <div class="brand-icon empty-icon">
          <img :src="appIcon" alt="" />
        </div>
        <span class="eyebrow">LOCAL GIT WORKSPACE</span>
        <h1>Open a Git repository</h1>
        <p>Browse commits, inspect changed files, and compare versions side by side.</p>
        <div class="empty-actions">
          <button class="primary" :disabled="picking || store.opening || indexBusy" @click="choose">Browse for a repository</button>
          <button :disabled="store.opening || indexBusy" @click="toggleDailyPanel('init')">Initialize a project…</button>
        </div>
      </main>
    </div>
    <ContextActions v-if="branchMenu && store.repository" :anchor="branchMenu.anchor" label="Branch actions" @close="closeBranchMenu">
      <strong class="ellipsis">{{ refLabel(branchMenu.item.name) }}</strong>
      <button v-if="branchMenu.item.name.startsWith('refs/heads/') && branchMenu.item.name !== 'refs/heads/' + store.repository.head"
              :disabled="indexBusy" @click="branchAction('switch')">Checkout</button>
      <button @click="branchAction('history')">Show in Git Log</button>
      <button :disabled="indexBusy" @click="branchAction('create')">New Branch from Here…</button>
      <hr />
      <button :disabled="store.opening || !store.headHash" @click="branchAction('compare')">Compare with HEAD</button>
      <template v-if="branchMenu.item.hash !== store.headHash">
        <button :disabled="indexBusy || store.opening || preparingOperation" @click="branchAction('merge')">Merge into current branch…</button>
        <button :disabled="indexBusy || store.opening || preparingOperation" @click="branchAction('rebase')">Interactive rebase onto this…</button>
      </template>
      <template v-if="branchMenu.item.name.startsWith('refs/heads/')">
        <hr />
        <template v-if="branchMenu.item.name === 'refs/heads/' + store.repository.head">
          <button :disabled="indexBusy" @click="branchAction('pull')">Pull…</button>
          <button :disabled="indexBusy" @click="branchAction('push')">Push…</button>
        </template>
        <button :disabled="indexBusy" @click="branchAction('rename')">Rename…</button>
        <button :disabled="indexBusy || branchMenu.item.name === 'refs/heads/' + store.repository.head"
                @click="branchAction('delete')">Delete…</button>
      </template>
    </ContextActions>
    <CommandPalette v-if="paletteOpen" :commands="paletteCommands" @run="runPalette" @close="closePalette" />
    <footer class="statusbar">
      <span class="ellipsis" :title="store.repository?.path">{{ store.repository ? store.repository.path + ' · ' + (store.repository.head === 'HEAD' ? 'detached HEAD' : store.repository.head) : 'No repository open' }}</span>
      <span><i class="status-dot" :class="{ busy: store.opening || indexBusy }" aria-hidden="true"></i>{{ store.opening ? 'Opening…' : indexBusy ? 'Running Git…' : 'Ready' }}<template v-if="store.gitVersion"> · {{ store.gitVersion }}</template></span>
    </footer>
  </div>
</template>
