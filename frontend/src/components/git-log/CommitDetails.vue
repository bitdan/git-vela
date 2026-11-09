<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import { usePreferences } from '../../stores/preferences'
import { issueSegments } from './issueLinks'
import { useWorkspace } from '../../stores/workspace'

/** Commit Details panel (IDEA CommitDetailsListPanel counterpart): hash,
 *  author, date, subject, full message, parents and refs for the commit
 *  selected in the Git Log. Reads the shared log selection state; it must
 *  never mutate it. */
const store = useWorkspace()
const message = ref('')
const messageError = ref('')
const loading = ref(false)
let requestToken = 0

const commit = computed(() => store.selected)
const commitTime = computed(() => commit.value
  // commit.time is a Unix timestamp in seconds; the missing scale here
  // rendered epoch dates like 1970年1月1日 08:52.
  ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(commit.value.time * 1000))
  : '')

watch(() => commit.value?.hash, async hash => {
  const token = ++requestToken
  message.value = ''
  messageError.value = ''
  const repo = store.repository?.path
  if (!hash || !repo) return
  loading.value = true
  try {
    const full = await api.CommitMessage(repo, hash)
    if (token === requestToken) message.value = full
  } catch (error) {
    if (token === requestToken) messageError.value = String(error)
  } finally {
    if (token === requestToken) loading.value = false
  }
}, { immediate: true })

function refLabel(ref: string) {
  return ref.replace(/^refs\/(heads|remotes|tags)\//, '')
}

const preferences = usePreferences()
/** Issue navigation renders configured issue IDs (e.g. PROJ-123) as links in
 *  the subject and the full message body. */
const subjectSegments = computed(() => issueSegments(commit.value?.subject ?? '', preferences.issuePattern, preferences.issueURL))
const messageSegments = computed(() => issueSegments(message.value, preferences.issuePattern, preferences.issueURL))
</script>

<template>
  <section class="panel details-panel" aria-label="Commit Details">
    <header class="panel-header">
      <h2>Commit Details</h2>
    </header>
    <p v-if="store.comparisonRange" class="notice">
      Comparing <code>{{ store.comparisonRange.before.slice(0, 10) }}</code> →
      <code>{{ store.comparisonRange.after.slice(0, 10) }}</code> · {{ store.comparisonRange.label }}
    </p>
    <p v-else-if="!commit" class="notice">Select a commit to see its details.</p>
    <div v-else class="details-body">
      <div class="details-subject ellipsis" :title="commit.subject">
        <template v-for="(segment, index) in subjectSegments" :key="index">
          <a v-if="segment.href" :href="segment.href" target="_blank" rel="noopener">{{ segment.text }}</a>
          <template v-else>{{ segment.text }}</template>
        </template>
      </div>
      <div class="details-grid">
        <span class="muted">Hash</span>
        <code :title="commit.hash">{{ commit.hash.slice(0, 12) }}</code>
        <span class="muted">Author</span>
        <span class="ellipsis">{{ commit.author }}</span>
        <span class="muted">Date</span>
        <span>{{ commitTime }}</span>
        <span class="muted">Parents</span>
        <span class="ellipsis">
          <template v-if="commit.parents?.length">
            <code v-for="parent in commit.parents" :key="parent" class="details-parent">{{ parent.slice(0, 10) }}</code>
          </template>
          <template v-else>Root or shallow boundary</template>
        </span>
        <template v-if="(commit.parents?.length ?? 0) > 1">
          <span class="muted">Compare</span>
          <select :value="store.parent" aria-label="Compare parent commit"
                  @change="store.selectCommit(store.selected!, Number(($event.target as HTMLSelectElement).value))">
            <option v-for="(hash, index) in commit.parents" :key="hash" :value="index">{{ index + 1 }} · {{ hash.slice(0, 10) }}</option>
          </select>
        </template>
        <template v-if="commit.refs?.length">
          <span class="muted">Refs</span>
          <span class="details-refs">
            <span v-for="ref in commit.refs" :key="ref" class="chip" :title="ref">{{ refLabel(ref) }}</span>
          </span>
        </template>
      </div>
      <pre v-if="message || loading || messageError" class="details-message"><template v-if="loading">Loading message…</template><template v-else-if="messageError">{{ messageError }}</template><template v-else-if="message"><template v-for="(segment, index) in messageSegments" :key="index"><a v-if="segment.href" :href="segment.href" target="_blank" rel="noopener">{{ segment.text }}</a><template v-else>{{ segment.text }}</template></template></template></pre>
    </div>
  </section>
</template>
