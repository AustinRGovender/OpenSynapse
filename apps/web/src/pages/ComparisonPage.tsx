import { useEffect, useMemo, useState } from 'react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { chartPalette } from '@opensynapse/ui'
import type { Comparison, MetricPoint } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'
import { AIAnalysisButton, AIInsights } from '../components/ai/AIAnalysis'
import { useChartTheme } from '../components/run-view/chart-theme'
import { Badge, Button, Card, EmptyState, PageHeader, Spinner, StatusBadge } from '../components/ui/primitives'
import { useUIStore } from '../stores/ui-store'
import { formatMs, formatSeconds } from '../utils/format'
import { navigate } from '../utils/navigate'

type MetricKey = 'p95' | 'p50' | 'rps' | 'errors' | 'vus'

const METRICS: Record<MetricKey, { label: string; unit: string; value: (p: MetricPoint) => number }> = {
  p95: { label: 'p95 response time', unit: 'ms', value: (p) => p.p95_ms },
  p50: { label: 'Median response time', unit: 'ms', value: (p) => p.p50_ms },
  rps: { label: 'Requests per second', unit: 'req/s', value: (p) => p.rps },
  errors: { label: 'Error rate', unit: '%', value: (p) => p.error_rate },
  vus: { label: 'Active VUs', unit: 'VUs', value: (p) => p.active_vus },
}

const CHANGE_LABELS: Record<string, [string, (v: number) => string]> = {
  p95_ms: ['p95', formatMs],
  p50_ms: ['p50', formatMs],
  error_rate: ['Error rate', (v) => `${v.toFixed(2)}%`],
  throughput_rps: ['Throughput', (v) => `${v.toFixed(1)} rps`],
}

