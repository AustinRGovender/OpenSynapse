import { describe, expect, it } from 'vitest'
import { parseDurationToSeconds } from './duration'
import { formatBytes, formatMs, formatSeconds } from './format'
import { clampInt } from './number'
import { describeProfile } from './profile'

describe('parseDurationToSeconds', () => {
  it.each([
    ['30s', 30],
    ['2m', 120],
    ['1h30m', 5400],
    ['1m30s', 90],
    ['500ms', 0.5],
    ['45', 45],
    ['', 0],
    [undefined, 0],
  ])('%s → %d', (input, want) => {
    expect(parseDurationToSeconds(input)).toBeCloseTo(want)
  })
})

describe('describeProfile', () => {
  it('summarises ramping stages with the peak and total time', () => {
    const p = describeProfile({ executor: 'ramping-vus', start_vus: 2, stages: [{ duration: '30s', target: 10 }, { duration: '1m', target: 25 }, { duration: '30s', target: 0 }] })
    expect(p.vuBased).toBe(true)
    expect(p.text).toMatch(/3 stages up to 25 VUs over 2m/)
  })

  it('treats arrival-rate executors as not VU-based', () => {
    expect(describeProfile({ executor: 'constant-arrival-rate', rate: 50, duration: '1m' }).vuBased).toBe(false)
  })

  it('handles a plan without a scenario', () => {
    expect(describeProfile(null).text).toMatch(/no scenario/)
  })
})

describe('formatters', () => {
  it('formats durations, bytes and milliseconds for people', () => {
    expect(formatSeconds(125)).toBe('2m 05s')
    expect(formatBytes(2048)).toMatch(/2(\.0)?\s*KB/)
    expect(formatMs(0.42)).toMatch(/ms$/)
  })
})

describe('clampInt', () => {
  it('keeps typed numbers in range and falls back when empty', () => {
    expect(clampInt('12', 1, 50, 5)).toBe(12)
    expect(clampInt('500', 1, 50, 5)).toBe(50)
    expect(clampInt('0', 1, 50, 5)).toBe(1)
    expect(clampInt('', 1, 50, 5)).toBe(5)
    expect(clampInt('abc', 1, 50, 5)).toBe(5)
  })
})
