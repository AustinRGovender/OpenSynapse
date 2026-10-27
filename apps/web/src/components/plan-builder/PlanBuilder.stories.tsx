import type { Meta, StoryObj } from '@storybook/react-vite'
import type { ReactElement } from 'react'
import { usePlanStore } from '../../stores/plan-store'
import { plan } from '../../test/fixtures'
import { FragmentLibrary } from '../fragments/FragmentLibrary'
import { FlowCanvas } from './FlowCanvas'
import { NodeTree } from './NodeTree'
import { PlanBuilder } from './PlanBuilder'
import { PropertyPanel } from './PropertyPanel'

function withPlan(selectedNodeId: string | null, child: ReactElement) {
  usePlanStore.setState({ plan: structuredClone(plan), selectedNodeId, loading: false, notFound: false, dirty: false, undoStack: [], redoStack: [] })
  return child
}

const meta = {
  title: 'Builder/PlanBuilder',
  component: PlanBuilder,
  args: { planId: 'plan-1' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PlanBuilder>

export default meta
type Story = StoryObj<typeof meta>

/** The whole builder, loading the plan from the (mock) API. */
export const Builder: Story = { render: (args) => <div className="h-[760px]"><PlanBuilder {...args} /></div> }

export const Tree: Story = { render: () => withPlan('order', <div className="h-[600px] w-72 border border-slate-800"><NodeTree /></div>) }

export const Canvas: Story = { render: () => withPlan('list', <div className="h-[600px] border border-slate-800"><FlowCanvas /></div>) }

export const HttpRequestProperties: Story = { render: () => withPlan('order', <div className="h-[700px] w-80 border border-slate-800"><PropertyPanel /></div>) }

export const ScenarioProperties: Story = { render: () => withPlan('scenario', <div className="h-[700px] w-80 border border-slate-800"><PropertyPanel /></div>) }

export const PlanPropertiesAndThresholds: Story = { render: () => withPlan(null, <div className="h-[700px] w-80 border border-slate-800"><PropertyPanel /></div>) }

export const AssertionProperties: Story = { render: () => withPlan('a1', <div className="h-[500px] w-80 border border-slate-800"><PropertyPanel /></div>) }

export const Fragments: Story = { render: () => withPlan('tx', <div className="h-[600px] w-80 border border-slate-800"><FragmentLibrary onClose={() => {}} /></div>) }
