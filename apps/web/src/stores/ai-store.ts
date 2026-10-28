import { create } from 'zustand'
import type { AIAnalysis, AIConfig } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'

export type AIProvider = '' | 'openai' | 'anthropic' | 'azure_openai' | 'nvidia_nim'

/** Sensible defaults per provider; any model name can be typed instead. */
export const MODEL_OPTIONS: Record<string, string[]> = {
  openai: ['gpt-4.1', 'gpt-4.1-mini', 'gpt-4o'],
  anthropic: ['claude-opus-5-5', 'claude-sonnet-5-5', 'claude-haiku-5-5'],
  azure_openai: [],
  nvidia_nim: ['z-ai/glm-5.3-flash', 'nvidia/nemotron-3-super-120b-a12b', 'deepseek-ai/deepseek-v4.1-flash', 'openai/gpt-oss-20b'],
}

export const PROVIDER_LABELS: Record<string, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  azure_openai: 'Azure OpenAI',
  nvidia_nim: 'NVIDIA NIM',
}

/** The standard questions from F-09. */
export const STANDARD_QUESTIONS = {
  run: 'What does this run tell me?',
  compare: 'What changed between these runs?',
  next: 'What test should I run next?',
}

interface AIState {
  config: AIConfig | null
  /** Analyses keyed by the comma-joined run IDs they describe. */
  analyses: Record<string, AIAnalysis[]>
  loadConfig: () => Promise<void>
  saveConfig: (c: { provider: string; api_key: string; model: string; monthly_cap_usd: number | null; enabled: boolean; azure_endpoint: string }) => Promise<void>
  testKey: () => Promise<{ valid: boolean; message: string }>
  preview: (runIds: string[], question: string) => Promise<string>
  analyse: (runIds: string[], question: string) => Promise<AIAnalysis>
}

function target(runIds: string[]) {
  return runIds.length === 1 ? { run_id: runIds[0] } : { run_ids: runIds }
}

export const useAIStore = create<AIState>((set, get) => ({
  config: null,
  analyses: {},

  loadConfig: async () => {
    try {
      set({ config: await unwrap(api.GET('/api/v1/ai/config')) })
    } catch {
      set({ config: null })
    }
  },

  saveConfig: async (c) => {
    set({ config: await unwrap(api.PUT('/api/v1/ai/config', { body: c })) })
  },

  testKey: async () => {
    try {
      return await unwrap(api.POST('/api/v1/ai/config/test'))
    } catch (e) {
      return { valid: false, message: errorMessage(e) }
    }
  },

  preview: async (runIds, question) => {
    const res = await unwrap(api.POST('/api/v1/ai/preview', { body: { ...target(runIds), question } }))
    return res.prompt
  },

  analyse: async (runIds, question) => {
    const res = await unwrap(api.POST('/api/v1/ai/analyse', { body: { ...target(runIds), question } }))
    const key = runIds.join(',')
    set({ analyses: { ...get().analyses, [key]: [res, ...(get().analyses[key] ?? []).filter((a) => a.question !== res.question)] } })
    return res
  },
}))
