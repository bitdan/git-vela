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

  return { dark, previewHeight, fileTree, density, resumeOnStart, sidebarWidth, branchesWidth, filesWidth, detailsHeight, rowHeight, repoRowHeight, workingRowHeight, toggleDensity, resetLayout }
})
