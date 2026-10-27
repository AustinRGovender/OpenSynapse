import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCrawlerStore } from '../stores/crawler-store'
import { emptyAuth, usePlaygroundStore } from '../stores/playground-store'
import { defaultRoutes, plan } from '../test/fixtures'
import { calls, setMockRoutes } from '../test/mock-api'
import { CrawlerPage } from './CrawlerPage'
import { PlaygroundPage } from './PlaygroundPage'

const response = {
  status: 201,
  status_text: '201 Created',
  final_url: 'http://mock-ecommerce:9002/orders',
  headers: { 'Content-Type': 'application/json' },
  body: '{"id":"o1","status":"pending"}',
  body_size: 30,
  truncated: false,
  cookies: [],
  timing: { dns_ms: 0, connect_ms: 0.2, tls_ms: 0, send_ms: 0, wait_ms: 1.1, receive_ms: 0.1, total_ms: 1.4 },
}

describe('PlaygroundPage', () => {
  beforeEach(() => {
    usePlaygroundStore.setState({ method: 'GET', url: '', params: [{ key: '', value: '', enabled: true }], headers: [{ key: '', value: '', enabled: true }], body: '', bodyType: 'none', authType: 'none', auth: { ...emptyAuth }, environmentId: '', response: null, error: null })
    setMockRoutes({ ...defaultRoutes(), 'POST /api/v1/playground/request': response })
  })

  it('sends a request through the selected environment and shows the response', async () => {
    render(<PlaygroundPage />)
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Method' }), 'POST')
    await userEvent.selectOptions(await screen.findByRole('combobox', { name: 'Environment' }), 'Test APIs (Docker)')
    await userEvent.type(screen.getByRole('textbox', { name: 'URL' }), '${{ECOMMERCE_API}/orders')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))

    expect(await screen.findByText('201 Created')).toBeInTheDocument()
    const sent = calls.find((c) => c.path === '/api/v1/playground/request')!
    expect(sent.body).toMatchObject({ method: 'POST', url: '${ECOMMERCE_API}/orders', environment_id: 'env-docker', auth_type: 'none' })
  })

  it('adds the request to a plan as an HTTP node with its auth as a header', async () => {
    let saved: { root?: { children: { children: { id: string; type: string; properties: Record<string, unknown> }[] }[] } } = {}
    setMockRoutes({ ...defaultRoutes(), 'PUT /api/v1/plans/{id}': (r: { body: unknown }) => ((saved = r.body as typeof saved), { ...plan, version: 4 }) })
    usePlaygroundStore.setState({ url: '${ECOMMERCE_API}/products', authType: 'bearer', auth: { ...emptyAuth, token: 'tok-123' } })
    render(<PlaygroundPage />)

    await userEvent.click(screen.getByRole('button', { name: 'Add to plan' }))
    const dialog = screen.getByRole('dialog')
    await within(dialog).findByRole('option', { name: plan.name })
    await userEvent.selectOptions(within(dialog).getByRole('combobox', { name: 'Plan' }), plan.id)
    await within(dialog).findByRole('combobox', { name: 'Add inside' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add to plan' }))

    await waitFor(() => expect(saved.root).toBeDefined())
    const scenario = saved.root!.children[0]
    const node = scenario.children[scenario.children.length - 1]
    expect(node.type).toBe('http')
    expect(node.properties).toMatchObject({ method: 'GET', url: '${ECOMMERCE_API}/products', headers: [{ key: 'Authorization', value: 'Bearer tok-123', enabled: true }] })
  })
})

describe('CrawlerPage', () => {
  beforeEach(() => {
    useCrawlerStore.setState({ crawl: null, error: null, starting: false })
  })

  it('imports an OpenAPI document and lists its operations', async () => {
    const spec = JSON.stringify({ paths: { '/products': { get: { summary: 'List products' } }, '/orders': { post: { summary: 'Create order' } } } })
    const crawl = { id: 'c1', entry_url: 'http://shop:9002/openapi.yaml', openapi_url: 'http://shop:9002/openapi.yaml', engine: 'rod', status: 'completed', depth: 3, same_origin: true, blocklist: [], request_limit: 500, auth_config: { type: '' }, progress: { pages_discovered: 2, requests_captured: 2 }, graph: { nodes: [], edges: [] }, requests: [], openapi_spec: spec, created_at: '', updated_at: '' }
    setMockRoutes({ 'POST /api/v1/crawls': { ...crawl, status: 'pending' }, 'GET /api/v1/crawls/{id}': crawl })

    render(<CrawlerPage />)
    await userEvent.click(screen.getByRole('radio', { name: 'Import OpenAPI' }))
    await userEvent.type(screen.getByRole('textbox', { name: 'OpenAPI / Swagger document URL' }), 'http://shop:9002/openapi.yaml')
    await userEvent.click(screen.getByRole('button', { name: 'Import' }))

    expect(await screen.findByText('List products')).toBeInTheDocument()
    expect(screen.getByText('2 operations')).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ entry_url: 'http://shop:9002/openapi.yaml', openapi_url: 'http://shop:9002/openapi.yaml' })
  })
})
