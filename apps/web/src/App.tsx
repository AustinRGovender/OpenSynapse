import { useEffect, useState } from 'react'
import { OpenSynapseClient, type Plan } from '@opensynapse/api-client'
import { PlanBuilder } from './components/plan-builder/PlanBuilder'
import { RunView } from './components/run-view/RunView'
import { RunsListPage } from './pages/RunsListPage'
import { ComparisonPage } from './pages/ComparisonPage'
import { PlaygroundPage } from './pages/PlaygroundPage'
import type { Run } from './stores/run-store'

const client = new OpenSynapseClient('/api/v1')

function useHash() {
  const [hash, setHash] = useState(window.location.hash)
  useEffect(() => {
    const handler = () => setHash(window.location.hash)
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, [])
  return hash
}

function App() {
  const hash = useHash()

  // Route: #/compare?ids=id1,id2,id3 → comparison view
  const compareMatch = hash.match(/^#\/compare\?ids=(.+)$/)
  if (compareMatch) {
    const ids = compareMatch[1].split(',').filter(Boolean)
    return (
      <ComparisonPage
        ids={ids}
        onBack={() => (window.location.hash = '#/runs')}
      />
    )
  }

  // Route: #/playground → playground
  if (hash === '#/playground') {
    return <PlaygroundPage onBack={() => (window.location.hash = '')} />
  }

  // Route: #/runs/:id → run view
  const runMatch = hash.match(/^#\/runs\/([^/]+)$/)
  if (runMatch) {
    return <RunView runId={runMatch[1]} onBack={() => (window.location.hash = '#/runs')} />
  }

  // Route: #/runs → runs list
  if (hash === '#/runs') {
    return <RunsListPage onBack={() => (window.location.hash = '')} />
  }

  // Route: #/plans/:id → builder
  const planMatch = hash.match(/^#\/plans\/(.+)$/)
  if (planMatch) {
    return (
      <PlanBuilder
        planId={planMatch[1]}
        onBack={() => (window.location.hash = '')}
      />
    )
  }

  // Default: plans list
  return <PlansListPage />
}

function PlansListPage() {
  const [health, setHealth] = useState<string>('checking...')
  const [plans, setPlans] = useState<Plan[]>([])
  const [runs, setRuns] = useState<Run[]>([])
  const [newPlanName, setNewPlanName] = useState('')

  useEffect(() => {
    client
      .health()
      .then((data) => setHealth(data.status))
      .catch(() => setHealth('unreachable'))
    loadPlans()
    loadRuns()
  }, [])

  async function loadPlans() {
    try {
      const result = await client.listPlans()
      setPlans(result.items)
    } catch {
      // API may not be available yet
    }
  }

  async function loadRuns() {
    try {
      const res = await fetch('/api/v1/runs?limit=10')
      if (res.ok) {
        const data = await res.json()
        setRuns(data.items ?? [])
      }
    } catch {
      // API may not be available yet
    }
  }

  async function createPlan() {
    if (!newPlanName.trim()) return
    const plan = await client.createPlan({
      name: newPlanName,
      description: '',
      tags: [],
      root: {
        id: crypto.randomUUID(),
        type: 'plan',
        name: newPlanName,
        enabled: true,
        properties: {},
        children: [],
      },
    })
    setNewPlanName('')
    window.location.hash = `#/plans/${plan.id}`
  }

  async function deletePlan(id: string) {
    await client.deletePlan(id)
    loadPlans()
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <header className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-semibold tracking-tight">OpenSynapse</h1>
          <nav className="flex items-center gap-1">
            <span className="rounded bg-teal-600/20 px-2.5 py-1 text-xs font-medium text-teal-400">
              Plans
            </span>
            <a
              href="#/runs"
              className="rounded px-2.5 py-1 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200"
            >
              Runs
            </a>
            <a
              href="#/playground"
              className="rounded px-2.5 py-1 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200"
            >
              Playground
            </a>
          </nav>
        </div>
        <span className="rounded bg-slate-900 px-3 py-1 font-mono text-xs text-teal-400">
          {health}
        </span>
      </header>

      <main className="mx-auto max-w-3xl px-6 py-8">
        <div className="flex gap-3">
          <input
            type="text"
            value={newPlanName}
            onChange={(e) => setNewPlanName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && createPlan()}
            placeholder="New test plan name..."
            className="flex-1 rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:border-teal-500"
          />
          <button
            onClick={createPlan}
            className="rounded-md bg-teal-600 px-4 py-2 text-sm font-medium text-white hover:bg-teal-500"
          >
            Create plan
          </button>
        </div>

        <div className="mt-6 space-y-2">
          {plans.length === 0 && (
            <p className="text-sm text-slate-500">No plans yet. Create one above.</p>
          )}
          {plans.map((plan) => (
            <div
              key={plan.id}
              className="flex cursor-pointer items-center justify-between rounded-lg border border-slate-800 bg-slate-900 px-4 py-3 hover:border-slate-700"
              onClick={() => (window.location.hash = `#/plans/${plan.id}`)}
            >
              <div>
                <p className="text-sm font-medium">{plan.name}</p>
                <p className="text-xs text-slate-500">
                  v{plan.version} &middot;{' '}
                  {new Date(plan.created_at).toLocaleDateString()}
                </p>
              </div>
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  deletePlan(plan.id)
                }}
                className="rounded px-2 py-1 text-xs text-slate-400 hover:bg-slate-800 hover:text-red-400"
              >
                Delete
              </button>
            </div>
          ))}
        </div>

        {/* Recent Runs */}
        <div className="mt-10">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-slate-300">Recent Runs</h2>
            <a
              href="#/runs"
              className="text-xs text-teal-400 hover:text-teal-300"
            >
              View all &rarr;
            </a>
          </div>
          <div className="space-y-2">
            {runs.length === 0 && (
              <p className="text-sm text-slate-500">No runs yet. Open a plan and click Run.</p>
            )}
            {runs.map((run) => {
              const statusColors: Record<string, string> = {
                pending: 'bg-yellow-500/20 text-yellow-400',
                running: 'bg-teal-500/20 text-teal-400',
                completed: 'bg-green-500/20 text-green-400',
                failed: 'bg-red-500/20 text-red-400',
                cancelled: 'bg-slate-500/20 text-slate-400',
              }
              const badge = statusColors[run.status] || 'bg-slate-500/20 text-slate-400'
              return (
                <div
                  key={run.id}
                  className="flex cursor-pointer items-center justify-between rounded-lg border border-slate-800 bg-slate-900 px-4 py-3 hover:border-slate-700"
                  onClick={() => (window.location.hash = `#/runs/${run.id}`)}
                >
                  <div className="flex items-center gap-3">
                    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge}`}>
                      {run.status}
                    </span>
                    <span className="font-mono text-sm text-slate-300">
                      {run.id.slice(0, 8)}
                    </span>
                  </div>
                  <span className="text-xs text-slate-500">
                    {run.started_at
                      ? new Date(run.started_at).toLocaleString()
                      : 'Not started'}
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      </main>
    </div>
  )
}

export default App
