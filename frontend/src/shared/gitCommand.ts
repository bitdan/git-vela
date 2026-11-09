// A single entry for Git write commands started from menus, toolbars and the
// command palette. It owns the cross-cutting rules every write needs:
//  - the target repository is passed explicitly and re-checked after the
//    command, so a result never lands on a repository the user switched to;
//  - a second click while Git is running is ignored instead of queued;
//  - success and failure are reported through one notification channel and
//    the workspace is refreshed for the repository the command ran against.

export interface BusyFlag {
  value: boolean
}

export type NotificationKind = 'info' | 'error'

export interface GitCommandHost {
  busy: BusyFlag
  /** Path of the repository currently shown, or undefined. */
  activeRepository: () => string | undefined
  notify: (kind: NotificationKind, title: string, detail: string) => void
  refresh: (path: string) => Promise<void>
}

export interface GitCommand<T> {
  title: string
  /** Repository the command targets; never derived after the fact. */
  path: string
  execute: () => Promise<T>
  success: (result: T) => string
  /** Some failures (conflicts) leave state the console should show. */
  failed?: (cause: unknown) => void
}

export function createGitCommandRunner(host: GitCommandHost) {
  async function run<T>(command: GitCommand<T>): Promise<T | undefined> {
    if (host.busy.value || !command.path || host.activeRepository() !== command.path) return undefined
    host.busy.value = true
    let result: T | undefined
    try {
      result = await command.execute()
      if (host.activeRepository() === command.path) host.notify('info', command.title, command.success(result))
    } catch (cause) {
      if (host.activeRepository() === command.path) {
        host.notify('error', command.title + ' failed', errorText(cause))
        command.failed?.(cause)
      }
    } finally {
      try {
        if (host.activeRepository() === command.path) await host.refresh(command.path)
      } finally {
        host.busy.value = false
      }
    }
    return result
  }
  return { run }
}

export function errorText(cause: unknown) {
  if (cause instanceof Error) return cause.message
  return String(cause)
}
