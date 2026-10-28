// Imported first by preview.tsx: the API client captures fetch when it is
// created, so the mock must be in place before any app module loads.
import { defaultRoutes } from '../src/test/fixtures'
import { mockFetch, setMockRoutes } from '../src/test/mock-api'

window.fetch = mockFetch
setMockRoutes(defaultRoutes())
