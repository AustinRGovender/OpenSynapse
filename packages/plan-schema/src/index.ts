/**
 * Plan schema types and validation.
 *
 * Node type JSON schemas live in schemas/nodes/.
 * This module exports TypeScript types for every node kind as a
 * discriminated union on the `type` field.
 */

// ---------------------------------------------------------------------------
// Shared / reusable types
// ---------------------------------------------------------------------------

/** k6 executor types. */
export type Executor =
  | 'ramping-vus'
  | 'constant-vus'
  | 'constant-arrival-rate'
  | 'ramping-arrival-rate'
  | 'shared-iterations'
  | 'per-vu-iterations'
  | 'externally-controlled'

/** HTTP methods supported by the HTTP sampler. */
export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS'

/** Body encoding types. */
export type HttpBodyType = 'none' | 'raw' | 'form' | 'multipart' | 'json' | 'file'

/** Assertion targets. */
export type AssertionTarget = 'status' | 'body' | 'header' | 'response_time'

/** Assertion conditions. */
export type AssertionCondition =
  | 'equals'
  | 'contains'
  | 'matches'
  | 'jsonpath'
  | 'xpath'
  | 'less_than'
  | 'greater_than'
  | 'exists'

/** Timer distribution types. */
export type TimerType = 'constant' | 'uniform_random' | 'gaussian'

/** Data source types. */
export type DataSourceType = 'csv' | 'json' | 'inline'

/** Data sharing strategies. */
export type DataSharing = 'shared' | 'per-vu' | 'sequential' | 'random'

/** WebSocket disconnect behaviors. */
export type DisconnectBehavior = 'close_gracefully' | 'close_immediately' | 'keep_open'

/** WebSocket expected-message match types. */
export type WsMatchType = 'exact' | 'contains' | 'regex' | 'jsonpath'

/** WebSocket message frame types. */
export type WsMessageType = 'text' | 'binary'

/** A k6 duration string, e.g. "30s", "10m", "1h". */
export type Duration = string

// ---------------------------------------------------------------------------
// Reusable sub-types
// ---------------------------------------------------------------------------

/** A single header entry used in HTTP and WebSocket nodes. */
export interface HeaderEntry {
  key: string
  value: string
  enabled?: boolean
}

/** A ramping stage used in scenario executors. */
export interface Stage {
  duration: Duration
  target: number
}

/** HTTP body configuration. */
export interface HttpBody {
  type: HttpBodyType
  content?: string
  form_fields?: FormField[]
  content_type?: string
}

/** A form / multipart field. */
export interface FormField {
  key: string
  value: string
  type?: 'text' | 'file'
}

/** A WebSocket message to send. */
export interface WsMessage {
  type?: WsMessageType
  data: string
  delay_before?: Duration
}

/** A WebSocket expected message matcher. */
export interface WsExpectedMessage {
  match_type?: WsMatchType
  value: string
  timeout?: Duration
}

// ---------------------------------------------------------------------------
// Common base shared by every node
// ---------------------------------------------------------------------------

interface NodeBase {
  id: string
  name: string
  enabled: boolean
}

// ---------------------------------------------------------------------------
// Node types
// ---------------------------------------------------------------------------

/** Root node of a test plan. */
export interface PlanNode extends NodeBase {
  type: 'plan'
  description?: string
  tags?: string[]
  default_environment_id?: string
  children: PlanNodeChild[]
}

/** Scenario executor node. */
export interface ScenarioNode extends NodeBase {
  type: 'scenario'
  executor: Executor
  vus?: number
  duration?: Duration
  iterations?: number
  stages?: Stage[]
  rate?: number
  time_unit?: Duration
  pre_allocated_vus?: number
  max_vus?: number
  graceful_stop?: Duration
  exec?: string
  start_time?: Duration
  tags?: Record<string, string>
  children: ScenarioNodeChild[]
}

/** HTTP request sampler. */
export interface HttpNode extends NodeBase {
  type: 'http'
  method: HttpMethod
  url: string
  headers?: HeaderEntry[]
  body: HttpBody
  timeout?: Duration
  follow_redirects?: boolean
  tags?: Record<string, string>
  children: SamplerChild[]
}

/** WebSocket request sampler. */
export interface WebSocketNode extends NodeBase {
  type: 'websocket'
  url: string
  connect_timeout?: Duration
  headers?: HeaderEntry[]
  messages?: WsMessage[]
  expected_messages?: WsExpectedMessage[]
  disconnect_behavior?: DisconnectBehavior
  close_timeout?: Duration
  children: SamplerChild[]
}

/** Code block sampler. */
export interface CodeBlockNode extends NodeBase {
  type: 'code-block'
  code: string
  children: SamplerChild[]
}

