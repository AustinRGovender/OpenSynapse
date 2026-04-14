import { useCallback } from 'react'
import {
  ReactFlow,
  type Node as FlowNode,
  type Edge,
  Background,
  BackgroundVariant,
  Controls,
  useNodesState,
  useEdgesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { Node } from '@opensynapse/api-client'
import { usePlanStore, findNode } from '../../stores/plan-store'
import { NODE_TYPES } from '../../config/nodes'
import { useEffect } from 'react'

// Convert a plan subtree to React Flow nodes and edges
function planToFlow(root: Node, selectedBranchId?: string | null): { nodes: FlowNode[]; edges: Edge[] } {
  const nodes: FlowNode[] = []
  const edges: Edge[] = []

  // If a node is selected, show that node and its children
  const branchRoot = selectedBranchId ? findNode(root, selectedBranchId) : root
  if (!branchRoot) return { nodes, edges }

  function walk(node: Node, x: number, y: number, parentId?: string) {
    const config = NODE_TYPES[node.type]
    nodes.push({
      id: node.id,
      type: 'default',
      position: { x, y },
      data: {
        label: `${config?.icon || '?'} ${node.name}`,
      },
      style: {
        background: '#1e293b',
        color: '#e2e8f0',
        border: '1px solid #334155',
        borderRadius: '6px',
        padding: '8px 12px',
        fontSize: '12px',
        minWidth: '140px',
      },
    })

    if (parentId) {
      edges.push({
        id: `${parentId}-${node.id}`,
        source: parentId,
        target: node.id,
        style: { stroke: '#475569' },
      })
    }

    const childSpacing = 180
    const totalWidth = (node.children.length - 1) * childSpacing
    const startX = x - totalWidth / 2

    node.children.forEach((child, i) => {
      walk(child, startX + i * childSpacing, y + 80, node.id)
    })
  }

  walk(branchRoot, 400, 50)
  return { nodes, edges }
}

export function FlowCanvas() {
  const { plan, selectedNodeId, setSelectedNode } = usePlanStore()
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState([] as FlowNode[])
  const [flowEdges, setFlowEdges, onEdgesChange] = useEdgesState([] as Edge[])

  // Recompute flow when plan or selection changes
  useEffect(() => {
    if (!plan) return
    const { nodes, edges } = planToFlow(plan.root, selectedNodeId)
    setFlowNodes(nodes)
    setFlowEdges(edges)
  }, [plan, selectedNodeId, setFlowNodes, setFlowEdges])

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: FlowNode) => {
      setSelectedNode(node.id)
    },
    [setSelectedNode],
  )

  if (!plan) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-500">
        No plan loaded
      </div>
    )
  }

  return (
    <div className="h-full w-full">
      <ReactFlow
        nodes={flowNodes}
        edges={flowEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={onNodeClick}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#1e293b" />
        <Controls
          style={{ background: '#0f172a', border: '1px solid #334155', borderRadius: '6px' }}
        />
      </ReactFlow>
    </div>
  )
}
