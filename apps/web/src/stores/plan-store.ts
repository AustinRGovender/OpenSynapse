import { create } from 'zustand'
import { OpenSynapseClient, type Plan, type Node } from '@opensynapse/api-client'

const client = new OpenSynapseClient('/api/v1')

interface HistoryEntry {
  root: Node
  description: string
}

interface PlanState {
  // Current plan
  plan: Plan | null
  loading: boolean
  saving: boolean
  dirty: boolean

  // Selection
  selectedNodeId: string | null

  // Undo/redo
  undoStack: HistoryEntry[]
  redoStack: HistoryEntry[]

  // Clipboard
  clipboard: Node | null

  // Actions
  loadPlan: (id: string) => Promise<void>
  savePlan: () => Promise<void>
  setSelectedNode: (id: string | null) => void

  // Node mutations (all push to undo stack)
  updateNode: (nodeId: string, updates: Partial<Node>) => void
  addChild: (parentId: string, node: Node, index?: number) => void
  removeNode: (nodeId: string) => void
  moveNode: (nodeId: string, newParentId: string, index: number) => void

  // Clipboard
  copyNode: (nodeId: string) => void
  cutNode: (nodeId: string) => void
  pasteNode: (parentId: string) => void
  duplicateNode: (nodeId: string) => void

  // Undo/redo
  undo: () => void
  redo: () => void
  canUndo: () => boolean
  canRedo: () => boolean
}

// Helper: find a node by ID in a tree
function findNode(root: Node, id: string): Node | null {
  if (root.id === id) return root
  for (const child of root.children) {
    const found = findNode(child, id)
    if (found) return found
  }
  return null
}

// Helper: find parent of a node
function findParent(root: Node, id: string): Node | null {
  for (const child of root.children) {
    if (child.id === id) return root
    const found = findParent(child, id)
    if (found) return found
  }
  return null
}

// Helper: deep clone a node tree with new IDs
function cloneWithNewIds(node: Node): Node {
  return {
    ...node,
    id: crypto.randomUUID(),
    properties: JSON.parse(JSON.stringify(node.properties)),
    children: node.children.map(cloneWithNewIds),
  }
}

// Helper: update a node in the tree immutably
function updateNodeInTree(root: Node, nodeId: string, updates: Partial<Node>): Node {
  if (root.id === nodeId) {
    return { ...root, ...updates }
  }
  return {
    ...root,
    children: root.children.map((child) => updateNodeInTree(child, nodeId, updates)),
  }
}

// Helper: remove a node from the tree immutably
function removeNodeFromTree(root: Node, nodeId: string): Node {
  return {
    ...root,
    children: root.children
      .filter((child) => child.id !== nodeId)
      .map((child) => removeNodeFromTree(child, nodeId)),
  }
}

// Helper: add a child to a parent node immutably
function addChildToTree(root: Node, parentId: string, newChild: Node, index?: number): Node {
  if (root.id === parentId) {
    const children = [...root.children]
    if (index !== undefined && index >= 0 && index <= children.length) {
      children.splice(index, 0, newChild)
    } else {
      children.push(newChild)
    }
    return { ...root, children }
  }
  return {
    ...root,
    children: root.children.map((child) => addChildToTree(child, parentId, newChild, index)),
  }
}

let saveTimeout: ReturnType<typeof setTimeout> | null = null

