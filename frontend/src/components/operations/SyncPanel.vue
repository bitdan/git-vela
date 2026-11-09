<script setup lang="ts">
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { BranchTracking, SyncState } from '../../../bindings/gitvela/internal/git'
import { LatestRequest, cancellableAll } from '../../stores/latestRequest'
import { usePreferences } from '../../stores/preferences'
import { matchesProtectedBranch } from './protectedBranches'
import { currentBranch, describeUpdate, parseRemoteRef } from '../../features/sync/branchActions'
import { pullModes, type PullMode } from '../../features/sync/pullOptions'

/** Fetch, Pull and Push dialogs. Each intent names its own target:
 *  - Fetch: a remote (or all); never touches HEAD or the working tree.
 *  - Pull: a remote branch chosen here, integrated into the checked-out branch
 *    (IDEA GitPullDialog); defaults to the upstream but is not limited to it.
 *  - Push: the explicit local `branch` (defaults to HEAD) and its own upstream
 *    (IDEA GitPushBranchAction). */
const props = defineProps<{
  path: string
  head: string
  intent: 'fetch' | 'pull' | 'push'
  /** Local branch to push; the checked-out branch when omitted. */
  branch?: string
  /** Known remote-tracking refs (refs/remotes/<remote>/<branch>). */
  remoteRefs: string[]
  refresh: () => Promise<void>
}>()
const busy = defineModel<boolean>('busy', { required: true })
const preferences = usePreferences()
const state = ref<SyncState>()
const tracking = ref<BranchTracking>()
const selectedRemote = ref('')
const pullBranch = ref('')
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

const pullTarget = computed(() => currentBranch(props.head))
const pushSource = computed(() => props.branch || pullTarget.value || '')
const remotes = computed(() => state.value?.remotes ?? [])
const upstream = computed<Destination | undefined>(() => {
  const info = tracking.value
  if (!info?.branch || !info.remoteBranch || !remotes.value.includes(info.remote)) return
  return { branch: info.branch, remote: info.remote, target: info.remoteBranch }
})

const destination = computed<Destination | undefined>(() => {
  if (tracking.value?.remote) return upstream.value
  if (!pushSource.value || !selectedRemote.value || (pushSource.value === props.head && !state.value?.hasHead)) return
  return { branch: pushSource.value, remote: selectedRemote.value, target: pushSource.value }
})

/** Remote branches of the selected remote, for the Pull source list. */
const remoteBranches = computed(() => props.remoteRefs
  .map(ref => parseRemoteRef(ref, remotes.value))
  .filter((entry): entry is { remote: string; branch: string } => !!entry && entry.remote === selectedRemote.value)
  .map(entry => entry.branch)
  .sort((a, b) => a.localeCompare(b)))

const confirmation = ref<Destination>()
const pullMode = ref<PullMode>('merge')
const pullNoCommit = ref(false)
const pullNoVerify = ref(false)
const pullAutostash = ref(false)
const fetchOptions = ref<{ prune: boolean; tags: '' | 'all' | 'none' }>({ prune: false, tags: '' })
const pushOptions = ref({ pushTags: false, setUpstream: false, forceWithLease: false })

/** Protected branch patterns are enforced again by the backend; the dialog
 *  only explains why the force option is unavailable. */
const targetProtected = computed(() => !!confirmation.value &&
  matchesProtectedBranch(preferences.protectedPatterns, confirmation.value.target))
watch(() => [props.intent, props.branch], () => {
  confirmation.value = undefined
  void load()
})
watch(selectedRemote, () => {
  confirmation.value = undefined
  choosePullBranch()
})
watch(remoteBranches, () => choosePullBranch())
watch(pullMode, mode => { if (mode === 'rebase') pullNoCommit.value = false })
watch(() => props.head, () => {
  confirmation.value = undefined
  if (!busy.value) void load()
})

/** IDEA preselects the tracked branch, else the same name as the current one. */
function choosePullBranch() {
  const choices = remoteBranches.value
  if (choices.includes(pullBranch.value)) return
  const tracked = tracking.value?.remote === selectedRemote.value ? tracking.value.remoteBranch : ''
  pullBranch.value = [tracked, pullTarget.value ?? ''].find(name => !!name && choices.includes(name)) ?? ''
}

