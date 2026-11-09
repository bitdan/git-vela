import { computed, ref, type Ref } from 'vue'
import { RepositoryService as api } from '../../../bindings/gitvela/internal/app'
import type { BranchDeletion } from '../../../bindings/gitvela/internal/git'
import { useNotifications } from '../../shared/notifications'
import { useWorkspace } from '../../stores/workspace'
import { createGitCommandRunner, errorText } from '../../shared/gitCommand'

export type BranchForm = 'create' | 'rename' | 'delete'
export type TagForm = 'create' | 'delete' | 'push'
export type BranchChange = 'create' | 'switch' | 'rename' | 'delete'

export interface RefOperationsHost {
  busy: Ref<boolean>
  /** Reload refs and history after a ref changed without moving HEAD. */
  refreshRefs: (path: string) => Promise<void>
  /** Reset editor/preview state and reload after HEAD moved (checkout). */
  afterHeadMoved: (path: string) => Promise<void>
  /** A branch was created, renamed or deleted. */
  branchChanged?: () => void
}

/** Branch and tag operations of the Branches panel: forms, confirmations and
 *  their inline feedback. Writes go through the shared Git command runner so
 *  they are exclusive and never land on a repository the user switched to. */
export function useRefOperations(host: RefOperationsHost) {
  const store = useWorkspace()
  const branchError = ref('')
  const branchNotice = ref('')
  const branchName = ref('')
  const branchForm = ref<BranchForm>()
  const managedBranch = ref('')
  const branchBase = ref('')
  const createBase = computed(() => branchBase.value || store.selected?.hash)
  const tagForm = ref<TagForm>()
  const tagName = ref('')
  const tagMessage = ref('')
  const tagRemote = ref('')
  const tagRemotes = ref<string[]>([])
  const tagBase = computed(() => store.selected?.hash ?? store.headHash)
  /** What deleting managedBranch would discard (IDEA "not fully merged"). */
  const deletion = ref<BranchDeletion>()
  const deletionError = ref('')
  const notifications = useNotifications()

  const runner = createGitCommandRunner({
    busy: host.busy,
    activeRepository: () => store.repository?.path,
    notify: (kind, _title, detail) => {
      if (kind === 'error') branchError.value = detail
      else branchNotice.value = detail
    },
    refresh: host.refreshRefs,
  })

  function clearFeedback() {
    branchError.value = ''
    branchNotice.value = ''
  }

  function openBranchForm(form?: BranchForm, branch = '', base = '') {
    branchBase.value = base
    branchForm.value = form
    managedBranch.value = branch
    branchName.value = form === 'rename' ? branch : ''
    clearFeedback()
    tagForm.value = undefined
    deletion.value = undefined
    deletionError.value = ''
    if (form === 'delete' && branch) void inspectDeletion(branch)
  }

  async function inspectDeletion(branch: string) {
    const path = store.repository?.path
    if (!path) return
    try {
      const result = await api.InspectBranchDeletion(path, branch)
      if (path === store.repository?.path && branchForm.value === 'delete' && managedBranch.value === branch) deletion.value = result
    } catch (cause) {
      if (path === store.repository?.path && managedBranch.value === branch) deletionError.value = errorText(cause)
    }
  }

  /** Deletes the reviewed branch. An unmerged branch is force-deleted only
   *  after the dialog showed its unmerged commits; the backend re-checks the
   *  tip so a branch that moved meanwhile is not lost. */
  function deleteBranch() {
    const path = store.repository?.path
    const info = deletion.value
    if (!path || !info || info.branch !== managedBranch.value) return
    const unmerged = info.unmerged > 0
    let deleted = false
    return command('Delete branch', () => api.DeleteBranch(path, info.branch, info.tip, unmerged), 'Deleted ' + info.branch, {
      applied: () => {
        deleted = true
        openBranchForm()
        host.branchChanged?.()
      },
    }).then(() => {
      // Offer Restore only once the command released the busy flag, so the
      // action can run as soon as it is visible.
      if (!deleted || path !== store.repository?.path) return
      notifications.push('info', 'Deleted branch ' + info.branch,
        unmerged ? `${info.unmerged} unmerged commit${info.unmerged === 1 ? ' was' : 's were'} discarded.` : 'The branch was fully merged.',
        [{ label: 'Restore', run: () => void restoreBranch(path, info.branch, info.tip) }])
    })
  }

  function restoreBranch(path: string, branch: string, tip: string) {
    if (path !== store.repository?.path) return
    return command('Restore branch', () => api.CreateBranch(path, branch, tip), 'Restored ' + branch)
  }

  function openTagForm(form?: TagForm, name = '') {
    branchForm.value = undefined
    tagForm.value = form
    tagName.value = name
    tagMessage.value = ''
    clearFeedback()
  }

  function reset() {
    openBranchForm()
    openTagForm()
    branchName.value = ''
  }

  /** Runs a ref command; `moves` marks checkouts that move HEAD on success. */
  function command(title: string, execute: () => Promise<unknown>, success: string, options: { moves?: boolean; applied?: () => void } = {}) {
    const path = store.repository?.path
    if (!path || host.busy.value) return Promise.resolve(undefined)
    clearFeedback()
    let applied = false
    return runner.run({
      title, path, execute,
      success: () => success,
      applied: () => {
        applied = true
        options.applied?.()
      },
      refresh: async repo => {
        if (!applied) return
        await (options.moves ? host.afterHeadMoved(repo) : host.refreshRefs(repo))
      },
    })
  }

  function changeBranch(operation: BranchChange, name: string) {
    const path = store.repository?.path
    const base = createBase.value
    const original = managedBranch.value
    if (!path || !name || (operation === 'create' && !base)) return
    const formDone = () => {
      openBranchForm()
      host.branchChanged?.()
    }
    switch (operation) {
      case 'create':
        return command('New branch', () => api.CreateBranch(path, name, base!), 'Created ' + name, { applied: formDone })
      case 'switch':
        return command('Checkout', () => api.SwitchBranch(path, name), 'Switched to ' + name, { moves: true })
      case 'rename':
        return command('Rename branch', () => api.RenameBranch(path, original, name), 'Renamed to ' + name, { applied: formDone })
      case 'delete':
        return deleteBranch()
    }
  }

  function createTag() {
    const path = store.repository?.path
    const name = tagName.value.trim()
    const base = tagBase.value
    if (!path || !name || !base) return
    return command('New tag', () => api.CreateTag(path, name, base, tagMessage.value.trim()), 'Created tag ' + name,
      { applied: () => openTagForm() })
  }

  function deleteTag(name: string) {
    const path = store.repository?.path
    if (!path || !name) return
    return command('Delete tag', () => api.DeleteTag(path, name), 'Deleted tag ' + name, { applied: () => openTagForm() })
  }

  function pushTag(name: string) {
    const path = store.repository?.path
    const remote = tagRemote.value
    if (!path || !name || !remote) return
    return command('Push tag', () => api.PushTag(path, remote, name), 'Pushed tag ' + name + ' to ' + remote,
      { applied: () => openTagForm() })
  }

  function checkoutTag(name: string) {
    const path = store.repository?.path
    if (!path || !name) return
    return command('Checkout tag', () => api.CheckoutTag(path, name), 'Checked out tag ' + name, { moves: true })
  }

  async function openTagPush(name: string) {
    const path = store.repository?.path
    if (!path || store.opening) return
    try {
      const state = await api.SyncState(path)
      if (path !== store.repository?.path) return
      const remotes = state.remotes ?? []
      if (!remotes.length) {
        branchError.value = 'Configure a Git remote before pushing tags.'
        return
      }
      tagRemotes.value = remotes
      tagRemote.value = remotes.includes('origin') ? 'origin' : remotes[0]
      openTagForm('push', name)
    } catch (cause) {
      if (path === store.repository?.path) branchError.value = errorText(cause)
    }
  }

  return {
    branchError, branchNotice, branchName, branchForm, managedBranch, branchBase, createBase, deletion, deletionError,
    tagForm, tagName, tagMessage, tagRemote, tagRemotes, tagBase,
    openBranchForm, openTagForm, reset, changeBranch, createTag, deleteTag, pushTag, checkoutTag, openTagPush,
  }
}
