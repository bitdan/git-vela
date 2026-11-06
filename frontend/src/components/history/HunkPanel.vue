<script setup lang="ts">
import { onUnmounted, ref, shallowRef, watch } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { HunkPatch } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string; file: string; staged: boolean; busy: boolean; applySelection: (file: string, staged: boolean, revision: string, index: number, lines?: number[]) => Promise<void> }>()
const applying = ref(false)
const emit = defineEmits<{ close: [] }>()
const patch = shallowRef<HunkPatch>()
const selection = ref<Record<number, number[]>>({})
const requests = new LatestRequest()
const loading = ref(false)
const error = ref('')
const notice = ref('')
const verb = () => props.staged ? 'Unstage' : 'Stage'

async function load() {
  selection.value = {}
  const call = api.WorkingHunks(props.path, props.file, props.staged)
  const current = requests.replace(call)
  loading.value = true
  try {
    const result = await call
    if (current()) patch.value = result
  } catch (cause) {
    if (current()) {
      patch.value = undefined
      error.value = String(cause)
    }
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}

watch([() => props.file, () => props.staged], () => {
  patch.value = undefined
  notice.value = ''
  error.value = ''
  void load()
}, { immediate: true })
onUnmounted(() => requests.clear())

function lines(hunk: string) {
  return hunk.replace(/\n$/, '').split('\n')
}

function toggle(index: number, line: number, checked: boolean) {
  const selected = selection.value[index] ?? []
  selection.value[index] = checked ? [...selected, line] : selected.filter(item => item !== line)
}

async function apply(index: number, partial: boolean) {
  const revision = patch.value?.revision
  if (props.busy || applying.value || loading.value || !revision) return
  applying.value = true
  error.value = ''
  notice.value = ''
  try {
    await props.applySelection(props.file, props.staged, revision, index, partial ? selection.value[index] ?? [] : undefined)
    notice.value = props.staged ? 'Index selection unstaged.' : 'Selection staged.'
  } catch (cause) {
    error.value = String(cause)
  } finally {
    try {
      await load()
    } finally {
      applying.value = false
    }
  }
}
</script>

<template>
  <div class="file-inspection hunk-panel">
    <div class="editor-tools">
      <strong>{{ verb() }} hunks or selected lines</strong>
      <div>
        <button :disabled="busy || applying || loading" @click="error = ''; load()">Refresh</button>
        <button :disabled="busy || applying" @click="emit('close')">Close</button>
      </div>
    </div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-if="loading" class="notice" role="status">Loading hunks…</p>
    <div v-else class="inspection-scroll">
      <p v-if="patch?.reason" class="notice">{{ patch.reason }}</p>
      <section v-for="(hunk, index) in patch?.hunks" :key="index" class="hunk-section">
        <div class="editor-tools">
          <code>{{ lines(hunk)[0] }}</code>
          <div>
            <button :disabled="busy || applying" @click="apply(index, false)">{{ verb() }} hunk</button>
            <button :disabled="busy || applying || !selection[index]?.length" @click="apply(index, true)">{{ verb() }} selected lines</button>
          </div>
        </div>
        <div v-for="(line, lineIndex) in lines(hunk).slice(1)" :key="lineIndex" class="hunk-line"
             :class="{ 'patch-added': line.startsWith('+'), 'patch-removed': line.startsWith('-') }">
          <input v-if="line.startsWith('+') || line.startsWith('-')" type="checkbox" :disabled="busy || applying"
                 :checked="selection[index]?.includes(lineIndex + 1)" :aria-label="'Select ' + line"
                 @change="toggle(index, lineIndex + 1, ($event.target as HTMLInputElement).checked)" />
          <span v-else></span>
          <code>{{ line }}</code>
        </div>
      </section>
    </div>
  </div>
</template>
