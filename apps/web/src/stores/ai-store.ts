import { create } from 'zustand'

// --- Types ---

export type AIProvider = 'none' | 'openai' | 'anthropic' | 'azure'

export interface AIConfig {
  provider: AIProvider
  model: string
  enabled: boolean
  monthly_cap: number | null
}

export interface AIAnalysis {
  response: string
  tokens: number
  cost: number
  question: string
}

interface AIState {
  config: AIConfig
  apiKey: string
  keyValidated: boolean | null
  analysis: AIAnalysis | null
  loading: boolean
  testLoading: boolean
  saveLoading: boolean
  error: string | null

  // Config actions
  setProvider: (provider: AIProvider) => void
  setModel: (model: string) => void
  setEnabled: (enabled: boolean) => void
  setMonthlyCap: (cap: number | null) => void
  setApiKey: (key: string) => void

  // API actions
  loadConfig: () => Promise<void>
  saveConfig: () => Promise<void>
  testKey: () => Promise<void>
  analyse: (runId: string, question?: string) => Promise<void>
  clearAnalysis: () => void
  clearError: () => void
}

const defaultConfig: AIConfig = {
  provider: 'none',
  model: '',
  enabled: false,
  monthly_cap: null,
}

export const MODEL_OPTIONS: Record<string, string[]> = {
  openai: ['gpt-4o', 'gpt-4-turbo', 'gpt-3.5-turbo'],
  anthropic: ['claude-sonnet-4-20250514', 'claude-haiku-4-20250414'],
}

export function maskApiKey(key: string): string {
  if (key.length <= 4) return key
  return '*'.repeat(key.length - 4) + key.slice(-4)
}

export const useAIStore = create<AIState>((set, get) => ({
  config: { ...defaultConfig },
  apiKey: '',
  keyValidated: null,
  analysis: null,
  loading: false,
  testLoading: false,
  saveLoading: false,
  error: null,

  setProvider: (provider) => {
    const models = MODEL_OPTIONS[provider]
    const model = models ? models[0] : ''
    set((s) => ({ config: { ...s.config, provider, model } }))
  },
  setModel: (model) => set((s) => ({ config: { ...s.config, model } })),
  setEnabled: (enabled) => set((s) => ({ config: { ...s.config, enabled } })),
  setMonthlyCap: (monthly_cap) => set((s) => ({ config: { ...s.config, monthly_cap } })),
  setApiKey: (key) => set({ apiKey: key, keyValidated: null }),

  loadConfig: async () => {
    try {
      const res = await fetch('/api/v1/ai/config')
      if (!res.ok) return
      const data = await res.json()
      set({
        config: {
          provider: data.provider ?? 'none',
          model: data.model ?? '',
          enabled: data.enabled ?? false,
          monthly_cap: data.monthly_cap ?? null,
        },
        keyValidated: data.key_configured ?? null,
      })
    } catch {
      // API may not be available yet
    }
  },

  saveConfig: async () => {
    const { config, apiKey } = get()
    set({ saveLoading: true, error: null })
    try {
      const body: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        enabled: config.enabled,
        monthly_cap: config.monthly_cap,
      }
      if (apiKey) {
        body.api_key = apiKey
      }
      const res = await fetch('/api/v1/ai/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ saveLoading: false, error: `Failed to save: ${res.status} ${text}` })
        return
      }
      set({ saveLoading: false, apiKey: '' })
    } catch {
      set({ saveLoading: false, error: 'Could not reach the backend.' })
    }
  },

  testKey: async () => {
    const { config, apiKey } = get()
    set({ testLoading: true, error: null, keyValidated: null })
    try {
      const body: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
      }
      if (apiKey) {
        body.api_key = apiKey
      }
      const res = await fetch('/api/v1/ai/config/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        set({ testLoading: false, keyValidated: false })
        return
      }
      const data = await res.json()
      set({ testLoading: false, keyValidated: data.valid === true })
    } catch {
      set({ testLoading: false, keyValidated: false })
    }
  },

  analyse: async (runId, question) => {
    set({ loading: true, error: null })
    try {
      const res = await fetch('/api/v1/ai/analyse', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          run_id: runId,
          question: question ?? undefined,
        }),
      })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ loading: false, error: `Analysis failed: ${res.status} ${text}` })
        return
      }
      const data = await res.json()
      set({
        loading: false,
        analysis: {
          response: data.response ?? '',
          tokens: data.tokens ?? 0,
          cost: data.cost ?? 0,
          question: question ?? 'What does this run tell me?',
        },
      })
    } catch {
      set({ loading: false, error: 'Could not reach the backend.' })
    }
  },

  clearAnalysis: () => set({ analysis: null }),
  clearError: () => set({ error: null }),
}))
