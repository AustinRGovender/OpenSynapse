import { create } from 'zustand'
import type { Crawl } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'

export type CrawlEngine = 'rod' | 'colly' | 'zap'
export type CrawlAuthType = 'none' | 'basic' | 'bearer' | 'form_login'
export type CrawlSource = 'site' | 'openapi'

export interface CrawlConfig {
  source: CrawlSource
  entry_url: string
  openapi_url: string
  engine: CrawlEngine
  auth_type: CrawlAuthType
  username: string
  password: string
  token: string
  depth: number
  same_origin: boolean
  blocklist: string
  limit: number
}

const LAST_CRAWL_KEY = 'opensynapse.lastCrawl'

const defaultConfig: CrawlConfig = {
  source: 'site',
  entry_url: '',
  openapi_url: '',
  engine: 'colly',
  auth_type: 'none',
  username: '',
  password: '',
  token: '',
  depth: 3,
  same_origin: true,
  blocklist: '/logout, /delete',
  limit: 500,
}

interface State {
  config: CrawlConfig
  crawl: Crawl | null
  starting: boolean
  error: string | null
  setConfig: (patch: Partial<CrawlConfig>) => void
  start: () => Promise<void>
  cancel: () => Promise<void>
  restore: () => Promise<void>
  generatePlan: () => Promise<string | null>
  reset: () => void
}

let pollTimer: ReturnType<typeof setTimeout> | null = null

function stopPolling() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

export const isActive = (c: Crawl | null) => c?.status === 'pending' || c?.status === 'crawling'

export const useCrawlerStore = create<State>((set, get) => {
  async function poll(id: string, failures = 0) {
    try {
      const crawl = await unwrap(api.GET('/api/v1/crawls/{id}', { params: { path: { id } } }))
      if (get().crawl?.id !== id) return
      set({ crawl, error: null })
      if (isActive(crawl)) pollTimer = setTimeout(() => poll(id), 1500)
    } catch (e) {
      if (get().crawl?.id !== id) return
      // Ride out brief network blips; give up after about half a minute.
      if (failures < 10) pollTimer = setTimeout(() => poll(id, failures + 1), 3000)
      else set({ error: `Lost track of the crawl: ${errorMessage(e)}` })
    }
  }

  return {
    config: { ...defaultConfig },
    crawl: null,
    starting: false,
    error: null,

    setConfig: (patch) => set({ config: { ...get().config, ...patch } }),

    start: async () => {
      const c = get().config
      const url = c.source === 'openapi' ? c.openapi_url.trim() : c.entry_url.trim()
      if (!url) {
        set({ error: c.source === 'openapi' ? 'Enter the URL of an OpenAPI or Swagger document' : 'Enter the URL to start crawling from' })
        return
      }
      stopPolling()
      set({ starting: true, error: null, crawl: null })
      try {
        const body =
          c.source === 'openapi'
            ? { entry_url: url, openapi_url: url }
            : {
                entry_url: url,
                engine: c.engine,
                depth: c.depth,
                same_origin: c.same_origin,
                limit: c.limit,
                blocklist: c.blocklist.split(',').map((s) => s.trim()).filter(Boolean),
                auth: c.auth_type === 'none' ? undefined : { type: c.auth_type, username: c.username, password: c.password, token: c.token },
              }
        const crawl = await unwrap(api.POST('/api/v1/crawls', { body }))
        try {
          localStorage.setItem(LAST_CRAWL_KEY, crawl.id)
        } catch {
          // Remembering the last crawl is a convenience only.
        }
        set({ crawl, starting: false })
        poll(crawl.id)
      } catch (e) {
        set({ starting: false, error: errorMessage(e) })
      }
    },

    cancel: async () => {
      const crawl = get().crawl
      if (!crawl) return
      try {
        await unwrap(api.POST('/api/v1/crawls/{id}/cancel', { params: { path: { id: crawl.id } } }))
        stopPolling()
        poll(crawl.id)
      } catch (e) {
        set({ error: errorMessage(e) })
      }
    },

    restore: async () => {
      if (get().crawl) return
      let id: string | null
      try {
        id = localStorage.getItem(LAST_CRAWL_KEY)
      } catch {
        return
      }
      if (!id) return
      try {
        const crawl = await unwrap(api.GET('/api/v1/crawls/{id}', { params: { path: { id } } }))
        set({ crawl })
        if (isActive(crawl)) poll(crawl.id)
      } catch {
        // The crawl was deleted or the data directory changed.
      }
    },

    generatePlan: async () => {
      const crawl = get().crawl
      if (!crawl) return null
      try {
        const plan = await unwrap(api.POST('/api/v1/crawls/{id}/generate-plan', { params: { path: { id: crawl.id } } }))
        return plan.id
      } catch (e) {
        set({ error: errorMessage(e) })
        return null
      }
    },

    reset: () => {
      stopPolling()
      try {
        localStorage.removeItem(LAST_CRAWL_KEY)
      } catch {
        // ignore
      }
      set({ crawl: null, error: null })
    },
  }
})
