<script setup lang="ts">
import { defineAsyncComponent, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import { useWorkspace } from './stores/workspace'
import CommitLog from './components/git-log/CommitLog.vue'
import ChangedFiles from './components/changed-files/ChangedFiles.vue'
import './styles.css'

const TextDiff = defineAsyncComponent(() => import('./components/diff/TextDiff.vue'))
const store = useWorkspace()
const log = useTemplateRef<InstanceType<typeof CommitLog>>('log')
const content = useTemplateRef<HTMLElement>('content')
const path = ref('')
const navigation = ref('')
const picking = ref(false)
const dark = ref(localStorage.getItem('gitvela.theme') !== 'light')
const logHeight = ref(Math.min(70, Math.max(28, Number(localStorage.getItem('gitvela.logHeight')) || 48)))
let timer: ReturnType<typeof setInterval>

watch(dark, value => localStorage.setItem('gitvela.theme', value ? 'dark' : 'light'))
watch(logHeight, value => localStorage.setItem('gitvela.logHeight', String(value)))
watch(() => store.repository, repo => { if (repo) path.value = repo.path; navigation.value = '' })

onMounted(() => {
  void store.detectGit()
  timer = setInterval(() => {
    if (!document.hidden) void store.checkForChanges()
  }, 5000)
})
onUnmounted(() => clearInterval(timer))

async function choose() {
  if (picking.value) return
  picking.value = true
  try { await store.choose() } finally { picking.value = false }
}

function refresh() {
  if (store.repository) void store.open(store.repository.path, true)
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
  logHeight.value = Math.min(70, Math.max(28, Math.round((event.clientY - bounds.top) / bounds.height * 100)))
}

function resizeWithKey(event: KeyboardEvent) {
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  logHeight.value = Math.min(70, Math.max(28, logHeight.value + (event.key === 'ArrowUp' ? -3 : 3)))
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
      <button class="primary" :disabled="picking || store.opening" @click="choose">Browse…</button>
      <button :disabled="!store.repository || store.opening" @click="refresh">Refresh</button>
      <button @click="dark = !dark">{{ dark ? 'Light' : 'Dark' }}</button>
    </header>
    <p v-if="store.repoError" class="notice error top-error" role="alert">{{ store.repoError }}</p>
    <div class="workspace">
      <aside class="sidebar" aria-label="Repositories and branches">
        <span class="eyebrow">WORKSPACE</span>
        <h2>Repositories</h2>
        <p v-if="!store.recent.length" class="muted">No recent repositories</p>
        <button v-for="repo in store.recent" :key="repo" class="side-item ellipsis"
                :class="{ active: repo === store.repository?.path }" :title="repo" @click="store.open(repo)">
          {{ repo.split(/[\\/]/).at(-1) || repo }}
        </button>
        <template v-if="store.repository">
          <div class="side-divider"></div>
          <span class="eyebrow">REFERENCES</span>
          <h2>Branches</h2>
          <p v-if="navigation" class="muted" role="status">{{ navigation }}</p>
          <p v-if="!store.branchRefs.length" class="muted">No branch refs yet</p>
          <button v-for="branch in store.branchRefs" :key="branch.name" class="side-item ellipsis"
                  :title="branch.name" @click="jump(branch.hash)">
            {{ branch.name.replace(/^refs\/(heads|remotes)\//, '') }}
          </button>
        </template>
      </aside>

      <main v-if="store.repository" ref="content" class="content">
        <div class="log-region" :style="{ flexBasis: logHeight + '%' }"><CommitLog ref="log" /></div>
        <div class="splitter" role="separator" aria-orientation="horizontal" aria-label="Resize Git Log"
             :aria-valuemin="28" :aria-valuemax="70" :aria-valuenow="logHeight" tabindex="0"
             @pointerdown="split" @pointermove="resize" @keydown="resizeWithKey"></div>
        <div class="lower">
          <ChangedFiles />
          <section class="panel diff-panel" aria-label="Diff Viewer">
            <header class="panel-header">
              <div class="ellipsis"><span class="eyebrow">FILE COMPARISON</span><h2 class="ellipsis" :title="store.file?.path">{{ store.file?.path ?? 'Diff Viewer' }}</h2></div>
            </header>
            <p v-if="store.loadingDiff" class="notice" role="status">Loading file comparison…</p>
            <p v-else-if="store.diffError" class="notice error" role="alert">{{ store.diffError }}</p>
            <p v-else-if="!store.comparison" class="notice">Select a changed file to see its comparison.</p>
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
          </section>
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
      <span>{{ store.opening ? 'Opening…' : 'Read only' }}<template v-if="store.gitVersion"> · {{ store.gitVersion }}</template></span>
    </footer>
  </div>
</template>
