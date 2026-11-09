<script setup lang="ts">
import { ref } from 'vue'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import type { OperationState } from '../../../bindings/gitvela/internal/git'

/** Console tab of the bottom Git tool window: shows an in-progress history
 *  operation (rebase/merge/...), its unmerged files and the continue/abort
 *  controls, mirroring the old top "advanced panel". */
const props = defineProps<{ preparing: boolean; operation?: OperationState; error: string; notice: string; busy: boolean }>()
const emit = defineEmits<{ continue: []; abort: []; dismissError: []; dismissNotice: []; showChanges: [] }>()
const confirmAbort = ref(false)
</script>

<template>
  <section class="console-panel" aria-label="Git operation console">
    <p v-if="preparing" class="notice" role="status">Checking current HEAD…</p>
    <div v-if="operation?.kind || operation?.conflicts?.length" class="daily-controls">
      <strong>{{ operation.kind ? operation.kind + ' in progress' : 'Unmerged files' }}</strong>
      <span class="muted">{{ operation.conflicts?.length ?? 0 }} unmerged</span>
      <button :disabled="busy" @click="emit('showChanges')">Show Changes</button>
      <button v-if="operation.kind" :disabled="busy || !!operation.conflicts?.length" @click="emit('continue')">Continue</button>
      <button v-if="operation.kind && !confirmAbort" :disabled="busy" @click="confirmAbort = true">Abort…</button>
      <ConfirmationDialog v-if="confirmAbort" title="Confirm abort" :busy="busy" @cancel="confirmAbort = false">
        <p>Discard the in-progress operation and its resolutions?</p>
        <template #actions>
          <button data-cancel :disabled="busy" @click="confirmAbort = false">Keep operation</button>
          <button class="danger-button" :disabled="busy" @click="confirmAbort = false; emit('abort')">Confirm abort</button>
        </template>
      </ConfirmationDialog>
    </div>
    <p v-else-if="!preparing" class="notice">No Git operation in progress.</p>
    <p v-if="error" class="notice error" role="alert">{{ error }} <button @click="emit('dismissError')">Dismiss</button></p>
    <p v-if="notice" class="notice" role="status">{{ notice }} <button @click="emit('dismissNotice')">Dismiss</button></p>
  </section>
</template>
