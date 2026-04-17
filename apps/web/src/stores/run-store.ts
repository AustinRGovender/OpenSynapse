import { create } from 'zustand'

// --- Types ---

export interface MetricSnapshot {
  timestamp: number
  rps: number
  p95: number
  errorRate: number
  activeVUs: number
}

export interface RunEvent {
  id: string
  timestamp: string
  level: 'info' | 'warn' | 'error'
  message: string
}

export interface RunSummary {
  totalRequests: number
  failed: number
  errorRate: number
  avgRps: number
  p95: number
  peakVUs: number
}

export interface Run {
  id: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  plan_id: string
  started_at: string | null
  ended_at: string | null
  summary: RunSummary | null
  paused?: boolean
}

export interface RunControlPayload {
  vus?: number
  rps?: number
  duration_seconds?: number
  paused?: boolean
}

export interface TimeSeriesPoint {
  time: number
  value: number
}

export interface RunListFilters {
  status?: string
  from?: string
  to?: string
  limit?: number
  cursor?: string
}

export interface RunWithPlanName extends Run {
  plan_name?: string
}

interface RunState {
  run: Run | null
  metrics: {
    rps: TimeSeriesPoint[]
    p95: TimeSeriesPoint[]
    errorRate: TimeSeriesPoint[]
    activeVUs: TimeSeriesPoint[]
  }
  events: RunEvent[]
  wsConnected: boolean
  loading: boolean

  // Runs list state
  runsList: RunWithPlanName[]
  runsListLoading: boolean
  runsListCursor: string | null
  runsListHasMore: boolean

  // Comparison state
  compareRuns: RunWithPlanName[]
  compareLoading: boolean

  // Actions
  loadRun: (id: string) => Promise<void>
  loadRuns: (filters?: RunListFilters) => Promise<void>
  loadMoreRuns: (filters?: RunListFilters) => Promise<void>
  loadMultipleRuns: (ids: string[]) => Promise<void>
  connectWebSocket: (runId: string) => void
  disconnectWebSocket: () => void
  appendMetrics: (snapshot: MetricSnapshot) => void
  addEvent: (event: RunEvent) => void
  controlRun: (runId: string, control: RunControlPayload) => Promise<{ ok: boolean; error?: string }>
  stopRun: (runId: string) => Promise<{ ok: boolean; error?: string }>
  killRun: (runId: string) => Promise<{ ok: boolean; error?: string }>
  reset: () => void
}

let ws: WebSocket | null = null

const initialMetrics = {
  rps: [] as TimeSeriesPoint[],
  p95: [] as TimeSeriesPoint[],
  errorRate: [] as TimeSeriesPoint[],
  activeVUs: [] as TimeSeriesPoint[],
}

// --- Snake-to-camel normalisers at the API boundary ---
//
// The control plane serialises summaries and metric snapshots in snake_case
// (e.g. `total_requests`, `p95_ms`, `timestamp_ms`). The TS interfaces above
// are camelCase. Without translation, every camelCase field resolves to
// `undefined`, and `p95.toFixed(1)` / `errorRate.toFixed(2)` in RunView
// throws as soon as the first WS tick arrives — React unmounts the tree
// and the run screen goes blank. Normalising at the store boundary keeps
// the rest of the app working against a single idiomatic shape.

type ApiRunSummary = Partial<{
  total_requests: number
  failed_requests: number
  error_rate: number
  throughput_rps: number
  p95_ms: number
  max_ms: number
  // camelCase variants are passed through unchanged when already present
  totalRequests: number
  failed: number
  errorRate: number
  avgRps: number
  p95: number
  peakVUs: number
}>

function normaliseSummary(raw: ApiRunSummary | null | undefined): RunSummary | null {
  if (!raw) return null
  return {
    totalRequests: raw.totalRequests ?? raw.total_requests ?? 0,
    failed: raw.failed ?? raw.failed_requests ?? 0,
    errorRate: raw.errorRate ?? raw.error_rate ?? 0,
    avgRps: raw.avgRps ?? raw.throughput_rps ?? 0,
    p95: raw.p95 ?? raw.p95_ms ?? 0,
    // The server does not currently expose peak VUs; fall back to 0 so the
    // UI never renders `undefined.toFixed(...)`.
    peakVUs: raw.peakVUs ?? 0,
  }
}

