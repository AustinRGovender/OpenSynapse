import { useEffect, useMemo, useState } from 'react'
import type { Environment, RunParameters } from '@opensynapse/api-client'
import { api, unwrap } from '../../api'
import { useUIStore } from '../../stores/ui-store'
import { parseDurationToSeconds } from '../../utils/duration'
import { formatSeconds } from '../../utils/format'
import { clampInt } from '../../utils/number'
import { describeProfile, type Stage } from '../../utils/profile'
import { LoadCurveSvg } from '../templates/LoadCurveSvg'
import { TEMPLATES } from '../templates/template-data'
import { Button, Field, Modal, Select, TextInput } from '../ui/primitives'

export interface RunRequest {
  parameters?: RunParameters
  environment_id?: string
}

type Choice = { kind: 'plan' } | { kind: 'template'; name: string } | { kind: 'custom' }

export function RunConfigDialog({
  open,
  onClose,
  onConfirm,
  planScenarioProperties,
  defaultEnvironmentId,
  starting,
}: {
  open: boolean
  onClose: () => void
  onConfirm: (req: RunRequest) => void
  planScenarioProperties: Record<string, unknown> | null
  defaultEnvironmentId: string | null
  starting?: boolean
}) {
  const kv = useUIStore((s) => s.system?.k6.kv ?? false)
  const execution = useUIStore((s) => s.system?.execution)
  const maxWorkers = execution?.mode === 'kubernetes' ? execution.max_workers : 1
  const [workersText, setWorkers] = useState('1')
  const workers = clampInt(workersText, 1, maxWorkers, 1)
  const [choice, setChoice] = useState<Choice>({ kind: 'plan' })
  const [vusText, setVus] = useState('10')
  const [secondsText, setSeconds] = useState('60')
  const vus = clampInt(vusText, 1, 100000, 1)
  const seconds = clampInt(secondsText, 1, 7 * 86400, 60)
  const [rps, setRps] = useState('')
  const [envs, setEnvs] = useState<Environment[]>([])
  const [envId, setEnvId] = useState<string>(defaultEnvironmentId ?? '')

  useEffect(() => {
    if (!open) return
    unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } })).then((r) => setEnvs(r.items)).catch(() => {})
  }, [open])

  const plan = describeProfile(planScenarioProperties)
  const template = choice.kind === 'template' ? TEMPLATES.find((t) => t.name === choice.name) : undefined
  const templateProfile = template ? describeProfile(template.scenarioProperties) : null
  const vuBased = choice.kind === 'plan' ? plan.vuBased : choice.kind === 'custom' ? true : (templateProfile?.vuBased ?? true)

  const profileParameters = useMemo<RunParameters | undefined>(() => {
    const limit = vuBased && Number(rps) > 0 ? { rps_target: Number(rps) } : {}
    if (choice.kind === 'plan') return Object.keys(limit).length ? limit : undefined
    if (choice.kind === 'custom') return { vus_target: vus, duration_seconds: seconds, ...limit }
    const p = template!.scenarioProperties
    if (p.executor === 'ramping-vus') {
      const stages = (p.stages as Stage[]).slice()
      if (p.start_vus) stages.unshift({ duration: '1s', target: Number(p.start_vus) })
      return { stages, ...limit }
    }
    if (p.executor === 'constant-arrival-rate') {
      return { rps_target: Number(p.rate), duration_seconds: parseDurationToSeconds(p.duration as string), vus_target: Number(p.pre_allocated_vus ?? 2) }
    }
    return { vus_target: Number(p.vus ?? 1), duration_seconds: parseDurationToSeconds(p.duration as string), ...limit }
  }, [choice, vus, seconds, rps, template, vuBased])
  const parameters = workers > 1 ? { ...profileParameters, worker_count: workers } : profileParameters

  const summary = choice.kind === 'plan' ? plan.text : choice.kind === 'custom' ? `${vus} VUs for ${formatSeconds(seconds)}` : (templateProfile?.text ?? '')
  // Only VU-based plans can take another profile; arrival-rate and iteration
  // executors are run as designed.
  const locked = !plan.vuBased

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Run test"
      width="max-w-4xl"
      footer={
        <>
          <span className="mr-auto self-center text-xs text-slate-400">
            {summary}
            {parameters?.rps_target && vuBased ? ` · at most ${parameters.rps_target} RPS` : ''}
            {workers > 1 ? ` · ${workers} workers` : ''}
          </span>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={starting} onClick={() => onConfirm({ parameters, environment_id: envId || undefined })}>{starting ? 'Starting…' : 'Start run'}</Button>
        </>
      }
    >
      <div className="space-y-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Environment" hint="Supplies ${VARIABLE} values such as base URLs and tokens.">
            <Select value={envId} onChange={(e) => setEnvId(e.target.value)}>
              <option value="">None</option>
              {envs.map((e) => <option key={e.id} value={e.id}>{e.name}{e.id === defaultEnvironmentId ? ' (plan default)' : ''}</option>)}
            </Select>
          </Field>
          {maxWorkers > 1 && (
            <Field label="Workers" hint={`k6 runner pods sharing the load (1–${maxWorkers}). VU counts and RPS limits are totals across all workers.`}>
              <TextInput type="number" min={1} max={maxWorkers} value={workersText} onChange={(e) => setWorkers(e.target.value)} />
            </Field>
          )}
          <Field label="RPS limit (optional)" hint={!vuBased ? 'Arrival-rate profiles set their own rate.' : kv ? 'Caps total requests per second; adjustable while running.' : 'Needs k6 with xk6-kv (bundled in the Docker image).'}>
            <TextInput type="number" min={0} value={rps} disabled={!vuBased || !kv} onChange={(e) => setRps(e.target.value)} placeholder="No limit" />
          </Field>
        </div>

        <div>
          <p className="mb-2 text-xs font-medium text-slate-400">Load profile</p>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            <button type="button" onClick={() => setChoice({ kind: 'plan' })} className={`rounded-lg border p-3 text-left ${choice.kind === 'plan' ? 'border-teal-500 bg-teal-500/10' : 'border-slate-800 bg-slate-900 hover:border-slate-600'}`}>
              <span className="block text-sm font-semibold text-slate-100">Plan’s own profile</span>
              <span className="mt-1 block text-xs text-slate-400">{plan.text}</span>
            </button>
            <button type="button" disabled={locked} onClick={() => setChoice({ kind: 'custom' })} className={`rounded-lg border p-3 text-left ${choice.kind === 'custom' ? 'border-teal-500 bg-teal-500/10' : 'border-slate-800 bg-slate-900 hover:border-slate-600'} disabled:opacity-40`}>
              <span className="block text-sm font-semibold text-slate-100">Custom</span>
              <span className="mt-1 block text-xs text-slate-400">Constant VUs for a duration you choose</span>
            </button>
            {TEMPLATES.map((t) => (
              <button key={t.name} type="button" disabled={locked} onClick={() => setChoice({ kind: 'template', name: t.name })} className={`rounded-lg border p-2 text-left ${choice.kind === 'template' && choice.name === t.name ? 'border-teal-500 bg-teal-500/10' : 'border-slate-800 bg-slate-900 hover:border-slate-600'} disabled:opacity-40`}>
                <LoadCurveSvg path={t.svgPath} name={t.name} />
                <span className="mt-1 block text-sm font-semibold text-slate-100">{t.name}</span>
                <span className="block text-[11px] text-slate-400">{describeProfile(t.scenarioProperties).text}</span>
              </button>
            ))}
          </div>
        </div>

        {choice.kind === 'custom' && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Virtual users"><TextInput type="number" min={1} value={vusText} onChange={(e) => setVus(e.target.value)} /></Field>
            <Field label="Duration (seconds)"><TextInput type="number" min={1} value={secondsText} onChange={(e) => setSeconds(e.target.value)} /></Field>
          </div>
        )}
        {locked && <p className="text-xs text-amber-300">This plan uses an arrival-rate or iteration executor, so it runs with its own profile. Change the scenario in the builder to use a different load shape.</p>}
        <p className="text-[11px] text-slate-500">Overrides apply to this run only; the plan is not changed. VU-based runs can be resized, rate-limited, paused and extended live from the run screen.</p>
      </div>
    </Modal>
  )
}
