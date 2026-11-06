<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { StashEntry } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; refresh: () => Promise<void> }>()
const busy = defineModel<boolean>('busy', { required: true })
const entries = ref<StashEntry[]>([])
const selection = ref<StashEntry>()
const entry = computed(() => entries.value.find(item =>
  item.hash === selection.value?.hash && item.selector === selection.value?.selector))
const confirmDrop = ref(false)
const message = ref('')
const includeUntracked = ref(false)
const restoreIndex = ref(true)
const requests = new LatestRequest()
const loading = ref(false)
const loadError = ref('')
const error = ref('')
const notice = ref('')
let unsubscribe: (() => void) | undefined

async function load() {
  const call = api.Stashes(props.path)
  const current = requests.replace(call)
  loading.value = true
  loadError.value = ''
  try {
    const result = await call
    if (!current()) return
    entries.value = result ?? []
    if (!entry.value) {
      selection.value = undefined
      confirmDrop.value = false
    }
  } catch (cause) {
    if (current()) loadError.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}

onMounted(() => {
  void load()
  unsubscribe = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === props.path && !busy.value && !loading.value) void load()
  })
})
onUnmounted(() => {
  unsubscribe?.()
  requests.clear()
})

async function run(action: 'save' | 'apply' | 'pop' | 'drop') {
  const item = entry.value
  if (busy.value || loading.value || (action !== 'save' && (!item || loadError.value))) return
  busy.value = true
  error.value = ''
  notice.value = ''
  confirmDrop.value = false
  try {
    if (action === 'save') {
      await api.SaveStash(props.path, message.value.trim(), includeUntracked.value)
      message.value = ''
      notice.value = 'Stash saved.'
    } else if (item) {
      await api.UpdateStash(props.path, action, item.selector, item.hash, restoreIndex.value)
      notice.value = { apply: 'Stash applied and kept.', pop: 'Stash applied and removed.', drop: 'Stash deleted.' }[action]
    }
  } catch (cause) {
    error.value = String(cause)
  } finally {
    try {
      await Promise.all([load(), props.refresh()])
    } finally {
      busy.value = false
    }
  }
}
</script>

<template>
  <section class="daily-panel" aria-label="Stashes">
    <form class="daily-controls" @submit.prevent="run('save')">
      <h2>Stashes</h2>
      <input v-model="message" maxlength="256" aria-label="Stash message" placeholder="Optional stash message" :disabled="busy" />
      <label><input v-model="includeUntracked" type="checkbox" :disabled="busy" /> Include untracked files</label>
      <button type="submit" :disabled="busy || loading">Save stash</button>
    </form>
    <div class="daily-controls">
      <label>Saved stash
        <select v-model="selection" :disabled="busy || loading || !entries.length" @change="confirmDrop = false">
          <option :value="undefined" disabled>{{ entries.length ? 'Select a stash' : 'No saved stashes' }}</option>
          <option v-for="item in entries" :key="item.selector" :value="item">{{ item.selector }} · {{ item.subject }}</option>
        </select>
      </label>
      <label><input v-model="restoreIndex" type="checkbox" :disabled="busy" /> Restore staged changes</label>
      <button :disabled="busy || loading || !!loadError || !entry" @click="run('apply')">Apply</button>
      <button :disabled="busy || loading || !!loadError || !entry" @click="run('pop')">Pop</button>
      <button :disabled="busy || loading || !!loadError || !entry" :aria-expanded="confirmDrop" @click="confirmDrop = true">Drop…</button>
      <button :disabled="busy || loading" @click="load">Reload list</button>
    </div>
    <p v-if="confirmDrop && entry" class="daily-confirm" role="group" aria-label="Confirm stash deletion">
      <span>Delete <strong>{{ entry.selector }} · {{ entry.subject }}</strong> from the stash list?</span>
      <button @click="confirmDrop = false">Cancel</button>
      <button class="danger-button" :disabled="busy || loading || !!loadError" @click="run('drop')">Confirm drop</button>
    </p>
    <p v-if="busy || loading" class="notice" role="status">{{ busy ? 'Running stash operation…' : 'Reading stashes…' }}</p>
    <p v-if="error || loadError" class="notice error" role="alert">{{ error || loadError }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  </section>
</template>