function normaliseRun(raw: Record<string, unknown>): Run {
  const summary = raw['summary'] as ApiRunSummary | null | undefined
  return { ...(raw as unknown as Run), summary: normaliseSummary(summary) }
}

type ApiMetricSnapshot = Partial<{
  timestamp: number
  timestamp_ms: number
  rps: number
  p95: number
  p95_ms: number
  errorRate: number
  error_rate: number
  activeVUs: number
  active_vus: number
}>

function normaliseSnapshot(raw: ApiMetricSnapshot): MetricSnapshot {
  return {
    timestamp: raw.timestamp ?? raw.timestamp_ms ?? 0,
    rps: raw.rps ?? 0,
    p95: raw.p95 ?? raw.p95_ms ?? 0,
    errorRate: raw.errorRate ?? raw.error_rate ?? 0,
    activeVUs: raw.activeVUs ?? raw.active_vus ?? 0,
  }
}

export const useRunStore = create<RunState>((set, get) => ({
  run: null,
  metrics: { ...initialMetrics },
  events: [],
  wsConnected: false,
  loading: false,
  runsList: [],
  runsListLoading: false,
  runsListCursor: null,
  runsListHasMore: false,
  compareRuns: [],
  compareLoading: false,

  loadRun: async (id: string) => {
    set({ loading: true })
    try {
      const res = await fetch(`/api/v1/runs/${id}`)
      if (!res.ok) {
        set({ loading: false })
        return
      }
      const run = normaliseRun(await res.json())
      set({ run, loading: false })
    } catch {
      set({ loading: false })
    }
  },

  loadRuns: async (filters?: RunListFilters) => {
    set({ runsListLoading: true })
    try {
      const params = new URLSearchParams()
      if (filters?.status && filters.status !== 'all') params.set('status', filters.status)
      if (filters?.from) params.set('from', filters.from)
      if (filters?.to) params.set('to', filters.to)
      params.set('limit', String(filters?.limit ?? 20))
      const qs = params.toString()
      const res = await fetch(`/api/v1/runs${qs ? '?' + qs : ''}`)
      if (res.ok) {
        const data = await res.json()
        const rawItems: Record<string, unknown>[] = data.items ?? []
        const items: RunWithPlanName[] = rawItems.map(
          (raw) => normaliseRun(raw) as RunWithPlanName,
        )
        set({
          runsList: items,
          runsListLoading: false,
          runsListCursor: data.next_cursor ?? null,
          runsListHasMore: !!data.next_cursor,
        })
      } else {
        set({ runsListLoading: false })
      }
    } catch {
      set({ runsListLoading: false })
    }
  },

  loadMoreRuns: async (filters?: RunListFilters) => {
    const cursor = get().runsListCursor
    if (!cursor) return
    set({ runsListLoading: true })
    try {
      const params = new URLSearchParams()
      if (filters?.status && filters.status !== 'all') params.set('status', filters.status)
      if (filters?.from) params.set('from', filters.from)
      if (filters?.to) params.set('to', filters.to)
      params.set('limit', String(filters?.limit ?? 20))
      params.set('cursor', cursor)
      const qs = params.toString()
      const res = await fetch(`/api/v1/runs${qs ? '?' + qs : ''}`)
      if (res.ok) {
        const data = await res.json()
        const rawItems: Record<string, unknown>[] = data.items ?? []
        const items: RunWithPlanName[] = rawItems.map(
          (raw) => normaliseRun(raw) as RunWithPlanName,
        )
        set({
          runsList: [...get().runsList, ...items],
          runsListLoading: false,
          runsListCursor: data.next_cursor ?? null,
          runsListHasMore: !!data.next_cursor,
        })
      } else {
        set({ runsListLoading: false })
      }
    } catch {
      set({ runsListLoading: false })
    }
  },

  loadMultipleRuns: async (ids: string[]) => {
    set({ compareLoading: true, compareRuns: [] })
    try {
      const results = await Promise.all(
        ids.map(async (id) => {
          const res = await fetch(`/api/v1/runs/${id}`)
          if (!res.ok) return null
          return normaliseRun(await res.json()) as RunWithPlanName
        }),
      )
      set({
        compareRuns: results.filter((r): r is RunWithPlanName => r !== null),
        compareLoading: false,
      })
    } catch {
      set({ compareLoading: false })
    }
  },

  connectWebSocket: (runId: string) => {
    get().disconnectWebSocket()

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    const url = `${protocol}//${host}/api/v1/ws`

    try {
      ws = new WebSocket(url)

      ws.onopen = () => {
        set({ wsConnected: true })
        ws?.send(
          JSON.stringify({
            type: 'subscribe',
            channel: `runs.${runId}.metrics`,
          }),
        )
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'event' && msg.channel === `runs.${runId}.metrics`) {
            // Server payload is MetricSnapshot with snake_case JSON tags
            // (`timestamp_ms`, `p95_ms`, `error_rate`, `active_vus`).
            // Normalise before appending so `.toFixed(...)` downstream never
            // sees `undefined` from a raw snake_case field.
            get().appendMetrics(normaliseSnapshot(msg.payload as ApiMetricSnapshot))
          }
          if (msg.type === 'event' && msg.channel === `runs.${runId}.events`) {
            get().addEvent(msg.payload as RunEvent)
          }
          if (msg.type === 'event' && msg.channel === `runs.${runId}.status`) {
            const run = get().run
            if (run) {
              const updated = { ...run, ...msg.payload }
              // Freeze ended_at on terminal states so duration calculation stops
              const terminalStates = ['completed', 'failed', 'cancelled']
              if (terminalStates.includes(updated.status) && !updated.ended_at) {
                updated.ended_at = new Date().toISOString()
              }
              set({ run: updated })
              // Disconnect WebSocket after short delay to allow final metrics to arrive
              if (terminalStates.includes(updated.status)) {
                setTimeout(() => get().disconnectWebSocket(), 2000)
              }
            }
          }
        } catch {
          // Ignore malformed messages
        }
      }

      ws.onclose = () => {
        set({ wsConnected: false })
      }

      ws.onerror = () => {
        set({ wsConnected: false })
      }
    } catch {
      set({ wsConnected: false })
    }
  },

  disconnectWebSocket: () => {
    if (ws) {
      ws.close()
      ws = null
    }
    set({ wsConnected: false })
  },

  appendMetrics: (snapshot: MetricSnapshot) => {
    const { metrics } = get()
    const time = snapshot.timestamp
    set({
      metrics: {
        rps: [...metrics.rps, { time, value: snapshot.rps }],
        p95: [...metrics.p95, { time, value: snapshot.p95 }],
        errorRate: [...metrics.errorRate, { time, value: snapshot.errorRate }],
        activeVUs: [...metrics.activeVUs, { time, value: snapshot.activeVUs }],
      },
    })
  },

  addEvent: (event: RunEvent) => {
    set({ events: [...get().events, event] })
  },

  controlRun: async (runId: string, control: RunControlPayload) => {
    try {
      const res = await fetch(`/api/v1/runs/${runId}/control`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(control),
      })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        return { ok: false, error: `${res.status}: ${text}` }
      }
      // Update paused state locally when toggling pause
      if (control.paused !== undefined) {
        const run = get().run
        if (run) {
          set({ run: { ...run, paused: control.paused } })
        }
      }
      return { ok: true }
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : 'Network error' }
    }
  },

  stopRun: async (runId: string) => {
    try {
      const res = await fetch(`/api/v1/runs/${runId}/stop`, { method: 'POST' })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        return { ok: false, error: `${res.status}: ${text}` }
      }
      const run = get().run
      if (run) {
        set({ run: { ...run, status: 'cancelled' } })
      }
      return { ok: true }
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : 'Network error' }
    }
  },

  killRun: async (runId: string) => {
    try {
      const res = await fetch(`/api/v1/runs/${runId}/kill`, { method: 'POST' })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        return { ok: false, error: `${res.status}: ${text}` }
      }
      const run = get().run
      if (run) {
        set({ run: { ...run, status: 'cancelled' } })
      }
      return { ok: true }
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : 'Network error' }
    }
  },

  reset: () => {
    get().disconnectWebSocket()
    set({
      run: null,
      metrics: {
        rps: [],
        p95: [],
        errorRate: [],
        activeVUs: [],
      },
      events: [],
      wsConnected: false,
      loading: false,
    })
  },
}))
