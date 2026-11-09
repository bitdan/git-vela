<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Events, Window } from '@wailsio/runtime'

defineProps<{ busy: boolean }>()
const emit = defineEmits<{ close: []; error: [message: string] }>()
const maximised = ref(false)
const fullscreen = ref(false)
const listeners: (() => void)[] = []

async function sync() {
  try {
    const [max, full] = await Promise.all([Window.IsMaximised(), Window.IsFullscreen()])
    maximised.value = max
    fullscreen.value = full
  } catch (error) {
    emit('error', String(error))
  }
}

onMounted(() => {
  void sync()
  const events = Events.Types.Common
  for (const name of [events.WindowMaximise, events.WindowUnMaximise, events.WindowFullscreen, events.WindowUnFullscreen]) {
    listeners.push(Events.On(name, () => { void sync() }))
  }
})
onUnmounted(() => listeners.forEach(off => off()))

async function minimise() {
  try {
    await Window.Minimise()
  } catch (error) {
    emit('error', 'Could not minimise the window: ' + String(error))
  }
}

async function toggleMaximise() {
  try {
    if (fullscreen.value) await Window.UnFullscreen()
    else await Window.ToggleMaximise()
    await sync()
  } catch (error) {
    emit('error', 'Could not resize the window: ' + String(error))
  }
}

defineExpose({ toggleMaximise })
</script>

<template>
  <div class="window-controls" role="group" aria-label="Window controls">
    <button type="button" aria-label="Minimise window" title="Minimise" @click="minimise">
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8h10" /></svg>
    </button>
    <button type="button" :aria-label="fullscreen || maximised ? 'Restore window' : 'Maximise window'"
            :title="fullscreen || maximised ? 'Restore' : 'Maximise'" @click="toggleMaximise">
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path v-if="fullscreen || maximised" d="M5.5 4V2.5h8v8H12M2.5 5.5h8v8h-8z" />
        <path v-else d="M3.5 3.5h9v9h-9z" />
      </svg>
    </button>
    <button type="button" class="window-close" aria-label="Close window" title="Close"
            :disabled="busy" @click="emit('close')">
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8m0-8-8 8" /></svg>
    </button>
  </div>
</template>
