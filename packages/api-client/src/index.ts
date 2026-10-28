// Typed client for the OpenSynapse REST API.
//
// The types in ./schema.d.ts are generated from the control plane's OpenAPI
// document (`pnpm --filter @opensynapse/api-client generate`); do not edit
// them by hand. CI regenerates them and fails if they drift.

import createClient, { type Client, type Middleware } from 'openapi-fetch'
import type { components, paths } from './schema'

export type { paths, components }
export type Schemas = components['schemas']

export type Plan = Schemas['Plan']
export type Node = Schemas['Node']
export type PlanVersion = Schemas['PlanVersion']
export type Environment = Schemas['Environment']
export type Variable = Schemas['Variable']
export type Run = Schemas['Run']
export type RunParameters = Schemas['RunParameters']
export type RunStage = Schemas['RunStage']
export type RunSummary = Schemas['RunSummary']
export type EndpointStat = Schemas['EndpointStat']
export type CheckStat = Schemas['CheckStat']
export type MetricPoint = Schemas['MetricPoint']
export type RunError = Schemas['RunError']
export type RunEvent = Schemas['RunEvent']
export type RunReport = Schemas['Document']
export type LiveState = Schemas['LiveState']
export type Fragment = Schemas['Fragment']
export type Binding = Schemas['Binding']
export type Crawl = Schemas['Crawl']
export type CapturedRequest = Schemas['CapturedRequest']
export type Collection = Schemas['Collection']
export type SavedRequest = Schemas['SavedRequest']
export type PlaygroundRequest = Schemas['PlaygroundRequest']
export type PlaygroundResponse = Schemas['PlaygroundResponse']
export type MicroTestResult = Schemas['MicroTestResult']
export type AIConfig = Schemas['AIConfig']
export type AIAnalysis = Schemas['AIAnalysis']
export type Settings = Schemas['Settings']
export type SystemInfo = Schemas['SystemInfo']
export type User = Schemas['User']
export type APIToken = Schemas['APIToken']
export type Principal = Schemas['Principal']
export type Session = Schemas['SessionResponse']
export type Comparison = Schemas['ComparisonResult']
export type MetricChange = Schemas['MetricChange']
export type ImportLog = Schemas['ImportLog']

/** A structured API error (docs/04-data-model-and-api.md section 4). */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details?: unknown

  constructor(status: number, code: string, message: string, details?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

export interface ClientOptions {
  /** Origin to call; defaults to the page's own origin. */
  baseUrl?: string
  /** Returns the bearer token to send, if any. */
  getToken?: () => string | null | undefined
  /** Called when the API answers 401 (e.g. to show the sign-in screen). */
  onUnauthorized?: () => void
}

export type ApiClient = Client<paths>

/** Creates a typed client that adds the bearer token to every request. */
export function createApiClient(opts: ClientOptions = {}): ApiClient {
  const client = createClient<paths>({ baseUrl: opts.baseUrl ?? '' })
  const auth: Middleware = {
    onRequest({ request }) {
      const token = opts.getToken?.()
      if (token && !request.headers.has('Authorization')) {
        request.headers.set('Authorization', `Bearer ${token}`)
      }
      return request
    },
    onResponse({ response }) {
      if (response.status === 401) opts.onUnauthorized?.()
      return response
    },
  }
  client.use(auth)
  return client
}

/** Turns an error body (or a thrown network error) into an ApiError. */
export function toApiError(error: unknown, response?: Response): ApiError {
  const status = response?.status ?? 0
  const body = error as { error?: { code?: string; message?: string; details?: unknown } } | undefined
  if (body?.error?.message) {
    return new ApiError(status, body.error.code ?? 'ERROR', body.error.message, body.error.details)
  }
  if (error instanceof Error) return new ApiError(status, 'NETWORK_ERROR', error.message)
  return new ApiError(status, 'ERROR', response ? `Request failed (${response.status})` : 'Request failed')
}

/**
 * Unwraps an openapi-fetch result: returns the data or throws an ApiError.
 *
 *   const plans = await unwrap(api.GET('/api/v1/plans'))
 */
export async function unwrap<T>(
  promise: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  let result: { data?: T; error?: unknown; response: Response }
  try {
    result = await promise
  } catch (e) {
    throw toApiError(e)
  }
  if (result.error !== undefined || !result.response.ok) {
    throw toApiError(result.error, result.response)
  }
  return result.data as T
}
