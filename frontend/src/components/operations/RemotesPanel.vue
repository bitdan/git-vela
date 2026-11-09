<script setup lang="ts">
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Browser, Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { PublishResult, RemoteInfo } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; name: string; branch: string; hasHead: boolean; publish: boolean; refresh: () => Promise<void> }>()
const busy = defineModel<boolean>('busy', { required: true })
const remotes = ref<RemoteInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const error = ref('')
const notice = ref('')
const form = ref<'add' | 'edit' | 'remove'>()
const remoteName = ref('')
const address = ref('')
const originalAddress = ref('')
const fetchRemote = ref(true)
const account = ref('')
const repoName = ref(props.name.replace(/[^a-zA-Z0-9_.-]/g, '-'))
const privateRepo = ref(true)
const confirming = ref(false)
const result = ref<PublishResult>()
const requests = new LatestRequest()
const canPublish = computed(() => props.hasHead && !loading.value && !loadError.value &&
  !remotes.value.some(remote => remote.name === 'origin'))
let unsubscribe: (() => void) | undefined

watch([repoName, privateRepo, () => props.branch], () => { confirming.value = false })

async function load() {
  const call = api.Remotes(props.path)
  const current = requests.replace(call)
  loading.value = true
  loadError.value = ''
  try {
    const list = await call
    if (current()) remotes.value = list ?? []
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

function openForm(action?: typeof form.value, remote?: RemoteInfo) {
  form.value = action
  remoteName.value = remote?.name ?? (action === 'add' && !remotes.value.some(item => item.name === 'origin') ? 'origin' : '')
  originalAddress.value = action === 'edit' ? remote?.fetchUrl.split('\n')[0] ?? '' : ''
  address.value = originalAddress.value
  fetchRemote.value = true
  error.value = ''
  notice.value = ''
}

async function changeRemote() {
  const action = form.value
  const name = remoteName.value.trim()
  const url = address.value.trim()
  if (busy.value || !action || !name || (action !== 'remove' && !url)) return
  const shouldFetch = action !== 'remove' && fetchRemote.value
  // The list shows sanitized URLs. Preserve the stored URL when the field was left as shown.
  const urlChanged = action === 'edit' && url !== originalAddress.value
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    if (action === 'add') await api.AddRemote(props.path, name, url)
    else if (action === 'remove') await api.RemoveRemote(props.path, name)
    else if (urlChanged) await api.SetRemoteURL(props.path, name, url)

    openForm()
    notice.value = { add: 'Added ', edit: urlChanged ? 'Updated ' : 'Kept ', remove: 'Removed ' }[action] + name + '.'
    if (shouldFetch) {
      try {
        await api.FetchRemote(props.path, name, { prune: false, tags: '' })
        notice.value += ' Successfully fetched ' + name + '.'
      } catch (cause) {
        error.value = 'Fetch failed for ' + name + ': ' + String(cause)
      }
    }
    await Promise.all([load(), props.refresh()])
  } catch (cause) {
    error.value = String(cause)
    await load()
  } finally {
    busy.value = false
  }
}

async function checkAccount() {
  if (busy.value) return
  busy.value = true
  account.value = ''
  confirming.value = false
  error.value = ''
  try {
    account.value = await api.GitHubLogin(props.path)
  } catch (cause) {
    error.value = String(cause)
  } finally {
    busy.value = false
  }
}

async function publishProject() {
  if (busy.value || !confirming.value || !canPublish.value || !account.value || !repoName.value.trim()) return
  busy.value = true
  confirming.value = false
  result.value = undefined
  error.value = ''
  try {
    result.value = await api.PublishGitHub(props.path, {
      account: account.value, name: repoName.value.trim(), branch: props.branch, private: privateRepo.value,
    })
    await Promise.all([load(), props.refresh()])
  } catch (cause) {
    error.value = String(cause)
  } finally {
    busy.value = false
  }
}

async function openResult(url: string) {
  try {
    await Browser.OpenURL(url)
  } catch (cause) {
    error.value = String(cause)
  }
}
</script>

<template>
  <section class="daily-panel" :aria-label="publish ? 'Publish to GitHub' : 'Manage remotes'">
    <template v-if="publish">
      <div class="daily-controls">
        <h2>Publish to GitHub</h2>
        <button :disabled="busy" @click="checkAccount">{{ account ? 'Check account again' : 'Check GitHub account' }}</button>
        <span v-if="account">Signed in as <strong>{{ account }}</strong></span>
        <label>Repository name <input v-model="repoName" maxlength="100" :disabled="busy" /></label>
        <label>Visibility
          <select v-model="privateRepo" :disabled="busy">
            <option :value="true">Private</option>
            <option :value="false">Public</option>
          </select>
        </label>
        <button :disabled="busy || !canPublish || !account || !repoName.trim()" @click="confirming = true">Review publication</button>
      </div>
      <ConfirmationDialog v-if="confirming" title="Confirm GitHub publication" :busy="busy" @cancel="confirming = false">
        <p>Create <strong>{{ account }}/{{ repoName.trim() }}</strong> as {{ privateRepo ? 'private' : 'public' }} and push the committed history of <strong>{{ branch }}</strong>?</p>
        <template #actions>
          <button data-cancel :disabled="busy" @click="confirming = false">Cancel</button>
          <button class="primary" :disabled="busy || !canPublish" @click="publishProject">Create and push</button>
        </template>
      </ConfirmationDialog>
      <p v-if="result" class="notice" role="status">
        {{ result.pushed ? 'Published to GitHub.' : result.remoteAdded ? 'GitHub repository created and origin added.' : 'GitHub repository created.' }}
        <a :href="result.url" @click.prevent="openResult(result.url)">{{ result.url }}</a>
      </p>
      <p v-if="result?.failure" class="notice error" role="alert">
        {{ result.failure }}
        {{ result.remoteAdded ? 'Review origin in Manage remotes, then push from the Git menu.' : 'Add the new repository in Manage remotes, then push from the Git menu.' }}
      </p>
      <p v-else-if="!result && !hasHead" class="notice">Create the first commit on a local branch before publishing.</p>
      <p v-else-if="!result && !canPublish && !loading && !loadError" class="notice">This repository already has an origin. Use Manage remotes or Push.</p>
    </template>
    <template v-else>
      <div class="daily-controls">
        <button :disabled="busy" @click="openForm('add')">Add remote…</button>
        <button :disabled="busy || loading" @click="load">Reload remotes</button>
      </div>
      <ConfirmationDialog v-if="form === 'add' || form === 'edit'" class="remote-definition-dialog"
                          :title="form === 'add' ? 'Define Remote' : 'Edit Remote'" :busy="busy" @cancel="openForm()">
        <form id="remote-definition" class="remote-definition" @submit.prevent="changeRemote">
          <label>Name: <input v-model="remoteName" name="name" :disabled="busy" :readonly="form === 'edit'"
                             placeholder="origin" spellcheck="false" /></label>
          <label>URL: <input v-model="address" name="url" data-initial-focus :disabled="busy"
                            placeholder="https://github.com/owner/project.git" spellcheck="false" /></label>
          <label class="remote-fetch"><input v-model="fetchRemote" type="checkbox" :disabled="busy" /> Fetch remote</label>
          <p v-if="error" class="notice error" role="alert">{{ error }}</p>
        </form>
        <template #actions>
          <button class="primary" type="submit" form="remote-definition"
                  :disabled="busy || !remoteName.trim() || !address.trim()">OK</button>
          <button type="button" data-cancel :disabled="busy" @click="openForm()">Cancel</button>
        </template>
      </ConfirmationDialog>
      <ConfirmationDialog v-if="form === 'remove'" title="Confirm remote removal" :busy="busy" @cancel="openForm()">
        <p>Remove the local configuration and tracking refs for <strong>{{ remoteName }}</strong>?</p>
        <p v-if="error" class="notice error" role="alert">{{ error }}</p>
        <template #actions>
          <button data-cancel :disabled="busy" @click="openForm()">Cancel</button>
          <button class="danger-button" :disabled="busy" @click="changeRemote">Remove remote</button>
        </template>
      </ConfirmationDialog>
      <p v-if="!remotes.length && !loading && !loadError" class="notice">No configured remotes.</p>
      <div v-for="remote in remotes" :key="remote.name" class="remote-row">
        <strong>{{ remote.name }}</strong>
        <span class="ellipsis" :title="remote.fetchUrl">{{ remote.fetchUrl }}</span>
        <span v-if="remote.pushUrl && remote.pushUrl !== remote.fetchUrl" class="ellipsis" :title="remote.pushUrl">Push {{ remote.pushUrl }}</span>
        <button :disabled="busy" @click="openForm('edit', remote)">Edit URL…</button>
        <button :disabled="busy" @click="openForm('remove', remote)">Remove…</button>
      </div>
    </template>
    <p v-if="busy || loading" class="notice" role="status">{{ busy ? 'Running Git…' : 'Reading remotes…' }}</p>
    <p v-if="(!form && error) || loadError" class="notice error" role="alert">{{ error || loadError }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  </section>
</template>
