import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, Legend, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { MetricPoint } from '@opensynapse/api-client'
import { formatSeconds } from '../../utils/format'
import { useChartTheme } from './chart-theme'

export interface Series {
  key: string
  label: string
  color: string
  value: (p: MetricPoint) => number
}

export interface Marker {
  t: number // seconds since start
  label: string
}

const MAX_POINTS = 1200

/** Keeps charts responsive for long runs: averages consecutive points into buckets. */
function downsample<T extends Record<string, number>>(rows: T[]): T[] {
  if (rows.length <= MAX_POINTS) return rows
  const size = Math.ceil(rows.length / MAX_POINTS)
  const out: T[] = []
  for (let i = 0; i < rows.length; i += size) {
    const chunk = rows.slice(i, i + size)
    const avg = { ...chunk[0] }
    for (const k of Object.keys(avg) as (keyof T)[]) {
      if (k === 't') continue
      avg[k] = (chunk.reduce((s, r) => s + (r[k] as number), 0) / chunk.length) as T[keyof T]
    }
    out.push(avg)
  }
  return out
}

export function MetricChart({
  title,
  points,
  series,
  unit,
  markers = [],
  area = false,
  step = false,
  height = 200,
  formatValue = (v: number) => (Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(1)),
}: {
  title: string
  points: MetricPoint[]
  series: Series[]
  unit: string
  markers?: Marker[]
  area?: boolean
  step?: boolean
  height?: number
  formatValue?: (v: number) => string
}) {
  const { chrome, metric } = useChartTheme()
  const t0 = points[0]?.timestamp_ms ?? 0
  const data = useMemo(
    () =>
      downsample(
        points.map((p) => {
          const row: Record<string, number> = { t: (p.timestamp_ms - t0) / 1000 }
          for (const s of series) row[s.key] = s.value(p)
          return row
        }),
      ),
    [points, series, t0],
  )

  const axis = { stroke: chrome.axis, fontSize: 11, tickLine: false }
  const common = (
    <>
      <CartesianGrid stroke={chrome.grid} strokeDasharray="3 3" vertical={false} />
      <XAxis dataKey="t" type="number" domain={['dataMin', 'dataMax']} tickFormatter={(v) => formatSeconds(v)} {...axis} />
      <YAxis width={48} tickFormatter={(v) => formatValue(v)} {...axis} />
      <Tooltip
        contentStyle={{ background: chrome.tooltipBg, border: `1px solid ${chrome.tooltipBorder}`, borderRadius: 6, fontSize: 12 }}
        labelFormatter={(v) => `t = ${formatSeconds(Number(v))}`}
        formatter={(v, name) => [`${formatValue(Number(v))} ${unit}`, name]}
      />
      {series.length > 1 && <Legend wrapperStyle={{ fontSize: 11 }} iconType="plainline" />}
      {markers.map((m, i) => (
        <ReferenceLine key={i} x={m.t} stroke={metric.marker} strokeDasharray="4 3" label={{ value: m.label, position: 'insideTopRight', fill: metric.marker, fontSize: 10 }} />
      ))}
    </>
  )

  return (
    <figure className="rounded-lg border border-slate-800 bg-slate-900 p-3">
      <figcaption className="mb-2 flex items-baseline justify-between">
        <span className="text-xs font-medium text-slate-300">{title}</span>
        <span className="text-[11px] text-slate-500">{unit}</span>
      </figcaption>
      {points.length === 0 ? (
        <div className="flex items-center justify-center text-xs text-slate-500" style={{ height }}>Waiting for data…</div>
      ) : (
        <ResponsiveContainer width="100%" height={height}>
          {area ? (
            <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
              {common}
              {series.map((s) => (
                <Area key={s.key} type="monotone" dataKey={s.key} name={s.label} stroke={s.color} fill={s.color} fillOpacity={0.15} isAnimationActive={false} dot={false} />
              ))}
            </AreaChart>
          ) : (
            <LineChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
              {common}
              {series.map((s) => (
                <Line key={s.key} type={step ? 'stepAfter' : 'monotone'} dataKey={s.key} name={s.label} stroke={s.color} strokeWidth={1.8} dot={false} isAnimationActive={false} />
              ))}
            </LineChart>
          )}
        </ResponsiveContainer>
      )}
    </figure>
  )
}
