import { describe, expect, it } from 'vitest'
import { parseDurationToSeconds } from './duration'
import { formatBytes, formatMs, formatSeconds } from './format'
import { clampInt } from './number'
import { describeProfile } from './profile'
import { requiredVariables } from './variables'

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

  it('uses the singular for one', () => {
    expect(describeProfile({ executor: 'constant-vus', vus: 1, duration: '30s' }).text).toBe('1 VU for 30s')
    expect(describeProfile({ executor: 'constant-arrival-rate', rate: 1, time_unit: '1s', duration: '1h' }).text).toBe('1 iteration per 1s for 1h')
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

describe('requiredVariables', () => {
  it('lists variables used by enabled nodes that the plan does not set itself', () => {
    const node = (type: string, properties: object, children: object[] = [], enabled = true) => ({ id: type, type, name: type, enabled, properties, children })
    const root = node('plan', {}, [
      node('scenario', {}, [
        node('http', { url: '${BASE_URL}/login', headers: [{ key: 'Authorization', value: 'Bearer ${TOKEN}' }] }),
        node('code-block', { code: "__vars.TOKEN = res.json('token'); __vars['USER'] = 'x'" }),
        node('http', { url: '${OTHER}/x' }, [], false),
        node('http', { url: '${USER}/${BASE_URL}' }),
      ]),
    ])
    expect(requiredVariables(root as never)).toEqual(['BASE_URL'])
  })
})
