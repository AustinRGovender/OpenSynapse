import type { Meta, StoryObj } from '@storybook/react-vite'
import { useAIStore } from '../../stores/ai-store'
import { AIAnalysisButton, AIBusy, AIInsights } from './AIAnalysis'

const meta = {
  title: 'AI/Analysis',
  component: AIAnalysisButton,
  args: { runIds: ['run-1'] },
} satisfies Meta<typeof AIAnalysisButton>

export default meta
type Story = StoryObj<typeof meta>

/** AI is opt-in: until a key is added in Settings › AI, no button is shown. */
export const NotConfigured: Story = {
  render: (args) => {
    useAIStore.setState({ config: { provider: 'anthropic', model: 'claude-sonnet-5-5', enabled: false, api_key_masked: '' } })
    return (
      <div className="text-xs text-slate-500">
        [<AIAnalysisButton {...args} />] ← nothing renders while AI is off
      </div>
    )
  },
}

export const Configured: Story = {
  render: (args) => {
    useAIStore.setState({ config: { provider: 'anthropic', model: 'claude-sonnet-5-5', enabled: true, api_key_masked: 'sk-ant-…a1b2' } })
    return <AIAnalysisButton {...args} />
  },
}

export const Insights: Story = {
  render: (args) => {
    useAIStore.setState({
      analyses: {
        'run-1': [
          {
            id: 'a1',
            question: 'What does this run tell me?',
            response: '**Healthy overall.** p95 stayed under 50 ms until VUs reached 10.\n\n- POST /orders had 8 failures (503) around 0:42, matching the deploy note.\n- Next: rerun at 20 VUs with an RPS limit of 60 to find the knee.',
            provider: 'anthropic',
            model: 'claude-sonnet-5-5',
            prompt: '',
            input_tokens: 2140,
            output_tokens: 180,
            cost_usd: 0.009,
            created_at: '2026-10-27T12:02:00Z',
          },
        ],
      },
    })
    return <AIInsights {...args} />
  },
}

export const Busy: Story = { render: () => <AIBusy /> }
