import type { Node } from '@opensynapse/api-client'

// The nine load-profile templates from F-01. Scenario properties use the
// plan schema's snake_case keys (the compiler ignores anything else).

export interface Threshold {
  metric: string
  condition: string
  abort_on_fail?: boolean
}

export interface TemplateConfig {
  name: string
  description: string
  svgPath: string
  scenarioProperties: Record<string, unknown>
  /** Plan-level k6 thresholds (pass/fail criteria). */
  thresholds?: Threshold[]
}

/** Ramps quickly to each level and holds it, producing a staircase. */
function plateaus(levels: number[], hold: string, ramp = '30s') {
  return levels.flatMap((target) => [
    { duration: ramp, target },
    { duration: hold, target },
  ])
}

export const TEMPLATES: TemplateConfig[] = [
  {
    name: 'Smoke',
    description: 'Verify the test plan works before loading it',
    svgPath: 'M 10,65 L 190,65',
    scenarioProperties: { executor: 'constant-vus', vus: 1, duration: '30s' },
    thresholds: [{ metric: 'http_req_failed', condition: 'rate<0.01', abort_on_fail: true }],
  },
  {
    name: 'Load',
    description: 'How does it behave under typical expected traffic?',
    svgPath: 'M 10,70 L 40,25 L 140,25 L 170,70',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '2m', target: 50 },
        { duration: '10m', target: 50 },
        { duration: '2m', target: 0 },
      ],
    },
  },
  {
    name: 'Stress',
    description: 'Where is the knee? Staircase of 50, 100, 200, 400 VUs',
    svgPath: 'M 10,70 L 10,55 L 55,55 L 55,40 L 100,40 L 100,25 L 145,25 L 145,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [...plateaus([50, 100, 200, 400], '3m'), { duration: '1m', target: 0 }],
    },
  },
  {
    name: 'Spike',
    description: 'Can it survive a sudden burst and recover?',
    svgPath: 'M 10,65 L 60,65 L 80,10 L 120,10 L 140,65 L 190,65',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '10s', target: 10 },
        { duration: '1m', target: 10 },
        { duration: '5s', target: 500 },
        { duration: '30s', target: 500 },
        { duration: '5s', target: 10 },
        { duration: '1m', target: 10 },
      ],
    },
  },
  {
    name: 'Soak',
    description: 'Does it stay stable for hours? (4h; configure longer soaks explicitly)',
    svgPath: 'M 10,40 L 190,40',
    scenarioProperties: { executor: 'constant-vus', vus: 50, duration: '4h' },
  },
  {
    name: 'Breakpoint',
    description: 'Add 10 VUs a minute until errors pass 5%',
    svgPath: 'M 10,70 L 160,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      start_vus: 10,
      // +10 VUs per minute; the threshold below ends the run at the breaking point.
      stages: [{ duration: '100m', target: 1010 }],
    },
    thresholds: [{ metric: 'http_req_failed', condition: 'rate<0.05', abort_on_fail: true }],
  },
  {
    name: 'Trickle Feed',
    description: '1 request per second for an hour',
    svgPath: 'M 10,58 L 190,58',
    scenarioProperties: {
      executor: 'constant-arrival-rate',
      rate: 1,
      time_unit: '1s',
      duration: '1h',
      pre_allocated_vus: 2,
      max_vus: 10,
    },
  },
  {
    name: 'Ramp-up',
    description: 'Linear increase from 0 to 200 VUs over 10 minutes',
    svgPath: 'M 10,70 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [{ duration: '10m', target: 200 }],
    },
  },
  {
    name: 'Step Load',
    description: '+50 VUs every 2 minutes, holding each plateau, up to 500',
    svgPath: 'M 10,70 L 10,58 L 46,58 L 46,46 L 82,46 L 82,34 L 118,34 L 118,22 L 154,22 L 154,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: plateaus([50, 100, 150, 200, 250, 300, 350, 400, 450, 500], '2m', '10s'),
    },
  },
]

const uid = () => crypto.randomUUID()

/** A ready-to-run plan: the template's scenario with one sample request. */
export function buildPlanRoot(template: TemplateConfig | null): Node {
  const name = template ? `New ${template.name} test` : 'New test'
  const request: Node = {
    id: uid(),
    type: 'http',
    name: 'GET /health',
    enabled: true,
    properties: { method: 'GET', url: '${ECHO_API}/health', headers: [], body: { type: 'none' } },
    children: [
      {
        id: uid(),
        type: 'assertion',
        name: 'status is 200',
        enabled: true,
        properties: { target: 'status', condition: 'equals', value: 200 },
        children: [],
      },
    ],
  }
  const think: Node = {
    id: uid(),
    type: 'timer',
    name: 'Think 1s',
    enabled: true,
    properties: { timer_type: 'constant', duration_ms: 1000 },
    children: [],
  }
  return {
    id: uid(),
    type: 'plan',
    name,
    enabled: true,
    properties: template?.thresholds ? { thresholds: template.thresholds } : {},
    children: [
      {
        id: uid(),
        type: 'scenario',
        name: template ? `${template.name} profile` : 'Main',
        enabled: true,
        properties: template?.scenarioProperties ?? { executor: 'constant-vus', vus: 1, duration: '30s' },
        children: [request, think],
      },
    ],
  }
}
