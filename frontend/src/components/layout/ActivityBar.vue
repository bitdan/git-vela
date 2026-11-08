<script setup lang="ts">
import { useTemplateRef } from 'vue'

/** IDEA-style vertical tool window bar. Icons are original GitVela SVGs, not
 *  IntelliJ assets. Each entry toggles or focuses a tool window and keeps its
 *  semantic name for tooltips and accessibility. */
export type ActivityItem = 'commit' | 'files' | 'worktrees' | 'git'

const props = defineProps<{ active: ActivityItem | undefined; historyVisible: boolean; busy?: boolean }>()
const emit = defineEmits<{ select: [item: ActivityItem]; settings: [] }>()
const settingsButton = useTemplateRef<HTMLButtonElement>('settings-button')

const icons: Record<ActivityItem, string> = {
  commit: 'M5 3h10a1.5 1.5 0 0 1 1.5 1.5v11A1.5 1.5 0 0 1 15 17H5a1.5 1.5 0 0 1-1.5-1.5v-11A1.5 1.5 0 0 1 5 3Zm2 3.5h6M7 10h6M7 13.5h4',
  files: 'M4.5 4.5h6l2 2h4.5a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1h-12.5a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1Zm0 4.5h13.5',
  worktrees: 'M10 3v4m0 0-4.5 4m4.5-4 4.5 4m-9.5 0v5m9.5-5v5m-12 5h5v-5h-5Zm9.5 0h5v-5h-5ZM10 7a1.4 1.4 0 1 0 0-2.8A1.4 1.4 0 0 0 10 7Z',
  git: 'M6 3.6a1.9 1.9 0 1 1 0 3.8 1.9 1.9 0 0 1 0-3.8Zm8 9a1.9 1.9 0 1 1 0 3.8 1.9 1.9 0 0 1 0-3.8ZM6 7.4v3.1a3 3 0 0 0 3 3h5m-8 0 .1-3.1',
}

function title(item: ActivityItem) {
  const names: Record<ActivityItem, string> = {
    commit: 'Commit', files: 'Files', worktrees: 'Worktrees', git: 'Git',
  }
  return names[item]
}

defineExpose({ focusSettings: () => settingsButton.value?.focus() })
</script>

<template>
  <nav class="activity-bar" aria-label="Tool windows">
    <button v-for="item in (['commit', 'files', 'worktrees', 'git'] as const)" :key="item"
            :aria-pressed="item === active" :aria-label="title(item)" :title="title(item)" :disabled="busy"
            @click="emit('select', item)">
      <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path :d="icons[item]" />
      </svg>
    </button>
    <span class="activity-spacer" aria-hidden="true"></span>
    <button ref="settings-button" aria-label="Settings" title="Settings" @click="emit('settings')">
      <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
        <circle cx="10" cy="10" r="2.2" />
        <path d="M10 2.8v2m0 10.4v2M2.8 10h2m10.4 0h2M4.9 4.9l1.4 1.4m7.4 7.4 1.4 1.4m0-10.2-1.4 1.4M6.3 13.7l-1.4 1.4" />
      </svg>
    </button>
  </nav>
</template>
