<script setup lang="ts">
import { onBeforeUnmount, onMounted, useTemplateRef } from 'vue'

const props = defineProps<{ title: string; busy?: boolean }>()
const emit = defineEmits<{ cancel: [] }>()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')

function cancel() {
  if (!props.busy) emit('cancel')
}

onMounted(() => {
  dialog.value?.showModal()
  const initial = dialog.value?.querySelector<HTMLElement>('[data-initial-focus]')
    ?? dialog.value?.querySelector<HTMLElement>('[data-cancel]')
  initial?.focus()
})

onBeforeUnmount(() => dialog.value?.close())
</script>

<template>
  <dialog ref="dialog" class="confirmation-dialog" :aria-label="title" @cancel.prevent="cancel" @keydown.stop>
    <header>
      <h2>{{ title }}</h2>
      <button type="button" aria-label="Close dialog" tabindex="-1" :disabled="busy" @click="cancel">×</button>
    </header>
    <div class="dialog-body"><slot /></div>
    <footer v-if="$slots.actions" class="dialog-actions"><slot name="actions" /></footer>
  </dialog>
</template>
