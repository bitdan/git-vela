<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, shallowRef, useTemplateRef } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { ProjectMatch } from '../../../bindings/gitvela/internal/git'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ repo: string; folder: string }>()
const emit = defineEmits<{ close: []; open: [path: string, line: number] }>()
const query = ref('')
const results = shallowRef<ProjectMatch[]>()
const loading = ref(false)
const error = ref('')
const input = useTemplateRef<HTMLInputElement>('query')
const requests = new LatestRequest()

onMounted(() => void nextTick(() => input.value?.focus()))
onUnmounted(() => requests.clear())

async function search() {
  const term = query.value.trim()
  if (!term) return
  results.value = undefined
  loading.value = true
  error.value = ''
  const call = api.SearchProject(props.repo, props.folder, term)
  const current = requests.replace(call)
  try {
    const matches = await call
    if (current()) results.value = matches ?? []
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}
</script>

<template>
  <ConfirmationDialog title="Find in Directory" class="project-search-dialog" @cancel="emit('close')">
    <p class="muted">{{ folder === '.' ? repo : folder }}</p>
    <form class="repository-dialog-path" @submit.prevent="search">
      <input ref="query" v-model="query" aria-label="Search text" placeholder="Find text in tracked and non-ignored files" />
      <button type="submit" :disabled="!query.trim()">Find</button>
    </form>
    <p v-if="loading" class="notice" role="status">Searching…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="results && !results.length" class="notice">No matches found.</p>
    <div v-if="results?.length" class="project-search-results">
      <button v-for="(match, index) in results" :key="index" class="project-match"
              @click="emit('open', match.path, match.line)">
        <strong>{{ match.path }}:{{ match.line }}</strong>
        <span class="ellipsis muted">{{ match.text }}</span>
      </button>
    </div>
    <template #actions><button data-cancel @click="emit('close')">Close</button></template>
  </ConfirmationDialog>
</template>
