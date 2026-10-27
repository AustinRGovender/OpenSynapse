// Sample data shaped like the API's responses, for stories and tests.
import type { CheckStat, EndpointStat, Environment, Fragment, LiveState, MetricPoint, Plan, Run, RunError, RunEvent, Settings } from '@opensynapse/api-client'
import type { MockRoutes } from './mock-api'

const t0 = Date.UTC(2026, 9, 27, 12, 0, 0)

export const environments: Environment[] = [
  { id: 'env-docker', name: 'Test APIs (Docker)', variables: { ECHO_API: { value: 'http://echo-api:9001', secret: false }, ECOMMERCE_API: { value: 'http://mock-ecommerce:9002', secret: false } }, created_at: '2026-10-27T12:00:00Z', updated_at: '2026-10-27T12:00:00Z' } as Environment,
  { id: 'env-local', name: 'Test APIs (localhost)', variables: { ECHO_API: { value: 'http://localhost:9001', secret: false } }, created_at: '2026-10-27T12:00:00Z', updated_at: '2026-10-27T12:00:00Z' } as Environment,
]

export const plan: Plan = {
  id: 'plan-1',
  name: 'E-commerce — Browse & Buy',
  description: 'Browse products, view one, place an order.',
  tags: ['ecommerce'],
  version: 3,
  built_in: false,
  default_environment_id: 'env-docker',
  created_at: '2026-10-27T12:00:00Z',
  updated_at: '2026-10-27T12:00:00Z',
  root: {
    id: 'root',
    type: 'plan',
    name: 'E-commerce — Browse & Buy',
    enabled: true,
    properties: { thresholds: [{ metric: 'http_req_duration', condition: 'p(95)<500' }] },
    children: [
      {
        id: 'scenario',
        type: 'scenario',
        name: 'Shoppers',
        enabled: true,
        properties: { executor: 'ramping-vus', start_vus: 1, stages: [{ duration: '30s', target: 10 }, { duration: '1m', target: 10 }, { duration: '30s', target: 0 }] },
        children: [
          {
            id: 'tx',
            type: 'transaction-controller',
            name: 'Buy',
            enabled: true,
            properties: {},
            children: [
              { id: 'list', type: 'http', name: 'GET /products', enabled: true, properties: { method: 'GET', url: '${ECOMMERCE_API}/products', headers: [], body: { type: 'none' } }, children: [{ id: 'a1', type: 'assertion', name: 'status is 200', enabled: true, properties: { target: 'status', condition: 'equals', value: 200 }, children: [] }] },
              { id: 'order', type: 'http', name: 'POST /orders', enabled: true, properties: { method: 'POST', url: '${ECOMMERCE_API}/orders', headers: [], body: { type: 'json', content: '{"product_ids":["p1"]}' } }, children: [] },
              { id: 'think', type: 'timer', name: 'Think 1s', enabled: true, properties: { timer_type: 'constant', duration_ms: 1000 }, children: [] },
            ],
          },
        ],
      },
    ],
  },
}

export const series: MetricPoint[] = Array.from({ length: 60 }, (_, i) => {
  const vus = Math.min(10, 1 + Math.floor(i / 3))
  const rps = vus * 9 + Math.sin(i / 4) * 3
  return {
    timestamp_ms: t0 + i * 1000,
    active_vus: vus,
    requests: Math.round(rps),
    rps,
    failed: i > 40 && i < 45 ? 2 : 0,
    error_rate: i > 40 && i < 45 ? 2.2 : 0,
    p50_ms: 12 + Math.sin(i / 5) * 2,
    p90_ms: 30 + i * 0.2,
    p95_ms: 42 + i * 0.3,
    p99_ms: 80 + i * 0.5,
    iterations: Math.round(rps / 3),
    checks_passed: Math.round(rps / 3),
    checks_failed: 0,
    bytes_sent: Math.round(rps * 180),
    bytes_received: Math.round(rps * 900),
  }
})

export const endpoints: EndpointStat[] = [
  { name: 'GET /products', method: 'GET', requests: 1820, failed: 0, error_rate: 0, rps: 30.3, avg_ms: 14.2, p50_ms: 12.1, p95_ms: 38.4, p99_ms: 70.2, max_ms: 140.3 },
  { name: 'POST /orders', method: 'POST', requests: 910, failed: 8, error_rate: 0.88, rps: 15.2, avg_ms: 22.5, p50_ms: 19.8, p95_ms: 55.1, p99_ms: 98.7, max_ms: 210.9 },
]

export const checks: CheckStat[] = [
  { group: '::Buy', name: 'status is 200', passes: 1820, fails: 0 },
  { group: '::Buy', name: 'order created', passes: 902, fails: 8 },
]

