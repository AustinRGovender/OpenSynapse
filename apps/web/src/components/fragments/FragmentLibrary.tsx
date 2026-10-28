import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Fragment, Node } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../../api'
import { NODE_TYPES } from '../../config/nodes'
import { canEdit, useAuthStore } from '../../stores/auth-store'
import { findNode, usePlanStore } from '../../stores/plan-store'
import { useToast } from '../../stores/ui-store'
import { Badge, Button, Field, Modal, TextInput } from '../ui/primitives'

function cloneWithNewIds(node: Node): Node {
  return { ...node, id: crypto.randomUUID(), children: node.children.map(cloneWithNewIds) }
}

/** Replaces ${NAME} for each bound variable throughout a subtree. */
function bind(node: Node, values: Record<string, string>): Node {
  let json = JSON.stringify(node)
  for (const [name, value] of Object.entries(values)) {
    if (value === '') continue
    // JSON.stringify(value).slice(1, -1) escapes quotes and backslashes for the JSON text.
    json = json.split('${' + name + '}').join(JSON.stringify(value).slice(1, -1))
  }
  return JSON.parse(json)
}

function nodeTypes(n: Node, acc = new Set<string>()): Set<string> {
  acc.add(n.type)
  n.children.forEach((c) => nodeTypes(c, acc))
  return acc
}

/** Fragments sidebar (F-10): reusable subtrees with variable bindings. */
export function FragmentLibrary({ onClose }: { onClose: () => void }) {
  const toast = useToast()
  const editable = canEdit(useAuthStore((s) => s.user))
  const plan = usePlanStore((s) => s.plan)
  const selectedNodeId = usePlanStore((s) => s.selectedNodeId)
  const addChild = usePlanStore((s) => s.addChild)
  const [fragments, setFragments] = useState<Fragment[]>([])
  const [query, setQuery] = useState('')
  const [inserting, setInserting] = useState<Fragment | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [saveName, setSaveName] = useState('')

  const load = useCallback(() => unwrap(api.GET('/api/v1/fragments')).then((r) => setFragments(r.items)).catch((e) => toast(errorMessage(e), 'error')), [toast])
  useEffect(() => {
    load()
  }, [load])

  const filtered = useMemo(() => {
    const q = query.toLowerCase()
    return fragments.filter((f) => !q || f.name.toLowerCase().includes(q) || f.tags.some((t) => t.includes(q)))
  }, [fragments, query])

  // Insert under the selected node if it can have children, else under the first scenario.
  function target(): Node | null {
    if (!plan) return null
    const sel = selectedNodeId ? findNode(plan.root, selectedNodeId) : null
    if (sel && NODE_TYPES[sel.type]?.canHaveChildren && sel.type !== 'plan') return sel
    return plan.root.children.find((c) => c.type === 'scenario') ?? plan.root
  }

  function startInsert(f: Fragment) {
    if (!f.bindings?.length) return insert(f, {})
    setValues(Object.fromEntries(f.bindings.map((b) => [b.name, b.default_value ?? ''])))
    setInserting(f)
  }

  function insert(f: Fragment, vals: Record<string, string>) {
    const t = target()
    if (!t) return
    addChild(t.id, cloneWithNewIds(bind(f.node_subtree, vals)))
    setInserting(null)
    toast(`Inserted “${f.name}” into ${t.name}`, 'success')
  }

  async function saveSelected() {
    const sel = plan && selectedNodeId ? findNode(plan.root, selectedNodeId) : null
    if (!sel || !saveName.trim()) return
    // Variables used in the subtree become bindings the next user fills in.
    const names = [...new Set([...JSON.stringify(sel).matchAll(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g)].map((m) => m[1]))]
    try {
      await unwrap(api.POST('/api/v1/fragments', { body: { name: saveName.trim(), node_subtree: sel, bindings: names.map((n) => ({ name: n, description: '', required: false })), tags: ['user'] } }))
      toast(`Saved fragment “${saveName}”`, 'success')
      setSaving(false)
      setSaveName('')
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  async function remove(f: Fragment) {
    try {
      await unwrap(api.DELETE('/api/v1/fragments/{id}', { params: { path: { id: f.id } } }))
      load()
    } catch (e) {
      toast(errorMessage(e), 'error')
    }
  }

  const selected = plan && selectedNodeId ? findNode(plan.root, selectedNodeId) : null
  const missingRequired = inserting?.bindings?.some((b) => b.required && !values[b.name])

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-slate-800 px-3 py-2">
        <span className="text-sm font-medium text-slate-200">Fragments</span>
        <button type="button" onClick={onClose} aria-label="Close fragments" className="text-slate-500 hover:text-slate-300">✕</button>
      </div>
      <div className="space-y-2 border-b border-slate-800 p-2">
        <TextInput value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search fragments" aria-label="Search fragments" />
        {editable && selected && selected.type !== 'plan' && (
          <Button size="sm" className="w-full" onClick={() => setSaving(true)}>Save “{selected.name}” as fragment</Button>
        )}
      </div>
      <ul className="flex-1 space-y-1.5 overflow-y-auto p-2">
        {filtered.map((f) => (
          <li key={f.id} className="rounded-md border border-slate-800 bg-slate-900 p-2">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-xs font-medium text-slate-200">{f.name}</span>
              {f.built_in && <Badge tone="accent">Built-in</Badge>}
            </div>
            {f.description && <p className="mt-0.5 text-[11px] text-slate-400">{f.description}</p>}
            <p className="mt-1 font-mono text-[10px] text-slate-500">{[...nodeTypes(f.node_subtree)].map((t) => NODE_TYPES[t]?.icon ?? t).join(' ')}</p>
            {editable && (
              <div className="mt-1.5 flex gap-1">
                <Button size="sm" variant="primary" onClick={() => startInsert(f)}>Insert</Button>
                {!f.built_in && <Button size="sm" variant="ghost" onClick={() => remove(f)}>Delete</Button>}
              </div>
            )}
          </li>
        ))}
        {filtered.length === 0 && <li className="p-2 text-xs text-slate-500">No fragments match.</li>}
      </ul>

      <Modal
        open={inserting !== null}
        onClose={() => setInserting(null)}
        title={`Insert ${inserting?.name ?? ''}`}
        footer={<><Button onClick={() => setInserting(null)}>Cancel</Button><Button variant="primary" disabled={missingRequired} onClick={() => inserting && insert(inserting, values)}>Insert</Button></>}
      >
        <p className="mb-3 text-xs text-slate-400">Fill in the fragment’s variables. Leave one empty to keep it as <code>{'${NAME}'}</code> and supply it from an environment instead.</p>
        <div className="space-y-3">
          {inserting?.bindings?.map((b) => (
            <Field key={b.name} label={`${b.name}${b.required ? ' *' : ''}`} hint={b.description}>
              <TextInput value={values[b.name] ?? ''} onChange={(e) => setValues({ ...values, [b.name]: e.target.value })} />
            </Field>
          ))}
        </div>
      </Modal>

      <Modal open={saving} onClose={() => setSaving(false)} title="Save as fragment" footer={<><Button onClick={() => setSaving(false)}>Cancel</Button><Button variant="primary" disabled={!saveName.trim()} onClick={saveSelected}>Save</Button></>}>
        <Field label="Fragment name" hint="${VARIABLES} used inside become bindings filled in when the fragment is inserted.">
          <TextInput value={saveName} onChange={(e) => setSaveName(e.target.value)} autoFocus />
        </Field>
      </Modal>
    </div>
  )
}
