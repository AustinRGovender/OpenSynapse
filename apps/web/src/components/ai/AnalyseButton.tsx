import { useState, useCallback } from 'react'
import { useAIStore } from '../../stores/ai-store'
import { AIInsightsPanel } from './AIInsightsPanel'
import { simpleMarkdown } from './markdown'

interface AnalyseButtonProps {
  runId: string
}

export function AnalyseButton({ runId }: AnalyseButtonProps) {
  const { config, analysis, loading, error, analyse, clearError } = useAIStore()
  const [modalOpen, setModalOpen] = useState(false)
  const [showInsights, setShowInsights] = useState(false)

  const isConfigured = config.provider !== 'none' && config.enabled

  const defaultPrompt = `Analyse the performance test run ${runId}.\n\nSummarise the key metrics, identify any anomalies or concerning trends, and suggest next steps.`

  const handleClick = useCallback(() => {
    if (analysis) {
      setShowInsights(true)
    } else {
      setModalOpen(true)
    }
  }, [analysis])

  const handleSend = useCallback(async () => {
    setModalOpen(false)
    setShowInsights(true)
    await analyse(runId)
  }, [runId, analyse])

  if (!isConfigured) return null

  return (
    <>
      <button
        onClick={handleClick}
        disabled={loading}
        className={`rounded border px-3 py-1 text-xs font-medium transition-colors ${
          analysis
            ? 'border-teal-600 bg-teal-600/20 text-teal-400 hover:bg-teal-600/30'
            : 'border-slate-700 bg-slate-800 text-slate-300 hover:bg-slate-700'
        } disabled:opacity-50`}
      >
        {loading ? 'Analysing...' : analysis ? 'AI Insights' : 'Analyse with AI'}
      </button>

      {/* Prompt preview modal */}
      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
          <div className="mx-4 w-full max-w-lg rounded-lg border border-slate-700 bg-slate-900 shadow-xl">
            <div className="border-b border-slate-800 px-4 py-3">
              <h3 className="text-sm font-semibold text-slate-200">AI Analysis Preview</h3>
              <p className="mt-1 text-xs text-slate-500">
                This prompt will be sent to {config.provider} ({config.model})
              </p>
            </div>
            <div className="px-4 py-3">
              <textarea
                readOnly
                value={defaultPrompt}
                className="h-32 w-full resize-none rounded border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs text-slate-300 outline-none"
              />
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-800 px-4 py-3">
              <button
                onClick={() => setModalOpen(false)}
                className="rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-slate-700"
              >
                Cancel
              </button>
              <button
                onClick={handleSend}
                className="rounded bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500"
              >
                Send
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Response modal */}
      {showInsights && analysis && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
          <div className="mx-4 flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border border-slate-700 bg-slate-900 shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
              <h3 className="text-sm font-semibold text-slate-200">AI Insights</h3>
              <button
                onClick={() => setShowInsights(false)}
                className="text-slate-500 hover:text-slate-300"
              >
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="flex-1 overflow-auto px-4 py-3">
              {error && (
                <div className="mb-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2">
                  <p className="text-xs text-red-400">{error}</p>
                  <button onClick={clearError} className="mt-1 text-xs text-red-500 hover:text-red-300">
                    Dismiss
                  </button>
                </div>
              )}
              <div
                className="prose-sm prose-invert max-w-none text-sm text-slate-300"
                dangerouslySetInnerHTML={{ __html: simpleMarkdown(analysis.response) }}
              />
              <div className="mt-3 flex items-center gap-3 text-xs text-slate-500">
                <span>{analysis.tokens.toLocaleString()} tokens</span>
                <span>${analysis.cost.toFixed(4)}</span>
              </div>
            </div>
            <div className="border-t border-slate-800 px-4 py-3">
              <AIInsightsPanel runId={runId} embedded={false} />
            </div>
          </div>
        </div>
      )}
    </>
  )
}
