import { useCallback, useEffect, useState } from 'react'
import type { APIToken, Environment, Settings, User } from '@opensynapse/api-client'
import { api, downloadFile, errorMessage, unwrap } from '../api'
import { TEMPLATES } from '../components/templates/template-data'
import { Badge, Button, Card, ConfirmDialog, Field, Modal, PageHeader, Select, Spinner, TextInput } from '../components/ui/primitives'
import { MODEL_OPTIONS, PROVIDER_LABELS, useAIStore } from '../stores/ai-store'
import { isAdmin, useAuthStore } from '../stores/auth-store'
import { useToast, useUIStore } from '../stores/ui-store'
import { formatAgo, formatBytes } from '../utils/format'
import { navigate } from '../utils/navigate'

type TabKey = 'general' | 'appearance' | 'ai' | 'integrations' | 'storage' | 'users' | 'account' | 'about'

function SaveBar({ onSave, saving, readOnly }: { onSave: () => void; saving: boolean; readOnly: boolean }) {
  if (readOnly) return <p className="mt-4 text-xs text-slate-500">Only admins can change these settings.</p>
  return (
    <div className="mt-6">
      <Button variant="primary" onClick={onSave} disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
    </div>
  )
}

function useSettingsDraft() {
  const settings = useUIStore((s) => s.settings)
  const saveSettings = useUIStore((s) => s.saveSettings)
  const toast = useToast()
  const [draft, setDraft] = useState<Settings | null>(settings)
  const [saving, setSaving] = useState(false)
  // Pick up settings once they finish loading.
  if (draft === null && settings !== null) setDraft(settings)
  async function save() {
    if (!draft) return
    setSaving(true)
    try {
      setDraft(await saveSettings(draft))
      toast('Settings saved', 'success')
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setSaving(false)
    }
  }
  return { draft, setDraft, save, saving }
}

function GeneralTab({ readOnly }: { readOnly: boolean }) {
  const { draft, setDraft, save, saving } = useSettingsDraft()
  const [envs, setEnvs] = useState<Environment[]>([])
  useEffect(() => {
    unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } })).then((r) => setEnvs(r.items)).catch(() => {})
  }, [])
  if (!draft) return <Spinner />
  const g = draft.general
  return (
    <div className="max-w-lg space-y-4">
      <Field label="Default environment for new plans" hint="Plans created without an environment start with this one.">
        <Select value={g.default_environment_id} disabled={readOnly} onChange={(e) => setDraft({ ...draft, general: { ...g, default_environment_id: e.target.value } })}>
          <option value="">None</option>
          {envs.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
        </Select>
      </Field>
      <Field label="Default template for “New test”" hint="Highlighted first in the template gallery.">
        <Select value={g.default_template} disabled={readOnly} onChange={(e) => setDraft({ ...draft, general: { ...g, default_template: e.target.value } })}>
          <option value="">None</option>
          {TEMPLATES.map((t) => <option key={t.name} value={t.name}>{t.name}</option>)}
        </Select>
      </Field>
      <SaveBar onSave={save} saving={saving} readOnly={readOnly} />
      <Card className="mt-8 p-4">
        <h3 className="mb-2 text-sm font-semibold text-slate-200">Keyboard shortcuts (plan builder)</h3>
        <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1 text-xs">
          {[
            ['Ctrl/⌘ + Z', 'Undo'],
            ['Ctrl/⌘ + Y or Ctrl/⌘ + Shift + Z', 'Redo'],
            ['Ctrl/⌘ + C / X / V', 'Copy, cut, paste the selected node'],
            ['Ctrl/⌘ + D', 'Duplicate the selected node'],
            ['Delete', 'Delete the selected node'],
            ['Esc', 'Close dialogs'],
          ].map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="font-mono text-slate-300">{k}</dt>
              <dd className="text-slate-400">{v}</dd>
            </div>
          ))}
        </dl>
      </Card>
    </div>
  )
}

