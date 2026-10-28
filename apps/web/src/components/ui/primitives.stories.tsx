import type { Meta, StoryObj } from '@storybook/react-vite'
import { useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, Field, Modal, PageHeader, Select, Spinner, StatusBadge, TextInput } from './primitives'

const meta = {
  title: 'UI/Primitives',
  component: Button,
  args: { children: 'Button' },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Buttons: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary">Primary</Button>
      <Button>Secondary</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="danger">Danger</Button>
      <Button variant="primary" size="sm">Small</Button>
      <Button variant="primary" disabled>Disabled</Button>
    </div>
  ),
}

export const Badges: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Badge>Neutral</Badge>
      <Badge tone="accent">Built-in</Badge>
      <Badge tone="warn">Truncated</Badge>
      <Badge tone="error">Failed</Badge>
      {['pending', 'running', 'completed', 'failed', 'aborted'].map((s) => <StatusBadge key={s} status={s} />)}
    </div>
  ),
}

export const CardAndHeader: Story = {
  render: () => (
    <div className="max-w-2xl">
      <PageHeader title="Test plans" description="A plan describes what virtual users do and how much load to apply." actions={<Button variant="primary">New test</Button>} />
      <Card className="p-4 text-sm text-slate-300">Card content</Card>
    </div>
  ),
}

export const Loading: Story = { render: () => <Spinner label="Loading plans…" /> }

export const Empty: Story = {
  render: () => (
    <EmptyState title="No runs yet" action={<Button variant="primary">Open a plan</Button>}>
      Runs appear here once you start one from the plan builder.
    </EmptyState>
  ),
}

export const FormFields: Story = {
  render: () => (
    <div className="grid max-w-md gap-4">
      <Field label="Name" hint="Shown in the plans list.">
        <TextInput placeholder="Checkout flow" />
      </Field>
      <Field label="Environment">
        <Select>
          <option>Test APIs (Docker)</option>
          <option>Test APIs (localhost)</option>
        </Select>
      </Field>
    </div>
  ),
}

function ModalDemo() {
  const [open, setOpen] = useState(true)
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open modal</Button>
      <Modal open={open} onClose={() => setOpen(false)} title="Save request" footer={<Button variant="primary" onClick={() => setOpen(false)}>Save</Button>}>
        <Field label="Request name"><TextInput defaultValue="GET /products" /></Field>
      </Modal>
    </>
  )
}

export const ModalDialog: Story = { render: () => <ModalDemo /> }

function ConfirmDemo() {
  const [open, setOpen] = useState(true)
  return (
    <>
      <Button variant="danger" onClick={() => setOpen(true)}>Delete plan</Button>
      <ConfirmDialog open={open} title="Delete this plan?" message="Runs of this plan are kept." confirmLabel="Delete" onConfirm={() => setOpen(false)} onCancel={() => setOpen(false)} />
    </>
  )
}

export const Confirm: Story = { render: () => <ConfirmDemo /> }
