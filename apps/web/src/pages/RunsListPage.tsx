import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Plan, Run } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'
import { Button, EmptyState, PageHeader, Select, Spinner, StatusBadge, TextInput } from '../components/ui/primitives'
import { useToast } from '../stores/ui-store'
import { formatMs, formatNumber, formatSeconds, runDuration } from '../utils/format'
import { navigate } from '../utils/navigate'

type SortKey = 'started' | 'plan' | 'duration' | 'status' | 'vus' | 'p95' | 'errors'

const COLUMNS: [SortKey, string, boolean][] = [
  ['plan', 'Plan', false],
  ['started', 'Started', false],
  ['duration', 'Duration', true],
  ['status', 'Status', false],
  ['vus', 'Peak VUs', true],
  ['p95', 'p95', true],
  ['errors', 'Error rate', true],
]

function sortValue(r: Run, key: SortKey): number | string {
  switch (key) {
    case 'plan':
      return r.plan_name.toLowerCase()
    case 'started':
      return r.started_at ?? r.created_at
    case 'duration':
      return runDuration(r.started_at, r.ended_at)
    case 'status':
      return r.status
    case 'vus':
      return r.summary?.peak_vus ?? 0
    case 'p95':
      return r.summary?.p95_ms ?? 0
    case 'errors':
      return r.summary?.error_rate ?? 0
  }
}

