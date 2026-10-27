import { create } from 'zustand'
import type { Principal } from '@opensynapse/api-client'

// Team-mode session tokens survive reloads; the local-mode install token is
// fetched fresh on every load and never stored.
const SESSION_KEY = 'opensynapse.session'

function readStored(): string | null {
  try {
    return localStorage.getItem(SESSION_KEY)
  } catch {
    return null
  }
}

function writeStored(token: string | null) {
  try {
    if (token) localStorage.setItem(SESSION_KEY, token)
    else localStorage.removeItem(SESSION_KEY)
  } catch {
    // storage unavailable (private mode): the session lasts for this tab
  }
}

type Status = 'loading' | 'ready' | 'login' | 'setup' | 'error'

interface AuthState {
  status: Status
  mode: 'local' | 'team' | null
  token: string | null
  user: Principal | null
  error: string | null
  bootstrap: () => Promise<void>
  login: (email: string, password: string) => Promise<void>
  setup: (email: string, password: string, displayName: string) => Promise<void>
  logout: () => Promise<void>
  handleUnauthorized: () => void
}

async function postJSON(path: string, body: unknown, token?: string | null) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(path, { method: 'POST', headers, body: JSON.stringify(body) })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data?.error?.message ?? `Request failed (${res.status})`)
  return data
}

export const useAuthStore = create<AuthState>((set, get) => ({
  status: 'loading',
  mode: null,
  token: readStored(),
  user: null,
  error: null,

  bootstrap: async () => {
    set({ status: 'loading', error: null })
    const stored = readStored()
    try {
      const res = await fetch('/api/v1/auth/session', {
        headers: stored ? { Authorization: `Bearer ${stored}` } : {},
      })
      if (!res.ok) throw new Error(`The control plane answered ${res.status}`)
      const s = await res.json()
      if (s.mode === 'local') {
        set({ status: 'ready', mode: 'local', token: s.token, user: s.user })
        return
      }
      if (s.authenticated) {
        set({ status: 'ready', mode: 'team', token: stored, user: s.user })
      } else {
        writeStored(null)
        set({ status: s.setup_required ? 'setup' : 'login', mode: 'team', token: null, user: null })
      }
    } catch (e) {
      set({
        status: 'error',
        error:
          e instanceof Error && e.message.startsWith('The control plane')
            ? e.message
            : 'Cannot reach the OpenSynapse control plane. Make sure it is running.',
      })
    }
  },

  login: async (email, password) => {
    const data = await postJSON('/api/v1/auth/login', { email, password })
    writeStored(data.token)
    set({ status: 'ready', mode: 'team', token: data.token, user: data.user, error: null })
  },

  setup: async (email, password, displayName) => {
    const data = await postJSON('/api/v1/auth/setup', { email, password, display_name: displayName })
    writeStored(data.token)
    set({ status: 'ready', mode: 'team', token: data.token, user: data.user, error: null })
  },

  logout: async () => {
    const { token, mode } = get()
    if (mode === 'team' && token) {
      await fetch('/api/v1/auth/logout', { method: 'POST', headers: { Authorization: `Bearer ${token}` } }).catch(() => {})
    }
    writeStored(null)
    set({ status: mode === 'team' ? 'login' : 'loading', token: null, user: null })
    if (mode !== 'team') get().bootstrap()
  },

  handleUnauthorized: () => {
    const { mode, status } = get()
    if (status !== 'ready') return
    if (mode === 'team') {
      writeStored(null)
      set({ status: 'login', token: null, user: null, error: 'Your session ended. Please sign in again.' })
    } else {
      get().bootstrap()
    }
  },
}))

/** True when the signed-in user may change things (editor or admin). */
export function canEdit(user: Principal | null): boolean {
  return user?.role === 'admin' || user?.role === 'editor'
}

export function isAdmin(user: Principal | null): boolean {
  return user?.role === 'admin'
}
