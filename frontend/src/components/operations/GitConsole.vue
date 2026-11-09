<script setup lang="ts">
import { ref } from 'vue'
import ConfirmationDialog from '../layout/ConfirmationDialog.vue'
import type { OperationState } from '../../../bindings/gitvela/internal/git'

/** Console tab of the bottom Git tool window: shows an in-progress history
 *  operation (rebase/merge/...), its unmerged files and the continue/abort
 *  controls, mirroring the old top "advanced panel". */
const props = defineProps<{ path: string; preparing: boolean; operation?: OperationState; error: string; notice: string; busy: boolean }>()
const emit = defineEmits<{ finish: [abort: boolean, revision: string]; dismissError: []; dismissNotice: []; showChanges: [] }>()
const pending = ref<{ abort: boolean; kind: string; revision: string }>()

function prepare(abort: boolean) {
  const operation = props.operation
  if (props.busy || !operation?.kind || !operation.revision) return
  pending.value = { abort, kind: operation.kind, revision: operation.revision }
}

function confirm() {
  const action = pending.value
  if (!action || props.busy || action.revision !== props.operation?.revision) return
  pending.value = undefined
  emit('finish', action.abort, action.revision)
}
</script>

<template>
  <section class="console-panel" aria-label="Git operation console">
    <p v-if="preparing" class="notice" role="status">Checking current HEAD…</p>
    <div v-if="operation?.kind || operation?.conflicts?.length" class="daily-controls">
      <strong>{{ operation.kind ? operation.kind + ' in progress' : 'Unmerged files' }}</strong>
      <span class="muted">{{ operation.conflicts?.length ?? 0 }} unmerged</span>
      <button :disabled="busy" @click="emit('showChanges')">Show Changes</button>
      <button v-if="operation.kind" :disabled="busy || !!operation.conflicts?.length" @click="prepare(false)">Continue</button>
      <button v-if="operation.kind" :disabled="busy" @click="prepare(true)">Abort…</button>
    </div>
    <p v-else-if="!preparing" class="notice">No Git operation in progress.</p>
    <ConfirmationDialog v-if="pending" :title="pending.abort ? 'Confirm abort' : 'Confirm continue'"
                        :busy="busy" @cancel="pending = undefined">
      <p>{{ pending.abort ? 'Abort' : 'Continue' }} the in-progress <strong>{{ pending.kind }}</strong>
        in <strong>{{ path }}</strong>?</p>
      <p v-if="pending.abort">Git will try to restore the state before the operation and discard its conflict resolutions.</p>
      <p v-else>Git will use the staged resolutions and proceed with the remaining steps.</p>
      <p v-if="pending.revision !== operation?.revision" class="notice error">
        The Git operation changed. Close this dialog and review the current operation.
      </p>
      <template #actions>
        <button data-cancel :disabled="busy" @click="pending = undefined">Cancel</button>
        <button :class="pending.abort ? 'danger-button' : 'primary'"
                :disabled="busy || pending.revision !== operation?.revision" @click="confirm">
          {{ pending.abort ? 'Confirm abort' : 'Confirm continue' }}
        </button>
      </template>
    </ConfirmationDialog>
    <p v-if="error" class="notice error" role="alert">{{ error }} <button @click="emit('dismissError')">Dismiss</button></p>
    <p v-if="notice" class="notice" role="status">{{ notice }} <button @click="emit('dismissNotice')">Dismiss</button></p>
  </section>
</template>
