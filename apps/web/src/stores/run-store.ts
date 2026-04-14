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
}

export interface TimeSeriesPoint {
  time: number
  value: number
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

  // Actions
  loadRun: (id: string) => Promise<void>
  connectWebSocket: (runId: string) => void
  disconnectWebSocket: () => void
  appendMetrics: (snapshot: MetricSnapshot) => void
  addEvent: (event: RunEvent) => void
  reset: () => void
}

let ws: WebSocket | null = null

const initialMetrics = {
  rps: [] as TimeSeriesPoint[],
  p95: [] as TimeSeriesPoint[],
  errorRate: [] as TimeSeriesPoint[],
  activeVUs: [] as TimeSeriesPoint[],
}

export const useRunStore = create<RunState>((set, get) => ({
  run: null,
  metrics: { ...initialMetrics },
  events: [],
  wsConnected: false,
  loading: false,

  loadRun: async (id: string) => {
    set({ loading: true })
    try {
      const res = await fetch(`/api/v1/runs/${id}`)
      if (!res.ok) {
        set({ loading: false })
        return
      }
      const run: Run = await res.json()
      set({ run, loading: false })
    } catch {
      set({ loading: false })
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
            const payload = msg.payload as MetricSnapshot
            get().appendMetrics(payload)
          }
          if (msg.type === 'event' && msg.channel === `runs.${runId}.events`) {
            get().addEvent(msg.payload as RunEvent)
          }
          if (msg.type === 'event' && msg.channel === `runs.${runId}.status`) {
            const run = get().run
            if (run) {
              set({ run: { ...run, ...msg.payload } })
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
