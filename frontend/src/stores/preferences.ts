import { ref, watch } from 'vue'
import { defineStore } from 'pinia'

export const defaultPreviewHeight = 60

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
  watch(dark, value => persist('gitvela.theme', value ? 'dark' : 'light'))
  watch(previewHeight, value => persist('gitvela.previewHeight', String(value)))
  watch(fileTree, value => persist('gitvela.fileTree', String(value)))
  return { dark, previewHeight, fileTree }
})
