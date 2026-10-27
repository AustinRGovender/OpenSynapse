import { useEffect, useState, type ReactNode } from 'react'
import type { Environment, Node } from '@opensynapse/api-client'
import { api, unwrap } from '../../api'
import { NODE_TYPES } from '../../config/nodes'
import { findNode, usePlanStore } from '../../stores/plan-store'
import { Button, ConfirmDialog } from '../ui/primitives'

type Props = Record<string, unknown>

const input = 'w-full rounded border border-slate-700 bg-slate-900 px-2 py-1.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-teal-500'
const mono = `${input} font-mono text-xs`

function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-slate-500">{label}</label>
      {children}
      {hint && <p className="mt-1 text-[11px] text-slate-500">{hint}</p>}
    </div>
  )
}

function Num({ value, onChange, min = 0, placeholder }: { value: unknown; onChange: (n: number) => void; min?: number; placeholder?: string }) {
  return <input type="number" min={min} value={value === undefined || value === null ? '' : Number(value)} placeholder={placeholder} onChange={(e) => onChange(e.target.value === '' ? 0 : Number(e.target.value))} className={input} />
}

function Text({ value, onChange, placeholder, monoFont }: { value: unknown; onChange: (s: string) => void; placeholder?: string; monoFont?: boolean }) {
  return <input type="text" value={(value as string) ?? ''} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} className={monoFont ? mono : input} />
}

/** Editable list of {key, value} rows (headers, form fields). */
function KeyValueEditor({ rows, onChange, keyLabel = 'Name', valueLabel = 'Value' }: { rows: { key: string; value: string }[]; onChange: (r: { key: string; value: string }[]) => void; keyLabel?: string; valueLabel?: string }) {
  return (
    <div className="space-y-1">
      {rows.map((r, i) => (
        <div key={i} className="flex gap-1">
          <input aria-label={keyLabel} value={r.key} placeholder={keyLabel} onChange={(e) => onChange(rows.map((x, j) => (j === i ? { ...x, key: e.target.value } : x)))} className={`${mono} w-2/5`} />
          <input aria-label={valueLabel} value={r.value} placeholder={valueLabel} onChange={(e) => onChange(rows.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))} className={`${mono} flex-1`} />
          <button type="button" aria-label="Remove row" onClick={() => onChange(rows.filter((_, j) => j !== i))} className="px-1 text-slate-500 hover:text-red-400">✕</button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...rows, { key: '', value: '' }])} className="text-xs text-teal-400 hover:text-teal-300">+ Add</button>
    </div>
  )
}

/** Editable stages for ramping executors. */
function StagesEditor({ stages, onChange, targetLabel }: { stages: { duration: string; target: number }[]; onChange: (s: { duration: string; target: number }[]) => void; targetLabel: string }) {
  return (
    <div className="space-y-1">
      <div className="flex gap-1 text-[10px] uppercase tracking-wide text-slate-500">
        <span className="w-1/2">Duration</span>
        <span className="flex-1">{targetLabel}</span>
      </div>
      {stages.map((s, i) => (
        <div key={i} className="flex gap-1">
          <input aria-label={`Stage ${i + 1} duration`} value={s.duration} onChange={(e) => onChange(stages.map((x, j) => (j === i ? { ...x, duration: e.target.value } : x)))} placeholder="2m" className={`${mono} w-1/2`} />
          <input aria-label={`Stage ${i + 1} target`} type="number" min={0} value={s.target} onChange={(e) => onChange(stages.map((x, j) => (j === i ? { ...x, target: Number(e.target.value) } : x)))} className={`${mono} flex-1`} />
          <button type="button" aria-label="Remove stage" onClick={() => onChange(stages.filter((_, j) => j !== i))} className="px-1 text-slate-500 hover:text-red-400">✕</button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...stages, { duration: '1m', target: stages[stages.length - 1]?.target ?? 10 }])} className="text-xs text-teal-400 hover:text-teal-300">+ Add stage</button>
    </div>
  )
}

