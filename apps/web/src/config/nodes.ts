import type { Node } from '@opensynapse/api-client'

export interface NodeTypeConfig {
  type: string
  label: string
  category: 'sampler' | 'controller' | 'auxiliary'
  icon: string // emoji as simple icon for now; replaced with proper icons later
  defaultProperties: Record<string, unknown>
  canHaveChildren: boolean
}

export const NODE_TYPES: Record<string, NodeTypeConfig> = {
  scenario: {
    type: 'scenario',
    label: 'Scenario',
    category: 'controller',
    icon: 'S',
    defaultProperties: { executor: 'constant-vus', vus: 10, duration: '1m' },
    canHaveChildren: true,
  },
  http: {
    type: 'http',
    label: 'HTTP Request',
    category: 'sampler',
    icon: 'H',
    defaultProperties: { method: 'GET', url: '', headers: [], body: { type: 'none' }, follow_redirects: true },
    canHaveChildren: true,
  },
  websocket: {
    type: 'websocket',
    label: 'WebSocket',
    category: 'sampler',
    icon: 'W',
    defaultProperties: { url: '', connect_timeout: '5s', messages: [], expected_messages: [], disconnect_behavior: 'close_gracefully' },
    canHaveChildren: true,
  },
  'code-block': {
    type: 'code-block',
    label: 'Code Block',
    category: 'sampler',
    icon: '{',
    defaultProperties: { code: '// your k6 code here' },
    canHaveChildren: true,
  },
  'if-controller': {
    type: 'if-controller',
    label: 'If',
    category: 'controller',
    icon: '?',
    defaultProperties: { condition: 'true' },
    canHaveChildren: true,
  },
  'else-controller': {
    type: 'else-controller',
    label: 'Else',
    category: 'controller',
    icon: ':',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'loop-controller': {
    type: 'loop-controller',
    label: 'Loop',
    category: 'controller',
    icon: 'L',
    defaultProperties: { count: 5 },
    canHaveChildren: true,
  },
  'transaction-controller': {
    type: 'transaction-controller',
    label: 'Transaction',
    category: 'controller',
    icon: 'T',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'once-only-controller': {
    type: 'once-only-controller',
    label: 'Once Only',
    category: 'controller',
    icon: '1',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'random-controller': {
    type: 'random-controller',
    label: 'Random',
    category: 'controller',
    icon: 'R',
    defaultProperties: { weights: [] },
    canHaveChildren: true,
  },
  assertion: {
    type: 'assertion',
    label: 'Assertion',
    category: 'auxiliary',
    icon: 'A',
    defaultProperties: { target: 'status', condition: 'equals', value: 200, negate: false },
    canHaveChildren: false,
  },
  timer: {
    type: 'timer',
    label: 'Timer',
    category: 'auxiliary',
    icon: 'Z',
    defaultProperties: { timer_type: 'constant', duration_ms: 1000 },
    canHaveChildren: false,
  },
  'data-source': {
    type: 'data-source',
    label: 'Data Source',
    category: 'auxiliary',
    icon: 'D',
    defaultProperties: { source_type: 'csv', path: '', variable_name: 'data', sharing: 'shared' },
    canHaveChildren: false,
  },
  'environment-binding': {
    type: 'environment-binding',
    label: 'Environment',
    category: 'auxiliary',
    icon: 'E',
    defaultProperties: { environment_id: '' },
    canHaveChildren: false,
  },
}

export function createNode(type: string, name?: string): Node {
  const config = NODE_TYPES[type]
  if (!config) throw new Error(`Unknown node type: ${type}`)
  return {
    id: crypto.randomUUID(),
    type,
    name: name || `New ${config.label}`,
    enabled: true,
    properties: { ...config.defaultProperties },
    children: [],
  }
}

export const SAMPLER_TYPES = Object.values(NODE_TYPES).filter((t) => t.category === 'sampler')
export const CONTROLLER_TYPES = Object.values(NODE_TYPES).filter((t) => t.category === 'controller')
export const AUXILIARY_TYPES = Object.values(NODE_TYPES).filter((t) => t.category === 'auxiliary')
