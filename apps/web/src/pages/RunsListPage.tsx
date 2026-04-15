import { useEffect, useState, useCallback } from 'react'
import { useRunStore, type RunWithPlanName, type RunListFilters } from '../stores/run-store'

type SortField = 'plan_name' | 'started_at' | 'duration' | 'status' | 'peakVUs' | 'p95' | 'errorRate'
type SortDir = 'asc' | 'desc'

const statusOptions = ['all', 'running', 'completed', 'failed', 'cancelled'] as const

function statusBadge(status: string) {
  const map: Record<string, string> = {
    pending: 'bg-yellow-500/20 text-yellow-400',
    running: 'bg-yellow-500/20 text-yellow-400',
    completed: 'bg-green-500/20 text-green-400',
    failed: 'bg-red-500/20 text-red-400',
    cancelled: 'bg-slate-500/20 text-slate-400',
  }
  return map[status] || 'bg-slate-500/20 text-slate-400'
}

function getDuration(run: RunWithPlanName): number {
  if (!run.started_at) return 0
  const start = new Date(run.started_at).getTime()
  const end = run.ended_at ? new Date(run.ended_at).getTime() : Date.now()
  return end - start
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000)
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  if (m === 0) return `${s}s`
  return `${m}m ${s}s`
}

function sortRuns(runs: RunWithPlanName[], field: SortField, dir: SortDir): RunWithPlanName[] {
  const sorted = [...runs]
  sorted.sort((a, b) => {
    let cmp = 0
    switch (field) {
      case 'plan_name':
        cmp = (a.plan_name ?? a.plan_id).localeCompare(b.plan_name ?? b.plan_id)
        break
      case 'started_at':
        cmp = (a.started_at ?? '').localeCompare(b.started_at ?? '')
        break
      case 'duration':
        cmp = getDuration(a) - getDuration(b)
        break
      case 'status':
        cmp = a.status.localeCompare(b.status)
        break
      case 'peakVUs':
        cmp = (a.summary?.peakVUs ?? 0) - (b.summary?.peakVUs ?? 0)
        break
      case 'p95':
        cmp = (a.summary?.p95 ?? 0) - (b.summary?.p95 ?? 0)
        break
      case 'errorRate':
        cmp = (a.summary?.errorRate ?? 0) - (b.summary?.errorRate ?? 0)
        break
    }
    return dir === 'asc' ? cmp : -cmp
  })
  return sorted
}

interface RunsListPageProps {
  onBack: () => void
}

