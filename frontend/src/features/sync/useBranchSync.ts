import { computed, ref, watch, type Ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { Ref as GitRef } from '../../../bindings/gitvela/internal/git'
import { useWorkspace } from '../../stores/workspace'
import { usePreferences } from '../../stores/preferences'
import { useNotifications } from '../../shared/notifications'
import { createGitCommandRunner } from '../../shared/gitCommand'
import { LatestRequest, cancellableAll } from '../../stores/latestRequest'
import {
  branchSyncActions, currentBranch, describeUpdate, localBranchName, parseRemoteRef,
  type BranchSyncAction, type UpdateMethod,
} from './branchActions'

export interface BranchSyncHost {
  busy: Ref<boolean>
  refresh: (path: string) => Promise<void>
  /** Called when an update stopped with conflicts so the console can show them. */
  conflicted: () => void
  openPush: (branch: string) => void
}

/** Branch-context Update, Push and Pull-into-current, plus Update Project.
 *  Every command names its repository and branch explicitly; selection
 *  changes while Git runs do not retarget it. */
export function useBranchSync(host: BranchSyncHost) {
  const store = useWorkspace()
  const preferences = usePreferences()
  const notifications = useNotifications()
  const remotes = ref<string[]>([])
  const otherWorktreeBranches = ref<string[]>([])
  const context = new LatestRequest()
  const updateProjectOpen = ref(false)
  const updateProjectMethod = ref<UpdateMethod>(preferences.updateMethod)

  const runner = createGitCommandRunner({
    busy: host.busy,
    activeRepository: () => store.repository?.path,
    notify: (kind, title, detail) => notifications.push(kind, title, detail),
    refresh: host.refresh,
  })

  async function loadContext() {
    const path = store.repository?.path
    if (!path) {
      remotes.value = []
      otherWorktreeBranches.value = []
      return
    }
    const call = cancellableAll(api.SyncState(path), api.Worktrees(path))
    const current = context.replace(call)
    try {
      const [state, trees] = await call
      if (!current()) return
      remotes.value = state.remotes ?? []
      otherWorktreeBranches.value = (trees ?? [])
        .filter(tree => tree.branch && tree.branch !== store.repository?.head)
        .map(tree => tree.branch)
    } catch {
      if (current()) {
        remotes.value = []
        otherWorktreeBranches.value = []
      }
    } finally {
      context.finish(call)
    }
  }
  watch(() => [store.repository?.path, store.repository?.head, store.branchRefs], () => void loadContext(), { immediate: true })

  const upstreams = computed(() => new Map((store.branchSync ?? []).map(entry => [entry.branch, entry.upstream])))

  function actionsFor(item: GitRef): BranchSyncAction[] {
    const local = localBranchName(item.name)
    return branchSyncActions({
      ref: item.name,
      head: store.repository?.head ?? '',
      hasRemotes: remotes.value.length > 0,
      upstream: local ? upstreams.value.get(local) ?? '' : '',
      otherWorktreeBranches: otherWorktreeBranches.value,
      updateMethod: preferences.updateMethod,
    })
  }

  function update(branch: string, method: UpdateMethod = preferences.updateMethod) {
    const path = store.repository?.path
    if (!path) return
    return runner.run({
      title: 'Update ' + branch,
      path,
      execute: () => api.UpdateBranch(path, branch, method),
      success: describeUpdate,
      failed: cause => { if (String(cause).includes('conflicts')) host.conflicted() },
    })
  }

  function pullIntoCurrent(item: GitRef, method: UpdateMethod) {
    const path = store.repository?.path
    const branch = currentBranch(store.repository?.head ?? '')
    const source = parseRemoteRef(item.name, remotes.value)
    if (!path || !branch || !source) return
    return runner.run({
      title: `Pull into ${branch}`,
      path,
      execute: () => api.Pull(path, {
        branch, remote: source.remote, remoteBranch: source.branch,
        mode: method, noCommit: false, noVerify: false, autostash: true,
      }),
      success: describeUpdate,
      failed: cause => { if (String(cause).includes('conflicts')) host.conflicted() },
    })
  }

  function run(item: GitRef, action: BranchSyncAction['id']) {
    const local = localBranchName(item.name)
    switch (action) {
      case 'update': if (local) void update(local); break
      case 'push': if (local) host.openPush(local); break
      case 'pull-rebase': void pullIntoCurrent(item, 'rebase'); break
      case 'pull-merge': void pullIntoCurrent(item, 'merge'); break
    }
  }

  /** Update Project (Ctrl+T in IDEA): updates the checked-out branch. */
  const canUpdateProject = computed(() => !!currentBranch(store.repository?.head ?? '') && remotes.value.length > 0)

  function openUpdateProject() {
    if (host.busy.value || !canUpdateProject.value) return
    updateProjectMethod.value = preferences.updateMethod
    updateProjectOpen.value = true
  }

  async function confirmUpdateProject() {
    const branch = currentBranch(store.repository?.head ?? '')
    if (!branch || host.busy.value) return
    preferences.updateMethod = updateProjectMethod.value
    updateProjectOpen.value = false
    await update(branch, updateProjectMethod.value)
  }

  return {
    remotes, actionsFor, run, update,
    updateProjectOpen, updateProjectMethod, canUpdateProject, openUpdateProject, confirmUpdateProject,
  }
}
