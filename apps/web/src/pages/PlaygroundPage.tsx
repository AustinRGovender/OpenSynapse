import { useEffect, useMemo, useState } from 'react'
import type { Collection, Environment, Node, Plan, PlaygroundResponse } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'
import { useChartTheme } from '../components/run-view/chart-theme'
import { MetricChart } from '../components/run-view/MetricChart'
import { Badge, Button, Card, EmptyState, Field, Modal, Select, Spinner, TextInput } from '../components/ui/primitives'
import { createNode, NODE_TYPES } from '../config/nodes'
import { canEdit, useAuthStore } from '../stores/auth-store'
import { type AuthType, type BodyType, type HttpMethod, type KeyValueRow, usePlaygroundStore } from '../stores/playground-store'
import { useToast } from '../stores/ui-store'
import { formatBytes, formatMs, formatNumber } from '../utils/format'

const METHODS: HttpMethod[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

const METHOD_TONE: Record<string, string> = {
  GET: 'text-teal-400',
  POST: 'text-violet-400',
  PUT: 'text-amber-400',
  PATCH: 'text-amber-400',
  DELETE: 'text-red-400',
}

const AUTH_LABELS: Record<AuthType, string> = {
  none: 'No auth',
  basic: 'Basic',
  bearer: 'Bearer token',
  api_key: 'API key',
  digest: 'Digest',
  oauth2: 'OAuth 2.0',
  inherit: 'Inherit from environment',
}

const TIMING_SEGMENTS = [
  { key: 'dns_ms', label: 'DNS', color: 'bg-sky-500' },
  { key: 'connect_ms', label: 'Connect', color: 'bg-teal-500' },
  { key: 'tls_ms', label: 'TLS', color: 'bg-violet-500' },
  { key: 'send_ms', label: 'Send', color: 'bg-amber-500' },
  { key: 'wait_ms', label: 'Waiting (TTFB)', color: 'bg-rose-500' },
  { key: 'receive_ms', label: 'Receive', color: 'bg-green-500' },
] as const

function statusTone(status: number) {
  if (status >= 500) return 'text-red-400'
  if (status >= 400) return 'text-amber-400'
  if (status >= 300) return 'text-sky-400'
  return 'text-green-400'
}

function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

function Tabs<T extends string>({ tabs, value, onChange, label }: { tabs: [T, string][]; value: T; onChange: (t: T) => void; label: string }) {
  return (
    <div role="tablist" aria-label={label} className="flex gap-1 border-b border-slate-800 px-3">
      {tabs.map(([key, text]) => (
        <button
          key={key}
          role="tab"
          type="button"
          aria-selected={value === key}
          onClick={() => onChange(key)}
          className={`-mb-px border-b-2 px-2.5 py-2 text-xs font-medium ${value === key ? 'border-teal-500 text-teal-300' : 'border-transparent text-slate-400 hover:text-slate-200'}`}
        >
          {text}
        </button>
      ))}
    </div>
  )
}

function KeyValueEditor({ rows, onChange, keyPlaceholder }: { rows: KeyValueRow[]; onChange: (rows: KeyValueRow[]) => void; keyPlaceholder: string }) {
  function update(i: number, patch: Partial<KeyValueRow>) {
    const next = rows.map((r, j) => (j === i ? { ...r, ...patch } : r))
    // Keep one blank row at the end to type into.
    if (next[next.length - 1].key || next[next.length - 1].value) next.push({ key: '', value: '', enabled: true })
    onChange(next)
  }
  return (
    <div className="space-y-1.5 p-3">
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-2">
          <input type="checkbox" checked={r.enabled} onChange={(e) => update(i, { enabled: e.target.checked })} aria-label="Enabled" className="accent-teal-500" />
          <TextInput value={r.key} onChange={(e) => update(i, { key: e.target.value })} placeholder={keyPlaceholder} aria-label="Name" />
          <TextInput value={r.value} onChange={(e) => update(i, { value: e.target.value })} placeholder="Value" aria-label="Value" />
          <button type="button" aria-label="Remove row" disabled={rows.length === 1} onClick={() => onChange(rows.filter((_, j) => j !== i))} className="px-1 text-slate-500 hover:text-red-400 disabled:invisible">✕</button>
        </div>
      ))}
    </div>
  )
}

