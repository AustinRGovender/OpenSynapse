import { useEffect } from 'react'
import { useRunStore } from '../../stores/run-store'
import { MetricCard } from './MetricCard'
import { LiveChart } from './LiveChart'
import { LiveControlPanel } from './LiveControlPanel'

interface RunViewProps {
  runId: string
  onBack: () => void
}

function statusBadge(status: string) {
  const colors: Record<string, string> = {
    pending: 'bg-yellow-500/20 text-yellow-400',
    running: 'bg-teal-500/20 text-teal-400',
    completed: 'bg-green-500/20 text-green-400',
    failed: 'bg-red-500/20 text-red-400',
    cancelled: 'bg-slate-500/20 text-slate-400',
  }
  return colors[status] || 'bg-slate-500/20 text-slate-400'
}

function formatDuration(startedAt: string | null, endedAt: string | null): string {
  if (!startedAt) return '--'
  const start = new Date(startedAt).getTime()
  const end = endedAt ? new Date(endedAt).getTime() : Date.now()
  const seconds = Math.floor((end - start) / 1000)
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}m ${s}s`
}

export function RunView({ runId, onBack }: RunViewProps) {
  const { run, metrics, events, wsConnected, loading, loadRun, connectWebSocket, reset } =
    useRunStore()

  useEffect(() => {
    loadRun(runId)
    connectWebSocket(runId)

    return () => {
      reset()
    }
  }, [runId, loadRun, connectWebSocket, reset])

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950 text-slate-400">
        Loading run...
      </div>
    )
  }

  if (!run) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-3 bg-slate-950 text-slate-400">
        <p>Run not found</p>
        <button
          onClick={onBack}
          className="rounded-md bg-slate-800 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700"
        >
          Back to plans
        </button>
      </div>
    )
  }

  // Compute live summary from metrics or use run.summary if completed
  const summary = run.summary
  const lastRps = metrics.rps.length > 0 ? metrics.rps[metrics.rps.length - 1].value : 0
  const lastP95 = metrics.p95.length > 0 ? metrics.p95[metrics.p95.length - 1].value : 0
  const lastErrorRate =
    metrics.errorRate.length > 0 ? metrics.errorRate[metrics.errorRate.length - 1].value : 0
  const lastVUs =
    metrics.activeVUs.length > 0 ? metrics.activeVUs[metrics.activeVUs.length - 1].value : 0

  const totalRequests = summary?.totalRequests ?? metrics.rps.reduce((s, p) => s + p.value, 0)
  const failed = summary?.failed ?? 0
  const errorRate = summary?.errorRate ?? lastErrorRate
  const rps = summary?.avgRps ?? lastRps
  const p95 = summary?.p95 ?? lastP95
  const activeVUs = summary?.peakVUs ?? lastVUs

  const isRunning = run.status === 'running' || run.status === 'pending'

  return (
    <div className="flex h-screen flex-col bg-slate-950 text-slate-100">
      {/* Toolbar */}
      <div className="flex h-12 items-center gap-3 border-b border-slate-800 px-4">
        <button
          onClick={onBack}
          className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
        >
          &larr; Plans
        </button>
        <div className="h-4 w-px bg-slate-800" />
        <span className="text-sm font-medium">Run {run.id.slice(0, 8)}</span>
        <span
          className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${statusBadge(run.status)}`}
        >
          {run.status}
        </span>
        {isRunning && (
          <span className="text-xs text-slate-500">
            {formatDuration(run.started_at, run.ended_at)}
          </span>
        )}
        <div className="ml-auto flex items-center gap-3">
          <span
            className={`flex items-center gap-1.5 text-xs ${wsConnected ? 'text-teal-400' : 'text-slate-600'}`}
          >
            <span
              className={`inline-block h-1.5 w-1.5 rounded-full ${wsConnected ? 'bg-teal-400' : 'bg-slate-600'}`}
            />
            {wsConnected ? 'Live' : 'Disconnected'}
          </span>
        </div>
      </div>

      {/* Content + optional sidebar */}
      <div className="flex flex-1 overflow-hidden">
        {/* Main content */}
        <div className="flex-1 overflow-auto px-6 py-6">
          {/* Metric cards strip */}
          <div className="grid grid-cols-6 gap-3">
            <MetricCard label="Total Requests" value={Math.round(totalRequests).toLocaleString()} />
            <MetricCard label="Failed" value={Math.round(failed).toLocaleString()} alert={failed > 0} />
            <MetricCard
              label="Error Rate"
              value={`${errorRate.toFixed(2)}%`}
              alert={errorRate > 0}
            />
            <MetricCard label="Throughput RPS" value={rps.toFixed(1)} />
            <MetricCard label="p95 ms" value={p95.toFixed(1)} />
            <MetricCard label="Active VUs" value={Math.round(activeVUs)} />
          </div>

          {/* Charts 2x2 grid */}
          <div className="mt-6 grid grid-cols-2 gap-4">
            <LiveChart
              title="Requests per Second"
              data={metrics.rps}
              color="#14b8a6"
              type="line"
              yLabel="rps"
            />
            <LiveChart
              title="Response Time p95"
              data={metrics.p95}
              color="#a78bfa"
              type="line"
              yLabel="ms"
            />
            <LiveChart
              title="Error Rate"
              data={metrics.errorRate}
              color="#f87171"
              type="area"
              yLabel="%"
            />
            <LiveChart
              title="Active Virtual Users"
              data={metrics.activeVUs}
              color="#38bdf8"
              type="step"
              yLabel="VUs"
            />
          </div>

          {/* Event log */}
          <div className="mt-6">
            <h3 className="mb-3 text-sm font-medium text-slate-300">Event Log</h3>
            <div className="max-h-64 overflow-auto rounded-lg border border-slate-800 bg-slate-900">
              {events.length === 0 ? (
                <p className="px-4 py-6 text-center text-xs text-slate-600">
                  {isRunning ? 'Waiting for events...' : 'No events recorded'}
                </p>
              ) : (
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-slate-800">
                      <th className="px-4 py-2 font-medium text-slate-500">Time</th>
                      <th className="px-4 py-2 font-medium text-slate-500">Level</th>
                      <th className="px-4 py-2 font-medium text-slate-500">Message</th>
                    </tr>
                  </thead>
                  <tbody>
                    {events.map((evt) => {
                      const levelColor =
                        evt.level === 'error'
                          ? 'text-red-400'
                          : evt.level === 'warn'
                            ? 'text-yellow-400'
                            : 'text-slate-400'
                      return (
                        <tr key={evt.id} className="border-b border-slate-800/50">
                          <td className="whitespace-nowrap px-4 py-1.5 font-mono text-slate-500">
                            {new Date(evt.timestamp).toLocaleTimeString()}
                          </td>
                          <td className={`px-4 py-1.5 font-medium uppercase ${levelColor}`}>
                            {evt.level}
                          </td>
                          <td className="px-4 py-1.5 text-slate-300">{evt.message}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>

        {/* Live control sidebar — visible only when running */}
        {isRunning && <LiveControlPanel runId={runId} />}
      </div>
    </div>
  )
}
