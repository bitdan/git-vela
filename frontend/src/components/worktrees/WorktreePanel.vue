<script setup lang="ts">
import { Events } from '@wailsio/runtime'
import { GitvelaRepositoryChanged } from '../../../bindings/events'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { Worktree } from '../../../bindings/gitvela/internal/git'
import { useWorkspace } from '../../stores/workspace'
import { LatestRequest } from '../../stores/latestRequest'

const props = defineProps<{ path: string }>()
const emit = defineEmits<{ open: [path: string] }>()
const busy = defineModel<boolean>('busy', { required: true })
const store = useWorkspace()
const trees = ref<Worktree[]>([])
const listError = ref('')
const actionError = ref('')
const notice = ref('')
const loading = ref(false)
const creating = ref(false)
const removal = ref<Worktree>()
const target = ref('')
const branch = ref('')
const newBranch = ref(true)
const requests = new LatestRequest()
const branches = computed(() => store.branchRefs
  .filter(item => item.name.startsWith('refs/heads/'))
  .map(item => item.name.slice('refs/heads/'.length))
  .filter(name => !trees.value.some(tree => tree.branch === name)))
let timer: ReturnType<typeof setInterval>
let unsubscribeChanges: (() => void) | undefined

async function load() {
  const call = api.Worktrees(props.path)
  const current = requests.replace(call)
  loading.value = true
  try {
    const result = await call
    if (current()) {
      trees.value = result ?? []
      listError.value = ''
    }
  } catch (error) {
    if (current()) listError.value = String(error)
  } finally {
    if (current()) loading.value = false
    requests.finish(call)
  }
}

onMounted(() => {
  void load()
  unsubscribeChanges = Events.On(GitvelaRepositoryChanged, event => {
    if (event.data === props.path && !document.hidden && !busy.value && !loading.value) void load()
  })
  timer = setInterval(() => {
    if (!document.hidden && !busy.value && !loading.value) void load()
  }, 5000)
})
onUnmounted(() => {
  clearInterval(timer)
  unsubscribeChanges?.()
  requests.clear()
})

function canRemove(tree: Worktree) {
  return tree.path !== props.path && !tree.main && !tree.bare && !tree.locked &&
    !tree.prunable && !tree.detached && !!tree.branch
}

function toggleCreate() {
  creating.value = !creating.value
  removal.value = undefined
  actionError.value = ''
  notice.value = ''
}

function requestRemoval(tree: Worktree) {
  removal.value = tree
  creating.value = false
  actionError.value = ''
  notice.value = ''
}

async function update(operation: 'add' | 'remove') {
  if (busy.value || store.opening) return
  const path = props.path
  const base = newBranch.value ? store.selected?.hash : ''
  const remove = removal.value
  if (operation === 'add' && (!target.value.trim() || !branch.value.trim() || base === undefined)) return
  if (operation === 'remove' && (!remove || !canRemove(remove))) return
  busy.value = true
  actionError.value = ''
  notice.value = ''
  try {
    if (operation === 'add') {
      await api.AddWorktree(path, target.value.trim(), branch.value.trim(), base ?? '')
      notice.value = 'Worktree created.'
      creating.value = false
      target.value = ''
      branch.value = ''
      if (newBranch.value && store.repository?.path === path) await store.open(path, true)
    } else if (remove) {
      await api.RemoveWorktree(path, remove.path)
      notice.value = 'Worktree removed.'
      removal.value = undefined
    }
  } catch (error) {
    actionError.value = String(error)
  } finally {
    if (store.repository?.path === path) await load()
    busy.value = false
  }
}

function description(tree: Worktree) {
  return [
    tree.bare ? 'Bare repository' : tree.branch || (tree.detached ? 'Detached HEAD' : 'Unknown branch'),
    tree.head && tree.head.slice(0, 8),
    tree.main && 'Main',
    tree.path === props.path && 'Current',
    tree.locked && 'Locked',
    tree.prunable && 'Unavailable',
  ].filter(Boolean).join(' · ')
}
</script>

<template>
  <aside class="panel branch-panel" aria-label="Worktrees">
    <header class="panel-header">
      <h2>Worktrees</h2>
      <div class="worktree-actions">
        <button :disabled="busy || loading" @click="load">Refresh</button>
        <button :disabled="busy" :aria-expanded="creating" @click="toggleCreate">New…</button>
      </div>
    </header>
    <div class="branch-scroll">
      <form v-if="creating" class="worktree-form" @submit.prevent="update('add')">
        <label>
          New directory
          <input v-model="target" placeholder="Absolute path" spellcheck="false" :disabled="busy" />
        </label>
        <label>
          Branch mode
          <select v-model="newBranch" :disabled="busy" @change="branch = ''">
            <option :value="true">Create branch</option>
            <option :value="false">Use existing branch</option>
          </select>
        </label>
        <label v-if="newBranch">
          New branch name
          <input v-model="branch" spellcheck="false" :disabled="busy" />
          <small class="muted">{{ store.selected ? 'From commit ' + store.selected.hash.slice(0, 8) : 'Select a commit first.' }}</small>
        </label>
        <label v-else>
          Local branch
          <select v-model="branch" :disabled="busy">
            <option value="" disabled>{{ branches.length ? 'Select a branch' : 'No available branches' }}</option>
            <option v-for="name in branches" :key="name">{{ name }}</option>
          </select>
        </label>
        <div class="worktree-actions">
          <button type="button" :disabled="busy" @click="toggleCreate">Cancel</button>
          <button type="submit" :disabled="busy || store.opening || !target.trim() || !branch.trim() || (newBranch && !store.selected)">Create</button>
        </div>
      </form>
      <div v-else-if="removal" class="worktree-form" aria-label="Confirm worktree removal">
        <p>Remove the checkout at <strong>{{ removal.path }}</strong>? Its branch will be kept.</p>
        <div class="worktree-actions">
          <button :disabled="busy" @click="removal = undefined; actionError = ''">Cancel</button>
          <button class="danger-button" :disabled="busy || store.opening" @click="update('remove')">Remove worktree</button>
        </div>
      </div>
      <p v-if="busy" class="notice" role="status">Git operation in progress…</p>
      <p v-if="listError" class="notice error" role="alert">{{ listError }}</p>
      <p v-if="actionError" class="notice error" role="alert">{{ actionError }}</p>
      <p v-if="notice" class="notice" role="status">{{ notice }}</p>
      <p v-if="loading && !trees.length" class="notice" role="status">Loading worktrees…</p>
      <div v-for="tree in trees" :key="tree.path" class="worktree-row">
        <button class="side-item ellipsis" :class="{ active: tree.path === path }" :title="tree.path"
                :disabled="busy || store.opening || tree.path === path || tree.bare || tree.prunable"
                @click="emit('open', tree.path)">{{ tree.path }}</button>
        <button v-if="canRemove(tree)" class="switch-branch" :disabled="busy || store.opening" :aria-label="'Remove ' + tree.path"
                @click="requestRemoval(tree)">Remove…</button>
        <small class="muted ellipsis" :title="description(tree)">{{ description(tree) }}</small>
      </div>
    </div>
  </aside>
</template>
