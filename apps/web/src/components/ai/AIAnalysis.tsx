import { useEffect, useState } from 'react'
import { errorMessage } from '../../api'
import { PROVIDER_LABELS, STANDARD_QUESTIONS, useAIStore } from '../../stores/ai-store'
import { Button, Modal, Spinner } from '../ui/primitives'
import { simpleMarkdown } from './markdown'

/**
 * "Analyse with AI" for a run (one ID) or a comparison (several). Nothing is
 * sent until the user has seen the exact prompt and pressed Send (F-09).
 */
export function AIAnalysisButton({ runIds }: { runIds: string[] }) {
  const config = useAIStore((s) => s.config)
  const loadConfig = useAIStore((s) => s.loadConfig)
  const preview = useAIStore((s) => s.preview)
  const analyse = useAIStore((s) => s.analyse)
  const [open, setOpen] = useState(false)
  const [question, setQuestion] = useState(runIds.length > 1 ? STANDARD_QUESTIONS.compare : STANDARD_QUESTIONS.run)
  const [custom, setCustom] = useState('')
  const [prompt, setPrompt] = useState<string | null>(null)
  const [busy, setBusy] = useState<'preview' | 'send' | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (config === null) loadConfig()
  }, [config, loadConfig])

  if (!config?.enabled || !config.provider) return null
  const providerName = PROVIDER_LABELS[config.provider] ?? config.provider
  const questions = runIds.length > 1 ? [STANDARD_QUESTIONS.compare, STANDARD_QUESTIONS.next] : [STANDARD_QUESTIONS.run, STANDARD_QUESTIONS.next]
  const asked = custom.trim() || question

  async function doPreview() {
    setBusy('preview')
    setError(null)
    try {
      setPrompt(await preview(runIds, asked))
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  async function send() {
    setBusy('send')
    setError(null)
    try {
      await analyse(runIds, asked)
      setOpen(false)
      setPrompt(null)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>Analyse with AI</Button>
      <Modal
        open={open}
        onClose={() => { setOpen(false); setPrompt(null) }}
        title="Analyse with AI"
        width="max-w-2xl"
        footer={
          prompt === null ? (
            <Button variant="primary" onClick={doPreview} disabled={busy !== null}>{busy === 'preview' ? 'Preparing…' : 'Preview prompt'}</Button>
          ) : (
            <>
              <Button onClick={() => setPrompt(null)}>Back</Button>
              <Button variant="primary" onClick={send} disabled={busy !== null}>{busy === 'send' ? 'Waiting for the answer…' : `Send to ${providerName}`}</Button>
            </>
          )
        }
      >
        {prompt === null ? (
          <div className="space-y-3">
            <p className="text-sm text-slate-400">Choose a question. You will see exactly what is sent before anything leaves this machine.</p>
            <div className="flex flex-wrap gap-2">
              {questions.map((q) => (
                <button key={q} type="button" onClick={() => { setQuestion(q); setCustom('') }} className={`rounded-full border px-3 py-1 text-xs ${q === question && !custom ? 'border-teal-500 bg-teal-500/10 text-teal-300' : 'border-slate-700 text-slate-300 hover:bg-slate-800'}`}>
                  {q}
                </button>
              ))}
            </div>
            <input value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="…or ask your own question" className="w-full rounded-md border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-sm text-slate-100 placeholder-slate-500 focus:border-teal-500 focus:outline-none" />
          </div>
        ) : (
          <div>
            <p className="mb-2 text-xs text-slate-400">
              This exact text will be sent to <b>{providerName}</b> ({config.model || 'default model'}). It contains summary statistics only — no raw samples, headers, or environment secrets.
            </p>
            <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-slate-800 bg-slate-900 p-3 font-mono text-xs text-slate-300">{prompt}</pre>
          </div>
        )}
        {error && <p role="alert" className="mt-3 text-sm text-red-400">{error}</p>}
      </Modal>
    </>
  )
}

/** Renders analyses already obtained for these runs. */
export function AIInsights({ runIds }: { runIds: string[] }) {
  const analyses = useAIStore((s) => s.analyses[runIds.join(',')])
  if (!analyses?.length) return null
  return (
    <section className="space-y-3">
      {analyses.map((a) => (
        <div key={a.id} className="rounded-lg border border-teal-500/30 bg-teal-500/5 p-4">
          <div className="mb-2 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-teal-300">AI · {a.question}</h3>
            <span className="text-[11px] text-slate-500">
              {(a.input_tokens + a.output_tokens).toLocaleString()} tokens · ${a.cost_usd.toFixed(4)} · {a.model}
            </span>
          </div>
          <div className="text-sm text-slate-300" dangerouslySetInnerHTML={{ __html: simpleMarkdown(a.response) }} />
        </div>
      ))}
    </section>
  )
}

export function AIBusy() {
  return <Spinner label="Asking the AI provider…" />
}
