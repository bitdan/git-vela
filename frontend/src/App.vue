<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import { useWorkspace } from './stores/workspace'
import CommitLog from './components/git-log/CommitLog.vue'
import ChangedFiles from './components/changed-files/ChangedFiles.vue'
import WorkingChanges from './components/changed-files/WorkingChanges.vue'
import { RepositoryService as api } from '../bindings/gitvela/internal/app'
import type { FilePreview, WorkingFile } from '../bindings/gitvela/internal/git'
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
type BranchForm = 'create' | 'manage' | 'rename' | 'delete'
const branchForm = ref<BranchForm>()
const managedBranch = ref('')
const refKind = ref<'branches' | 'tags'>('branches')
const visibleRefs = computed(() => {
  const query = branchFilter.value.trim().toLowerCase()
  const refs = refKind.value === 'branches' ? store.branchRefs : store.tagRefs
  return refs.filter(item => refLabel(item.name).toLowerCase().includes(query))
})

function refLabel(ref: string) {
  return ref.replace(/^refs\/(heads|remotes|tags)\//, '')
}

function openBranchForm(form?: BranchForm, branch = '') {
  branchForm.value = form
  managedBranch.value = branch
  branchName.value = form === 'rename' ? branch : ''
  branchError.value = ''
  branchNotice.value = ''
}
const picking = ref(false)
const dark = ref(localStorage.getItem('gitvela.theme') === 'dark')
const previewHeight = ref(Math.min(70, Math.max(28, Number(localStorage.getItem('gitvela.previewHeight')) || 46)))
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
const commitMessage = ref('')
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
const previewRequest = new LatestRequest()
let timer: ReturnType<typeof setInterval>

watch(dark, value => localStorage.setItem('gitvela.theme', value ? 'dark' : 'light'))
watch(previewHeight, value => localStorage.setItem('gitvela.previewHeight', String(value)))
watch(() => store.repository?.path, repo => {
  if (repo) path.value = repo
  navigation.value = ''
  branchError.value = ''
  branchNotice.value = ''
  branchName.value = ''
  branchFilter.value = ''
  branchForm.value = undefined
  managedBranch.value = ''
  refKind.value = 'branches'
  statusRequest.clear()
  previewRequest.clear()
  workingFiles.value = []
  workingSelection.value = undefined
  commitMessage.value = ''
  commitNotice.value = ''
  indexError.value = ''
  preview.value = undefined
  previewError.value = ''
  previewLoading.value = false
  view.value = 'working'
  void loadWorking()
})


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

async function selectView(next: FileView | 'commit') {
  view.value = next
  if (next === 'commit') return
  const repo = store.repository?.path
  const file = workingSelection.value
  if (!repo || !file) return
  preview.value = undefined
  previewError.value = ''
  if (next !== 'working' && !currentWorkingFile.value) {
    previewRequest.clear()
    previewLoading.value = false
    preview.value = { kind: 'text', content: '' }
    return
  }
  previewLoading.value = true
  const call = next === 'working' ? api.WorkingPreview(repo, file.path) : api.WorkingDiff(repo, file.path, next === 'staged')
  const current = previewRequest.replace(call)
  try {
    const result = await call
    if (current()) preview.value = result
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

async function commitChanges() {
  const repo = store.repository?.path
  const message = commitMessage.value
  if (!repo || indexBusy.value || !message.trim() || !workingFiles.value.some(file => file.canUnstage)) return
  indexBusy.value = true
  indexError.value = ''
  commitNotice.value = ''
  try {
    await api.CommitIndex(repo, message)
    if (repo !== store.repository?.path) return
    commitMessage.value = ''
    commitNotice.value = 'Commit created'
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
  void store.detectGit()
  timer = setInterval(() => {
    if (!document.hidden && !indexBusy.value) {
      void store.checkForChanges()
      if (!workingLoading.value) void loadWorking()
    }
  }, 5000)
})
onUnmounted(() => {
  clearInterval(timer)
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
  const base = store.selected?.hash
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
      <form class="repo-form" @submit.prevent="store.open(path.trim())">
        <input v-model="path" aria-label="Repository path" placeholder="Enter a local repository path" spellcheck="false" />
        <button type="submit" :disabled="!path.trim() || store.opening">Open path</button>
      </form>
      <select v-if="store.recent.length" class="recent-repos" aria-label="Recent repositories"
              value="" @change="store.open(($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''">
        <option value="" disabled>Recent</option>
        <option v-for="repo in store.recent" :key="repo" :value="repo">{{ repo.split(/[\\/]/).at(-1) || repo }}</option>
      </select>
      <button class="primary" :disabled="picking || store.opening" @click="choose">Browse…</button>
      <button :disabled="!store.repository || store.opening" @click="refresh">Refresh</button>
      <button @click="dark = !dark">{{ dark ? 'Light' : 'Dark' }}</button>
    </header>
    <p v-if="store.repoError" class="notice error top-error" role="alert">{{ store.repoError }}</p>
    <div class="workspace">
      <main v-if="store.repository" ref="content" class="content">
        <div class="upper" :style="{ flexBasis: previewHeight + '%' }">
          <WorkingChanges v-model:message="commitMessage" :files="workingFiles" :active="view !== 'commit' ? workingSelection?.path : undefined"
                          :error="indexError || workingError" :loading="workingLoading"
                          :can-stage="view !== 'commit' && currentWorkingFile?.canStage" :can-unstage="view !== 'commit' && currentWorkingFile?.canUnstage"
                          :index-busy="indexBusy" :notice="commitNotice" @select="selectWorking" @update-index="updateIndex" @commit="commitChanges" />
          <section class="panel preview-panel" aria-label="File Preview">
            <header class="preview-tabs">
              <button v-for="tab in workingTabs" :key="tab.id" :aria-pressed="view === tab.id" @click="selectView(tab.id)">{{ tab.title }}</button>
              <button :aria-pressed="view === 'commit'" @click="selectView('commit')">Commit diff</button>
              <span class="ellipsis muted" :title="view !== 'commit' ? workingSelection?.path : store.file?.path">
                {{ view !== 'commit' ? workingSelection?.path : store.file?.path }}
              </span>
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
        <div class="splitter" role="separator" aria-orientation="horizontal" aria-label="Resize preview and Git history"
             :aria-valuemin="28" :aria-valuemax="70" :aria-valuenow="previewHeight" tabindex="0"
             @pointerdown="split" @pointermove="resize" @keydown="resizeWithKey"></div>
        <div class="lower">
          <aside class="panel branch-panel" aria-label="Branches and tags">
            <header class="panel-header">
              <h2>{{ refKind === 'branches' ? 'Branches' : 'Tags' }}</h2>
              <button v-if="refKind === 'branches'" :disabled="!store.selected || indexBusy" :aria-expanded="branchForm === 'create'"
                      @click="openBranchForm(branchForm === 'create' ? undefined : 'create')">New…</button>
            </header>
            <form v-if="branchForm === 'create' || branchForm === 'rename'" class="new-branch"
                  @submit.prevent="updateBranch(branchForm, branchName.trim())">
              <input v-model="branchName" aria-label="New branch name" placeholder="Branch name" spellcheck="false" />
              <small v-if="branchForm === 'create' && store.selected" class="muted">From {{ store.selected.hash.slice(0, 8) }}</small>
              <small v-if="branchForm === 'rename'" class="muted ellipsis" :title="managedBranch">Rename {{ managedBranch }}</small>
              <button type="button" :disabled="indexBusy" @click="openBranchForm()">Cancel</button>
              <button type="submit" :disabled="!branchName.trim() || indexBusy || (branchForm === 'create' && !store.selected)">
                {{ branchForm === 'create' ? 'Create' : 'Rename' }}
              </button>
            </form>
            <div v-else-if="branchForm === 'manage'" class="new-branch" aria-label="Branch actions">
              <strong class="branch-action-label ellipsis" :title="managedBranch">{{ managedBranch }}</strong>
              <button :disabled="indexBusy" @click="openBranchForm('rename', managedBranch)">Rename</button>
              <button :disabled="indexBusy || managedBranch === store.repository.head" @click="openBranchForm('delete', managedBranch)">Delete…</button>
              <button :disabled="indexBusy" @click="openBranchForm()">Cancel</button>
            </div>
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
            <div class="branch-scroll">
              <p v-if="branchError" class="notice error" role="alert">{{ branchError }}</p>
              <p v-if="branchNotice" class="notice" role="status">{{ branchNotice }}</p>
              <p v-if="navigation" class="notice" role="status">{{ navigation }}</p>
              <p v-if="!visibleRefs.length" class="notice">{{ branchFilter ? 'No matching references' : 'No ' + refKind + ' yet' }}</p>
              <div v-for="item in visibleRefs" :key="item.name" class="branch-row">
                <button class="side-item ellipsis"
                        :class="{ active: item.name === 'refs/heads/' + store.repository.head }"
                        :title="item.name" @click="jump(item.hash)">{{ refLabel(item.name) }}</button>
                <template v-if="item.name.startsWith('refs/heads/')">
                  <button v-if="item.name !== 'refs/heads/' + store.repository.head" class="switch-branch"
                          :title="'Switch to ' + refLabel(item.name)" :disabled="indexBusy"
                          @click="updateBranch('switch', refLabel(item.name))">Switch</button>
                  <button class="switch-branch" :aria-label="'Manage ' + refLabel(item.name)" :disabled="indexBusy"
                          @click="openBranchForm('manage', refLabel(item.name))">…</button>
                </template>
              </div>
            </div>
          </aside>
          <CommitLog ref="log" />
          <ChangedFiles @select="view = 'commit'" />
        </div>
      </main>
      <main v-else class="empty">
        <div class="empty-icon" aria-hidden="true">◇</div>
        <span class="eyebrow">LOCAL GIT WORKSPACE</span>
        <h1>Open a Git repository</h1>
        <p>Browse commits, inspect changed files, and compare versions side by side.</p>
        <button class="primary" :disabled="picking || store.opening" @click="choose">Browse for a repository</button>
      </main>
    </div>
    <footer class="statusbar">
      <span class="ellipsis" :title="store.repository?.path">{{ store.repository ? store.repository.path + ' · ' + (store.repository.head === 'HEAD' ? 'detached HEAD' : store.repository.head) : 'No repository open' }}</span>
      <span>{{ store.opening ? 'Opening…' : 'Ready' }}<template v-if="store.gitVersion"> · {{ store.gitVersion }}</template></span>
    </footer>
  </div>
</template>
