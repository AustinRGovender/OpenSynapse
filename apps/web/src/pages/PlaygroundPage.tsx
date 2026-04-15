import { useState, useCallback } from 'react'
import {
  usePlaygroundStore,
} from '../stores/playground-store'
import type {
  HttpMethod,
  KeyValueRow,
  BodyType,
  AuthType,
  SavedRequest,
} from '../stores/playground-store'

// --- Constants ---

const METHODS: HttpMethod[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

const METHOD_COLORS: Record<HttpMethod, string> = {
  GET: 'bg-teal-600/20 text-teal-400 border-teal-600/30',
  POST: 'bg-violet-600/20 text-violet-400 border-violet-600/30',
  PUT: 'bg-amber-600/20 text-amber-400 border-amber-600/30',
  PATCH: 'bg-amber-600/20 text-amber-400 border-amber-600/30',
  DELETE: 'bg-red-600/20 text-red-400 border-red-600/30',
  HEAD: 'bg-slate-600/20 text-slate-400 border-slate-600/30',
  OPTIONS: 'bg-slate-600/20 text-slate-400 border-slate-600/30',
}

const METHOD_SELECT_COLORS: Record<HttpMethod, string> = {
  GET: 'text-teal-400',
  POST: 'text-violet-400',
  PUT: 'text-amber-400',
  PATCH: 'text-amber-400',
  DELETE: 'text-red-400',
  HEAD: 'text-slate-400',
  OPTIONS: 'text-slate-400',
}

type RequestTab = 'params' | 'headers' | 'body' | 'auth'
type ResponseTab = 'body' | 'headers' | 'timing'

const TIMING_SEGMENTS: Array<{ key: string; label: string; color: string }> = [
  { key: 'dns_ms', label: 'DNS', color: 'bg-sky-500' },
  { key: 'connect_ms', label: 'Connect', color: 'bg-teal-500' },
  { key: 'tls_ms', label: 'TLS', color: 'bg-violet-500' },
  { key: 'send_ms', label: 'Send', color: 'bg-amber-500' },
  { key: 'wait_ms', label: 'Wait', color: 'bg-rose-500' },
  { key: 'receive_ms', label: 'Receive', color: 'bg-green-500' },
]

// --- Helpers ---

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function statusColorClass(status: number): string {
  if (status >= 200 && status < 300) return 'text-green-400 bg-green-500/20'
  if (status >= 300 && status < 400) return 'text-yellow-400 bg-yellow-500/20'
  return 'text-red-400 bg-red-500/20'
}

function tryPrettyJson(str: string): string {
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}

function groupByCollection(items: SavedRequest[]): Record<string, Array<{ item: SavedRequest; index: number }>> {
  const groups: Record<string, Array<{ item: SavedRequest; index: number }>> = {}
  items.forEach((item, index) => {
    const col = item.collection || 'Unsorted'
    if (!groups[col]) groups[col] = []
    groups[col].push({ item, index })
  })
  return groups
}

// --- Sub-components ---

function KeyValueEditor({
  rows,
  onChange,
  showEnabled,
}: {
  rows: KeyValueRow[]
  onChange: (rows: KeyValueRow[]) => void
  showEnabled?: boolean
}) {
  const updateRow = (idx: number, updates: Partial<KeyValueRow>) => {
    const next = rows.map((r, i) => (i === idx ? { ...r, ...updates } : r))
    onChange(next)
  }

  const addRow = () => {
    onChange([...rows, { key: '', value: '', enabled: true }])
  }

  const removeRow = (idx: number) => {
    if (rows.length <= 1) {
      onChange([{ key: '', value: '', enabled: true }])
      return
    }
    onChange(rows.filter((_, i) => i !== idx))
  }

  return (
    <div className="space-y-1.5">
      {rows.map((row, idx) => (
        <div key={idx} className="flex items-center gap-2">
          {showEnabled && (
            <input
              type="checkbox"
              checked={row.enabled}
              onChange={(e) => updateRow(idx, { enabled: e.target.checked })}
              className="h-3.5 w-3.5 rounded border-slate-600 bg-slate-800 accent-teal-500"
            />
          )}
          <input
            type="text"
            value={row.key}
            onChange={(e) => updateRow(idx, { key: e.target.value })}
            placeholder="Key"
            className="flex-1 rounded border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <input
            type="text"
            value={row.value}
            onChange={(e) => updateRow(idx, { value: e.target.value })}
            placeholder="Value"
            className="flex-1 rounded border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <button
            onClick={() => removeRow(idx)}
            className="rounded px-1.5 py-1 text-xs text-slate-500 hover:bg-slate-800 hover:text-red-400"
            title="Remove row"
          >
            x
          </button>
        </div>
      ))}
      <button
        onClick={addRow}
        className="rounded px-2 py-1 text-xs text-teal-400 hover:bg-slate-800"
      >
        + Add row
      </button>
    </div>
  )
}

function MethodBadge({ method }: { method: HttpMethod }) {
  return (
    <span
      className={`inline-block rounded border px-1.5 py-0.5 text-[10px] font-bold leading-none ${METHOD_COLORS[method]}`}
    >
      {method}
    </span>
  )
}

// --- Main component ---

export function PlaygroundPage({ onBack }: { onBack: () => void }) {
  const store = usePlaygroundStore()

  const [requestTab, setRequestTab] = useState<RequestTab>('params')
  const [responseTab, setResponseTab] = useState<ResponseTab>('body')
  const [prettyPrint, setPrettyPrint] = useState(true)
  const [saveModalOpen, setSaveModalOpen] = useState(false)
  const [saveName, setSaveName] = useState('')
  const [saveCollection, setSaveCollection] = useState('Default')
  const [sidebarOpen, setSidebarOpen] = useState(true)

  const handleSend = useCallback(() => {
    store.sendRequest()
  }, [store])

  const handleSaveToCollection = useCallback(() => {
    if (!saveName.trim()) return
    store.saveToCollection(saveName.trim(), saveCollection.trim() || 'Default')
    setSaveName('')
    setSaveModalOpen(false)
  }, [store, saveName, saveCollection])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSend()
      }
    },
    [handleSend],
  )

  const grouped = groupByCollection(store.collections)

  const reqTabClass = (tab: RequestTab) =>
    `px-3 py-1.5 text-xs font-medium rounded-t transition-colors ${
      requestTab === tab
        ? 'bg-slate-800 text-slate-100 border-b-2 border-teal-500'
        : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'
    }`

  const resTabClass = (tab: ResponseTab) =>
    `px-3 py-1.5 text-xs font-medium rounded-t transition-colors ${
      responseTab === tab
        ? 'bg-slate-800 text-slate-100 border-b-2 border-teal-500'
        : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50'
    }`

  return (
    <div className="flex h-screen flex-col bg-slate-950 text-slate-100" onKeyDown={handleKeyDown}>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between border-b border-slate-800 px-6 py-3">
        <div className="flex items-center gap-3">
          <button
            onClick={onBack}
            className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            &larr; Home
          </button>
          <div className="h-4 w-px bg-slate-800" />
          <h1 className="text-lg font-semibold tracking-tight">Playground</h1>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSaveModalOpen(true)}
            className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-slate-800"
          >
            Save to Collection
          </button>
          <button
            onClick={() => store.setSaveToPlanOpen(true)}
            className="rounded-md bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500"
          >
            Save to Plan
          </button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        {/* Collections sidebar */}
        {sidebarOpen && (
          <aside className="flex w-52 shrink-0 flex-col border-r border-slate-800 bg-slate-950">
            <div className="flex items-center justify-between border-b border-slate-800 px-3 py-2">
              <span className="text-xs font-semibold text-slate-400">Collections</span>
              <button
                onClick={() => setSidebarOpen(false)}
                className="rounded px-1 py-0.5 text-xs text-slate-500 hover:bg-slate-800 hover:text-slate-300"
                title="Collapse sidebar"
              >
                &lsaquo;
              </button>
            </div>
            <div className="flex-1 overflow-y-auto px-2 py-2">
              {store.collections.length === 0 && (
                <p className="px-1 text-xs text-slate-600">No saved requests yet.</p>
              )}
              {Object.entries(grouped).map(([col, items]) => (
                <div key={col} className="mb-3">
                  <p className="mb-1 px-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500">
                    {col}
                  </p>
                  {items.map(({ item, index }) => (
                    <button
                      key={index}
                      onClick={() => store.loadFromCollection(index)}
                      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-slate-800"
                    >
                      <MethodBadge method={item.method} />
                      <span className="flex-1 truncate text-xs text-slate-300">{item.name}</span>
                    </button>
                  ))}
                </div>
              ))}
            </div>
          </aside>
        )}

        {!sidebarOpen && (
          <button
            onClick={() => setSidebarOpen(true)}
            className="flex w-8 shrink-0 items-center justify-center border-r border-slate-800 bg-slate-950 text-slate-500 hover:bg-slate-900 hover:text-slate-300"
            title="Expand sidebar"
          >
            <span className="text-xs">&rsaquo;</span>
          </button>
        )}

        {/* Main content */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* Request builder */}
          <div className="flex shrink-0 flex-col border-b border-slate-800">
            {/* URL bar */}
            <div className="flex items-center gap-2 px-4 py-3">
              <select
                value={store.method}
                onChange={(e) => store.setMethod(e.target.value as HttpMethod)}
                className={`w-28 rounded-md border border-slate-700 bg-slate-900 px-2 py-2 text-xs font-bold outline-none focus:border-teal-500 ${METHOD_SELECT_COLORS[store.method]}`}
              >
                {METHODS.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
              <input
                type="text"
                value={store.url}
                onChange={(e) => store.setUrl(e.target.value)}
                placeholder="https://api.example.com/endpoint"
                className="flex-1 rounded-md border border-slate-700 bg-slate-900 px-3 py-2 font-mono text-sm text-slate-100 placeholder-slate-600 outline-none focus:border-teal-500"
              />
              <button
                onClick={handleSend}
                disabled={store.loading}
                className="rounded-md bg-teal-600 px-5 py-2 text-sm font-semibold text-white hover:bg-teal-500 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {store.loading ? 'Sending...' : 'Send'}
              </button>
            </div>

            {/* Request tabs */}
            <div className="flex gap-0.5 border-b border-slate-800 px-4">
              <button className={reqTabClass('params')} onClick={() => setRequestTab('params')}>
                Params
              </button>
              <button className={reqTabClass('headers')} onClick={() => setRequestTab('headers')}>
                Headers
              </button>
              <button className={reqTabClass('body')} onClick={() => setRequestTab('body')}>
                Body
              </button>
              <button className={reqTabClass('auth')} onClick={() => setRequestTab('auth')}>
                Auth
              </button>
            </div>

            {/* Tab content */}
            <div className="max-h-56 overflow-y-auto px-4 py-3">
              {requestTab === 'params' && (
                <KeyValueEditor rows={store.params} onChange={store.setParams} />
              )}

              {requestTab === 'headers' && (
                <KeyValueEditor
                  rows={store.headers}
                  onChange={store.setHeaders}
                  showEnabled
                />
              )}

              {requestTab === 'body' && (
                <BodyEditor
                  bodyType={store.bodyType}
                  body={store.body}
                  onBodyTypeChange={store.setBodyType}
                  onBodyChange={store.setBody}
                />
              )}

              {requestTab === 'auth' && (
                <AuthEditor
                  authType={store.authType}
                  authConfig={store.authConfig}
                  onAuthTypeChange={store.setAuthType}
                  onAuthConfigChange={store.setAuthConfig}
                />
              )}
            </div>
          </div>

          {/* Response viewer */}
          <div className="flex min-h-0 flex-1 flex-col">
            {store.error && (
              <div className="mx-4 mt-3 rounded-md border border-red-800/50 bg-red-950/50 px-4 py-3 text-xs text-red-400">
                {store.error}
              </div>
            )}

            {store.loading && !store.response && (
              <div className="flex flex-1 items-center justify-center text-sm text-slate-500">
                Sending request...
              </div>
            )}

            {!store.response && !store.loading && !store.error && (
              <div className="flex flex-1 items-center justify-center text-sm text-slate-600">
                Enter a URL and click Send to get started.
              </div>
            )}

            {store.response && (
              <>
                {/* Status line */}
                <div className="flex shrink-0 items-center gap-3 border-b border-slate-800 px-4 py-2">
                  <span
                    className={`rounded px-2 py-0.5 text-xs font-bold ${statusColorClass(store.response.status)}`}
                  >
                    {store.response.status} {store.response.statusText}
                  </span>
                  <span className="text-xs text-slate-500">
                    {store.response.duration.toFixed(0)} ms
                  </span>
                  <span className="text-xs text-slate-500">
                    {formatBytes(store.response.size)}
                  </span>
                </div>

                {/* Response tabs */}
                <div className="flex shrink-0 gap-0.5 border-b border-slate-800 px-4">
                  <button
                    className={resTabClass('body')}
                    onClick={() => setResponseTab('body')}
                  >
                    Body
                  </button>
                  <button
                    className={resTabClass('headers')}
                    onClick={() => setResponseTab('headers')}
                  >
                    Headers
                  </button>
                  <button
                    className={resTabClass('timing')}
                    onClick={() => setResponseTab('timing')}
                  >
                    Timing
                  </button>
                </div>

                {/* Response tab content */}
                <div className="min-h-0 flex-1 overflow-auto">
                  {responseTab === 'body' && (
                    <ResponseBodyView
                      body={store.response.body}
                      prettyPrint={prettyPrint}
                      onTogglePretty={() => setPrettyPrint((p) => !p)}
                    />
                  )}

                  {responseTab === 'headers' && (
                    <ResponseHeadersView headers={store.response.headers} />
                  )}

                  {responseTab === 'timing' && (
                    <TimingView timing={store.response.timing} />
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Save to Collection modal */}
      {saveModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80">
          <div className="w-96 rounded-lg border border-slate-700 bg-slate-900 p-5">
            <h2 className="mb-4 text-sm font-semibold text-slate-200">Save to Collection</h2>
            <div className="space-y-3">
              <div>
                <label className="mb-1 block text-xs text-slate-500">Request Name</label>
                <input
                  type="text"
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  placeholder="My Request"
                  className="w-full rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
                  autoFocus
                />
              </div>
              <div>
                <label className="mb-1 block text-xs text-slate-500">Collection</label>
                <input
                  type="text"
                  value={saveCollection}
                  onChange={(e) => setSaveCollection(e.target.value)}
                  placeholder="Default"
                  className="w-full rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
                />
              </div>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button
                onClick={() => setSaveModalOpen(false)}
                className="rounded-md border border-slate-700 px-3 py-1.5 text-xs text-slate-400 hover:bg-slate-800"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveToCollection}
                disabled={!saveName.trim()}
                className="rounded-md bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Save to Plan modal */}
      {store.saveToPlanOpen && (
        <SaveToPlanModal onClose={() => store.setSaveToPlanOpen(false)} />
      )}
    </div>
  )
}

// --- Body editor ---

function BodyEditor({
  bodyType,
  body,
  onBodyTypeChange,
  onBodyChange,
}: {
  bodyType: BodyType
  body: string
  onBodyTypeChange: (t: BodyType) => void
  onBodyChange: (b: string) => void
}) {
  const types: BodyType[] = ['none', 'raw', 'json', 'form']

  return (
    <div>
      <div className="mb-2 flex gap-2">
        {types.map((t) => (
          <button
            key={t}
            onClick={() => onBodyTypeChange(t)}
            className={`rounded px-2.5 py-1 text-xs font-medium ${
              bodyType === t
                ? 'bg-teal-600/20 text-teal-400'
                : 'text-slate-500 hover:bg-slate-800 hover:text-slate-300'
            }`}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {bodyType === 'none' && (
        <p className="text-xs text-slate-600">This request does not have a body.</p>
      )}

      {(bodyType === 'raw' || bodyType === 'json') && (
        <textarea
          value={body}
          onChange={(e) => onBodyChange(e.target.value)}
          placeholder={
            bodyType === 'json'
              ? '{\n  "key": "value"\n}'
              : 'Request body...'
          }
          className="h-32 w-full resize-y rounded border border-slate-700 bg-slate-900 px-3 py-2 font-mono text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
        />
      )}

      {bodyType === 'form' && (
        <textarea
          value={body}
          onChange={(e) => onBodyChange(e.target.value)}
          placeholder="key1=value1&key2=value2"
          className="h-32 w-full resize-y rounded border border-slate-700 bg-slate-900 px-3 py-2 font-mono text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
        />
      )}
    </div>
  )
}

// --- Auth editor ---

function AuthEditor({
  authType,
  authConfig,
  onAuthTypeChange,
  onAuthConfigChange,
}: {
  authType: AuthType
  authConfig: { username: string; password: string; token: string; apiKeyName: string; apiKeyValue: string; apiKeyIn: 'header' | 'query' }
  onAuthTypeChange: (t: AuthType) => void
  onAuthConfigChange: (c: Partial<typeof authConfig>) => void
}) {
  const types: AuthType[] = ['none', 'basic', 'bearer', 'apikey']
  const labels: Record<AuthType, string> = {
    none: 'None',
    basic: 'Basic',
    bearer: 'Bearer',
    apikey: 'API Key',
  }

  return (
    <div>
      <div className="mb-3 flex gap-2">
        {types.map((t) => (
          <button
            key={t}
            onClick={() => onAuthTypeChange(t)}
            className={`rounded px-2.5 py-1 text-xs font-medium ${
              authType === t
                ? 'bg-teal-600/20 text-teal-400'
                : 'text-slate-500 hover:bg-slate-800 hover:text-slate-300'
            }`}
          >
            {labels[t]}
          </button>
        ))}
      </div>

      {authType === 'none' && (
        <p className="text-xs text-slate-600">No authentication.</p>
      )}

      {authType === 'basic' && (
        <div className="space-y-2">
          <input
            type="text"
            value={authConfig.username}
            onChange={(e) => onAuthConfigChange({ username: e.target.value })}
            placeholder="Username"
            className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <input
            type="password"
            value={authConfig.password}
            onChange={(e) => onAuthConfigChange({ password: e.target.value })}
            placeholder="Password"
            className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
        </div>
      )}

      {authType === 'bearer' && (
        <input
          type="text"
          value={authConfig.token}
          onChange={(e) => onAuthConfigChange({ token: e.target.value })}
          placeholder="Bearer token"
          className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
        />
      )}

      {authType === 'apikey' && (
        <div className="space-y-2">
          <input
            type="text"
            value={authConfig.apiKeyName}
            onChange={(e) => onAuthConfigChange({ apiKeyName: e.target.value })}
            placeholder="Key name (e.g. X-API-Key)"
            className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <input
            type="text"
            value={authConfig.apiKeyValue}
            onChange={(e) => onAuthConfigChange({ apiKeyValue: e.target.value })}
            placeholder="Key value"
            className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <div className="flex items-center gap-3">
            <span className="text-xs text-slate-500">Add to:</span>
            <label className="flex items-center gap-1.5 text-xs text-slate-300">
              <input
                type="radio"
                name="apiKeyIn"
                checked={authConfig.apiKeyIn === 'header'}
                onChange={() => onAuthConfigChange({ apiKeyIn: 'header' })}
                className="accent-teal-500"
              />
              Header
            </label>
            <label className="flex items-center gap-1.5 text-xs text-slate-300">
              <input
                type="radio"
                name="apiKeyIn"
                checked={authConfig.apiKeyIn === 'query'}
                onChange={() => onAuthConfigChange({ apiKeyIn: 'query' })}
                className="accent-teal-500"
              />
              Query Param
            </label>
          </div>
        </div>
      )}
    </div>
  )
}

// --- Response body view ---

function ResponseBodyView({
  body,
  prettyPrint,
  onTogglePretty,
}: {
  body: string
  prettyPrint: boolean
  onTogglePretty: () => void
}) {
  const displayed = prettyPrint ? tryPrettyJson(body) : body

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-end border-b border-slate-800 px-4 py-1.5">
        <button
          onClick={onTogglePretty}
          className={`rounded px-2 py-0.5 text-[10px] font-medium ${
            prettyPrint
              ? 'bg-teal-600/20 text-teal-400'
              : 'text-slate-500 hover:bg-slate-800 hover:text-slate-300'
          }`}
        >
          Pretty
        </button>
      </div>
      <pre className="flex-1 overflow-auto whitespace-pre-wrap break-all bg-slate-950 p-4 font-mono text-xs leading-relaxed text-slate-300">
        {displayed}
      </pre>
    </div>
  )
}

// --- Response headers view ---

function ResponseHeadersView({ headers }: { headers: Record<string, string> }) {
  const entries = Object.entries(headers)

  return (
    <div className="p-4">
      {entries.length === 0 ? (
        <p className="text-xs text-slate-600">No headers returned.</p>
      ) : (
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-slate-800">
              <th className="pb-2 pr-4 font-medium text-slate-500">Name</th>
              <th className="pb-2 font-medium text-slate-500">Value</th>
            </tr>
          </thead>
          <tbody>
            {entries.map(([name, value]) => (
              <tr key={name} className="border-b border-slate-800/50">
                <td className="py-1.5 pr-4 font-mono text-teal-400">{name}</td>
                <td className="py-1.5 font-mono text-slate-300">{value}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

// --- Timing view ---

function TimingView({ timing }: { timing: { dns_ms: number; connect_ms: number; tls_ms: number; send_ms: number; wait_ms: number; receive_ms: number; total_ms: number } }) {
  const total = timing.total_ms || 1

  return (
    <div className="p-4">
      <p className="mb-3 text-xs text-slate-500">
        Total: <span className="font-mono text-slate-300">{timing.total_ms.toFixed(0)} ms</span>
      </p>

      {/* Stacked bar */}
      <div className="mb-4 flex h-6 overflow-hidden rounded">
        {TIMING_SEGMENTS.map(({ key, color }) => {
          const val = timing[key as keyof typeof timing] as number
          const pct = (val / total) * 100
          if (pct < 0.5) return null
          return (
            <div
              key={key}
              className={`${color} flex items-center justify-center`}
              style={{ width: `${pct}%` }}
              title={`${key}: ${val.toFixed(0)} ms`}
            >
              {pct > 8 && (
                <span className="text-[9px] font-bold text-white">
                  {val.toFixed(0)}
                </span>
              )}
            </div>
          )
        })}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-x-4 gap-y-2">
        {TIMING_SEGMENTS.map(({ key, label, color }) => {
          const val = timing[key as keyof typeof timing] as number
          return (
            <div key={key} className="flex items-center gap-1.5">
              <span className={`inline-block h-2.5 w-2.5 rounded-sm ${color}`} />
              <span className="text-xs text-slate-400">
                {label}: <span className="font-mono text-slate-300">{val.toFixed(0)} ms</span>
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// --- Save to Plan modal ---

function SaveToPlanModal({ onClose }: { onClose: () => void }) {
  const store = usePlaygroundStore()
  const [planId, setPlanId] = useState('')
  const [parentNodeId, setParentNodeId] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    if (!planId.trim() || !parentNodeId.trim()) return
    setSaving(true)
    try {
      await store.saveToPlan(planId.trim(), parentNodeId.trim())
      onClose()
    } catch {
      // Error handled by store
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80">
      <div className="w-96 rounded-lg border border-slate-700 bg-slate-900 p-5">
        <h2 className="mb-4 text-sm font-semibold text-slate-200">Save to Plan</h2>
        <p className="mb-3 text-xs text-slate-500">
          Add this request as an HTTP sampler node to an existing plan.
        </p>
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs text-slate-500">Plan ID</label>
            <input
              type="text"
              value={planId}
              onChange={(e) => setPlanId(e.target.value)}
              placeholder="Plan UUID"
              className="w-full rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
              autoFocus
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">Parent Node ID</label>
            <input
              type="text"
              value={parentNodeId}
              onChange={(e) => setParentNodeId(e.target.value)}
              placeholder="Parent node UUID"
              className="w-full rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-md border border-slate-700 px-3 py-1.5 text-xs text-slate-400 hover:bg-slate-800"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={!planId.trim() || !parentNodeId.trim() || saving}
            className="rounded-md bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Add to Plan'}
          </button>
        </div>
      </div>
    </div>
  )
}