/** If controller. */
export interface IfControllerNode extends NodeBase {
  type: 'if-controller'
  condition: string
  children: ControllerChild[]
}

/** Else controller (must immediately follow an IfControllerNode). */
export interface ElseControllerNode extends NodeBase {
  type: 'else-controller'
  children: ControllerChild[]
}

/** Loop controller — fixed count or while-condition. */
export interface LoopControllerNode extends NodeBase {
  type: 'loop-controller'
  count?: number
  while_condition?: string
  children: ControllerChild[]
}

/** Transaction controller — groups children for aggregate metrics. */
export interface TransactionControllerNode extends NodeBase {
  type: 'transaction-controller'
  children: ControllerChild[]
}

/** Once-only controller — children run once per VU. */
export interface OnceOnlyControllerNode extends NodeBase {
  type: 'once-only-controller'
  children: ControllerChild[]
}

/** Random controller — picks one child at random per iteration. */
export interface RandomControllerNode extends NodeBase {
  type: 'random-controller'
  weights?: number[]
  children: ControllerChild[]
}

/** Assertion — validates a property of the response. */
export interface AssertionNode extends NodeBase {
  type: 'assertion'
  target: AssertionTarget
  header_name?: string
  condition: AssertionCondition
  value: string | number
  negate?: boolean
  children: never[]
}

/** Timer — inserts a sleep after the parent node. */
export interface TimerNode extends NodeBase {
  type: 'timer'
  timer_type: TimerType
  /** Constant timer delay. */
  duration_ms?: number
  /** Uniform random minimum. */
  min_ms?: number
  /** Uniform random maximum. */
  max_ms?: number
  /** Gaussian mean. */
  mean_ms?: number
  /** Gaussian standard deviation. */
  deviation_ms?: number
  children: never[]
}

/** Data source — provides test data from CSV, JSON, or inline. */
export interface DataSourceNode extends NodeBase {
  type: 'data-source'
  source_type: DataSourceType
  path?: string
  data?: Record<string, unknown>[]
  delimiter?: string
  sharing?: DataSharing
  variable_name: string
  first_row_is_header?: boolean
  children: never[]
}

/** Environment binding — references an Environment entity. */
export interface EnvironmentBindingNode extends NodeBase {
  type: 'environment-binding'
  environment_id: string
  children: never[]
}

// ---------------------------------------------------------------------------
// Discriminated union of all node types
// ---------------------------------------------------------------------------

/** Every concrete node type, discriminated on the `type` field. */
export type AnyNode =
  | PlanNode
  | ScenarioNode
  | HttpNode
  | WebSocketNode
  | CodeBlockNode
  | IfControllerNode
  | ElseControllerNode
  | LoopControllerNode
  | TransactionControllerNode
  | OnceOnlyControllerNode
  | RandomControllerNode
  | AssertionNode
  | TimerNode
  | DataSourceNode
  | EnvironmentBindingNode

/** String literal union of every node type discriminator. */
export type NodeType = AnyNode['type']

// ---------------------------------------------------------------------------
// Child-position unions (what can appear inside each parent)
// ---------------------------------------------------------------------------

/** Nodes that can be direct children of a Plan node. */
export type PlanNodeChild =
  | ScenarioNode
  | DataSourceNode
  | EnvironmentBindingNode

/** Nodes that can appear inside a scenario. */
export type ScenarioNodeChild =
  | HttpNode
  | WebSocketNode
  | CodeBlockNode
  | IfControllerNode
  | ElseControllerNode
  | LoopControllerNode
  | TransactionControllerNode
  | OnceOnlyControllerNode
  | RandomControllerNode
  | AssertionNode
  | TimerNode
  | DataSourceNode

/** Nodes that can appear as children of a sampler (HTTP, WebSocket, CodeBlock). */
export type SamplerChild =
  | AssertionNode
  | TimerNode

/** Nodes that can appear inside a controller. */
export type ControllerChild =
  | HttpNode
  | WebSocketNode
  | CodeBlockNode
  | IfControllerNode
  | ElseControllerNode
  | LoopControllerNode
  | TransactionControllerNode
  | OnceOnlyControllerNode
  | RandomControllerNode
  | AssertionNode
  | TimerNode
  | DataSourceNode

// ---------------------------------------------------------------------------
// Plan document (top-level persisted shape)
// ---------------------------------------------------------------------------

/** A persisted test plan document. */
export interface Plan {
  id: string
  name: string
  description: string
  tags: string[]
  created_at: string
  updated_at: string
  version: number
  default_environment_id?: string
  root: PlanNode
}
