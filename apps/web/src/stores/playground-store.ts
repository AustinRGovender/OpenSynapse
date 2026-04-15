import { create } from 'zustand'

// --- Types ---

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS'

export interface KeyValueRow {
  key: string
  value: string
  enabled: boolean
}

export type BodyType = 'none' | 'raw' | 'json' | 'form'

export type AuthType = 'none' | 'basic' | 'bearer' | 'apikey'

export interface AuthConfig {
  username: string
  password: string
  token: string
  apiKeyName: string
  apiKeyValue: string
  apiKeyIn: 'header' | 'query'
}

export interface ResponseTiming {
  dns_ms: number
  connect_ms: number
  tls_ms: number
  send_ms: number
  wait_ms: number
  receive_ms: number
  total_ms: number
}

export interface PlaygroundResponse {
  status: number
  statusText: string
  headers: Record<string, string>
  body: string
  timing: ResponseTiming
  size: number
  duration: number
}

export interface SavedRequest {
  name: string
  collection: string
  method: HttpMethod
  url: string
  headers: KeyValueRow[]
  params: KeyValueRow[]
  body: string
  bodyType: BodyType
  authType: AuthType
  authConfig: AuthConfig
}

interface PlaygroundState {
  // Request config
  method: HttpMethod
  url: string
  headers: KeyValueRow[]
  params: KeyValueRow[]
  body: string
  bodyType: BodyType
  authType: AuthType
  authConfig: AuthConfig

  // Response
  response: PlaygroundResponse | null
  loading: boolean
  error: string | null

  // Collections
  collections: SavedRequest[]

  // Save-to-plan modal
  saveToPlanOpen: boolean
  setSaveToPlanOpen: (open: boolean) => void

  // Actions
  setMethod: (method: HttpMethod) => void
  setUrl: (url: string) => void
  setHeaders: (headers: KeyValueRow[]) => void
  setParams: (params: KeyValueRow[]) => void
  setBody: (body: string) => void
  setBodyType: (bodyType: BodyType) => void
  setAuthType: (authType: AuthType) => void
  setAuthConfig: (authConfig: Partial<AuthConfig>) => void
  sendRequest: () => Promise<void>
  saveToCollection: (name: string, collection: string) => void
  loadFromCollection: (index: number) => void
  removeFromCollection: (index: number) => void
  saveToPlan: (planId: string, parentNodeId: string) => Promise<void>
}

const defaultAuthConfig: AuthConfig = {
  username: '',
  password: '',
  token: '',
  apiKeyName: '',
  apiKeyValue: '',
  apiKeyIn: 'header',
}

function buildFullUrl(url: string, params: KeyValueRow[]): string {
  const enabledParams = params.filter((p) => p.enabled && p.key.trim())
  if (enabledParams.length === 0) return url

  const separator = url.includes('?') ? '&' : '?'
  const qs = enabledParams
    .map((p) => `${encodeURIComponent(p.key)}=${encodeURIComponent(p.value)}`)
    .join('&')
  return `${url}${separator}${qs}`
}

function buildAuthHeaders(authType: AuthType, authConfig: AuthConfig): Record<string, string> {
  switch (authType) {
    case 'basic': {
      const encoded = btoa(`${authConfig.username}:${authConfig.password}`)
      return { Authorization: `Basic ${encoded}` }
    }
    case 'bearer':
      return { Authorization: `Bearer ${authConfig.token}` }
    case 'apikey':
      if (authConfig.apiKeyIn === 'header' && authConfig.apiKeyName.trim()) {
        return { [authConfig.apiKeyName]: authConfig.apiKeyValue }
      }
      return {}
    default:
      return {}
  }
}

function buildAuthParams(authType: AuthType, authConfig: AuthConfig): Record<string, string> {
  if (authType === 'apikey' && authConfig.apiKeyIn === 'query' && authConfig.apiKeyName.trim()) {
    return { [authConfig.apiKeyName]: authConfig.apiKeyValue }
  }
  return {}
}

