import { useCallback, useMemo } from 'react'
import { Background, BackgroundVariant, Controls, type Edge, type Node as FlowNode, ReactFlow } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { Node } from '@opensynapse/api-client'
import { NODE_TYPES } from '../../config/nodes'
import { findNode, findParent, usePlanStore } from '../../stores/plan-store'

const NODE_W = 170
const H_GAP = 24
const ROW_H = 90

// Theme-aware colours: Tailwind exposes its palette as CSS variables, which
// index.css remaps for the light theme.
const nodeStyle = (selected: boolean, enabled: boolean) => ({
  background: 'var(--color-slate-800)',
  color: 'var(--color-slate-200)',
  border: `1px solid ${selected ? 'var(--color-teal-500)' : 'var(--color-slate-700)'}`,
  boxShadow: selected ? '0 0 0 2px color-mix(in oklab, var(--color-teal-500) 35%, transparent)' : undefined,
  borderRadius: 6,
  padding: '8px 10px',
  fontSize: 12,
  width: NODE_W,
  opacity: enabled ? 1 : 0.45,
})

/**
 * Tidy tree layout: every subtree reserves the width of its widest level,
 * and each parent sits centred over its children, so siblings never overlap.
 */
function layout(root: Node, selectedId: string | null): { nodes: FlowNode[]; edges: Edge[] } {
  const nodes: FlowNode[] = []
  const edges: Edge[] = []
  const width = new Map<string, number>()

  function measure(n: Node): number {
    const w = n.children.length === 0 ? NODE_W : Math.max(NODE_W, n.children.reduce((s, c) => s + measure(c), 0) + H_GAP * (n.children.length - 1))
    width.set(n.id, w)
    return w
  }
  measure(root)

  function place(n: Node, left: number, depth: number, parentId?: string) {
    const w = width.get(n.id)!
    const cfg = NODE_TYPES[n.type]
    nodes.push({
      id: n.id,
      position: { x: left + w / 2 - NODE_W / 2, y: depth * ROW_H },
      data: { label: `${cfg?.icon ?? '•'}  ${n.name}${n.enabled ? '' : ' (disabled)'}` },
      style: nodeStyle(n.id === selectedId, n.enabled),
      draggable: false,
    })
    if (parentId) edges.push({ id: `${parentId}-${n.id}`, source: parentId, target: n.id, style: { stroke: 'var(--color-slate-600)' } })
    let x = left + (w - (n.children.reduce((s, c) => s + width.get(c.id)!, 0) + H_GAP * Math.max(0, n.children.length - 1))) / 2
    for (const c of n.children) {
      place(c, x, depth + 1, n.id)
      x += width.get(c.id)! + H_GAP
    }
  }
  place(root, 0, 0)
  return { nodes, edges }
}

/** Centre pane: the selected branch (or whole plan) as a flow diagram. */
export function FlowCanvas() {
  const plan = usePlanStore((s) => s.plan)
  const selectedNodeId = usePlanStore((s) => s.selectedNodeId)
  const setSelectedNode = usePlanStore((s) => s.setSelectedNode)

  // Show the selected node's parent branch so its siblings stay in view.
  const branchRoot = useMemo(() => {
    if (!plan) return null
    if (!selectedNodeId) return plan.root
    const parent = findParent(plan.root, selectedNodeId)
    return parent ?? findNode(plan.root, selectedNodeId) ?? plan.root
  }, [plan, selectedNodeId])

  const { nodes, edges } = useMemo(() => (branchRoot ? layout(branchRoot, selectedNodeId) : { nodes: [], edges: [] }), [branchRoot, selectedNodeId])

  const onNodeClick = useCallback((_: React.MouseEvent, node: FlowNode) => setSelectedNode(node.id), [setSelectedNode])

  if (!plan) return <div className="flex h-full items-center justify-center text-sm text-slate-500">No plan loaded</div>

  return (
    <div className="relative h-full w-full">
      {branchRoot && branchRoot.id !== plan.root.id && (
        <button type="button" onClick={() => setSelectedNode(null)} className="absolute left-3 top-3 z-10 rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-300 hover:bg-slate-800">
          ↑ Whole plan
        </button>
      )}
      <ReactFlow
        key={branchRoot?.id}
        nodes={nodes}
        edges={edges}
        onNodeClick={onNodeClick}
        nodesConnectable={false}
        fitView
        fitViewOptions={{ padding: 0.2, maxZoom: 1.2 }}
        minZoom={0.1}
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="var(--color-slate-800)" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}
