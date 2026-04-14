import {
  ResponsiveContainer,
  LineChart,
  AreaChart,
  Line,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts'
import type { TimeSeriesPoint } from '../../stores/run-store'

interface LiveChartProps {
  title: string
  data: TimeSeriesPoint[]
  color: string
  type: 'line' | 'area' | 'step'
  yLabel: string
}

function formatTime(epoch: number): string {
  const d = new Date(epoch * 1000)
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${m}:${s}`
}

function CustomTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  payload?: Array<{ value: number }>
  label?: number
}) {
  if (!active || !payload || payload.length === 0) return null
  return (
    <div className="rounded border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-white shadow-lg">
      <p className="text-slate-400">{label ? formatTime(label) : ''}</p>
      <p className="font-medium">{payload[0].value.toFixed(2)}</p>
    </div>
  )
}

export function LiveChart({ title, data, color, type, yLabel }: LiveChartProps) {
  const ChartComponent = type === 'area' ? AreaChart : LineChart

  return (
    <div className="flex flex-col rounded-lg border border-slate-800 bg-slate-900 p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium text-slate-300">{title}</h3>
        <span className="text-xs text-slate-500">{yLabel}</span>
      </div>
      <div className="h-48 w-full">
        {data.length === 0 ? (
          <div className="flex h-full items-center justify-center text-xs text-slate-600">
            Waiting for data...
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <ChartComponent data={data}>
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
                width={45}
              />
              <Tooltip content={<CustomTooltip />} />
              {type === 'area' ? (
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke={color}
                  fill={color}
                  fillOpacity={0.15}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                />
              ) : (
                <Line
                  type={type === 'step' ? 'stepAfter' : 'monotone'}
                  dataKey="value"
                  stroke={color}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                />
              )}
            </ChartComponent>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}
