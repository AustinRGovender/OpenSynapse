import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  /** Changing this key resets the boundary (e.g. on navigation). */
  resetKey?: string
}

interface State {
  error: Error | null
}

/**
 * Catches render errors so a bug in one page shows a recoverable message
 * instead of a blank screen. Details stay collapsed (no stack traces up front).
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('OpenSynapse UI error', error, info.componentStack)
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div role="alert" className="mx-auto mt-16 max-w-lg rounded-lg border border-red-500/30 bg-red-500/5 p-6">
        <h2 className="text-base font-semibold text-red-300">Something went wrong on this page</h2>
        <p className="mt-2 text-sm text-slate-300">
          The rest of OpenSynapse still works. Try again, or go back and reopen the page.
        </p>
        <div className="mt-4 flex gap-2">
          <button type="button" onClick={() => this.setState({ error: null })} className="rounded-md bg-teal-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-teal-500">
            Try again
          </button>
          <button type="button" onClick={() => window.location.reload()} className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800">
            Reload
          </button>
        </div>
        <details className="mt-4 text-xs text-slate-500">
          <summary className="cursor-pointer">Technical details</summary>
          <pre className="mt-2 whitespace-pre-wrap">{this.state.error.message}</pre>
        </details>
      </div>
    )
  }
}
