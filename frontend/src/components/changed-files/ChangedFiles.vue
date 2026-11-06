<script setup lang="ts">
import { computed, useTemplateRef, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useWorkspace } from '../../stores/workspace'

const store = useWorkspace()
const emit = defineEmits<{ select: [] }>()
const list = useTemplateRef<HTMLElement>('list')
const virtual = useVirtualizer(computed(() => ({
  count: store.files.length,
  getScrollElement: () => list.value,
  estimateSize: () => 34,
  overscan: 8,
})))
watch(() => [store.selected?.hash, store.parent], () => {
  if (list.value) list.value.scrollTop = 0
})
</script>

<template>
  <section class="panel files-panel" aria-label="Changed Files">
    <header class="panel-header">
      <div><span class="eyebrow">COMMIT DETAILS</span><h2>Changed Files <small>{{ store.files.length }}</small></h2></div>
    </header>
    <div v-if="store.selected" class="commit-summary">
      <strong class="ellipsis" :title="store.selected.subject">{{ store.selected.subject }}</strong>
      <div class="meta"><code :title="store.selected.hash">{{ store.selected.hash.slice(0, 12) }}</code><span class="ellipsis">{{ store.selected.author }}</span></div>
      <label v-if="(store.selected.parents?.length ?? 0) > 1">
        Compare parent
        <select :value="store.parent" @change="store.selectCommit(store.selected, Number(($event.target as HTMLSelectElement).value))">
          <option v-for="(hash, index) in store.selected.parents" :key="hash" :value="index">{{ index + 1 }} · {{ hash.slice(0, 10) }}</option>
        </select>
      </label>
      <span v-else class="muted">{{ store.selected.parents?.[0] ? 'Parent ' + store.selected.parents[0].slice(0, 10) : 'Root or shallow boundary' }}</span>
    </div>
    <p v-if="store.loadingFiles" class="notice" role="status">Loading changed files…</p>
    <p v-else-if="store.filesError" class="notice error" role="alert">{{ store.filesError }}</p>
    <p v-else-if="!store.files.length" class="notice">{{ store.selected ? 'No changed files.' : 'Select a commit.' }}</p>
    <div ref="list" class="file-scroll">
      <div class="virtual-content" :style="{ height: virtual.getTotalSize() + 'px' }">
        <button v-for="item in virtual.getVirtualItems()" :key="item.index" class="file-row"
                :class="{ active: store.file?.index === item.index }" :aria-pressed="store.file?.index === item.index"
                :style="{ transform: 'translateY(' + item.start + 'px)' }"
                :title="store.files[item.index].oldPath ? store.files[item.index].oldPath + ' → ' + store.files[item.index].path : store.files[item.index].path"
                @click="emit('select'); store.selectFile(store.files[item.index])">
          <code class="file-status" :data-status="store.files[item.index].status">{{ store.files[item.index].status }}</code>
          <span class="ellipsis">{{ store.files[item.index].oldPath ? store.files[item.index].oldPath + ' → ' : '' }}{{ store.files[item.index].path }}</span>
        </button>
      </div>
    </div>
  </section>
</template>