export const usePlanStore = create<PlanState>((set, get) => ({
  plan: null,
  loading: false,
  saving: false,
  dirty: false,
  selectedNodeId: null,
  undoStack: [],
  redoStack: [],
  clipboard: null,

  loadPlan: async (id: string) => {
    set({ loading: true })
    try {
      const plan = await client.getPlan(id)
      set({ plan, loading: false, dirty: false, undoStack: [], redoStack: [] })
    } catch {
      set({ loading: false })
    }
  },

  savePlan: async () => {
    const { plan } = get()
    if (!plan) return

    set({ saving: true })
    try {
      const updated = await client.updatePlan(plan.id, {
        name: plan.name,
        description: plan.description,
        tags: plan.tags,
        root: plan.root,
        default_environment_id: plan.default_environment_id,
      })
      set({ plan: updated, saving: false, dirty: false })
    } catch {
      set({ saving: false })
    }
  },

  setSelectedNode: (id) => set({ selectedNodeId: id }),

  updateNode: (nodeId, updates) => {
    const { plan } = get()
    if (!plan) return

    // Push current state to undo stack
    const undoStack = [...get().undoStack, { root: plan.root, description: 'update node' }]

    const newRoot = updateNodeInTree(plan.root, nodeId, updates)
    set({
      plan: { ...plan, root: newRoot },
      undoStack,
      redoStack: [],
      dirty: true,
    })
    debouncedSave(get)
  },

  addChild: (parentId, node, index) => {
    const { plan } = get()
    if (!plan) return

    const undoStack = [...get().undoStack, { root: plan.root, description: 'add node' }]
    const newRoot = addChildToTree(plan.root, parentId, node, index)
    set({
      plan: { ...plan, root: newRoot },
      undoStack,
      redoStack: [],
      dirty: true,
      selectedNodeId: node.id,
    })
    debouncedSave(get)
  },

  removeNode: (nodeId) => {
    const { plan, selectedNodeId } = get()
    if (!plan) return

    const undoStack = [...get().undoStack, { root: plan.root, description: 'remove node' }]
    const newRoot = removeNodeFromTree(plan.root, nodeId)
    set({
      plan: { ...plan, root: newRoot },
      undoStack,
      redoStack: [],
      dirty: true,
      selectedNodeId: selectedNodeId === nodeId ? null : selectedNodeId,
    })
    debouncedSave(get)
  },

  moveNode: (nodeId, newParentId, index) => {
    const { plan } = get()
    if (!plan) return

    const node = findNode(plan.root, nodeId)
    if (!node) return

    const undoStack = [...get().undoStack, { root: plan.root, description: 'move node' }]
    let newRoot = removeNodeFromTree(plan.root, nodeId)
    newRoot = addChildToTree(newRoot, newParentId, node, index)
    set({
      plan: { ...plan, root: newRoot },
      undoStack,
      redoStack: [],
      dirty: true,
    })
    debouncedSave(get)
  },

  copyNode: (nodeId) => {
    const { plan } = get()
    if (!plan) return
    const node = findNode(plan.root, nodeId)
    if (node) set({ clipboard: JSON.parse(JSON.stringify(node)) })
  },

  cutNode: (nodeId) => {
    const { plan } = get()
    if (!plan) return
    const node = findNode(plan.root, nodeId)
    if (!node) return
    set({ clipboard: JSON.parse(JSON.stringify(node)) })
    get().removeNode(nodeId)
  },

  pasteNode: (parentId) => {
    const { clipboard } = get()
    if (!clipboard) return
    const cloned = cloneWithNewIds(clipboard)
    get().addChild(parentId, cloned)
  },

  duplicateNode: (nodeId) => {
    const { plan } = get()
    if (!plan) return
    const node = findNode(plan.root, nodeId)
    const parent = findParent(plan.root, nodeId)
    if (!node || !parent) return

    const cloned = cloneWithNewIds(node)
    cloned.name = node.name + ' (copy)'
    const index = parent.children.findIndex((c) => c.id === nodeId) + 1
    get().addChild(parent.id, cloned, index)
  },

  undo: () => {
    const { plan, undoStack } = get()
    if (!plan || undoStack.length === 0) return

    const prev = undoStack[undoStack.length - 1]
    const newUndoStack = undoStack.slice(0, -1)
    const redoStack = [...get().redoStack, { root: plan.root, description: 'redo' }]

    set({
      plan: { ...plan, root: prev.root },
      undoStack: newUndoStack,
      redoStack,
      dirty: true,
    })
    debouncedSave(get)
  },

  redo: () => {
    const { plan, redoStack } = get()
    if (!plan || redoStack.length === 0) return

    const next = redoStack[redoStack.length - 1]
    const newRedoStack = redoStack.slice(0, -1)
    const undoStack = [...get().undoStack, { root: plan.root, description: 'undo' }]

    set({
      plan: { ...plan, root: next.root },
      undoStack,
      redoStack: newRedoStack,
      dirty: true,
    })
    debouncedSave(get)
  },

  canUndo: () => get().undoStack.length > 0,
  canRedo: () => get().redoStack.length > 0,
}))

function debouncedSave(get: () => PlanState) {
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    get().savePlan()
  }, 300)
}

// Export helpers for external use
export { findNode, findParent }
