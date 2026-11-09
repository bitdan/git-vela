<script setup lang="ts">
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { SyncState } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; head: string; intent: 'fetch' | 'pull' | 'push'; refresh: () => Promise<void> }>()
const busy = defineModel<boolean>('busy', { required: true })
const state = ref<SyncState>()
const selectedRemote = ref('')
const requests = new LatestRequest()
const loading = ref(false)
const loadError = ref('')
const error = ref('')
const notice = ref('')
const running = ref('')
let unsubscribe: (() => void) | undefined

interface Destination {
  branch: string
  remote: string
  target: string
}

const upstream = computed<Destination | undefined>(() => {
  const info = state.value
  if (!info?.branch || !info.upstreamBranch || !info.remotes?.includes(info.upstreamRemote)) return
  return { branch: info.branch, remote: info.upstreamRemote, target: info.upstreamBranch }
})

const destination = computed<Destination | undefined>(() => {
  if (state.value?.upstreamRemote) return upstream.value
  if (!state.value?.hasHead || !state.value.branch || !selectedRemote.value) return
  return { branch: state.value.branch, remote: selectedRemote.value, target: state.value.branch }
})

const confirmation = ref<Destination>()
const pullMode = ref<'ff' | 'merge' | 'rebase'>('ff')
const pullAutostash = ref(false)
const fetchOptions = ref<{ prune: boolean; tags: '' | 'all' | 'none' }>({ prune: false, tags: '' })
const pushOptions = ref({ pushTags: false, setUpstream: false, forceWithLease: false })
watch(() => props.intent, () => { confirmation.value = undefined })
watch(selectedRemote, () => { confirmation.value = undefined })
watch(() => props.head, () => {
  confirmation.value = undefined
  if (!busy.value) void load()
})

