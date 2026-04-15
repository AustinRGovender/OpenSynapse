import { useState, useEffect, useCallback, useRef } from 'react'
import {
  useCrawlerStore,
} from '../stores/crawler-store'
import type {
  CrawlAuthType,
  CapturedRequest,
  GraphNode,
  GraphEdge,
  OpenApiOperation,
} from '../stores/crawler-store'

// --- Constants ---

const AUTH_TYPES: { value: CrawlAuthType; label: string }[] = [
  { value: 'none', label: 'None' },
  { value: 'form_login', label: 'Form Login' },
  { value: 'bearer', label: 'Bearer Token' },
  { value: 'basic', label: 'Basic Auth' },
]

const METHOD_COLORS: Record<string, string> = {
  GET: 'bg-teal-600/20 text-teal-400 border-teal-600/30',
  POST: 'bg-violet-600/20 text-violet-400 border-violet-600/30',
  PUT: 'bg-amber-600/20 text-amber-400 border-amber-600/30',
  PATCH: 'bg-amber-600/20 text-amber-400 border-amber-600/30',
  DELETE: 'bg-red-600/20 text-red-400 border-red-600/30',
  HEAD: 'bg-slate-600/20 text-slate-400 border-slate-600/30',
  OPTIONS: 'bg-slate-600/20 text-slate-400 border-slate-600/30',
}

function statusCodeColor(status: number): string {
  if (status >= 200 && status < 300) return 'text-green-400'
  if (status >= 300 && status < 400) return 'text-yellow-400'
  return 'text-red-400'
}

function MethodBadge({ method }: { method: string }) {
  const upper = method.toUpperCase()
  const colors = METHOD_COLORS[upper] ?? 'bg-slate-600/20 text-slate-400 border-slate-600/30'
  return (
    <span
      className={`inline-block rounded border px-1.5 py-0.5 text-[10px] font-bold leading-none ${colors}`}
    >
      {upper}
    </span>
  )
}

// --- Left pane: config form ---

