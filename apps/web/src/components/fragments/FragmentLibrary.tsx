import { useState } from 'react'
import { usePlanStore, findNode } from '../../stores/plan-store'
import type { Node } from '@opensynapse/api-client'

interface FragmentLibraryProps {
  onClose: () => void
}

interface FragmentDef {
  name: string
  description: string
  tags: string[]
  nodeTypes: string[]
}

const SHIPPED_FRAGMENTS: FragmentDef[] = [
  {
    name: 'Generic Form Login',
    description: 'POST username/password to a login endpoint and extract session cookie',
    tags: ['auth', 'form'],
    nodeTypes: ['http', 'assertion'],
  },
  {
    name: 'CSRF Token Extraction',
    description: 'GET a page, extract the CSRF token via regex, and pass it to subsequent requests',
    tags: ['auth', 'csrf'],
    nodeTypes: ['http', 'code-block'],
  },
  {
    name: 'Pagination Walker',
    description: 'Loop through paginated API responses until no more pages remain',
    tags: ['api', 'pagination'],
    nodeTypes: ['loop-controller', 'http', 'code-block'],
  },
  {
    name: 'Search-then-Select',
    description: 'Search for items then pick one at random from the results',
    tags: ['api', 'workflow'],
    nodeTypes: ['http', 'code-block'],
  },
  {
    name: 'Cart Checkout',
    description: 'Add items to cart, view cart, apply coupon, and complete checkout',
    tags: ['e-commerce', 'workflow'],
    nodeTypes: ['transaction-controller', 'http'],
  },
  {
    name: 'File Upload (multipart)',
    description: 'Upload a file using multipart/form-data encoding',
    tags: ['upload', 'file'],
    nodeTypes: ['http'],
  },
  {
    name: 'File Download with Hash',
    description: 'Download a file and verify its integrity via content hash',
    tags: ['download', 'file', 'verification'],
    nodeTypes: ['http', 'code-block', 'assertion'],
  },
  {
    name: 'OAuth Authorization Code',
    description: 'Simulate the OAuth 2.0 authorization code flow with token exchange',
    tags: ['auth', 'oauth'],
    nodeTypes: ['http', 'code-block'],
  },
  {
    name: 'SAML Login Stub',
    description: 'Stub a SAML-based SSO login flow for load testing',
    tags: ['auth', 'saml', 'sso'],
    nodeTypes: ['http', 'code-block'],
  },
  {
    name: 'Wait-for-Condition Polling',
    description: 'Poll an endpoint until a condition is met or timeout is reached',
    tags: ['polling', 'async'],
    nodeTypes: ['loop-controller', 'http', 'code-block', 'timer'],
  },
]

interface SavedFragment {
  id: string
  name: string
  tags: string[]
  nodeTypes: string[]
  root: Node
  createdAt: string
}

function getStoredFragments(): SavedFragment[] {
  try {
    const raw = localStorage.getItem('opensynapse_fragments')
    if (raw) return JSON.parse(raw)
  } catch {
    // ignore parse errors
  }
  return []
}

function saveFragmentToStorage(fragment: SavedFragment) {
  const existing = getStoredFragments()
  existing.push(fragment)
  localStorage.setItem('opensynapse_fragments', JSON.stringify(existing))
}

function removeFragmentFromStorage(id: string) {
  const existing = getStoredFragments().filter((f) => f.id !== id)
  localStorage.setItem('opensynapse_fragments', JSON.stringify(existing))
}

function collectNodeTypes(node: Node): string[] {
  const types = new Set<string>()
  function walk(n: Node) {
    if (n.type !== 'plan') types.add(n.type)
    n.children.forEach(walk)
  }
  walk(node)
  return Array.from(types)
}

