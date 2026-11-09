import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { NotificationKind } from './gitCommand'

export interface NotificationAction {
  label: string
  run: () => void
}

export interface Notification {
  id: number
  kind: NotificationKind
  title: string
  detail: string
  /** IDEA notification actions such as Restore or View Commits. */
  actions?: NotificationAction[]
}

/** IDEA-style balloon notifications for background Git results. Errors stay
 *  until dismissed; information fades after a few seconds. */
export const useNotifications = defineStore('notifications', () => {
  const items = ref<Notification[]>([])
  let next = 1

  function push(kind: NotificationKind, title: string, detail: string, actions?: NotificationAction[]) {
    const id = next++
    items.value = [...items.value, { id, kind, title, detail, actions }].slice(-4)
    // Actionable balloons stay until used or dismissed, like IDEA's sticky ones.
    if (kind === 'info' && !actions?.length) setTimeout(() => dismiss(id), 6000)
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
