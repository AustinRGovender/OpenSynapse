import { useCallback, useEffect, useState } from 'react'
import type { Environment } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'
import { Badge, Button, Card, ConfirmDialog, EmptyState, Modal, PageHeader, Spinner, TextInput } from '../components/ui/primitives'
import { canEdit, useAuthStore } from '../stores/auth-store'
import { useToast } from '../stores/ui-store'
import { formatAgo } from '../utils/format'

const REDACTED = '********'
const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/

interface Row {
  name: string
  value: string
  secret: boolean
}

function toRows(env: Environment | null): Row[] {
  if (!env) return [{ name: 'BASE_URL', value: 'https://', secret: false }]
  const rows = Object.entries(env.variables).map(([name, v]) => ({ name, value: v.value, secret: v.secret }))
  return rows.length ? rows.sort((a, b) => a.name.localeCompare(b.name)) : [{ name: '', value: '', secret: false }]
}

function Editor({ env, onClose, onSaved }: { env: Environment | null; onClose: () => void; onSaved: () => void }) {
  const toast = useToast()
  const [name, setName] = useState(env?.name ?? '')
  const [rows, setRows] = useState<Row[]>(() => toRows(env))
  const [saving, setSaving] = useState(false)

  const invalid = rows.filter((r) => r.name && !NAME_RE.test(r.name)).map((r) => r.name)
  const dupes = rows.map((r) => r.name).filter((n, i, all) => n && all.indexOf(n) !== i)

  function update(i: number, patch: Partial<Row>) {
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  }

  async function save() {
    setSaving(true)
    const variables = Object.fromEntries(rows.filter((r) => r.name.trim()).map((r) => [r.name.trim(), { value: r.value, secret: r.secret }]))
    try {
      if (env) await unwrap(api.PUT('/api/v1/environments/{id}', { params: { path: { id: env.id } }, body: { name, variables } }))
      else await unwrap(api.POST('/api/v1/environments', { body: { name, variables } }))
      toast(`Saved “${name}”`, 'success')
      onSaved()
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={env ? `Edit ${env.name}` : 'New environment'}
      width="max-w-2xl"
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={save} disabled={saving || !name.trim() || invalid.length > 0 || dupes.length > 0}>{saving ? 'Saving…' : 'Save'}</Button>
        </>
      }
    >
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-slate-400">Name</span>
        <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="Staging" autoFocus />
      </label>
      <p className="mb-2 mt-4 text-xs text-slate-400">
        Reference variables in URLs, headers and bodies as <code className="text-slate-200">{'${NAME}'}</code>. Secret values are encrypted at rest and never shown again; leave
        <code className="mx-1 text-slate-200">{REDACTED}</code>to keep the stored value.
      </p>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-500">
            <th className="pb-1 pr-2 font-medium">Variable</th>
            <th className="pb-1 pr-2 font-medium">Value</th>
            <th className="pb-1 pr-2 font-medium">Secret</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="py-1 pr-2">
                <TextInput aria-label="Variable name" value={r.name} onChange={(e) => update(i, { name: e.target.value })} placeholder="NAME" className={`font-mono ${r.name && !NAME_RE.test(r.name) ? 'border-red-500' : ''}`} />
              </td>
              <td className="py-1 pr-2">
                <TextInput aria-label={`Value of ${r.name || 'variable'}`} type={r.secret ? 'password' : 'text'} value={r.value} onChange={(e) => update(i, { value: e.target.value })} onFocus={(e) => r.secret && r.value === REDACTED && e.target.select()} className="font-mono" />
              </td>
              <td className="py-1 pr-2 text-center">
                <input type="checkbox" aria-label={`${r.name || 'variable'} is secret`} checked={r.secret} onChange={(e) => update(i, { secret: e.target.checked })} className="accent-teal-500" />
              </td>
              <td className="py-1">
                <Button size="sm" variant="ghost" aria-label={`Remove ${r.name || 'row'}`} onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>✕</Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <Button size="sm" className="mt-2" onClick={() => setRows((rs) => [...rs, { name: '', value: '', secret: false }])}>+ Add variable</Button>
      {invalid.length > 0 && <p className="mt-2 text-xs text-red-400">Names must start with a letter or underscore and contain only letters, digits and underscores: {invalid.join(', ')}</p>}
      {dupes.length > 0 && <p className="mt-2 text-xs text-red-400">Duplicate names: {[...new Set(dupes)].join(', ')}</p>}
    </Modal>
  )
}

export function EnvironmentsPage() {
  const user = useAuthStore((s) => s.user)
  const editable = canEdit(user)
  const toast = useToast()
  const [envs, setEnvs] = useState<Environment[] | null>(null)
  const [editing, setEditing] = useState<Environment | 'new' | null>(null)
  const [toDelete, setToDelete] = useState<Environment | null>(null)

  const load = useCallback(
    () =>
      unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } }))
        .then((r) => setEnvs(r.items))
        .catch((e) => {
          toast(errorMessage(e), 'error')
          setEnvs([])
        }),
    [toast],
  )

  useEffect(() => {
    load()
  }, [load])

  async function remove() {
    if (!toDelete) return
    try {
      await unwrap(api.DELETE('/api/v1/environments/{id}', { params: { path: { id: toDelete.id } } }))
      toast(`Deleted “${toDelete.name}”`, 'success')
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setToDelete(null)
    }
  }

  return (
    <div className="mx-auto max-w-4xl px-6 py-8">
      <PageHeader
        title="Environments"
        description="Named sets of variables (base URLs, credentials) so one plan can run against local, staging and production. Pick one when you start a run."
        actions={editable && <Button variant="primary" onClick={() => setEditing('new')}>New environment</Button>}
      />
      {envs === null ? (
        <Spinner />
      ) : envs.length === 0 ? (
        <EmptyState title="No environments yet" />
      ) : (
        <div className="space-y-2">
          {envs.map((env) => {
            const vars = Object.entries(env.variables)
            return (
              <Card key={env.id} className="px-4 py-3">
                <div className="flex items-center gap-3">
                  <span className="text-sm font-medium text-slate-100">{env.name}</span>
                  {env.name.startsWith('Test APIs') && <Badge tone="accent">Bundled test APIs</Badge>}
                  <span className="text-xs text-slate-500">{vars.length} variable{vars.length === 1 ? '' : 's'} · updated {formatAgo(env.updated_at)}</span>
                  {editable && (
                    <div className="ml-auto flex gap-1">
                      <Button size="sm" onClick={() => setEditing(env)}>Edit</Button>
                      <Button size="sm" variant="ghost" onClick={() => setToDelete(env)}>Delete</Button>
                    </div>
                  )}
                </div>
                {vars.length > 0 && (
                  <dl className="mt-2 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-0.5 font-mono text-xs">
                    {vars.slice(0, 8).map(([k, v]) => (
                      <div key={k} className="contents">
                        <dt className="text-slate-400">{k}</dt>
                        <dd className="truncate text-slate-300">{v.secret ? '•••••••• (secret)' : v.value}</dd>
                      </div>
                    ))}
                  </dl>
                )}
              </Card>
            )
          })}
        </div>
      )}
      {editing && <Editor env={editing === 'new' ? null : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load() }} />}
      <ConfirmDialog
        open={toDelete !== null}
        title="Delete environment"
        message={<>Delete <b>{toDelete?.name}</b>? Plans that use it as their default fall back to no environment.</>}
        onConfirm={remove}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
