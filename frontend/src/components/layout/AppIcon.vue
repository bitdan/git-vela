<script setup lang="ts">
import { iconPaths, type IconName } from './icons'

/** Inline Ant Design (iconfont) outline icon; inherits `currentColor`. */
defineProps<{ name: IconName }>()
</script>

<template>
  <svg class="app-icon" viewBox="64 64 896 896" fill="currentColor" aria-hidden="true">
    <path v-for="(d, index) in iconPaths[name]" :key="index" :d="d" />
  </svg>
</template>
