import { create } from 'zustand'

// --- Types ---

export type CrawlAuthType = 'none' | 'form_login' | 'bearer' | 'basic'

export interface CrawlAuthConfig {
  username: string
  password: string
  token: string
}

export interface CrawlConfig {
  entry_url: string
  auth_type: CrawlAuthType
  auth: CrawlAuthConfig
  depth: number
  same_origin: boolean
  blocklist: string
  request_limit: number
}

export type CrawlStatus = 'idle' | 'crawling' | 'completed' | 'failed'

export interface CrawlProgress {
  pages_discovered: number
  requests_captured: number
}

export interface GraphNode {
  url: string
  title: string
}

export interface GraphEdge {
  source: string
  target: string
}

export interface CrawlGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface CapturedRequest {
  id: string
  method: string
  url: string
  path: string
  status: number
  headers?: Record<string, string>
  body?: string
  response_body?: string
}

export interface OpenApiOperation {
  method: string
  path: string
  summary: string
  operationId: string
}

interface CrawlerState {
  // Config
  config: CrawlConfig

  // Crawl state
  crawlId: string | null
  status: CrawlStatus
  progress: CrawlProgress
  error: string | null

  // Results
  graph: CrawlGraph
  requests: CapturedRequest[]
  selectedRequestId: string | null

  // OpenAPI
  openApiUrl: string
  openApiOps: OpenApiOperation[]

  // Config actions
  setEntryUrl: (url: string) => void
  setAuthType: (authType: CrawlAuthType) => void
  setAuth: (auth: Partial<CrawlAuthConfig>) => void
  setDepth: (depth: number) => void
  setSameOrigin: (sameOrigin: boolean) => void
  setBlocklist: (blocklist: string) => void
  setRequestLimit: (limit: number) => void
  setOpenApiUrl: (url: string) => void
  setSelectedRequest: (id: string | null) => void

  // Crawl actions
  startCrawl: () => Promise<void>
  cancelCrawl: () => Promise<void>
  fetchOpenAPI: () => Promise<void>
  generatePlan: () => Promise<string | null>
  pollProgress: () => Promise<void>
  reset: () => void
}

let pollTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const defaultConfig: CrawlConfig = {
  entry_url: '',
  auth_type: 'none',
  auth: { username: '', password: '', token: '' },
  depth: 3,
  same_origin: true,
  blocklist: '/logout, /delete',
  request_limit: 500,
}

