interface MetricCardProps {
  label: string
  value: string | number
  trend?: 'up' | 'down' | 'stable'
  trendLabel?: string
  alert?: boolean
}

export function MetricCard({ label, value, trend, trendLabel, alert }: MetricCardProps) {
  const trendColor =
    trend === 'up'
      ? 'text-red-400'
      : trend === 'down'
        ? 'text-teal-400'
        : 'text-slate-500'

  const borderColor = alert ? 'border-red-500/50' : 'border-slate-800'

  return (
    <div
      className={`rounded-lg border ${borderColor} bg-slate-900 px-4 py-3`}
    >
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 text-3xl font-semibold text-slate-100">{value}</p>
      {trend && trendLabel && (
        <p className={`mt-1 text-xs ${trendColor}`}>
          {trend === 'up' ? '\u2191' : trend === 'down' ? '\u2193' : '\u2022'}{' '}
          {trendLabel}
        </p>
      )}
    </div>
  )
}
