import { useState, type FormEvent } from 'react'
import type { CheckStat, EndpointStat, RunError, RunEvent } from '@opensynapse/api-client'
import { eventLevel, eventText, formatMs, formatNumber } from '../../utils/format'
import { Button, EmptyState } from '../ui/primitives'

const th = 'px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wide text-slate-500'
const td = 'px-3 py-1.5 text-xs text-slate-300'
const num = 'text-right tabular-nums'

/** Per-endpoint breakdown (F-06). */
export function EndpointTable({ endpoints }: { endpoints: EndpointStat[] }) {
  if (endpoints.length === 0) return <EmptyState title="No requests recorded" />
  return (
    <div className="overflow-x-auto rounded-lg border border-slate-800">
      <table className="w-full">
        <thead className="bg-slate-900">
          <tr>
            <th className={th}>Endpoint</th>
            <th className={`${th} ${num}`}>Requests</th>
            <th className={`${th} ${num}`}>RPS</th>
            <th className={`${th} ${num}`}>Errors</th>
            <th className={`${th} ${num}`}>Avg</th>
            <th className={`${th} ${num}`}>p95</th>
            <th className={`${th} ${num}`}>p99</th>
            <th className={`${th} ${num}`}>Max</th>
          </tr>
        </thead>
        <tbody>
          {endpoints.map((e) => (
            <tr key={`${e.method} ${e.name}`} className="border-t border-slate-800">
              <td className={`${td} max-w-md truncate font-mono`} title={`${e.method} ${e.name}`}>
                <span className="mr-2 text-teal-300">{e.method}</span>
                {/* Request nodes are usually named "GET /path"; don't repeat the method. */}
                {e.method && e.name.startsWith(`${e.method} `) ? e.name.slice(e.method.length + 1) : e.name}
              </td>
              <td className={`${td} ${num}`}>{formatNumber(e.requests)}</td>
              <td className={`${td} ${num}`}>{e.rps.toFixed(1)}</td>
              <td className={`${td} ${num} ${e.failed > 0 ? 'text-red-400' : ''}`}>{e.error_rate.toFixed(2)}%</td>
              <td className={`${td} ${num}`}>{formatMs(e.avg_ms)}</td>
              <td className={`${td} ${num}`}>{formatMs(e.p95_ms)}</td>
              <td className={`${td} ${num}`}>{formatMs(e.p99_ms)}</td>
              <td className={`${td} ${num}`}>{formatMs(e.max_ms)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** Assertion (check) results. */
export function ChecksTable({ checks }: { checks: CheckStat[] }) {
  if (checks.length === 0) return <EmptyState title="This plan has no assertions">Add an Assertion node under a request to check its status, body or response time.</EmptyState>
  return (
    <div className="overflow-x-auto rounded-lg border border-slate-800">
      <table className="w-full">
        <thead className="bg-slate-900">
          <tr>
            <th className={th}>Assertion</th>
            <th className={th}>Group</th>
            <th className={`${th} ${num}`}>Passed</th>
            <th className={`${th} ${num}`}>Failed</th>
            <th className={`${th} ${num}`}>Pass rate</th>
          </tr>
        </thead>
        <tbody>
          {checks.map((c) => {
            const total = c.passes + c.fails
            const rate = total ? (c.passes / total) * 100 : 0
            return (
              <tr key={`${c.group}/${c.name}`} className="border-t border-slate-800">
                <td className={td}>
                  <span className={`mr-2 ${c.fails > 0 ? 'text-red-400' : 'text-emerald-400'}`}>{c.fails > 0 ? '✗' : '✓'}</span>
                  {c.name}
                </td>
                <td className={`${td} text-slate-500`}>{c.group.replace(/^::/, '') || '—'}</td>
                <td className={`${td} ${num}`}>{formatNumber(c.passes)}</td>
                <td className={`${td} ${num} ${c.fails > 0 ? 'text-red-400' : ''}`}>{formatNumber(c.fails)}</td>
                <td className={`${td} ${num}`}>{rate.toFixed(1)}%</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/** Most recent failed requests, newest first (F-06 error log). */
export function ErrorLog({ errors }: { errors: RunError[] }) {
  if (errors.length === 0) return <EmptyState title="No failed requests" />
  const recent = [...errors].reverse().slice(0, 50)
  return (
    <div className="max-h-80 overflow-auto rounded-lg border border-slate-800">
      <table className="w-full">
        <thead className="sticky top-0 bg-slate-900">
          <tr>
            <th className={th}>Time</th>
            <th className={th}>Status</th>
            <th className={th}>Request</th>
            <th className={th}>Detail</th>
          </tr>
        </thead>
        <tbody>
          {recent.map((e, i) => (
            <tr key={`${e.timestamp_ms}-${i}`} className="border-t border-slate-800">
              <td className={`${td} whitespace-nowrap font-mono text-slate-500`}>{new Date(e.timestamp_ms).toLocaleTimeString()}</td>
              <td className={`${td} font-mono text-red-400`}>{e.status || e.error_code || '—'}</td>
              <td className={`${td} max-w-sm truncate font-mono`} title={e.url}>{e.method} {e.url}</td>
              <td className={`${td} text-slate-400`}>{e.message || (e.status >= 500 ? 'Server error' : e.status >= 400 ? 'Client error' : 'Request failed')}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** Run timeline with the option to add notes. */
export function Timeline({ events, onNote, canNote }: { events: RunEvent[]; onNote: (m: string) => Promise<void>; canNote: boolean }) {
  const [note, setNote] = useState('')
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!note.trim()) return
    await onNote(note.trim())
    setNote('')
  }
  const tone = { info: 'text-slate-300', warn: 'text-amber-300', error: 'text-red-400' }
  return (
    <div className="space-y-3">
      {events.length === 0 ? (
        <EmptyState title="No events yet" />
      ) : (
        <ol className="max-h-80 overflow-auto rounded-lg border border-slate-800">
          {events.map((e) => (
            <li key={e.id} className="flex gap-3 border-b border-slate-800 px-3 py-1.5 text-xs last:border-0">
              <span className="whitespace-nowrap font-mono text-slate-500">{new Date(e.timestamp_ms).toLocaleTimeString()}</span>
              <span className={tone[eventLevel(e.type)]}>{eventText(e)}</span>
            </li>
          ))}
        </ol>
      )}
      {canNote && (
        <form onSubmit={submit} className="flex gap-2">
          <input value={note} onChange={(e) => setNote(e.target.value)} placeholder="Add a note to the timeline (e.g. “deployed v2.3”)" className="flex-1 rounded-md border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-sm text-slate-100 placeholder-slate-500" />
          <Button type="submit" disabled={!note.trim()}>Add note</Button>
        </form>
      )}
    </div>
  )
}
