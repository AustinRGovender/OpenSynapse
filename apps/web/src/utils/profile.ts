import { parseDurationToSeconds } from './duration'
import { formatSeconds } from './format'

export type Stage = { duration: string; target: number }

const count = (n: unknown, one: string, many: string) => `${n} ${Number(n) === 1 ? one : many}`

/** Describes a scenario's load profile in plain words. */
export function describeProfile(props: Record<string, unknown> | null): { text: string; vuBased: boolean; stages?: Stage[] } {
  if (!props) return { text: '1 VU for 30s (no scenario in this plan)', vuBased: true }
  const ex = (props.executor as string) || 'constant-vus'
  if (ex === 'ramping-vus') {
    const stages = (props.stages as Stage[]) ?? []
    const peak = Math.max(Number(props.start_vus ?? 0), ...stages.map((s) => s.target))
    const total = stages.reduce((s, x) => s + parseDurationToSeconds(x.duration), 0)
    return { text: `Ramping in ${stages.length} stage${stages.length === 1 ? '' : 's'} up to ${count(peak, 'VU', 'VUs')} over ${formatSeconds(total)}`, vuBased: true, stages }
  }
  if (ex === 'constant-vus' || ex === 'externally-controlled') return { text: `${count(props.vus ?? 1, 'VU', 'VUs')} for ${props.duration ?? '30s'}`, vuBased: true }
  if (ex === 'constant-arrival-rate') return { text: `${count(props.rate, 'iteration', 'iterations')} per ${props.time_unit ?? '1s'} for ${props.duration}`, vuBased: false }
  if (ex === 'ramping-arrival-rate') return { text: 'Ramping arrival rate', vuBased: false }
  return { text: `${count(props.iterations ?? '?', 'iteration', 'iterations')} across ${count(props.vus ?? 1, 'VU', 'VUs')}`, vuBased: false }
}