export const useCrawlerStore = create<CrawlerState>((set, get) => ({
  config: { ...defaultConfig },
  crawlId: null,
  status: 'idle',
  progress: { pages_discovered: 0, requests_captured: 0 },
  error: null,
  graph: { nodes: [], edges: [] },
  requests: [],
  selectedRequestId: null,
  openApiUrl: '',
  openApiOps: [],

  setEntryUrl: (url) =>
    set((s) => ({ config: { ...s.config, entry_url: url } })),
  setAuthType: (authType) =>
    set((s) => ({ config: { ...s.config, auth_type: authType } })),
  setAuth: (partial) =>
    set((s) => ({ config: { ...s.config, auth: { ...s.config.auth, ...partial } } })),
  setDepth: (depth) =>
    set((s) => ({ config: { ...s.config, depth } })),
  setSameOrigin: (same_origin) =>
    set((s) => ({ config: { ...s.config, same_origin } })),
  setBlocklist: (blocklist) =>
    set((s) => ({ config: { ...s.config, blocklist } })),
  setRequestLimit: (request_limit) =>
    set((s) => ({ config: { ...s.config, request_limit } })),
  setOpenApiUrl: (url) => set({ openApiUrl: url }),
  setSelectedRequest: (id) => set({ selectedRequestId: id }),

  startCrawl: async () => {
    const { config } = get()
    if (!config.entry_url.trim()) {
      set({ error: 'Entry URL is required' })
      return
    }

    set({
      status: 'crawling',
      error: null,
      progress: { pages_discovered: 0, requests_captured: 0 },
      graph: { nodes: [], edges: [] },
      requests: [],
      selectedRequestId: null,
      openApiOps: [],
    })

    try {
      const res = await fetch('/api/v1/crawls', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          entry_url: config.entry_url,
          auth_type: config.auth_type,
          auth: config.auth_type !== 'none' ? config.auth : undefined,
          depth: config.depth,
          same_origin: config.same_origin,
          blocklist: config.blocklist.split(',').map((s) => s.trim()).filter(Boolean),
          request_limit: config.request_limit,
        }),
      })

      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ status: 'failed', error: `Failed to start crawl: ${res.status} ${text}` })
        return
      }

      const data = await res.json()
      set({ crawlId: data.id })

      // Start polling
      stopPolling()
      pollTimer = setInterval(() => {
        get().pollProgress()
      }, 2000)
    } catch {
      set({
        status: 'failed',
        error: 'Could not reach the backend. Make sure the server is running.',
      })
    }
  },

  cancelCrawl: async () => {
    const { crawlId } = get()
    if (!crawlId) return

    stopPolling()

    try {
      await fetch(`/api/v1/crawls/${crawlId}/cancel`, { method: 'POST' })
    } catch {
      // Best effort
    }

    set({ status: 'idle' })
  },

  fetchOpenAPI: async () => {
    const { openApiUrl } = get()
    if (!openApiUrl.trim()) {
      set({ error: 'OpenAPI URL is required' })
      return
    }

    set({
      status: 'crawling',
      error: null,
      progress: { pages_discovered: 0, requests_captured: 0 },
      graph: { nodes: [], edges: [] },
      requests: [],
      selectedRequestId: null,
      openApiOps: [],
    })

    try {
      const res = await fetch('/api/v1/crawls', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ openapi_url: openApiUrl }),
      })

      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ status: 'failed', error: `Failed to fetch OpenAPI spec: ${res.status} ${text}` })
        return
      }

      const data = await res.json()
      set({
        crawlId: data.id,
        status: 'completed',
        openApiOps: data.operations ?? [],
        progress: {
          pages_discovered: data.operations?.length ?? 0,
          requests_captured: data.operations?.length ?? 0,
        },
      })
    } catch {
      set({
        status: 'failed',
        error: 'Could not reach the backend. Make sure the server is running.',
      })
    }
  },

  pollProgress: async () => {
    const { crawlId, status } = get()
    if (!crawlId || status !== 'crawling') {
      stopPolling()
      return
    }

    try {
      const res = await fetch(`/api/v1/crawls/${crawlId}`)
      if (!res.ok) return

      const data = await res.json()

      set({
        progress: {
          pages_discovered: data.pages_discovered ?? 0,
          requests_captured: data.requests_captured ?? 0,
        },
        graph: data.graph ?? get().graph,
        requests: data.requests ?? get().requests,
      })

      if (data.status === 'completed' || data.status === 'failed') {
        stopPolling()
        set({
          status: data.status,
          graph: data.graph ?? get().graph,
          requests: data.requests ?? get().requests,
          error: data.status === 'failed' ? (data.error ?? 'Crawl failed') : null,
        })
      }
    } catch {
      // Polling failure is transient; keep trying
    }
  },

  generatePlan: async () => {
    const { crawlId } = get()
    if (!crawlId) return null

    try {
      const res = await fetch(`/api/v1/crawls/${crawlId}/generate-plan`, {
        method: 'POST',
      })

      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ error: `Failed to generate plan: ${res.status} ${text}` })
        return null
      }

      const data = await res.json()
      return data.id ?? null
    } catch {
      set({ error: 'Could not reach the backend.' })
      return null
    }
  },

  reset: () => {
    stopPolling()
    set({
      config: { ...defaultConfig },
      crawlId: null,
      status: 'idle',
      progress: { pages_discovered: 0, requests_captured: 0 },
      error: null,
      graph: { nodes: [], edges: [] },
      requests: [],
      selectedRequestId: null,
      openApiUrl: '',
      openApiOps: [],
    })
  },
}))
