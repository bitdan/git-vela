import type { Extension } from '@codemirror/state'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { javascript } from '@codemirror/lang-javascript'
import { go } from '@codemirror/lang-go'
import { json } from '@codemirror/lang-json'

export const textHighlighting = syntaxHighlighting(HighlightStyle.define([
  { tag: [tags.keyword, tags.modifier], color: 'var(--syntax-keyword)' },
  { tag: [tags.string, tags.regexp], color: 'var(--syntax-string)' },
  { tag: [tags.number, tags.bool, tags.null, tags.atom], color: 'var(--syntax-literal)' },
  { tag: [tags.typeName, tags.className, tags.namespace], color: 'var(--syntax-type)' },
  { tag: tags.function(tags.variableName), color: 'var(--syntax-function)' },
  { tag: tags.propertyName, color: 'var(--syntax-property)' },
  { tag: [tags.comment, tags.meta], color: 'var(--muted)' },
  { tag: tags.invalid, color: 'var(--error)' },
]))

export function textLanguage(path: string): Extension {
  const ext = path.split('.').at(-1)?.toLowerCase()
  if (ext === 'go') return go()
  if (ext === 'json') return json()
  if (ext === 'ts' || ext === 'tsx' || ext === 'js' || ext === 'jsx' || ext === 'mjs' || ext === 'cjs') {
    return javascript({ typescript: ext.startsWith('ts'), jsx: ext.endsWith('x') })
  }
  return []
}
