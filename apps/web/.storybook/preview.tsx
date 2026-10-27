// Stories talk to fixture data, never to a server.
import './install-mocks'
import type { Decorator, Preview } from '@storybook/react-vite'
import { type ReactNode, useEffect } from 'react'
import '../src/index.css'
import { useAuthStore } from '../src/stores/auth-store'
import { applyAppearance } from '../src/stores/ui-store'

useAuthStore.setState({ status: 'ready', mode: 'local', token: 'osl_story', user: { display_name: 'Local user', role: 'admin' } })

function ThemeFrame({ theme, children }: { theme: 'dark' | 'light'; children: ReactNode }) {
  useEffect(() => applyAppearance(theme, 'comfortable'), [theme])
  return <div className="min-h-screen bg-slate-950 p-6 text-slate-100">{children}</div>
}

const withTheme: Decorator = (Story, context) => (
  <ThemeFrame theme={context.globals.theme as 'dark' | 'light'}>
    <Story />
  </ThemeFrame>
)

const preview: Preview = {
  decorators: [withTheme],
  globalTypes: {
    theme: {
      description: 'Colour theme',
      toolbar: { title: 'Theme', icon: 'mirror', items: ['dark', 'light'], dynamicTitle: true },
    },
  },
  initialGlobals: { theme: 'dark' },
  parameters: {
    layout: 'fullscreen',
    controls: { expanded: true },
  },
}

export default preview
