import { useEffect, useMemo, useRef, useState } from 'react'
import type { MetricPoint, RunEvent } from '@opensynapse/api-client'
import { api, downloadFile, errorMessage, unwrap } from '../../api'
import { canEdit, useAuthStore } from '../../stores/auth-store'
import { TERMINAL, useRunStore } from '../../stores/run-store'
import { useToast } from '../../stores/ui-store'
import { eventText, formatBytes, formatMs, formatNumber, formatSeconds, runDuration } from '../../utils/format'
import { navigate } from '../../utils/navigate'
import { AIAnalysisButton, AIInsights } from '../ai/AIAnalysis'
import { Button, ConfirmDialog, EmptyState, Spinner, StatusBadge } from '../ui/primitives'
import { LiveControlPanel } from './LiveControlPanel'
import { MetricCard } from './MetricCard'
import { useChartTheme } from './chart-theme'
import { type Marker, MetricChart } from './MetricChart'
import { ChecksTable, EndpointTable, ErrorLog, Timeline } from './RunTables'

/** Re-renders every second while `active`, for live elapsed times. */
function useTick(active: boolean) {
  const [, setTick] = useState(0)
  useEffect(() => {
    if (!active) return
    const id = setInterval(() => setTick((t) => t + 1), 1000)
    return () => clearInterval(id)
  }, [active])
}

const MARKER_TYPES = new Set(['vu_change', 'rps_change', 'duration_change', 'pause', 'resume', 'stop_requested', 'user_note'])

function markersFor(events: RunEvent[], series: MetricPoint[]): Marker[] {
  const t0 = series[0]?.timestamp_ms
  if (t0 === undefined) return []
  return events
    .filter((e) => MARKER_TYPES.has(e.type) && e.timestamp_ms >= t0)
    .map((e) => ({ t: (e.timestamp_ms - t0) / 1000, label: eventText(e).replace(/^Note: /, '✎ ') }))
}

