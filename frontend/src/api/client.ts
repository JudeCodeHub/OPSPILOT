import { apiHealth } from './health'
import type { ApiErrorBody } from './types'

/** Base path of the API. Relative, so the same build works behind any host. */
const BASE = '/api'

/**
 * An API failure. `requestId` is always set: we send our own X-Request-ID with
 * every call, so even a network error can be traced in the server logs.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId: string

  constructor(status: number, code: string, message: string, requestId: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}

function newRequestId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `web-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

export interface ApiResponse<T> {
  data: T
  response: Response
}

/** Sends a request and parses the JSON body. Throws ApiError on any failure. */
export async function request<T>(method: string, path: string, body?: unknown): Promise<ApiResponse<T>> {
  const sentId = newRequestId()
  const headers: Record<string, string> = { Accept: 'application/json', 'X-Request-ID': sentId }
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  let response: Response
  try {
    response = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    apiHealth.failure()
    throw new ApiError(0, 'NETWORK_ERROR', 'Network request failed', sentId)
  }

  const requestId = response.headers.get('X-Request-ID') ?? sentId
  const text = await response.text()
  let json: unknown
  try {
    json = text ? JSON.parse(text) : undefined
  } catch {
    json = undefined
  }

  if (!response.ok) {
    // Only server-side failures count towards "degraded"; a 409 is normal.
    if (response.status >= 500) apiHealth.failure()
    else apiHealth.success()

    const err = (json as ApiErrorBody | undefined)?.error
    if (err?.code) {
      throw new ApiError(response.status, err.code, err.message, err.request_id || requestId)
    }
    // A non-JSON error usually comes from a proxy in front of the gateway.
    const code = response.status >= 502 && response.status <= 504 ? 'UPSTREAM_UNAVAILABLE' : 'INTERNAL_ERROR'
    throw new ApiError(response.status, code, `HTTP ${response.status}`, requestId)
  }

  apiHealth.success()
  return { data: json as T, response }
}
