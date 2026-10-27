import type { Meta, StoryObj } from '@storybook/react-vite'
import { useUIStore } from '../../stores/ui-store'
import { RunConfigDialog } from './RunConfigDialog'

const meta = {
  title: 'Runs/RunConfigDialog',
  component: RunConfigDialog,
  args: {
    open: true,
    onClose: () => {},
    onConfirm: () => {},
    planScenarioProperties: { executor: 'ramping-vus', start_vus: 1, stages: [{ duration: '30s', target: 10 }, { duration: '1m', target: 10 }, { duration: '30s', target: 0 }] },
    defaultEnvironmentId: 'env-docker',
  },
} satisfies Meta<typeof RunConfigDialog>

export default meta
type Story = StoryObj<typeof meta>

/** With xk6-kv available, an RPS limit can be set. */
export const VUBasedPlan: Story = {
  render: (args) => {
    useUIStore.setState({ system: { k6: { version: 'v1.8.2', kv: true } } as never })
    return <RunConfigDialog {...args} />
  },
}

/** Arrival-rate plans run with their own profile. */
export const ArrivalRatePlan: Story = {
  args: { planScenarioProperties: { executor: 'constant-arrival-rate', rate: 50, time_unit: '1s', duration: '2m', pre_allocated_vus: 20 } },
}

export const Starting: Story = { args: { starting: true } }
