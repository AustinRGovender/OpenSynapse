// Display formatting shared across pages.
import type { RunEvent } from '@opensynapse/api-client'

export function formatAgo(iso: string | null | undefined): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const s = Math.round((Date.now() - t) / 1000)
  if (s < 45) return 'just now'
  if (s < 3600) return `${Math.round(s / 60)} min ago`
  if (s < 86400) return `${Math.round(s / 3600)} h ago`
  if (s < 86400 * 7) return `${Math.round(s / 86400)} d ago`
  return new Date(iso).toLocaleDateString()
}

/** 125 → "2m 05s"; 3725 → "1h 02m". */
export function formatSeconds(total: number): string {
  if (!Number.isFinite(total) || total < 0) return '—'
  const s = Math.floor(total)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`
  return `${sec}s`
}

export function runDuration(started: string | null | undefined, ended: string | null | undefined, now = Date.now()): number {
  if (!started) return 0
  const end = ended ? new Date(ended).getTime() : now
  return Math.max(0, (end - new Date(started).getTime()) / 1000)
}

export function formatBytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(2)} GB`
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`
  return `${n} B`
}

export function formatNumber(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  if (n >= 1e4) return `${(n / 1e3).toFixed(1)}k`
  return Math.round(n).toLocaleString()
}

export function formatMs(n: number): string {
  if (n >= 10000) return `${(n / 1000).toFixed(1)} s`
  if (n >= 100) return `${n.toFixed(0)} ms`
  return `${n.toFixed(1)} ms`
}

/** Human description of a run event, shared by the timeline, charts and reports. */
export function eventText(e: Pick<RunEvent, 'type' | 'payload'>): string {
  const p = (e.payload ?? {}) as Record<string, unknown>
  switch (e.type) {
    case 'start':
      return `Run started (${p.vus ?? '?'} VUs, ${p.duration_seconds ?? '?'}s${p.rps ? `, ${p.rps} RPS limit` : ''})`
    case 'stop':
      return `Run ended: ${p.status ?? 'done'}${p.message ? ` — ${p.message}` : ''}`
    case 'vu_change':
      return p.schedule === 'resumed' ? 'VUs handed back to the load profile' : `VUs set to ${p.vus}`
    case 'rps_change':
      return Number(p.rps) > 0 ? `RPS limit set to ${p.rps}` : 'RPS limit removed'
    case 'duration_change':
      return `Duration set to ${formatSeconds(Number(p.duration_seconds))}`
    case 'pause':
      return 'Paused'
    case 'resume':
      return 'Resumed'
    case 'stop_requested':
      return 'Stop requested'
    case 'kill':
      return 'Killed'
    case 'error':
      return `Error: ${p.message ?? 'unknown'}`
    case 'user_note':
      return `Note: ${p.message ?? ''}`
    default:
      return e.type
  }
}

export function eventLevel(type: string): 'info' | 'warn' | 'error' {
  if (type === 'error') return 'error'
  if (type === 'kill' || type === 'stop_requested' || type === 'pause') return 'warn'
  return 'info'
}