function AppearanceTab({ readOnly }: { readOnly: boolean }) {
  const { draft, setDraft, save, saving } = useSettingsDraft()
  if (!draft) return <Spinner />
  const a = draft.appearance
  const opt = (key: keyof typeof a, value: string, label: string) => (
    <button key={value} type="button" disabled={readOnly} onClick={() => setDraft({ ...draft, appearance: { ...a, [key]: value } })} className={`rounded-md border px-3 py-1.5 text-sm ${a[key] === value ? 'border-teal-500 bg-teal-500/10 text-teal-300' : 'border-slate-700 text-slate-300 hover:bg-slate-800'}`}>
      {label}
    </button>
  )
  return (
    <div className="max-w-lg space-y-5">
      <Field label="Theme"><div className="flex gap-2">{opt('theme', 'system', 'System')}{opt('theme', 'light', 'Light')}{opt('theme', 'dark', 'Dark')}</div></Field>
      <Field label="Density"><div className="flex gap-2">{opt('density', 'comfortable', 'Comfortable')}{opt('density', 'compact', 'Compact')}</div></Field>
      <Field label="Chart colours" hint="The high-contrast palette uses the colour-blind-safe Okabe–Ito set.">
        <div className="flex gap-2">{opt('palette', 'default', 'Default')}{opt('palette', 'high-contrast', 'High contrast')}</div>
      </Field>
      <SaveBar onSave={save} saving={saving} readOnly={readOnly} />
    </div>
  )
}

