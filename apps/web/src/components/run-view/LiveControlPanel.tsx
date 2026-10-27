import { useState } from 'react'
import type { LiveState } from '@opensynapse/api-client'
import { errorMessage } from '../../api'
import { type ControlPayload } from '../../stores/run-store'
import { useToast } from '../../stores/ui-store'
import { formatSeconds } from '../../utils/format'
import { Button, ConfirmDialog } from '../ui/primitives'

interface Props {
  live: LiveState | null
  currentRps: number
  control: (p: ControlPayload) => Promise<LiveState>
  stop: () => Promise<void>
  kill: () => Promise<void>
  disabled?: boolean
}

function Section({ title, children, note }: { title: string; children: React.ReactNode; note?: React.ReactNode }) {
  return (
    <section className="border-b border-slate-800 px-4 py-4">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">{title}</h3>
      {children}
      {note && <p className="mt-2 text-[11px] text-slate-500">{note}</p>}
    </section>
  )
}

/** Live test control (F-05): VUs, RPS ceiling, duration, pause, stop, kill. */
export function LiveControlPanel(props: Props) {
  if (!props.live) {
    return (
      <aside className="w-80 shrink-0 border-l border-slate-800 bg-slate-950 p-4 text-sm text-slate-400">
        Connecting to the running test…
      </aside>
    )
  }
  // Mounted once live state exists, so the inputs start from the run's values.
  return <Controls {...props} live={props.live} />
}