export const errors: RunError[] = [
  { timestamp_ms: t0 + 42000, method: 'POST', name: 'POST /orders', url: 'http://mock-ecommerce:9002/orders', status: 503, error_code: '1503', message: 'service unavailable' },
  { timestamp_ms: t0 + 43000, method: 'POST', name: 'POST /orders', url: 'http://mock-ecommerce:9002/orders', status: 0, error_code: '1211', message: 'request timeout' },
]

export const events: RunEvent[] = [
  { id: 'e1', run_id: 'run-1', type: 'run_started', timestamp_ms: t0, payload: {} },
  { id: 'e2', run_id: 'run-1', type: 'control', timestamp_ms: t0 + 20000, payload: { vus: 10, message: 'VUs set to 10' } },
  { id: 'e3', run_id: 'run-1', type: 'note', timestamp_ms: t0 + 41000, payload: { message: 'Deploy started on the order service' } },
  { id: 'e4', run_id: 'run-1', type: 'run_completed', timestamp_ms: t0 + 60000, payload: {} },
]

export const summary = {
  total_requests: 2730,
  failed_requests: 8,
  error_rate: 0.29,
  throughput_rps: 45.5,
  duration_seconds: 60,
  iterations: 910,
  avg_ms: 17,
  min_ms: 2.1,
  max_ms: 210.9,
  p50_ms: 13.4,
  p90_ms: 33.2,
  p95_ms: 44.8,
  p99_ms: 85.3,
  peak_vus: 10,
  bytes_sent: 491400,
  bytes_received: 2457000,
  assertion_passes: 2722,
  assertion_failures: 8,
  thresholds_passed: true,
  endpoints,
  checks,
}

export const completedRun: Run = {
  id: 'run-1',
  plan_id: plan.id,
  plan_name: plan.name,
  plan_version: plan.version,
  plan_snapshot: plan.root,
  status: 'completed',
  parameters: {},
  created_at: '2026-10-27T12:00:00Z',
  started_at: '2026-10-27T12:00:00Z',
  ended_at: '2026-10-27T12:01:00Z',
  summary,
} as unknown as Run

export const live: LiveState = {
  active_vus: 8,
  target_vus: 8,
  max_vus: 50,
  manual_vus: false,
  can_control_vus: true,
  can_control_rps: true,
  can_control_duration: true,
  rps_target: 0,
  paused: false,
  stopping: false,
  elapsed_seconds: 95,
  remaining_seconds: 205,
  max_duration_seconds: 1500,
  workers: 1,
  starting: false,
}

export const fragments: Fragment[] = [
  {
    id: 'frag-login',
    name: 'Log in and keep the token',
    description: 'POST credentials, extract the token for later requests.',
    tags: ['auth'],
    built_in: true,
    bindings: [{ name: 'BASE_URL', description: 'API root, e.g. https://api.example.com', required: true }, { name: 'USERNAME', description: 'Account to log in as', required: false, default_value: 'demo' }],
    node_subtree: { id: 'f1', type: 'http', name: 'POST /login', enabled: true, properties: { method: 'POST', url: '${BASE_URL}/login', headers: [], body: { type: 'json', content: '{"user":"${USERNAME}"}' } }, children: [] },
    created_at: '2026-10-27T12:00:00Z',
    updated_at: '2026-10-27T12:00:00Z',
  },
]

export const settings: Settings = {
  general: { default_environment_id: 'env-docker', default_template: 'Load' },
  appearance: { theme: 'dark', density: 'comfortable', palette: 'default' },
  integrations: { webhook_url: '', slack_webhook_url: '' },
  storage: { retention_mode: 'keep_all', retention_count: 100, retention_days: 30 },
}

/** Routes that let most components render with sample data. */
export function defaultRoutes(): MockRoutes {
  return {
    'GET /api/v1/environments': { items: environments },
    'GET /api/v1/plans': { items: [plan] },
    'GET /api/v1/plans/{id}': plan,
    'PUT /api/v1/plans/{id}': (r: { body: unknown }) => ({ ...plan, ...(r.body as object), version: plan.version + 1 }),
    'GET /api/v1/fragments': { items: fragments },
    'GET /api/v1/runs': { items: [completedRun] },
    'GET /api/v1/runs/{id}': completedRun,
    'GET /api/v1/runs/{id}/samples': { items: series },
    'GET /api/v1/runs/{id}/events': { items: events },
    'GET /api/v1/runs/{id}/errors': { items: errors },
    'GET /api/v1/runs/{id}/live': live,
    'GET /api/v1/settings': settings,
    'GET /api/v1/playground/collections': { items: [] },
    'GET /api/v1/ai/config': { provider: 'anthropic', model: 'claude-sonnet-5-5', enabled: false, api_key_masked: '' },
  }
}