function AITab({ readOnly }: { readOnly: boolean }) {
  const { config, loadConfig, saveConfig, testKey } = useAIStore()
  const toast = useToast()
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [key, setKey] = useState('')
  const [cap, setCap] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [azure, setAzure] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    loadConfig()
  }, [loadConfig])
  if (config && !loaded) {
    setLoaded(true)
    setProvider(config.provider)
    setModel(config.model)
    setCap(config.monthly_cap_usd != null ? String(config.monthly_cap_usd) : '')
    setEnabled(config.enabled)
    setAzure(config.azure_endpoint ?? '')
  }

  async function save() {
    setBusy(true)
    try {
      await saveConfig({ provider, api_key: key, model, monthly_cap_usd: cap ? Number(cap) : null, enabled: enabled && !!provider, azure_endpoint: azure })
      setKey('')
      toast('AI settings saved', 'success')
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setBusy(false)
    }
  }

  async function test() {
    setBusy(true)
    const res = await testKey()
    toast(res.valid ? 'The key works' : `Key check failed: ${res.message}`, res.valid ? 'success' : 'error')
    setBusy(false)
  }

  return (
    <div className="max-w-lg space-y-4">
      <p className="text-sm text-slate-400">
        AI analysis is off by default and uses your own provider key. Nothing is sent unless you press <b>Analyse with AI</b> on a run and confirm the exact prompt; only summary statistics are included.
      </p>
      <Field label="Provider">
        <Select value={provider} disabled={readOnly} onChange={(e) => { setProvider(e.target.value); setModel(MODEL_OPTIONS[e.target.value]?.[0] ?? '') }}>
          <option value="">None (AI disabled)</option>
          {Object.entries(PROVIDER_LABELS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Select>
      </Field>
      {provider && (
        <>
          <Field label="API key" hint={config?.api_key_masked ? `Stored key: ${config.api_key_masked}. Leave empty to keep it.` : 'Encrypted at rest; never shown again.'}>
            <TextInput type="password" value={key} disabled={readOnly} onChange={(e) => setKey(e.target.value)} placeholder={config?.api_key_masked || 'Paste your key'} autoComplete="off" />
          </Field>
          {provider === 'azure_openai' && (
            <Field label="Azure endpoint" hint="https://<resource>.openai.azure.com">
              <TextInput value={azure} disabled={readOnly} onChange={(e) => setAzure(e.target.value)} />
            </Field>
          )}
          <Field label={provider === 'azure_openai' ? 'Deployment name' : 'Model'}>
            <TextInput list="ai-models" value={model} disabled={readOnly} onChange={(e) => setModel(e.target.value)} />
            <datalist id="ai-models">{(MODEL_OPTIONS[provider] ?? []).map((m) => <option key={m} value={m} />)}</datalist>
          </Field>
          <Field label="Monthly spend cap (USD)" hint="Analyses stop once this month’s estimated spend reaches the cap.">
            <TextInput type="number" min={0} step="0.5" value={cap} disabled={readOnly} onChange={(e) => setCap(e.target.value)} placeholder="No cap" />
          </Field>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input type="checkbox" checked={enabled} disabled={readOnly} onChange={(e) => setEnabled(e.target.checked)} className="accent-teal-500" />
            Show “Analyse with AI” on runs and comparisons
          </label>
        </>
      )}
      {!readOnly && (
        <div className="flex gap-2 pt-2">
          <Button variant="primary" onClick={save} disabled={busy}>Save</Button>
          {config?.provider && config.api_key_masked && <Button onClick={test} disabled={busy}>Test key (sends one tiny request)</Button>}
        </div>
      )}
    </div>
  )
}

function IntegrationsTab({ readOnly }: { readOnly: boolean }) {
  const { draft, setDraft, save, saving } = useSettingsDraft()
  if (!draft) return <Spinner />
  const i = draft.integrations
  return (
    <div className="max-w-lg space-y-4">
      <p className="text-sm text-slate-400">OpenSynapse posts to these URLs when a run starts, completes, fails or is aborted. Nothing is sent while they are empty.</p>
      <Field label="Webhook URL" hint={<>Receives JSON: <code>{'{event, run_id, plan_name, status, summary, url}'}</code></>}>
        <TextInput value={i.webhook_url} disabled={readOnly} onChange={(e) => setDraft({ ...draft, integrations: { ...i, webhook_url: e.target.value } })} placeholder="https://example.com/hooks/opensynapse" />
      </Field>
      <Field label="Slack incoming webhook URL" hint="Posts a one-line summary to the channel the webhook belongs to.">
        <TextInput value={i.slack_webhook_url} disabled={readOnly} onChange={(e) => setDraft({ ...draft, integrations: { ...i, slack_webhook_url: e.target.value } })} placeholder="https://hooks.slack.com/services/…" />
      </Field>
      <p className="text-xs text-slate-500">Prometheus: scrape <code>/metrics</code> on the control plane for OpenSynapse’s own metrics.</p>
      <SaveBar onSave={save} saving={saving} readOnly={readOnly} />
    </div>
  )
}

interface StorageInfo {
  data_dir: string
  database: string
  database_bytes: number
  samples_bytes: number
  runs: number
  prunable_runs: number
  prunable_bytes_est: number
}

function StorageTab({ readOnly }: { readOnly: boolean }) {
  const { draft, setDraft, save, saving } = useSettingsDraft()
  const toast = useToast()
  const [info, setInfo] = useState<StorageInfo | null>(null)
  const [confirm, setConfirm] = useState(false)
  const load = useCallback(() => unwrap(api.GET('/api/v1/storage')).then((r) => setInfo(r as StorageInfo)).catch(() => {}), [])
  useEffect(() => {
    load()
  }, [load])
  if (!draft) return <Spinner />
  const st = draft.storage
  async function cleanup() {
    setConfirm(false)
    try {
      const r = await unwrap(api.POST('/api/v1/storage/cleanup'))
      toast(`Deleted ${r.deleted_runs} run(s)`, 'success')
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }
  return (
    <div className="max-w-xl space-y-5">
      {info && (
        <Card className="p-4 text-sm">
          <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1">
            <dt className="text-slate-400">Data folder</dt><dd className="truncate font-mono text-xs text-slate-200">{info.data_dir}</dd>
            <dt className="text-slate-400">Database</dt><dd className="text-slate-200">{formatBytes(info.database_bytes)}</dd>
            <dt className="text-slate-400">Raw samples</dt><dd className="text-slate-200">{formatBytes(info.samples_bytes)}</dd>
            <dt className="text-slate-400">Runs</dt><dd className="text-slate-200">{info.runs}</dd>
          </dl>
        </Card>
      )}
      <Field label="Keep runs">
        <Select value={st.retention_mode} disabled={readOnly} onChange={(e) => setDraft({ ...draft, storage: { ...st, retention_mode: e.target.value } })}>
          <option value="keep_all">Keep everything</option>
          <option value="keep_last_n">Keep the most recent N runs</option>
          <option value="keep_days">Keep runs from the last N days</option>
        </Select>
      </Field>
      {st.retention_mode === 'keep_last_n' && (
        <Field label="Number of runs"><TextInput type="number" min={1} value={st.retention_count} disabled={readOnly} onChange={(e) => setDraft({ ...draft, storage: { ...st, retention_count: Number(e.target.value) } })} /></Field>
      )}
      {st.retention_mode === 'keep_days' && (
        <Field label="Days"><TextInput type="number" min={1} value={st.retention_days} disabled={readOnly} onChange={(e) => setDraft({ ...draft, storage: { ...st, retention_days: Number(e.target.value) } })} /></Field>
      )}
      <p className="text-xs text-slate-500">The policy is applied hourly. Running tests are never deleted.</p>
      <SaveBar onSave={save} saving={saving} readOnly={readOnly} />
      {!readOnly && info && (
        <div className="border-t border-slate-800 pt-4">
          <p className="mb-2 text-sm text-slate-300">
            With the saved policy, a cleanup now would delete <b>{info.prunable_runs}</b> run(s) (about {formatBytes(info.prunable_bytes_est)} of samples plus their database rows).
          </p>
          <Button variant="danger" disabled={info.prunable_runs === 0} onClick={() => setConfirm(true)}>Clean up now</Button>
        </div>
      )}
      <ConfirmDialog open={confirm} title="Delete old runs" message={`Delete ${info?.prunable_runs ?? 0} run(s) now? This cannot be undone.`} onConfirm={cleanup} onCancel={() => setConfirm(false)} />
    </div>
  )
}

function UsersTab() {
  const toast = useToast()
  const me = useAuthStore((s) => s.user)
  const [users, setUsers] = useState<User[] | null>(null)
  const [form, setForm] = useState<{ id?: string; email: string; display_name: string; password: string; role: string } | null>(null)
  const [toDelete, setToDelete] = useState<User | null>(null)
  const load = useCallback(() => unwrap(api.GET('/api/v1/users')).then((r) => setUsers(r.items)).catch((e) => toast(errorMessage(e), 'error')), [toast])
  useEffect(() => {
    load()
  }, [load])

  async function submit() {
    if (!form) return
    try {
      if (form.id) await unwrap(api.PUT('/api/v1/users/{id}', { params: { path: { id: form.id } }, body: { display_name: form.display_name, role: form.role, password: form.password || undefined } }))
      else await unwrap(api.POST('/api/v1/users', { body: form }))
      toast('User saved', 'success')
      setForm(null)
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  return (
    <div className="max-w-3xl">
      <div className="mb-3 flex items-center justify-between">
        <p className="text-sm text-slate-400">Viewers can see everything; editors can also change plans and run tests; admins can also manage users and settings.</p>
        <Button variant="primary" className="shrink-0" onClick={() => setForm({ email: '', display_name: '', password: '', role: 'editor' })}>Add user</Button>
      </div>
      {users === null ? <Spinner /> : (
        <div className="divide-y divide-slate-800 rounded-lg border border-slate-800">
          {users.map((u) => (
            <div key={u.id} className="flex items-center gap-3 px-4 py-2 text-sm">
              <span className="text-slate-100">{u.display_name}</span>
              <span className="text-xs text-slate-500">{u.email}</span>
              <Badge tone={u.role === 'admin' ? 'accent' : 'neutral'}>{u.role}</Badge>
              <div className="ml-auto flex gap-1">
                <Button size="sm" onClick={() => setForm({ id: u.id, email: u.email, display_name: u.display_name, password: '', role: u.role })}>Edit</Button>
                {u.id !== me?.id && <Button size="sm" variant="ghost" onClick={() => setToDelete(u)}>Delete</Button>}
              </div>
            </div>
          ))}
        </div>
      )}
      <Modal open={form !== null} onClose={() => setForm(null)} title={form?.id ? 'Edit user' : 'Add user'} footer={<><Button onClick={() => setForm(null)}>Cancel</Button><Button variant="primary" onClick={submit}>Save</Button></>}>
        {form && (
          <div className="space-y-3">
            <Field label="Email"><TextInput type="email" value={form.email} disabled={!!form.id} onChange={(e) => setForm({ ...form, email: e.target.value })} /></Field>
            <Field label="Name"><TextInput value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} /></Field>
            <Field label={form.id ? 'New password (optional)' : 'Password'} hint="At least 8 characters. Changing it signs the user out everywhere."><TextInput type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} autoComplete="new-password" /></Field>
            <Field label="Role">
              <Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                <option value="viewer">Viewer</option>
                <option value="editor">Editor</option>
                <option value="admin">Admin</option>
              </Select>
            </Field>
          </div>
        )}
      </Modal>
      <ConfirmDialog
        open={toDelete !== null}
        title="Delete user"
        message={<>Delete <b>{toDelete?.email}</b>? Their runs stay in history.</>}
        onCancel={() => setToDelete(null)}
        onConfirm={async () => {
          try {
            await unwrap(api.DELETE('/api/v1/users/{id}', { params: { path: { id: toDelete!.id } } }))
            load()
          } catch (e) {
            toast(errorMessage(e), 'error')
          }
          setToDelete(null)
        }}
      />
    </div>
  )
}

function AccountTab() {
  const toast = useToast()
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [name, setName] = useState('')
  const [created, setCreated] = useState<string | null>(null)
  const [pw, setPw] = useState({ current: '', next: '' })
  const load = useCallback(() => unwrap(api.GET('/api/v1/auth/tokens')).then((r) => setTokens(r.items)).catch(() => {}), [])
  useEffect(() => {
    load()
  }, [load])
  return (
    <div className="max-w-xl space-y-8">
      <section>
        <h3 className="mb-2 text-sm font-semibold text-slate-200">Personal API tokens</h3>
        <p className="mb-3 text-xs text-slate-400">Use these from scripts and CI: <code>Authorization: Bearer osk_…</code>. Each can be revoked on its own.</p>
        <div className="flex gap-2">
          <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="Token name, e.g. CI pipeline" />
          <Button variant="primary" disabled={!name.trim()} onClick={async () => {
            try {
              const r = await unwrap(api.POST('/api/v1/auth/tokens', { body: { name } }))
              setCreated(r.token)
              setName('')
              load()
            } catch (e) {
              toast(errorMessage(e), 'error')
            }
          }}>Create</Button>
        </div>
        {created && (
          <div className="mt-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
            <p className="text-amber-300">Copy this token now; it will not be shown again.</p>
            <code className="mt-1 block break-all text-slate-100">{created}</code>
          </div>
        )}
        <ul className="mt-3 divide-y divide-slate-800 rounded-md border border-slate-800">
          {tokens.length === 0 && <li className="px-3 py-2 text-xs text-slate-500">No tokens yet.</li>}
          {tokens.map((t) => (
            <li key={t.id} className="flex items-center gap-3 px-3 py-2 text-sm">
              <span className="text-slate-200">{t.name}</span>
              <span className="text-xs text-slate-500">created {formatAgo(t.created_at)} · {t.last_used_at ? `used ${formatAgo(t.last_used_at)}` : 'never used'}</span>
              <Button size="sm" variant="ghost" className="ml-auto" onClick={async () => { await unwrap(api.DELETE('/api/v1/auth/tokens/{id}', { params: { path: { id: t.id } } })).catch(() => {}); load() }}>Revoke</Button>
            </li>
          ))}
        </ul>
      </section>
      <section>
        <h3 className="mb-2 text-sm font-semibold text-slate-200">Change password</h3>
        <div className="space-y-2">
          <TextInput type="password" placeholder="Current password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} autoComplete="current-password" />
          <TextInput type="password" placeholder="New password (8+ characters)" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} autoComplete="new-password" />
          <Button disabled={!pw.current || pw.next.length < 8} onClick={async () => {
            try {
              await unwrap(api.PUT('/api/v1/auth/password', { body: { current_password: pw.current, new_password: pw.next } }))
              toast('Password changed; sign in again', 'success')
              useAuthStore.getState().handleUnauthorized()
            } catch (e) {
              toast(errorMessage(e), 'error')
            }
          }}>Change password</Button>
        </div>
      </section>
    </div>
  )
}

function AboutTab({ admin }: { admin: boolean }) {
  const system = useUIStore((s) => s.system)
  const mode = useAuthStore((s) => s.mode)
  const token = useAuthStore((s) => s.token)
  const toast = useToast()
  const [reveal, setReveal] = useState(false)
  const [logs, setLogs] = useState<string[] | null>(null)
  if (!system) return <Spinner />
  return (
    <div className="max-w-2xl space-y-6">
      <Card className="p-4 text-sm">
        <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1.5">
          <dt className="text-slate-400">Version</dt><dd className="text-slate-200">{system.version}</dd>
          <dt className="text-slate-400">Mode</dt><dd className="text-slate-200">{system.mode}{system.team_mode ? ' · team (sign-in required)' : ' · local (single user)'}</dd>
          <dt className="text-slate-400">Load engine</dt>
          <dd className="text-slate-200">
            {system.k6.available ? `k6 ${system.k6.version}${system.k6.kv ? ' with xk6-kv' : ' (no xk6-kv: live RPS limits unavailable)'}` : 'k6 not found — install k6 or use the Docker image to run tests'}
          </dd>
          <dt className="text-slate-400">Data folder</dt><dd className="break-all font-mono text-xs text-slate-200">{system.data_dir || '—'}</dd>
          <dt className="text-slate-400">License</dt><dd className="text-slate-200">See LICENSE in the repository</dd>
          <dt className="text-slate-400">Documentation</dt><dd><a className="text-teal-400 hover:underline" href="https://github.com/AustinRGovender/OpenSynapse/blob/main/docs/user/HOW-TO.md" target="_blank" rel="noreferrer">How-to guide</a> · <a className="text-teal-400 hover:underline" href="/openapi.json" target="_blank" rel="noreferrer">API (OpenAPI)</a></dd>
        </dl>
      </Card>
      {mode === 'local' && (
        <section>
          <h3 className="mb-1 text-sm font-semibold text-slate-200">API token</h3>
          <p className="mb-2 text-xs text-slate-400">Scripts and the CLI authenticate with this installation token: <code>Authorization: Bearer …</code>. It is also in <code>api-token</code> in the data folder.</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 truncate rounded-md border border-slate-800 bg-slate-900 px-2 py-1.5 text-xs text-slate-200">{reveal ? token : '•'.repeat(32)}</code>
            <Button size="sm" onClick={() => setReveal(!reveal)}>{reveal ? 'Hide' : 'Reveal'}</Button>
            <Button size="sm" onClick={() => { navigator.clipboard?.writeText(token ?? ''); toast('Copied', 'success') }}>Copy</Button>
          </div>
        </section>
      )}
      {admin && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-slate-200">Troubleshooting</h3>
          <div className="flex gap-2">
            <Button onClick={() => downloadFile('/api/v1/diagnostics', undefined, 'opensynapse-diagnostics.zip').catch((e) => toast(errorMessage(e), 'error'))}>Download diagnostic bundle</Button>
            <Button onClick={() => unwrap(api.GET('/api/v1/system/logs')).then((r) => setLogs(r.lines)).catch((e) => toast(errorMessage(e), 'error'))}>Show recent logs</Button>
          </div>
          <p className="text-xs text-slate-500">The bundle contains system information, settings and recent logs with secrets removed.</p>
        </section>
      )}
      <Modal open={logs !== null} onClose={() => setLogs(null)} title="Control-plane logs" width="max-w-4xl">
        <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap font-mono text-[11px] text-slate-300">{logs?.join('\n') || 'No log lines yet.'}</pre>
      </Modal>
    </div>
  )
}