export function RunsListPage() {
  const toast = useToast()
  const [runs, setRuns] = useState<Run[] | null>(null)
  const [cursor, setCursor] = useState<string | null>(null)
  const [plans, setPlans] = useState<Plan[]>([])
  const [status, setStatus] = useState('all')
  const [planId, setPlanId] = useState('')
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: 'started', desc: true })
  const [selected, setSelected] = useState<string[]>([])

  const query = useMemo(() => {
    const q: Record<string, string> = { limit: '50' }
    if (status !== 'all') q.status = status
    if (planId) q.plan_id = planId
    if (since) q.since = new Date(since).toISOString()
    if (until) q.until = new Date(`${until}T23:59:59`).toISOString()
    return q
  }, [status, planId, since, until])

  const load = useCallback(
    (more?: string) =>
      unwrap(api.GET('/api/v1/runs', { params: { query: more ? { ...query, cursor: more } : query } }))
        .then((res) => {
          setRuns((prev) => (more && prev ? [...prev, ...res.items] : res.items))
          setCursor(res.next_cursor ?? null)
        })
        .catch((e) => {
          toast(`Could not load runs: ${errorMessage(e)}`, 'error')
          setRuns([])
        }),
    [query, toast],
  )

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    unwrap(api.GET('/api/v1/plans', { params: { query: { limit: '500' } } }))
      .then((r) => setPlans(r.items))
      .catch(() => {})
  }, [])

  // Keep running runs fresh without a full reload.
  const anyRunning = runs?.some((r) => r.status === 'running' || r.status === 'queued')
  useEffect(() => {
    if (!anyRunning) return
    const id = setInterval(() => load(), 5000)
    return () => clearInterval(id)
  }, [anyRunning, load])

  const sorted = useMemo(() => {
    if (!runs) return []
    const out = [...runs]
    out.sort((a, b) => {
      const x = sortValue(a, sort.key)
      const y = sortValue(b, sort.key)
      const c = x < y ? -1 : x > y ? 1 : 0
      return sort.desc ? -c : c
    })
    return out
  }, [runs, sort])

  function toggle(id: string) {
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : s.length >= 5 ? s : [...s, id]))
  }

  const th = 'px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wide text-slate-500'
  const td = 'px-3 py-2 text-xs text-slate-300'

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <PageHeader
        title="Runs"
        description="Every test run with its outcome. Tick two to five runs to compare them."
        actions={
          <Button variant="primary" disabled={selected.length < 2} onClick={() => navigate(`#/compare?ids=${selected.join(',')}`)}>
            Compare {selected.length > 0 ? `(${selected.length})` : ''}
          </Button>
        }
      />

      <div className="mb-4 flex flex-wrap items-end gap-3">
        <label className="text-xs text-slate-400">
          Status
          <Select value={status} onChange={(e) => setStatus(e.target.value)} className="mt-1 w-36">
            <option value="all">All</option>
            <option value="running">Running</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="aborted">Aborted</option>
          </Select>
        </label>
        <label className="text-xs text-slate-400">
          Plan
          <Select value={planId} onChange={(e) => setPlanId(e.target.value)} className="mt-1 w-56">
            <option value="">All plans</option>
            {plans.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </label>
        <label className="text-xs text-slate-400">
          From
          <TextInput type="date" value={since} onChange={(e) => setSince(e.target.value)} className="mt-1 w-40" />
        </label>
        <label className="text-xs text-slate-400">
          To
          <TextInput type="date" value={until} onChange={(e) => setUntil(e.target.value)} className="mt-1 w-40" />
        </label>
        {(status !== 'all' || planId || since || until) && (
          <Button size="sm" variant="ghost" onClick={() => { setStatus('all'); setPlanId(''); setSince(''); setUntil('') }}>Clear filters</Button>
        )}
      </div>

      {runs === null ? (
        <Spinner />
      ) : sorted.length === 0 ? (
        <EmptyState title="No runs match">Open a plan and press Run to create one.</EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-slate-800">
          <table className="w-full">
            <thead className="bg-slate-900">
              <tr>
                <th className={`${th} w-8`}><span className="sr-only">Select</span></th>
                {COLUMNS.map(([key, label, right]) => (
                  <th key={key} className={`${th} ${right ? 'text-right' : ''}`}>
                    <button type="button" onClick={() => setSort((s) => ({ key, desc: s.key === key ? !s.desc : true }))} className="uppercase hover:text-slate-300">
                      {label} {sort.key === key ? (sort.desc ? '↓' : '↑') : ''}
                    </button>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sorted.map((r) => (
                <tr key={r.id} className="cursor-pointer border-t border-slate-800 hover:bg-slate-900" onClick={() => navigate(`#/runs/${r.id}`)}>
                  <td className={td} onClick={(e) => e.stopPropagation()}>
                    <input type="checkbox" aria-label={`Select run ${r.id.slice(0, 8)}`} checked={selected.includes(r.id)} disabled={!selected.includes(r.id) && selected.length >= 5} onChange={() => toggle(r.id)} className="accent-teal-500" />
                  </td>
                  <td className={td}>
                    <span className="text-slate-100">{r.plan_name || 'Deleted plan'}</span>
                    <span className="ml-2 font-mono text-[10px] text-slate-500">{r.id.slice(0, 8)}</span>
                    {r.created_by_name && <span className="ml-2 text-[10px] text-slate-500">by {r.created_by_name}</span>}
                  </td>
                  <td className={`${td} whitespace-nowrap`}>{r.started_at ? new Date(r.started_at).toLocaleString() : '—'}</td>
                  <td className={`${td} text-right tabular-nums`}>{formatSeconds(runDuration(r.started_at, r.ended_at))}</td>
                  <td className={td}><StatusBadge status={r.status} /></td>
                  <td className={`${td} text-right tabular-nums`}>{r.summary ? formatNumber(r.summary.peak_vus) : '—'}</td>
                  <td className={`${td} text-right tabular-nums`}>{r.summary ? formatMs(r.summary.p95_ms) : '—'}</td>
                  <td className={`${td} text-right tabular-nums ${r.summary && r.summary.error_rate > 0 ? 'text-red-400' : ''}`}>{r.summary ? `${r.summary.error_rate.toFixed(2)}%` : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {cursor && (
        <div className="mt-4 flex justify-center">
          <Button onClick={() => load(cursor)}>Load more</Button>
        </div>
      )}
    </div>
  )
}
