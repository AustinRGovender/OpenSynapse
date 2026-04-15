import { useState, useCallback } from 'react'
import { useAIStore } from '../../stores/ai-store'
import { simpleMarkdown } from './markdown'

interface AIInsightsPanelProps {
  runId: string
  /** When true, renders as a full collapsible panel with its own header and response. */
  embedded?: boolean
}

const QUICK_QUESTIONS = [
  'What does this run tell me?',
  'What changed?',
  'What test next?',
]

export function AIInsightsPanel({ runId, embedded = true }: AIInsightsPanelProps) {
  const { analysis, loading, error, analyse, clearError } = useAIStore()
  const [collapsed, setCollapsed] = useState(false)
  const [customQuestion, setCustomQuestion] = useState('')

  const handleQuickQuestion = useCallback(
    async (question: string) => {
      await analyse(runId, question)
    },
    [runId, analyse],
  )

  const handleAsk = useCallback(async () => {
    if (!customQuestion.trim()) return
    const q = customQuestion.trim()
    setCustomQuestion('')
    await analyse(runId, q)
  }, [runId, customQuestion, analyse])

  // Non-embedded mode: just the input section (used inside the AnalyseButton modal)
  if (!embedded) {
    return (
      <div>
        <div className="flex flex-wrap gap-1.5">
          {QUICK_QUESTIONS.map((q) => (
            <button
              key={q}
              onClick={() => handleQuickQuestion(q)}
              disabled={loading}
              className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-xs text-slate-400 hover:bg-slate-700 hover:text-slate-200 disabled:opacity-50"
            >
              {q}
            </button>
          ))}
        </div>
        <div className="mt-2 flex gap-2">
          <input
            type="text"
            value={customQuestion}
            onChange={(e) => setCustomQuestion(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleAsk()}
            placeholder="Ask another question..."
            className="flex-1 rounded border border-slate-700 bg-slate-950 px-2 py-1.5 text-xs text-slate-300 placeholder-slate-600 outline-none focus:border-teal-500"
          />
          <button
            onClick={handleAsk}
            disabled={loading || !customQuestion.trim()}
            className="rounded bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
          >
            Ask
          </button>
        </div>
      </div>
    )
  }

  // Embedded mode: full collapsible panel for RunView
  if (!analysis && !loading) return null

  return (
    <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900">
      {/* Header */}
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex w-full items-center justify-between px-4 py-3 text-left"
      >
        <div className="flex items-center gap-2">
          <svg
            className={`h-3.5 w-3.5 text-slate-500 transition-transform ${collapsed ? '' : 'rotate-90'}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
          </svg>
          <h3 className="text-sm font-medium text-slate-300">AI Insights</h3>
        </div>
        {analysis && (
          <span className="text-xs text-slate-500">
            {analysis.tokens.toLocaleString()} tokens &middot; ${analysis.cost.toFixed(4)}
          </span>
        )}
      </button>

      {!collapsed && (
        <div className="border-t border-slate-800 px-4 py-3">
          {/* Loading state */}
          {loading && (
            <div className="flex items-center gap-2 py-4">
              <div className="h-3 w-3 animate-spin rounded-full border-2 border-teal-500 border-t-transparent" />
              <span className="text-xs text-slate-400">Analysing run data...</span>
            </div>
          )}

          {/* Error */}
          {error && (
            <div className="mb-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2">
              <p className="text-xs text-red-400">{error}</p>
              <button onClick={clearError} className="mt-1 text-xs text-red-500 hover:text-red-300">
                Dismiss
              </button>
            </div>
          )}

          {/* Response */}
          {analysis && (
            <>
              <div
                className="max-w-none text-sm text-slate-300"
                dangerouslySetInnerHTML={{ __html: simpleMarkdown(analysis.response) }}
              />
              <div className="mt-3 flex items-center gap-3 text-xs text-slate-500">
                <span>{analysis.tokens.toLocaleString()} tokens</span>
                <span>${analysis.cost.toFixed(4)}</span>
              </div>
            </>
          )}

          {/* Quick questions and custom input */}
          <div className="mt-4 border-t border-slate-800 pt-3">
            <div className="flex flex-wrap gap-1.5">
              {QUICK_QUESTIONS.map((q) => (
                <button
                  key={q}
                  onClick={() => handleQuickQuestion(q)}
                  disabled={loading}
                  className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-xs text-slate-400 hover:bg-slate-700 hover:text-slate-200 disabled:opacity-50"
                >
                  {q}
                </button>
              ))}
            </div>
            <div className="mt-2 flex gap-2">
              <input
                type="text"
                value={customQuestion}
                onChange={(e) => setCustomQuestion(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleAsk()}
                placeholder="Ask another question..."
                className="flex-1 rounded border border-slate-700 bg-slate-950 px-2 py-1.5 text-xs text-slate-300 placeholder-slate-600 outline-none focus:border-teal-500"
              />
              <button
                onClick={handleAsk}
                disabled={loading || !customQuestion.trim()}
                className="rounded bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 disabled:opacity-50"
              >
                Ask
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
