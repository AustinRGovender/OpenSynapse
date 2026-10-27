import type { Node } from '@opensynapse/api-client'

export interface NodeTypeConfig {
  type: string
  label: string
  category: 'sampler' | 'controller' | 'auxiliary'
  icon: string // short glyph shown in the tree and canvas
  description: string
  defaultProperties: Record<string, unknown>
  canHaveChildren: boolean
}

export const NODE_TYPES: Record<string, NodeTypeConfig> = {
  plan: {
    type: 'plan',
    label: 'Plan',
    category: 'controller',
    icon: 'P',
    description: 'The whole test: its name, default environment and pass/fail thresholds.',
    defaultProperties: {},
    canHaveChildren: true,
  },
  scenario: {
    type: 'scenario',
    label: 'Scenario',
    category: 'controller',
    icon: 'S',
    description: 'How much load to apply (executor, VUs, stages). Its children are what each virtual user does.',
    defaultProperties: { executor: 'constant-vus', vus: 10, duration: '1m' },
    canHaveChildren: true,
  },
  http: {
    type: 'http',
    label: 'HTTP Request',
    category: 'sampler',
    icon: 'H',
    description: 'Sends one HTTP request.',
    defaultProperties: { method: 'GET', url: '', headers: [], body: { type: 'none' }, follow_redirects: true },
    canHaveChildren: true,
  },
  websocket: {
    type: 'websocket',
    label: 'WebSocket',
    category: 'sampler',
    icon: 'W',
    description: 'Opens a WebSocket, sends messages and checks replies.',
    defaultProperties: { url: '', connect_timeout: '5s', messages: [], expected_messages: [], disconnect_behavior: 'close_gracefully' },
    canHaveChildren: true,
  },
  'code-block': {
    type: 'code-block',
    label: 'Code Block',
    category: 'sampler',
    icon: '{',
    description: 'Custom k6 JavaScript for anything the builder cannot express.',
    defaultProperties: { code: '// your k6 code here' },
    canHaveChildren: true,
  },
  'if-controller': {
    type: 'if-controller',
    label: 'If',
    category: 'controller',
    icon: '?',
    description: 'Runs its children only when a condition is true.',
    defaultProperties: { condition: 'true' },
    canHaveChildren: true,
  },
  'else-controller': {
    type: 'else-controller',
    label: 'Else',
    category: 'controller',
    icon: ':',
    description: 'Runs its children when the preceding If was false.',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'loop-controller': {
    type: 'loop-controller',
    label: 'Loop',
    category: 'controller',
    icon: 'L',
    description: 'Repeats its children a number of times or while a condition holds.',
    defaultProperties: { count: 5 },
    canHaveChildren: true,
  },
  'transaction-controller': {
    type: 'transaction-controller',
    label: 'Transaction',
    category: 'controller',
    icon: 'T',
    description: 'Groups requests into one named transaction in the results.',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'once-only-controller': {
    type: 'once-only-controller',
    label: 'Once Only',
    category: 'controller',
    icon: '1',
    description: 'Runs its children once per virtual user (e.g. log in).',
    defaultProperties: {},
    canHaveChildren: true,
  },
  'random-controller': {
    type: 'random-controller',
    label: 'Random',
    category: 'controller',
    icon: 'R',
    description: 'Runs one child at random each iteration, optionally weighted.',
    defaultProperties: { weights: [] },
    canHaveChildren: true,
  },
  assertion: {
    type: 'assertion',
    label: 'Assertion',
    category: 'auxiliary',
    icon: 'A',
    description: 'Checks the parent request’s response; failures show in Assertions.',
    defaultProperties: { target: 'status', condition: 'equals', value: 200, negate: false },
    canHaveChildren: false,
  },
  timer: {
    type: 'timer',
    label: 'Timer',
    category: 'auxiliary',
    icon: 'Z',
    description: 'Waits (think time) before the next step.',
    defaultProperties: { timer_type: 'constant', duration_ms: 1000 },
    canHaveChildren: false,
  },
  'data-source': {
    type: 'data-source',
    label: 'Data Source',
    category: 'auxiliary',
    icon: 'D',
    description: 'Loads CSV or JSON test data shared across VUs.',
    defaultProperties: { source_type: 'csv', path: '', variable_name: 'data', sharing: 'shared' },
    canHaveChildren: false,
  },
  'environment-binding': {
    type: 'environment-binding',
    label: 'Environment',
    category: 'auxiliary',
    icon: 'E',
    description: 'Notes which environment the plan expects.',
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
export const CONTROLLER_TYPES = Object.values(NODE_TYPES).filter((t) => t.category === 'controller' && t.type !== 'plan')
export const AUXILIARY_TYPES = Object.values(NODE_TYPES).filter((t) => t.category === 'auxiliary')