export const usePlaygroundStore = create<PlaygroundState>((set, get) => ({
  method: 'GET',
  url: '',
  headers: [{ key: '', value: '', enabled: true }],
  params: [{ key: '', value: '', enabled: true }],
  body: '',
  bodyType: 'none',
  authType: 'none',
  authConfig: { ...defaultAuthConfig },
  response: null,
  loading: false,
  error: null,
  collections: [],
  saveToPlanOpen: false,

  setSaveToPlanOpen: (open) => set({ saveToPlanOpen: open }),
  setMethod: (method) => set({ method }),
  setUrl: (url) => set({ url }),
  setHeaders: (headers) => set({ headers }),
  setParams: (params) => set({ params }),
  setBody: (body) => set({ body }),
  setBodyType: (bodyType) => set({ bodyType }),
  setAuthType: (authType) => set({ authType }),
  setAuthConfig: (partial) =>
    set((state) => ({ authConfig: { ...state.authConfig, ...partial } })),

  sendRequest: async () => {
    const { method, url, headers, params, body, bodyType, authType, authConfig } = get()
    if (!url.trim()) {
      set({ error: 'URL is required' })
      return
    }

    set({ loading: true, error: null, response: null })

    // Build headers
    const reqHeaders: Record<string, string> = {}
    for (const h of headers) {
      if (h.enabled && h.key.trim()) {
        reqHeaders[h.key] = h.value
      }
    }
    Object.assign(reqHeaders, buildAuthHeaders(authType, authConfig))

    // Build URL with params + auth query params
    let fullUrl = buildFullUrl(url, params)
    const authParams = buildAuthParams(authType, authConfig)
    for (const [k, v] of Object.entries(authParams)) {
      const sep = fullUrl.includes('?') ? '&' : '?'
      fullUrl += `${sep}${encodeURIComponent(k)}=${encodeURIComponent(v)}`
    }

    // Build body
    let reqBody: string | undefined
    if (method !== 'GET' && method !== 'HEAD') {
      if (bodyType === 'json' || bodyType === 'raw') {
        reqBody = body
        if (bodyType === 'json' && !reqHeaders['Content-Type']) {
          reqHeaders['Content-Type'] = 'application/json'
        }
      } else if (bodyType === 'form') {
        reqBody = body
        if (!reqHeaders['Content-Type']) {
          reqHeaders['Content-Type'] = 'application/x-www-form-urlencoded'
        }
      }
    }

    try {
      const res = await fetch('/api/v1/playground/request', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          method,
          url: fullUrl,
          headers: reqHeaders,
          body: reqBody,
        }),
      })

      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        set({ loading: false, error: `Proxy error: ${res.status} ${text}` })
        return
      }

      const data = await res.json()
      const bodyStr = typeof data.body === 'string' ? data.body : JSON.stringify(data.body, null, 2)
      const size = new Blob([bodyStr]).size

      set({
        loading: false,
        response: {
          status: data.status,
          statusText: data.statusText ?? '',
          headers: data.headers ?? {},
          body: bodyStr,
          timing: data.timing ?? {
            dns_ms: 0,
            connect_ms: 0,
            tls_ms: 0,
            send_ms: 0,
            wait_ms: 0,
            receive_ms: 0,
            total_ms: 0,
          },
          size,
          duration: data.timing?.total_ms ?? 0,
        },
      })
    } catch {
      set({
        loading: false,
        error: 'Could not reach the backend. Make sure the server is running.',
      })
    }
  },

  saveToCollection: (name, collection) => {
    const { method, url, headers, params, body, bodyType, authType, authConfig, collections } =
      get()
    const saved: SavedRequest = {
      name,
      collection,
      method,
      url,
      headers: JSON.parse(JSON.stringify(headers)),
      params: JSON.parse(JSON.stringify(params)),
      body,
      bodyType,
      authType,
      authConfig: { ...authConfig },
    }
    set({ collections: [...collections, saved] })
  },

  loadFromCollection: (index) => {
    const { collections } = get()
    const saved = collections[index]
    if (!saved) return
    set({
      method: saved.method,
      url: saved.url,
      headers: JSON.parse(JSON.stringify(saved.headers)),
      params: JSON.parse(JSON.stringify(saved.params)),
      body: saved.body,
      bodyType: saved.bodyType,
      authType: saved.authType,
      authConfig: { ...saved.authConfig },
      response: null,
      error: null,
    })
  },

  removeFromCollection: (index) => {
    const { collections } = get()
    set({ collections: collections.filter((_, i) => i !== index) })
  },

  saveToPlan: async (planId, parentNodeId) => {
    const { method, url, headers, params, body, bodyType, authType, authConfig } = get()

    const reqHeaders: Record<string, string> = {}
    for (const h of headers) {
      if (h.enabled && h.key.trim()) {
        reqHeaders[h.key] = h.value
      }
    }

    const node = {
      id: crypto.randomUUID(),
      type: 'http-sampler',
      name: `${method} ${url}`,
      enabled: true,
      properties: {
        method,
        url,
        headers: reqHeaders,
        params: params.filter((p) => p.enabled && p.key.trim()),
        body: bodyType !== 'none' ? body : undefined,
        bodyType,
        authType,
        authConfig: authType !== 'none' ? authConfig : undefined,
      },
      children: [],
    }

    await fetch(`/api/v1/plans/${planId}/nodes`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ parent_id: parentNodeId, node }),
    })

    set({ saveToPlanOpen: false })
  },
}))
