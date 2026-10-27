import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, authFetch, errorMessage, unwrap } from '../../api'
import { canEdit, useAuthStore } from '../../stores/auth-store'
import { findNode, usePlanStore } from '../../stores/plan-store'
import { useToast } from '../../stores/ui-store'
import { navigate } from '../../utils/navigate'
import { requiredVariables } from '../../utils/variables'
import { FragmentLibrary } from '../fragments/FragmentLibrary'
import { RunConfigDialog, type RunRequest } from '../run-config/RunConfigDialog'
import { Badge, Button, EmptyState, Spinner } from '../ui/primitives'
import { FlowCanvas } from './FlowCanvas'
import { NodeTree } from './NodeTree'
import { PropertyPanel } from './PropertyPanel'

function isTyping(target: EventTarget | null) {
  const el = target as HTMLElement | null
  return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable)
}

export function PlanBuilder({ planId }: { planId: string }) {
  const { plan, loading, notFound, saving, dirty, saveError, loadPlan, savePlan, undo, redo, canUndo, canRedo } = usePlanStore()
  const user = useAuthStore((s) => s.user)
  const editable = canEdit(user)
  const toast = useToast()
  const [script, setScript] = useState<{ text: string; managed: boolean } | null>(null)
  const [showFragments, setShowFragments] = useState(false)
  const [showRun, setShowRun] = useState(false)
  const [starting, setStarting] = useState(false)

  useEffect(() => {
    loadPlan(planId)
  }, [planId, loadPlan])

  const variables = useMemo(() => (plan ? requiredVariables(plan.root) : []), [plan])

  const scenarioProps = useMemo(() => {
    const scenario = plan?.root.children.find((c) => c.type === 'scenario' && c.enabled)
    return (scenario?.properties as Record<string, unknown>) ?? null
  }, [plan])

  // Builder shortcuts (Settings › General lists them).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const s = usePlanStore.getState()
      const mod = e.ctrlKey || e.metaKey
      if (mod && e.key.toLowerCase() === 's') {
        e.preventDefault()
        s.savePlan()
        return
      }
      if (isTyping(e.target)) return
      if (mod && e.key.toLowerCase() === 'z' && !e.shiftKey) { e.preventDefault(); s.undo() }
      else if (mod && (e.key.toLowerCase() === 'y' || (e.key.toLowerCase() === 'z' && e.shiftKey))) { e.preventDefault(); s.redo() }
      else if (!s.selectedNodeId) return
      else if (mod && e.key.toLowerCase() === 'c') s.copyNode(s.selectedNodeId)
      else if (mod && e.key.toLowerCase() === 'x') s.cutNode(s.selectedNodeId)
      else if (mod && e.key.toLowerCase() === 'v') {
        const sel = s.plan && findNode(s.plan.root, s.selectedNodeId)
        if (sel) s.pasteNode(sel.id)
      } else if (mod && e.key.toLowerCase() === 'd') { e.preventDefault(); s.duplicateNode(s.selectedNodeId) }
      else if (e.key === 'Delete') s.removeNode(s.selectedNodeId)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const showScript = useCallback(
    async (managed: boolean) => {
      if (!plan) return
      if (dirty) await savePlan()
      try {
        const res = await authFetch(`/api/v1/plans/${plan.id}/compile${managed ? '?mode=managed' : ''}`, { method: 'POST' })
        setScript({ text: await res.text(), managed })
      } catch (e) {
        setScript({ text: `// ${errorMessage(e)}`, managed })
      }
    },
    [plan, dirty, savePlan],
  )

  async function startRun(req: RunRequest) {
    if (!plan) return
    setStarting(true)
    try {
      if (usePlanStore.getState().dirty) await savePlan()
      const run = await unwrap(api.POST('/api/v1/runs', { body: { plan_id: plan.id, ...req } }))
      setShowRun(false)
      navigate(`#/runs/${run.id}`)
    } catch (e) {
      toast(`Could not start the run: ${errorMessage(e)}`, 'error')
    } finally {
      setStarting(false)
    }
  }

  if (loading) return <div className="p-8"><Spinner label="Loading plan…" /></div>
  if (notFound || !plan) {
    return (
      <div className="mx-auto max-w-lg p-8">
        <EmptyState title="Plan not found" action={<Button onClick={() => navigate('#/plans')}>Back to plans</Button>}>It may have been deleted.</EmptyState>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-11 shrink-0 items-center gap-3 border-b border-slate-800 px-4">
        <a href="#/plans" className="text-sm text-slate-400 hover:text-slate-200">← Plans</a>
        <span className="truncate text-sm font-medium text-slate-100">{plan.name}</span>
        {plan.built_in && <Badge tone="accent">Built-in</Badge>}
        <span className="text-xs text-slate-500">v{plan.version}</span>
        <span className={`text-xs ${saveError ? 'text-red-400' : 'text-slate-500'}`} role="status">
          {saveError ?? (saving ? 'Saving…' : dirty ? 'Unsaved changes' : 'All changes saved')}
        </span>
        <div className="ml-auto flex items-center gap-1.5">
          <Button size="sm" variant="ghost" onClick={undo} disabled={!canUndo()} title="Undo (Ctrl+Z)">Undo</Button>
          <Button size="sm" variant="ghost" onClick={redo} disabled={!canRedo()} title="Redo (Ctrl+Y)">Redo</Button>
          <Button size="sm" onClick={() => setShowFragments((v) => !v)} aria-pressed={showFragments}>Fragments</Button>
          <Button size="sm" onClick={() => (script ? setScript(null) : showScript(false))} aria-pressed={script !== null}>{script ? 'Hide script' : 'Show script'}</Button>
          {editable && <Button size="sm" variant="primary" onClick={() => setShowRun(true)}>Run</Button>}
        </div>
      </div>

      <div className="flex min-h-0 flex-1">
        <div className="w-72 shrink-0 overflow-hidden border-r border-slate-800">
          <NodeTree />
        </div>
        <div className="min-w-0 flex-1 overflow-hidden">
          {script ? (
            <div className="flex h-full flex-col">
              <div className="flex items-center gap-3 border-b border-slate-800 px-4 py-2 text-xs">
                <span className="font-medium text-slate-300">Generated k6 script</span>
                <label className="flex items-center gap-1.5 text-slate-400">
                  <input type="checkbox" checked={script.managed} onChange={(e) => showScript(e.target.checked)} className="accent-teal-500" />
                  Show the script OpenSynapse runs (with live-control glue)
                </label>
                <Button size="sm" variant="ghost" className="ml-auto" onClick={() => navigator.clipboard?.writeText(script.text).then(() => toast('Copied', 'success'))}>Copy</Button>
              </div>
              <p className="border-b border-slate-800 px-4 py-1.5 text-[11px] text-slate-500">
                {script.managed
                  ? 'VU-based scenarios become externally-controlled so the load profile can be changed live; an internal control scenario feeds RPS limits and pause/stop.'
                  : 'Portable script: runs with stock k6 as-is (k6 run script.js). Variables come from environment variables, e.g. k6 run -e BASE_URL=… script.js.'}
              </p>
              <pre className="flex-1 overflow-auto bg-slate-900 p-4 font-mono text-xs text-slate-300">{script.text}</pre>
            </div>
          ) : (
            <FlowCanvas />
          )}
        </div>
        <div className="w-80 shrink-0 overflow-hidden border-l border-slate-800">
          {showFragments ? <FragmentLibrary onClose={() => setShowFragments(false)} /> : <PropertyPanel />}
        </div>
      </div>

      <RunConfigDialog
        open={showRun}
        onClose={() => setShowRun(false)}
        onConfirm={startRun}
        planScenarioProperties={scenarioProps}
        defaultEnvironmentId={plan.default_environment_id ?? null}
        starting={starting}
        requiredVariables={variables}
      />
    </div>
  )
}
