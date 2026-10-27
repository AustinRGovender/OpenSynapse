import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { findNode, usePlanStore } from '../stores/plan-store'
import { useUIStore } from '../stores/ui-store'
import { defaultRoutes, live, plan } from '../test/fixtures'
import { calls, setMockRoutes } from '../test/mock-api'
import { FragmentLibrary } from './fragments/FragmentLibrary'
import { RunConfigDialog } from './run-config/RunConfigDialog'
import { LiveControlPanel } from './run-view/LiveControlPanel'
import { ConfirmDialog, Field, TextInput } from './ui/primitives'

describe('primitives', () => {
  it('names a field by its label only, not its hint', () => {
    render(<Field label="Collection" hint="Pick one or type a new name."><TextInput /></Field>)
    expect(screen.getByRole('textbox', { name: 'Collection' })).toBeInTheDocument()
  })

  it('confirms and cancels', async () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<ConfirmDialog open title="Delete plan?" message="Runs are kept." onConfirm={onConfirm} onCancel={onCancel} />)
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(onCancel).toHaveBeenCalledOnce()
  })
})

describe('RunConfigDialog', () => {
  beforeEach(() => {
    setMockRoutes(defaultRoutes())
    useUIStore.setState({ system: { k6: { version: 'v1.8.2', kv: true } } as never })
  })

  it('runs a custom constant-VU profile with an RPS limit in the chosen environment', async () => {
    const onConfirm = vi.fn()
    render(<RunConfigDialog open onClose={() => {}} onConfirm={onConfirm} planScenarioProperties={{ executor: 'constant-vus', vus: 5, duration: '1m' }} defaultEnvironmentId="env-docker" />)
    await screen.findByRole('option', { name: /Test APIs \(Docker\) \(plan default\)/ })
    await userEvent.click(screen.getByRole('button', { name: /^Custom/ }))
    const vus = screen.getByRole('spinbutton', { name: 'Virtual users' })
    await userEvent.clear(vus)
    await userEvent.type(vus, '12')
    await userEvent.type(screen.getByRole('spinbutton', { name: 'RPS limit (optional)' }), '40')
    await userEvent.click(screen.getByRole('button', { name: 'Start run' }))
    expect(onConfirm).toHaveBeenCalledWith({ environment_id: 'env-docker', parameters: { vus_target: 12, duration_seconds: 60, rps_target: 40 } })
  })

  it('locks arrival-rate plans to their own profile', () => {
    render(<RunConfigDialog open onClose={() => {}} onConfirm={() => {}} planScenarioProperties={{ executor: 'constant-arrival-rate', rate: 20, duration: '1m' }} defaultEnvironmentId={null} />)
    expect(screen.getByRole('button', { name: /^Custom/ })).toBeDisabled()
    expect(screen.getByRole('spinbutton', { name: 'RPS limit (optional)' })).toBeDisabled()
  })
})

describe('LiveControlPanel', () => {
  it('applies a new VU target', async () => {
    const control = vi.fn(async () => live)
    render(<LiveControlPanel live={live} currentRps={80} control={control} stop={async () => {}} kill={async () => {}} />)
    const input = screen.getByRole('spinbutton', { name: 'Target VUs value' })
    await userEvent.clear(input)
    await userEvent.type(input, '20')
    await userEvent.click(within(input.parentElement!).getByRole('button', { name: 'Apply' }))
    expect(control).toHaveBeenCalledWith({ vus: 20 })
  })

  it('asks before killing a run', async () => {
    const kill = vi.fn(async () => {})
    render(<LiveControlPanel live={live} currentRps={80} control={async () => live} stop={async () => {}} kill={kill} />)
    await userEvent.click(screen.getByRole('button', { name: 'Kill' }))
    expect(kill).not.toHaveBeenCalled()
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Kill/ }))
    expect(kill).toHaveBeenCalledOnce()
  })
})

describe('FragmentLibrary', () => {
  beforeEach(() => {
    setMockRoutes(defaultRoutes())
    usePlanStore.setState({ plan: structuredClone(plan), selectedNodeId: 'tx', undoStack: [], redoStack: [] })
  })

  it('fills bindings and inserts the fragment into the selected controller', async () => {
    render(<FragmentLibrary onClose={() => {}} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Insert' }))
    const dialog = screen.getByRole('dialog')
    const insert = within(dialog).getByRole('button', { name: 'Insert' })
    expect(insert).toBeDisabled() // BASE_URL is required
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'BASE_URL *' }), 'http://shop.test')
    await userEvent.click(insert)

    const tx = findNode(usePlanStore.getState().plan!.root, 'tx')!
    const added = tx.children[tx.children.length - 1]
    expect(added.name).toBe('POST /login')
    expect(added.id).not.toBe('f1')
    expect((added.properties as { url: string }).url).toBe('http://shop.test/login')
    expect(JSON.stringify(added.properties)).toContain('"user\\":\\"demo\\"')
  })
})

describe('mock API', () => {
  it('records the requests components make', async () => {
    setMockRoutes(defaultRoutes())
    usePlanStore.setState({ plan: structuredClone(plan), selectedNodeId: null })
    render(<FragmentLibrary onClose={() => {}} />)
    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/fragments')).toBe(true))
  })
})
