// Vitest setup: jsdom gaps, the mock API, and a clean slate between tests.
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import { useAuthStore } from '../stores/auth-store'
import { mockFetch, setMockRoutes } from './mock-api'

// openapi-fetch builds Requests from relative URLs, which Node rejects.
const NodeRequest = globalThis.Request
class RelativeRequest extends NodeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(typeof input === 'string' && input.startsWith('/') ? new URL(input, 'http://localhost') : input, init)
  }
}
vi.stubGlobal('Request', RelativeRequest)
vi.stubGlobal('fetch', mockFetch)

// Charts and the flow canvas measure their containers.
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', NoopResizeObserver)
if (!window.matchMedia) {
  vi.stubGlobal('matchMedia', (query: string) => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, onchange: null, dispatchEvent: () => false }))
}

// Tests act as the local-mode user, who may edit everything.
useAuthStore.setState({ status: 'ready', mode: 'local', token: 'osl_test', user: { display_name: 'Local user', role: 'admin' } })

afterEach(() => {
  cleanup()
  setMockRoutes({})
})
