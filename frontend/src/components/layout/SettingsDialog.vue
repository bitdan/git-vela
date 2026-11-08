<script setup lang="ts">
import { onBeforeUnmount, onMounted, useTemplateRef } from 'vue'
import { usePreferences } from '../../stores/preferences'
import ShortcutSettings from './ShortcutSettings.vue'

/** The preferences are applied live as the user toggles them, mirroring the
 *  IDEA settings dialog where controls commit their value immediately. */
const emit = defineEmits<{
  close: []
  'forget-session': []
  'toggle-fullscreen': []
}>()
defineProps<{ sessionNotice?: string }>()

const preferences = usePreferences()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')

function close() {
  emit('close')
}

onMounted(() => {
  dialog.value?.showModal()
  dialog.value?.querySelector<HTMLElement>('[data-initial-focus]')?.focus()
})

onBeforeUnmount(() => dialog.value?.close())
</script>

<template>
  <dialog ref="dialog" class="settings-dialog" aria-label="Settings" @cancel.prevent="close" @keydown.stop>
    <header>
      <h2>Settings</h2>
      <button type="button" aria-label="Close settings" tabindex="-1" @click="close">×</button>
    </header>
    <div class="dialog-body settings-body">
      <section class="settings-group">
        <h3>Appearance</h3>
        <label><input v-model="preferences.dark" type="checkbox" /> Dark theme</label>
        <label><input type="checkbox" :checked="preferences.density === 'compact'" @change="preferences.toggleDensity()" /> Compact rows</label>
      </section>
      <section class="settings-group">
        <h3>Version Control</h3>
        <label><input v-model="preferences.fileTree" type="checkbox" /> Show Changed Files as a tree</label>
      </section>
      <section class="settings-group">
        <h3>Startup</h3>
        <label><input v-model="preferences.resumeOnStart" type="checkbox" /> Reopen last repository on startup</label>
      </section>
      <section class="settings-group">
        <h3>Window</h3>
        <button type="button" @click="emit('toggle-fullscreen')">Toggle fullscreen</button>
        <button type="button" @click="preferences.resetLayout()">Reset panel sizes</button>
      </section>
      <section class="settings-group">
        <h3>Session</h3>
        <button type="button" @click="emit('forget-session')">Forget saved session</button>
        <p v-if="sessionNotice" class="notice" role="status">{{ sessionNotice }}</p>
      </section>
      <section class="settings-group">
        <h3>Keyboard Shortcuts</h3>
        <ShortcutSettings />
      </section>
    </div>
    <footer class="dialog-actions">
      <button type="button" class="primary" data-initial-focus @click="close">Close</button>
    </footer>
  </dialog>
</template>
