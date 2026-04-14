import { useEffect, useState } from 'react'

function App() {
  const [health, setHealth] = useState<string>('checking...')

  useEffect(() => {
    fetch('/health')
      .then((res) => res.json())
      .then((data) => setHealth(data.status))
      .catch(() => setHealth('unreachable'))
  }, [])

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-slate-950 text-slate-100">
      <h1 className="text-3xl font-semibold tracking-tight">OpenSynapse</h1>
      <p className="mt-2 text-sm text-slate-400">
        Self-hosted performance testing platform
      </p>
      <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900 px-6 py-4">
        <p className="text-xs font-medium uppercase tracking-wider text-slate-500">
          Control plane
        </p>
        <p className="mt-1 font-mono text-sm text-teal-400">{health}</p>
      </div>
    </div>
  )
}

export default App
