import type { Meta, StoryObj } from '@storybook/react-vite'
import { LoadCurveSvg } from './LoadCurveSvg'
import { TEMPLATES } from './template-data'
import { TemplateGallery } from './TemplateGallery'

const meta = {
  title: 'Plans/TemplateGallery',
  component: TemplateGallery,
  args: { open: true, onClose: () => {}, defaultEnvironmentId: 'env-docker' },
} satisfies Meta<typeof TemplateGallery>

export default meta
type Story = StoryObj<typeof meta>

export const Gallery: Story = {}

export const LoadCurves: Story = {
  render: () => (
    <div className="grid max-w-4xl grid-cols-3 gap-4">
      {TEMPLATES.map((t) => (
        <div key={t.name} className="rounded-lg border border-slate-800 p-3">
          <LoadCurveSvg path={t.svgPath} name={t.name} />
          <p className="mt-1 text-sm text-slate-200">{t.name}</p>
        </div>
      ))}
    </div>
  ),
}
