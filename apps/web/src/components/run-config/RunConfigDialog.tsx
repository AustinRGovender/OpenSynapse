import { useState, useMemo } from 'react'
import { TEMPLATES, type TemplateConfig } from '../templates/template-data'
import { LoadCurveSvg } from '../templates/LoadCurveSvg'
import { parseDurationToSeconds, formatSecondsToDuration } from '../../utils/duration'

export interface RunParameters {
  vus_target: number
  duration_seconds: number
  rps_target?: number
}

interface RunConfigDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: (params: RunParameters) => void
  planScenarioProperties: Record<string, unknown> | null
}

type Selection = 'plan-defaults' | string // template name

interface Stage {
  duration: string
  target: number
}

function extractPlanDefaults(props: Record<string, unknown> | null): {
  executor: string
  vus: number
  duration: string
  label: string
} {
  if (!props) return { executor: 'constant-vus', vus: 10, duration: '30s', label: '10 VUs / 30s' }

  const executor = (props.executor as string) || 'constant-vus'
  if (executor === 'constant-vus') {
    const vus = (props.vus as number) || 10
    const duration = (props.duration as string) || '30s'
    return { executor, vus, duration, label: `${vus} VUs / ${duration}` }
  }
  if (executor === 'ramping-vus') {
    const stages = (props.stages as Stage[]) || []
    const peak = stages.reduce((max, s) => Math.max(max, s.target), 0)
    const totalDur = stages.reduce((sum, s) => sum + parseDurationToSeconds(s.duration), 0)
    return {
      executor,
      vus: peak,
      duration: formatSecondsToDuration(totalDur),
      label: `Ramping to ${peak} VUs / ${formatSecondsToDuration(totalDur)}`,
    }
  }
  if (executor === 'constant-arrival-rate') {
    const rate = (props.rate as number) || 1
    const duration = (props.duration as string) || '1m'
    return { executor, vus: rate, duration, label: `${rate} rps / ${duration}` }
  }

  return { executor, vus: 10, duration: '30s', label: 'Custom' }
}

function templateToParams(props: Record<string, unknown>): RunParameters {
  const executor = props.executor as string

  if (executor === 'constant-vus') {
    return {
      vus_target: (props.vus as number) || 10,
      duration_seconds: parseDurationToSeconds((props.duration as string) || '30s'),
    }
  }

  if (executor === 'ramping-vus') {
    const stages = (props.stages as Stage[]) || []
    const peak = stages.reduce((max, s) => Math.max(max, s.target), 0)
    const totalDur = stages.reduce((sum, s) => sum + parseDurationToSeconds(s.duration), 0)
    return { vus_target: peak, duration_seconds: totalDur }
  }

  if (executor === 'constant-arrival-rate') {
    return {
      vus_target: (props.preAllocatedVUs as number) || 2,
      duration_seconds: parseDurationToSeconds((props.duration as string) || '1m'),
      rps_target: (props.rate as number) || 1,
    }
  }

  return { vus_target: 10, duration_seconds: 30 }
}

