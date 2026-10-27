import { create } from 'zustand'
import type { Node, Plan } from '@opensynapse/api-client'
import { api, errorMessage, unwrap } from '../api'

interface HistoryEntry {
  root: Node
  description: string
}

export interface PlanMeta {
  name: string
  description: string
  tags: string[]
  default_environment_id: string | null
}

interface PlanState {
  plan: Plan | null
  loading: boolean
  notFound: boolean
  saving: boolean
  dirty: boolean
  saveError: string | null

  selectedNodeId: string | null
  undoStack: HistoryEntry[]
  redoStack: HistoryEntry[]
  clipboard: Node | null

  loadPlan: (id: string) => Promise<void>
  savePlan: () => Promise<void>
  setSelectedNode: (id: string | null) => void
  updateMeta: (meta: Partial<PlanMeta>) => void

  updateNode: (nodeId: string, updates: Partial<Node>) => void
  addChild: (parentId: string, node: Node, index?: number) => void
  removeNode: (nodeId: string) => void
  moveNode: (nodeId: string, newParentId: string, index: number) => void

  copyNode: (nodeId: string) => void
  cutNode: (nodeId: string) => void
  pasteNode: (parentId: string) => void
  duplicateNode: (nodeId: string) => void

  undo: () => void
  redo: () => void
  canUndo: () => boolean
  canRedo: () => boolean
}

/** Finds a node by ID. */
function findNode(root: Node, id: string): Node | null {
  if (root.id === id) return root
  for (const child of root.children) {
    const found = findNode(child, id)
    if (found) return found
  }
  return null
}

/** Finds a node's parent. */
function findParent(root: Node, id: string): Node | null {
  for (const child of root.children) {
    if (child.id === id) return root
    const found = findParent(child, id)
    if (found) return found
  }
  return null
}

function cloneWithNewIds(node: Node): Node {
  return {
    ...node,
    id: crypto.randomUUID(),
    properties: JSON.parse(JSON.stringify(node.properties ?? {})),
    children: node.children.map(cloneWithNewIds),
  }
}

function updateNodeInTree(root: Node, nodeId: string, updates: Partial<Node>): Node {
  if (root.id === nodeId) return { ...root, ...updates }
  return { ...root, children: root.children.map((c) => updateNodeInTree(c, nodeId, updates)) }
}

function removeNodeFromTree(root: Node, nodeId: string): Node {
  return { ...root, children: root.children.filter((c) => c.id !== nodeId).map((c) => removeNodeFromTree(c, nodeId)) }
}

function addChildToTree(root: Node, parentId: string, newChild: Node, index?: number): Node {
  if (root.id === parentId) {
    const children = [...root.children]
    if (index !== undefined && index >= 0 && index <= children.length) children.splice(index, 0, newChild)
    else children.push(newChild)
    return { ...root, children }
  }
  return { ...root, children: root.children.map((c) => addChildToTree(c, parentId, newChild, index)) }
}

const MAX_HISTORY = 200
let saveTimeout: ReturnType<typeof setTimeout> | null = null
// Bumped on every local edit; a save only clears "dirty" if no edit happened meanwhile.
let editSeq = 0

