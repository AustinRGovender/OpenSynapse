import { useState, type DragEvent } from 'react'
import type { Node } from '@opensynapse/api-client'
import { AUXILIARY_TYPES, CONTROLLER_TYPES, createNode, NODE_TYPES, SAMPLER_TYPES } from '../../config/nodes'
import { findNode, findParent, usePlanStore } from '../../stores/plan-store'

const DRAG_TYPE = 'application/x-opensynapse-node'

type DropZone = 'before' | 'inside' | 'after'

function matches(node: Node, q: string): boolean {
  if (!q) return true
  const s = q.toLowerCase()
  // F-02: search by name, type and property values.
  return node.name.toLowerCase().includes(s) || node.type.includes(s) || JSON.stringify(node.properties ?? {}).toLowerCase().includes(s)
}

function subtreeMatches(node: Node, q: string): boolean {
  return matches(node, q) || node.children.some((c) => subtreeMatches(c, q))
}

function TreeItem({ node, depth, query }: { node: Node; depth: number; query: string }) {
  const [expanded, setExpanded] = useState(true)
  const [drop, setDrop] = useState<DropZone | null>(null)
  const selectedNodeId = usePlanStore((s) => s.selectedNodeId)
  const setSelectedNode = usePlanStore((s) => s.setSelectedNode)
  const moveNode = usePlanStore((s) => s.moveNode)
  const config = NODE_TYPES[node.type]
  const isRoot = node.type === 'plan'
  const isSelected = selectedNodeId === node.id || (!selectedNodeId && isRoot)

  if (query && !subtreeMatches(node, query)) return null

  function zoneFor(e: DragEvent<HTMLDivElement>): DropZone {
    const r = e.currentTarget.getBoundingClientRect()
    const y = (e.clientY - r.top) / r.height
    if (isRoot) return 'inside'
    if (y < 0.25) return 'before'
    if (y > 0.75 || !config?.canHaveChildren) return 'after'
    return 'inside'
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    const zone = zoneFor(e)
    setDrop(null)
    const draggedId = e.dataTransfer.getData(DRAG_TYPE)
    const { plan } = usePlanStore.getState()
    if (!draggedId || !plan || draggedId === node.id) return
    if (zone === 'inside') {
      moveNode(draggedId, node.id, node.children.length)
      return
    }
    const parent = findParent(plan.root, node.id)
    if (!parent) return
    const siblings = parent.children.filter((c) => c.id !== draggedId)
    const idx = siblings.findIndex((c) => c.id === node.id)
    moveNode(draggedId, parent.id, zone === 'before' ? idx : idx + 1)
  }

  const highlight = query && matches(node, query)
  return (
    <div>
      <div
        role="treeitem"
        aria-selected={isSelected}
        aria-expanded={node.children.length ? expanded : undefined}
        tabIndex={0}
        draggable={!isRoot}
        onDragStart={(e) => {
          e.dataTransfer.setData(DRAG_TYPE, node.id)
          e.dataTransfer.effectAllowed = 'move'
        }}
        onDragOver={(e) => {
          if (!e.dataTransfer.types.includes(DRAG_TYPE)) return
          e.preventDefault()
          setDrop(zoneFor(e))
        }}
        onDragLeave={() => setDrop(null)}
        onDrop={onDrop}
        onClick={() => setSelectedNode(isRoot ? null : node.id)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') setSelectedNode(isRoot ? null : node.id)
          if (e.key === 'ArrowRight') setExpanded(true)
          if (e.key === 'ArrowLeft') setExpanded(false)
        }}
        className={`relative flex cursor-pointer items-center gap-1.5 rounded px-2 py-1 text-sm ${isSelected ? 'bg-teal-500/20 text-teal-200' : 'text-slate-300 hover:bg-slate-800'} ${!node.enabled ? 'opacity-50' : ''} ${drop === 'inside' ? 'ring-1 ring-teal-500' : ''}`}
        style={{ paddingLeft: depth * 14 + 6 }}
      >
        {drop === 'before' && <span className="absolute inset-x-0 top-0 h-0.5 bg-teal-500" />}
        {drop === 'after' && <span className="absolute inset-x-0 bottom-0 h-0.5 bg-teal-500" />}
        {node.children.length ? (
          <button type="button" aria-label={expanded ? 'Collapse' : 'Expand'} onClick={(e) => { e.stopPropagation(); setExpanded(!expanded) }} className="flex h-4 w-4 items-center justify-center text-xs text-slate-500">
            {expanded ? '▾' : '▸'}
          </button>
        ) : (
          <span className="h-4 w-4" />
        )}
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-slate-800 font-mono text-[11px] text-slate-400">{config?.icon ?? '•'}</span>
        <span className={`truncate ${highlight ? 'font-semibold text-teal-300' : ''}`}>{node.name}</span>
        <span className="ml-auto shrink-0 text-[10px] text-slate-600">{config?.label ?? node.type}</span>
      </div>
      {expanded && node.children.length > 0 && (
        <div role="group">
          {node.children.map((c) => <TreeItem key={c.id} node={c} depth={depth + 1} query={query} />)}
        </div>
      )}
    </div>
  )
}

