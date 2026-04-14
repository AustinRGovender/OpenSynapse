/**
 * OpenSynapse API client.
 *
 * Hand-written for Phase 1. Will be auto-generated from OpenAPI spec
 * via openapi-typescript in a later phase.
 */

// --- Types matching the OpenAPI spec ---

export interface Node {
  id: string
  type: string
  name: string
  enabled: boolean
  properties: Record<string, unknown>
  children: Node[]
}

export interface Plan {
  id: string
  name: string
  description: string
  tags: string[]
  created_at: string
  updated_at: string
  version: number
  default_environment_id: string | null
  root: Node
}

export interface PlanVersion {
  id: string
  plan_id: string
  version: number
  root: Node
  name: string
  description: string
  tags: string[]
  created_at: string
}

export interface Variable {
  value: string
  secret: boolean
}

export interface Environment {
  id: string
  name: string
  variables: Record<string, Variable>
  created_at: string
  updated_at: string
}

export interface ListResult<T> {
  items: T[]
  next_cursor?: string
}

export interface ValidationResult {
  valid: boolean
  errors: string[]
}

export interface APIError {
  error: {
    code: string
    message: string
    details?: unknown
  }
}

// --- Request types ---

export interface CreatePlanRequest {
  name: string
  description?: string
  tags?: string[]
  default_environment_id?: string | null
  root: Node
}

export interface UpdatePlanRequest {
  name: string
  description?: string
  tags?: string[]
  default_environment_id?: string | null
  root: Node
}

export interface CreateEnvironmentRequest {
  name: string
  variables?: Record<string, Variable>
}

export interface UpdateEnvironmentRequest {
  name: string
  variables?: Record<string, Variable>
}

// --- Client ---

export class OpenSynapseClient {
  private baseUrl: string

  constructor(baseUrl: string = '/api/v1') {
    this.baseUrl = baseUrl
  }

  // System
  async health(): Promise<{ status: string }> {
    return this.get('/health', true)
  }

  async version(): Promise<{ version: string }> {
    return this.get('/version', true)
  }

  // Plans
  async listPlans(limit?: number, cursor?: string): Promise<ListResult<Plan>> {
    const params = new URLSearchParams()
    if (limit) params.set('limit', String(limit))
    if (cursor) params.set('cursor', cursor)
    const qs = params.toString()
    return this.get(`/plans${qs ? '?' + qs : ''}`)
  }

  async createPlan(req: CreatePlanRequest): Promise<Plan> {
    return this.post('/plans', req)
  }

  async getPlan(id: string): Promise<Plan> {
    return this.get(`/plans/${id}`)
  }

  async updatePlan(id: string, req: UpdatePlanRequest): Promise<Plan> {
    return this.put(`/plans/${id}`, req)
  }

  async deletePlan(id: string): Promise<void> {
    return this.del(`/plans/${id}`)
  }

  async listPlanVersions(id: string): Promise<{ items: PlanVersion[] }> {
    return this.get(`/plans/${id}/versions`)
  }

  async getPlanVersion(id: string, version: number): Promise<PlanVersion> {
    return this.get(`/plans/${id}/versions/${version}`)
  }

  async validatePlan(id: string, root: Node): Promise<ValidationResult> {
    return this.post(`/plans/${id}/validate`, { root })
  }

  // Environments
  async listEnvironments(limit?: number, cursor?: string): Promise<ListResult<Environment>> {
    const params = new URLSearchParams()
    if (limit) params.set('limit', String(limit))
    if (cursor) params.set('cursor', cursor)
    const qs = params.toString()
    return this.get(`/environments${qs ? '?' + qs : ''}`)
  }

  async createEnvironment(req: CreateEnvironmentRequest): Promise<Environment> {
    return this.post('/environments', req)
  }

  async getEnvironment(id: string): Promise<Environment> {
    return this.get(`/environments/${id}`)
  }

  async updateEnvironment(id: string, req: UpdateEnvironmentRequest): Promise<Environment> {
    return this.put(`/environments/${id}`, req)
  }

  async deleteEnvironment(id: string): Promise<void> {
    return this.del(`/environments/${id}`)
  }

  // --- HTTP helpers ---

  private async get<T>(path: string, absolute = false): Promise<T> {
    const url = absolute ? path : `${this.baseUrl}${path}`
    const res = await fetch(url)
    if (!res.ok) throw await this.parseError(res)
    return res.json()
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw await this.parseError(res)
    return res.json()
  }

  private async put<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw await this.parseError(res)
    return res.json()
  }

  private async del(path: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}${path}`, { method: 'DELETE' })
    if (!res.ok) throw await this.parseError(res)
  }

  private async parseError(res: Response): Promise<APIError> {
    try {
      return await res.json()
    } catch {
      return { error: { code: 'UNKNOWN', message: res.statusText } }
    }
  }
}
