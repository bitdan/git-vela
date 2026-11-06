import { describe, expect, it } from 'vitest'
import { filterCommands, scoreCommand, type PaletteCommand } from './commandFilter'

function command(id: string, label: string, group = 'View', extra: Partial<PaletteCommand> = {}): PaletteCommand {
  return { id, label, group, run: () => {}, ...extra }
}

const commands = [
  command('refresh', 'Refresh repository', 'Repository', { keywords: 'reload' }),
  command('commit', 'Commit changes…', 'Git operations'),
  command('toggle', 'Toggle dark theme', 'View'),
  command('jump', 'Jump to feature/app', 'Branches & tags', { keywords: 'refs/heads/feature/app' }),
  command('disabled', 'Push…', 'Git operations', { disabled: true }),
]

describe('command ranking', () => {
  it('prefers exact and prefix matches over loose matches', () => {
    expect(scoreCommand(commands[0], 'refresh')).toBeGreaterThan(scoreCommand(commands[0], 'reload'))
    expect(scoreCommand(commands[1], 'commit')).toBeGreaterThan(scoreCommand(commands[1], 'changes'))
  })

  it('matches keywords, groups and subsequences but not unrelated text', () => {
    expect(scoreCommand(commands[0], 'reload')).toBeGreaterThanOrEqual(0)
    expect(scoreCommand(commands[3], 'feature/app')).toBeGreaterThan(0)
    expect(scoreCommand(commands[2], 'tgdt')).toBeGreaterThan(0)
    expect(scoreCommand(commands[2], 'stash')).toBe(-1)
  })

  it('requires every word of a multi-word query', () => {
    expect(scoreCommand(commands[0], 'refresh missing')).toBe(-1)
    expect(scoreCommand(commands[0], 'refresh repository')).toBeGreaterThan(0)
  })
})

describe('command filtering', () => {
  it('keeps insertion order for an empty query and honours the limit', () => {
    expect(filterCommands(commands, '').map(item => item.id)).toEqual(['refresh', 'commit', 'toggle', 'jump', 'disabled'])
    expect(filterCommands(commands, '', 2)).toHaveLength(2)
  })

  it('puts the best match first and excludes non-matches', () => {
    const result = filterCommands(commands, 'toggle')
    expect(result.map(item => item.id)).toEqual(['toggle'])
  })

  it('still returns disabled entries so the UI can explain why', () => {
    expect(filterCommands(commands, 'push').map(item => item.id)).toEqual(['disabled'])
  })
})
