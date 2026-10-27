import type { Meta, StoryObj } from '@storybook/react-vite'
import { useEffect } from 'react'
import { useAuthStore } from '../../stores/auth-store'
import { useUIStore } from '../../stores/ui-store'
import { EmptyState } from '../ui/primitives'
import { AppShell } from './AppShell'
import { ConnectionError, LoginScreen, SetupScreen } from './AuthScreens'
import { ErrorBoundary } from './ErrorBoundary'
import { Toaster } from './Toaster'

const meta = {
  title: 'Shell/AppShell',
  component: AppShell,
  args: { path: '/plans', children: <div className="p-8"><EmptyState title="Page content" /></div> },
} satisfies Meta<typeof AppShell>

export default meta
type Story = StoryObj<typeof meta>

export const LocalMode: Story = {}

export const TeamModeViewer: Story = {
  render: (args) => {
    useAuthStore.setState({ mode: 'team', user: { id: 'u1', display_name: 'Riley Chen', email: 'riley@example.com', role: 'viewer' } })
    return <AppShell {...args} />
  },
}

export const Login: Story = {
  render: () => {
    useAuthStore.setState({ status: 'login', mode: 'team', error: null })
    return <LoginScreen />
  },
}

export const FirstRunSetup: Story = { render: () => <SetupScreen /> }

export const CannotConnect: Story = {
  render: () => <ConnectionError message="The control plane answered 502" onRetry={() => {}} />,
}

function Boom(): never {
  throw new Error('Cannot read properties of undefined (reading "summary")')
}

export const RenderError: Story = {
  render: () => (
    <ErrorBoundary>
      <Boom />
    </ErrorBoundary>
  ),
}

function ToastDemo() {
  useEffect(() => {
    const { toast } = useUIStore.getState()
    toast('Plan saved', 'success')
    toast('Run started', 'info')
    toast('Could not reach the control plane', 'error')
  }, [])
  return <Toaster />
}

export const Toasts: Story = { render: () => <ToastDemo /> }
