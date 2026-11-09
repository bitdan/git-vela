<script setup lang="ts">
import ConfirmationDialog from '../../components/layout/ConfirmationDialog.vue'
import type { UpdateMethod } from './branchActions'

/** IDEA "Update Project" options dialog: choose merge or rebase for the
 *  checked-out branch. Local changes are preserved with an autostash. */
defineProps<{ branch: string; busy: boolean }>()
const method = defineModel<UpdateMethod>('method', { required: true })
const emit = defineEmits<{ cancel: []; confirm: [] }>()
</script>

<template>
  <ConfirmationDialog title="Update Project" :busy="busy" @cancel="emit('cancel')">
    <p>Fetch the upstream of <strong>{{ branch }}</strong> and integrate it into the checked-out branch.</p>
    <fieldset class="pull-options" :disabled="busy">
      <legend>Update type</legend>
      <label><input v-model="method" type="radio" value="merge" /> Merge the incoming changes into the current branch</label>
      <label><input v-model="method" type="radio" value="rebase" /> Rebase the current branch on top of the incoming changes</label>
    </fieldset>
    <p class="muted">Uncommitted changes are stashed and restored automatically. A fast-forward is used when possible.</p>
    <template #actions>
      <button data-cancel :disabled="busy" @click="emit('cancel')">Cancel</button>
      <button class="primary" :disabled="busy" @click="emit('confirm')">OK</button>
    </template>
  </ConfirmationDialog>
</template>
