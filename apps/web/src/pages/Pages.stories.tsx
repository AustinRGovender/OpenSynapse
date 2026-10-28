import type { Meta, StoryObj } from '@storybook/react-vite'
import type { Run } from '@opensynapse/api-client'
import { useCrawlerStore } from '../stores/crawler-store'
import { completedRun, defaultRoutes, shopCrawl, summary } from '../test/fixtures'
import { setMockRoutes } from '../test/mock-api'
import { ComparisonPage } from './ComparisonPage'
import { CrawlerPage } from './CrawlerPage'
import { EnvironmentsPage } from './EnvironmentsPage'
import { PlansPage } from './PlansPage'
import { PlaygroundPage } from './PlaygroundPage'
import { RunsListPage } from './RunsListPage'
import { SettingsPage } from './SettingsPage'

const runB = { ...completedRun, id: 'run-2', summary: { ...summary, p95_ms: 61.2, p99_ms: 120.4, error_rate: 1.1, throughput_rps: 41.8 } } as unknown as Run

function routes() {
  return {
    ...defaultRoutes(),
    'GET /api/v1/runs': { items: [completedRun, runB] },
    'POST /api/v1/compare': {
      runs: [completedRun, runB],
      changes: [
        { metric: 'p95_ms', value_a: 44.8, value_b: 61.2, abs_diff: 16.4, pct_change: 36.6, improved: false, significant: true, p_value: 0.004, run_a_id: 'run-1', run_b_id: 'run-2' },
        { metric: 'throughput_rps', value_a: 45.5, value_b: 41.8, abs_diff: -3.7, pct_change: -8.1, improved: false, significant: false, p_value: 0.21, run_a_id: 'run-1', run_b_id: 'run-2' },
      ],
    },
    'GET /api/v1/storage': { data_dir: '/data', database: 'sqlite', database_bytes: 4_200_000, samples_bytes: 1_100_000, runs: 2, prunable_runs: 0, prunable_bytes_est: 0, retention: { retention_mode: 'keep_all', retention_count: 100, retention_days: 30 } },
    'POST /api/v1/playground/request': {
      status: 200,
      status_text: '200 OK',
      final_url: 'http://mock-ecommerce:9002/products',
      headers: { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': '173' },
      body: '[{"id":"p1","name":"Laptop","price":999.99,"stock":10},{"id":"p2","name":"Mouse","price":29.99,"stock":50}]',
      body_size: 173,
      truncated: false,
      cookies: [{ name: 'session', value: 'abc123', path: '/', http_only: true, secure: false }],
      timing: { dns_ms: 0.4, connect_ms: 0.3, tls_ms: 0, send_ms: 0.1, wait_ms: 2.2, receive_ms: 0.2, total_ms: 3.2 },
    },
  }
}

const meta = {
  title: 'Pages/Plans',
  component: PlansPage,
  render: () => {
    setMockRoutes(routes())
    return <PlansPage />
  },
} satisfies Meta<typeof PlansPage>

export default meta
type Story = StoryObj<typeof meta>

export const Plans: Story = {}

export const Runs: Story = {
  render: () => {
    setMockRoutes(routes())
    return <RunsListPage />
  },
}

export const Compare: Story = {
  render: () => {
    setMockRoutes(routes())
    return <ComparisonPage ids={['run-1', 'run-2']} />
  },
}

export const Environments: Story = {
  render: () => {
    setMockRoutes(routes())
    return <EnvironmentsPage />
  },
}

export const Playground: Story = {
  render: () => {
    setMockRoutes(routes())
    return <div className="h-[760px]"><PlaygroundPage /></div>
  },
}

export const Crawler: Story = {
  render: () => {
    setMockRoutes(routes())
    useCrawlerStore.setState({ crawl: null })
    return <div className="h-[600px]"><CrawlerPage /></div>
  },
}

export const CrawlerResults: Story = {
  render: () => {
    setMockRoutes(routes())
    useCrawlerStore.setState({ crawl: shopCrawl })
    return <div className="h-[600px]"><CrawlerPage /></div>
  },
}

export const Settings: Story = {
  render: () => {
    setMockRoutes(routes())
    return <SettingsPage tab="appearance" />
  },
}