function ConfigPane() {
  const store = useCrawlerStore()
  const { config, status, openApiUrl } = store

  return (
    <div className="flex w-[300px] shrink-0 flex-col border-r border-slate-800 bg-slate-950">
      <div className="border-b border-slate-800 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-200">Crawl Configuration</h2>
      </div>

      <div className="flex-1 overflow-y-auto px-4 py-4">
        <div className="space-y-4">
          {/* Entry URL */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Entry URL</label>
            <input
              type="text"
              value={config.entry_url}
              onChange={(e) => store.setEntryUrl(e.target.value)}
              placeholder="https://example.com"
              disabled={status === 'crawling'}
              className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
            />
          </div>

          {/* Auth section */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Authentication</label>
            <select
              value={config.auth_type}
              onChange={(e) => store.setAuthType(e.target.value as CrawlAuthType)}
              disabled={status === 'crawling'}
              className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 outline-none focus:border-teal-500 disabled:opacity-50"
            >
              {AUTH_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>

            {config.auth_type === 'form_login' && (
              <div className="mt-2 space-y-2">
                <input
                  type="text"
                  value={config.auth.username}
                  onChange={(e) => store.setAuth({ username: e.target.value })}
                  placeholder="Username"
                  disabled={status === 'crawling'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
                />
                <input
                  type="password"
                  value={config.auth.password}
                  onChange={(e) => store.setAuth({ password: e.target.value })}
                  placeholder="Password"
                  disabled={status === 'crawling'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
                />
              </div>
            )}

            {config.auth_type === 'bearer' && (
              <div className="mt-2">
                <input
                  type="text"
                  value={config.auth.token}
                  onChange={(e) => store.setAuth({ token: e.target.value })}
                  placeholder="Bearer token"
                  disabled={status === 'crawling'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
                />
              </div>
            )}

            {config.auth_type === 'basic' && (
              <div className="mt-2 space-y-2">
                <input
                  type="text"
                  value={config.auth.username}
                  onChange={(e) => store.setAuth({ username: e.target.value })}
                  placeholder="Username"
                  disabled={status === 'crawling'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
                />
                <input
                  type="password"
                  value={config.auth.password}
                  onChange={(e) => store.setAuth({ password: e.target.value })}
                  placeholder="Password"
                  disabled={status === 'crawling'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
                />
              </div>
            )}
          </div>

          {/* Crawl depth */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Crawl Depth</label>
            <input
              type="number"
              value={config.depth}
              onChange={(e) => store.setDepth(Math.max(1, parseInt(e.target.value) || 1))}
              min={1}
              max={20}
              disabled={status === 'crawling'}
              className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100 outline-none focus:border-teal-500 disabled:opacity-50"
            />
          </div>

          {/* Same-origin toggle */}
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={config.same_origin}
              onChange={(e) => store.setSameOrigin(e.target.checked)}
              disabled={status === 'crawling'}
              className="h-3.5 w-3.5 rounded border-slate-600 bg-slate-800 accent-teal-500"
            />
            <span className="text-xs text-slate-300">Same-origin only</span>
          </label>

          {/* Path blocklist */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Path Blocklist</label>
            <textarea
              value={config.blocklist}
              onChange={(e) => store.setBlocklist(e.target.value)}
              placeholder="/logout, /delete"
              rows={2}
              disabled={status === 'crawling'}
              className="w-full resize-y rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
            />
          </div>

          {/* Request limit */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Request Limit</label>
            <input
              type="number"
              value={config.request_limit}
              onChange={(e) => store.setRequestLimit(Math.max(1, parseInt(e.target.value) || 1))}
              min={1}
              disabled={status === 'crawling'}
              className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100 outline-none focus:border-teal-500 disabled:opacity-50"
            />
          </div>

          {/* Start crawl button */}
          <button
            onClick={() => store.startCrawl()}
            disabled={status === 'crawling' || !config.entry_url.trim()}
            className="w-full rounded-md bg-teal-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-teal-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {status === 'crawling' ? 'Crawling...' : 'Start Crawl'}
          </button>

          {/* Divider */}
          <div className="flex items-center gap-3">
            <div className="h-px flex-1 bg-slate-800" />
            <span className="text-xs text-slate-600">OR</span>
            <div className="h-px flex-1 bg-slate-800" />
          </div>

          {/* Import OpenAPI */}
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Import OpenAPI</label>
            <input
              type="text"
              value={openApiUrl}
              onChange={(e) => store.setOpenApiUrl(e.target.value)}
              placeholder="https://example.com/openapi.json"
              disabled={status === 'crawling'}
              className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100 placeholder-slate-600 outline-none focus:border-teal-500 disabled:opacity-50"
            />
            <button
              onClick={() => store.fetchOpenAPI()}
              disabled={status === 'crawling' || !openApiUrl.trim()}
              className="mt-2 w-full rounded-md border border-slate-700 bg-slate-900 px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Fetch Spec
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// --- Centre pane: progress and results ---

function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(100, Math.max(0, percent))
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-slate-800">
      <div
        className="h-full rounded-full bg-teal-500 transition-all duration-300"
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}

function CrawlingProgress() {
  const { progress, config } = useCrawlerStore()
  const percent = config.request_limit > 0
    ? (progress.requests_captured / config.request_limit) * 100
    : 0

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center gap-3">
        <div className="h-3 w-3 animate-pulse rounded-full bg-teal-500" />
        <span className="text-sm font-medium text-slate-200">Crawling in progress...</span>
      </div>
      <ProgressBar percent={percent} />
      <div className="flex gap-6">
        <div>
          <p className="text-xs text-slate-500">Pages Discovered</p>
          <p className="text-xl font-semibold text-slate-100">{progress.pages_discovered}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">Requests Captured</p>
          <p className="text-xl font-semibold text-slate-100">{progress.requests_captured}</p>
        </div>
      </div>
    </div>
  )
}

// Simple force-directed graph using SVG + canvas-free approach
function SiteGraph({ nodes, edges }: { nodes: GraphNode[]; edges: GraphEdge[] }) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [positions, setPositions] = useState<Map<string, { x: number; y: number }>>(new Map())

  useEffect(() => {
    if (nodes.length === 0) return

    // Simple radial layout for nodes
    const posMap = new Map<string, { x: number; y: number }>()
    const centerX = 300
    const centerY = 200

    if (nodes.length === 1) {
      posMap.set(nodes[0].url, { x: centerX, y: centerY })
    } else {
      const angleStep = (2 * Math.PI) / nodes.length
      const radius = Math.min(150, 40 + nodes.length * 8)
      nodes.forEach((node, i) => {
        const angle = i * angleStep - Math.PI / 2
        posMap.set(node.url, {
          x: centerX + radius * Math.cos(angle),
          y: centerY + radius * Math.sin(angle),
        })
      })
    }

    setPositions(posMap)
  }, [nodes])

  if (nodes.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-600">
        No graph data available.
      </div>
    )
  }

  return (
    <svg
      ref={svgRef}
      viewBox="0 0 600 400"
      className="h-full w-full"
      preserveAspectRatio="xMidYMid meet"
    >
      {/* Edges */}
      {edges.map((edge, i) => {
        const from = positions.get(edge.source)
        const to = positions.get(edge.target)
        if (!from || !to) return null
        return (
          <line
            key={`edge-${i}`}
            x1={from.x}
            y1={from.y}
            x2={to.x}
            y2={to.y}
            className="stroke-slate-700"
            strokeWidth={1}
          />
        )
      })}

      {/* Nodes */}
      {nodes.map((node) => {
        const pos = positions.get(node.url)
        if (!pos) return null
        // Truncate the label to just the pathname
        let label = node.title || node.url
        try {
          const parsed = new URL(node.url)
          label = parsed.pathname || '/'
        } catch {
          // Use raw url
        }
        if (label.length > 24) label = label.slice(0, 22) + '...'

        return (
          <g key={node.url}>
            <circle
              cx={pos.x}
              cy={pos.y}
              r={6}
              className="fill-teal-500 stroke-teal-400"
              strokeWidth={1.5}
            />
            <text
              x={pos.x}
              y={pos.y + 16}
              textAnchor="middle"
              className="fill-slate-400 text-[9px]"
            >
              {label}
            </text>
          </g>
        )
      })}
    </svg>
  )
}

function OpenApiOpsList({ operations }: { operations: OpenApiOperation[] }) {
  return (
    <div className="space-y-1 p-4">
      <h3 className="mb-3 text-sm font-semibold text-slate-200">
        Parsed Operations ({operations.length})
      </h3>
      {operations.map((op, i) => (
        <div
          key={`${op.method}-${op.path}-${i}`}
          className="flex items-center gap-3 rounded-md border border-slate-800 bg-slate-900/50 px-3 py-2"
        >
          <MethodBadge method={op.method} />
          <span className="flex-1 truncate font-mono text-xs text-slate-300">{op.path}</span>
          <span className="truncate text-xs text-slate-500">{op.summary}</span>
        </div>
      ))}
    </div>
  )
}

function CentrePane() {
  const { status, graph, openApiOps, error } = useCrawlerStore()

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <div className="border-b border-slate-800 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-200">
          {status === 'idle' && 'Results'}
          {status === 'crawling' && 'Progress'}
          {status === 'completed' && 'Crawl Complete'}
          {status === 'failed' && 'Crawl Failed'}
        </h2>
      </div>

      <div className="flex-1 overflow-auto">
        {error && (
          <div className="mx-4 mt-4 rounded-md border border-red-800/50 bg-red-950/50 px-4 py-3 text-xs text-red-400">
            {error}
          </div>
        )}

        {status === 'idle' && !error && (
          <div className="flex h-full items-center justify-center text-sm text-slate-600">
            Configure and start a crawl, or import an OpenAPI spec.
          </div>
        )}

        {status === 'crawling' && <CrawlingProgress />}

        {status === 'completed' && openApiOps.length > 0 && (
          <OpenApiOpsList operations={openApiOps} />
        )}

        {status === 'completed' && openApiOps.length === 0 && (
          <div className="flex h-full flex-col">
            <div className="px-4 py-3">
              <div className="flex gap-6">
                <div>
                  <p className="text-xs text-slate-500">Pages Discovered</p>
                  <p className="text-lg font-semibold text-slate-100">
                    {graph.nodes.length}
                  </p>
                </div>
              </div>
            </div>
            <div className="min-h-0 flex-1 px-2">
              <SiteGraph nodes={graph.nodes} edges={graph.edges} />
            </div>
          </div>
        )}

        {status === 'failed' && !error && (
          <div className="flex h-full items-center justify-center text-sm text-red-400">
            The crawl encountered an error.
          </div>
        )}
      </div>
    </div>
  )
}

// --- Right pane: captured requests ---

function RequestDetail({ request }: { request: CapturedRequest }) {
  return (
    <div className="border-t border-slate-800 p-3">
      <div className="mb-2 flex items-center gap-2">
        <MethodBadge method={request.method} />
        <span className={`text-xs font-mono ${statusCodeColor(request.status)}`}>
          {request.status}
        </span>
      </div>
      <p className="mb-2 break-all font-mono text-[10px] text-slate-400">{request.url}</p>
      {request.headers && Object.keys(request.headers).length > 0 && (
        <div className="mb-2">
          <p className="mb-1 text-[10px] font-semibold text-slate-500">Headers</p>
          <div className="max-h-24 overflow-auto rounded bg-slate-950 p-2 text-[10px]">
            {Object.entries(request.headers).map(([k, v]) => (
              <div key={k}>
                <span className="text-teal-400">{k}:</span>{' '}
                <span className="text-slate-400">{v}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      {request.body && (
        <div>
          <p className="mb-1 text-[10px] font-semibold text-slate-500">Body</p>
          <pre className="max-h-24 overflow-auto rounded bg-slate-950 p-2 font-mono text-[10px] text-slate-400">
            {request.body}
          </pre>
        </div>
      )}
    </div>
  )
}

function RightPane() {
  const { requests, selectedRequestId, setSelectedRequest } = useCrawlerStore()
  const selectedReq = requests.find((r) => r.id === selectedRequestId) ?? null

  return (
    <div className="flex w-[280px] shrink-0 flex-col border-l border-slate-800 bg-slate-950">
      <div className="border-b border-slate-800 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-200">
          Captured Requests ({requests.length})
        </h2>
      </div>

      <div className="flex-1 overflow-y-auto">
        {requests.length === 0 && (
          <div className="px-4 py-6 text-center text-xs text-slate-600">
            No requests captured yet.
          </div>
        )}

        {requests.map((req) => {
          let shortPath = req.path || req.url
          try {
            shortPath = new URL(req.url).pathname
          } catch {
            // Use raw
          }
          if (shortPath.length > 32) shortPath = shortPath.slice(0, 30) + '...'

          const isSelected = req.id === selectedRequestId
          return (
            <button
              key={req.id}
              onClick={() => setSelectedRequest(isSelected ? null : req.id)}
              className={`flex w-full items-center gap-2 border-b border-slate-800/50 px-3 py-2 text-left transition-colors hover:bg-slate-900 ${
                isSelected ? 'bg-slate-900' : ''
              }`}
            >
              <MethodBadge method={req.method} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-slate-300">
                {shortPath}
              </span>
              <span className={`text-[10px] font-mono ${statusCodeColor(req.status)}`}>
                {req.status}
              </span>
            </button>
          )
        })}
      </div>

      {selectedReq && <RequestDetail request={selectedReq} />}
    </div>
  )
}

// --- Bottom strip ---

function BottomStrip() {
  const { status, crawlId, cancelCrawl, generatePlan } = useCrawlerStore()
  const [generating, setGenerating] = useState(false)

  const handleGenerate = useCallback(async () => {
    setGenerating(true)
    const planId = await generatePlan()
    setGenerating(false)
    if (planId) {
      window.location.hash = `#/plans/${planId}`
    }
  }, [generatePlan])

  const isComplete = status === 'completed'
  const isCrawling = status === 'crawling'

  return (
    <div className="flex shrink-0 items-center justify-between border-t border-slate-800 bg-slate-950 px-4 py-3">
      <div className="text-xs text-slate-500">
        {crawlId && (
          <span>
            Crawl ID: <span className="font-mono text-slate-400">{crawlId.slice(0, 8)}</span>
          </span>
        )}
      </div>
      <div className="flex gap-2">
        {isCrawling && (
          <button
            onClick={cancelCrawl}
            className="rounded-md border border-slate-700 bg-slate-900 px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-800"
          >
            Cancel
          </button>
        )}
        {isComplete && (
          <button
            onClick={handleGenerate}
            disabled={generating}
            className="rounded-md bg-teal-600 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {generating ? 'Generating...' : 'Generate Plan'}
          </button>
        )}
      </div>
    </div>
  )
}

// --- Main page ---

export function CrawlerPage({ onBack }: { onBack: () => void }) {
  const store = useCrawlerStore()

  return (
    <div className="flex h-screen flex-col bg-slate-950 text-slate-100">
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
          <h1 className="text-lg font-semibold tracking-tight">Crawler</h1>
        </div>
        {store.status !== 'idle' && (
          <button
            onClick={store.reset}
            className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-slate-800"
          >
            Reset
          </button>
        )}
      </header>

      {/* Main content */}
      <div className="flex min-h-0 flex-1">
        <ConfigPane />
        <CentrePane />
        <RightPane />
      </div>

      {/* Bottom strip */}
      <BottomStrip />
    </div>
  )
}
