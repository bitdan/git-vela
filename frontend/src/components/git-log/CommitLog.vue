<script setup lang="ts">
import { computed, ref, useTemplateRef, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useWorkspace } from '../../stores/workspace'
import GraphCell from './GraphCell.vue'

const store = useWorkspace()
const emit = defineEmits<{ select: [] }>()
const scroll = useTemplateRef<HTMLElement>('scroll')
const query = ref('')
const notice = ref('')
const graphPixels = computed(() => Math.max(52, store.graphWidth * 13 + 14))
const columns = computed(() => ({ '--graph-width': graphPixels.value + 'px' }))
const virtual = useVirtualizer(computed(() => ({
  count: store.commits.length,
  getScrollElement: () => scroll.value,
  estimateSize: () => 34,
  overscan: 12,
})))
const rows = computed(() => virtual.value.getVirtualItems())
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })
const tooltip = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

watch(() => rows.value.at(-1)?.index, index => {
  if (index !== undefined && index >= store.commits.length - 20 && !store.logError) void store.loadPage()
})

function select(index: number) {
  const commit = store.commits[index]
  if (!commit) return
  virtual.value.scrollToIndex(index, { align: 'auto' })
  emit('select')
  void store.selectCommit(commit)
}

function jump(hash: string) {
  const index = store.commits.findIndex(item => item.hash === hash)
  if (index >= 0) {
    select(index)
    return true
  }
  return false
}

function navigate(event: KeyboardEvent, direction: number) {
  if (!store.commits.length) return
  event.preventDefault()
  const index = store.commits.findIndex(item => item.hash === store.selected?.hash)
  select(Math.max(0, Math.min(store.commits.length - 1, index + direction)))
}

function search() {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return
  const index = store.commits.findIndex(item => item.hash === store.selected?.hash)
  for (let i = 1; i <= store.commits.length; i++) {
    const position = (index + i) % store.commits.length
    const row = store.commits[position]
    if ([row.subject, row.author, row.hash, ...(row.refs ?? [])].some(text => text.toLocaleLowerCase().includes(value))) {
      select(position)
      notice.value = ''
      return
    }
  }
  notice.value = 'No match in loaded commits.'
}

watch(query, () => { notice.value = '' })
defineExpose({ jump })
</script>

<template>
  <section class="panel log-panel" aria-label="Git Log">
    <header class="panel-header">
      <div><span class="eyebrow">HISTORY</span><h2>Git Log</h2></div>
      <div class="log-tools">
        <span class="muted">{{ store.commits.length.toLocaleString() }} loaded</span>
        <form class="search" @submit.prevent="search">
          <input v-model="query" aria-label="Find in loaded commits" placeholder="Find in loaded commits" />
          <button type="submit" :disabled="!query.trim()">Find next</button>
        </form>
      </div>
    </header>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-if="store.logError" class="notice error" role="alert">{{ store.logError }}</p>
    <div ref="scroll" class="log-scroll" :style="columns" tabindex="0" aria-label="Commit history"
         @keydown.up="navigate($event, -1)" @keydown.down="navigate($event, 1)">
      <div class="log-grid log-heading"><span>Graph</span><span>Commit message</span><span>Author</span><span>Date</span><span>Hash</span></div>
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <button v-for="item in rows" :key="store.commits[item.index].hash"
                class="log-grid commit-row" :class="{ active: store.selected?.hash === store.commits[item.index].hash }"
                :aria-pressed="store.selected?.hash === store.commits[item.index].hash"
                :style="{ transform: 'translateY(' + item.start + 'px)' }" @click="select(item.index)">
          <GraphCell :row="store.commits[item.index].graph" :width="graphPixels" />
          <span class="commit-title" :title="store.commits[item.index].subject">
            <span v-for="label in store.commits[item.index].refs" :key="label" class="chip" :title="label">
              {{ label.replace(/^refs\/(heads|remotes|tags)\//, '') }}
            </span>
            <span class="ellipsis">{{ store.commits[item.index].subject || '(no subject)' }}</span>
          </span>
          <span class="ellipsis muted">{{ store.commits[item.index].author }}</span>
          <time class="muted" :title="tooltip.format(store.commits[item.index].time * 1000)">{{ date.format(store.commits[item.index].time * 1000) }}</time>
          <code class="muted" :title="store.commits[item.index].hash">{{ store.commits[item.index].hash.slice(0, 8) }}</code>
        </button>
      </div>
      <p v-if="!store.commits.length && !store.loading && !store.logError" class="notice">This repository has no commits yet.</p>
      <p v-if="store.loading" class="notice" role="status">Loading commits…</p>
      <button v-else-if="store.more" class="load-more" @click="store.loadPage()">Load more commits</button>
    </div>
  </section>
</template>
