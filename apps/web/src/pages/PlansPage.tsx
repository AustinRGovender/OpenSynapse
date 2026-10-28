import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Environment, ImportLog, Plan, Run } from '@opensynapse/api-client'
import { api, authFetch, errorMessage, unwrap } from '../api'
import { TemplateGallery } from '../components/templates/TemplateGallery'
import { Badge, Button, Card, ConfirmDialog, EmptyState, Modal, PageHeader, Spinner, StatusBadge, TextInput } from '../components/ui/primitives'
import { canEdit, useAuthStore } from '../stores/auth-store'
import { useToast } from '../stores/ui-store'
import { formatAgo } from '../utils/format'
import { navigate } from '../utils/navigate'

export function PlansPage() {
  const user = useAuthStore((s) => s.user)
  const editable = canEdit(user)
  const toast = useToast()
  const [plans, setPlans] = useState<Plan[] | null>(null)
  const [envs, setEnvs] = useState<Environment[]>([])
  const [runs, setRuns] = useState<Run[]>([])
  const [query, setQuery] = useState('')
  const [galleryOpen, setGalleryOpen] = useState(false)
  const [toDelete, setToDelete] = useState<Plan | null>(null)
  const [importLog, setImportLog] = useState<{ plan: Plan; log: ImportLog } | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const load = useCallback(
    () =>
      Promise.all([
        unwrap(api.GET('/api/v1/plans', { params: { query: { limit: '500' } } })),
        unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } })),
        unwrap(api.GET('/api/v1/runs', { params: { query: { limit: '8' } } })),
      ])
        .then(([p, e, r]) => {
          setPlans(p.items)
          setEnvs(e.items)
          setRuns(r.items)
        })
        .catch((err) => {
          toast(`Could not load plans: ${errorMessage(err)}`, 'error')
          setPlans([])
        }),
    [toast],
  )

  useEffect(() => {
    load()
  }, [load])

  const envName = useMemo(() => new Map(envs.map((e) => [e.id, e.name])), [envs])
  // New plans target the same test-API environment as the built-in plans.
  const defaultEnv = useMemo(() => plans?.find((p) => p.built_in && p.default_environment_id)?.default_environment_id ?? null, [plans])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!plans) return []
    return plans.filter((p) => !q || p.name.toLowerCase().includes(q) || p.description.toLowerCase().includes(q) || p.tags.some((t) => t.includes(q)))
  }, [plans, query])

  async function confirmDelete() {
    if (!toDelete) return
    try {
      await unwrap(api.DELETE('/api/v1/plans/{id}', { params: { path: { id: toDelete.id } } }))
      toast(`Deleted “${toDelete.name}”`, 'success')
      setToDelete(null)
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
      setToDelete(null)
    }
  }

  async function importJmx(file: File) {
    const form = new FormData()
    form.append('file', file)
    try {
      const res = await authFetch('/api/v1/plans/import/jmx', { method: 'POST', body: form })
      const data = (await res.json()) as { plan: Plan; log: ImportLog }
      setImportLog(data)
      load()
    } catch (e) {
      toast(`Import failed: ${errorMessage(e)}`, 'error')
    } finally {
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const onlyBuiltIns = plans !== null && plans.every((p) => p.built_in)

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <PageHeader
        title="Test plans"
        description="A plan describes what virtual users do and how much load to apply. Open one to edit it in the builder, then press Run."
        actions={
          editable && (
            <>
              <input ref={fileRef} type="file" accept=".jmx,application/xml,text/xml" className="hidden" onChange={(e) => e.target.files?.[0] && importJmx(e.target.files[0])} />
              <Button onClick={() => fileRef.current?.click()}>Import JMeter (.jmx)</Button>
              <Button variant="primary" onClick={() => setGalleryOpen(true)}>New test</Button>
            </>
          )
        }
      />

      {onlyBuiltIns && (
        <Card className="mb-6 border-teal-500/30 bg-teal-500/5 p-4">
          <p className="text-sm font-medium text-teal-300">Start here</p>
          <p className="mt-1 text-sm text-slate-300">
            The plans below are built in and target the four test APIs that ship with OpenSynapse (echo, e-commerce, slow and
            error APIs). Start them with <code className="text-slate-100">docker compose -f user-testing/docker-compose.yml up -d</code>, open{' '}
            <b>Echo API — Smoke Test</b> and press <b>Run</b>. When you are ready, click <b>New test</b> to build your own.
          </p>
        </Card>
      )}

      <div className="mb-3 flex items-center gap-3">
        <TextInput placeholder="Search plans by name, description or tag" value={query} onChange={(e) => setQuery(e.target.value)} className="max-w-sm" aria-label="Search plans" />
        {plans && <span className="text-xs text-slate-500">{filtered.length} of {plans.length}</span>}
      </div>

      {plans === null ? (
        <Spinner />
      ) : filtered.length === 0 ? (
        <EmptyState title={plans.length === 0 ? 'No plans yet' : 'No plans match your search'} action={editable && plans.length === 0 && <Button variant="primary" onClick={() => setGalleryOpen(true)}>New test</Button>}>
          {plans.length === 0 && 'Create a plan from a template, import a JMeter file, or generate one with the crawler.'}
        </EmptyState>
      ) : (
        <ul className="space-y-2">
          {filtered.map((plan) => (
            <li key={plan.id}>
              <Card className="flex items-center gap-4 px-4 py-3 hover:border-slate-700">
                <a href={`#/plans/${plan.id}`} className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium text-slate-100">{plan.name}</span>
                    {plan.built_in && <Badge tone="accent">Built-in</Badge>}
                    {plan.tags.filter((t) => t !== 'built-in').slice(0, 3).map((t) => <Badge key={t}>{t}</Badge>)}
                  </div>
                  {plan.description && <p className="mt-0.5 truncate text-xs text-slate-400">{plan.description}</p>}
                  <p className="mt-1 text-[11px] text-slate-500">
                    v{plan.version} · updated {formatAgo(plan.updated_at)}
                    {plan.default_environment_id && envName.get(plan.default_environment_id) && <> · environment: {envName.get(plan.default_environment_id)}</>}
                  </p>
                </a>
                <Button size="sm" onClick={() => navigate(`#/plans/${plan.id}`)}>Open</Button>
                {editable && !plan.built_in && (
                  <Button size="sm" variant="ghost" onClick={() => setToDelete(plan)} aria-label={`Delete ${plan.name}`}>Delete</Button>
                )}
              </Card>
            </li>
          ))}
        </ul>
      )}

      <section className="mt-10">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-300">Recent runs</h2>
          <a href="#/runs" className="text-xs text-teal-400 hover:text-teal-300">All runs →</a>
        </div>
        {runs.length === 0 ? (
          <p className="text-sm text-slate-500">No runs yet. Open a plan and press Run.</p>
        ) : (
          <ul className="space-y-1.5">
            {runs.map((run) => (
              <li key={run.id}>
                <a href={`#/runs/${run.id}`} className="flex items-center gap-3 rounded-md border border-slate-800 bg-slate-900 px-3 py-2 hover:border-slate-700">
                  <StatusBadge status={run.status} />
                  <span className="flex-1 truncate text-sm text-slate-200">{run.plan_name || 'Deleted plan'}</span>
                  {run.summary && (
                    <span className="hidden text-xs text-slate-400 sm:inline">
                      p95 {run.summary.p95_ms.toFixed(0)} ms · {run.summary.error_rate.toFixed(2)}% errors
                    </span>
                  )}
                  <span className="text-xs text-slate-500">{formatAgo(run.created_at)}</span>
                </a>
              </li>
            ))}
          </ul>
        )}
      </section>

      <TemplateGallery open={galleryOpen} onClose={() => setGalleryOpen(false)} defaultEnvironmentId={defaultEnv} />
      <ConfirmDialog
        open={toDelete !== null}
        title="Delete plan"
        message={<>Delete <b>{toDelete?.name}</b>? Its runs stay in history.</>}
        onConfirm={confirmDelete}
        onCancel={() => setToDelete(null)}
      />
      <Modal
        open={importLog !== null}
        onClose={() => setImportLog(null)}
        title="JMeter import"
        footer={
          <>
            <Button onClick={() => setImportLog(null)}>Close</Button>
            <Button variant="primary" onClick={() => importLog && navigate(`#/plans/${importLog.plan.id}`)}>Open plan</Button>
          </>
        }
      >
        {importLog && (
          <div className="space-y-3 text-sm">
            <p className="text-slate-300">Created <b>{importLog.plan.name}</b>. Review it in the builder before running.</p>
            {(['imported', 'approximated', 'unsupported'] as const).map((k) => {
              const items = (importLog.log[k] as string[] | null) ?? []
              if (items.length === 0) return null
              return (
                <div key={k}>
                  <p className={`text-xs font-semibold uppercase tracking-wide ${k === 'imported' ? 'text-emerald-400' : k === 'approximated' ? 'text-amber-400' : 'text-red-400'}`}>{k} ({items.length})</p>
                  <ul className="mt-1 list-inside list-disc text-xs text-slate-400">
                    {items.map((i) => <li key={i}>{i}</li>)}
                  </ul>
                </div>
              )
            })}
          </div>
        )}
      </Modal>
    </div>
  )
}
