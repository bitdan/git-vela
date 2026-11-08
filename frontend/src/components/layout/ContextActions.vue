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
  panel.value.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus({ preventScroll: true })
}

function outside(event: PointerEvent) {
  if (event.target instanceof Node && !panel.value?.contains(event.target)) emit('close')
}

function close() {
  emit('close')
  props.anchor.trigger.focus({ preventScroll: true })
}

function move(event: KeyboardEvent, direction: number) {
  const buttons = Array.from(panel.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
  if (!buttons.length) return
  const index = buttons.indexOf(event.target as HTMLButtonElement)
  buttons[(index + direction + buttons.length) % buttons.length]?.focus()
}

function dismiss() { emit('close') }

onMounted(() => {
  place()
  document.addEventListener('pointerdown', outside)
  window.addEventListener('resize', dismiss)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', outside)
  window.removeEventListener('resize', dismiss)
})
watch(() => props.anchor, place, { flush: 'post' })
</script>

<template>
  <div ref="panel" class="command-menu context-actions" :style="position" role="group" :aria-label="label"
       @keydown.esc.stop.prevent="close" @keydown.up.stop.prevent="move($event, -1)"
       @keydown.down.stop.prevent="move($event, 1)">
    <slot />
  </div>
</template>
