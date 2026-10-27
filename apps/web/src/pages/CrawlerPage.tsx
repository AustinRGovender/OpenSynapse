import { useEffect, useMemo, useState } from 'react'
import type { Crawl } from '@opensynapse/api-client'
import { Badge, Button, Card, EmptyState, Field, Select, Spinner, StatusBadge, TextInput } from '../components/ui/primitives'
import { canEdit, useAuthStore } from '../stores/auth-store'
import { type CrawlAuthType, type CrawlEngine, isActive, useCrawlerStore } from '../stores/crawler-store'
import { useToast } from '../stores/ui-store'
import { navigate } from '../utils/navigate'

const ENGINES: Record<CrawlEngine, { label: string; hint: string }> = {
  colly: { label: 'Colly (fast HTML)', hint: 'Follows links in server-rendered HTML and records form posts. No JavaScript.' },
  rod: { label: 'Rod (headless Chromium)', hint: 'Runs a real browser, so single-page apps work. Needs Chromium on the server (bundled in the Docker image).' },
  zap: { label: 'OWASP ZAP spider', hint: 'Uses a running ZAP daemon (ZAP_API_URL, default http://zap:8080).' },
}

const METHOD_TONE: Record<string, string> = {
  GET: 'text-teal-400',
  POST: 'text-violet-400',
  PUT: 'text-amber-400',
  PATCH: 'text-amber-400',
  DELETE: 'text-red-400',
}

interface SpecOperation {
  method: string
  path: string
  summary: string
}

function specOperations(spec: string | undefined): SpecOperation[] {
  if (!spec) return []
  try {
    const parsed = JSON.parse(spec) as { paths?: Record<string, Record<string, { summary?: string; operationId?: string }>> }
    return Object.entries(parsed.paths ?? {})
      .flatMap(([path, ops]) => Object.entries(ops).map(([method, op]) => ({ method: method.toUpperCase(), path, summary: op.summary || op.operationId || '' })))
      .sort((a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method))
  } catch {
    return []
  }
}

/** Pages as a tree, breadth-first from the entry page along discovered links. */
function siteTree(crawl: Crawl): { url: string; title: string; depth: number }[] {
  const nodes = crawl.graph.nodes
  if (!nodes.length) return []
  const children = new Map<string, string[]>()
  for (const e of crawl.graph.edges) children.set(e.source, [...(children.get(e.source) ?? []), e.target])
  const titles = new Map(nodes.map((n) => [n.url, n.title]))
  const root = titles.has(crawl.entry_url) ? crawl.entry_url : nodes[0].url
  const out: { url: string; title: string; depth: number }[] = []
  const seen = new Set<string>()
  const walk = (url: string, depth: number) => {
    if (seen.has(url)) return
    seen.add(url)
    out.push({ url, title: titles.get(url) ?? '', depth })
    for (const c of children.get(url) ?? []) walk(c, depth + 1)
  }
  walk(root, 0)
  // Pages reached by redirects or forms rather than links.
  for (const n of nodes) walk(n.url, 0)
  return out
}

function pathOf(url: string) {
  try {
    const u = new URL(url)
    return u.pathname + u.search
  } catch {
    return url
  }
}