export function ComparisonPage({ ids }: { ids: string[] }) {
  const palette = useUIStore((s) => s.palette)
  const { chrome } = useChartTheme()
  const colorsFor = chartPalette(palette)
  const [data, setData] = useState<Comparison | null>(null)
  const [series, setSeries] = useState<Record<string, MetricPoint[]>>({})
  const [error, setError] = useState<string | null>(null)
  const [metrics, setMetrics] = useState<MetricKey[]>(['p95', 'rps', 'errors'])
  const [normalise, setNormalise] = useState(false)
  const key = ids.join(',')

  useEffect(() => {
    if (ids.length < 2) return
    let cancelled = false
    ;(async () => {
      try {
        const cmp = await unwrap(api.POST('/api/v1/compare', { body: { run_ids: ids } }))
        const samples = await Promise.all(ids.map((id) => unwrap(api.GET('/api/v1/runs/{id}/samples', { params: { path: { id } } }))))
        if (cancelled) return
        setData(cmp)
        setSeries(Object.fromEntries(ids.map((id, i) => [id, samples[i].items])))
      } catch (e) {
        if (!cancelled) setError(errorMessage(e))
      }
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  const labels = useMemo(() => {
    const m: Record<string, string> = {}
    data?.runs.forEach((r, i) => {
      m[r.id] = `${i === 0 ? 'Baseline' : `Run ${i + 1}`} · ${r.plan_name} (${r.id.slice(0, 6)})`
    })
    return m
  }, [data])

  if (ids.length < 2) {
    return (
      <div className="mx-auto max-w-lg p-8">
        <EmptyState title="Pick at least two runs" action={<Button onClick={() => navigate('#/runs')}>Choose runs</Button>} />
      </div>
    )
  }
  if (error) return <div className="mx-auto max-w-lg p-8"><EmptyState title="Could not compare these runs">{error}</EmptyState></div>
  if (!data) return <div className="p-8"><Spinner label="Comparing…" /></div>

  function chartRows(metric: MetricKey) {
    // Align runs on elapsed seconds (or % of run) so lines overlay.
    const byX = new Map<number, Record<string, number>>()
    for (const r of data!.runs) {
      const pts = series[r.id] ?? []
      const t0 = pts[0]?.timestamp_ms ?? 0
      const span = Math.max(1, (pts[pts.length - 1]?.timestamp_ms ?? t0) - t0)
      for (const p of pts) {
        const x = normalise ? Math.round(((p.timestamp_ms - t0) / span) * 100) : Math.round((p.timestamp_ms - t0) / 1000)
        const row = byX.get(x) ?? { x }
        row[r.id] = METRICS[metric].value(p)
        byX.set(x, row)
      }
    }
    return [...byX.values()].sort((a, b) => a.x - b.x)
  }

  const significant = data.changes.filter((c) => c.significant)

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <PageHeader
        title="Compare runs"
        description="The first run is the baseline. A change is only called out when it is statistically significant (Welch’s t-test on per-second values, p < 0.05) and at least 2%."
        actions={<AIAnalysisButton runIds={ids} />}
      />

      <div className="mb-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {data.runs.map((r, i) => (
          <a key={r.id} href={`#/runs/${r.id}`} className="flex items-center gap-2 rounded-md border border-slate-800 bg-slate-900 px-3 py-2 hover:border-slate-700">
            <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: colorsFor[i % colorsFor.length] }} />
            <span className="min-w-0 flex-1 truncate text-xs text-slate-200">{labels[r.id]}</span>
            <StatusBadge status={r.status} />
          </a>
        ))}
      </div>

      <Card className="mb-6 p-4">
        <h2 className="mb-3 text-sm font-semibold text-slate-200">Summary</h2>
        {significant.length === 0 ? (
          <p className="text-sm text-slate-400">No statistically significant differences from the baseline in p95, median, error rate or throughput.</p>
        ) : (
          <ul className="space-y-1.5">
            {significant.map((c) => {
              const [label, fmt] = CHANGE_LABELS[c.metric] ?? [c.metric, (v: number) => v.toFixed(2)]
              return (
                <li key={`${c.run_b_id}-${c.metric}`} className="flex flex-wrap items-center gap-2 text-sm">
                  <Badge tone={c.improved ? 'accent' : 'error'}>{c.improved ? 'Improved' : 'Degraded'}</Badge>
                  <span className="text-slate-200">{label}</span>
                  <span className="text-slate-400">{labels[c.run_b_id]}:</span>
                  <span className="tabular-nums text-slate-300">{fmt(c.value_a)} → {fmt(c.value_b)}</span>
                  <span className={`tabular-nums ${c.improved ? 'text-emerald-400' : 'text-red-400'}`}>({c.pct_change > 0 ? '+' : ''}{c.pct_change.toFixed(1)}%)</span>
                  {c.p_value < 1 && <span className="text-[11px] text-slate-500">p = {c.p_value < 0.001 ? '<0.001' : c.p_value.toFixed(3)}</span>}
                </li>
              )
            })}
          </ul>
        )}
      </Card>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        {(Object.keys(METRICS) as MetricKey[]).map((m) => (
          <button key={m} type="button" onClick={() => setMetrics((cur) => (cur.includes(m) ? cur.filter((x) => x !== m) : [...cur, m]))} className={`rounded-full border px-3 py-1 text-xs ${metrics.includes(m) ? 'border-teal-500 bg-teal-500/10 text-teal-300' : 'border-slate-700 text-slate-400 hover:bg-slate-800'}`}>
            {METRICS[m].label}
          </button>
        ))}
        <label className="ml-auto flex items-center gap-2 text-xs text-slate-400">
          <input type="checkbox" checked={normalise} onChange={(e) => setNormalise(e.target.checked)} className="accent-teal-500" />
          Align by % of run instead of elapsed time
        </label>
      </div>

      <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
        {metrics.map((m) => (
          <figure key={m} className="rounded-lg border border-slate-800 bg-slate-900 p-3">
            <figcaption className="mb-2 flex justify-between text-xs">
              <span className="font-medium text-slate-300">{METRICS[m].label}</span>
              <span className="text-slate-500">{METRICS[m].unit}</span>
            </figcaption>
            <ResponsiveContainer width="100%" height={240}>
              <LineChart data={chartRows(m)} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
                <CartesianGrid stroke={chrome.grid} strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="x" type="number" domain={['dataMin', 'dataMax']} stroke={chrome.axis} fontSize={11} tickFormatter={(v) => (normalise ? `${v}%` : formatSeconds(v))} />
                <YAxis width={48} stroke={chrome.axis} fontSize={11} />
                <Tooltip contentStyle={{ background: chrome.tooltipBg, border: `1px solid ${chrome.tooltipBorder}`, fontSize: 12 }} labelFormatter={(v) => (normalise ? `${v}% through` : `t = ${formatSeconds(Number(v))}`)} />
                <Legend wrapperStyle={{ fontSize: 11 }} />
                {data.runs.map((r, i) => (
                  <Line key={r.id} dataKey={r.id} name={labels[r.id]} stroke={colorsFor[i % colorsFor.length]} dot={false} strokeWidth={1.8} isAnimationActive={false} connectNulls />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </figure>
        ))}
      </div>

      <div className="mt-6">
        <AIInsights runIds={ids} />
      </div>
    </div>
  )
}
