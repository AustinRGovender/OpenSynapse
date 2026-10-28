import { useRef, useState, type KeyboardEvent } from 'react'
import { api, errorMessage, unwrap } from '../../api'
import { useToast } from '../../stores/ui-store'
import { navigate } from '../../utils/navigate'
import { Modal } from '../ui/primitives'
import { LoadCurveSvg } from './LoadCurveSvg'
import { buildPlanRoot, type TemplateConfig, TEMPLATES } from './template-data'

interface TemplateGalleryProps {
  open: boolean
  onClose: () => void
  /** Environment new plans start with (the bundled test APIs by default). */
  defaultEnvironmentId?: string | null
}

export function TemplateGallery({ open, onClose, defaultEnvironmentId }: TemplateGalleryProps) {
  const [creating, setCreating] = useState<string | null>(null)
  const toast = useToast()
  const gridRef = useRef<HTMLDivElement>(null)

  async function create(template: TemplateConfig | null) {
    setCreating(template?.name ?? 'blank')
    try {
      const root = buildPlanRoot(template)
      const plan = await unwrap(
        api.POST('/api/v1/plans', {
          body: {
            name: root.name,
            description: template?.description ?? '',
            tags: template ? [template.name.toLowerCase().replace(/\s+/g, '-')] : [],
            root,
            default_environment_id: defaultEnvironmentId ?? null,
          },
        }),
      )
      onClose()
      navigate(`#/plans/${plan.id}`)
    } catch (e) {
      toast(`Could not create the plan: ${errorMessage(e)}`, 'error')
    } finally {
      setCreating(null)
    }
  }

  // Arrow keys move between cards (F-01: the gallery is keyboard-navigable).
  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const cards = Array.from(gridRef.current?.querySelectorAll<HTMLButtonElement>('[data-card]') ?? [])
    const i = cards.indexOf(document.activeElement as HTMLButtonElement)
    if (i < 0) return
    const cols = window.innerWidth >= 1024 ? 4 : window.innerWidth >= 640 ? 2 : 1
    const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[e.key]
    if (step === undefined) return
    e.preventDefault()
    cards[Math.max(0, Math.min(cards.length - 1, i + step))]?.focus()
  }

  return (
    <Modal open={open} onClose={onClose} title="New test" width="max-w-5xl">
      <p className="mb-4 text-sm text-slate-400">
        Pick a load profile. The plan starts with a sample request to the bundled Echo test API, so you can run it straight
        away and then point it at your own system.
      </p>
      <div ref={gridRef} onKeyDown={onKeyDown} className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <button
          type="button"
          data-card
          onClick={() => create(null)}
          disabled={creating !== null}
          className="flex flex-col items-start justify-center rounded-lg border border-dashed border-slate-700 p-4 text-left hover:border-slate-500 focus:border-teal-500 disabled:opacity-50"
        >
          <span className="text-sm font-semibold text-slate-100">{creating === 'blank' ? 'Creating…' : 'Blank plan'}</span>
          <span className="mt-1 text-xs text-slate-400">1 VU for 30 seconds; build the rest yourself</span>
        </button>
        {TEMPLATES.map((t) => (
          <button
            key={t.name}
            type="button"
            data-card
            onClick={() => create(t)}
            disabled={creating !== null}
            className="flex flex-col overflow-hidden rounded-lg border border-slate-800 bg-slate-900 text-left transition-colors hover:border-slate-600 focus:border-teal-500 disabled:opacity-50"
          >
            <div className="border-b border-slate-800 px-3 pb-2 pt-3">
              <LoadCurveSvg path={t.svgPath} name={t.name} />
            </div>
            <div className="flex flex-1 flex-col p-3">
              <span className="text-sm font-semibold text-slate-100">{creating === t.name ? 'Creating…' : t.name}</span>
              <span className="mt-0.5 text-xs text-slate-400">{t.description}</span>
            </div>
          </button>
        ))}
      </div>
    </Modal>
  )
}