function buildFragmentNodes(fragmentName: string): Node {
  // Shipped fragments produce placeholder subtrees
  const id = () => crypto.randomUUID()
  switch (fragmentName) {
    case 'Generic Form Login':
      return {
        id: id(), type: 'transaction-controller', name: 'Login', enabled: true,
        properties: {},
        children: [
          {
            id: id(), type: 'http', name: 'POST /login', enabled: true,
            properties: { method: 'POST', url: '${BASE_URL}/login', headers: [], body: { type: 'form', content: 'username=${USERNAME}&password=${PASSWORD}' }, follow_redirects: true },
            children: [],
          },
          {
            id: id(), type: 'assertion', name: 'Check status 200', enabled: true,
            properties: { target: 'status', condition: 'equals', value: 200, negate: false },
            children: [],
          },
        ],
      }
    case 'CSRF Token Extraction':
      return {
        id: id(), type: 'transaction-controller', name: 'CSRF Token Extraction', enabled: true,
        properties: {},
        children: [
          {
            id: id(), type: 'http', name: 'GET page with token', enabled: true,
            properties: { method: 'GET', url: '${BASE_URL}/form', headers: [], body: { type: 'none' }, follow_redirects: true },
            children: [],
          },
          {
            id: id(), type: 'code-block', name: 'Extract CSRF token', enabled: true,
            properties: { code: '// Extract CSRF token from response body\nconst match = response.body.match(/name="csrf_token" value="([^"]+)"/);\nif (match) vars.csrf_token = match[1];' },
            children: [],
          },
        ],
      }
    default:
      return {
        id: id(), type: 'transaction-controller', name: fragmentName, enabled: true,
        properties: {},
        children: [
          {
            id: id(), type: 'code-block', name: `${fragmentName} placeholder`, enabled: true,
            properties: { code: `// TODO: implement ${fragmentName}` },
            children: [],
          },
        ],
      }
  }
}

function FragmentCard({
  name,
  description,
  tags,
  nodeTypes,
  onInsert,
  onDelete,
}: {
  name: string
  description?: string
  tags: string[]
  nodeTypes: string[]
  onInsert: () => void
  onDelete?: () => void
}) {
  return (
    <div className="group rounded-lg border border-slate-800 bg-slate-900 p-3 transition-colors hover:border-slate-700">
      <div className="flex items-start justify-between gap-2">
        <h4 className="text-xs font-semibold text-slate-200">{name}</h4>
        <div className="flex flex-shrink-0 gap-1">
          {onDelete && (
            <button
              onClick={(e) => { e.stopPropagation(); onDelete() }}
              className="hidden rounded p-0.5 text-slate-500 hover:bg-slate-800 hover:text-red-400 group-hover:block"
              title="Delete fragment"
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <path d="M10.5 3.5L3.5 10.5M3.5 3.5l7 7" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
              </svg>
            </button>
          )}
        </div>
      </div>
      {description && (
        <p className="mt-1 text-xs leading-relaxed text-slate-500">{description}</p>
      )}
      <div className="mt-2 flex flex-wrap gap-1">
        {tags.map((tag) => (
          <span
            key={tag}
            className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] font-medium text-slate-400"
          >
            {tag}
          </span>
        ))}
      </div>
      <div className="mt-1.5 flex flex-wrap gap-1">
        {nodeTypes.map((nt) => (
          <span
            key={nt}
            className="rounded bg-teal-500/10 px-1.5 py-0.5 text-[10px] font-medium text-teal-400"
          >
            {nt}
          </span>
        ))}
      </div>
      <button
        onClick={onInsert}
        className="mt-2 w-full rounded bg-slate-800 px-2 py-1 text-xs font-medium text-slate-300 transition-colors hover:bg-slate-700 hover:text-slate-100"
      >
        Insert
      </button>
    </div>
  )
}