function ConfigForm() {
  const { config, setConfig, start, starting, crawl } = useCrawlerStore()
  const running = isActive(crawl)
  const set = (patch: Partial<typeof config>) => setConfig(patch)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        start()
      }}
    >
      <div role="radiogroup" aria-label="Source" className="grid grid-cols-2 gap-1 rounded-lg bg-slate-900 p-1 text-xs">
        {([['site', 'Crawl a website'], ['openapi', 'Import OpenAPI']] as const).map(([key, label]) => (
          <button key={key} type="button" role="radio" aria-checked={config.source === key} onClick={() => set({ source: key })} className={`rounded-md px-2 py-1.5 font-medium ${config.source === key ? 'bg-slate-700 text-slate-100' : 'text-slate-400 hover:text-slate-200'}`}>
            {label}
          </button>
        ))}
      </div>

      {config.source === 'openapi' ? (
        <Field label="OpenAPI / Swagger document URL" hint="JSON or YAML, OpenAPI 3 or Swagger 2. Each operation becomes a request in the draft plan.">
          <TextInput type="url" value={config.openapi_url} onChange={(e) => set({ openapi_url: e.target.value })} placeholder="https://api.example.com/openapi.json" required />
        </Field>
      ) : (
        <>
          <Field label="Start URL">
            <TextInput type="url" value={config.entry_url} onChange={(e) => set({ entry_url: e.target.value })} placeholder="https://shop.example.com/" required />
          </Field>
          <Field label="Engine" hint={ENGINES[config.engine].hint}>
            <Select value={config.engine} onChange={(e) => set({ engine: e.target.value as CrawlEngine })}>
              {(Object.keys(ENGINES) as CrawlEngine[]).map((k) => <option key={k} value={k}>{ENGINES[k].label}</option>)}
            </Select>
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Link depth"><TextInput type="number" min={1} max={10} value={config.depth || ''} onChange={(e) => set({ depth: Number(e.target.value) })} /></Field>
            <Field label="Request limit"><TextInput type="number" min={1} max={5000} value={config.limit || ''} onChange={(e) => set({ limit: Number(e.target.value) })} /></Field>
          </div>
          <label className="flex items-center gap-2 text-xs text-slate-300">
            <input type="checkbox" checked={config.same_origin} onChange={(e) => set({ same_origin: e.target.checked })} className="accent-teal-500" />
            Stay on the start URL’s host
          </label>
          <Field label="Skip URLs containing" hint="Comma-separated. Keeps the crawler away from logout and destructive links.">
            <TextInput value={config.blocklist} onChange={(e) => set({ blocklist: e.target.value })} />
          </Field>
          <Field label="Authentication">
            <Select value={config.auth_type} onChange={(e) => set({ auth_type: e.target.value as CrawlAuthType })}>
              <option value="none">None</option>
              <option value="basic">Basic</option>
              <option value="bearer">Bearer token</option>
              <option value="form_login">Log in through a form on the start page</option>
            </Select>
          </Field>
          {(config.auth_type === 'basic' || config.auth_type === 'form_login') && (
            <div className="grid grid-cols-2 gap-3">
              <Field label="Username"><TextInput value={config.username} onChange={(e) => set({ username: e.target.value })} autoComplete="off" /></Field>
              <Field label="Password"><TextInput type="password" value={config.password} onChange={(e) => set({ password: e.target.value })} autoComplete="off" /></Field>
            </div>
          )}
          {config.auth_type === 'bearer' && <Field label="Token"><TextInput value={config.token} onChange={(e) => set({ token: e.target.value })} autoComplete="off" /></Field>}
        </>
      )}

      <Button type="submit" variant="primary" className="w-full" disabled={starting || running}>
        {starting ? 'Starting…' : running ? 'Crawling…' : config.source === 'openapi' ? 'Import' : 'Start crawl'}
      </Button>
      <p className="text-[11px] text-slate-500">Only crawl sites you own or have permission to test. The crawler sends real requests, including form submissions.</p>
    </form>
  )
}

