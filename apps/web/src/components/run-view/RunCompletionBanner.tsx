import type { Run } from '../../stores/run-store'

interface RunCompletionBannerProps {
  run: Run
  duration: string
}

function StatusIcon({ status }: { status: string }) {
  if (status === 'completed') {
    return (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" className="text-green-400">
        <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" />
        <path d="M8 12l2.5 2.5L16 9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }
  if (status === 'failed') {
    return (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" className="text-red-400">
        <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" />
        <path d="M15 9l-6 6M9 9l6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      </svg>
    )
  }
  // cancelled
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" className="text-slate-400">
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" />
      <path d="M8 12h8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

const statusText: Record<string, string> = {
  completed: 'Run Completed',
  failed: 'Run Failed',
  cancelled: 'Run Cancelled',
}

const borderColor: Record<string, string> = {
  completed: 'border-green-500/50',
  failed: 'border-red-500/50',
  cancelled: 'border-slate-600',
}

export function RunCompletionBanner({ run, duration }: RunCompletionBannerProps) {
  const summary = run.summary

  return (
    <div className={`mb-6 rounded-lg border-2 ${borderColor[run.status] ?? 'border-slate-600'} bg-slate-900 p-4`}>
      <div className="flex items-center gap-3">
        <StatusIcon status={run.status} />
        <div>
          <h2 className="text-base font-semibold text-slate-100">
            {statusText[run.status] ?? 'Run Ended'}
          </h2>
          <p className="text-sm text-slate-400">Duration: {duration}</p>
        </div>
      </div>

      {summary && (
        <div className="mt-3 grid grid-cols-5 gap-3 border-t border-slate-800 pt-3">
          <div>
            <p className="text-xs text-slate-500">Total Requests</p>
            <p className="text-lg font-semibold text-slate-200">
              {Math.round(summary.totalRequests).toLocaleString()}
            </p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Error Rate</p>
            <p className="text-lg font-semibold text-slate-200">{summary.errorRate.toFixed(2)}%</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">p95</p>
            <p className="text-lg font-semibold text-slate-200">{summary.p95.toFixed(1)} ms</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">RPS</p>
            <p className="text-lg font-semibold text-slate-200">{summary.avgRps.toFixed(1)}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Peak VUs</p>
            <p className="text-lg font-semibold text-slate-200">{summary.peakVUs}</p>
          </div>
        </div>
      )}
    </div>
  )
}
