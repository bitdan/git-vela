import { describe, expect, it } from 'vitest'
import { issueSegments } from './issueLinks'

describe('issueSegments', () => {
  it('passes plain text through when navigation is not configured', () => {
    expect(issueSegments('fix ABC-1 bug', '', 'https://x/$1')).toEqual([{ text: 'fix ABC-1 bug' }])
    expect(issueSegments('fix ABC-1 bug', '[A-Z]+-\\d+', '')).toEqual([{ text: 'fix ABC-1 bug' }])
  })

  it('links issue IDs with the template placeholder', () => {
    expect(issueSegments('fix ABC-1 now', '[A-Z]+-\\d+', 'https://youtrack/issue/$1')).toEqual([
      { text: 'fix ' },
      { text: 'ABC-1', href: 'https://youtrack/issue/ABC-1' },
      { text: ' now' },
    ])
  })

  it('exposes regex groups as $1..$9 and the whole match as $0', () => {
    expect(issueSegments('#42', '#(\\d+)', 'https://github/repo/issues/$1')).toEqual([
      { text: '#42', href: 'https://github/repo/issues/42' },
    ])
    expect(issueSegments('ABC-1', '[A-Z]+-\\d+', 'https://x/?id=$0')).toEqual([
      { text: 'ABC-1', href: 'https://x/?id=ABC-1' },
    ])
  })

  it('handles multiple matches and invalid patterns', () => {
    const text = 'a ABC-1 b DEF-2 c'
    const segments = issueSegments(text, '[A-Z]+-\\d+', 'https://x/$1')
    expect(segments).toHaveLength(5)
    expect(segments.map(segment => segment.text).join('')).toBe(text)
    expect(segments.filter(segment => segment.href)).toHaveLength(2)
    expect(issueSegments('ABC-1', '([unclosed', 'https://x/$1')).toEqual([{ text: 'ABC-1' }])
  })

  it('returns a single segment for empty text', () => {
    expect(issueSegments('', '[A-Z]+-\\d+', 'https://x/$1')).toEqual([])
  })
})
