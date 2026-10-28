import type { StorybookConfig } from '@storybook/react-vite'

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  framework: { name: '@storybook/react-vite', options: {} },
  core: {
    // OpenSynapse never phones home; neither does its Storybook.
    disableTelemetry: true,
    disableWhatsNewNotifications: true,
  },
}

export default config