export function FragmentLibrary({ onClose }: FragmentLibraryProps) {
  const { plan, selectedNodeId, addChild } = usePlanStore()
  const [userFragments, setUserFragments] = useState<SavedFragment[]>(getStoredFragments)
  const [saveError, setSaveError] = useState<string | null>(null)

  function getInsertTarget(): string | null {
    if (!plan) return null
    if (selectedNodeId) return selectedNodeId
    return plan.root.id
  }

  function handleInsertShipped(fragment: FragmentDef) {
    const targetId = getInsertTarget()
    if (!targetId) return
    const nodes = buildFragmentNodes(fragment.name)
    addChild(targetId, nodes)
  }

  function handleInsertSaved(fragment: SavedFragment) {
    const targetId = getInsertTarget()
    if (!targetId) return
    // Deep clone with new IDs
    function cloneWithNewIds(node: Node): Node {
      return {
        ...node,
        id: crypto.randomUUID(),
        properties: JSON.parse(JSON.stringify(node.properties)),
        children: node.children.map(cloneWithNewIds),
      }
    }
    addChild(targetId, cloneWithNewIds(fragment.root))
  }

  function handleSaveAsFragment() {
    if (!plan || !selectedNodeId) {
      setSaveError('Select a node to save as a fragment')
      setTimeout(() => setSaveError(null), 3000)
      return
    }
    const node = findNode(plan.root, selectedNodeId)
    if (!node) return
    if (node.type === 'plan') {
      setSaveError('Cannot save the root plan node as a fragment')
      setTimeout(() => setSaveError(null), 3000)
      return
    }
    const fragment: SavedFragment = {
      id: crypto.randomUUID(),
      name: node.name,
      tags: ['custom'],
      nodeTypes: collectNodeTypes(node),
      root: JSON.parse(JSON.stringify(node)),
      createdAt: new Date().toISOString(),
    }
    saveFragmentToStorage(fragment)
    setUserFragments(getStoredFragments())
  }

  function handleDeleteFragment(id: string) {
    removeFragmentFromStorage(id)
    setUserFragments(getStoredFragments())
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">
          Fragments
        </span>
        <button
          onClick={onClose}
          className="rounded p-1 text-slate-500 hover:bg-slate-800 hover:text-slate-300"
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M10.5 3.5L3.5 10.5M3.5 3.5l7 7" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
          </svg>
        </button>
      </div>

      {/* Save as fragment button */}
      <div className="border-b border-slate-800 px-3 py-2">
        <button
          onClick={handleSaveAsFragment}
          className="w-full rounded-md border border-dashed border-slate-700 bg-slate-900/50 px-3 py-2 text-xs font-medium text-slate-400 transition-colors hover:border-slate-600 hover:text-slate-300"
        >
          Save selection as Fragment
        </button>
        {saveError && (
          <p className="mt-1 text-[10px] text-red-400">{saveError}</p>
        )}
      </div>

      {/* Scrollable list */}
      <div className="flex-1 overflow-y-auto px-3 py-3">
        {/* Shipped section */}
        <div className="mb-4">
          <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-slate-500">
            Shipped
          </h3>
          <div className="space-y-2">
            {SHIPPED_FRAGMENTS.map((f) => (
              <FragmentCard
                key={f.name}
                name={f.name}
                description={f.description}
                tags={f.tags}
                nodeTypes={f.nodeTypes}
                onInsert={() => handleInsertShipped(f)}
              />
            ))}
          </div>
        </div>

        {/* My Fragments section */}
        <div>
          <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-slate-500">
            My Fragments
          </h3>
          {userFragments.length === 0 ? (
            <p className="text-xs text-slate-600">
              No saved fragments yet. Select a node and click "Save selection as Fragment" above.
            </p>
          ) : (
            <div className="space-y-2">
              {userFragments.map((f) => (
                <FragmentCard
                  key={f.id}
                  name={f.name}
                  tags={f.tags}
                  nodeTypes={f.nodeTypes}
                  onInsert={() => handleInsertSaved(f)}
                  onDelete={() => handleDeleteFragment(f.id)}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
