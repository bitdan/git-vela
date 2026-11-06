import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import { RepositoryService as api } from '../../bindings/gitvela/internal/app'
import type { Change, Commit, Comparison, Cursor, Ref, Repository } from '../../bindings/gitvela/internal/git'
import { LatestRequest } from './latestRequest'

type ComparisonRange = { before: string; after: string; label: string }

const initialCursor: Cursor = { offset: 0, snapshot: '', lanes: null }

function readRecent(): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem('gitvela.recent') ?? '[]')
    return Array.isArray(value) ? value.filter((path): path is string => typeof path === 'string').slice(0, 8) : []
  } catch {
    return []
  }
}

export const useWorkspace = defineStore('workspace', () => {
  const repository = shallowRef<Repository>()
  const commits = shallowRef<Commit[]>([])
  const refs = shallowRef<Ref[]>([])
  const graphWidth = ref(1)
  const cursor = shallowRef<Cursor>(initialCursor)
  const more = ref(false)
  const selected = shallowRef<Commit>()
  const parent = ref(0)
  const files = shallowRef<Change[]>([])
  const file = shallowRef<Change>()
  const comparison = shallowRef<Comparison>()
  const comparisonRange = shallowRef<ComparisonRange>()
  const headHash = computed(() => refs.value.find(item => item.name === 'HEAD')?.hash)
  const recent = ref(readRecent())
  const gitVersion = ref('')
  const repoError = ref('')
  const logError = ref('')
  const filesError = ref('')
  const diffError = ref('')
  const opening = ref(false)
  const loading = ref(false)
  const loadingFiles = ref(false)
  const loadingDiff = ref(false)
  const branchRefs = computed(() => refs.value.filter(item => /^refs\/(heads|remotes)\//.test(item.name)))
  const tagRefs = computed(() => refs.value.filter(item => item.name.startsWith('refs/tags/')))
  const openRequest = new LatestRequest()
  const historyRequest = new LatestRequest()
  const filesRequest = new LatestRequest()
  const diffRequest = new LatestRequest()
  let checking = false

  async function detectGit() {
    try {
      gitVersion.value = await api.GitVersion()
    } catch (error) {
      repoError.value = String(error)
    }
  }

  async function open(path: string, preserveSelection = false) {
    repoError.value = ''
    opening.value = true
    const call = api.Open(path)
    const current = openRequest.replace(call)
    try {
      const repo = await call
      if (!current()) return
      const previous = preserveSelection && selected.value
        ? { hash: selected.value.hash, parent: parent.value, path: file.value?.path }
        : undefined
      const previousRange = preserveSelection && repo.path === repository.value?.path && comparisonRange.value
        ? { ...comparisonRange.value, path: file.value?.path }
        : undefined
      historyRequest.clear()
      filesRequest.clear()
      diffRequest.clear()
      repository.value = repo
      commits.value = []
      graphWidth.value = 1
      refs.value = []
      cursor.value = initialCursor
      more.value = false
      selected.value = undefined
      parent.value = 0
      files.value = []
      file.value = undefined
      comparison.value = undefined
      comparisonRange.value = undefined
      loadingFiles.value = false
      loadingDiff.value = false
      filesError.value = ''
      diffError.value = ''
      recent.value = [repo.path, ...recent.value.filter(item => item !== repo.path)].slice(0, 8)
      try { localStorage.setItem('gitvela.recent', JSON.stringify(recent.value)) } catch { /* storage unavailable */ }
      await loadPage(true, previous, previousRange)
    } catch (error) {
      if (current()) repoError.value = String(error)
    } finally {
      if (current()) opening.value = false
      openRequest.finish(call)
    }
  }

  async function choose(openSelected: (path: string) => void | Promise<void> = open) {
    try {
      const path = await api.ChooseDirectory()
      if (path) await openSelected(path)
    } catch (error) {
      repoError.value = String(error)
    }
  }

  async function loadPage(first = false, previous?: { hash: string; parent: number; path?: string }, previousRange?: ComparisonRange & { path?: string }) {
    if (!repository.value || (!first && (loading.value || !more.value))) return
    loading.value = true
    logError.value = ''
    const call = api.History(repository.value.path, cursor.value)
    const current = historyRequest.replace(call)
    try {
      const page = await call
      if (!current()) return
      const rows = page.commits ?? []
      commits.value = [...commits.value, ...rows]
      graphWidth.value = Math.max(graphWidth.value, ...rows.map(commit => commit.graph.width))
      refs.value = page.refs ?? []
      more.value = page.more
      cursor.value = page.next
      if (first && previousRange) {
        void selectComparison(previousRange.before, previousRange.after, previousRange.label, previousRange.path)
      } else if (first && rows.length) {
        const retained = previous && rows.find(row => row.hash === previous.hash)
        if (retained) void selectCommit(retained, previous.parent, previous.path)
        else void selectCommit(rows[0])
      }
    } catch (error) {
      if (current()) logError.value = String(error)
    } finally {
      if (current()) loading.value = false
      historyRequest.finish(call)
    }
  }

  async function selectCommit(commit: Commit, parentIndex = 0, previousPath = file.value?.path) {
    if (!repository.value) return
    selected.value = commit
    parent.value = parentIndex
    comparisonRange.value = undefined
    await loadFiles(api.Files(repository.value.path, commit.hash, parentIndex), previousPath)
  }

  async function selectComparison(before: string, after: string, label: string, previousPath?: string) {
    if (!repository.value) return
    comparisonRange.value = { before, after, label }
    await loadFiles(api.CompareFiles(repository.value.path, before, after), previousPath)
  }

  async function loadFiles(call: ReturnType<typeof api.Files>, previousPath?: string) {
    files.value = []
    file.value = undefined
    comparison.value = undefined
    filesError.value = ''
    diffError.value = ''
    diffRequest.clear()
    loadingDiff.value = false
    loadingFiles.value = true
    const current = filesRequest.replace(call)
    try {
      const list = await call
      if (!current()) return
      files.value = list ?? []
      const next = files.value.find(change => change.path === previousPath) ?? files.value[0]
      if (next) void selectFile(next)
    } catch (error) {
      if (current()) filesError.value = String(error)
    } finally {
      if (current()) loadingFiles.value = false
      filesRequest.finish(call)
    }
  }

  async function selectFile(change: Change) {
    if (!repository.value || (!selected.value && !comparisonRange.value)) return
    file.value = change
    comparison.value = undefined
    diffError.value = ''
    loadingDiff.value = true
    const range = comparisonRange.value
    const call = range
      ? api.CompareDiff(repository.value.path, range.before, range.after, change.index)
      : api.Diff(repository.value.path, selected.value!.hash, parent.value, change.index)
    const current = diffRequest.replace(call)
    try {
      const result = await call
      if (current()) comparison.value = result
    } catch (error) {
      if (current()) diffError.value = String(error)
    } finally {
      if (current()) loadingDiff.value = false
      diffRequest.finish(call)
    }
  }

  async function checkForChanges() {
    const repo = repository.value
    const snapshot = cursor.value.snapshot
    if (!repo || !snapshot || checking || opening.value || loading.value) return
    checking = true
    try {
      const current = await api.Snapshot(repo.path)
      if (repo === repository.value && snapshot === cursor.value.snapshot && !opening.value && current !== snapshot) {
        await open(repo.path, true)
      }
    } catch (error) {
      if (repo === repository.value && !opening.value) logError.value = String(error)
    } finally {
      checking = false
    }
  }

  return {
    repository, commits, branchRefs, tagRefs, graphWidth, more, selected, parent, files, file, comparison, comparisonRange, headHash,
    recent, gitVersion, repoError, logError, filesError, diffError, opening, loading, loadingFiles, loadingDiff,
    detectGit, open, choose, loadPage, selectCommit, selectComparison, selectFile, checkForChanges,
  }
})
