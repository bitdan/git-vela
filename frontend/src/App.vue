<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onErrorCaptured, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../bindings/events'
import { storeToRefs } from 'pinia'
import { usePreferences } from './stores/preferences'
import { useWorkspace } from './stores/workspace'
import CommitLog from './components/git-log/CommitLog.vue'
import WorktreePanel from './components/worktrees/WorktreePanel.vue'
import ChangedFiles from './components/changed-files/ChangedFiles.vue'
import WorkingChanges from './components/changed-files/WorkingChanges.vue'
import StashPanel from './components/operations/StashPanel.vue'
import SyncPanel from './components/operations/SyncPanel.vue'
import RemotesPanel from './components/operations/RemotesPanel.vue'
import { RepositoryService as api } from '../bindings/gitvela/internal/app'
import type { AmendTarget, FilePreview, Ref as GitRef, WorkingFile } from '../bindings/gitvela/internal/git'
import { LatestRequest } from './stores/latestRequest'
import './styles.css'

const TextDiff = defineAsyncComponent(() => import('./components/diff/TextDiff.vue'))
const store = useWorkspace()
const log = useTemplateRef<InstanceType<typeof CommitLog>>('log')
const content = useTemplateRef<HTMLElement>('content')
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
const branchActions = useTemplateRef<HTMLElement>('branch-actions')
const branchMenu = ref<{ item: GitRef; trigger: HTMLElement; x: number; y: number }>()
const showWorktrees = ref(false)
const refKind = ref<'branches' | 'tags'>('branches')
const visibleRefs = computed(() => {
  const query = branchFilter.value.trim().toLowerCase()
  const refs = refKind.value === 'branches' ? store.branchRefs : store.tagRefs
  return refs.filter(item => refLabel(item.name).toLowerCase().includes(query))
})

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
const { dark, previewHeight, fileTree } = storeToRefs(usePreferences())
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
const view = ref<FileView | 'commit'>('working')
const diffExpanded = ref(false)
const restoreDiff = useTemplateRef<HTMLButtonElement>('restore-diff')
const commitMessage = ref('')
const amendTarget = ref<AmendTarget>()
let draftBeforeAmend = ''
type DailyPanel = 'stash' | 'sync' | 'init' | 'remotes' | 'publish'
type SyncIntent = 'fetch' | 'pull' | 'push'
const dailyPanel = ref<DailyPanel>()
const syncIntent = ref<SyncIntent>('fetch')
const projectPath = ref('')
const projectError = ref('')
const hasLocalHead = computed(() => store.branchRefs.some(item => item.name === 'refs/heads/' + store.repository?.head))
const commitNotice = ref('')
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

watch(view, value => { if (value !== 'commit') diffExpanded.value = false })
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
  branchForm.value = undefined
  managedBranch.value = ''
  branchBase.value = ''
  branchMenu.value = undefined
  refKind.value = 'branches'
  statusRequest.clear()
  previewRequest.clear()
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
  view.value = 'working'
  void loadWorking()
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
  if (document.hidden || indexBusy.value) return
  void store.checkForChanges()
  if (workingLoading.value) return
  const repo = store.repository?.path
  await loadWorking()
  if (repo === store.repository?.path && workingSelection.value && view.value !== 'commit' && !previewLoading.value) {
    void selectView(view.value, true)
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

function selectWorking(file: WorkingFile) {
  workingSelection.value = file
  indexError.value = ''
  void selectView('working')
}

async function selectView(next: FileView | 'commit', background = false) {
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
  const call = next === 'working' ? api.WorkingPreview(repo, file.path) : api.WorkingDiff(repo, file.path, next === 'staged')
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

async function updateIndex(staged: boolean) {
  const repo = store.repository?.path
  const file = currentWorkingFile.value
  if (!repo || !file || indexBusy.value) return
  indexBusy.value = true
  indexError.value = ''
  try {
    await api.SetStaged(repo, file.path, staged)
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
  if (repo === store.repository?.path && workingSelection.value && view.value !== 'commit') {
    await selectView(view.value, true)
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
      if (workingSelection.value && view.value !== 'commit') await selectView(view.value, true)
    } finally {
      indexBusy.value = false
    }
  }
}

async function commitChanges() {
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
  } catch (error) {
    if (repo === store.repository?.path) indexError.value = String(error)
  } finally {
    indexBusy.value = false
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', outsideMenus)
  void store.detectGit()
  unsubscribeChanges = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === store.repository?.path) void refreshExternalChanges()
  })
  timer = setInterval(() => { void refreshExternalChanges() }, 5000)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', outsideMenus)
  clearInterval(timer)
  unsubscribeChanges?.()
  watchRequest.clear()
  statusRequest.clear()
  previewRequest.clear()
})

