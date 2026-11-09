<script setup lang="ts">
import { useTemplateRef } from 'vue'
import AppIcon from './AppIcon.vue'
import type { IconName } from './icons'

/** IDEA-style vertical tool window bar. Icons are Ant Design outline glyphs
 *  (MIT, the official Ant Design library on iconfont.cn), not IntelliJ assets.
 *  Each entry toggles or focuses a tool window and keeps its semantic name for
 *  tooltips and accessibility. */
export type ActivityItem = 'commit' | 'files' | 'worktrees' | 'git'

const props = defineProps<{ active: ActivityItem | undefined; historyVisible: boolean; busy?: boolean }>()
const emit = defineEmits<{ select: [item: ActivityItem]; settings: [] }>()
const settingsButton = useTemplateRef<HTMLButtonElement>('settings-button')

const icons: Record<ActivityItem, IconName> = {
  commit: 'commit',
  files: 'files',
  worktrees: 'worktrees',
  git: 'branches',
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
      <AppIcon :name="icons[item]" />
    </button>
    <span class="activity-spacer" aria-hidden="true"></span>
    <button ref="settings-button" aria-label="Settings" title="Settings" @click="emit('settings')">
      <AppIcon name="settings" />
    </button>
  </nav>
</template>
