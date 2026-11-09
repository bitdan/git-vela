<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ name: string; folder: boolean; root?: boolean }>()
const extension = computed(() => props.name.toLowerCase().split('.').at(-1) || '')
const label = computed(() => ({ go: 'Go', ts: 'TS', js: 'JS', vue: 'V', json: '{}', md: 'M' })[extension.value])
</script>

<template>
  <svg v-if="folder" class="project-node-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M2 4.5a1 1 0 0 1 1-1h3l1.5 1.5H13a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z"
          stroke="currentColor" stroke-linejoin="round" />
    <rect v-if="root" x="9" y="9" width="6" height="6" rx="1" fill="var(--panel)" stroke="var(--accent)" />
  </svg>
  <span v-else-if="label" class="project-node-icon project-type-icon" :data-type="extension" aria-hidden="true">{{ label }}</span>
  <svg v-else class="project-node-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M4 2.5h5l3 3v8H4zM9 2.5v3h3" stroke="currentColor" stroke-linejoin="round" />
  </svg>
</template>
