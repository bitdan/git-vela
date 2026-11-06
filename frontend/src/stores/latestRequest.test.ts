import { describe, expect, it, vi } from 'vitest'
import { LatestRequest } from './latestRequest'

describe('LatestRequest', () => {
  it('cancels and invalidates the previous request when selection changes', () => {
    const first = { cancel: vi.fn() }
    const second = { cancel: vi.fn() }
    const slot = new LatestRequest()
    const isFirst = slot.replace(first)
    const isSecond = slot.replace(second)
    expect(first.cancel).toHaveBeenCalledOnce()
    expect(isFirst()).toBe(false)
    expect(isSecond()).toBe(true)
    slot.clear()
    expect(second.cancel).toHaveBeenCalledOnce()
    expect(isSecond()).toBe(false)
  })

  it('does not cancel a finished request', () => {
    const call = { cancel: vi.fn() }
    const slot = new LatestRequest()
    const isCurrent = slot.replace(call)
    slot.finish(call)
    expect(isCurrent()).toBe(false)
    slot.clear()
    expect(call.cancel).not.toHaveBeenCalled()
  })
})
