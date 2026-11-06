import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

export const defaultPreviewHeight = 60
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
  const rowHeight = computed(() => rowHeights[density.value].row)
  const repoRowHeight = computed(() => rowHeights[density.value].repo)
  const workingRowHeight = computed(() => rowHeights[density.value].working)

  watch(dark, value => persist('gitvela.theme', value ? 'dark' : 'light'))
  watch(previewHeight, value => persist('gitvela.previewHeight', String(value)))
  watch(fileTree, value => persist('gitvela.fileTree', String(value)))
  watch(density, value => persist('gitvela.density', value))
  watch(resumeOnStart, value => persist('gitvela.resumeOnStart', String(value)))

  function toggleDensity() {
    density.value = density.value === 'compact' ? 'comfortable' : 'compact'
  }

  return { dark, previewHeight, fileTree, density, resumeOnStart, rowHeight, repoRowHeight, workingRowHeight, toggleDensity }
})
