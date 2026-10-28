import type { Meta, StoryObj } from '@storybook/react-vite'
import { checks, endpoints, errors, events, live, series } from '../../test/fixtures'
import { setMockRoutes } from '../../test/mock-api'
import { defaultRoutes } from '../../test/fixtures'
import { useChartTheme } from './chart-theme'
import { LiveControlPanel } from './LiveControlPanel'
import { MetricCard } from './MetricCard'
import { MetricChart } from './MetricChart'
import { ChecksTable, EndpointTable, ErrorLog, Timeline } from './RunTables'
import { RunView } from './RunView'

const meta = {
  title: 'Runs/RunView',
  component: RunView,
  args: { runId: 'run-1' },
} satisfies Meta<typeof RunView>

export default meta
type Story = StoryObj<typeof meta>

/** A finished run: summary, charts, tables. */
export const Completed: Story = {
  render: (args) => {
    setMockRoutes(defaultRoutes())
    return <RunView {...args} />
  },
}

export const NotFound: Story = {
  render: () => {
    setMockRoutes({})
    return <RunView runId="missing" />
  },
}

export const Cards: Story = {
  render: () => (
    <div className="grid max-w-3xl grid-cols-4 gap-3">
      <MetricCard label="Requests" value="2,730" />
      <MetricCard label="p95" value="44.8 ms" sub="last second" />
      <MetricCard label="Error rate" value="2.2%" alert />
      <MetricCard label="Active VUs" value="10" />
    </div>
  ),
}

function Charts() {
  const { metric } = useChartTheme()
  return (
    <div className="grid max-w-5xl gap-4 lg:grid-cols-2">
      <MetricChart title="Requests per second" unit="req/s" points={series} markers={[{ t: 20, label: 'VUs set to 10' }]} series={[{ key: 'rps', label: 'RPS', color: metric.rps, value: (p) => p.rps }]} />
      <MetricChart
        title="Response time percentiles"
        unit="ms"
        points={series}
        series={[
          { key: 'p50', label: 'p50', color: metric.p50, value: (p) => p.p50_ms },
          { key: 'p95', label: 'p95', color: metric.p95, value: (p) => p.p95_ms },
          { key: 'p99', label: 'p99', color: metric.p99, value: (p) => p.p99_ms },
        ]}
      />
      <MetricChart title="Error rate" unit="%" area points={series} series={[{ key: 'err', label: 'Errors', color: metric.error, value: (p) => p.error_rate }]} />
      <MetricChart title="Active virtual users" unit="VUs" step points={series} series={[{ key: 'vus', label: 'VUs', color: metric.vus, value: (p) => p.active_vus }]} formatValue={(v) => v.toFixed(0)} />
    </div>
  )
}

export const ChartsStory: Story = { name: 'Charts', render: () => <Charts /> }

export const Tables: Story = {
  render: () => (
    <div className="max-w-4xl space-y-6">
      <EndpointTable endpoints={endpoints} />
      <ChecksTable checks={checks} />
      <ErrorLog errors={errors} />
      <Timeline events={events} canNote onNote={async () => {}} />
    </div>
  ),
}

export const EmptyTables: Story = {
  render: () => (
    <div className="max-w-4xl space-y-6">
      <EndpointTable endpoints={[]} />
      <ChecksTable checks={[]} />
      <ErrorLog errors={[]} />
    </div>
  ),
}

export const LiveControl: Story = {
  render: () => (
    <div className="flex h-[720px] justify-end">
      <LiveControlPanel live={live} currentRps={84} control={async (p) => ({ ...live, target_vus: p.vus ?? live.target_vus })} stop={async () => {}} kill={async () => {}} />
    </div>
  ),
}

export const LiveControlPaused: Story = {
  render: () => (
    <div className="flex h-[720px] justify-end">
      <LiveControlPanel live={{ ...live, paused: true, rps_target: 40, manual_vus: true }} currentRps={0} control={async () => live} stop={async () => {}} kill={async () => {}} />
    </div>
  ),
}

export const LiveControlWorkersStarting: Story = {
  render: () => (
    <div className="flex h-[720px] justify-end">
      <LiveControlPanel live={{ ...live, workers: 4, starting: true, active_vus: 0, elapsed_seconds: 0 }} currentRps={0} control={async () => live} stop={async () => {}} kill={async () => {}} />
    </div>
  ),
}

export const LiveControlUnavailable: Story = {
  render: () => (
    <div className="flex h-[400px] justify-end">
      <LiveControlPanel live={null} currentRps={0} control={async () => live} stop={async () => {}} kill={async () => {}} />
    </div>
  ),
}