function Results({ crawl }: { crawl: Crawl }) {
  const toast = useToast()
  const editable = canEdit(useAuthStore((s) => s.user))
  const { cancel, generatePlan } = useCrawlerStore()
  const [generating, setGenerating] = useState(false)
  const [tab, setTab] = useState<'requests' | 'pages'>('requests')
  const ops = useMemo(() => specOperations(crawl.openapi_spec), [crawl.openapi_spec])
  const tree = useMemo(() => siteTree(crawl), [crawl])
  const isImport = !!crawl.openapi_url
  const running = isActive(crawl)

  async function onGenerate() {
    setGenerating(true)
    const id = await generatePlan()
    setGenerating(false)
    if (id) {
      toast('Draft plan created; review it before running', 'success')
      navigate(`#/plans/${id}`)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-slate-800 px-4 py-3">
        <StatusBadge status={crawl.status} />
        <span className="truncate font-mono text-xs text-slate-400" title={crawl.entry_url}>{crawl.entry_url}</span>
        <span className="text-xs text-slate-400">{isImport ? `${ops.length || crawl.progress.requests_captured} operations` : `${crawl.progress.pages_discovered} pages · ${crawl.progress.requests_captured} requests`}</span>
        <div className="ml-auto flex gap-2">
          {running && <Button size="sm" variant="danger" onClick={cancel}>Cancel</Button>}
          {crawl.generated_plan_id && <Button size="sm" onClick={() => navigate(`#/plans/${crawl.generated_plan_id}`)}>Open generated plan</Button>}
          {editable && crawl.status === 'completed' && (isImport || crawl.requests.length > 0) && (
            <Button size="sm" variant="primary" disabled={generating} onClick={onGenerate}>{generating ? 'Generating…' : 'Generate plan'}</Button>
          )}
        </div>
      </div>
      {running && <div className="px-4 py-3"><Spinner label={isImport ? 'Reading the document…' : 'Crawling… results appear as pages are found.'} /></div>}
      {crawl.status === 'failed' && <div className="m-4 rounded border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-300" role="alert">{crawl.error_message || 'The crawl failed.'}</div>}
      {crawl.status === 'completed' && !isImport && crawl.requests.length === 0 && (
        <div className="m-4 rounded border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200" role="status">
          Nothing was found. The start page may not return HTML with links (a JSON API, an error page or a login wall). For an API, use Import OpenAPI; for a JavaScript single-page app, try the Rod engine; behind a login, set Authentication.
        </div>
      )}

      {isImport ? (
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-slate-950 text-left text-slate-500">
              <tr><th className="px-4 py-2 font-medium">Method</th><th className="px-4 py-2 font-medium">Path</th><th className="px-4 py-2 font-medium">Summary</th></tr>
            </thead>
            <tbody>
              {ops.map((o) => (
                <tr key={o.method + o.path} className="border-t border-slate-800/60">
                  <td className={`px-4 py-1.5 font-mono font-semibold ${METHOD_TONE[o.method] ?? 'text-slate-400'}`}>{o.method}</td>
                  <td className="px-4 py-1.5 font-mono text-slate-200">{o.path}</td>
                  <td className="px-4 py-1.5 text-slate-400">{o.summary}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <>
          <div role="tablist" className="flex gap-1 border-b border-slate-800 px-3">
            {([['requests', `Requests (${crawl.requests.length})`], ['pages', `Site map (${crawl.graph.nodes.length})`]] as const).map(([key, label]) => (
              <button key={key} role="tab" type="button" aria-selected={tab === key} onClick={() => setTab(key)} className={`-mb-px border-b-2 px-2.5 py-2 text-xs font-medium ${tab === key ? 'border-teal-500 text-teal-300' : 'border-transparent text-slate-400 hover:text-slate-200'}`}>{label}</button>
            ))}
          </div>
          <div className="min-h-0 flex-1 overflow-auto">
            {tab === 'requests' ? (
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-slate-950 text-left text-slate-500">
                  <tr><th className="px-4 py-2 font-medium">Method</th><th className="px-4 py-2 font-medium">URL</th><th className="px-4 py-2 font-medium">Status</th><th className="px-4 py-2 font-medium">Body</th></tr>
                </thead>
                <tbody>
                  {crawl.requests.map((r, i) => (
                    <tr key={i} className="border-t border-slate-800/60">
                      <td className={`px-4 py-1.5 font-mono font-semibold ${METHOD_TONE[r.method] ?? 'text-slate-400'}`}>{r.method}</td>
                      <td className="max-w-md truncate px-4 py-1.5 font-mono text-slate-200" title={r.url}>{r.url}</td>
                      <td className={`px-4 py-1.5 font-mono ${r.status_code >= 400 ? 'text-red-400' : 'text-slate-300'}`}>{r.status_code || '—'}</td>
                      <td className="max-w-xs truncate px-4 py-1.5 font-mono text-slate-500" title={r.body}>{r.body || ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <ul className="p-3 text-xs">
                {tree.map((n) => (
                  <li key={n.url} className="flex items-center gap-2 py-0.5" style={{ paddingLeft: n.depth * 16 }}>
                    <span className="text-slate-600">{n.depth ? '└' : '●'}</span>
                    <span className="font-mono text-slate-200" title={n.url}>{pathOf(n.url)}</span>
                    {n.title && <span className="truncate text-slate-500">{n.title}</span>}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </>
      )}
      {crawl.status === 'completed' && (
        <p className="border-t border-slate-800 px-4 py-2 text-[11px] text-slate-500">
          Generate plan creates a draft with one request per {isImport ? 'operation (using the document’s examples)' : 'unique request'}. Review think times, data and assertions before running it at load.
        </p>
      )}
    </div>
  )
}

/** Crawler (F-07): discover endpoints from a site or an OpenAPI document and draft a plan. */
export function CrawlerPage() {
  const { crawl, error, restore, reset } = useCrawlerStore()
  useEffect(() => {
    restore()
  }, [restore])

  return (
    <div className="flex h-full">
      <aside className="w-80 shrink-0 overflow-y-auto border-r border-slate-800 p-4">
        <div className="mb-4 flex items-center justify-between">
          <h1 className="text-base font-semibold text-slate-100">Crawler</h1>
          {crawl && !isActive(crawl) && <Button size="sm" variant="ghost" onClick={reset}>New crawl</Button>}
        </div>
        <ConfigForm />
        {error && <p className="mt-3 rounded border border-red-500/40 bg-red-500/10 p-2 text-xs text-red-300" role="alert">{error}</p>}
      </aside>
      <section aria-label="Crawl results" className="min-w-0 flex-1">
        {crawl ? (
          <Results crawl={crawl} />
        ) : (
          <div className="mx-auto max-w-xl p-8">
            <EmptyState title="Discover endpoints automatically">
              Crawl a website to record the pages and requests a user would make, or import an OpenAPI document. Either way you get a draft plan to refine in the builder.
            </EmptyState>
            <Card className="mt-6 p-4 text-xs text-slate-400">
              <p className="mb-1 font-medium text-slate-300">Try it with the bundled test APIs</p>
              <p>With the Docker test stack running, choose Import OpenAPI and enter <code className="text-slate-200">http://mock-ecommerce:9002/openapi.yaml</code>.</p>
              <p className="mt-2"><Badge>Tip</Badge> The crawler runs on the server, so URLs are resolved from there: inside Docker use service names such as <code>mock-ecommerce</code>, not <code>localhost</code>.</p>
            </Card>
          </div>
        )}
      </section>
    </div>
  )
}
