<script setup lang="ts">
import { computed, ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'

const busy = defineModel<boolean>('busy', { required: true })
const emit = defineEmits<{ close: []; cloned: [path: string] }>()
const address = ref('')
const parent = ref('')
const name = ref('')
const branch = ref('')
const picking = ref(false)
const error = ref('')
const target = computed(() => {
  const folder = parent.value.trim().replace(/[\\/]+$/, '')
  const leaf = name.value.trim()
  if (!folder || !leaf || leaf === '.' || leaf === '..' || /[\\/]/.test(leaf)) return ''
  return folder + (/^[A-Za-z]:|^\\\\/.test(folder) ? '\\' : '/') + leaf
})

async function browse() {
  if (busy.value || picking.value) return
  picking.value = true
  error.value = ''
  try {
    const folder = await api.ChooseDirectory()
    if (folder) parent.value = folder
  } catch (cause) {
    error.value = String(cause)
  } finally {
    picking.value = false
  }
}

async function clone() {
  if (busy.value || picking.value || !address.value.trim() || !target.value) return
  busy.value = true
  error.value = ''
  let repoPath: string | undefined
  try {
    const repo = await api.Clone(address.value.trim(), target.value, branch.value.trim())
    repoPath = repo.path
  } catch (cause) {
    error.value = String(cause)
  } finally {
    busy.value = false
  }
  if (repoPath) emit('cloned', repoPath)
}
</script>

<template>
  <ConfirmationDialog title="Clone repository" :busy="busy || picking" @cancel="emit('close')">
    <form id="clone-repository" class="repository-dialog-form" @submit.prevent="clone">
      <p>Copy a remote repository into a new local folder.</p>
      <label for="clone-address">Repository URL or path</label>
      <input id="clone-address" aria-label="Repository URL or path" v-model="address" placeholder="https://… or a local path" spellcheck="false"
             :disabled="busy || picking" />
      <label for="clone-parent">Clone parent folder</label>
      <div class="repository-dialog-path">
        <input id="clone-parent" aria-label="Clone parent folder" v-model="parent" placeholder="Absolute existing folder" spellcheck="false"
               :disabled="busy || picking" />
        <button type="button" :disabled="busy || picking" @click="browse">Browse…</button>
      </div>
      <label for="clone-name">Clone folder name</label>
      <input id="clone-name" aria-label="Clone folder name" v-model="name" placeholder="project-name" spellcheck="false" :disabled="busy || picking" />
      <label for="clone-branch">Clone branch <span class="muted">(optional)</span></label>
      <input id="clone-branch" v-model="branch" aria-label="Clone branch" placeholder="Default branch" spellcheck="false"
             :disabled="busy || picking" />
      <p v-if="target" class="muted">Destination <strong>{{ target }}</strong></p>
      <p v-else-if="name.trim() && parent.trim()" class="notice error">Enter a single folder name.</p>
      <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    </form>
    <template #actions>
      <button data-cancel :disabled="busy || picking" @click="emit('close')">Cancel</button>
      <button class="primary" type="submit" form="clone-repository"
              :disabled="busy || picking || !address.trim() || !target">{{ busy ? 'Cloning…' : 'Clone' }}</button>
    </template>
  </ConfirmationDialog>
</template>
