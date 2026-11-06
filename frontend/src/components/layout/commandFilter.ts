// Pure ranking used by the command palette. Kept separate from the component
// so its matching behaviour is covered by unit tests.

export interface PaletteCommand {
  id: string
  label: string
  group: string
  hint?: string
  keywords?: string
  disabled?: boolean
  run: () => void
}

function subsequence(needle: string, haystack: string): boolean {
  let position = 0
  for (const character of haystack) {
    if (character === needle[position]) position++
    if (position === needle.length) return true
  }
  return false
}

export function scoreCommand(command: PaletteCommand, query: string): number {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return command.disabled ? 1 : 10
  const label = command.label.toLocaleLowerCase()
  const words = needle.split(/\s+/)
  const haystack = [label, command.group.toLocaleLowerCase(), (command.keywords ?? '').toLocaleLowerCase()].join(' ')
  const direct = words.every(word => haystack.includes(word))
  const fuzzy = subsequence(needle.replace(/\s+/g, ''), label.replace(/[^a-z0-9]/gi, ''))
  if (!direct && !fuzzy) return -1
  if (label === needle) return 120
  if (label.startsWith(needle)) return 100
  if (direct && words.every(word => label.split(/[^a-z0-9]+/i).some(part => part.startsWith(word)))) return 85
  if (label.includes(needle)) return 70
  if (fuzzy) return 50
  return 25
}

export function filterCommands(commands: PaletteCommand[], query: string, limit = 60): PaletteCommand[] {
  return commands
    .map((command, index) => ({ command, index, score: scoreCommand(command, query) }))
    .filter(entry => entry.score >= 0)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .slice(0, limit)
    .map(entry => entry.command)
}