function useOAuthFlow() {
  const toast = useToast()
  const { auth, setAuth } = usePlaygroundStore()
  const [busy, setBusy] = useState(false)

  async function authorize() {
    setBusy(true)
    try {
      const start = await unwrap(api.POST('/api/v1/playground/oauth/start', { body: { auth_url: auth.auth_url, token_url: auth.token_url, client_id: auth.client_id, client_secret: auth.client_secret, scope: auth.scope } }))
      const popup = window.open(start.authorize_url, 'opensynapse-oauth', 'width=520,height=680')
      if (!popup) throw new Error('The sign-in window was blocked; allow pop-ups for OpenSynapse and try again.')
      // The callback page posts back once the provider redirects to it.
      const ok = await new Promise<boolean>((resolve, reject) => {
        const timer = setInterval(() => {
          if (popup.closed) {
            cleanup()
            reject(new Error('The sign-in window was closed before it finished.'))
          }
        }, 500)
        function onMessage(e: MessageEvent) {
          if (e.origin !== window.location.origin || e.data?.type !== 'opensynapse-oauth' || e.data.state !== start.state) return
          cleanup()
          resolve(!!e.data.ok)
        }
        function cleanup() {
          clearInterval(timer)
          window.removeEventListener('message', onMessage)
        }
        window.addEventListener('message', onMessage)
      })
      if (!ok) throw new Error('The provider did not grant access.')
      const token = await unwrap(api.GET('/api/v1/playground/oauth/result', { params: { query: { state: start.state } } }))
      setAuth({ access_token: token.access_token })
      toast('Access token received', 'success')
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setBusy(false)
    }
  }

  async function clientCredentials() {
    setBusy(true)
    try {
      const token = await unwrap(api.POST('/api/v1/playground/oauth/client-credentials', { body: { token_url: auth.token_url, client_id: auth.client_id, client_secret: auth.client_secret, scope: auth.scope } }))
      setAuth({ access_token: token.access_token })
      toast('Access token received', 'success')
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setBusy(false)
    }
  }

  return { busy, getToken: auth.grant_type === 'client_credentials' ? clientCredentials : authorize }
}

function AuthEditor() {
  const { authType, auth, set, setAuth } = usePlaygroundStore()
  const oauth = useOAuthFlow()
  const input = (key: keyof typeof auth, label: string, type = 'text', placeholder = '') => (
    <Field label={label}>
      <TextInput type={type} value={auth[key]} onChange={(e) => setAuth({ [key]: e.target.value })} placeholder={placeholder} autoComplete="off" />
    </Field>
  )

  return (
    <div className="space-y-3 p-3">
      <Field label="Auth type">
        <Select value={authType} onChange={(e) => set({ authType: e.target.value as AuthType })}>
          {(Object.keys(AUTH_LABELS) as AuthType[]).map((t) => <option key={t} value={t}>{AUTH_LABELS[t]}</option>)}
        </Select>
      </Field>
      {(authType === 'basic' || authType === 'digest') && (
        <div className="grid gap-3 sm:grid-cols-2">
          {input('username', 'Username')}
          {input('password', 'Password', 'password')}
          {authType === 'digest' && <p className="text-[11px] text-slate-500 sm:col-span-2">The request is sent once, and repeated with a digest response when the server answers 401 with a challenge.</p>}
        </div>
      )}
      {authType === 'bearer' && input('token', 'Token', 'text', 'eyJhbGciOi… or ${AUTH_TOKEN}')}
      {authType === 'api_key' && (
        <div className="grid gap-3 sm:grid-cols-3">
          {input('key', 'Name', 'text', 'X-API-Key')}
          {input('value', 'Value')}
          <Field label="Send in">
            <Select value={auth.add_to} onChange={(e) => setAuth({ add_to: e.target.value as 'header' | 'query' })}>
              <option value="header">Header</option>
              <option value="query">Query string</option>
            </Select>
          </Field>
        </div>
      )}
      {authType === 'oauth2' && (
        <div className="space-y-3">
          <Field label="Grant type">
            <Select value={auth.grant_type} onChange={(e) => setAuth({ grant_type: e.target.value as 'authorization_code' | 'client_credentials' })}>
              <option value="authorization_code">Authorization code (sign in through a browser window)</option>
              <option value="client_credentials">Client credentials (machine to machine)</option>
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            {auth.grant_type === 'authorization_code' && input('auth_url', 'Authorization URL', 'url', 'https://provider/authorize')}
            {input('token_url', 'Token URL', 'url', 'https://provider/token')}
            {input('client_id', 'Client ID')}
            {input('client_secret', 'Client secret', 'password')}
            {input('scope', 'Scope', 'text', 'read write')}
          </div>
          <div className="flex items-end gap-2">
            <div className="flex-1">{input('access_token', 'Access token', 'text', 'Filled in by “Get new access token”')}</div>
            <Button onClick={oauth.getToken} disabled={oauth.busy || !auth.token_url || !auth.client_id}>{oauth.busy ? 'Waiting…' : 'Get new access token'}</Button>
          </div>
          {auth.grant_type === 'client_credentials' && <p className="text-[11px] text-slate-500">If the access token is empty, one is fetched automatically when you send.</p>}
        </div>
      )}
      {authType === 'inherit' && (
        <p className="text-xs text-slate-400">
          Uses the selected environment’s <code>AUTH_TOKEN</code> (bearer), <code>AUTH_USERNAME</code>/<code>AUTH_PASSWORD</code> (basic) or <code>API_KEY</code> (header named by <code>API_KEY_HEADER</code>, default X-API-Key).
        </p>
      )}
    </div>
  )
}

