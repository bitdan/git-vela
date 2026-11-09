<script setup lang="ts">
import { useNotifications } from './notifications'

/** IDEA-style balloons in the lower right, above the status bar. */
const notifications = useNotifications()
</script>

<template>
  <div class="notification-balloons" aria-live="polite">
    <div v-for="item in notifications.items" :key="item.id" class="notification-balloon" :class="item.kind"
         :role="item.kind === 'error' ? 'alert' : 'status'">
      <div class="notification-text">
        <strong>{{ item.title }}</strong>
        <span>{{ item.detail }}</span>
      </div>
      <button class="notification-close" aria-label="Dismiss notification" @click="notifications.dismiss(item.id)">×</button>
    </div>
  </div>
</template>