function ExportMenu({ runId }: { runId: string }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const toast = useToast()
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && setOpen(false)
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  async function run(format: string) {
    setOpen(false)
    setBusy(format)
    try {
      await downloadFile(`/api/v1/runs/${runId}/export`, { format }, `opensynapse-run-${runId.slice(0, 8)}.${format}`)
    } catch (e) {
      toast(`Export failed: ${errorMessage(e)}`, 'error')
    } finally {
      setBusy(null)
    }
  }

  const formats = [
    ['html', 'HTML report', 'Self-contained, opens offline'],
    ['pdf', 'PDF report', 'Printable report with charts'],
    ['csv', 'CSV samples', 'One row per raw sample'],
    ['json', 'JSON document', 'Everything, machine-readable'],
  ]
  return (
    <div ref={ref} className="relative">
      <Button size="sm" onClick={() => setOpen(!open)} aria-haspopup="menu" aria-expanded={open} disabled={busy !== null}>
        {busy ? `Exporting ${busy.toUpperCase()}…` : 'Export'}
      </Button>
      {open && (
        <div role="menu" className="absolute right-0 top-full z-20 mt-1 w-56 rounded-md border border-slate-700 bg-slate-900 py-1 shadow-xl">
          {formats.map(([f, label, hint]) => (
            <button key={f} role="menuitem" type="button" onClick={() => run(f)} className="block w-full px-3 py-1.5 text-left hover:bg-slate-800">
              <span className="block text-xs text-slate-200">{label}</span>
              <span className="block text-[11px] text-slate-500">{hint}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

type Tab = 'endpoints' | 'assertions' | 'timeline'

export function RunView({ runId }: { runId: string }) {
  const { run, series, events, errors, live, wsConnected, loading, notFound, open, close, control, stop, kill, addNote } = useRunStore()
  const user = useAuthStore((s) => s.user)
  const editable = canEdit(user)
  const toast = useToast()
  const { metric } = useChartTheme()
  const [tab, setTab] = useState<Tab>('endpoints')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [rerunning, setRerunning] = useState(false)

  useEffect(() => {
    open(runId)
    return () => close()
  }, [runId, open, close])

  const running = !!run && !TERMINAL.includes(run.status)
  useTick(running)

  const markers = useMemo(() => markersFor(events, series), [events, series])
  const last = series[series.length - 1]
  const recent = series.slice(-5).filter((p) => p.requests > 0)
  const liveRps = recent.length ? recent.reduce((s, p) => s + p.rps, 0) / recent.length : 0
  const failureReason = useMemo(() => [...events].reverse().find((e) => e.type === 'error'), [events])

  if (loading) {
    return (
      <div className="p-8">
        <Spinner label="Loading run…" />
      </div>
    )
  }
  if (notFound || !run) {
    return (
      <div className="mx-auto max-w-lg p-8">
        <EmptyState title="Run not found" action={<Button onClick={() => navigate('#/runs')}>Back to runs</Button>}>
          It may have been deleted, or the link is wrong.
        </EmptyState>
      </div>
    )
  }

  const s = run.summary
  const duration = runDuration(run.started_at, run.ended_at)
  const totalRequests = s?.total_requests ?? series.reduce((a, p) => a + p.requests, 0)
  const failed = s?.failed_requests ?? series.reduce((a, p) => a + p.failed, 0)
  const errorRate = s?.error_rate ?? (totalRequests ? (failed / totalRequests) * 100 : 0)
  const checksPass = s?.assertion_passes ?? series.reduce((a, p) => a + p.checks_passed, 0)
  const checksFail = s?.assertion_failures ?? series.reduce((a, p) => a + p.checks_failed, 0)
  const checkRate = checksPass + checksFail ? (checksPass / (checksPass + checksFail)) * 100 : null

  async function runAgain() {
    if (!run) return
    setRerunning(true)
    try {
      const startEvent = events.find((e) => e.type === 'start')
      const envId = (startEvent?.payload as { environment_id?: string } | undefined)?.environment_id
      const next = await unwrap(
        api.POST('/api/v1/runs', {
          body: { plan_id: run.plan_id, parameters: run.parameters, ...(envId ? { environment_id: envId } : {}) },
        }),
      )
      navigate(`#/runs/${next.id}`)
    } catch (e) {
      toast(`Could not start the run: ${errorMessage(e)}`, 'error')
    } finally {
      setRerunning(false)
    }
  }

  async function deleteRun() {
    setConfirmDelete(false)
    try {
      await unwrap(api.DELETE('/api/v1/runs/{id}', { params: { path: { id: runId } } }))
      toast('Run deleted', 'success')
      navigate('#/runs')
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  return (
    <div className="flex h-full min-h-0">
      <div className="min-w-0 flex-1 overflow-y-auto px-6 py-5">
        {/* Header */}
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <a href="#/runs" className="text-sm text-slate-400 hover:text-slate-200">← Runs</a>
          <h1 className="text-lg font-semibold text-slate-100">
            {run.plan_id ? <a href={`#/plans/${run.plan_id}`} className="hover:underline">{run.plan_name || 'Plan'}</a> : run.plan_name || 'Deleted plan'}
          </h1>
          <StatusBadge status={run.status} />
          <span className="font-mono text-xs text-slate-500" title={run.id}>{run.id.slice(0, 8)}</span>
          <span className="text-xs text-slate-400">{formatSeconds(duration)}</span>
          {running && <span className={`text-xs ${wsConnected ? 'text-teal-400' : 'text-amber-400'}`}>{wsConnected ? '● live' : 'reconnecting…'}</span>}
          <div className="ml-auto flex items-center gap-2">
            {!running && editable && run.plan_id && <Button size="sm" variant="primary" onClick={runAgain} disabled={rerunning}>{rerunning ? 'Starting…' : 'Run again'}</Button>}
            {!running && <AIAnalysisButton runIds={[run.id]} />}
            <ExportMenu runId={run.id} />
            {!running && editable && <Button size="sm" variant="ghost" onClick={() => setConfirmDelete(true)}>Delete</Button>}
          </div>
        </div>

        <p className="mb-4 text-xs text-slate-500">
          Started {run.started_at ? new Date(run.started_at).toLocaleString() : '—'}
          {run.created_by_name && <> by {run.created_by_name}</>} · target {run.parameters.vus_target ?? '?'} VUs
          {run.parameters.stages?.length ? ` over ${run.parameters.stages.length} stages` : ''} for {formatSeconds(run.parameters.duration_seconds ?? 0)}
          {run.parameters.rps_target ? ` · RPS limit ${run.parameters.rps_target}` : ''}
        </p>

        {/* Outcome */}
        {!running && (
          <div className={`mb-4 rounded-lg border px-4 py-3 text-sm ${run.status === 'completed' ? (s?.thresholds_passed === false ? 'border-amber-500/40 bg-amber-500/5' : 'border-emerald-500/30 bg-emerald-500/5') : 'border-red-500/40 bg-red-500/5'}`}>
            {run.status === 'completed' && s?.thresholds_passed !== false && (
              <span className="text-emerald-300">Completed: {formatNumber(totalRequests)} requests in {formatSeconds(duration)}, {errorRate.toFixed(2)}% failed, p95 {formatMs(s?.p95_ms ?? 0)}.</span>
            )}
            {run.status === 'completed' && s?.thresholds_passed === false && <span className="text-amber-300">Completed, but one or more thresholds failed (pass/fail criteria set on the plan).</span>}
            {run.status === 'failed' && <span className="text-red-300">The run failed. {failureReason ? eventText(failureReason) : 'See the timeline for details.'}</span>}
            {run.status === 'aborted' && <span className="text-slate-300">The run was killed before it finished. Results up to that point are shown.</span>}
          </div>
        )}

        {/* Headline numbers */}
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-8">
          <MetricCard label="Requests" value={formatNumber(totalRequests)} />
          <MetricCard label="Failed" value={formatNumber(failed)} alert={failed > 0} />
          <MetricCard label="Error rate" value={`${errorRate.toFixed(2)}%`} alert={errorRate > 0} />
          <MetricCard label={running ? 'RPS now' : 'Avg RPS'} value={(running ? liveRps : (s?.throughput_rps ?? 0)).toFixed(1)} />
          <MetricCard label="p95" value={formatMs(running ? (last?.p95_ms ?? 0) : (s?.p95_ms ?? 0))} sub={running ? 'last second' : 'whole run'} />
          <MetricCard label="p99" value={formatMs(running ? (last?.p99_ms ?? 0) : (s?.p99_ms ?? 0))} sub={running ? 'last second' : 'whole run'} />
          <MetricCard label={running ? 'Active VUs' : 'Peak VUs'} value={running ? (live?.active_vus ?? last?.active_vus ?? 0) : (s?.peak_vus ?? 0)} />
          <MetricCard label="Checks passed" value={checkRate === null ? '—' : `${checkRate.toFixed(1)}%`} alert={checksFail > 0} />
        </div>

        {/* Six dashboard panels (F-06) */}
        <div className="mt-4 grid grid-cols-1 gap-3 xl:grid-cols-2">
          <MetricChart title="Requests per second" unit="req/s" points={series} markers={markers} series={[{ key: 'rps', label: 'RPS', color: metric.rps, value: (p) => p.rps }]} />
          <MetricChart
            title="Response time percentiles"
            unit="ms"
            points={series}
            markers={markers}
            series={[
              { key: 'p50', label: 'p50', color: metric.p50, value: (p) => p.p50_ms },
              { key: 'p90', label: 'p90', color: metric.p90, value: (p) => p.p90_ms },
              { key: 'p95', label: 'p95', color: metric.p95, value: (p) => p.p95_ms },
              { key: 'p99', label: 'p99', color: metric.p99, value: (p) => p.p99_ms },
            ]}
          />
          <MetricChart title="Error rate" unit="%" area points={series} markers={markers} series={[{ key: 'err', label: 'Errors', color: metric.error, value: (p) => p.error_rate }]} />
          <MetricChart title="Active virtual users" unit="VUs" step points={series} markers={markers} series={[{ key: 'vus', label: 'VUs', color: metric.vus, value: (p) => p.active_vus }]} formatValue={(v) => v.toFixed(0)} />
          <MetricChart
            title="Throughput"
            unit="bytes/s"
            points={series}
            markers={markers}
            formatValue={(v) => formatBytes(v)}
            series={[
              { key: 'sent', label: 'Sent', color: metric.sent, value: (p) => p.bytes_sent },
              { key: 'recv', label: 'Received', color: metric.received, value: (p) => p.bytes_received },
            ]}
          />
          <figure className="rounded-lg border border-slate-800 bg-slate-900 p-3">
            <figcaption className="mb-2 flex items-baseline justify-between">
              <span className="text-xs font-medium text-slate-300">Error log</span>
              <span className="text-[11px] text-slate-500">last 50</span>
            </figcaption>
            <ErrorLog errors={errors} />
          </figure>
        </div>

        <div className="mt-4">
          <AIInsights runIds={[run.id]} />
        </div>

        {/* Breakdown tabs */}
        <div className="mt-6">
          <div role="tablist" className="mb-3 flex gap-1 border-b border-slate-800">
            {(
              [
                ['endpoints', `Endpoints${s?.endpoints?.length ? ` (${s.endpoints.length})` : ''}`],
                ['assertions', `Assertions${s?.checks?.length ? ` (${s.checks.length})` : ''}`],
                ['timeline', `Timeline (${events.length})`],
              ] as [Tab, string][]
            ).map(([k, label]) => (
              <button key={k} role="tab" type="button" aria-selected={tab === k} onClick={() => setTab(k)} className={`-mb-px border-b-2 px-3 py-2 text-xs font-medium ${tab === k ? 'border-teal-500 text-teal-300' : 'border-transparent text-slate-400 hover:text-slate-200'}`}>
                {label}
              </button>
            ))}
          </div>
          {tab === 'endpoints' && (s ? <EndpointTable endpoints={s.endpoints ?? []} /> : <EmptyState title="Available when the run ends" />)}
          {tab === 'assertions' && (s ? <ChecksTable checks={s.checks ?? []} /> : <EmptyState title="Available when the run ends">Live pass/fail counts are on the “Checks passed” card above.</EmptyState>)}
          {tab === 'timeline' && <Timeline events={events} onNote={addNote} canNote={editable} />}
        </div>

        <p className="mt-6 text-[11px] text-slate-500">
          Response times are k6’s <code>http_req_duration</code> (sending + waiting + receiving). k6 typically reports 10–20% lower than JMeter for the same server because of how it accounts for connection reuse.
        </p>
      </div>

      {running && editable && <LiveControlPanel live={live} currentRps={liveRps} control={control} stop={stop} kill={kill} />}

      <ConfirmDialog open={confirmDelete} title="Delete run" message="Delete this run and all its samples? This cannot be undone." onConfirm={deleteRun} onCancel={() => setConfirmDelete(false)} />
    </div>
  )
}
