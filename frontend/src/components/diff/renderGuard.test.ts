import { describe, expect, it } from 'vitest'
import { exceedsRenderLimit, maxRenderBytes, maxRenderLines } from './renderGuard'

describe('expanded diff render guard', () => {
  it('allows diffs inside the render budget', () => {
    expect(exceedsRenderLimit('a\nb\n', 'b\nc\nd\n')).toBe(false)
    expect(exceedsRenderLimit('x\n'.repeat(maxRenderLines), '')).toBe(false)
    expect(exceedsRenderLimit('', 'y'.repeat(maxRenderBytes))).toBe(false)
  })

  it('rejects sides that exceed the line or size budget', () => {
    expect(exceedsRenderLimit('x\n'.repeat(maxRenderLines + 1), '')).toBe(true)
    expect(exceedsRenderLimit('', 'x\n'.repeat(maxRenderLines + 1))).toBe(true)
    expect(exceedsRenderLimit('a'.repeat(maxRenderBytes + 1), '')).toBe(true)
    expect(exceedsRenderLimit('', 'b'.repeat(maxRenderBytes + 1))).toBe(true)
  })

  it('treats empty sides as zero lines', () => {
    expect(exceedsRenderLimit('', '')).toBe(false)
  })
})
