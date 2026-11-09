<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { bracketMatching, indentOnInput, indentUnit } from '@codemirror/language'
import { highlightSelectionMatches, openSearchPanel, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState } from '@codemirror/state'
import { drawSelection, EditorView, highlightActiveLine, keymap, lineNumbers } from '@codemirror/view'
import ContextActions from '../layout/ContextActions.vue'
import { contextAnchor, type ContextAnchor } from '../layout/contextAnchor'
import { usePreferences } from '../../stores/preferences'
import { textHighlighting, textLanguage } from './textLanguage'

const props = defineProps<{
  path: string; initialContent: string; dark: boolean; active: boolean
  dirty: boolean; saving: boolean; loading: boolean; blocked: boolean
}>()
const emit = defineEmits<{ change: [content: string]; save: []; reload: []; gitMenu: [event: MouseEvent | KeyboardEvent]; cursor: [line: number, col: number] }>()
const host = useTemplateRef<HTMLElement>('host')
const lineInput = useTemplateRef<HTMLInputElement>('line-input')
const preferences = usePreferences()
const theme = new Compartment()
const wrapping = new Compartment()
const indentation = new Compartment()
const actions = shallowRef<ContextAnchor>()
const goToLineOpen = ref(false)
const goToLineValue = ref('')
const goToLineError = ref('')
let editor: EditorView | undefined

function indentationConfig() {
  return [EditorState.tabSize.of(preferences.editorTabSize), indentUnit.of(' '.repeat(preferences.editorTabSize))]
}

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
        wrapping.of(preferences.editorWordWrap ? EditorView.lineWrapping : []),
        indentation.of(indentationConfig()),
        EditorView.contentAttributes.of({ 'aria-label': 'Edit ' + props.path }),
        theme.of(EditorView.theme({}, { dark: props.dark })),
        keymap.of([
          { key: 'Mod-s', run: () => { emit('save'); return true } },
          { key: 'Mod-g', run: () => { openGoToLine(); return true } },
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
  if (props.active) editor.focus()
})
watch(() => props.active, active => {
  if (active) editor?.focus()
  else {
    closeActions()
    goToLineOpen.value = false
  }
}, { flush: 'post' })
watch(() => props.dark, dark => editor?.dispatch({ effects: theme.reconfigure(EditorView.theme({}, { dark })) }))
watch(() => preferences.editorWordWrap, wrap => editor?.dispatch({
  effects: wrapping.reconfigure(wrap ? EditorView.lineWrapping : []),
}))
watch([() => preferences.editorFontSize, () => preferences.editorTabSize], () => {
  editor?.dispatch({ effects: indentation.reconfigure(indentationConfig()) })
})
onUnmounted(() => editor?.destroy())

function closeActions() { actions.value = undefined }

function find() {
  if (editor) openSearchPanel(editor)
}

function openGoToLine() {
  if (!editor) return
  goToLineValue.value = String(editor.state.doc.lineAt(editor.state.selection.main.head).number)
  goToLineError.value = ''
  goToLineOpen.value = true
  void nextTick(() => lineInput.value?.select())
}

function cancelGoToLine() {
  goToLineOpen.value = false
  goToLineError.value = ''
  editor?.focus()
}

/** Ctrl+G: jump the cursor to the typed line, keeping the view centred even
 *  when the target is far outside the rendered range. */
function submitGoToLine() {
  if (!editor) return
  const line = Number.parseInt(goToLineValue.value.trim(), 10)
  if (!Number.isFinite(line) || line < 1 || line > editor.state.doc.lines) {
    goToLineError.value = 'Enter a line between 1 and ' + editor.state.doc.lines
    return
  }
  const target = editor.state.doc.line(line)
  editor.dispatch({
    selection: { anchor: target.from },
    effects: EditorView.scrollIntoView(target.from, { y: 'center' }),
  })
  goToLineOpen.value = false
  goToLineError.value = ''
  editor.focus()
}
</script>

<template>
  <div class="workspace-editor" tabindex="-1"
       @contextmenu.prevent.stop="closeActions(); emit('gitMenu', $event)"
       @keydown.shift.f10.prevent.stop="closeActions(); emit('gitMenu', $event)">
    <Teleport to="#source-tools" v-if="active">
      <button class="source-actions-trigger" aria-label="Editor actions" title="Editor actions"
              :aria-expanded="!!actions" @click="actions = actions ? undefined : contextAnchor($event, false)">⋯</button>
    </Teleport>
    <Teleport to=".shell" v-if="active && actions">
      <ContextActions :anchor="actions" label="Editor actions" class="editor-tools" @close="closeActions">
        <button @click="closeActions(); find()">Find / Replace</button>
        <button @click="closeActions(); openGoToLine()">Go to Line</button>
        <button :disabled="blocked || loading" @click="closeActions(); emit('reload')">Reload</button>
        <button :disabled="!dirty || blocked || loading" @click="closeActions(); emit('save')">{{ saving ? 'Saving…' : 'Save' }}</button>
        <slot name="actions" :close="closeActions" />
      </ContextActions>
    </Teleport>
    <form v-if="goToLineOpen" class="go-to-line" @submit.prevent="submitGoToLine" @keydown.esc.prevent="cancelGoToLine">
      <label>Line
        <input ref="line-input" v-model="goToLineValue" type="text" inputmode="numeric" spellcheck="false"
               aria-label="Go to line number" />
      </label>
      <button class="primary" type="submit">Go</button>
      <button type="button" @click="cancelGoToLine">Cancel</button>
      <span v-if="goToLineError" class="notice error" role="alert">{{ goToLineError }}</span>
    </form>
    <div ref="host" class="editor-host" :style="{ '--editor-font-size': preferences.editorFontSize + 'px' }"></div>
  </div>
</template>
