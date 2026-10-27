// A stand-in for the control plane in Storybook and tests. Routes are keyed
// "METHOD /path/{param}"; a value is returned as JSON, a function is called
// with the request. Unmatched requests get a 404 error body.

export interface MockRequest {
  method: string
  path: string
  params: Record<string, string>
  query: URLSearchParams
  body: unknown
}

export type MockRoutes = Record<string, unknown | ((req: MockRequest) => unknown)>

let routes: MockRoutes = {}
export const calls: MockRequest[] = []

export function setMockRoutes(next: MockRoutes) {
  routes = next
  calls.length = 0
}

function json(body: unknown, status = 200) {
  return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function match(pattern: string, path: string): Record<string, string> | null {
  const names: string[] = []
  const re = new RegExp('^' + pattern.replace(/\{(\w+)\}/g, (_, n) => (names.push(n), '([^/]+)')) + '$')
  const m = re.exec(path)
  return m ? Object.fromEntries(names.map((n, i) => [n, decodeURIComponent(m[i + 1])])) : null
}

export async function mockFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const req = input instanceof Request ? input : new Request(new URL(String(input), 'http://localhost'), init)
  const url = new URL(req.url)
  for (const [key, value] of Object.entries(routes)) {
    const [method, pattern] = key.split(' ')
    const params = method === req.method ? match(pattern, url.pathname) : null
    if (!params) continue
    const text = req.method === 'GET' || req.method === 'HEAD' ? '' : await req.clone().text()
    let body: unknown = text
    try {
      body = text ? JSON.parse(text) : undefined
    } catch {
      // not JSON
    }
    const call = { method: req.method, path: url.pathname, params, query: url.searchParams, body }
    calls.push(call)
    const out = typeof value === 'function' ? await (value as (r: MockRequest) => unknown)(call) : value
    return out instanceof Response ? out : json(out, req.method === 'POST' ? 201 : 200)
  }
  return json({ error: { code: 'NOT_MOCKED', message: `No mock for ${req.method} ${url.pathname}` } }, 404)
}
