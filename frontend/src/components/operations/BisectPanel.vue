<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { BisectState } from '../../../bindings/gitvela/internal/git'
import { LatestRequest } from '../../stores/latestRequest'

/** Bisect wizard (IDEA Log → Bisect). The dialog only drives Git's own
 *  bisect state; every step re-reads it so a terminal user stays in sync. */
const props = defineProps<{ path: string; refresh: () => Promise<void> }>()
const emit = defineEmits<{ close: [] }>()
const busy = defineModel<boolean>('busy', { required: true })
const state = ref<BisectState>()
const requests = new LatestRequest()
const loading = ref(false)
const error = ref('')
const notice = ref('')
const badInput = ref('HEAD')
const goodInput = ref('')
let unsubscribe: (() => void) | undefined

const active = computed(() => !!state.value?.active)
const finished = computed(() => !!state.value?.finished)
const current = computed(() => state.value?.current ?? '')
const short = (hash: string) => hash.slice(0, 8)

async function load() {
  const call = api.BisectState(props.path)
  const latest = requests.replace(call)
  loading.value = true
  try {
    const result = await call
    if (!latest()) return
    state.value = result
    if (!result?.active) {
      badInput.value = 'HEAD'
      goodInput.value = ''
    }
  } catch (cause) {
    if (latest()) error.value = String(cause)
  } finally {
    if (latest()) loading.value = false
    requests.finish(call)
  }
}

async function run(action: () => Promise<void>, message: string) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await action()
    notice.value = message
    await load()
    await props.refresh()
  } catch (cause) {
    error.value = String(cause)
  } finally {
    busy.value = false
  }
}

function start() {
  void run(async () => {
    await api.StartBisect(props.path, badInput.value.trim(), goodInput.value.trim())
  }, 'Bisect started.')
}

function mark(verdict: 'good' | 'bad') {
  void run(async () => {
    await api.MarkBisect(props.path, verdict)
  }, verdict === 'good' ? 'Marked good.' : 'Marked bad.')
}

function reset() {
  void run(async () => {
    await api.ResetBisect(props.path)
  }, 'Bisect finished; the original branch is checked out again.')
}

onMounted(() => {
  void load()
  unsubscribe = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === props.path && !busy.value) void load()
  })
})
onUnmounted(() => {
  unsubscribe?.()
  requests.clear()
})
</script>

<template>
  <section class="daily-panel" aria-label="Bisect">
    <div class="daily-controls">
      <h2>Bisect</h2>
      <template v-if="!active">
        <p class="notice">Find the commit that introduced a problem by testing the commits Git picks between a known bad and a known good revision.</p>
        <label>Bad commit
          <input v-model="badInput" placeholder="HEAD or a commit hash" spellcheck="false" :disabled="busy || loading"
                 aria-label="Known bad commit" />
        </label>
        <label>Good commit
          <input v-model="goodInput" placeholder="Tag or commit hash of a known good revision" spellcheck="false"
                 :disabled="busy || loading" aria-label="Known good commit" />
        </label>
        <div>
          <button class="primary" :disabled="busy || loading || !badInput.trim() || !goodInput.trim()" @click="start">Start bisect</button>
          <button :disabled="busy || loading" @click="load">Reload</button>
          <button :disabled="busy" @click="emit('close')">Close</button>
        </div>
      </template>
      <template v-else>
        <p v-if="finished" class="notice" role="status">
          First bad commit: <code>{{ short(state?.firstBad ?? '') }}</code>.
          Finish the run to return to your branch.
        </p>
        <template v-else>
          <p class="notice">Test <code>{{ short(current) }}</code> {{ state?.subject }} and mark it.</p>
          <p class="muted">{{ state?.remaining ?? 0 }} candidate commits left. Marked {{ state?.good?.length ?? 0 }} good,
            {{ state?.bad ? 1 : 0 }} bad.</p>
          <div>
            <button class="primary" :disabled="busy || loading" @click="mark('good')">Mark good</button>
            <button :disabled="busy || loading" @click="mark('bad')">Mark bad</button>
          </div>
        </template>
        <div>
          <button :disabled="busy || loading" @click="reset">Finish bisect</button>
          <button :disabled="busy" @click="emit('close')">Close</button>
        </div>
      </template>
    </div>
    <p v-if="loading" class="notice" role="status">Reading bisect state…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  </section>
</template>
