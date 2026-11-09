<script setup lang="ts">
/** IDEA-style status bar: repository and branch, editor caret and file
 *  metadata, and the Git activity indicator. Hook classes (statusbar,
 *  editor-meta, line-ending-widget, status-dot) are kept for desktop smoke tests. */
defineProps<{
  path?: string
  head?: string
  cursor?: { line: number; col: number }
  editor?: { saving: boolean; dirty: boolean; bom: boolean; lineEnding: string }
  opening: boolean
  busy: boolean
  gitVersion?: string
}>()
const emit = defineEmits<{ switchLineEnding: [] }>()
</script>

<template>
  <footer class="statusbar flex flex-none items-center justify-between gap-2 bg-bg px-3 py-[5px] text-xs text-muted">
    <span class="ellipsis" :title="path">{{ path ? path + ' · ' + (head === 'HEAD' ? 'detached HEAD' : head) : 'No repository open' }}</span>
    <span v-if="cursor" class="editor-cursor flex-none tabular-nums">Ln {{ cursor.line }}, Col {{ cursor.col }}</span>
    <span v-if="editor" class="editor-meta inline-flex items-center gap-[3px]">
      <span role="status">{{ editor.saving ? 'Saving…' : editor.dirty ? 'Unsaved changes' : 'Saved' }}</span> ·
      UTF-8{{ editor.bom ? ' BOM' : '' }} ·
      <!-- IDEA line-ending widget: converts the active file on click. -->
      <button class="line-ending-widget rounded-[3px] border-0 bg-transparent px-[3px] py-0 text-[inherit] hover:bg-hover disabled:bg-transparent"
              :disabled="busy || opening"
              :title="'Convert line endings to ' + (editor.lineEnding === 'CRLF' ? 'LF' : 'CRLF')"
              @click="emit('switchLineEnding')">{{ editor.lineEnding }}</button>
    </span>
    <span class="flex flex-none items-center gap-[5px]">
      <i class="status-dot size-1.5 rounded-full" :class="opening || busy ? 'busy bg-accent' : 'bg-good'" aria-hidden="true"></i>
      {{ opening ? 'Opening…' : busy ? 'Running Git…' : 'Ready' }}<template v-if="gitVersion"> · {{ gitVersion }}</template>
    </span>
  </footer>
</template>
