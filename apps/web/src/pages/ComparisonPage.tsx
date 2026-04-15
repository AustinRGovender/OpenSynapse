import { useEffect, useState, useCallback, useMemo } from 'react'
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts'
import { useRunStore, type RunWithPlanName, type TimeSeriesPoint } from '../stores/run-store'

/** Chart palette colors -- Tailwind equivalents used for classes, raw values for recharts strokes */
const CHART_COLORS = [
  { tw: 'bg-teal-500', hex: '#0d9488' },
  { tw: 'bg-violet-500', hex: '#8b5cf6' },
  { tw: 'bg-amber-500', hex: '#f59e0b' },
  { tw: 'bg-teal-400', hex: '#14b8a6' },
  { tw: 'bg-pink-500', hex: '#ec4899' },
  { tw: 'bg-indigo-500', hex: '#6366f1' },
  { tw: 'bg-lime-500', hex: '#84cc16' },
  { tw: 'bg-cyan-500', hex: '#06b6d4' },
]

function formatTime(epoch: number): string {
  const d = new Date(epoch * 1000)
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${m}:${s}`
}

function runLabel(run: RunWithPlanName): string {
  return run.plan_name ?? run.plan_id.slice(0, 8)
}

function runChipLabel(run: RunWithPlanName): string {
  return `${run.id.slice(0, 6)} - ${runLabel(run)}`
}

interface MetricChange {
  label: string
  firstValue: number
  lastValue: number
  absoluteChange: number
  percentChange: number
  improved: boolean
  unit: string
}

function computeChanges(runs: RunWithPlanName[]): MetricChange[] {
  if (runs.length < 2) return []

  const first = runs[0]
  const last = runs[runs.length - 1]

  if (!first.summary || !last.summary) return []

  const metrics: Array<{
    label: string
    key: keyof typeof first.summary
    unit: string
    lowerIsBetter: boolean
  }> = [
    { label: 'p95 Response Time', key: 'p95', unit: 'ms', lowerIsBetter: true },
    { label: 'Error Rate', key: 'errorRate', unit: '%', lowerIsBetter: true },
    { label: 'Throughput (RPS)', key: 'avgRps', unit: 'rps', lowerIsBetter: false },
  ]

  const changes: MetricChange[] = []

  for (const m of metrics) {
    const firstVal = first.summary[m.key] as number
    const lastVal = last.summary[m.key] as number
    const abs = lastVal - firstVal
    const pct = firstVal !== 0 ? (abs / firstVal) * 100 : 0

    // Only flag changes > 5%
    if (Math.abs(pct) <= 5) continue

    const improved = m.lowerIsBetter ? lastVal < firstVal : lastVal > firstVal

    changes.push({
      label: m.label,
      firstValue: firstVal,
      lastValue: lastVal,
      absoluteChange: abs,
      percentChange: pct,
      improved,
      unit: m.unit,
    })
  }

  return changes
}

/** Merge time series from multiple runs into a single recharts-compatible dataset */
function mergeTimeSeries(
  runs: RunWithPlanName[],
  getMetricData: (runId: string) => TimeSeriesPoint[],
): Array<Record<string, number>> {
  // Collect all timestamps then normalize each run to relative time
  const allPoints: Array<Record<string, number>> = []

  for (let i = 0; i < runs.length; i++) {
    const data = getMetricData(runs[i].id)
    if (data.length === 0) continue

    const baseTime = data[0].time
    for (const pt of data) {
      const relTime = pt.time - baseTime
      // Find existing point at this relative time or create new
      let existing = allPoints.find((p) => p.time === relTime)
      if (!existing) {
        existing = { time: relTime }
        allPoints.push(existing)
      }
      existing[`run_${i}`] = pt.value
    }
  }

  allPoints.sort((a, b) => a.time - b.time)
  return allPoints
}

interface ComparisonChartProps {
  title: string
  yLabel: string
  runs: RunWithPlanName[]
  getMetricData: (runId: string) => TimeSeriesPoint[]
}

function ComparisonChart({ title, yLabel, runs, getMetricData }: ComparisonChartProps) {
  const data = useMemo(
    () => mergeTimeSeries(runs, getMetricData),
    [runs, getMetricData],
  )

  if (data.length === 0) {
    return (
      <div className="flex flex-col rounded-lg border border-slate-800 bg-slate-900 p-4">
        <h3 className="mb-3 text-sm font-medium text-slate-300">{title}</h3>
        <div className="flex h-48 items-center justify-center text-xs text-slate-600">
          No data available
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col rounded-lg border border-slate-800 bg-slate-900 p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium text-slate-300">{title}</h3>
        <span className="text-xs text-slate-500">{yLabel}</span>
      </div>
      <div className="h-56 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data}>
            <CartesianGrid
              strokeDasharray="3 3"
              stroke="#1e293b"
              horizontal={true}
              vertical={false}
            />
            <XAxis
              dataKey="time"
              tickFormatter={formatTime}
              stroke="#94a3b8"
              tick={{ fontSize: 11 }}
              axisLine={{ stroke: '#334155' }}
              tickLine={false}
            />
            <YAxis
              stroke="#94a3b8"
              tick={{ fontSize: 11 }}
              axisLine={{ stroke: '#334155' }}
              tickLine={false}
              width={50}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: '#0f172a',
                border: '1px solid #334155',
                borderRadius: 6,
                fontSize: 12,
              }}
              labelFormatter={(label) => formatTime(Number(label))}
            />
            <Legend
              verticalAlign="bottom"
              wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
            />
            {runs.map((run, i) => (
              <Line
                key={run.id}
                type="monotone"
                dataKey={`run_${i}`}
                name={runChipLabel(run)}
                stroke={CHART_COLORS[i % CHART_COLORS.length].hex}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
                connectNulls={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
      {/* Legend below chart */}
      <div className="mt-2 flex flex-wrap gap-3">
        {runs.map((run, i) => (
          <div key={run.id} className="flex items-center gap-1.5">
            <span
              className={`inline-block h-2.5 w-2.5 rounded-full ${CHART_COLORS[i % CHART_COLORS.length].tw}`}
            />
            <span className="text-xs text-slate-400">{runChipLabel(run)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

interface ComparisonPageProps {
  ids: string[]
  onBack: () => void
}

export function ComparisonPage({ ids, onBack }: ComparisonPageProps) {
  const { compareRuns, compareLoading, loadMultipleRuns } = useRunStore()

  // Per-run metric data loaded from API
  const [runMetrics, setRunMetrics] = useState<
    Record<string, { rps: TimeSeriesPoint[]; p95: TimeSeriesPoint[]; errorRate: TimeSeriesPoint[]; activeVUs: TimeSeriesPoint[] }>
  >({})

  useEffect(() => {
    loadMultipleRuns(ids)
  }, [ids, loadMultipleRuns])

  // Once runs are loaded, fetch their metrics
  useEffect(() => {
    if (compareRuns.length === 0) return

    async function fetchMetrics() {
      const metricsMap: typeof runMetrics = {}

      await Promise.all(
        compareRuns.map(async (run) => {
          try {
            const res = await fetch(`/api/v1/runs/${run.id}/metrics`)
            if (res.ok) {
              const data = await res.json()
              metricsMap[run.id] = {
                rps: data.rps ?? [],
                p95: data.p95 ?? [],
                errorRate: data.errorRate ?? [],
                activeVUs: data.activeVUs ?? [],
              }
            } else {
              metricsMap[run.id] = { rps: [], p95: [], errorRate: [], activeVUs: [] }
            }
          } catch {
            metricsMap[run.id] = { rps: [], p95: [], errorRate: [], activeVUs: [] }
          }
        }),
      )

      setRunMetrics(metricsMap)
    }

    fetchMetrics()
  }, [compareRuns])

  const [removedIds, setRemovedIds] = useState<Set<string>>(new Set())

  const removeRun = useCallback((id: string) => {
    setRemovedIds((prev) => new Set([...prev, id]))
  }, [])

  const visibleRuns = compareRuns.filter((r) => !removedIds.has(r.id))
  const changes = useMemo(() => computeChanges(visibleRuns), [visibleRuns])

  const getMetricGetter = useCallback(
    (metric: 'rps' | 'p95' | 'errorRate' | 'activeVUs') => {
      return (runId: string): TimeSeriesPoint[] => {
        return runMetrics[runId]?.[metric] ?? []
      }
    },
    [runMetrics],
  )

  if (compareLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950 text-slate-400">
        Loading comparison data...
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      {/* Header */}
      <header className="flex items-center gap-3 border-b border-slate-800 px-6 py-4">
        <button
          onClick={onBack}
          className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
        >
          &larr; Runs
        </button>
        <div className="h-4 w-px bg-slate-800" />
        <h1 className="text-lg font-semibold tracking-tight">Compare Runs</h1>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-6">
        {/* Top strip: run chips */}
        <div className="flex flex-wrap gap-2">
          {visibleRuns.map((run, i) => (
            <div
              key={run.id}
              className="flex items-center gap-2 rounded-full border border-slate-700 bg-slate-900 px-3 py-1"
            >
              <span
                className={`inline-block h-2.5 w-2.5 rounded-full ${CHART_COLORS[i % CHART_COLORS.length].tw}`}
              />
              <span className="text-xs text-slate-300">{runChipLabel(run)}</span>
              <button
                onClick={() => removeRun(run.id)}
                className="ml-1 text-xs text-slate-500 hover:text-slate-200"
                title="Remove from comparison"
              >
                x
              </button>
            </div>
          ))}
        </div>

        {visibleRuns.length < 2 && (
          <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900 px-6 py-8 text-center text-sm text-slate-500">
            Select at least 2 runs to compare. Go back to the runs list and select runs.
          </div>
        )}

        {/* Summary block */}
        {changes.length > 0 && visibleRuns.length >= 2 && (
          <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900 p-4">
            <h2 className="mb-3 text-sm font-semibold text-slate-300">
              Summary: {runChipLabel(visibleRuns[0])} vs {runChipLabel(visibleRuns[visibleRuns.length - 1])}
            </h2>
            <div className="space-y-2">
              {changes.map((c) => (
                <div key={c.label} className="flex items-baseline gap-2">
                  <span
                    className={`text-sm font-medium ${c.improved ? 'text-green-400' : 'text-red-400'}`}
                  >
                    {c.improved ? '\u2193' : '\u2191'} {c.label}:
                  </span>
                  <span className="text-xs text-slate-400">
                    {c.firstValue.toFixed(2)} {c.unit} &rarr; {c.lastValue.toFixed(2)} {c.unit}
                  </span>
                  <span
                    className={`text-xs font-semibold ${c.improved ? 'text-green-400' : 'text-red-400'}`}
                  >
                    ({c.absoluteChange > 0 ? '+' : ''}
                    {c.absoluteChange.toFixed(2)} {c.unit}, {c.percentChange > 0 ? '+' : ''}
                    {c.percentChange.toFixed(1)}%)
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {changes.length === 0 && visibleRuns.length >= 2 && (
          <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900 p-4">
            <h2 className="mb-2 text-sm font-semibold text-slate-300">Summary</h2>
            <p className="text-xs text-slate-500">
              No significant changes detected between selected runs (threshold: 5%).
            </p>
          </div>
        )}

        {/* Charts area */}
        {visibleRuns.length >= 2 && (
          <div className="mt-6 grid grid-cols-2 gap-4">
            <ComparisonChart
              title="p95 Response Time"
              yLabel="ms"
              runs={visibleRuns}
              getMetricData={getMetricGetter('p95')}
            />
            <ComparisonChart
              title="Requests per Second"
              yLabel="rps"
              runs={visibleRuns}
              getMetricData={getMetricGetter('rps')}
            />
            <ComparisonChart
              title="Error Rate"
              yLabel="%"
              runs={visibleRuns}
              getMetricData={getMetricGetter('errorRate')}
            />
            <ComparisonChart
              title="Active Virtual Users"
              yLabel="VUs"
              runs={visibleRuns}
              getMetricData={getMetricGetter('activeVUs')}
            />
          </div>
        )}
      </main>
    </div>
  )
}
