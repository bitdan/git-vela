// Issue navigation (IDEA IssueNavigationConfiguration counterpart): commit
// message text is split so that issue IDs matching the configured pattern
// become links built from the configured URL template. $0 is the whole match
// and $1..$9 are regex groups; a message without a configured pattern or
// template renders as plain text.

export interface IssueSegment {
  text: string
  href?: string
}

function placeholder(template: string, match: RegExpMatchArray): string {
  return template.replace(/\$(\d)/g, (_, group: string) => match[Number(group)] ?? match[0])
}

/** Pure splitter: always returns at least one segment covering all of `text`. */
export function issueSegments(text: string, pattern: string, urlTemplate: string): IssueSegment[] {
  if (!text) return []
  if (!pattern.trim() || !urlTemplate.trim()) return [{ text }]
  let regex: RegExp
  try {
    regex = new RegExp(pattern, 'g')
  } catch {
    return [{ text }]
  }
  const segments: IssueSegment[] = []
  let last = 0
  for (const match of text.matchAll(regex)) {
    const id = match[0]
    if (!id) continue
    const at = match.index ?? 0
    if (at < last) continue
    if (at > last) segments.push({ text: text.slice(last, at) })
    segments.push({ text: id, href: placeholder(urlTemplate, match) })
    last = at + id.length
  }
  if (last < text.length) segments.push({ text: text.slice(last) })
  return segments.length ? segments : [{ text }]
}
