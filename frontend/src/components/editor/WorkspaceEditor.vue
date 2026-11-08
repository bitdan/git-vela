<script setup lang="ts">
import { onMounted, onUnmounted, useTemplateRef, watch } from 'vue'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { bracketMatching, indentOnInput } from '@codemirror/language'
import { highlightSelectionMatches, openSearchPanel, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState } from '@codemirror/state'
import { drawSelection, EditorView, highlightActiveLine, keymap, lineNumbers } from '@codemirror/view'
import { textHighlighting, textLanguage } from './textLanguage'

const props = defineProps<{
  path: string; initialContent: string; dark: boolean; lineEnding: string; bom: boolean
  dirty: boolean; saving: boolean; loading: boolean; blocked: boolean
}>()
const emit = defineEmits<{ change: [content: string]; save: []; reload: []; close: []; cursor: [line: number, col: number] }>()
const host = useTemplateRef<HTMLElement>('host')
const theme = new Compartment()
let editor: EditorView | undefined

onMounted(() => {
  if (!host.value) return
  editor = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.initialContent,
      extensions: [
        lineNumbers(), drawSelection(), history(), highlightActiveLine(), highlightSelectionMatches(),
        bracketMatching(), indentOnInput(), textHighlighting, textLanguage(props.path),
        EditorState.allowMultipleSelections.of(true),
        EditorView.contentAttributes.of({ 'aria-label': 'Edit ' + props.path }),
        theme.of(EditorView.theme({}, { dark: props.dark })),
        keymap.of([
          { key: 'Mod-s', run: () => { emit('save'); return true } },
          ...searchKeymap, ...defaultKeymap, ...historyKeymap,
        ]),
        EditorView.updateListener.of(update => {
          if (update.docChanged) emit('change', update.state.doc.toString())
          if (update.docChanged || update.selectionSet) {
            const head = update.state.selection.main.head
            const line = update.state.doc.lineAt(head)
            emit('cursor', line.number, head - line.from + 1)
          }
        }),
      ],
    }),
  })
  editor.focus()
})
watch(() => props.dark, dark => editor?.dispatch({ effects: theme.reconfigure(EditorView.theme({}, { dark })) }))
onUnmounted(() => editor?.destroy())

function find() {
  if (editor) openSearchPanel(editor)
}
</script>

<template>
  <div class="workspace-editor">
    <div class="editor-tools">
      <span class="muted" role="status">{{ dirty ? 'Unsaved changes' : 'Saved' }} · UTF-8{{ bom ? ' BOM' : '' }} · {{ lineEnding }}</span>
      <div>
        <button @click="find">Find / Replace</button>
        <button :disabled="blocked || loading" @click="emit('reload')">Reload</button>
        <button :disabled="blocked || loading" @click="emit('close')">Close</button>
        <button class="primary" :disabled="!dirty || blocked || loading" @click="emit('save')">{{ saving ? 'Saving…' : 'Save' }}</button>
      </div>
    </div>
    <div ref="host" class="editor-host"></div>
  </div>
</template>