async function choose() {
  if (picking.value) return
  picking.value = true
  try { await store.choose() } finally { picking.value = false }
}

function refresh() {
  if (store.repository) void store.open(store.repository.path, true)
  void loadWorking()
  if (workingSelection.value && view.value !== 'commit') void selectView(view.value)
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
  void nextTick(() => content.value?.querySelector<HTMLButtonElement>('.file-row.active')?.focus())
}

function createBranchFromCommit() {
  if (indexBusy.value) return
  showWorktrees.value = false
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

function focusCommit() {
  diffExpanded.value = false
  void nextTick(() => document.getElementById('commit-message')?.focus())
}

function closeBranchMenu(restoreFocus = false) {
  const trigger = branchMenu.value?.trigger
  branchMenu.value = undefined
  if (restoreFocus) trigger?.focus()
}

function openBranchActions(item: GitRef, event: MouseEvent | KeyboardEvent) {
  const trigger = event.currentTarget
  if (!(trigger instanceof HTMLElement)) return
  const rect = trigger.getBoundingClientRect()
  branchMenu.value = {
    item, trigger, x: Math.max(8, Math.min(rect.right, window.innerWidth - 254)),
    y: Math.max(8, Math.min(rect.top, window.innerHeight - 250)),
  }
  void nextTick(() => branchActions.value?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus())
}

function outsideMenus(event: PointerEvent) {
  if (!(event.target instanceof Node)) return
  if (!gitMenu.value?.contains(event.target)) closeGitMenu()
  if (!branchActions.value?.contains(event.target) && !branchMenu.value?.trigger.contains(event.target)) closeBranchMenu()
}

function branchAction(action: 'history' | 'create' | 'switch' | 'rename' | 'delete' | SyncIntent) {
  const item = branchMenu.value?.item
  if (!item) return
  closeBranchMenu()
  const name = refLabel(item.name)
  switch (action) {
    case 'history': void jump(item.hash); break
    case 'create': openBranchForm('create', name, item.hash); break
    case 'switch': void updateBranch('switch', name); break
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
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  previewHeight.value = Math.min(70, Math.max(28, previewHeight.value + (event.key === 'ArrowUp' ? -3 : 3)))
}
</script>

<template>
  <div class="shell" :data-theme="dark ? 'dark' : 'light'">
    <header class="topbar">
      <div class="brand"><span class="brand-icon" aria-hidden="true">◇</span><strong>GitVela</strong></div>
      <details ref="git-menu" class="git-menu" @keydown.esc.stop="closeGitMenu(); gitMenu?.querySelector('summary')?.focus()">
        <summary>Git</summary>
        <div class="command-menu" role="group" aria-label="Git operations">
          <button :disabled="indexBusy || store.opening" @click="gitAction(() => toggleDailyPanel('init'))">Initialize Project…</button>
          <template v-if="store.repository">
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(focusCommit)">Commit…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('push'))">Push…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('pull'))">Pull…</button>
            <button :disabled="indexBusy" @click="gitAction(() => openSync('fetch'))">Fetch…</button>
            <button :disabled="indexBusy" @click="gitAction(() => toggleDailyPanel('stash'))">Stash…</button>
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(() => { showWorktrees = false; refKind = 'branches'; diffExpanded = false })">Branches</button>
            <button :disabled="indexBusy || !store.selected" @click="gitAction(createBranchFromCommit)">New Branch…</button>
            <button :disabled="indexBusy" @click="gitAction(() => { showWorktrees = !showWorktrees; diffExpanded = false })">Worktrees</button>
            <button :disabled="indexBusy || store.opening" @click="gitAction(refresh)">Refresh</button>
            <div class="command-divider"></div>
            <button :disabled="indexBusy" @click="gitAction(() => toggleDailyPanel('remotes'))">Manage Remotes…</button>
            <button :disabled="indexBusy" @click="gitAction(() => toggleDailyPanel('publish'))">Publish to GitHub…</button>
          </template>
        </div>
      </details>
      <form class="repo-form" @submit.prevent="!indexBusy && store.open(path.trim())">
        <input v-model="path" aria-label="Repository path" placeholder="Enter a local repository path" spellcheck="false" />
        <button type="submit" :disabled="!path.trim() || store.opening || indexBusy">Open path</button>
      </form>
      <select v-if="store.recent.length" class="recent-repos" aria-label="Recent repositories" :disabled="indexBusy || store.opening"
              value="" @change="store.open(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''">
        <option value="" disabled>Recent</option>
        <option v-for="repo in store.recent" :key="repo" :value="repo">{{ repo.split(/[\\/]/).at(-1) || repo }}</option>
      </select>
      <button class="primary" :disabled="picking || store.opening || indexBusy" @click="choose">Browse…</button>
      <details class="preferences">
        <summary>Settings</summary>
        <div class="preferences-panel">
          <label><input v-model="dark" type="checkbox" /> Dark theme</label>
          <label><input v-model="fileTree" type="checkbox" /> Show Changed Files as a tree</label>
          <button @click="previewHeight = 46">Reset panel sizes</button>
        </div>
      </details>
    </header>
    <div v-if="workspaceError" class="notice error top-error" role="alert">
      {{ workspaceError }}
      <button @click="workspaceError = ''; refresh()">Reload workspace</button>
    </div>
    <p v-if="store.repoError" class="notice error top-error" role="alert">{{ store.repoError }}</p>
    <p v-if="watchError" class="notice top-error" role="status" :title="watchError">File notifications unavailable. Checking for Git changes every 5 seconds.</p>
    <section v-if="dailyPanel === 'init'" class="daily-panel" aria-label="Initialize project">
      <form class="daily-controls" @submit.prevent="initializeProject">
        <h2>Initialize project</h2>
        <label>Project folder <input v-model="projectPath" placeholder="Absolute folder path" :disabled="indexBusy" /></label>
        <button type="button" :disabled="indexBusy || picking" @click="pickProject">Browse…</button>
        <button type="submit" class="primary" :disabled="indexBusy || picking || !projectPath.trim()">Initialize Git</button>
        <button type="button" :disabled="indexBusy || picking" @click="dailyPanel = undefined">Cancel</button>
      </form>
      <p v-if="projectError" class="notice error" role="alert">{{ projectError }}</p>
      <p v-else class="notice">{{ indexBusy ? 'Initializing…' : 'Create a Git repository in an existing project folder.' }}</p>
    </section>
    <RemotesPanel v-if="(dailyPanel === 'remotes' || dailyPanel === 'publish') && store.repository"
                  :key="store.repository.path + ':' + dailyPanel" :path="store.repository.path" :name="store.repository.name"
                  :branch="store.repository.head" :has-head="hasLocalHead" :publish="dailyPanel === 'publish'"
                  :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
    <StashPanel v-if="dailyPanel === 'stash' && store.repository" :key="store.repository.path"
                :path="store.repository.path" :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
    <SyncPanel v-if="dailyPanel === 'sync' && store.repository" :key="store.repository.path"
               :path="store.repository.path" :head="store.repository.head" :intent="syncIntent" :refresh="refreshAfterDailyOperation" v-model:busy="indexBusy" />
    <div class="workspace">
      <main v-if="store.repository" ref="content" class="content">
        <div class="upper" :class="{ 'diff-expanded': diffExpanded }" :style="{ flexBasis: diffExpanded ? '100%' : previewHeight + '%' }">
          <WorkingChanges v-show="!diffExpanded" v-model:message="commitMessage" :files="workingFiles" :active="view !== 'commit' ? workingSelection?.path : undefined"
                          :error="indexError || workingError" :loading="workingLoading"
                          :can-stage="view !== 'commit' && currentWorkingFile?.canStage" :can-unstage="view !== 'commit' && currentWorkingFile?.canUnstage"
                          :index-busy="indexBusy" :notice="commitNotice" :amend-hash="amendTarget?.hash" :can-amend="!!store.commits.length && !store.opening"
                          @select="selectWorking" @update-index="updateIndex" @commit="commitChanges"
                          @start-amend="startAmend" @cancel-amend="cancelAmend" @stage-all="stageAll" />
          <section class="panel preview-panel" aria-label="File Preview" @keydown.esc="diffExpanded && restoreDiffLayout()">
            <header class="preview-tabs">
              <button v-for="tab in workingTabs" v-show="!diffExpanded" :key="tab.id" :aria-pressed="view === tab.id" @click="selectView(tab.id)">{{ tab.title }}</button>
              <button v-show="!diffExpanded" :aria-pressed="view === 'commit'" @click="selectView('commit')">Commit diff</button>
              <span class="ellipsis muted" :title="view !== 'commit' ? workingSelection?.path : store.file?.path">
                {{ view !== 'commit' ? workingSelection?.path : store.file?.path }}
              </span>
              <button v-if="diffExpanded" ref="restore-diff" class="diff-size" @click="restoreDiffLayout">Restore layout</button>
              <button v-else-if="view === 'commit' && store.file" class="diff-size" @click="maximizeDiff">Maximize diff</button>
            </header>
            <template v-if="view !== 'commit'">
              <p v-if="previewLoading" class="notice" role="status">Loading file preview…</p>
              <p v-else-if="previewError" class="notice error" role="alert">{{ previewError }}</p>
              <pre v-else-if="preview?.kind === 'text' && view === 'working'" class="working-preview" tabindex="0"><code>{{ preview.content }}</code></pre>
              <pre v-else-if="preview?.kind === 'text' && preview.content" class="working-preview patch" tabindex="0"><code><span v-for="(line, index) in patchLines" :key="index" :class="patchClass(line)">{{ line + '\n' }}</span></code></pre>
              <p v-else-if="preview?.kind === 'text'" class="notice">{{ view === 'staged' ? 'No staged diff for this file.' : 'No tracked unstaged diff for this file. Use Working file to read untracked files.' }}</p>
              <p v-else-if="preview?.kind === 'missing'" class="notice">This file is absent from the working tree.</p>
              <p v-else-if="preview?.kind === 'large'" class="notice">Preview limit reached (512 KiB or 12,000 lines).</p>
              <p v-else-if="preview?.kind === 'binary'" class="notice">Binary or non-UTF-8 file.</p>
              <p v-else-if="preview?.kind === 'special'" class="notice">This path is not a regular file.</p>
              <p v-else class="notice">Select a working change to preview its current contents.</p>
            </template>
            <template v-else>
              <p v-if="store.loadingDiff" class="notice" role="status">Loading file comparison…</p>
              <p v-else-if="store.diffError" class="notice error" role="alert">{{ store.diffError }}</p>
              <p v-else-if="!store.comparison" class="notice">Select a file from the commit changes below.</p>
              <template v-else-if="store.comparison.kind === 'text' || store.comparison.kind === 'lfs'">
                <p v-if="store.comparison.kind === 'lfs'" class="notice">Showing the Git LFS pointer.</p>
                <TextDiff :comparison="store.comparison" :path="store.file?.path ?? ''" :dark="dark" />
              </template>
              <p v-else-if="store.comparison.kind === 'large'" class="notice">Preview limit reached (512 KiB or 12,000 lines per side).</p>
              <p v-else-if="store.comparison.kind === 'binary'" class="notice">Binary or non-UTF-8 content has no text preview.</p>
              <div v-else-if="store.comparison.kind === 'submodule'" class="notice submodule">
                <strong>Submodule update</strong>
                <div>Before <code>{{ store.comparison.before || '—' }}</code></div>
                <div>After <code>{{ store.comparison.after || '—' }}</code></div>
              </div>
            </template>
          </section>
        </div>
        <div v-show="!diffExpanded" class="splitter" role="separator" aria-orientation="horizontal" aria-label="Resize preview and Git history"
             :aria-valuemin="28" :aria-valuemax="70" :aria-valuenow="previewHeight" tabindex="0"
             @pointerdown="split" @pointermove="resize" @keydown="resizeWithKey"></div>
        <div v-show="!diffExpanded" class="lower">
          <WorktreePanel v-if="showWorktrees" :key="store.repository.path" :path="store.repository.path" v-model:busy="indexBusy" />
          <aside v-else class="panel branch-panel" aria-label="Branches and tags">
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
            <div v-else-if="branchForm === 'delete'" class="new-branch" aria-label="Confirm branch deletion">
              <p class="branch-action-label">Delete <strong>{{ managedBranch }}</strong> if it is merged into the current HEAD?</p>
              <button :disabled="indexBusy" @click="openBranchForm()">Cancel</button>
              <button class="danger-button" :disabled="indexBusy" @click="updateBranch('delete', managedBranch)">Delete branch</button>
            </div>
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
              <div v-for="item in visibleRefs" :key="item.name" class="branch-row">
                <button class="side-item ellipsis"
                        :class="{ active: item.name === 'refs/heads/' + store.repository.head }"
                        :title="item.name" :aria-expanded="refKind === 'branches' ? branchMenu?.item.name === item.name : undefined"
                        @click="refKind === 'branches' ? openBranchActions(item, $event) : jump(item.hash)"
                        @contextmenu.prevent="refKind === 'branches' && openBranchActions(item, $event)"
                        @keydown.shift.f10.prevent="refKind === 'branches' && openBranchActions(item, $event)">{{ refLabel(item.name) }}</button>
              </div>
            </div>
          </aside>
          <CommitLog ref="log" @select="view = 'commit'" @create-branch="createBranchFromCommit" />
          <ChangedFiles @select="view = 'commit'" @maximize="maximizeDiff" />
        </div>
      </main>
      <main v-else class="empty">
        <div class="empty-icon" aria-hidden="true">◇</div>
        <span class="eyebrow">LOCAL GIT WORKSPACE</span>
        <h1>Open a Git repository</h1>
        <p>Browse commits, inspect changed files, and compare versions side by side.</p>
        <button class="primary" :disabled="picking || store.opening || indexBusy" @click="choose">Browse for a repository</button>
        <button :disabled="store.opening || indexBusy" @click="toggleDailyPanel('init')">Initialize a project…</button>
      </main>
    </div>
    <div v-if="branchMenu && store.repository" ref="branch-actions" class="command-menu branch-commands"
         :style="{ left: branchMenu.x + 'px', top: branchMenu.y + 'px' }" role="group" aria-label="Branch actions"
         @keydown.esc.stop="closeBranchMenu(true)">
      <strong class="ellipsis">{{ refLabel(branchMenu.item.name) }}</strong>
      <button @click="branchAction('history')">Show in Git Log</button>
      <button :disabled="indexBusy" @click="branchAction('create')">New Branch from Here…</button>
      <template v-if="branchMenu.item.name.startsWith('refs/heads/')">
        <template v-if="branchMenu.item.name === 'refs/heads/' + store.repository.head">
          <button :disabled="indexBusy" @click="branchAction('pull')">Pull…</button>
          <button :disabled="indexBusy" @click="branchAction('push')">Push…</button>
        </template>
        <button v-else :disabled="indexBusy" @click="branchAction('switch')">Checkout</button>
        <button :disabled="indexBusy" @click="branchAction('rename')">Rename…</button>
        <button :disabled="indexBusy || branchMenu.item.name === 'refs/heads/' + store.repository.head"
                @click="branchAction('delete')">Delete…</button>
      </template>
    </div>
    <footer class="statusbar">
      <span class="ellipsis" :title="store.repository?.path">{{ store.repository ? store.repository.path + ' · ' + (store.repository.head === 'HEAD' ? 'detached HEAD' : store.repository.head) : 'No repository open' }}</span>
      <span>{{ store.opening ? 'Opening…' : 'Ready' }}<template v-if="store.gitVersion"> · {{ store.gitVersion }}</template></span>
    </footer>
  </div>
</template>