export function SettingsPage({ tab }: { tab: string }) {
  const user = useAuthStore((s) => s.user)
  const mode = useAuthStore((s) => s.mode)
  const admin = isAdmin(user)
  const tabs: [TabKey, string][] = [
    ['general', 'General'],
    ['appearance', 'Appearance'],
    ['ai', 'AI'],
    ['integrations', 'Integrations'],
    ['storage', 'Storage'],
    ...(mode === 'team' ? ([['account', 'Account']] as [TabKey, string][]) : []),
    ...(mode === 'team' && admin ? ([['users', 'Users']] as [TabKey, string][]) : []),
    ['about', 'About'],
  ]
  const active = (tabs.find(([k]) => k === tab)?.[0] ?? 'general') as TabKey

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <PageHeader title="Settings" />
      <div className="flex flex-col gap-6 md:flex-row">
        <nav aria-label="Settings sections" className="flex shrink-0 gap-1 md:w-44 md:flex-col">
          {tabs.map(([k, label]) => (
            <button key={k} type="button" onClick={() => navigate(`#/settings/${k}`)} aria-current={active === k ? 'page' : undefined} className={`rounded-md px-3 py-1.5 text-left text-sm ${active === k ? 'bg-slate-800 text-slate-100' : 'text-slate-400 hover:bg-slate-900 hover:text-slate-200'}`}>
              {label}
            </button>
          ))}
        </nav>
        <div className="min-w-0 flex-1">
          {active === 'general' && <GeneralTab readOnly={!admin} />}
          {active === 'appearance' && <AppearanceTab readOnly={!admin} />}
          {active === 'ai' && <AITab readOnly={!admin} />}
          {active === 'integrations' && <IntegrationsTab readOnly={!admin} />}
          {active === 'storage' && <StorageTab readOnly={!admin} />}
          {active === 'users' && <UsersTab />}
          {active === 'account' && <AccountTab />}
          {active === 'about' && <AboutTab admin={admin} />}
        </div>
      </div>
    </div>
  )
}
