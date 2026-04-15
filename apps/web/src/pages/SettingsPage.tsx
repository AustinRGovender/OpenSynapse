import { useEffect, useState } from 'react'
import { useAIStore, MODEL_OPTIONS, maskApiKey } from '../stores/ai-store'
import type { AIProvider } from '../stores/ai-store'

interface SettingsPageProps {
  onBack: () => void
}

type SettingsTab = 'general' | 'appearance' | 'ai' | 'integrations' | 'storage' | 'about'

const TABS: { key: SettingsTab; label: string }[] = [
  { key: 'general', label: 'General' },
  { key: 'appearance', label: 'Appearance' },
  { key: 'ai', label: 'AI' },
  { key: 'integrations', label: 'Integrations' },
  { key: 'storage', label: 'Storage' },
  { key: 'about', label: 'About' },
]

const PROVIDERS: { value: AIProvider; label: string }[] = [
  { value: 'none', label: 'None (disabled)' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'azure', label: 'Azure OpenAI' },
]

function PlaceholderTab({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center py-24">
      <p className="text-sm text-slate-500">{name} &mdash; Coming soon</p>
    </div>
  )
}

function AISettingsTab() {
  const {
    config,
    apiKey,
    keyValidated,
    testLoading,
    saveLoading,
    error,
    setProvider,
    setModel,
    setEnabled,
    setMonthlyCap,
    setApiKey,
    loadConfig,
    saveConfig,
    testKey,
    clearError,
  } = useAIStore()

  const [showKey, setShowKey] = useState(false)

  useEffect(() => {
    loadConfig()
  }, [loadConfig])

  const models = MODEL_OPTIONS[config.provider]
  const isAzure = config.provider === 'azure'
  const hasProvider = config.provider !== 'none'

  return (
    <div className="max-w-lg space-y-6">
      {/* Error banner */}
      {error && (
        <div className="rounded border border-red-500/30 bg-red-500/10 px-3 py-2">
          <div className="flex items-start justify-between gap-2">
            <p className="text-xs text-red-400">{error}</p>
            <button onClick={clearError} className="shrink-0 text-xs text-red-500 hover:text-red-300">
              x
            </button>
          </div>
        </div>
      )}

      {/* Provider */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-400">Provider</label>
        <select
          value={config.provider}
          onChange={(e) => setProvider(e.target.value as AIProvider)}
          className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 outline-none focus:border-teal-500"
        >
          {PROVIDERS.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
      </div>

      {hasProvider && (
        <>
          {/* API Key */}
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-400">API Key</label>
            <div className="flex items-center gap-2">
              <div className="relative flex-1">
                <input
                  type={showKey ? 'text' : 'password'}
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder={keyValidated ? maskApiKey('configured-key-****') : 'Enter API key...'}
                  className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-2 pr-10 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
                />
                <button
                  type="button"
                  onClick={() => setShowKey(!showKey)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-slate-500 hover:text-slate-300"
                >
                  {showKey ? 'Hide' : 'Show'}
                </button>
              </div>
              <button
                onClick={testKey}
                disabled={testLoading || config.provider === 'none'}
                className="shrink-0 rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50"
              >
                {testLoading ? 'Testing...' : 'Test Key'}
              </button>
            </div>
            {keyValidated === true && (
              <div className="mt-1.5 flex items-center gap-1.5">
                <svg className="h-3.5 w-3.5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
                <span className="text-xs text-green-400">Key validated</span>
              </div>
            )}
            {keyValidated === false && (
              <div className="mt-1.5 flex items-center gap-1.5">
                <svg className="h-3.5 w-3.5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
                <span className="text-xs text-red-400">Key validation failed</span>
              </div>
            )}
          </div>

          {/* Model */}
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-400">Model</label>
            {isAzure ? (
              <input
                type="text"
                value={config.model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="Deployment name..."
                className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
              />
            ) : (
              <select
                value={config.model}
                onChange={(e) => setModel(e.target.value)}
                className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 outline-none focus:border-teal-500"
              >
                {models?.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            )}
          </div>

          {/* Monthly spend cap */}
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-400">
              Monthly spend cap (USD, optional)
            </label>
            <input
              type="number"
              min={0}
              step={1}
              value={config.monthly_cap ?? ''}
              onChange={(e) => {
                const val = e.target.value
                setMonthlyCap(val === '' ? null : Math.max(0, parseFloat(val)))
              }}
              placeholder="No limit"
              className="w-full rounded border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </div>

          {/* Enable/disable */}
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-slate-200">Enable AI analysis</p>
              <p className="text-xs text-slate-500">Allow AI-powered insights for test runs</p>
            </div>
            <button
              onClick={() => setEnabled(!config.enabled)}
              className={`relative h-6 w-11 rounded-full transition-colors ${
                config.enabled ? 'bg-teal-600' : 'bg-slate-700'
              }`}
            >
              <span
                className={`absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white transition-transform ${
                  config.enabled ? 'translate-x-5' : 'translate-x-0'
                }`}
              />
            </button>
          </div>

          {/* Save */}
          <div className="border-t border-slate-800 pt-4">
            <button
              onClick={saveConfig}
              disabled={saveLoading}
              className="rounded bg-teal-600 px-4 py-2 text-sm font-medium text-white hover:bg-teal-500 disabled:opacity-50"
            >
              {saveLoading ? 'Saving...' : 'Save'}
            </button>
          </div>
        </>
      )}
    </div>
  )
}

export function SettingsPage({ onBack }: SettingsPageProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('ai')

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      {/* Header */}
      <header className="flex items-center gap-4 border-b border-slate-800 px-6 py-4">
        <button
          onClick={onBack}
          className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
        >
          &larr; Back
        </button>
        <h1 className="text-lg font-semibold">Settings</h1>
      </header>

      <div className="mx-auto flex max-w-4xl gap-8 px-6 py-6">
        {/* Sidebar tabs */}
        <nav className="w-40 shrink-0 space-y-0.5">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`w-full rounded px-3 py-1.5 text-left text-sm ${
                activeTab === tab.key
                  ? 'bg-teal-600/20 font-medium text-teal-400'
                  : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>

        {/* Tab content */}
        <div className="flex-1">
          {activeTab === 'ai' && <AISettingsTab />}
          {activeTab === 'general' && <PlaceholderTab name="General" />}
          {activeTab === 'appearance' && <PlaceholderTab name="Appearance" />}
          {activeTab === 'integrations' && <PlaceholderTab name="Integrations" />}
          {activeTab === 'storage' && <PlaceholderTab name="Storage" />}
          {activeTab === 'about' && <PlaceholderTab name="About" />}
        </div>
      </div>
    </div>
  )
}