export function RunConfigDialog({
  open,
  onClose,
  onConfirm,
  planScenarioProperties,
}: RunConfigDialogProps) {
  const [selected, setSelected] = useState<Selection>('plan-defaults')
  const [vus, setVus] = useState(10)
  const [durationSeconds, setDurationSeconds] = useState(30)
  const [rpsTarget, setRpsTarget] = useState<number | undefined>(undefined)

  // Stages state for ramping-vus editing
  const [stages, setStages] = useState<Stage[]>([{ duration: '2m', target: 50 }])

  // Arrival rate state
  const [arrivalRate, setArrivalRate] = useState(1)
  const [arrivalDuration, setArrivalDuration] = useState('1m')
  const [preAllocatedVUs, setPreAllocatedVUs] = useState(2)

  const planDefaults = useMemo(
    () => extractPlanDefaults(planScenarioProperties),
    [planScenarioProperties],
  )

  // Track which executor type the current selection uses
  const selectedExecutor = useMemo(() => {
    if (selected === 'plan-defaults') return planDefaults.executor
    const t = TEMPLATES.find((t) => t.name === selected)
    return (t?.scenarioProperties.executor as string) || 'constant-vus'
  }, [selected, planDefaults])

  function handleSelect(sel: Selection, template?: TemplateConfig) {
    setSelected(sel)
    if (sel === 'plan-defaults') {
      if (planScenarioProperties) {
        const p = templateToParams(planScenarioProperties)
        setVus(p.vus_target)
        setDurationSeconds(p.duration_seconds)
        setRpsTarget(p.rps_target)
        // Populate stages if ramping
        if (planScenarioProperties.executor === 'ramping-vus') {
          setStages((planScenarioProperties.stages as Stage[]) || [])
        }
        if (planScenarioProperties.executor === 'constant-arrival-rate') {
          setArrivalRate((planScenarioProperties.rate as number) || 1)
          setArrivalDuration((planScenarioProperties.duration as string) || '1m')
          setPreAllocatedVUs((planScenarioProperties.preAllocatedVUs as number) || 2)
        }
      }
    } else if (template) {
      const props = template.scenarioProperties
      const executor = props.executor as string
      const p = templateToParams(props)
      setVus(p.vus_target)
      setDurationSeconds(p.duration_seconds)
      setRpsTarget(p.rps_target)
      if (executor === 'ramping-vus') {
        setStages((props.stages as Stage[]) || [])
      }
      if (executor === 'constant-arrival-rate') {
        setArrivalRate((props.rate as number) || 1)
        setArrivalDuration((props.duration as string) || '1m')
        setPreAllocatedVUs((props.preAllocatedVUs as number) || 2)
      }
    }
  }

  function handleConfirm() {
    if (selectedExecutor === 'ramping-vus') {
      const peak = stages.reduce((max, s) => Math.max(max, s.target), 0)
      const totalDur = stages.reduce((sum, s) => sum + parseDurationToSeconds(s.duration), 0)
      onConfirm({ vus_target: peak, duration_seconds: totalDur })
    } else if (selectedExecutor === 'constant-arrival-rate') {
      onConfirm({
        vus_target: preAllocatedVUs,
        duration_seconds: parseDurationToSeconds(arrivalDuration),
        rps_target: arrivalRate,
      })
    } else {
      onConfirm({
        vus_target: vus,
        duration_seconds: durationSeconds,
        rps_target: rpsTarget,
      })
    }
  }

  function updateStage(index: number, field: 'duration' | 'target', value: string | number) {
    setStages((prev) =>
      prev.map((s, i) => (i === index ? { ...s, [field]: value } : s)),
    )
  }

  function addStage() {
    const lastTarget = stages.length > 0 ? stages[stages.length - 1].target : 0
    setStages((prev) => [...prev, { duration: '2m', target: lastTarget }])
  }

  function removeStage(index: number) {
    if (stages.length <= 1) return
    setStages((prev) => prev.filter((_, i) => i !== index))
  }

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="mx-4 max-h-[90vh] w-full max-w-[960px] overflow-y-auto rounded-xl border border-slate-800 bg-slate-950 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold text-slate-100">Run Configuration</h2>
            <p className="mt-0.5 text-sm text-slate-400">
              Choose a test type and adjust parameters before starting
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
              <path d="M15 5L5 15M5 5l10 10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        {/* "Use plan defaults" card */}
        <div className="px-6 pt-5">
          <button
            onClick={() => handleSelect('plan-defaults')}
            className={`flex w-full items-center gap-4 rounded-lg border px-4 py-3 text-left transition-colors ${
              selected === 'plan-defaults'
                ? 'border-teal-500 bg-teal-500/10 ring-1 ring-teal-500/30'
                : 'border-slate-700 bg-slate-900/50 hover:border-slate-600 hover:bg-slate-900'
            }`}
          >
            <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-slate-700 bg-slate-800">
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                <path d="M10 3v14M5 8l5-5 5 5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-teal-400" />
              </svg>
            </div>
            <div>
              <p className="text-sm font-medium text-slate-200">Use Plan Defaults</p>
              <p className="text-xs text-slate-500">{planDefaults.label}</p>
            </div>
          </button>
        </div>

        {/* Template grid */}
        <div className="grid grid-cols-1 gap-3 px-6 pt-4 sm:grid-cols-2 lg:grid-cols-3">
          {TEMPLATES.map((template) => (
            <button
              key={template.name}
              onClick={() => handleSelect(template.name, template)}
              className={`flex flex-col overflow-hidden rounded-lg border text-left transition-colors ${
                selected === template.name
                  ? 'border-teal-500 ring-1 ring-teal-500/30'
                  : 'border-slate-800 hover:border-slate-700'
              } bg-slate-900`}
            >
              <div className="border-b border-slate-800 px-3 pt-3 pb-2">
                <LoadCurveSvg path={template.svgPath} name={template.name} />
              </div>
              <div className="p-3">
                <h3 className="text-sm font-bold text-slate-100">{template.name}</h3>
                <p className="mt-0.5 text-xs text-slate-400">{template.description}</p>
              </div>
            </button>
          ))}
        </div>

        {/* Parameter panel */}
        <div className="border-t border-slate-800 mt-4 px-6 py-4">
          <h3 className="mb-3 text-sm font-medium text-slate-300">Parameters</h3>

          {selectedExecutor === 'constant-vus' && (
            <div className="flex gap-4">
              <label className="flex flex-col gap-1">
                <span className="text-xs text-slate-500">Virtual Users</span>
                <input
                  type="number"
                  min={1}
                  value={vus}
                  onChange={(e) => setVus(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-32 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-xs text-slate-500">Duration (seconds)</span>
                <input
                  type="number"
                  min={1}
                  value={durationSeconds}
                  onChange={(e) => setDurationSeconds(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-32 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                />
              </label>
            </div>
          )}

          {selectedExecutor === 'ramping-vus' && (
            <div className="space-y-2">
              <div className="grid grid-cols-[1fr_1fr_auto] gap-2 text-xs text-slate-500">
                <span>Duration</span>
                <span>Target VUs</span>
                <span className="w-8" />
              </div>
              {stages.map((stage, i) => (
                <div key={i} className="grid grid-cols-[1fr_1fr_auto] gap-2">
                  <input
                    type="text"
                    value={stage.duration}
                    onChange={(e) => updateStage(i, 'duration', e.target.value)}
                    className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                    placeholder="2m"
                  />
                  <input
                    type="number"
                    min={0}
                    value={stage.target}
                    onChange={(e) => updateStage(i, 'target', Math.max(0, parseInt(e.target.value) || 0))}
                    className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                  />
                  <button
                    onClick={() => removeStage(i)}
                    disabled={stages.length <= 1}
                    className="w-8 rounded-md text-slate-500 hover:bg-slate-800 hover:text-slate-300 disabled:opacity-30"
                  >
                    &times;
                  </button>
                </div>
              ))}
              <button
                onClick={addStage}
                className="rounded-md border border-dashed border-slate-700 px-3 py-1 text-xs text-slate-400 hover:border-slate-600 hover:text-slate-300"
              >
                + Add stage
              </button>
            </div>
          )}

          {selectedExecutor === 'constant-arrival-rate' && (
            <div className="flex flex-wrap gap-4">
              <label className="flex flex-col gap-1">
                <span className="text-xs text-slate-500">Rate (rps)</span>
                <input
                  type="number"
                  min={1}
                  value={arrivalRate}
                  onChange={(e) => setArrivalRate(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-32 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-xs text-slate-500">Duration</span>
                <input
                  type="text"
                  value={arrivalDuration}
                  onChange={(e) => setArrivalDuration(e.target.value)}
                  className="w-32 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                  placeholder="1h"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-xs text-slate-500">Pre-allocated VUs</span>
                <input
                  type="number"
                  min={1}
                  value={preAllocatedVUs}
                  onChange={(e) => setPreAllocatedVUs(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-32 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 focus:border-teal-500 focus:outline-none"
                />
              </label>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 border-t border-slate-800 px-6 py-4">
          <button
            onClick={onClose}
            className="rounded-md bg-slate-800 px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-700"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            className="rounded-md bg-teal-600 px-4 py-2 text-sm font-medium text-white hover:bg-teal-500"
          >
            Run
          </button>
        </div>
      </div>
    </div>
  )
}
