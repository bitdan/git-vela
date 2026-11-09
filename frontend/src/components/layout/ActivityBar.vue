<script setup lang="ts">
import AppIcon from './AppIcon.vue'

export type ActivityItem = 'files' | 'commit' | 'git'

defineProps<{ active: ActivityItem | undefined; historyVisible: boolean; busy?: boolean }>()
const emit = defineEmits<{ select: [item: ActivityItem] }>()
</script>

<template>
  <nav class="activity-bar" aria-label="Tool windows">
    <button aria-label="Files" title="Files" :aria-pressed="active === 'files'" :disabled="busy"
            @click="emit('select', 'files')">
      <AppIcon name="files" />
    </button>
    <button aria-label="Commit" title="Commit" :aria-pressed="active === 'commit'" :disabled="busy"
            @click="emit('select', 'commit')">
      <AppIcon name="commit" />
    </button>
    <span class="activity-spacer" aria-hidden="true"></span>
    <button aria-label="Git Log" title="Git Log" :aria-pressed="historyVisible" :disabled="busy"
            @click="emit('select', 'git')">
      <AppIcon name="branches" />
    </button>
  </nav>
</template>
