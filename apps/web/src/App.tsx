import { useEffect, useState } from 'react'
import { OpenSynapseClient, type Plan } from '@opensynapse/api-client'

const client = new OpenSynapseClient('/api/v1')

function App() {
  const [health, setHealth] = useState<string>('checking...')
  const [plans, setPlans] = useState<Plan[]>([])
  const [newPlanName, setNewPlanName] = useState('')

  useEffect(() => {
    client
      .health()
      .then((data) => setHealth(data.status))
      .catch(() => setHealth('unreachable'))

    loadPlans()
  }, [])

  async function loadPlans() {
    try {
      const result = await client.listPlans()
      setPlans(result.items)
    } catch {
      // API may not be available yet
    }
  }

  async function createPlan() {
    if (!newPlanName.trim()) return
    await client.createPlan({
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
    loadPlans()
  }

  async function deletePlan(id: string) {
    await client.deletePlan(id)
    loadPlans()
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      {/* Header */}
      <header className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
        <h1 className="text-xl font-semibold tracking-tight">OpenSynapse</h1>
        <span className="rounded bg-slate-900 px-3 py-1 font-mono text-xs text-teal-400">
          {health}
        </span>
      </header>

      {/* Main content */}
      <main className="mx-auto max-w-3xl px-6 py-8">
        {/* Create plan */}
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

        {/* Plans list */}
        <div className="mt-6 space-y-2">
          {plans.length === 0 && (
            <p className="text-sm text-slate-500">
              No plans yet. Create one above.
            </p>
          )}
          {plans.map((plan) => (
            <div
              key={plan.id}
              className="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-900 px-4 py-3"
            >
              <div>
                <p className="text-sm font-medium">{plan.name}</p>
                <p className="text-xs text-slate-500">
                  v{plan.version} &middot; {new Date(plan.created_at).toLocaleDateString()}
                </p>
              </div>
              <button
                onClick={() => deletePlan(plan.id)}
                className="rounded px-2 py-1 text-xs text-slate-400 hover:bg-slate-800 hover:text-red-400"
              >
                Delete
              </button>
            </div>
          ))}
        </div>
      </main>
    </div>
  )
}

export default App
