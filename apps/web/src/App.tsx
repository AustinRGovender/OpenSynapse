import { lazy, Suspense, useEffect } from 'react'
import { AppShell } from './components/shell/AppShell'
import { ConnectionError, LoginScreen, SetupScreen } from './components/shell/AuthScreens'
import { ErrorBoundary } from './components/shell/ErrorBoundary'
import { Toaster } from './components/shell/Toaster'
import { Spinner } from './components/ui/primitives'
import { match, useRoute } from './router'
import { useAuthStore } from './stores/auth-store'
import { applyAppearance, useUIStore } from './stores/ui-store'

// Pages load on demand so the first screen stays small.
const PlansPage = lazy(() => import('./pages/PlansPage').then((m) => ({ default: m.PlansPage })))
const PlanBuilder = lazy(() => import('./components/plan-builder/PlanBuilder').then((m) => ({ default: m.PlanBuilder })))
const RunsListPage = lazy(() => import('./pages/RunsListPage').then((m) => ({ default: m.RunsListPage })))
const RunView = lazy(() => import('./components/run-view/RunView').then((m) => ({ default: m.RunView })))
const ComparisonPage = lazy(() => import('./pages/ComparisonPage').then((m) => ({ default: m.ComparisonPage })))
const EnvironmentsPage = lazy(() => import('./pages/EnvironmentsPage').then((m) => ({ default: m.EnvironmentsPage })))
const PlaygroundPage = lazy(() => import('./pages/PlaygroundPage').then((m) => ({ default: m.PlaygroundPage })))
const CrawlerPage = lazy(() => import('./pages/CrawlerPage').then((m) => ({ default: m.CrawlerPage })))
const SettingsPage = lazy(() => import('./pages/SettingsPage').then((m) => ({ default: m.SettingsPage })))

function Page({ path, query }: { path: string; query: URLSearchParams }) {
  let p: Record<string, string> | null
  if ((p = match('/plans/:id', path))) return <PlanBuilder planId={p.id} />
  if ((p = match('/runs/:id', path))) return <RunView runId={p.id} />
  if (path === '/runs') return <RunsListPage />
  if (path === '/compare') return <ComparisonPage ids={(query.get('ids') ?? '').split(',').filter(Boolean)} />
  if (path === '/environments') return <EnvironmentsPage />
  if (path === '/playground') return <PlaygroundPage />
  if (path === '/crawler') return <CrawlerPage />
  if ((p = match('/settings/:tab', path))) return <SettingsPage tab={p.tab} />
  if (path === '/settings') return <SettingsPage tab="general" />
  return <PlansPage />
}

function App() {
  const { path, query } = useRoute()
  const status = useAuthStore((s) => s.status)
  const error = useAuthStore((s) => s.error)
  const bootstrap = useAuthStore((s) => s.bootstrap)
  const loadSettings = useUIStore((s) => s.loadSettings)
  const loadSystem = useUIStore((s) => s.loadSystem)

  useEffect(() => {
    applyAppearance('dark', 'comfortable')
    bootstrap()
  }, [bootstrap])

  useEffect(() => {
    if (status === 'ready') {
      loadSettings()
      loadSystem()
    }
  }, [status, loadSettings, loadSystem])

  if (status === 'loading') {
    return (
      <div className="flex h-full items-center justify-center bg-slate-950">
        <Spinner label="Connecting to OpenSynapse…" />
      </div>
    )
  }
  if (status === 'error') return <ConnectionError message={error ?? 'Unknown error'} onRetry={bootstrap} />
  if (status === 'login') return <LoginScreen />
  if (status === 'setup') return <SetupScreen />

  return (
    <>
      <AppShell path={path}>
        <ErrorBoundary resetKey={path}>
          <Suspense
            fallback={
              <div className="p-8">
                <Spinner />
              </div>
            }
          >
            <Page path={path} query={query} />
          </Suspense>
        </ErrorBoundary>
      </AppShell>
      <Toaster />
    </>
  )
}

export default App
