import type { Charger } from '../types/api'

function sortableUTC(value: string): string {
  const match = /^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z$/.exec(value)
  if (!match) throw new Error('Invalid UTC timestamp from server.')
  return `${match[1]}.${(match[2] ?? '').padEnd(9, '0')}`
}

export function compareTimestamp(left: string, right: string): number {
  return sortableUTC(left).localeCompare(sortableUTC(right))
}

export function mergeChargers(base: Charger[], updates: Charger[]): Charger[] {
  const merged = new Map(base.map((charger) => [charger.id, charger]))
  for (const charger of updates) {
    const previous = merged.get(charger.id)
    if (!previous || compareTimestamp(charger.updated_at, previous.updated_at) > 0) merged.set(charger.id, charger)
  }
  return [...merged.values()].sort((a, b) => a.id.localeCompare(b.id))
}

export function localToUtc(value: string): string {
  if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d$/.test(value)) throw new Error('Choose a valid date and time.')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) throw new Error('Choose a valid date and time.')
  return date.toISOString()
}