/** Left pane: the plan as a tree; drag to move, search to filter. */
export function NodeTree() {
  const plan = usePlanStore((s) => s.plan)
  const addChild = usePlanStore((s) => s.addChild)
  const [search, setSearch] = useState('')
  const [menu, setMenu] = useState(false)
  if (!plan) return null

  function add(type: string) {
    const { selectedNodeId } = usePlanStore.getState()
    const selected = selectedNodeId ? findNode(plan!.root, selectedNodeId) : plan!.root
    const node = createNode(type)
    if (selected && NODE_TYPES[selected.type]?.canHaveChildren) {
      addChild(selected.id, node)
    } else if (selected) {
      // Leaf selected (assertion, timer…): add next to it.
      const parent = findParent(plan!.root, selected.id) ?? plan!.root
      addChild(parent.id, node, parent.children.findIndex((c) => c.id === selected.id) + 1)
    }
    setMenu(false)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-slate-800 p-2">
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search name, type or value…" aria-label="Search nodes" className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200 placeholder-slate-500 outline-none focus:border-teal-500" />
      </div>
      <div role="tree" aria-label="Plan nodes" className="flex-1 overflow-y-auto p-1">
        <TreeItem node={plan.root} depth={0} query={search} />
      </div>
      <p className="border-t border-slate-800 px-3 pt-2 text-[10px] text-slate-500">Drag nodes to move them. Drop on the middle of a node to nest inside it.</p>
      <div className="relative p-2">
        <button type="button" onClick={() => setMenu(!menu)} aria-expanded={menu} className="w-full rounded bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-slate-700">
          + Add node
        </button>
        {menu && (
          <div role="menu" className="absolute bottom-full left-2 right-2 mb-1 max-h-80 overflow-y-auto rounded-md border border-slate-700 bg-slate-900 py-1 shadow-xl">
            {[
              ['Requests', SAMPLER_TYPES],
              ['Logic and structure', CONTROLLER_TYPES],
              ['Checks, timing and data', AUXILIARY_TYPES],
            ].map(([label, types]) => (
              <div key={label as string}>
                <div className="px-3 pb-0.5 pt-1.5 text-[10px] font-semibold uppercase tracking-wide text-slate-500">{label as string}</div>
                {(types as typeof SAMPLER_TYPES).map((t) => (
                  <button key={t.type} role="menuitem" type="button" onClick={() => add(t.type)} title={t.description} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-slate-300 hover:bg-slate-800">
                    <span className="flex h-4 w-4 items-center justify-center rounded bg-slate-800 font-mono text-[10px] text-slate-400">{t.icon}</span>
                    {t.label}
                  </button>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