async function load() {
  const call = api.SyncState(props.path)
  const current = requests.replace(call)
  loading.value = true
  loadError.value = ''
  try {
    const result = await call
    if (!current()) return
    state.value = result
    const remotes = result.remotes ?? []
    if (!remotes.includes(selectedRemote.value)) {
      selectedRemote.value = remotes.includes(result.upstreamRemote)
        ? result.upstreamRemote
        : remotes.includes('origin') ? 'origin' : remotes[0] ?? ''
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

async function run(operation: 'fetch' | 'pull' | 'push') {
  const remote = selectedRemote.value
  const target = operation === 'push' ? confirmation.value : upstream.value
  if (busy.value || loading.value || loadError.value || (operation === 'fetch' ? !remote : !target)) return
  busy.value = true
  error.value = ''
  notice.value = ''
  running.value = operation === 'fetch' ? 'Fetching…' : operation === 'pull' ? 'Pulling…' : 'Pushing…'
  confirmation.value = undefined
  try {
    if (operation === 'fetch') {
      await api.FetchRemote(props.path, remote, fetchOptions.value)
      notice.value = 'Fetched ' + remote + '.'
    } else if (target && operation === 'pull') {
      await api.PullUpstream(props.path, target.branch, target.remote, target.target,
        { mode: pullMode.value, autostash: pullAutostash.value })
      notice.value = 'Pull complete from ' + target.remote + '/' + target.target + '.'
    } else if (target) {
      await api.PushBranch(props.path, target.branch, target.remote, target.target,
        { ...pushOptions.value })
      notice.value = 'Push complete to ' + target.remote + '/' + target.target + '.'
      pushOptions.value = { pushTags: false, setUpstream: false, forceWithLease: false }
    }
  } catch (cause) {
    error.value = String(cause)
  } finally {
    try {
      await Promise.all([load(), props.refresh()])
    } finally {
      running.value = ''
      busy.value = false
    }
  }
}
</script>

<template>
  <section class="daily-panel" aria-label="Remote synchronization">
    <div class="daily-controls">
      <h2>{{ { fetch: 'Fetch', pull: 'Pull', push: 'Push' }[intent] }}</h2>
      <label v-if="intent === 'fetch' || (intent === 'push' && !upstream)">Remote
        <select v-model="selectedRemote" :disabled="busy || loading || !state?.remotes?.length">
          <option v-if="!state?.remotes?.length" value="">No remotes</option>
          <option v-for="remote in state?.remotes" :key="remote">{{ remote }}</option>
        </select>
      </label>
      <fieldset v-if="intent === 'fetch'" class="pull-options" :disabled="busy || loading || !!loadError">
        <legend>Fetch options</legend>
        <label><input type="checkbox" v-model="fetchOptions.prune" /> Prune remote-tracking branches deleted on the remote</label>
        <label>Tags
          <select v-model="fetchOptions.tags">
            <option value="">Tags pointing to fetched commits</option>
            <option value="all">All tags</option>
            <option value="none">No tags</option>
          </select>
        </label>
      </fieldset>
      <button v-if="intent === 'fetch'" :disabled="busy || loading || !!loadError || !selectedRemote" @click="run('fetch')">Fetch</button>
      <fieldset v-if="intent === 'pull'" class="pull-options" :disabled="busy || loading || !upstream">
        <legend>Pull strategy</legend>
        <label><input type="radio" value="ff" v-model="pullMode" /> Fast-forward only</label>
        <label><input type="radio" value="merge" v-model="pullMode" /> Merge</label>
        <label><input type="radio" value="rebase" v-model="pullMode" /> Rebase</label>
        <label><input type="checkbox" v-model="pullAutostash" /> Autostash local changes</label>
      </fieldset>
      <button v-if="intent === 'pull'" :disabled="busy || loading || !!loadError || !upstream" @click="run('pull')">Pull</button>
      <button v-if="intent === 'push'" :disabled="busy || loading || !!loadError || !destination" :aria-expanded="!!confirmation"
              @click="confirmation = destination">Push…</button>
      <button :disabled="busy || loading" @click="load">Reload targets</button>
    </div>
    <ConfirmationDialog v-if="confirmation" title="Confirm push" :busy="busy" @cancel="confirmation = undefined">
      <span>Push <strong>{{ confirmation.branch }}</strong> to <strong>{{ confirmation.remote }}/{{ confirmation.target }}</strong>?
        <template v-if="!upstream">This sets the branch upstream.</template>
      </span>
      <label class="push-option"><input type="checkbox" v-model="pushOptions.pushTags" :disabled="busy" />
        Push annotated tags reachable from these commits</label>
      <label v-if="upstream" class="push-option"><input type="checkbox" v-model="pushOptions.setUpstream" :disabled="busy" />
        Re-point the upstream to {{ confirmation.remote }}/{{ confirmation.target }}</label>
      <label class="push-option"><input type="checkbox" v-model="pushOptions.forceWithLease" :disabled="busy" />
        Force overwrite the remote branch (force-with-lease)</label>
      <p v-if="pushOptions.forceWithLease" class="notice error" role="alert">
        Force overwrites commits on {{ confirmation.remote }}/{{ confirmation.target }} that this clone has not fetched.
        Only use it after rewriting local history; the lease is taken against the last fetched state.
      </p>
      <template #actions>
        <button data-cancel :disabled="busy" @click="confirmation = undefined">Cancel</button>
        <button class="primary" :class="{ 'danger-button': pushOptions.forceWithLease }" :disabled="busy || loading || !!loadError"
                @click="run('push')">{{ pushOptions.forceWithLease ? 'Force push (lease)' : 'Confirm push' }}</button>
      </template>
    </ConfirmationDialog>
    <p v-if="running || loading" class="notice" role="status">{{ running || 'Reading Git remotes…' }}</p>
    <p v-if="error || loadError" class="notice error" role="alert">{{ error || loadError }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-else-if="state && !running && !loading && !error" class="notice">
      <template v-if="upstream">Upstream {{ upstream.remote }}/{{ upstream.target }} for {{ upstream.branch }}.</template>
      <template v-else-if="!state.remotes?.length">Configure a Git remote to synchronize this repository.</template>
      <template v-else-if="!state.branch">Check out a local branch to pull or push.</template>
      <template v-else-if="!state.hasHead">Create the first commit before publishing this branch.</template>
      <template v-else-if="state.upstreamRemote">This branch requires a configured remote branch as its upstream.</template>
      <template v-else>Branch {{ state.branch }} has no upstream. Push can publish it to the selected remote.</template>
    </p>
  </section>
</template>
