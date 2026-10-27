import { create } from 'zustand'
import type { Settings, SystemInfo } from '@opensynapse/api-client'
import type { ChartPaletteName } from '@opensynapse/ui'
import { api, errorMessage, unwrap } from '../api'

export type ToastKind = 'info' | 'success' | 'error'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

type Theme = 'system' | 'light' | 'dark'

interface UIState {
  toasts: Toast[]
  toast: (message: string, kind?: ToastKind) => void
  dismiss: (id: number) => void

  theme: Theme
  density: 'comfortable' | 'compact'
  palette: ChartPaletteName
  settings: Settings | null
  system: SystemInfo | null

  loadSettings: () => Promise<void>
  saveSettings: (s: Settings) => Promise<Settings>
  loadSystem: () => Promise<void>
}

let nextToast = 1

/** Applies appearance settings to <html> (Tailwind styles key off these attributes). */
export function applyAppearance(theme: Theme, density: string) {
  const root = document.documentElement
  const resolved =
    theme === 'system' ? (window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark') : theme
  root.dataset.theme = resolved
  root.dataset.density = density
  root.style.colorScheme = resolved
}

export const useUIStore = create<UIState>((set, get) => ({
  toasts: [],
  toast: (message, kind = 'info') => {
    const id = nextToast++
    set({ toasts: [...get().toasts, { id, kind, message }] })
    setTimeout(() => get().dismiss(id), kind === 'error' ? 8000 : 4000)
  },
  dismiss: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),

  theme: 'dark',
  density: 'comfortable',
  palette: 'default',
  settings: null,
  system: null,

  loadSettings: async () => {
    try {
      const s = await unwrap(api.GET('/api/v1/settings'))
      const a = s.appearance
      set({
        settings: s,
        theme: (a.theme as Theme) || 'dark',
        density: a.density === 'compact' ? 'compact' : 'comfortable',
        palette: a.palette === 'high-contrast' ? 'high-contrast' : 'default',
      })
      applyAppearance(get().theme, get().density)
    } catch {
      // Settings are optional for rendering; keep defaults.
    }
  },

  saveSettings: async (s) => {
    const saved = await unwrap(api.PUT('/api/v1/settings', { body: s }))
    const a = saved.appearance
    set({
      settings: saved,
      theme: (a.theme as Theme) || 'dark',
      density: a.density === 'compact' ? 'compact' : 'comfortable',
      palette: a.palette === 'high-contrast' ? 'high-contrast' : 'default',
    })
    applyAppearance(get().theme, get().density)
    return saved
  },

  loadSystem: async () => {
    try {
      set({ system: await unwrap(api.GET('/api/v1/system/info')) })
    } catch (e) {
      get().toast(`Could not read system info: ${errorMessage(e)}`, 'error')
    }
  },
}))

/** Shorthand for components: useToast()('Saved', 'success'). */
export function useToast() {
  return useUIStore((s) => s.toast)
}
