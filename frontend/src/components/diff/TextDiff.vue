<script setup lang="ts">
import { ref, useTemplateRef, watchPostEffect } from 'vue'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'
import { MergeView, getChunks, goToNextChunk, goToPreviousChunk, unifiedMergeView } from '@codemirror/merge'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { javascript } from '@codemirror/lang-javascript'
import { go } from '@codemirror/lang-go'
import { json } from '@codemirror/lang-json'
import type { Comparison } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ comparison: Comparison; path: string; dark: boolean }>()
const host = useTemplateRef<HTMLDivElement>('host')
const unified = ref(false)
const changes = ref(0)
let editor: EditorView | undefined

const diffHighlighting = syntaxHighlighting(HighlightStyle.define([
  { tag: [tags.keyword, tags.modifier], color: 'var(--syntax-keyword)' },
  { tag: [tags.string, tags.regexp], color: 'var(--syntax-string)' },
  { tag: [tags.number, tags.bool, tags.null, tags.atom], color: 'var(--syntax-literal)' },
  { tag: [tags.typeName, tags.className, tags.namespace], color: 'var(--syntax-type)' },
  { tag: tags.function(tags.variableName), color: 'var(--syntax-function)' },
  { tag: tags.propertyName, color: 'var(--syntax-property)' },
  { tag: [tags.comment, tags.meta], color: 'var(--muted)' },
  { tag: tags.invalid, color: 'var(--error)' },
]))

function language(path: string): Extension {
  const ext = path.split('.').at(-1)?.toLowerCase()
  if (ext === 'go') return go()
  if (ext === 'json') return json()
  if (ext === 'ts' || ext === 'tsx' || ext === 'js' || ext === 'jsx' || ext === 'mjs' || ext === 'cjs') {
    return javascript({ typescript: ext.startsWith('ts'), jsx: ext.endsWith('x') })
  }
  return []
}

watchPostEffect(onCleanup => {
  if (!host.value) return
  const { before, after } = props.comparison
  const parent = host.value
  const extensions = [
    lineNumbers(), EditorState.readOnly.of(true), EditorView.editable.of(false),
    EditorView.contentAttributes.of({ tabindex: '0' }),
    diffHighlighting,
    EditorView.theme({}, { dark: props.dark }), language(props.path),
  ]
  const collapseUnchanged = { margin: 4, minSize: 10 }
  if (unified.value) {
    const view = new EditorView({
      parent,
      doc: after,
      extensions: [...extensions, unifiedMergeView({ original: before, mergeControls: false, collapseUnchanged })],
    })
    editor = view
    onCleanup(() => { view.destroy(); editor = undefined })
  } else {
    const view = new MergeView({
      parent, a: { doc: before, extensions }, b: { doc: after, extensions },
      collapseUnchanged, gutter: true,
    })
    editor = view.b
    onCleanup(() => { view.destroy(); editor = undefined })
  }
  changes.value = getChunks(editor.state)?.chunks.length ?? 0
})

function navigate(direction: number) {
  if (!editor) return
  const command = direction > 0 ? goToNextChunk : goToPreviousChunk
  command(editor)
}
</script>

<template>
  <div class="text-diff" @keydown.alt.up.prevent="navigate(-1)" @keydown.alt.down.prevent="navigate(1)">
    <div class="diff-tools">
      <span class="muted">{{ changes }} change{{ changes === 1 ? '' : 's' }}</span>
      <div class="diff-actions">
        <button :aria-pressed="unified" @click="unified = !unified">{{ unified ? 'Side by side' : 'Unified' }}</button>
        <button :disabled="!changes" title="Previous change (Alt+Up)" @click="navigate(-1)">↑</button>
        <button :disabled="!changes" title="Next change (Alt+Down)" @click="navigate(1)">↓</button>
      </div>
    </div>
    <div v-if="!unified" class="diff-labels"><span>Before</span><span>After</span></div>
    <div ref="host" class="diff-host" />
  </div>
</template>
