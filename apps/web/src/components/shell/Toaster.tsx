import { useUIStore } from '../../stores/ui-store'

const tones = {
  info: 'border-slate-700 bg-slate-900 text-slate-200',
  success: 'border-emerald-500/40 bg-emerald-950 text-emerald-200',
  error: 'border-red-500/40 bg-red-950 text-red-200',
}

/** Bottom-right notifications; errors stay longer than confirmations. */
export function Toaster() {
  const toasts = useUIStore((s) => s.toasts)
  const dismiss = useUIStore((s) => s.dismiss)
  return (
    <div aria-live="polite" className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-80 flex-col gap-2">
      {toasts.map((t) => (
        <div key={t.id} role={t.kind === 'error' ? 'alert' : 'status'} className={`pointer-events-auto flex items-start gap-2 rounded-lg border px-3 py-2 text-sm shadow-lg ${tones[t.kind]}`}>
          <span className="flex-1">{t.message}</span>
          <button type="button" aria-label="Dismiss" onClick={() => dismiss(t.id)} className="text-xs opacity-60 hover:opacity-100">✕</button>
        </div>
      ))}
    </div>
  )
}