function BodyEditor() {
  const { method, body, bodyType, set } = usePlaygroundStore()
  if (method === 'GET' || method === 'HEAD') return <p className="p-3 text-xs text-slate-500">{method} requests have no body.</p>
  return (
    <div className="flex h-full flex-col gap-2 p-3">
      <div className="flex items-center gap-3 text-xs text-slate-400">
        {(['none', 'json', 'form', 'raw'] as BodyType[]).map((t) => (
          <label key={t} className="flex items-center gap-1">
            <input type="radio" name="body-type" checked={bodyType === t} onChange={() => set({ bodyType: t })} className="accent-teal-500" />
            {t === 'form' ? 'Form (url-encoded)' : t === 'json' ? 'JSON' : t === 'raw' ? 'Raw text' : 'None'}
          </label>
        ))}
        {bodyType === 'json' && <Button size="sm" variant="ghost" className="ml-auto" onClick={() => set({ body: pretty(body) })}>Format</Button>}
      </div>
      {bodyType !== 'none' && (
        <textarea
          value={body}
          onChange={(e) => set({ body: e.target.value })}
          spellCheck={false}
          aria-label="Request body"
          placeholder={bodyType === 'json' ? '{\n  "name": "${USER_NAME}"\n}' : bodyType === 'form' ? 'username=alice&password=secret' : ''}
          className="min-h-40 flex-1 resize-none rounded border border-slate-700 bg-slate-900 p-2 font-mono text-xs text-slate-200 outline-none focus:border-teal-500"
        />
      )}
    </div>
  )
}

function TimingView({ timing }: { timing: PlaygroundResponse['timing'] }) {
  const total = timing.total_ms || 1
  return (
    <div className="p-4">
      <p className="mb-3 text-xs text-slate-400">Total <span className="font-mono text-slate-200">{formatMs(timing.total_ms)}</span></p>
      <div className="mb-4 flex h-5 overflow-hidden rounded bg-slate-800">
        {TIMING_SEGMENTS.map(({ key, color, label }) => {
          const pct = (timing[key] / total) * 100
          return pct < 0.5 ? null : <div key={key} className={color} style={{ width: `${pct}%` }} title={`${label}: ${formatMs(timing[key])}`} />
        })}
      </div>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-xs sm:grid-cols-3">
        {TIMING_SEGMENTS.map(({ key, label, color }) => (
          <div key={key} className="flex items-center gap-1.5">
            <span className={`h-2.5 w-2.5 rounded-sm ${color}`} />
            <dt className="text-slate-400">{label}</dt>
            <dd className="ml-auto font-mono text-slate-200">{formatMs(timing[key])}</dd>
          </div>
        ))}
      </dl>
      <p className="mt-4 text-[11px] text-slate-500">DNS, connect and TLS are zero when the connection was reused.</p>
    </div>
  )
}

