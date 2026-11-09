import { describe, expect, it } from 'vitest'
import { commitHistoryLimit, pushCommitMessage } from './commitHistory'

describe('pushCommitMessage', () => {
  it('moves a repeated message to the front without duplication', () => {
    const history = pushCommitMessage(['a', 'b', 'c'], 'b')
    expect(history).toEqual(['b', 'a', 'c'])
  })

  it('caps the history at the configured limit', () => {
    let history: string[] = []
    for (let index = 0; index < commitHistoryLimit + 5; index++) {
      history = pushCommitMessage(history, 'message ' + index)
    }
    expect(history).toHaveLength(commitHistoryLimit)
    expect(history[0]).toBe('message 24')
    expect(history.at(-1)).toBe('message 5')
  })

  it('keeps a custom limit and trims surrounding whitespace', () => {
    const history = pushCommitMessage(['b', 'c'], '  a  ', 2)
    expect(history).toEqual(['a', 'b'])
    expect(pushCommitMessage(['b'], '   ', 2)).toEqual(['b'])
  })

  it('returns the original array untouched for empty input', () => {
    const history = ['a', 'b']
    expect(pushCommitMessage(history, '')).toBe(history)
  })
})
