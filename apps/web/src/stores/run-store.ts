import { create } from 'zustand'
import type { LiveState, MetricPoint, Run, RunError, RunEvent } from '@opensynapse/api-client'
import { api, unwrap, wsUrl } from '../api'
import { useAuthStore } from './auth-store'

export const TERMINAL = ['completed', 'failed', 'aborted']

export type ControlPayload = {
  vus?: number
  rps?: number
  duration_seconds?: number
  paused?: boolean
  resume_schedule?: boolean
}

interface RunState {
  runId: string | null
  run: Run | null
  series: MetricPoint[]
  events: RunEvent[]
  errors: RunError[]
  live: LiveState | null
  wsConnected: boolean
  loading: boolean
  notFound: boolean

  open: (id: string) => Promise<void>
  close: () => void
  control: (payload: ControlPayload) => Promise<LiveState>
  stop: () => Promise<void>
  kill: () => Promise<void>
  addNote: (message: string) => Promise<void>
}

let socket: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

function mergePoints(existing: MetricPoint[], incoming: MetricPoint[]): MetricPoint[] {
  if (incoming.length === 0) return existing
  const last = existing.length ? existing[existing.length - 1].timestamp_ms : -1
  const fresh = incoming.filter((p) => p.timestamp_ms > last)
  return fresh.length ? [...existing, ...fresh] : existing
}

export const useRunStore = create<RunState>((set, get) => {
  function disconnect() {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = null
    if (socket) {
      socket.onclose = null
      socket.close()
      socket = null
    }
    set({ wsConnected: false })
  }

  function connect(id: string) {
    disconnect()
    const ws = new WebSocket(wsUrl())
    socket = ws
    ws.onopen = () => {
      ws.send(JSON.stringify({ type: 'auth', payload: { token: useAuthStore.getState().token ?? '' } }))
      for (const ch of ['metrics', 'events', 'status', 'errors']) {
        ws.send(JSON.stringify({ type: 'subscribe', channel: `runs.${id}.${ch}` }))
      }
      set({ wsConnected: true })
    }
    ws.onmessage = (ev) => {
      if (get().runId !== id) return
      let msg: { type: string; channel: string; payload: unknown }
      try {
        msg = JSON.parse(ev.data)
      } catch {
        return
      }
      const channel = msg.channel.slice(`runs.${id}.`.length)
      if (channel === 'metrics') {
        const p = msg.payload as { points: MetricPoint[]; state: LiveState }
        set({ series: mergePoints(get().series, p.points ?? []), live: p.state })
      } else if (channel === 'events') {
        const e = msg.payload as RunEvent
        if (!get().events.some((x) => x.id === e.id)) set({ events: [...get().events, e] })
      } else if (channel === 'errors') {
        const p = msg.payload as { errors: RunError[] }
        set({ errors: [...get().errors, ...(p.errors ?? [])].slice(-200) })
      } else if (channel === 'status') {
        const p = msg.payload as Pick<Run, 'status' | 'started_at' | 'ended_at' | 'summary'>
        const run = get().run
        if (run) set({ run: { ...run, ...p } })
        if (TERMINAL.includes(p.status)) {
          set({ live: null })
          // Pull the final, complete series once the engine has flushed it.
          setTimeout(() => refresh(id), 800)
          disconnect()
        }
      }
    }
    ws.onclose = () => {
      set({ wsConnected: false })
      const run = get().run
      if (get().runId === id && run && !TERMINAL.includes(run.status)) {
        reconnectTimer = setTimeout(() => connect(id), 2000)
      }
    }
  }

  async function refresh(id: string) {
    const [run, samples, events, errors] = await Promise.all([
      unwrap(api.GET('/api/v1/runs/{id}', { params: { path: { id } } })),
      unwrap(api.GET('/api/v1/runs/{id}/samples', { params: { path: { id } } })),
      unwrap(api.GET('/api/v1/runs/{id}/events', { params: { path: { id } } })),
      unwrap(api.GET('/api/v1/runs/{id}/errors', { params: { path: { id }, query: { limit: '200' } } })),
    ])
    if (get().runId !== id) return
    set({ run, series: samples.items, events: events.items, errors: errors.items })
  }

  return {
    runId: null,
    run: null,
    series: [],
    events: [],
    errors: [],
    live: null,
    wsConnected: false,
    loading: false,
    notFound: false,

    open: async (id) => {
      disconnect()
      set({ runId: id, run: null, series: [], events: [], errors: [], live: null, loading: true, notFound: false })
      try {
        await refresh(id)
        set({ loading: false })
        const run = get().run
        if (run && !TERMINAL.includes(run.status)) {
          connect(id)
          try {
            set({ live: await unwrap(api.GET('/api/v1/runs/{id}/live', { params: { path: { id } } })) })
          } catch {
            // The run may have just finished.
          }
        }
      } catch {
        if (get().runId === id) set({ loading: false, notFound: true })
      }
    },

    close: () => {
      disconnect()
      set({ runId: null, run: null, series: [], events: [], errors: [], live: null })
    },

    control: async (payload) => {
      const id = get().runId
      if (!id) throw new Error('No run open')
      const live = await unwrap(api.PATCH('/api/v1/runs/{id}/control', { params: { path: { id } }, body: payload }))
      set({ live })
      return live
    },

    stop: async () => {
      const id = get().runId
      if (id) await unwrap(api.POST('/api/v1/runs/{id}/stop', { params: { path: { id } } }))
    },

    kill: async () => {
      const id = get().runId
      if (id) await unwrap(api.POST('/api/v1/runs/{id}/kill', { params: { path: { id } } }))
    },

    addNote: async (message) => {
      const id = get().runId
      if (!id) return
      const ev = await unwrap(api.POST('/api/v1/runs/{id}/events', { params: { path: { id } }, body: { type: 'user_note', payload: { message } } }))
      if (!get().events.some((x) => x.id === ev.id)) set({ events: [...get().events, ev] })
    },
  }
})