export const usePlanStore = create<PlanState>((set, get) => {
  function commit(root: Node, description: string, extra: Partial<PlanState> = {}) {
    const { plan, undoStack } = get()
    if (!plan) return
    editSeq++
    set({
      plan: { ...plan, root },
      undoStack: [...undoStack, { root: plan.root, description }].slice(-MAX_HISTORY),
      redoStack: [],
      dirty: true,
      ...extra,
    })
    debouncedSave()
  }

  function debouncedSave() {
    if (saveTimeout) clearTimeout(saveTimeout)
    saveTimeout = setTimeout(() => get().savePlan(), 300)
  }

  return {
    plan: null,
    loading: false,
    notFound: false,
    saving: false,
    dirty: false,
    saveError: null,
    selectedNodeId: null,
    undoStack: [],
    redoStack: [],
    clipboard: null,

    loadPlan: async (id) => {
      set({ loading: true, notFound: false, plan: null, selectedNodeId: null })
      try {
        const plan = await unwrap(api.GET('/api/v1/plans/{id}', { params: { path: { id } } }))
        set({ plan, loading: false, dirty: false, saveError: null, undoStack: [], redoStack: [] })
      } catch {
        set({ loading: false, notFound: true })
      }
    },

    savePlan: async () => {
      const { plan } = get()
      if (!plan) return
      const seq = editSeq
      set({ saving: true })
      try {
        const saved = await unwrap(
          api.PUT('/api/v1/plans/{id}', {
            params: { path: { id: plan.id } },
            body: { name: plan.name, description: plan.description, tags: plan.tags, root: plan.root, default_environment_id: plan.default_environment_id },
          }),
        )
        const current = get().plan
        // Keep local edits made while the request was in flight; take only the server's bookkeeping.
        if (current) {
          set({
            plan: { ...current, version: saved.version, updated_at: saved.updated_at },
            saving: false,
            dirty: seq !== editSeq,
            saveError: null,
          })
        }
      } catch (e) {
        set({ saving: false, saveError: `Not saved: ${errorMessage(e)}` })
      }
    },

    setSelectedNode: (id) => set({ selectedNodeId: id }),

    updateMeta: (meta) => {
      const { plan } = get()
      if (!plan) return
      editSeq++
      const next = { ...plan, ...meta }
      // The root node's name mirrors the plan name.
      if (meta.name !== undefined) next.root = { ...plan.root, name: meta.name }
      set({ plan: next, dirty: true })
      debouncedSave()
    },

    updateNode: (nodeId, updates) => {
      const { plan } = get()
      if (!plan) return
      commit(updateNodeInTree(plan.root, nodeId, updates), 'update node')
      // Renaming the root renames the plan.
      if (nodeId === plan.root.id && updates.name !== undefined) set({ plan: { ...get().plan!, name: updates.name } })
    },

    addChild: (parentId, node, index) => {
      const { plan } = get()
      if (!plan) return
      commit(addChildToTree(plan.root, parentId, node, index), 'add node', { selectedNodeId: node.id })
    },

    removeNode: (nodeId) => {
      const { plan, selectedNodeId } = get()
      if (!plan || nodeId === plan.root.id) return
      commit(removeNodeFromTree(plan.root, nodeId), 'remove node', { selectedNodeId: selectedNodeId === nodeId ? null : selectedNodeId })
    },

    moveNode: (nodeId, newParentId, index) => {
      const { plan } = get()
      if (!plan) return
      const node = findNode(plan.root, nodeId)
      // A node cannot move into its own subtree.
      if (!node || findNode(node, newParentId)) return
      commit(addChildToTree(removeNodeFromTree(plan.root, nodeId), newParentId, node, index), 'move node')
    },

    copyNode: (nodeId) => {
      const { plan } = get()
      const node = plan && findNode(plan.root, nodeId)
      if (node) set({ clipboard: JSON.parse(JSON.stringify(node)) })
    },

    cutNode: (nodeId) => {
      const { plan } = get()
      const node = plan && findNode(plan.root, nodeId)
      if (!node || nodeId === plan!.root.id) return
      set({ clipboard: JSON.parse(JSON.stringify(node)) })
      get().removeNode(nodeId)
    },

    pasteNode: (parentId) => {
      const { clipboard } = get()
      if (clipboard) get().addChild(parentId, cloneWithNewIds(clipboard))
    },

    duplicateNode: (nodeId) => {
      const { plan } = get()
      if (!plan) return
      const node = findNode(plan.root, nodeId)
      const parent = findParent(plan.root, nodeId)
      if (!node || !parent) return
      const cloned = cloneWithNewIds(node)
      cloned.name = `${node.name} (copy)`
      get().addChild(parent.id, cloned, parent.children.findIndex((c) => c.id === nodeId) + 1)
    },

    undo: () => {
      const { plan, undoStack, redoStack } = get()
      if (!plan || undoStack.length === 0) return
      const prev = undoStack[undoStack.length - 1]
      editSeq++
      set({ plan: { ...plan, root: prev.root }, undoStack: undoStack.slice(0, -1), redoStack: [...redoStack, { root: plan.root, description: 'redo' }], dirty: true })
      debouncedSave()
    },

    redo: () => {
      const { plan, undoStack, redoStack } = get()
      if (!plan || redoStack.length === 0) return
      const next = redoStack[redoStack.length - 1]
      editSeq++
      set({ plan: { ...plan, root: next.root }, undoStack: [...undoStack, { root: plan.root, description: 'undo' }], redoStack: redoStack.slice(0, -1), dirty: true })
      debouncedSave()
    },

    canUndo: () => get().undoStack.length > 0,
    canRedo: () => get().redoStack.length > 0,
  }
})

export { findNode, findParent }
