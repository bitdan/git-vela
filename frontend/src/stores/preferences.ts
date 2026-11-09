import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

export const defaultPreviewHeight = 58
/** Layout bounds keep titles, buttons and essential content usable while a
 *  splitter is dragged; they also prevent negative or runaway sizes. */
export const layoutBounds = {
  sidebar: { min: 200, max: 520, default: 300 },
  branches: { min: 140, max: 420, default: 230 },
  files: { min: 200, max: 480, default: 320 },
  details: { min: 20, max: 70, default: 40 },
} as const

function clampLayout(value: number | null, bounds: { min: number; max: number; default: number }) {
  if (value === null || Number.isNaN(value)) return bounds.default
  return Math.min(bounds.max, Math.max(bounds.min, value))
}

export type Density = 'comfortable' | 'compact'
/** Row heights must mirror the CSS variables in styles.css so the virtual
 *  lists measure real layout: --row-height, --repo-row-height and
 *  --working-row-height respectively. */
const rowHeights = {
  comfortable: { row: 34, repo: 28, working: 26 },
  compact: { row: 27, repo: 22, working: 22 },
} as const

function preference(key: string) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function persist(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch (error) {
    console.warn('GitVela could not save preference', key, error)
  }
}

function clampEditorFont(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 13
  return Math.min(24, Math.max(10, Math.round(value)))
}

function clampTabSize(value: number) {
  return value === 2 || value === 8 ? value : 4
}

/** Workspace roots are stored as JSON; malformed or oversized values are
 *  dropped instead of breaking startup. */
function readRoots(): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem('gitvela.roots') ?? '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.filter((entry): entry is string => typeof entry === 'string' && entry.trim() !== '').slice(0, 20)
  } catch {
    return []
  }
}

export const usePreferences = defineStore('preferences', () => {
  const dark = ref(preference('gitvela.theme') === 'dark')
  const previewHeight = ref(Math.min(70, Math.max(28, Number(preference('gitvela.previewHeight')) || defaultPreviewHeight)))
  const fileTree = ref(preference('gitvela.fileTree') !== 'false')
  const density = ref<Density>(preference('gitvela.density') === 'compact' ? 'compact' : 'comfortable')
  const resumeOnStart = ref(preference('gitvela.resumeOnStart') !== 'false')
  const sidebarWidth = ref(clampLayout(Number(preference('gitvela.sidebarWidth')) || null, layoutBounds.sidebar))
  const branchesWidth = ref(clampLayout(Number(preference('gitvela.branchesWidth')) || null, layoutBounds.branches))
  const filesWidth = ref(clampLayout(Number(preference('gitvela.filesWidth')) || null, layoutBounds.files))
  const detailsHeight = ref(clampLayout(Number(preference('gitvela.detailsHeight')) || null, layoutBounds.details))
  /** Issue navigation (IDEA IssueNavigationConfiguration): commit message IDs
   *  matching the regex pattern become links built from the URL template. */
  const issuePattern = ref(preference('gitvela.issuePattern') ?? '[A-Z]+-\\d+')
  const issueURL = ref(preference('gitvela.issueURL') ?? '')
  /** Editor preferences mirror IDEA's Editor → General page. They are applied
   *  through compartments, so changing them never reloads the document. */
  const editorWordWrap = ref(preference('gitvela.editorWordWrap') === 'true')
  const editorFontSize = ref(clampEditorFont(Number(preference('gitvela.editorFontSize'))))
  const editorTabSize = ref(clampTabSize(Number(preference('gitvela.editorTabSize'))))
  /** Git executable path (Settings → Version Control → Git). Empty means the
   *  PATH lookup; the value is validated by the backend before it is applied. */
  const gitPath = ref(preference('gitvela.gitPath') ?? '')
  /** Protected branch patterns (IDEA Protected branches): exact names or
   *  shell wildcards, one per line. Force pushes to them are refused. */
  const protectedBranches = ref(preference('gitvela.protectedBranches') ?? '')
  const protectedPatterns = computed(() =>
    protectedBranches.value.split(/[\n,]/).map(value => value.trim()).filter(Boolean))
  /** Workspace roots (IDEA Directory Mappings): the repositories this
   *  workspace knows about. One root is active at a time; switching reopens it
   *  so every panel keeps working against a single repository. */
  const roots = ref<string[]>(readRoots())
  const activeRoot = ref(preference('gitvela.activeRoot') ?? '')
  const rowHeight = computed(() => rowHeights[density.value].row)
  const repoRowHeight = computed(() => rowHeights[density.value].repo)
  const workingRowHeight = computed(() => rowHeights[density.value].working)

  watch(dark, value => persist('gitvela.theme', value ? 'dark' : 'light'))
  watch(previewHeight, value => persist('gitvela.previewHeight', String(value)))
  watch(fileTree, value => persist('gitvela.fileTree', String(value)))
  watch(density, value => persist('gitvela.density', value))
  watch(resumeOnStart, value => persist('gitvela.resumeOnStart', String(value)))
  watch(sidebarWidth, value => persist('gitvela.sidebarWidth', String(value)))
  watch(branchesWidth, value => persist('gitvela.branchesWidth', String(value)))
  watch(filesWidth, value => persist('gitvela.filesWidth', String(value)))
  watch(detailsHeight, value => persist('gitvela.detailsHeight', String(value)))
  watch(issuePattern, value => persist('gitvela.issuePattern', value))
  watch(issueURL, value => persist('gitvela.issueURL', value))
  watch(editorWordWrap, value => persist('gitvela.editorWordWrap', String(value)))
  watch(editorFontSize, value => persist('gitvela.editorFontSize', String(clampEditorFont(value))))
  watch(editorTabSize, value => persist('gitvela.editorTabSize', String(value)))
  watch(gitPath, value => persist('gitvela.gitPath', value))
  watch(protectedBranches, value => persist('gitvela.protectedBranches', value))
  watch(roots, value => persist('gitvela.roots', JSON.stringify(value)), { deep: true })
  watch(activeRoot, value => persist('gitvela.activeRoot', value))

  /** Remember a repository as a workspace root, most recent first. */
  function rememberRoot(path: string) {
    if (!path) return
    roots.value = [path, ...roots.value.filter(root => root !== path)].slice(0, 20)
    activeRoot.value = path
  }

  function forgetRoot(path: string) {
    roots.value = roots.value.filter(root => root !== path)
    if (activeRoot.value === path) activeRoot.value = roots.value[0] ?? ''
  }

  function resetLayout() {
    previewHeight.value = defaultPreviewHeight
    sidebarWidth.value = layoutBounds.sidebar.default
    branchesWidth.value = layoutBounds.branches.default
    filesWidth.value = layoutBounds.files.default
    detailsHeight.value = layoutBounds.details.default
  }

  function toggleDensity() {
    density.value = density.value === 'compact' ? 'comfortable' : 'compact'
  }

  return { dark, previewHeight, fileTree, density, resumeOnStart, sidebarWidth, branchesWidth, filesWidth, detailsHeight, issuePattern, issueURL, editorWordWrap, editorFontSize, editorTabSize, gitPath, protectedBranches, protectedPatterns, roots, activeRoot, rememberRoot, forgetRoot, rowHeight, repoRowHeight, workingRowHeight, toggleDensity, resetLayout }
})
