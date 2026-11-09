import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { NotificationKind } from './gitCommand'

export interface Notification {
  id: number
  kind: NotificationKind
  title: string
  detail: string
}

/** IDEA-style balloon notifications for background Git results. Errors stay
 *  until dismissed; information fades after a few seconds. */
export const useNotifications = defineStore('notifications', () => {
  const items = ref<Notification[]>([])
  let next = 1

  function push(kind: NotificationKind, title: string, detail: string) {
    const id = next++
    items.value = [...items.value, { id, kind, title, detail }].slice(-4)
    if (kind === 'info') setTimeout(() => dismiss(id), 6000)
    return id
  }

  function dismiss(id: number) {
    items.value = items.value.filter(item => item.id !== id)
  }

  function clear() {
    items.value = []
  }

  return { items, push, dismiss, clear }
})
