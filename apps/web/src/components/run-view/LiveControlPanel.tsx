import { useState, useCallback, useEffect } from 'react'
import { useRunStore } from '../../stores/run-store'

interface LiveControlPanelProps {
  runId: string
}

function formatSeconds(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return `${m}m ${s.toString().padStart(2, '0')}s`
}

export function LiveControlPanel({ runId }: LiveControlPanelProps) {
  const { run, metrics, controlRun, stopRun, killRun } = useRunStore()

  // Local control state
  const [vuTarget, setVuTarget] = useState(0)
  const [rpsTarget, setRpsTarget] = useState(0)
  const [durationTarget, setDurationTarget] = useState(0)

  // Loading / error state per section
  const [vuLoading, setVuLoading] = useState(false)
  const [rpsLoading, setRpsLoading] = useState(false)
  const [durationLoading, setDurationLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Confirm state for destructive actions
  const [confirmStop, setConfirmStop] = useState(false)
  const [confirmKill, setConfirmKill] = useState(false)

  // Current live values
  const currentVUs =
    metrics.activeVUs.length > 0 ? metrics.activeVUs[metrics.activeVUs.length - 1].value : 0
  const currentRps =
    metrics.rps.length > 0 ? metrics.rps[metrics.rps.length - 1].value : 0

  // Elapsed time — tick a clock state so Date.now() is never called during render
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const interval = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(interval)
  }, [])
  const elapsedSeconds = run?.started_at
    ? Math.floor((now - new Date(run.started_at).getTime()) / 1000)
    : 0

  const isPaused = run?.paused ?? false

  const clearError = useCallback(() => {
    setError(null)
  }, [])

  const handleApplyVUs = useCallback(async () => {
    clearError()
    setVuLoading(true)
    const result = await controlRun(runId, { vus: vuTarget })
    setVuLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to set VUs')
  }, [runId, vuTarget, controlRun, clearError])

  const handleApplyRps = useCallback(async () => {
    clearError()
    setRpsLoading(true)
    const result = await controlRun(runId, { rps: rpsTarget })
    setRpsLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to set RPS')
  }, [runId, rpsTarget, controlRun, clearError])

  const handleApplyDuration = useCallback(async () => {
    clearError()
    setDurationLoading(true)
    const result = await controlRun(runId, { duration_seconds: durationTarget })
    setDurationLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to set duration')
  }, [runId, durationTarget, controlRun, clearError])

  const handleAddDuration = useCallback(async (delta: number) => {
    clearError()
    setDurationLoading(true)
    const newDuration = Math.max(0, durationTarget + delta)
    setDurationTarget(newDuration)
    const result = await controlRun(runId, { duration_seconds: newDuration })
    setDurationLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to adjust duration')
  }, [runId, durationTarget, controlRun, clearError])

  const handlePauseResume = useCallback(async () => {
    clearError()
    setActionLoading(true)
    const result = await controlRun(runId, { paused: !isPaused })
    setActionLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to toggle pause')
  }, [runId, isPaused, controlRun, clearError])

  const handleStop = useCallback(async () => {
    if (!confirmStop) {
      setConfirmStop(true)
      return
    }
    clearError()
    setActionLoading(true)
    setConfirmStop(false)
    const result = await stopRun(runId)
    setActionLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to stop run')
  }, [runId, confirmStop, stopRun, clearError])

  const handleKill = useCallback(async () => {
    if (!confirmKill) {
      setConfirmKill(true)
      return
    }
    clearError()
    setActionLoading(true)
    setConfirmKill(false)
    const result = await killRun(runId)
    setActionLoading(false)
    if (!result.ok) setError(result.error ?? 'Failed to kill run')
  }, [runId, confirmKill, killRun, clearError])

  return (
    <div className="flex h-full w-70 flex-col border-l border-slate-800 bg-slate-900">
      {/* Header */}
      <div className="border-b border-slate-800 px-3 py-2.5">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
          Live Control
        </h2>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* Error banner */}
        {error && (
          <div className="mx-3 mt-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2">
            <div className="flex items-start justify-between gap-2">
              <p className="text-xs text-red-400">{error}</p>
              <button
                onClick={clearError}
                className="shrink-0 text-xs text-red-500 hover:text-red-300"
              >
                x
              </button>
            </div>
          </div>
        )}

        {/* VU Control */}
        <div className="border-b border-slate-800 px-3 py-3">
          <div className="flex items-baseline justify-between">
            <span className="text-xs font-medium text-slate-500">Virtual Users</span>
            <span className="text-sm font-semibold text-slate-200">
              {Math.round(currentVUs)}
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={500}
            step={1}
            value={vuTarget}
            onChange={(e) => setVuTarget(parseInt(e.target.value))}
            className="mt-2 h-1.5 w-full cursor-pointer appearance-none rounded-full bg-slate-700 accent-teal-500"
          />
          <div className="mt-2 flex items-center gap-2">
            <input
              type="number"
              min={0}
              max={500}
              value={vuTarget}
              onChange={(e) => setVuTarget(Math.max(0, Math.min(500, parseInt(e.target.value) || 0)))}
              className="w-full rounded border border-slate-700 bg-slate-800 px-2 py-1 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
            <button
              onClick={handleApplyVUs}
              disabled={vuLoading}
              className="shrink-0 rounded bg-teal-600 px-3 py-1 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
            >
              {vuLoading ? '...' : 'Apply'}
            </button>
          </div>
        </div>

        {/* RPS Control */}
        <div className="border-b border-slate-800 px-3 py-3">
          <div className="flex items-baseline justify-between">
            <span className="text-xs font-medium text-slate-500">Requests/sec</span>
            <span className="text-sm font-semibold text-slate-200">
              {currentRps.toFixed(1)}
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={10000}
            step={10}
            value={rpsTarget}
            onChange={(e) => setRpsTarget(parseInt(e.target.value))}
            className="mt-2 h-1.5 w-full cursor-pointer appearance-none rounded-full bg-slate-700 accent-teal-500"
          />
          <div className="mt-2 flex items-center gap-2">
            <input
              type="number"
              min={0}
              max={10000}
              step={10}
              value={rpsTarget}
              onChange={(e) => setRpsTarget(Math.max(0, Math.min(10000, parseInt(e.target.value) || 0)))}
              className="w-full rounded border border-slate-700 bg-slate-800 px-2 py-1 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
            <button
              onClick={handleApplyRps}
              disabled={rpsLoading}
              className="shrink-0 rounded bg-teal-600 px-3 py-1 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
            >
              {rpsLoading ? '...' : 'Apply'}
            </button>
          </div>
        </div>

        {/* Duration Control */}
        <div className="border-b border-slate-800 px-3 py-3">
          <div className="flex items-baseline justify-between">
            <span className="text-xs font-medium text-slate-500">Duration</span>
            <span className="text-sm font-semibold text-slate-200">
              {formatSeconds(elapsedSeconds)}
            </span>
          </div>
          <div className="mt-2 flex items-center gap-2">
            <button
              onClick={() => handleAddDuration(-300)}
              disabled={durationLoading}
              className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-xs text-slate-300 hover:bg-slate-700 disabled:opacity-50"
            >
              -5 min
            </button>
            <button
              onClick={() => handleAddDuration(300)}
              disabled={durationLoading}
              className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-xs text-slate-300 hover:bg-slate-700 disabled:opacity-50"
            >
              +5 min
            </button>
          </div>
          <div className="mt-2 flex items-center gap-2">
            <input
              type="number"
              min={0}
              placeholder="seconds"
              value={durationTarget || ''}
              onChange={(e) => setDurationTarget(Math.max(0, parseInt(e.target.value) || 0))}
              className="w-full rounded border border-slate-700 bg-slate-800 px-2 py-1 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
            <button
              onClick={handleApplyDuration}
              disabled={durationLoading}
              className="shrink-0 rounded bg-teal-600 px-3 py-1 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
            >
              {durationLoading ? '...' : 'Apply'}
            </button>
          </div>
          <p className="mt-1 text-xs text-slate-600">Set total duration in seconds</p>
        </div>

        {/* Action Buttons */}
        <div className="px-3 py-3">
          <span className="text-xs font-medium text-slate-500">Actions</span>
          <div className="mt-2 flex flex-col gap-2">
            {/* Pause / Resume */}
            <button
              onClick={handlePauseResume}
              disabled={actionLoading}
              className="w-full rounded bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
            >
              {actionLoading ? '...' : isPaused ? 'Resume' : 'Pause'}
            </button>

            {/* Stop */}
            <button
              onClick={handleStop}
              disabled={actionLoading}
              onBlur={() => setConfirmStop(false)}
              className={`w-full rounded px-3 py-1.5 text-xs font-medium disabled:opacity-50 ${
                confirmStop
                  ? 'bg-amber-600 text-white hover:bg-amber-500'
                  : 'border border-slate-700 bg-slate-800 text-slate-300 hover:bg-slate-700'
              }`}
            >
              {confirmStop ? 'Confirm stop?' : 'Stop'}
            </button>

            {/* Kill */}
            <button
              onClick={handleKill}
              disabled={actionLoading}
              onBlur={() => setConfirmKill(false)}
              className={`w-full rounded px-3 py-1.5 text-xs font-medium disabled:opacity-50 ${
                confirmKill
                  ? 'bg-red-600 text-white hover:bg-red-500'
                  : 'bg-red-500/10 text-red-400 hover:bg-red-500/20'
              }`}
            >
              {confirmKill ? 'Confirm kill?' : 'Kill'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
