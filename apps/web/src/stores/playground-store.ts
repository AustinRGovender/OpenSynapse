import { create } from 'zustand'
import type { Collection, MicroTestResult, PlaygroundRequest, PlaygroundResponse, SavedRequest } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS'
export type BodyType = 'none' | 'json' | 'raw' | 'form'
export type AuthType = 'none' | 'basic' | 'bearer' | 'api_key' | 'digest' | 'oauth2' | 'inherit'

export interface KeyValueRow {
  key: string
  value: string
  enabled: boolean
}

/** Fields for every auth method; only those of the active method are sent. */
export interface AuthConfig {
  username: string
  password: string
  token: string
  key: string
  value: string
  add_to: 'header' | 'query'
  grant_type: 'authorization_code' | 'client_credentials'
  auth_url: string
  token_url: string
  client_id: string
  client_secret: string
  scope: string
  access_token: string
}

export const emptyAuth: AuthConfig = {
  username: '',
  password: '',
  token: '',
  key: 'X-API-Key',
  value: '',
  add_to: 'header',
  grant_type: 'authorization_code',
  auth_url: '',
  token_url: '',
  client_id: '',
  client_secret: '',
  scope: '',
  access_token: '',
}

interface State {
  method: HttpMethod
  url: string
  params: KeyValueRow[]
  headers: KeyValueRow[]
  body: string
  bodyType: BodyType
  authType: AuthType
  auth: AuthConfig
  environmentId: string

  response: PlaygroundResponse | null
  sending: boolean
  error: string | null

  micro: MicroTestResult | null
  microRunning: boolean

  collections: Collection[]

  set: (patch: Partial<State>) => void
  setAuth: (patch: Partial<AuthConfig>) => void
  buildRequest: () => PlaygroundRequest
  send: () => Promise<void>
  microTest: (vus: number, seconds: number) => Promise<void>
  loadCollections: () => Promise<void>
  saveToCollection: (collectionName: string, requestName: string) => Promise<void>
  load: (r: SavedRequest) => void
  removeFromCollection: (collection: Collection, index: number) => Promise<void>
}

const blankRow = (): KeyValueRow => ({ key: '', value: '', enabled: true })

function withParams(url: string, params: KeyValueRow[]): string {
  const active = params.filter((p) => p.enabled && p.key.trim())
  if (!active.length) return url
  const qs = active.map((p) => `${encodeURIComponent(p.key)}=${encodeURIComponent(p.value)}`).join('&')
  return url + (url.includes('?') ? '&' : '?') + qs
}

function authFields(type: AuthType, a: AuthConfig): Record<string, string> {
  switch (type) {
    case 'basic':
    case 'digest':
      return { username: a.username, password: a.password }
    case 'bearer':
      return { token: a.token }
    case 'api_key':
      return { key: a.key, value: a.value, add_to: a.add_to }
    case 'oauth2':
      return { grant_type: a.grant_type, access_token: a.access_token, token_url: a.token_url, client_id: a.client_id, client_secret: a.client_secret, scope: a.scope }
    default:
      return {}
  }
}

export const usePlaygroundStore = create<State>((set, get) => ({
  method: 'GET',
  url: '',
  params: [blankRow()],
  headers: [blankRow()],
  body: '',
  bodyType: 'none',
  authType: 'none',
  auth: { ...emptyAuth },
  environmentId: '',
  response: null,
  sending: false,
  error: null,
  micro: null,
  microRunning: false,
  collections: [],

  set: (patch) => set(patch),
  setAuth: (patch) => set({ auth: { ...get().auth, ...patch } }),

  buildRequest: () => {
    const s = get()
    const headers: Record<string, string> = {}
    for (const h of s.headers) if (h.enabled && h.key.trim()) headers[h.key.trim()] = h.value
    const hasBody = s.bodyType !== 'none' && s.method !== 'GET' && s.method !== 'HEAD'
    return {
      method: s.method,
      url: withParams(s.url.trim(), s.params),
      headers,
      body: hasBody ? s.body : '',
      body_type: hasBody ? s.bodyType : 'none',
      auth_type: s.authType,
      auth_config: authFields(s.authType, s.auth),
      environment_id: s.environmentId || undefined,
    }
  },

  send: async () => {
    if (!get().url.trim()) {
      set({ error: 'Enter a URL first' })
      return
    }
    set({ sending: true, error: null })
    try {
      set({ response: await unwrap(api.POST('/api/v1/playground/request', { body: get().buildRequest() })), sending: false })
    } catch (e) {
      set({ sending: false, error: errorMessage(e), response: null })
    }
  },

  microTest: async (vus, seconds) => {
    set({ microRunning: true, error: null, micro: null })
    try {
      set({ micro: await unwrap(api.POST('/api/v1/playground/microtest', { body: { request: get().buildRequest(), vus, duration_seconds: seconds } })), microRunning: false })
    } catch (e) {
      set({ microRunning: false, error: errorMessage(e) })
    }
  },

  loadCollections: async () => {
    try {
      set({ collections: (await unwrap(api.GET('/api/v1/playground/collections'))).items })
    } catch {
      // Collections are optional for using the playground.
    }
  },

  saveToCollection: async (collectionName, requestName) => {
    const r = get().buildRequest()
    const saved: SavedRequest = {
      name: requestName,
      method: r.method,
      url: get().url,
      headers: r.headers ?? {},
      body: r.body ?? '',
      body_type: r.body_type ?? 'none',
      auth_type: r.auth_type ?? 'none',
      auth_config: r.auth_config,
    }
    const existing = get().collections.find((c) => c.name === collectionName)
    if (existing) {
      await unwrap(api.PUT('/api/v1/playground/collections/{id}', { params: { path: { id: existing.id } }, body: { name: existing.name, requests: [...existing.requests, saved] } }))
    } else {
      await unwrap(api.POST('/api/v1/playground/collections', { body: { name: collectionName, requests: [saved] } }))
    }
    await get().loadCollections()
  },

  load: (r) => {
    const cfg = (r.auth_config ?? {}) as Partial<AuthConfig>
    set({
      method: (r.method as HttpMethod) || 'GET',
      url: r.url,
      params: [blankRow()],
      headers: [...Object.entries(r.headers ?? {}).map(([key, value]) => ({ key, value, enabled: true })), blankRow()],
      body: r.body,
      bodyType: (r.body_type as BodyType) || 'none',
      authType: (r.auth_type as AuthType) || 'none',
      auth: { ...emptyAuth, ...cfg },
      response: null,
      error: null,
    })
  },

  removeFromCollection: async (collection, index) => {
    const requests = collection.requests.filter((_, i) => i !== index)
    if (requests.length === 0) await unwrap(api.DELETE('/api/v1/playground/collections/{id}', { params: { path: { id: collection.id } } }))
    else await unwrap(api.PUT('/api/v1/playground/collections/{id}', { params: { path: { id: collection.id } }, body: { name: collection.name, requests } }))
    await get().loadCollections()
  },
}))
