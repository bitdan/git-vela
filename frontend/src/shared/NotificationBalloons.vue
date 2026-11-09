<script setup lang="ts">
import { useNotifications } from './notifications'

/** IDEA-style balloons in the lower right, above the status bar. */
const notifications = useNotifications()
</script>

<template>
  <div class="notification-balloons pointer-events-none fixed right-3.5 bottom-[34px] z-60 flex w-[min(360px,calc(100vw-28px))] flex-col gap-1.5"
       aria-live="polite">
    <div v-for="item in notifications.items" :key="item.id"
         class="notification-balloon pointer-events-auto flex items-start gap-2 rounded-lg border bg-popup px-2.5 py-2 shadow-popup"
         :class="item.kind === 'error' ? 'error border-error' : 'border-line'"
         :role="item.kind === 'error' ? 'alert' : 'status'">
      <div class="flex min-w-0 flex-1 flex-col gap-0.5 wrap-anywhere">
        <strong :class="{ 'text-error': item.kind === 'error' }">{{ item.title }}</strong>
        <span>{{ item.detail }}</span>
      </div>
      <button class="border-0 bg-transparent px-1 leading-none text-muted hover:text-fg" aria-label="Dismiss notification"
              @click="notifications.dismiss(item.id)">×</button>
    </div>
  </div>
</template>