function Controls({ live, currentRps, control, stop, kill, disabled }: Props & { live: LiveState }) {
  const toast = useToast()
  const [vus, setVus] = useState(live.target_vus)
  const [rps, setRps] = useState(live.rps_target)
  const [minutes, setMinutes] = useState('')
  const [busy, setBusy] = useState(false)
  const [confirmKill, setConfirmKill] = useState(false)

  async function apply(payload: ControlPayload, success: string) {
    setBusy(true)
    try {
      await control(payload)
      toast(success, 'success')
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setBusy(false)
    }
  }

  const total =live.elapsed_seconds + Math.max(0, live.remaining_seconds)
  const vuMax = Math.max(100, live.max_vus * 2, vus)
  const rpsMax = Math.max(200, Math.ceil(currentRps * 3), rps)
  const off = disabled || busy || live.stopping

  return (
    <aside aria-label="Live control" className="w-80 shrink-0 overflow-y-auto border-l border-slate-800 bg-slate-950">
      <div className="flex items-center justify-between px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-100">Live control</h2>
        {live.stopping ? <span className="text-xs text-amber-300">Stopping…</span> : live.paused ? <span className="text-xs text-amber-300">Paused</span> : <span className="text-xs text-teal-300">Running</span>}
      </div>

      <Section
        title="Virtual users"
        note={
          !live.can_control_vus
            ? 'This plan uses an arrival-rate or iteration executor, which k6 cannot resize mid-run.'
            : live.manual_vus
              ? 'You have taken manual control; the plan’s load profile is paused.'
              : 'Following the plan’s load profile. Changing VUs switches to manual control.'
        }
      >
        <div className="mb-2 flex items-baseline justify-between text-sm">
          <span className="text-slate-400">Active <b className="text-slate-100">{live.active_vus}</b></span>
          <span className="text-slate-400">Target <b className="text-slate-100">{live.target_vus}</b></span>
        </div>
        <input type="range" aria-label="Target VUs" min={0} max={vuMax} value={vus} disabled={!live.can_control_vus || off} onChange={(e) => setVus(Number(e.target.value))} className="w-full accent-teal-500" />
        <div className="mt-2 flex gap-2">
          <input type="number" aria-label="Target VUs value" min={0} value={vus} disabled={!live.can_control_vus || off} onChange={(e) => setVus(Math.max(0, Number(e.target.value)))} className="w-20 rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100" />
          <Button size="sm" variant="primary" disabled={!live.can_control_vus || off || vus === live.target_vus} onClick={() => apply({ vus }, `VUs set to ${vus}`)}>Apply</Button>
          {live.manual_vus && (
            <Button size="sm" variant="ghost" disabled={off} onClick={() => apply({ resume_schedule: true }, 'Back on the plan’s load profile')}>Use plan profile</Button>
          )}
        </div>
      </Section>

      <Section
        title="Requests per second"
        note={live.can_control_rps ? '0 means no limit. The limit spreads evenly across VUs; it cannot raise throughput beyond what the VUs can generate.' : 'Live RPS limits need k6 with the xk6-kv extension (bundled in the Docker image).'}
      >
        <div className="mb-2 flex items-baseline justify-between text-sm">
          <span className="text-slate-400">Measured <b className="text-slate-100">{currentRps.toFixed(0)}</b></span>
          <span className="text-slate-400">Limit <b className="text-slate-100">{live.rps_target > 0 ? live.rps_target : 'none'}</b></span>
        </div>
        <input type="range" aria-label="RPS limit" min={0} max={rpsMax} value={rps} disabled={!live.can_control_rps || off} onChange={(e) => setRps(Number(e.target.value))} className="w-full accent-teal-500" />
        <div className="mt-2 flex gap-2">
          <input type="number" aria-label="RPS limit value" min={0} value={rps} disabled={!live.can_control_rps || off} onChange={(e) => setRps(Math.max(0, Number(e.target.value)))} className="w-20 rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100" />
          <Button size="sm" variant="primary" disabled={!live.can_control_rps || off || rps === live.rps_target} onClick={() => apply({ rps }, rps > 0 ? `RPS limited to ${rps}` : 'RPS limit removed')}>Apply</Button>
        </div>
      </Section>

      <Section
        title="Duration"
        note={live.can_control_duration ? `Can be extended up to ${formatSeconds(live.max_duration_seconds)} in total.` : 'This plan’s executors decide when the run ends.'}
      >
        <div className="mb-2 flex items-baseline justify-between text-sm">
          <span className="text-slate-400">Elapsed <b className="text-slate-100">{formatSeconds(live.elapsed_seconds)}</b></span>
          <span className="text-slate-400">Remaining <b className="text-slate-100">{live.remaining_seconds >= 0 ? formatSeconds(live.remaining_seconds) : '—'}</b></span>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={!live.can_control_duration || off} onClick={() => apply({ duration_seconds: total + 300 }, 'Extended by 5 minutes')}>+5 min</Button>
          <Button size="sm" disabled={!live.can_control_duration || off} onClick={() => apply({ duration_seconds: Math.max(live.elapsed_seconds + 5, total - 300) }, 'Shortened by 5 minutes')}>−5 min</Button>
        </div>
        <div className="mt-2 flex gap-2">
          <input type="number" min={1} placeholder="Total minutes" aria-label="Total duration in minutes" value={minutes} disabled={!live.can_control_duration || off} onChange={(e) => setMinutes(e.target.value)} className="w-28 rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100" />
          <Button size="sm" disabled={!live.can_control_duration || off || !minutes} onClick={() => apply({ duration_seconds: Math.round(Number(minutes) * 60) }, `Total duration set to ${minutes} min`)}>Set</Button>
        </div>
      </Section>

      <section className="space-y-2 px-4 py-4">
        <Button className="w-full" disabled={off} onClick={() => apply({ paused: !live.paused }, live.paused ? 'Resumed' : 'Paused')}>
          {live.paused ? 'Resume' : 'Pause'}
        </Button>
        <Button
          className="w-full"
          disabled={disabled || live.stopping}
          onClick={async () => {
            try {
              await stop()
              toast('Stopping: VUs finish their current iteration, then the run completes', 'info')
            } catch (e) {
              toast(errorMessage(e), 'error')
            }
          }}
        >
          Stop
        </Button>
        <Button className="w-full" variant="danger" disabled={disabled} onClick={() => setConfirmKill(true)}>Kill</Button>
        <p className="text-[11px] text-slate-500">Stop finishes gracefully and keeps all results. Kill ends k6 immediately and marks the run aborted.</p>
      </section>

      <ConfirmDialog
        open={confirmKill}
        title="Kill this run?"
        message="k6 is terminated immediately. In-flight requests are lost and the run is marked aborted."
        confirmLabel="Kill"
        onCancel={() => setConfirmKill(false)}
        onConfirm={async () => {
          setConfirmKill(false)
          try {
            await kill()
          } catch (e) {
            toast(errorMessage(e), 'error')
          }
        }}
      />
    </aside>
  )
}