function useEnvironments() {
  const [envs, setEnvs] = useState<Environment[]>([])
  useEffect(() => {
    unwrap(api.GET('/api/v1/environments', { params: { query: { limit: '500' } } })).then((r) => setEnvs(r.items)).catch(() => {})
  }, [])
  return envs
}

/** Plan-level settings: name, description, environment, pass/fail thresholds. */
function PlanFields({ node }: { node: Node }) {
  const plan = usePlanStore((s) => s.plan)!
  const updateMeta = usePlanStore((s) => s.updateMeta)
  const updateNode = usePlanStore((s) => s.updateNode)
  const envs = useEnvironments()
  const props = node.properties as Props
  const thresholds = (props.thresholds as { metric: string; condition: string; abort_on_fail?: boolean }[]) ?? []
  const setThresholds = (t: typeof thresholds) => updateNode(node.id, { properties: { ...props, thresholds: t } })
  return (
    <>
      <Field label="Plan name"><Text value={plan.name} onChange={(v) => updateMeta({ name: v })} /></Field>
      <Field label="Description"><textarea value={plan.description} rows={2} onChange={(e) => updateMeta({ description: e.target.value })} className={input} /></Field>
      <Field label="Tags" hint="Comma-separated">
        <Text value={plan.tags.join(', ')} onChange={(v) => updateMeta({ tags: v.split(',').map((t) => t.trim()).filter(Boolean) })} />
      </Field>
      <Field label="Default environment" hint="Variables like ${BASE_URL} come from here unless you pick another environment when you run.">
        <select value={plan.default_environment_id ?? ''} onChange={(e) => updateMeta({ default_environment_id: e.target.value || null })} className={input}>
          <option value="">None</option>
          {envs.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
        </select>
      </Field>
      <Field label="Pass/fail thresholds" hint={<>k6 expressions, e.g. <code>http_req_failed</code> · <code>rate&lt;0.01</code> or <code>http_req_duration</code> · <code>p(95)&lt;500</code>. “Abort” ends the run as soon as it fails.</>}>
        <div className="space-y-1">
          {thresholds.map((t, i) => (
            <div key={i} className="space-y-1 rounded border border-slate-800 p-1.5">
              <input aria-label="Threshold metric" value={t.metric} onChange={(e) => setThresholds(thresholds.map((x, j) => (j === i ? { ...x, metric: e.target.value } : x)))} placeholder="http_req_duration" className={mono} />
              <div className="flex items-center gap-1">
                <input aria-label="Threshold condition" value={t.condition} onChange={(e) => setThresholds(thresholds.map((x, j) => (j === i ? { ...x, condition: e.target.value } : x)))} placeholder="p(95)<500" className={`${mono} flex-1`} />
                <label className="flex items-center gap-1 text-[11px] text-slate-400">
                  <input type="checkbox" checked={!!t.abort_on_fail} onChange={(e) => setThresholds(thresholds.map((x, j) => (j === i ? { ...x, abort_on_fail: e.target.checked } : x)))} className="accent-teal-500" />
                  Abort
                </label>
                <button type="button" aria-label="Remove threshold" onClick={() => setThresholds(thresholds.filter((_, j) => j !== i))} className="px-1 text-slate-500 hover:text-red-400">✕</button>
              </div>
            </div>
          ))}
          <button type="button" onClick={() => setThresholds([...thresholds, { metric: 'http_req_failed', condition: 'rate<0.01' }])} className="text-xs text-teal-400 hover:text-teal-300">+ Add threshold</button>
        </div>
      </Field>
    </>
  )
}

const EXECUTORS: [string, string][] = [
  ['constant-vus', 'Constant VUs'],
  ['ramping-vus', 'Ramping VUs (stages)'],
  ['constant-arrival-rate', 'Constant arrival rate'],
  ['ramping-arrival-rate', 'Ramping arrival rate'],
  ['shared-iterations', 'Shared iterations'],
  ['per-vu-iterations', 'Per-VU iterations'],
  ['externally-controlled', 'Externally controlled'],
]

function ScenarioFields({ props, set }: { props: Props; set: (k: string, v: unknown) => void }) {
  const ex = (props.executor as string) || 'constant-vus'
  const stages = (props.stages as { duration: string; target: number }[]) ?? []
  const vuBased = ['constant-vus', 'ramping-vus', 'externally-controlled'].includes(ex)
  return (
    <>
      <Field label="Executor" hint={vuBased ? 'VU-based: VUs, RPS limit and duration can all be changed while the test runs.' : 'k6 cannot resize this executor mid-run; live control is limited to pause, stop and RPS.'}>
        <select value={ex} onChange={(e) => set('executor', e.target.value)} className={input}>
          {EXECUTORS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </Field>
      {(ex === 'constant-vus' || ex === 'externally-controlled' || ex === 'shared-iterations' || ex === 'per-vu-iterations') && <Field label="VUs"><Num value={props.vus} min={1} onChange={(n) => set('vus', n)} /></Field>}
      {(ex === 'constant-vus' || ex === 'externally-controlled' || ex === 'constant-arrival-rate') && <Field label="Duration" hint="e.g. 30s, 5m, 1h30m"><Text value={props.duration} onChange={(v) => set('duration', v)} placeholder="1m" /></Field>}
      {ex === 'ramping-vus' && (
        <>
          <Field label="Start VUs"><Num value={props.start_vus} onChange={(n) => set('start_vus', n)} /></Field>
          <Field label="Stages" hint="VUs ramp linearly to each target over its duration."><StagesEditor stages={stages} onChange={(s) => set('stages', s)} targetLabel="Target VUs" /></Field>
        </>
      )}
      {(ex === 'constant-arrival-rate' || ex === 'ramping-arrival-rate') && (
        <>
          {ex === 'constant-arrival-rate' && <Field label="Rate (iterations per time unit)"><Num value={props.rate} min={1} onChange={(n) => set('rate', n)} /></Field>}
          {ex === 'ramping-arrival-rate' && <Field label="Stages"><StagesEditor stages={stages} onChange={(s) => set('stages', s)} targetLabel="Target rate" /></Field>}
          <Field label="Time unit"><Text value={props.time_unit} onChange={(v) => set('time_unit', v)} placeholder="1s" /></Field>
          <Field label="Pre-allocated VUs"><Num value={props.pre_allocated_vus} min={1} onChange={(n) => set('pre_allocated_vus', n)} /></Field>
        </>
      )}
      {(ex === 'externally-controlled' || ex === 'constant-arrival-rate' || ex === 'ramping-arrival-rate') && <Field label="Max VUs"><Num value={props.max_vus} onChange={(n) => set('max_vus', n)} /></Field>}
      {(ex === 'shared-iterations' || ex === 'per-vu-iterations') && <Field label="Iterations"><Num value={props.iterations} min={1} onChange={(n) => set('iterations', n)} /></Field>}
    </>
  )
}

function HttpFields({ props, set, setMany }: { props: Props; set: (k: string, v: unknown) => void; setMany: (p: Props) => void }) {
  const body = (props.body as { type?: string; content?: string }) ?? { type: 'none' }
  const headers = (props.headers as { key: string; value: string }[]) ?? []
  const method = (props.method as string) || 'GET'
  return (
    <>
      <Field label="Method">
        <select value={method} onChange={(e) => set('method', e.target.value)} className={input}>
          {['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].map((m) => <option key={m}>{m}</option>)}
        </select>
      </Field>
      <Field label="URL" hint="Use ${NAME} for environment variables, e.g. ${BASE_URL}/api/items">
        <Text value={props.url} onChange={(v) => set('url', v)} placeholder="${BASE_URL}/path" monoFont />
      </Field>
      <Field label="Headers"><KeyValueEditor rows={headers} onChange={(h) => set('headers', h)} /></Field>
      {!['GET', 'HEAD', 'OPTIONS'].includes(method) && (
        <Field label="Body">
          <select
            value={body.type ?? 'none'}
            onChange={(e) => {
              const type = e.target.value
              const next: Props = { ...props, body: { ...body, type } }
              if (type === 'json' && !headers.some((h) => h.key.toLowerCase() === 'content-type')) next.headers = [...headers, { key: 'Content-Type', value: 'application/json' }]
              setMany(next)
            }}
            className={`${input} mb-1`}
          >
            <option value="none">None</option>
            <option value="json">JSON</option>
            <option value="form">Form (urlencoded)</option>
            <option value="raw">Raw text</option>
          </select>
          {body.type && body.type !== 'none' && (
            <textarea value={body.content ?? ''} rows={6} onChange={(e) => set('body', { ...body, content: e.target.value })} placeholder={body.type === 'json' ? '{"name": "${USER}"}' : body.type === 'form' ? 'user=${USER}&pass=${PASS}' : ''} className={mono} />
          )}
        </Field>
      )}
      <Field label="Timeout" hint="Defaults to k6's 60s"><Text value={props.timeout} onChange={(v) => set('timeout', v)} placeholder="30s" /></Field>
      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input type="checkbox" checked={props.follow_redirects !== false} onChange={(e) => set('follow_redirects', e.target.checked)} className="accent-teal-500" />
        Follow redirects
      </label>
      <p className="text-[11px] text-slate-500">Add Assertion and Timer nodes under this request to check its response and pause afterwards.</p>
    </>
  )
}

function AssertionFields({ props, set }: { props: Props; set: (k: string, v: unknown) => void }) {
  const target = (props.target as string) || 'status'
  const condition = (props.condition as string) || 'equals'
  const numeric = target === 'status' || target === 'response_time'
  const conditions = numeric ? ['equals', 'less_than', 'greater_than'] : ['equals', 'contains', 'matches', 'jsonpath', 'exists']
  return (
    <>
      <Field label="Check">
        <select value={target} onChange={(e) => set('target', e.target.value)} className={input}>
          <option value="status">Status code</option>
          <option value="response_time">Response time (ms)</option>
          <option value="body">Response body</option>
          <option value="header">Response header</option>
        </select>
      </Field>
      {target === 'header' && <Field label="Header name"><Text value={props.header_name} onChange={(v) => set('header_name', v)} placeholder="Content-Type" /></Field>}
      <Field label="Condition">
        <select value={conditions.includes(condition) ? condition : conditions[0]} onChange={(e) => set('condition', e.target.value)} className={input}>
          {conditions.map((c) => <option key={c} value={c}>{c.replace('_', ' ')}</option>)}
        </select>
      </Field>
      {condition !== 'exists' && (
        <Field label="Value" hint={condition === 'jsonpath' ? 'A JSON path that must exist, e.g. $.items' : condition === 'matches' ? 'A regular expression' : undefined}>
          {numeric ? <Num value={props.value} onChange={(n) => set('value', n)} /> : <Text value={props.value} onChange={(v) => set('value', v)} />}
        </Field>
      )}
      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input type="checkbox" checked={!!props.negate} onChange={(e) => set('negate', e.target.checked)} className="accent-teal-500" />
        Negate (pass when the condition is false)
      </label>
    </>
  )
}

function TimerFields({ props, set }: { props: Props; set: (k: string, v: unknown) => void }) {
  const type = (props.timer_type as string) || 'constant'
  return (
    <>
      <Field label="Timer">
        <select value={type} onChange={(e) => set('timer_type', e.target.value)} className={input}>
          <option value="constant">Constant</option>
          <option value="uniform_random">Uniform random</option>
          <option value="gaussian">Gaussian</option>
        </select>
      </Field>
      {type === 'constant' && <Field label="Duration (ms)"><Num value={props.duration_ms} onChange={(n) => set('duration_ms', n)} /></Field>}
      {type === 'uniform_random' && (
        <>
          <Field label="Minimum (ms)"><Num value={props.min_ms} onChange={(n) => set('min_ms', n)} /></Field>
          <Field label="Maximum (ms)"><Num value={props.max_ms} onChange={(n) => set('max_ms', n)} /></Field>
        </>
      )}
      {type === 'gaussian' && (
        <>
          <Field label="Mean (ms)"><Num value={props.mean_ms} onChange={(n) => set('mean_ms', n)} /></Field>
          <Field label="Deviation (ms)"><Num value={props.deviation_ms} onChange={(n) => set('deviation_ms', n)} /></Field>
        </>
      )}
    </>
  )
}

/** Raw JSON editing for anything without a dedicated form. */
function JsonFields({ props, setAll }: { props: Props; setAll: (p: Props) => void }) {
  const [text, setText] = useState(() => JSON.stringify(props, null, 2))
  const [error, setError] = useState<string | null>(null)
  return (
    <Field label="Properties (JSON)">
      <textarea
        value={text}
        rows={10}
        onChange={(e) => setText(e.target.value)}
        onBlur={() => {
          try {
            setAll(JSON.parse(text))
            setError(null)
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Invalid JSON')
          }
        }}
        className={mono}
      />
      {error && <p className="mt-1 text-[11px] text-red-400">{error}</p>}
    </Field>
  )
}

function TypeFields({ node }: { node: Node }) {
  const updateNode = usePlanStore((s) => s.updateNode)
  const plan = usePlanStore((s) => s.plan)!
  const envs = useEnvironments()
  const props = (node.properties as Props) ?? {}
  const set = (k: string, v: unknown) => updateNode(node.id, { properties: { ...props, [k]: v } })
  const setAll = (p: Props) => updateNode(node.id, { properties: p })

  switch (node.type) {
    case 'plan':
      return <PlanFields node={node} />
    case 'scenario':
      return <ScenarioFields props={props} set={set} />
    case 'http':
      return <HttpFields props={props} set={set} setMany={setAll} />
    case 'assertion':
      return <AssertionFields props={props} set={set} />
    case 'timer':
      return <TimerFields props={props} set={set} />
    case 'if-controller':
      return (
        <Field label="Condition (JavaScript)" hint={<>Runs children when true. <code>res</code> is the latest response, e.g. <code>res.status === 200</code> or <code>res.json('items').length &gt; 0</code>.</>}>
          <textarea value={(props.condition as string) ?? ''} rows={3} onChange={(e) => set('condition', e.target.value)} className={mono} />
        </Field>
      )
    case 'else-controller':
      return <p className="text-xs text-slate-500">Runs its children when the If controller directly above it was false.</p>
    case 'loop-controller':
      return (
        <>
          <Field label="Repeat count"><Num value={props.count} onChange={(n) => set('count', n)} /></Field>
          <Field label="…or while (JavaScript)" hint="Leave empty to use the count."><Text value={props.while_condition} onChange={(v) => set('while_condition', v)} monoFont /></Field>
        </>
      )
    case 'transaction-controller':
      return <p className="text-xs text-slate-500">Groups its children; their timings are also reported as one transaction (a k6 group).</p>
    case 'once-only-controller':
      return <p className="text-xs text-slate-500">Children run on each VU’s first iteration only — use it for logging in.</p>
    case 'random-controller':
      return (
        <Field label="Weights" hint={`One per child (${node.children.length}), comma-separated. Leave empty for equal chances.`}>
          <Text value={((props.weights as number[]) ?? []).join(', ')} onChange={(v) => set('weights', v.split(',').map((x) => Number(x.trim())).filter((x) => !Number.isNaN(x) && x > 0))} placeholder="80, 20" />
        </Field>
      )
    case 'code-block':
      return (
        <Field label="JavaScript" hint={<>Runs inside the VU. <code>http</code>, <code>check</code>, <code>sleep</code> and the latest response <code>res</code> are available; set <code>__vars.NAME = …</code> to use <code>{'${NAME}'}</code> in later requests.</>}>
          <textarea value={(props.code as string) ?? ''} rows={10} onChange={(e) => set('code', e.target.value)} className={mono} />
        </Field>
      )
    case 'data-source':
      return (
        <>
          <Field label="Type">
            <select value={(props.source_type as string) || 'csv'} onChange={(e) => set('source_type', e.target.value)} className={input}>
              <option value="csv">CSV file</option>
              <option value="json">JSON file</option>
            </select>
          </Field>
          <Field label="File path" hint="Relative to where k6 runs; mount data files next to the control plane."><Text value={props.path} onChange={(v) => set('path', v)} monoFont /></Field>
          <Field label="Variable name"><Text value={props.variable_name} onChange={(v) => set('variable_name', v)} monoFont /></Field>
        </>
      )
    case 'environment-binding':
      return (
        <Field label="Environment" hint="Informational: the run’s environment provides variables.">
          <select value={(props.environment_id as string) ?? plan.default_environment_id ?? ''} onChange={(e) => set('environment_id', e.target.value)} className={input}>
            <option value="">None</option>
            {envs.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
          </select>
        </Field>
      )
    case 'websocket':
      return (
        <>
          <Field label="URL"><Text value={props.url} onChange={(v) => set('url', v)} placeholder="wss://example.com/socket" monoFont /></Field>
          <Field label="Connect timeout"><Text value={props.connect_timeout} onChange={(v) => set('connect_timeout', v)} placeholder="10s" /></Field>
          <JsonFields key={node.id} props={props} setAll={setAll} />
        </>
      )
    default:
      return <JsonFields key={node.id} props={props} setAll={setAll} />
  }
}

export function PropertyPanel() {
  const plan = usePlanStore((s) => s.plan)
  const selectedNodeId = usePlanStore((s) => s.selectedNodeId)
  const updateNode = usePlanStore((s) => s.updateNode)
  const removeNode = usePlanStore((s) => s.removeNode)
  const [confirm, setConfirm] = useState(false)

  if (!plan) return null
  const node = findNode(plan.root, selectedNodeId ?? plan.root.id) ?? plan.root
  const config = NODE_TYPES[node.type]

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="border-b border-slate-800 p-3">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded bg-slate-800 font-mono text-xs text-slate-400">{config?.icon || '?'}</span>
          <span className="text-xs font-medium uppercase tracking-wider text-slate-500">{config?.label || node.type}</span>
        </div>
        {config?.description && <p className="mt-1 text-[11px] text-slate-500">{config.description}</p>}
      </div>

      <div className="space-y-3 p-3">
        {node.type !== 'plan' && (
          <>
            <Field label="Name"><Text value={node.name} onChange={(v) => updateNode(node.id, { name: v })} /></Field>
            <label className="flex items-center gap-2 text-sm text-slate-300">
              <input type="checkbox" checked={node.enabled} onChange={(e) => updateNode(node.id, { enabled: e.target.checked })} className="accent-teal-500" />
              Enabled
            </label>
          </>
        )}
        <TypeFields key={node.id} node={node} />
      </div>

      {node.type !== 'plan' && (
        <div className="mt-auto border-t border-slate-800 p-3">
          <Button variant="ghost" className="w-full text-red-400" onClick={() => setConfirm(true)}>Delete node</Button>
        </div>
      )}
      <ConfirmDialog
        open={confirm}
        title="Delete node"
        message={<>Delete <b>{node.name}</b>{node.children.length ? ` and its ${node.children.length} child node(s)` : ''}? You can undo with Ctrl+Z.</>}
        onCancel={() => setConfirm(false)}
        onConfirm={() => {
          setConfirm(false)
          removeNode(node.id)
        }}
      />
    </div>
  )
}
