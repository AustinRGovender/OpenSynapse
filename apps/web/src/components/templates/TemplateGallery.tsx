import { useState } from 'react'
import { OpenSynapseClient, type Node } from '@opensynapse/api-client'

const client = new OpenSynapseClient('/api/v1')

interface TemplateGalleryProps {
  open: boolean
  onClose: () => void
}

interface TemplateConfig {
  name: string
  description: string
  svgPath: string
  scenarioProperties: Record<string, unknown>
}

const TEMPLATES: TemplateConfig[] = [
  {
    name: 'Smoke',
    description: 'Verify the test plan works',
    svgPath: 'M 10,65 L 190,65',
    scenarioProperties: { executor: 'constant-vus', vus: 1, duration: '30s' },
  },
  {
    name: 'Load',
    description: 'Sustained typical traffic',
    svgPath: 'M 10,70 L 40,25 L 140,25 L 170,70',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '2m', target: 50 },
        { duration: '10m', target: 50 },
        { duration: '2m', target: 0 },
      ],
    },
  },
  {
    name: 'Stress',
    description: 'Find the breaking point',
    svgPath: 'M 10,70 L 10,55 L 55,55 L 55,40 L 100,40 L 100,25 L 145,25 L 145,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '3m', target: 50 },
        { duration: '3m', target: 100 },
        { duration: '3m', target: 200 },
        { duration: '3m', target: 400 },
      ],
    },
  },
  {
    name: 'Spike',
    description: 'Sudden burst and drop',
    svgPath: 'M 10,65 L 60,65 L 80,10 L 120,10 L 140,65 L 190,65',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '1m', target: 10 },
        { duration: '10s', target: 500 },
        { duration: '30s', target: 500 },
        { duration: '10s', target: 10 },
        { duration: '1m', target: 10 },
      ],
    },
  },
  {
    name: 'Soak',
    description: 'Long-duration stability',
    svgPath: 'M 10,40 L 190,40',
    scenarioProperties: { executor: 'constant-vus', vus: 50, duration: '4h' },
  },
  {
    name: 'Breakpoint',
    description: 'Ramp until failure',
    svgPath: 'M 10,70 L 160,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [{ duration: '10m', target: 1000 }],
      startVUs: 10,
      gracefulRampDown: '0s',
    },
  },
  {
    name: 'Trickle Feed',
    description: 'Low constant rate for endurance',
    svgPath: 'M 10,58 L 190,58',
    scenarioProperties: {
      executor: 'constant-arrival-rate',
      rate: 1,
      timeUnit: '1s',
      duration: '1h',
      preAllocatedVUs: 2,
    },
  },
  {
    name: 'Ramp-up',
    description: 'Linear increase only',
    svgPath: 'M 10,70 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [{ duration: '10m', target: 200 }],
    },
  },
  {
    name: 'Step Load',
    description: 'Discrete plateaus',
    svgPath: 'M 10,70 L 10,58 L 46,58 L 46,46 L 82,46 L 82,34 L 118,34 L 118,22 L 154,22 L 154,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '2m', target: 50 },
        { duration: '2m', target: 100 },
        { duration: '2m', target: 150 },
        { duration: '2m', target: 200 },
        { duration: '2m', target: 250 },
        { duration: '2m', target: 300 },
        { duration: '2m', target: 350 },
        { duration: '2m', target: 400 },
        { duration: '2m', target: 450 },
        { duration: '2m', target: 500 },
      ],
    },
  },
]

function LoadCurveSvg({ path, name }: { path: string; name: string }) {
  const styleId = `draw-${name.replace(/[\s-]/g, '_').toLowerCase()}`
  return (
    <svg viewBox="0 0 200 80" className="h-20 w-full" aria-label={`${name} load curve`}>
      <style>{`
        @keyframes ${styleId} {
          0% { stroke-dashoffset: 600; }
          80% { stroke-dashoffset: 0; }
          100% { stroke-dashoffset: 0; }
        }
        .${styleId} {
          stroke-dasharray: 600;
          stroke-dashoffset: 600;
          animation: ${styleId} 4s ease-in-out infinite;
        }
      `}</style>
      <rect width="200" height="80" rx="4" className="fill-slate-950" />
      {/* Grid lines */}
      <line x1="10" y1="70" x2="190" y2="70" className="stroke-slate-800" strokeWidth="0.5" />
      <line x1="10" y1="40" x2="190" y2="40" className="stroke-slate-800" strokeWidth="0.5" />
      <line x1="10" y1="10" x2="190" y2="10" className="stroke-slate-800" strokeWidth="0.5" />
      {/* Load curve */}
      <path
        d={path}
        fill="none"
        className={`${styleId} stroke-teal-500`}
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {name === 'Breakpoint' && (
        <>
          <line x1="155" y1="10" x2="165" y2="20" className="stroke-red-500" strokeWidth="2.5" strokeLinecap="round" />
          <line x1="165" y1="10" x2="155" y2="20" className="stroke-red-500" strokeWidth="2.5" strokeLinecap="round" />
        </>
      )}
    </svg>
  )
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
