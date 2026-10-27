interface MetricCardProps {
  label: string
  value: string | number
  sub?: string
  alert?: boolean
}

/** One headline number on the run dashboard. */
export function MetricCard({ label, value, sub, alert = false }: MetricCardProps) {
  return (
    <div className={`rounded-lg border px-3 py-2.5 ${alert ? 'border-red-500/40 bg-red-500/5' : 'border-slate-800 bg-slate-900'}`}>
      <p className="text-[11px] font-medium uppercase tracking-wide text-slate-500">{label}</p>
      <p className={`mt-0.5 text-xl font-semibold tabular-nums ${alert ? 'text-red-400' : 'text-slate-100'}`}>{value}</p>
      {sub && <p className="text-[11px] text-slate-500">{sub}</p>}
    </div>
  )
}
