import { useEffect, useState, useCallback } from 'react'
import { usePlanStore } from '../../stores/plan-store'
import { NodeTree } from './NodeTree'
import { FlowCanvas } from './FlowCanvas'
import { PropertyPanel } from './PropertyPanel'
import { FragmentLibrary } from '../fragments/FragmentLibrary'

type RunStatus = 'idle' | 'starting' | 'error'

interface PlanBuilderProps {
  planId: string
  onBack: () => void
}

export function PlanBuilder({ planId, onBack }: PlanBuilderProps) {
  const { plan, loading, saving, dirty, loadPlan, savePlan, undo, redo, canUndo, canRedo } =
    usePlanStore()
  const [showScript, setShowScript] = useState(false)
  const [script, setScript] = useState<string | null>(null)
  const [scriptLoading, setScriptLoading] = useState(false)
  const [runStatus, setRunStatus] = useState<RunStatus>('idle')
  const [showFragments, setShowFragments] = useState(false)

  const handleStartRun = useCallback(async () => {
    if (!plan) return
    setRunStatus('starting')
    try {
      const res = await fetch('/api/v1/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plan_id: plan.id }),
      })
      if (res.ok) {
        const run = await res.json()
        window.location.hash = `#/runs/${run.id}`
      } else {
        setRunStatus('error')
        setTimeout(() => setRunStatus('idle'), 3000)
      }
    } catch {
      setRunStatus('error')
      setTimeout(() => setRunStatus('idle'), 3000)
    }
  }, [plan])

  useEffect(() => {
    loadPlan(planId)
  }, [planId, loadPlan])

  // Keyboard shortcuts
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
        e.preventDefault()
        undo()
      }
      if ((e.ctrlKey || e.metaKey) && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) {
        e.preventDefault()
        redo()
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault()
        savePlan()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [undo, redo, savePlan])

  const handleShowScript = useCallback(async () => {
    if (!plan) return
    setShowScript(true)
    setScriptLoading(true)
    try {
      const resp = await fetch(`/api/v1/plans/${plan.id}/compile`, { method: 'POST' })
      if (resp.ok) {
        setScript(await resp.text())
      } else {
        setScript('// Compilation failed: ' + (await resp.text()))
      }
    } catch {
      setScript('// Could not reach the control plane')
    }
    setScriptLoading(false)
  }, [plan])

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950 text-slate-400">
        Loading plan...
      </div>
    )
  }

  if (!plan) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950 text-slate-400">
        Plan not found
      </div>
    )
  }

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
        <span className="text-sm font-medium">{plan.name}</span>
        <span className="text-xs text-slate-500">v{plan.version}</span>
        <span className="text-xs text-slate-600">
          {saving ? 'Saving...' : dirty ? 'Unsaved' : 'Saved'}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={undo}
            disabled={!canUndo()}
            className="rounded px-2 py-1 text-xs text-slate-400 hover:bg-slate-800 disabled:opacity-30"
            title="Undo (Ctrl+Z)"
          >
            Undo
          </button>
          <button
            onClick={redo}
            disabled={!canRedo()}
            className="rounded px-2 py-1 text-xs text-slate-400 hover:bg-slate-800 disabled:opacity-30"
            title="Redo (Ctrl+Y)"
          >
            Redo
          </button>
          <div className="h-4 w-px bg-slate-800" />
          <button
            onClick={() => setShowFragments((v) => !v)}
            className={`rounded px-3 py-1 text-xs font-medium ${
              showFragments
                ? 'bg-teal-600/20 text-teal-400'
                : 'bg-slate-800 text-slate-300 hover:bg-slate-700'
            }`}
          >
            Fragments
          </button>
          <button
            onClick={handleShowScript}
            className="rounded bg-slate-800 px-3 py-1 text-xs font-medium text-slate-300 hover:bg-slate-700"
          >
            Show Script
          </button>
          <button
            onClick={handleStartRun}
            disabled={runStatus === 'starting'}
            className={`rounded px-3 py-1 text-xs font-medium text-white ${
              runStatus === 'error'
                ? 'bg-red-600 hover:bg-red-500'
                : runStatus === 'starting'
                  ? 'bg-teal-700 opacity-70'
                  : 'bg-teal-600 hover:bg-teal-500'
            }`}
          >
            {runStatus === 'starting'
              ? 'Starting...'
              : runStatus === 'error'
                ? 'Failed'
                : 'Run'}
          </button>
        </div>
      </div>

      {/* Three-pane layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left: Node Tree (30%) */}
        <div className="w-[280px] flex-shrink-0 border-r border-slate-800 overflow-hidden">
          <NodeTree />
        </div>

        {/* Centre: Canvas (flexible) */}
        <div className="flex-1 overflow-hidden">
          {showScript ? (
            <div className="flex h-full flex-col">
              <div className="flex items-center justify-between border-b border-slate-800 px-4 py-2">
                <span className="text-xs font-medium text-slate-400">Generated k6 Script</span>
                <button
                  onClick={() => setShowScript(false)}
                  className="rounded px-2 py-1 text-xs text-slate-400 hover:bg-slate-800"
                >
                  Close
                </button>
              </div>
              <pre className="flex-1 overflow-auto bg-slate-900 p-4 font-mono text-xs text-slate-300">
                {scriptLoading ? 'Compiling...' : script || 'No script generated'}
              </pre>
            </div>
          ) : (
            <FlowCanvas />
          )}
        </div>

        {/* Right: Property Panel or Fragment Library (280px) */}
        <div className="w-[280px] flex-shrink-0 border-l border-slate-800 overflow-hidden">
          {showFragments ? (
            <FragmentLibrary onClose={() => setShowFragments(false)} />
          ) : (
            <PropertyPanel />
          )}
        </div>
      </div>
    </div>
  )
}
