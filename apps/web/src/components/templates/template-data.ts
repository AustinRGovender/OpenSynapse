export interface TemplateConfig {
  name: string
  description: string
  svgPath: string
  scenarioProperties: Record<string, unknown>
}

export const TEMPLATES: TemplateConfig[] = [
  {
    name: 'Smoke',
    description: 'Verify the test plan works',
    svgPath: 'M 10,65 L 190,65',
    scenarioProperties: { executor: 'constant-vus', vus: 1, duration: '30s' },
  },
  {
    name: 'Load',
    description: 'Sustained typical traffic',
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
    description: 'Find the breaking point',
    svgPath: 'M 10,70 L 10,55 L 55,55 L 55,40 L 100,40 L 100,25 L 145,25 L 145,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '3m', target: 50 },
        { duration: '3m', target: 100 },
        { duration: '3m', target: 200 },
        { duration: '3m', target: 400 },
      ],
    },
  },
  {
    name: 'Spike',
    description: 'Sudden burst and drop',
    svgPath: 'M 10,65 L 60,65 L 80,10 L 120,10 L 140,65 L 190,65',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '1m', target: 10 },
        { duration: '10s', target: 500 },
        { duration: '30s', target: 500 },
        { duration: '10s', target: 10 },
        { duration: '1m', target: 10 },
      ],
    },
  },
  {
    name: 'Soak',
    description: 'Long-duration stability',
    svgPath: 'M 10,40 L 190,40',
    scenarioProperties: { executor: 'constant-vus', vus: 50, duration: '4h' },
  },
  {
    name: 'Breakpoint',
    description: 'Ramp until failure',
    svgPath: 'M 10,70 L 160,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [{ duration: '10m', target: 1000 }],
      startVUs: 10,
      gracefulRampDown: '0s',
    },
  },
  {
    name: 'Trickle Feed',
    description: 'Low constant rate for endurance',
    svgPath: 'M 10,58 L 190,58',
    scenarioProperties: {
      executor: 'constant-arrival-rate',
      rate: 1,
      timeUnit: '1s',
      duration: '1h',
      preAllocatedVUs: 2,
    },
  },
  {
    name: 'Ramp-up',
    description: 'Linear increase only',
    svgPath: 'M 10,70 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [{ duration: '10m', target: 200 }],
    },
  },
  {
    name: 'Step Load',
    description: 'Discrete plateaus',
    svgPath: 'M 10,70 L 10,58 L 46,58 L 46,46 L 82,46 L 82,34 L 118,34 L 118,22 L 154,22 L 154,15 L 190,15',
    scenarioProperties: {
      executor: 'ramping-vus',
      stages: [
        { duration: '2m', target: 50 },
        { duration: '2m', target: 100 },
        { duration: '2m', target: 150 },
        { duration: '2m', target: 200 },
        { duration: '2m', target: 250 },
        { duration: '2m', target: 300 },
        { duration: '2m', target: 350 },
        { duration: '2m', target: 400 },
        { duration: '2m', target: 450 },
        { duration: '2m', target: 500 },
      ],
    },
  },
]
