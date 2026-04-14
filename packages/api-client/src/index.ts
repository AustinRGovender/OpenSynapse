/**
 * OpenSynapse API client.
 * This will be auto-generated from the OpenAPI spec in later phases.
 * For now, export a minimal health check client.
 */

export interface HealthResponse {
  status: string
}

export interface VersionResponse {
  version: string
}

const DEFAULT_BASE_URL = ''

export class OpenSynapseClient {
  private baseUrl: string

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl
  }

  async health(): Promise<HealthResponse> {
    const res = await fetch(`${this.baseUrl}/health`)
    return res.json()
  }

  async version(): Promise<VersionResponse> {
    const res = await fetch(`${this.baseUrl}/version`)
    return res.json()
  }
}