export function RunsListPage({ onBack }: RunsListPageProps) {
  const {
    runsList,
    runsListLoading,
    runsListHasMore,
    loadRuns,
    loadMoreRuns,
  } = useRunStore()

  const [statusFilter, setStatusFilter] = useState('all')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')
  const [sortField, setSortField] = useState<SortField>('started_at')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const filters: RunListFilters = {
    status: statusFilter,
    from: dateFrom || undefined,
    to: dateTo || undefined,
  }

  useEffect(() => {
    loadRuns(filters)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilter, dateFrom, dateTo])

  const handleSort = useCallback(
    (field: SortField) => {
      if (sortField === field) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
      } else {
        setSortField(field)
        setSortDir('desc')
      }
    },
    [sortField],
  )

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        if (next.size >= 5) return prev
        next.add(id)
      }
      return next
    })
  }, [])

  const handleCompare = useCallback(() => {
    const ids = Array.from(selected).join(',')
    window.location.hash = `#/compare?ids=${ids}`
  }, [selected])

  const sortedRuns = sortRuns(runsList, sortField, sortDir)
  const canCompare = selected.size >= 2 && selected.size <= 5

  const sortIndicator = (field: SortField) => {
    if (sortField !== field) return ''
    return sortDir === 'asc' ? ' \u2191' : ' \u2193'
  }

  const headerClass =
    'px-3 py-2 text-left text-xs font-medium text-slate-500 cursor-pointer select-none hover:text-slate-300'

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      {/* Header */}
      <header className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
        <div className="flex items-center gap-3">
          <button
            onClick={onBack}
            className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            &larr; Home
          </button>
          <div className="h-4 w-px bg-slate-800" />
          <h1 className="text-lg font-semibold tracking-tight">Runs</h1>
        </div>
        <button
          disabled={!canCompare}
          onClick={handleCompare}
          className="rounded-md bg-teal-600 px-4 py-2 text-sm font-medium text-white hover:bg-teal-500 disabled:cursor-not-allowed disabled:opacity-40"
        >
          Compare Selected ({selected.size})
        </button>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-6">
        {/* Filter bar */}
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label className="mb-1 block text-xs text-slate-500">Status</label>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              {statusOptions.map((s) => (
                <option key={s} value={s}>
                  {s.charAt(0).toUpperCase() + s.slice(1)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">From</label>
            <input
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">To</label>
            <input
              type="date"
              value={dateTo}
              onChange={(e) => setDateTo(e.target.value)}
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </div>
        </div>

        {/* Table */}
        <div className="mt-4 overflow-x-auto rounded-lg border border-slate-800">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-slate-800 bg-slate-900">
              <tr>
                <th className="w-10 px-3 py-2">
                  {/* Checkbox column header */}
                </th>
                <th className={headerClass} onClick={() => handleSort('plan_name')}>
                  Plan Name{sortIndicator('plan_name')}
                </th>
                <th className={headerClass} onClick={() => handleSort('started_at')}>
                  Started{sortIndicator('started_at')}
                </th>
                <th className={headerClass} onClick={() => handleSort('duration')}>
                  Duration{sortIndicator('duration')}
                </th>
                <th className={headerClass} onClick={() => handleSort('status')}>
                  Status{sortIndicator('status')}
                </th>
                <th className={headerClass} onClick={() => handleSort('peakVUs')}>
                  Peak VUs{sortIndicator('peakVUs')}
                </th>
                <th className={headerClass} onClick={() => handleSort('p95')}>
                  p95{sortIndicator('p95')}
                </th>
                <th className={headerClass} onClick={() => handleSort('errorRate')}>
                  Error Rate{sortIndicator('errorRate')}
                </th>
              </tr>
            </thead>
            <tbody>
              {sortedRuns.length === 0 && !runsListLoading && (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-sm text-slate-500">
                    No runs found.
                  </td>
                </tr>
              )}
              {sortedRuns.map((run) => (
                <tr
                  key={run.id}
                  className="cursor-pointer border-b border-slate-800/50 hover:bg-slate-900/80"
                  onClick={() => (window.location.hash = `#/runs/${run.id}`)}
                >
                  <td className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      checked={selected.has(run.id)}
                      onChange={() => toggleSelect(run.id)}
                      className="h-3.5 w-3.5 cursor-pointer rounded border-slate-600 bg-slate-800 accent-teal-500"
                    />
                  </td>
                  <td className="px-3 py-2 text-sm text-slate-200">
                    {run.plan_name ?? run.plan_id.slice(0, 8)}
                  </td>
                  <td className="px-3 py-2 text-xs text-slate-400">
                    {run.started_at
                      ? new Date(run.started_at).toLocaleString()
                      : '--'}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-slate-400">
                    {formatDuration(getDuration(run))}
                  </td>
                  <td className="px-3 py-2">
                    <span
                      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadge(run.status)}`}
                    >
                      {run.status}
                    </span>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-slate-300">
                    {run.summary?.peakVUs ?? '--'}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-slate-300">
                    {run.summary?.p95 != null ? `${run.summary.p95.toFixed(1)} ms` : '--'}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-slate-300">
                    {run.summary?.errorRate != null
                      ? `${run.summary.errorRate.toFixed(2)}%`
                      : '--'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Load more */}
        {runsListHasMore && (
          <div className="mt-4 flex justify-center">
            <button
              onClick={() => loadMoreRuns(filters)}
              disabled={runsListLoading}
              className="rounded-md border border-slate-700 bg-slate-900 px-4 py-2 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-50"
            >
              {runsListLoading ? 'Loading...' : 'Load More'}
            </button>
          </div>
        )}

        {runsListLoading && runsList.length === 0 && (
          <div className="mt-8 text-center text-sm text-slate-500">Loading runs...</div>
        )}
      </main>
    </div>
  )
}