function ResponsePane() {
  const { response, sending, error } = usePlaygroundStore()
  const [tab, setTab] = useState<'body' | 'headers' | 'cookies' | 'timing'>('body')
  const [raw, setRaw] = useState(false)

  if (sending) return <div className="p-6"><Spinner label="Sending…" /></div>
  if (error) return <div className="m-4 rounded border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-300" role="alert">{error}</div>
  if (!response) return <div className="p-6"><EmptyState title="No response yet">Enter a URL and press Send (Ctrl+Enter). Requests go out from the OpenSynapse server, so <code>localhost</code> means the server’s machine.</EmptyState></div>

  const headers = Object.entries(response.headers).sort(([a], [b]) => a.localeCompare(b))
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-slate-800 px-4 py-2 text-xs">
        <span className={`font-mono text-sm font-semibold ${statusTone(response.status)}`}>{response.status} {response.status_text.replace(/^\d+\s*/, '')}</span>
        <span className="text-slate-400">{formatMs(response.timing.total_ms)}</span>
        <span className="text-slate-400">{formatBytes(response.body_size)}</span>
        {response.final_url && <span className="truncate text-slate-500" title={response.final_url}>→ {response.final_url}</span>}
        {response.truncated && <Badge tone="warn">Body truncated</Badge>}
      </div>
      <Tabs label="Response" value={tab} onChange={setTab} tabs={[['body', 'Body'], ['headers', `Headers (${headers.length})`], ['cookies', `Cookies (${response.cookies.length})`], ['timing', 'Timing']]} />
      <div className="min-h-0 flex-1 overflow-auto">
        {tab === 'body' && (
          <div className="relative">
            <button type="button" onClick={() => setRaw(!raw)} className="absolute right-3 top-2 text-[11px] text-slate-500 hover:text-slate-300">{raw ? 'Pretty' : 'Raw'}</button>
            <pre className="whitespace-pre-wrap break-all p-4 font-mono text-xs text-slate-300">{(raw ? response.body : pretty(response.body)) || <span className="text-slate-500">(empty body)</span>}</pre>
          </div>
        )}
        {tab === 'headers' && (
          <table className="w-full text-xs">
            <tbody>
              {headers.map(([k, v]) => (
                <tr key={k} className="border-b border-slate-800/60">
                  <td className="w-1/3 px-4 py-1.5 align-top font-mono text-slate-400">{k}</td>
                  <td className="break-all px-4 py-1.5 font-mono text-slate-200">{v}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {tab === 'cookies' &&
          (response.cookies.length === 0 ? (
            <p className="p-4 text-xs text-slate-500">The response set no cookies.</p>
          ) : (
            <table className="w-full text-xs">
              <thead className="text-left text-slate-500">
                <tr>{['Name', 'Value', 'Domain', 'Path', 'Expires', 'Flags'].map((h) => <th key={h} className="px-4 py-1.5 font-medium">{h}</th>)}</tr>
              </thead>
              <tbody>
                {response.cookies.map((c) => (
                  <tr key={c.name + c.path} className="border-t border-slate-800/60 font-mono text-slate-300">
                    <td className="px-4 py-1.5">{c.name}</td>
                    <td className="max-w-xs truncate px-4 py-1.5" title={c.value}>{c.value}</td>
                    <td className="px-4 py-1.5">{c.domain || '—'}</td>
                    <td className="px-4 py-1.5">{c.path || '/'}</td>
                    <td className="px-4 py-1.5">{c.expires || 'Session'}</td>
                    <td className="px-4 py-1.5">{[c.http_only && 'HttpOnly', c.secure && 'Secure'].filter(Boolean).join(' ') || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ))}
        {tab === 'timing' && <TimingView timing={response.timing} />}
      </div>
    </div>
  )
}

function MicroTestPanel({ onClose }: { onClose: () => void }) {
  const { micro, microRunning, microTest } = usePlaygroundStore()
  const { metric } = useChartTheme()
  const [vus, setVus] = useState(5)
  const [seconds, setSeconds] = useState(10)
  const s = micro?.summary

  return (
    <div className="border-t border-slate-800 bg-slate-950/40">
      <div className="flex flex-wrap items-end gap-3 px-4 py-3">
        <div>
          <p className="text-sm font-medium text-slate-200">Micro-test</p>
          <p className="text-[11px] text-slate-500">A short k6 burst of this one request (1–50 VUs, 1–60 s). It is not saved as a run.</p>
        </div>
        <div className="w-24"><Field label="VUs"><TextInput type="number" min={1} max={50} value={vus} onChange={(e) => setVus(Math.min(50, Math.max(1, Number(e.target.value))))} /></Field></div>
        <div className="w-28"><Field label="Seconds"><TextInput type="number" min={1} max={60} value={seconds} onChange={(e) => setSeconds(Math.min(60, Math.max(1, Number(e.target.value))))} /></Field></div>
        <Button variant="primary" disabled={microRunning} onClick={() => microTest(vus, seconds)}>{microRunning ? `Running for ${seconds}s…` : 'Start micro-test'}</Button>
        <Button variant="ghost" className="ml-auto" onClick={onClose}>Close</Button>
      </div>
      {micro && (
        <div className="space-y-3 px-4 pb-4">
          {micro.message && <p className={`text-xs ${micro.status === 'completed' ? 'text-slate-400' : 'text-red-300'}`}>{micro.message}</p>}
          {s && (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-7">
              {[
                ['Requests', formatNumber(s.total_requests)],
                ['RPS', s.throughput_rps.toFixed(1)],
                ['p50', formatMs(s.p50_ms)],
                ['p95', formatMs(s.p95_ms)],
                ['p99', formatMs(s.p99_ms)],
                ['Max', formatMs(s.max_ms)],
                ['Errors', `${s.error_rate.toFixed(1)}%`],
              ].map(([label, value]) => (
                <Card key={label} className="px-3 py-2">
                  <p className="text-[11px] text-slate-500">{label}</p>
                  <p className="font-mono text-sm text-slate-100">{value}</p>
                </Card>
              ))}
            </div>
          )}
          {micro.series.length > 1 && (
            <div className="grid gap-3 lg:grid-cols-2">
              <MetricChart title="Requests per second" unit="req/s" height={150} points={micro.series} series={[{ key: 'rps', label: 'RPS', color: metric.rps, value: (p) => p.rps }]} />
              <MetricChart
                title="Response time"
                unit="ms"
                height={150}
                points={micro.series}
                series={[
                  { key: 'p50', label: 'p50', color: metric.p50, value: (p) => p.p50_ms },
                  { key: 'p95', label: 'p95', color: metric.p95, value: (p) => p.p95_ms },
                ]}
              />
            </div>
          )}
          {micro.errors.length > 0 && (
            <details className="text-xs text-slate-400">
              <summary className="cursor-pointer">{micro.errors.length} error sample{micro.errors.length === 1 ? '' : 's'}</summary>
              <ul className="mt-1 space-y-0.5 font-mono">
                {micro.errors.slice(0, 20).map((e, i) => <li key={i}>{e.status || e.error_code} {e.message}</li>)}
              </ul>
            </details>
          )}
        </div>
      )}
    </div>
  )
}

function toBase64(s: string) {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)))
}

/** Converts the current request into an HTTP Request node for a plan. */
function toHttpNode(name: string): { node: Node; warning?: string } {
  const s = usePlaygroundStore.getState()
  const headers = s.headers.filter((h) => h.enabled && h.key.trim()).map((h) => ({ key: h.key.trim(), value: h.value, enabled: true }))
  let url = s.url.trim()
  const params = s.params.filter((p) => p.enabled && p.key.trim())
  if (params.length) url += (url.includes('?') ? '&' : '?') + params.map((p) => `${encodeURIComponent(p.key)}=${encodeURIComponent(p.value)}`).join('&')
  let warning: string | undefined
  const a = s.auth
  // Plans carry auth as headers; methods that need a handshake can't be expressed that way.
  if (s.authType === 'basic') headers.push({ key: 'Authorization', value: `Basic ${toBase64(`${a.username}:${a.password}`)}`, enabled: true })
  else if (s.authType === 'bearer') headers.push({ key: 'Authorization', value: `Bearer ${a.token}`, enabled: true })
  else if (s.authType === 'api_key' && a.add_to === 'header') headers.push({ key: a.key, value: a.value, enabled: true })
  else if (s.authType === 'api_key') url += `${url.includes('?') ? '&' : '?'}${encodeURIComponent(a.key)}=${encodeURIComponent(a.value)}`
  else if (s.authType === 'oauth2') {
    headers.push({ key: 'Authorization', value: 'Bearer ${AUTH_TOKEN}', enabled: true })
    warning = 'OAuth tokens expire, so the node uses ${AUTH_TOKEN}: set it in the run’s environment.'
  } else if (s.authType === 'inherit') {
    headers.push({ key: 'Authorization', value: 'Bearer ${AUTH_TOKEN}', enabled: true })
  } else if (s.authType === 'digest') warning = 'Digest auth is not carried into plans; add credentials to the node by hand.'

  const hasBody = s.bodyType !== 'none' && s.method !== 'GET' && s.method !== 'HEAD'
  let body: Record<string, unknown> = { type: 'none' }
  if (hasBody && s.bodyType === 'form') body = { type: 'form', form_fields: [...new URLSearchParams(s.body)].map(([key, value]) => ({ key, value })) }
  else if (hasBody) body = { type: s.bodyType, content: s.body }

  const node = createNode('http', name)
  node.properties = { ...NODE_TYPES.http.defaultProperties, method: s.method, url, headers, body }
  return { node, warning }
}

function flatten(n: Node, depth = 0, out: { node: Node; depth: number }[] = []) {
  if (NODE_TYPES[n.type]?.canHaveChildren) out.push({ node: n, depth })
  n.children.forEach((c) => flatten(c, depth + 1, out))
  return out
}

function insertInto(root: Node, parentId: string, child: Node): Node {
  if (root.id === parentId) return { ...root, children: [...root.children, child] }
  return { ...root, children: root.children.map((c) => insertInto(c, parentId, child)) }
}

/** "GET /products/p1": a default name for a saved request. */
function requestName() {
  const { method, url } = usePlaygroundStore.getState()
  return `${method} ${url.replace(/^(https?:\/\/[^/]+|\$\{\w+\})/, '') || '/'}`
}

function SaveToPlanModal({ onClose }: { onClose: () => void }) {
  const toast = useToast()
  const [plans, setPlans] = useState<{ id: string; name: string }[]>([])
  const [plan, setPlan] = useState<Plan | null>(null)
  const [parentId, setParentId] = useState('')
  const [name, setName] = useState(requestName)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    unwrap(api.GET('/api/v1/plans', { params: { query: { limit: '500' } } })).then((r) => setPlans(r.items)).catch((e) => toast(errorMessage(e), 'error'))
  }, [toast])

  async function choosePlan(id: string) {
    setPlan(null)
    setParentId('')
    if (!id) return
    try {
      const p = await unwrap(api.GET('/api/v1/plans/{id}', { params: { path: { id } } }))
      setPlan(p)
      setParentId((p.root.children.find((c) => c.type === 'scenario') ?? p.root).id)
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  async function save() {
    if (!plan || !parentId) return
    setSaving(true)
    try {
      const { node, warning } = toHttpNode(name.trim() || 'Request')
      await unwrap(api.PUT('/api/v1/plans/{id}', { params: { path: { id: plan.id } }, body: { name: plan.name, description: plan.description, tags: plan.tags, root: insertInto(plan.root, parentId, node), default_environment_id: plan.default_environment_id } }))
      toast(`Added to “${plan.name}”`, 'success')
      if (warning) toast(warning, 'info')
      onClose()
    } catch (e) {
      toast(errorMessage(e), 'error')
    } finally {
      setSaving(false)
    }
  }

  const targets = useMemo(() => (plan ? flatten(plan.root) : []), [plan])
  return (
    <Modal
      open
      onClose={onClose}
      title="Add request to a plan"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" disabled={!plan || !parentId || saving} onClick={save}>{saving ? 'Adding…' : 'Add to plan'}</Button></>}
    >
      <div className="space-y-3">
        <Field label="Node name"><TextInput value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Plan">
          <Select value={plan?.id ?? ''} onChange={(e) => choosePlan(e.target.value)}>
            <option value="">Choose a plan…</option>
            {plans.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </Field>
        {plan && (
          <Field label="Add inside" hint="The request is appended as the last child of this node.">
            <Select value={parentId} onChange={(e) => setParentId(e.target.value)}>
              {targets.map(({ node, depth }) => <option key={node.id} value={node.id}>{'  '.repeat(depth)}{node.name} ({NODE_TYPES[node.type]?.label ?? node.type})</option>)}
            </Select>
          </Field>
        )}
      </div>
    </Modal>
  )
}

function SaveToCollectionModal({ onClose, collections }: { onClose: () => void; collections: Collection[] }) {
  const toast = useToast()
  const save = usePlaygroundStore((s) => s.saveToCollection)
  const [collection, setCollection] = useState(collections[0]?.name || 'My requests')
  const [name, setName] = useState(requestName)

  async function submit() {
    try {
      await save(collection.trim(), name.trim())
      toast(`Saved to “${collection}”`, 'success')
      onClose()
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  return (
    <Modal open onClose={onClose} title="Save request" footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" disabled={!collection.trim() || !name.trim()} onClick={submit}>Save</Button></>}>
      <div className="space-y-3">
        <Field label="Request name"><TextInput value={name} onChange={(e) => setName(e.target.value)} autoFocus /></Field>
        <Field label="Collection" hint="Pick an existing collection or type a new name.">
          <TextInput list="collection-names" value={collection} onChange={(e) => setCollection(e.target.value)} />
          <datalist id="collection-names">{collections.map((c) => <option key={c.id} value={c.name} />)}</datalist>
        </Field>
        <p className="text-[11px] text-slate-500">Credentials in the auth settings are saved with the request.</p>
      </div>
    </Modal>
  )
}

function CollectionsSidebar() {
  const { collections, load, removeFromCollection } = usePlaygroundStore()
  const toast = useToast()
  if (collections.length === 0) return <p className="p-3 text-xs text-slate-500">Saved requests appear here. Use Save to keep one.</p>
  return (
    <div className="space-y-3 p-2">
      {collections.map((c) => (
        <div key={c.id}>
          <p className="px-1 pb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500">{c.name}</p>
          <ul>
            {c.requests.map((r, i) => (
              <li key={i} className="group flex items-center gap-1 rounded hover:bg-slate-800">
                <button type="button" onClick={() => load(r)} className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1 text-left text-xs">
                  <span className={`w-11 shrink-0 font-mono text-[10px] font-semibold ${METHOD_TONE[r.method] ?? 'text-slate-400'}`}>{r.method}</span>
                  <span className="truncate text-slate-300" title={r.url}>{r.name.startsWith(`${r.method} `) ? r.name.slice(r.method.length + 1) : r.name}</span>
                </button>
                <button type="button" aria-label={`Delete ${r.name}`} onClick={() => removeFromCollection(c, i).catch((e) => toast(errorMessage(e), 'error'))} className="invisible px-1.5 text-xs text-slate-500 hover:text-red-400 group-hover:visible">✕</button>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}

/** API Playground (F-03): send single requests, inspect responses, burst-test them. */
export function PlaygroundPage() {
  const store = usePlaygroundStore()
  const { method, url, params, headers, environmentId, set, send, sending, loadCollections, collections } = store
  const editable = canEdit(useAuthStore((s) => s.user))
  const [tab, setTab] = useState<'params' | 'headers' | 'body' | 'auth'>('params')
  const [envs, setEnvs] = useState<Environment[]>([])
  const [showMicro, setShowMicro] = useState(false)
  const [saveCollection, setSaveCollection] = useState(false)
  const [savePlan, setSavePlan] = useState(false)

  useEffect(() => {
    loadCollections()
    unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } })).then((r) => setEnvs(r.items)).catch(() => {})
  }, [loadCollections])

  const activeParams = params.filter((p) => p.enabled && p.key).length
  const activeHeaders = headers.filter((h) => h.enabled && h.key).length

  return (
    <div className="flex h-full">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-slate-800 md:flex">
        <div className="border-b border-slate-800 px-3 py-2 text-sm font-medium text-slate-200">Collections</div>
        <div className="flex-1 overflow-y-auto"><CollectionsSidebar /></div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <form
          className="flex flex-wrap items-center gap-2 border-b border-slate-800 p-3"
          onSubmit={(e) => {
            e.preventDefault()
            send()
          }}
        >
          <div className="w-28">
            <Select value={method} onChange={(e) => set({ method: e.target.value as HttpMethod })} aria-label="Method" className={`font-mono font-semibold ${METHOD_TONE[method] ?? ''}`}>
              {METHODS.map((m) => <option key={m}>{m}</option>)}
            </Select>
          </div>
          <input
            value={url}
            onChange={(e) => set({ url: e.target.value })}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) send()
            }}
            placeholder="https://api.example.com/items or ${BASE_URL}/items"
            aria-label="URL"
            className="min-w-64 flex-1 rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 font-mono text-sm text-slate-100 placeholder-slate-500 outline-none focus:border-teal-500"
          />
          <div className="w-44">
            <Select value={environmentId} onChange={(e) => set({ environmentId: e.target.value })} aria-label="Environment">
              <option value="">No environment</option>
              {envs.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
            </Select>
          </div>
          <Button type="submit" variant="primary" disabled={sending}>{sending ? 'Sending…' : 'Send'}</Button>
          <Button type="button" onClick={() => setShowMicro((v) => !v)} aria-pressed={showMicro} disabled={!url.trim()}>Micro-test</Button>
          <Button type="button" onClick={() => setSaveCollection(true)} disabled={!url.trim()}>Save</Button>
          {editable && <Button type="button" onClick={() => setSavePlan(true)} disabled={!url.trim()}>Add to plan</Button>}
        </form>
        {environmentId && <p className="border-b border-slate-800 px-3 py-1 text-[11px] text-slate-500">${'{'}VARIABLES{'}'} in the URL, headers, body and auth are filled from the selected environment.</p>}

        <div className="grid min-h-0 flex-1 grid-rows-2 lg:grid-cols-2 lg:grid-rows-1">
          <section className="flex min-h-0 flex-col border-b border-slate-800 lg:border-b-0 lg:border-r" aria-label="Request">
            <Tabs label="Request" value={tab} onChange={setTab} tabs={[['params', `Params${activeParams ? ` (${activeParams})` : ''}`], ['headers', `Headers${activeHeaders ? ` (${activeHeaders})` : ''}`], ['body', 'Body'], ['auth', `Auth${store.authType !== 'none' ? ' •' : ''}`]]} />
            <div className="min-h-0 flex-1 overflow-y-auto">
              {tab === 'params' && <KeyValueEditor rows={params} onChange={(rows) => set({ params: rows })} keyPlaceholder="Query parameter" />}
              {tab === 'headers' && <KeyValueEditor rows={headers} onChange={(rows) => set({ headers: rows })} keyPlaceholder="Header" />}
              {tab === 'body' && <BodyEditor />}
              {tab === 'auth' && <AuthEditor />}
            </div>
          </section>
          <section className="min-h-0" aria-label="Response">
            <ResponsePane />
          </section>
        </div>

        {showMicro && <MicroTestPanel onClose={() => setShowMicro(false)} />}
      </div>

      {saveCollection && <SaveToCollectionModal onClose={() => setSaveCollection(false)} collections={collections} />}
      {savePlan && <SaveToPlanModal onClose={() => setSavePlan(false)} />}
    </div>
  )
}
