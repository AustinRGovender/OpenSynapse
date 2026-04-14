import { usePlanStore, findNode } from '../../stores/plan-store'
import { NODE_TYPES } from '../../config/nodes'
import type { Node } from '@opensynapse/api-client'

export function PropertyPanel() {
  const { plan, selectedNodeId, updateNode, removeNode } = usePlanStore()

  if (!plan || !selectedNodeId) {
    return (
      <div className="flex h-full items-center justify-center p-4 text-sm text-slate-500">
        Select a node to edit its properties
      </div>
    )
  }

  const node = findNode(plan.root, selectedNodeId)
  if (!node) return null

  const config = NODE_TYPES[node.type]

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      {/* Header */}
      <div className="border-b border-slate-800 p-3">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded bg-slate-800 font-mono text-xs text-slate-400">
            {config?.icon || '?'}
          </span>
          <span className="text-xs font-medium uppercase tracking-wider text-slate-500">
            {config?.label || node.type}
          </span>
        </div>
      </div>

      {/* Common fields */}
      <div className="space-y-3 p-3">
        <Field label="Name">
          <input
            type="text"
            value={node.name}
            onChange={(e) => updateNode(node.id, { name: e.target.value })}
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
          />
        </Field>

        <Field label="Enabled">
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={node.enabled}
              onChange={(e) => updateNode(node.id, { enabled: e.target.checked })}
              className="h-4 w-4 rounded border-slate-600 accent-teal-500"
            />
            <span className="text-sm text-slate-300">{node.enabled ? 'Yes' : 'No'}</span>
          </label>
        </Field>

        {/* Type-specific properties */}
        <TypeSpecificFields node={node} />
      </div>

      {/* Actions */}
      {node.type !== 'plan' && (
        <div className="mt-auto border-t border-slate-800 p-3">
          <button
            className="w-full rounded bg-red-500/10 px-3 py-1.5 text-xs font-medium text-red-400 hover:bg-red-500/20"
            onClick={() => {
              if (confirm(`Delete "${node.name}"?`)) {
                removeNode(node.id)
              }
            }}
          >
            Delete node
          </button>
        </div>
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-slate-500">{label}</label>
      {children}
    </div>
  )
}

function TypeSpecificFields({ node }: { node: Node }) {
  const { updateNode } = usePlanStore()
  const props = node.properties as Record<string, unknown>

  const updateProp = (key: string, value: unknown) => {
    updateNode(node.id, {
      properties: { ...props, [key]: value },
    })
  }

  switch (node.type) {
    case 'http':
      return (
        <>
          <Field label="Method">
            <select
              value={(props.method as string) || 'GET'}
              onChange={(e) => updateProp('method', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              {['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].map((m) => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </Field>
          <Field label="URL">
            <input
              type="text"
              value={(props.url as string) || ''}
              onChange={(e) => updateProp('url', e.target.value)}
              placeholder="https://example.com/api/..."
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </Field>
          <Field label="Timeout">
            <input
              type="text"
              value={(props.timeout as string) || ''}
              onChange={(e) => updateProp('timeout', e.target.value)}
              placeholder="30s"
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </Field>
        </>
      )

    case 'scenario':
      return (
        <>
          <Field label="Executor">
            <select
              value={(props.executor as string) || 'constant-vus'}
              onChange={(e) => updateProp('executor', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              {['constant-vus', 'ramping-vus', 'constant-arrival-rate', 'ramping-arrival-rate',
                'shared-iterations', 'per-vu-iterations', 'externally-controlled'].map((e) => (
                <option key={e} value={e}>{e}</option>
              ))}
            </select>
          </Field>
          <Field label="VUs">
            <input
              type="number"
              value={(props.vus as number) || 0}
              onChange={(e) => updateProp('vus', parseInt(e.target.value) || 0)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </Field>
          <Field label="Duration">
            <input
              type="text"
              value={(props.duration as string) || ''}
              onChange={(e) => updateProp('duration', e.target.value)}
              placeholder="1m"
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </Field>
        </>
      )

    case 'if-controller':
      return (
        <Field label="Condition (JavaScript)">
          <textarea
            value={(props.condition as string) || ''}
            onChange={(e) => updateProp('condition', e.target.value)}
            rows={3}
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 font-mono text-sm text-slate-200 outline-none focus:border-teal-500"
          />
        </Field>
      )

    case 'loop-controller':
      return (
        <>
          <Field label="Count">
            <input
              type="number"
              value={(props.count as number) || 0}
              onChange={(e) => updateProp('count', parseInt(e.target.value) || 0)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </Field>
          <Field label="While Condition (optional)">
            <input
              type="text"
              value={(props.while_condition as string) || ''}
              onChange={(e) => updateProp('while_condition', e.target.value)}
              placeholder="Leave empty to use count"
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500"
            />
          </Field>
        </>
      )

    case 'timer':
      return (
        <>
          <Field label="Type">
            <select
              value={(props.timer_type as string) || 'constant'}
              onChange={(e) => updateProp('timer_type', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              <option value="constant">Constant</option>
              <option value="uniform_random">Uniform Random</option>
              <option value="gaussian">Gaussian</option>
            </select>
          </Field>
          <Field label="Duration (ms)">
            <input
              type="number"
              value={(props.duration_ms as number) || 0}
              onChange={(e) => updateProp('duration_ms', parseInt(e.target.value) || 0)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </Field>
        </>
      )

    case 'assertion':
      return (
        <>
          <Field label="Target">
            <select
              value={(props.target as string) || 'status'}
              onChange={(e) => updateProp('target', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              <option value="status">Status Code</option>
              <option value="body">Response Body</option>
              <option value="header">Header</option>
              <option value="response_time">Response Time</option>
            </select>
          </Field>
          <Field label="Condition">
            <select
              value={(props.condition as string) || 'equals'}
              onChange={(e) => updateProp('condition', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            >
              {['equals', 'contains', 'matches', 'jsonpath', 'xpath', 'less_than', 'greater_than'].map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </Field>
          <Field label="Value">
            <input
              type="text"
              value={String(props.value ?? '')}
              onChange={(e) => updateProp('value', e.target.value)}
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 outline-none focus:border-teal-500"
            />
          </Field>
        </>
      )

    case 'code-block':
      return (
        <Field label="JavaScript Code">
          <textarea
            value={(props.code as string) || ''}
            onChange={(e) => updateProp('code', e.target.value)}
            rows={8}
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 font-mono text-xs text-slate-200 outline-none focus:border-teal-500"
          />
        </Field>
      )

    default:
      return (
        <div className="text-xs text-slate-500">
          <p>Properties (JSON):</p>
          <pre className="mt-1 max-h-40 overflow-auto rounded bg-slate-800 p-2 text-xs">
            {JSON.stringify(props, null, 2)}
          </pre>
        </div>
      )
  }
}
