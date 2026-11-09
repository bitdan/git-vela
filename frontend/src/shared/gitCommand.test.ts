import { describe, expect, it, vi } from 'vitest'
import { createGitCommandRunner, type GitCommandHost } from './gitCommand'

function host(active = '/repo') {
  const state = { active }
  const value: GitCommandHost & { notes: string[] } = {
    busy: { value: false },
    activeRepository: () => state.active,
    notes: [],
    notify(kind, title, detail) { value.notes.push(`${kind}:${title}:${detail}`) },
    refresh: vi.fn(async () => {}),
  }
  return { value, state }
}

describe('git command runner', () => {
  it('runs once, reports success and refreshes the target repository', async () => {
    const { value } = host()
    const runner = createGitCommandRunner(value)
    const execute = vi.fn(async () => 'ok')
    const first = runner.run({ title: 'Update', path: '/repo', execute, success: result => result })
    const second = await runner.run({ title: 'Update', path: '/repo', execute, success: result => result })
    expect(second).toBeUndefined()
    expect(await first).toBe('ok')
    expect(execute).toHaveBeenCalledTimes(1)
    expect(value.notes).toEqual(['info:Update:ok'])
    expect(value.refresh).toHaveBeenCalledWith('/repo')
    expect(value.busy.value).toBe(false)
  })

  it('refuses a command for a repository that is not shown', async () => {
    const { value } = host('/other')
    const execute = vi.fn(async () => 1)
    await createGitCommandRunner(value).run({ title: 'Push', path: '/repo', execute, success: () => '' })
    expect(execute).not.toHaveBeenCalled()
  })

  it('drops results after the user switches repositories', async () => {
    const { value, state } = host()
    let release!: () => void
    const pending = createGitCommandRunner(value).run({
      title: 'Pull', path: '/repo', success: () => 'done',
      execute: () => new Promise<void>(resolve => { release = resolve }),
    })
    state.active = '/next'
    release()
    await pending
    expect(value.notes).toEqual([])
    expect(value.refresh).not.toHaveBeenCalled()
    expect(value.busy.value).toBe(false)
  })

  it('reports failures and lets the caller react', async () => {
    const { value } = host()
    const failed = vi.fn()
    await createGitCommandRunner(value).run({
      title: 'Update', path: '/repo', success: () => '', failed,
      execute: async () => { throw new Error('conflicts') },
    })
    expect(value.notes).toEqual(['error:Update failed:conflicts'])
    expect(failed).toHaveBeenCalled()
    expect(value.busy.value).toBe(false)
  })
})
