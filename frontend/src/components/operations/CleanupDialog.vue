<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'

const props = defineProps<{ path: string }>()
const busy = defineModel<boolean>('busy', { required: true })
const emit = defineEmits<{ close: [] }>()
const scanning = ref(true)
const unreachable = ref<number>()
const scanError = ref('')
const error = ref('')
const finished = ref(false)

onMounted(async () => {
  try {
    unreachable.value = await api.UnreachableCommits(props.path)
  } catch (cause) {
    scanError.value = String(cause)
  } finally {
    scanning.value = false
  }
})

async function cleanup() {
  if (busy.value || finished.value) return
  busy.value = true
  error.value = ''
  try {
    await api.RunGC(props.path)
    finished.value = true
  } catch (cause) {
    error.value = String(cause)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <ConfirmationDialog title="Git cleanup" :busy="busy" @cancel="emit('close')">
    <p><strong>{{ path }}</strong></p>
    <p>Run Git maintenance to compact the object database. Git may expire reflogs and permanently remove
      unreachable objects according to this repository’s retention settings.</p>
    <p v-if="scanning" class="notice" role="status">Checking for unreachable objects…</p>
    <p v-else-if="scanError" class="notice error" role="alert">Diagnostic scan failed. {{ scanError }}</p>
    <p v-else class="notice">
      {{ unreachable }} unreachable commit{{ unreachable === 1 ? '' : 's' }} found when ignoring reflogs.
      Git may retain these commits.
    </p>
    <p v-if="finished" class="notice" role="status">Git cleanup finished.</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <template #actions>
      <button data-cancel :disabled="busy" @click="emit('close')">{{ finished ? 'Close' : 'Cancel' }}</button>
      <button v-if="!finished" class="primary" :disabled="busy || scanning" @click="cleanup">
        {{ busy ? 'Cleaning…' : 'Run git gc' }}
      </button>
    </template>
  </ConfirmationDialog>
</template>
