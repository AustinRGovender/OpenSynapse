// The app's single API client. Every request carries the bearer token from
// the auth store; a 401 sends the user back to the sign-in screen.
import { ApiError, createApiClient, toApiError, unwrap } from '@opensynapse/api-client'
import { useAuthStore } from './stores/auth-store'

export const api = createApiClient({
  getToken: () => useAuthStore.getState().token,
  onUnauthorized: () => useAuthStore.getState().handleUnauthorized(),
})

export { ApiError, unwrap }

/** Human-readable message for any thrown value. */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return String(e)
}

/** Raw fetch with the auth header, for downloads and non-JSON bodies. */
export async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  const token = useAuthStore.getState().token
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const res = await fetch(path, { ...init, headers })
  if (res.status === 401) useAuthStore.getState().handleUnauthorized()
  if (!res.ok) {
    let body: unknown
    try {
      body = await res.json()
    } catch {
      body = undefined
    }
    throw toApiError(body, res)
  }
  return res
}

/** POSTs a JSON body and saves the response as a file. */
export async function downloadFile(path: string, body: unknown, fallbackName: string): Promise<void> {
  const res = await authFetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const disposition = res.headers.get('Content-Disposition') ?? ''
  const name = /filename="([^"]+)"/.exec(disposition)?.[1] ?? fallbackName
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** WebSocket URL for the realtime API on this origin. */
export function wsUrl(): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}/api/v1/ws`
}