async function load() {
  // Pull reads the checked-out branch's upstream, Push the selected branch's.
  const branch = props.intent === 'push' ? pushSource.value : pullTarget.value ?? ''
  const call = cancellableAll(
    api.SyncState(props.path),
    branch ? api.BranchTracking(props.path, branch) : Promise.resolve(undefined),
  )
  const current = requests.replace(call)
  loading.value = true
  loadError.value = ''
  try {
    const [result, branchTracking] = await call
    if (!current()) return
    state.value = result
    tracking.value = branchTracking ?? undefined
    const names = result.remotes ?? []
    if (!names.includes(selectedRemote.value)) {
      const tracked = branchTracking?.remote ?? ''
      selectedRemote.value = names.includes(tracked) ? tracked : names.includes('origin') ? 'origin' : names[0] ?? ''
    }
    choosePullBranch()
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

async function exclusive(label: string, action: () => Promise<string>) {
  if (busy.value || loading.value || loadError.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  running.value = label
  confirmation.value = undefined
  try {
    notice.value = await action()
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

function fetchRemote() {
  const remote = selectedRemote.value
  if (!remote) return
  void exclusive('Fetching…', async () => {
    await api.FetchRemote(props.path, remote, fetchOptions.value)
    return 'Fetched ' + remote + '.'
  })
}

function fetchAll() {
  if (!remotes.value.length) return
  void exclusive('Fetching all remotes…', async () => {
    await api.FetchAllRemotes(props.path, fetchOptions.value)
    return 'Fetched all remotes.'
  })
}

function pull() {
  const branch = pullTarget.value
  const remote = selectedRemote.value
  const source = pullBranch.value
  if (!branch || !remote || !source) return
  void exclusive('Pulling…', async () => {
    const result = await api.Pull(props.path, {
      branch, remote, remoteBranch: source, mode: pullMode.value,
      noCommit: pullNoCommit.value, noVerify: pullNoVerify.value, autostash: pullAutostash.value,
    })
    return 'Pull complete: ' + describeUpdate(result)
  })
}

function push() {
  const target = confirmation.value
  if (!target) return
  void exclusive('Pushing…', async () => {
    await api.PushBranch(props.path, target.branch, target.remote, target.target,
      { ...pushOptions.value, protected: preferences.protectedPatterns })
    pushOptions.value = { pushTags: false, setUpstream: false, forceWithLease: false }
    return 'Push complete to ' + target.remote + '/' + target.target + '.'
  })
}
</script>

<template>
  <section class="daily-panel" aria-label="Remote synchronization">
    <div class="daily-controls">
      <h2>{{ intent === 'fetch' ? 'Fetch' : intent === 'pull' ? 'Pull to ' + (pullTarget ?? 'HEAD') : 'Push ' + (pushSource || 'HEAD') }}</h2>
      <label v-if="intent !== 'push' || !upstream">Remote
        <select v-model="selectedRemote" aria-label="Remote" :disabled="busy || loading || !state?.remotes?.length">
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
      <button v-if="intent === 'fetch'" :disabled="busy || loading || !!loadError || !selectedRemote" @click="fetchRemote">Fetch</button>
      <button v-if="intent === 'fetch'" :disabled="busy || loading || !!loadError || !state?.remotes?.length"
              title="Fetch every configured remote with the same options" @click="fetchAll">Fetch all remotes</button>
      <label v-if="intent === 'pull'">Branch
        <select v-model="pullBranch" aria-label="Remote branch to pull" :disabled="busy || loading || !remoteBranches.length">
          <option v-if="!remoteBranches.length" value="">No fetched branches</option>
          <option v-for="name in remoteBranches" :key="name" :value="name">{{ selectedRemote }}/{{ name }}</option>
        </select>
      </label>
      <fieldset v-if="intent === 'pull'" class="pull-options" :disabled="busy || loading || !pullTarget">
        <legend>Pull options</legend>
        <label v-for="mode in pullModes" :key="mode.value" :title="mode.option">
          <input type="radio" :value="mode.value" v-model="pullMode" /> {{ mode.label }}</label>
        <label title="--no-commit"><input type="checkbox" v-model="pullNoCommit" :disabled="pullMode === 'rebase'" /> Do not commit the merge</label>
        <label title="--no-verify"><input type="checkbox" v-model="pullNoVerify" /> Bypass Git hooks</label>
        <label title="--autostash"><input type="checkbox" v-model="pullAutostash" /> Autostash local changes</label>
      </fieldset>
      <button v-if="intent === 'pull'" :disabled="busy || loading || !!loadError || !pullTarget || !pullBranch" @click="pull">Pull</button>
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
      <label class="push-option"><input v-model="pushOptions.forceWithLease" type="checkbox" :disabled="busy || targetProtected" />
        Force overwrite the remote branch (force-with-lease)</label>
      <p v-if="targetProtected" class="notice error" role="alert">
        {{ confirmation.remote }}/{{ confirmation.target }} matches a protected branch pattern, so it cannot be force pushed.
        Remove the protection in Settings → Version Control to override.
      </p>
      <p v-else-if="pushOptions.forceWithLease" class="notice error" role="alert">
        Force overwrites commits on {{ confirmation.remote }}/{{ confirmation.target }} that this clone has not fetched.
        Only use it after rewriting local history; the lease is taken against the last fetched state.
      </p>
      <template #actions>
        <button data-cancel :disabled="busy" @click="confirmation = undefined">Cancel</button>
        <button class="primary" :class="{ 'danger-button': pushOptions.forceWithLease }" :disabled="busy || loading || !!loadError"
                @click="push">{{ pushOptions.forceWithLease ? 'Force push (lease)' : 'Confirm push' }}</button>
      </template>
    </ConfirmationDialog>
    <p v-if="running || loading" class="notice" role="status">{{ running || 'Reading Git remotes…' }}</p>
    <p v-if="error || loadError" class="notice error" role="alert">{{ error || loadError }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-else-if="state && !running && !loading && !error" class="notice">
      <template v-if="!state.remotes?.length">Configure a Git remote to synchronize this repository.</template>
      <template v-else-if="intent === 'fetch'">Fetch updates remote-tracking branches only; HEAD and the working tree are unchanged.</template>
      <template v-else-if="intent === 'pull' && !pullTarget">Check out a local branch to pull into.</template>
      <template v-else-if="intent === 'pull'">Pull {{ selectedRemote }}/{{ pullBranch || '…' }} into {{ pullTarget }}<template v-if="upstream"> (upstream {{ upstream.remote }}/{{ upstream.target }})</template>.</template>
      <template v-else-if="!pushSource">Check out a local branch to push.</template>
      <template v-else-if="upstream">Upstream {{ upstream.remote }}/{{ upstream.target }} for {{ upstream.branch }}.</template>
      <template v-else-if="pushSource === head && !state.hasHead">Create the first commit before publishing this branch.</template>
      <template v-else-if="tracking?.remote">This branch requires a configured remote branch as its upstream.</template>
      <template v-else>Branch {{ pushSource }} has no upstream. Push can publish it to the selected remote.</template>
    </p>
  </section>
</template>
