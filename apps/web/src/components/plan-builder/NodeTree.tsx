import { useState } from 'react'
import type { Node } from '@opensynapse/api-client'
import { usePlanStore } from '../../stores/plan-store'
import { NODE_TYPES, createNode } from '../../config/nodes'

interface NodeTreeItemProps {
  node: Node
  depth: number
  searchQuery: string
}

function NodeTreeItem({ node, depth, searchQuery }: NodeTreeItemProps) {
  const [expanded, setExpanded] = useState(true)
  const { selectedNodeId, setSelectedNode } = usePlanStore()
  const isSelected = selectedNodeId === node.id
  const config = NODE_TYPES[node.type]
  const hasChildren = node.children.length > 0

  const matchesSearch =
    !searchQuery ||
    node.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    node.type.toLowerCase().includes(searchQuery.toLowerCase())

  if (!matchesSearch && !node.children.some((c) => nodeMatchesSearch(c, searchQuery))) {
    return null
  }

  return (
    <div>
      <div
        className={`flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-sm ${
          isSelected
            ? 'bg-teal-500/20 text-teal-300'
            : 'text-slate-300 hover:bg-slate-800'
        } ${!node.enabled ? 'opacity-50' : ''}`}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        onClick={() => setSelectedNode(node.id)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') setSelectedNode(node.id)
        }}
        tabIndex={0}
        role="treeitem"
        aria-selected={isSelected}
        aria-expanded={hasChildren ? expanded : undefined}
      >
        {hasChildren ? (
          <button
            className="flex h-4 w-4 items-center justify-center text-xs text-slate-500"
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
          >
            {expanded ? '▾' : '▸'}
          </button>
        ) : (
          <span className="h-4 w-4" />
        )}
        <span className="flex h-5 w-5 items-center justify-center rounded bg-slate-800 text-xs font-mono text-slate-400">
          {config?.icon || '?'}
        </span>
        <span className="truncate">{node.name}</span>
        <span className="ml-auto text-xs text-slate-600">{node.type}</span>
      </div>

      {expanded && hasChildren && (
        <div role="group">
          {node.children.map((child) => (
            <NodeTreeItem
              key={child.id}
              node={child}
              depth={depth + 1}
              searchQuery={searchQuery}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function nodeMatchesSearch(node: Node, query: string): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  if (node.name.toLowerCase().includes(q) || node.type.toLowerCase().includes(q)) return true
  return node.children.some((c) => nodeMatchesSearch(c, q))
}

export function NodeTree() {
  const { plan, addChild } = usePlanStore()
  const [search, setSearch] = useState('')
  const [showAddMenu, setShowAddMenu] = useState(false)

  if (!plan) return null

  return (
    <div className="flex h-full flex-col">
      {/* Search */}
      <div className="border-b border-slate-800 p-2">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search nodes..."
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200 placeholder-slate-500 outline-none focus:border-teal-500"
        />
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-y-auto p-1" role="tree">
        <NodeTreeItem node={plan.root} depth={0} searchQuery={search} />
      </div>

      {/* Add node button */}
      <div className="relative border-t border-slate-800 p-2">
        <button
          className="w-full rounded bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-slate-700"
          onClick={() => setShowAddMenu(!showAddMenu)}
        >
          + Add Node
        </button>
        {showAddMenu && (
          <AddNodeMenu
            onSelect={(type) => {
              const { selectedNodeId } = usePlanStore.getState()
              const parentId = selectedNodeId || plan.root.id
              addChild(parentId, createNode(type))
              setShowAddMenu(false)
            }}
            onClose={() => setShowAddMenu(false)}
          />
        )}
      </div>
    </div>
  )
}

function AddNodeMenu({ onSelect, onClose }: { onSelect: (type: string) => void; onClose: () => void }) {
  const categories = [
    { label: 'Samplers', types: Object.values(NODE_TYPES).filter((t) => t.category === 'sampler') },
    { label: 'Controllers', types: Object.values(NODE_TYPES).filter((t) => t.category === 'controller') },
    { label: 'Auxiliary', types: Object.values(NODE_TYPES).filter((t) => t.category === 'auxiliary') },
  ]

  return (
    <div className="absolute bottom-full left-0 right-0 mb-1 max-h-64 overflow-y-auto rounded-md border border-slate-700 bg-slate-900 shadow-lg">
      {categories.map((cat) => (
        <div key={cat.label}>
          <div className="px-3 py-1 text-xs font-medium text-slate-500">{cat.label}</div>
          {cat.types.map((t) => (
            <button
              key={t.type}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-slate-300 hover:bg-slate-800"
              onClick={() => onSelect(t.type)}
            >
              <span className="flex h-4 w-4 items-center justify-center rounded bg-slate-800 font-mono text-xs text-slate-400">
                {t.icon}
              </span>
              {t.label}
            </button>
          ))}
        </div>
      ))}
      <button
        className="w-full border-t border-slate-800 px-3 py-1.5 text-xs text-slate-500 hover:bg-slate-800"
        onClick={onClose}
      >
        Cancel
      </button>
    </div>
  )
}
