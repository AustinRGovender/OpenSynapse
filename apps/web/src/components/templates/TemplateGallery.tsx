import { useState } from 'react'
import { OpenSynapseClient, type Node } from '@opensynapse/api-client'
import { type TemplateConfig, TEMPLATES } from './template-data'
import { LoadCurveSvg } from './LoadCurveSvg'

const client = new OpenSynapseClient('/api/v1')

interface TemplateGalleryProps {
  open: boolean
  onClose: () => void
}

function buildPlanRoot(template: TemplateConfig): Node {
  return {
    id: crypto.randomUUID(),
    type: 'plan',
    name: `${template.name} Test`,
    enabled: true,
    properties: {},
    children: [
      {
        id: crypto.randomUUID(),
        type: 'scenario',
        name: `${template.name} Scenario`,
        enabled: true,
        properties: template.scenarioProperties,
        children: [],
      },
    ],
  }
}

function buildBlankRoot(): Node {
  return {
    id: crypto.randomUUID(),
    type: 'plan',
    name: 'Untitled Plan',
    enabled: true,
    properties: {},
    children: [],
  }
}

export function TemplateGallery({ open, onClose }: TemplateGalleryProps) {
  const [creating, setCreating] = useState<string | null>(null)

  if (!open) return null

  async function handleUseTemplate(template: TemplateConfig) {
    setCreating(template.name)
    try {
      const root = buildPlanRoot(template)
      const plan = await client.createPlan({
        name: `${template.name} Test`,
        description: template.description,
        tags: [template.name.toLowerCase().replace(/[\s-]/g, '-')],
        root,
      })
      window.location.hash = `#/plans/${plan.id}`
      onClose()
    } catch {
      // Plan creation may fail if API is down
    } finally {
      setCreating(null)
    }
  }

  async function handleBlankPlan() {
    setCreating('blank')
    try {
      const root = buildBlankRoot()
      const plan = await client.createPlan({
        name: 'Untitled Plan',
        description: '',
        tags: [],
        root,
      })
      window.location.hash = `#/plans/${plan.id}`
      onClose()
    } catch {
      // Plan creation may fail if API is down
    } finally {
      setCreating(null)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="mx-4 max-h-[90vh] w-full max-w-[960px] overflow-y-auto rounded-xl border border-slate-800 bg-slate-950 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold text-slate-100">New Test</h2>
            <p className="mt-0.5 text-sm text-slate-400">
              Choose a template or start with a blank plan
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
              <path d="M15 5L5 15M5 5l10 10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        {/* Blank plan card */}
        <div className="px-6 pt-5">
          <button
            onClick={handleBlankPlan}
            disabled={creating !== null}
            className="flex w-full items-center gap-4 rounded-lg border border-dashed border-slate-700 bg-slate-900/50 px-4 py-3 text-left transition-colors hover:border-slate-600 hover:bg-slate-900 disabled:opacity-50"
          >
            <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-slate-700 bg-slate-800">
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                <path d="M10 4v12M4 10h12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className="text-slate-400" />
              </svg>
            </div>
            <div>
              <p className="text-sm font-medium text-slate-200">
                {creating === 'blank' ? 'Creating...' : 'Blank Plan'}
              </p>
              <p className="text-xs text-slate-500">Start from scratch with an empty plan</p>
            </div>
          </button>
        </div>

        {/* Template grid */}
        <div className="grid grid-cols-1 gap-4 p-6 sm:grid-cols-2 lg:grid-cols-4">
          {TEMPLATES.map((template) => (
            <div
              key={template.name}
              className="flex flex-col overflow-hidden rounded-lg border border-slate-800 bg-slate-900 transition-colors hover:border-slate-700"
            >
              {/* SVG animation */}
              <div className="border-b border-slate-800 px-3 pt-3 pb-2">
                <LoadCurveSvg path={template.svgPath} name={template.name} />
              </div>

              {/* Card body */}
              <div className="flex flex-1 flex-col p-3">
                <h3 className="text-sm font-bold text-slate-100">{template.name}</h3>
                <p className="mt-0.5 flex-1 text-xs text-slate-400">{template.description}</p>
                <button
                  onClick={() => handleUseTemplate(template)}
                  disabled={creating !== null}
                  className="mt-3 w-full rounded-md bg-teal-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-teal-500 disabled:opacity-50"
                >
                  {creating === template.name ? 'Creating...' : 'Use template'}
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
