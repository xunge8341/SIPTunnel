import { describe, expect, it } from 'vitest'
import { formatDateTimeText } from '../date'

describe('formatDateTimeText', () => {
  it('treats unified timestamp as local wall time', () => {
    expect(formatDateTimeText('2026-03-18 12:53:16.672')).toContain('2026-03-18 12:53:16.672')
  })

  it('still converts RFC3339 UTC timestamp into local display text', () => {
    const out = formatDateTimeText('2026-03-18T04:53:16.672Z')
    expect(out).toMatch(/^2026-03-18 /)
  })
})
