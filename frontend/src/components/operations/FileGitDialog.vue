<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { FilePreview, Ref as GitRef } from '../../../bindings/gitvela/internal/git'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import { LatestRequest } from '../../stores/latestRequest'

export type FileGitDialogAction = 'commit' | 'rollback' | 'revision' | 'ref' | 'current' | 'diff'

const props = defineProps<{ repo: string; file: string; action: FileGitDialogAction; refs: GitRef[] }>()
const emit = defineEmits<{ close: []; applied: [message: string] }>()
const busy = defineModel<boolean>('busy', { required: true })
const title = computed(() => ({
  commit: 'Commit File', rollback: 'Rollback File', revision: 'Compare with Revision',
  ref: 'Compare with Branch or Tag', current: 'Show Current Revision', diff: 'Show Diff',
})[props.action])
const mutation = computed(() => props.action === 'commit' || props.action === 'rollback')
const revision = ref(props.action === 'ref' ? '' : 'HEAD')
const message = ref('')
const loading = ref(false)
const token = ref('')
const preview = ref<FilePreview>()
const error = ref('')
const requests = new LatestRequest()

async function readRevision() {
  if (!revision.value.trim() || loading.value) return
  loading.value = true
  preview.value = undefined
  error.value = ''
  const call = api.ReadFileRevision(props.repo, props.file, revision.value.trim(), props.action !== 'current')
  const current = requests.replace(call)
  try {
    const result = await call
    if (current()) preview.value = result
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}

onMounted(async () => {
  if (props.action === 'current' || props.action === 'diff') {
    await readRevision()
    return
  }
  if (!mutation.value) return
  loading.value = true
  const call = api.PrepareFileMutation(props.repo, props.file, props.action)
  const current = requests.replace(call)
  try {
    const result = await call
    if (current()) token.value = result
  } catch (cause) {
    if (current()) error.value = String(cause)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
})
onUnmounted(() => requests.clear())

async function apply() {
  if (busy.value || !token.value || (props.action === 'commit' && !message.value.trim())) return
  busy.value = true
  error.value = ''
  try {
    await api.ApplyFileMutation(props.repo, props.file, props.action, token.value, message.value.trim())
    emit('applied', message.value.trim())
  } catch (cause) {
    token.value = ''
    error.value = String(cause)
    busy.value = false
  }
}

function refLabel(ref: GitRef) {
  return ref.name.replace(/^refs\/(heads|remotes|tags)\//, '')
}
</script>

<template>
  <ConfirmationDialog :title="title" :busy="busy" class="file-git-dialog" @cancel="emit('close')">
    <p><strong>{{ file }}</strong><br><span class="muted">{{ repo }}</span></p>
    <template v-if="mutation">
      <p v-if="action === 'rollback'">Restore this file to HEAD in both the working tree and the index.
        Its staged and unstaged changes will be discarded.</p>
      <template v-else>
        <p>Commit the working copy of this file. Other staged files will remain staged.</p>
        <label class="repository-dialog-form">Commit message
          <textarea v-model="message" aria-label="File commit message" rows="4" :disabled="busy" />
        </label>
      </template>
    </template>
    <form v-else-if="action === 'revision' || action === 'ref'" class="repository-dialog-path" @submit.prevent="readRevision">
      <select v-if="action === 'ref'" v-model="revision" aria-label="Branch or tag" :disabled="loading">
        <option value="" disabled>Select a branch or tag</option>
        <option v-for="ref in refs" :key="ref.name" :value="ref.name">{{ refLabel(ref) }}</option>
      </select>
      <input v-else v-model="revision" aria-label="Git revision" placeholder="HEAD~1, tag, or commit hash" :disabled="loading" />
      <button type="submit" :disabled="loading || !revision.trim()">Compare</button>
    </form>
    <p v-if="loading" class="notice" role="status">{{ mutation ? 'Checking the file and HEAD…' : 'Reading revision…' }}</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <template v-if="preview">
      <pre v-if="preview.kind === 'text' && preview.content" class="file-revision-content" tabindex="0">{{ preview.content }}</pre>
      <p v-else-if="preview.kind === 'large'" class="notice">This preview exceeds the file size or line limit.</p>
      <p v-else-if="preview.kind === 'binary'" class="notice">This revision contains binary or non-UTF-8 content.</p>
      <p v-else class="notice">{{ action === 'current' ? 'This revision is empty.' : 'No differences from the working copy.' }}</p>
    </template>
    <template #actions>
      <button data-cancel :disabled="busy" @click="emit('close')">{{ mutation ? 'Cancel' : 'Close' }}</button>
      <button v-if="mutation" :class="action === 'rollback' ? 'danger-button' : 'primary'"
              :disabled="busy || !token || (action === 'commit' && !message.trim())" @click="apply">
        {{ action === 'rollback' ? 'Rollback File' : 'Commit File' }}
      </button>
    </template>
  </ConfirmationDialog>
</template>
