<script setup lang="ts">
import { onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import type { ContextAnchor } from './contextAnchor'

const props = defineProps<{ anchor: ContextAnchor; label: string }>()
const emit = defineEmits<{ close: [] }>()
const panel = useTemplateRef<HTMLElement>('panel')
const position = ref<{ left: string; top: string }>()

function place() {
  if (!panel.value) return
  const { offsetWidth, offsetHeight } = panel.value
  const { x, y } = props.anchor
  position.value = {
    left: Math.max(8, Math.min(x, window.innerWidth - offsetWidth - 8)) + 'px',
    top: Math.max(8, Math.min(y, window.innerHeight - offsetHeight - 8)) + 'px',
  }
  const initial = panel.value.querySelector<HTMLElement>('[data-initial-focus]') ?? panel.value.querySelector<HTMLButtonElement>('button:not(:disabled)')
  initial?.focus({ preventScroll: true })
}

function outside(event: PointerEvent) {
  if (!(event.target instanceof Node) || panel.value?.contains(event.target)) return
  if (props.anchor.toggle && props.anchor.trigger.contains(event.target)) return
  dismiss()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || event.defaultPrevented || event.isComposing) return
  if (event.target instanceof Element && event.target.closest('dialog[open]') &&
      !panel.value?.contains(event.target)) return
  event.preventDefault()
  event.stopImmediatePropagation()
  dismiss()
  props.anchor.trigger.focus({ preventScroll: true })
}

function move(event: KeyboardEvent, direction: number) {
  if (!(event.target instanceof HTMLButtonElement)) return
  event.preventDefault()
  const buttons = Array.from(panel.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
  if (!buttons.length) return
  const index = buttons.indexOf(event.target as HTMLButtonElement)
  buttons[(index + direction + buttons.length) % buttons.length]?.focus()
}

function dismiss() { emit('close') }

onMounted(() => {
  place()
  document.addEventListener('pointerdown', outside, true)
  document.addEventListener('keydown', onKeydown, true)
  window.addEventListener('resize', dismiss)
  window.addEventListener('blur', dismiss)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', outside, true)
  document.removeEventListener('keydown', onKeydown, true)
  window.removeEventListener('resize', dismiss)
  window.removeEventListener('blur', dismiss)
})
watch(() => props.anchor, place, { flush: 'post' })
</script>

<template>
  <div ref="panel" class="command-menu context-actions" :style="position" role="group" :aria-label="label"
       @keydown.up.stop="move($event, -1)"
       @keydown.down.stop="move($event, 1)">
    <slot />
  </div>
</template>
