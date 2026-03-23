import type { ApiResponse } from '../types'

interface RequestOptions extends Omit<RequestInit, 'body'> {
  params?: Record<string, string | number | boolean | undefined>
  body?: unknown
}

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api'

const browserAdminHeaders = () => {
  if (typeof window === 'undefined') return {}
  const read = (key: string) => window.localStorage.getItem(key) || window.sessionStorage.getItem(key) || ''
  const token = read('siptunnel.adminToken').trim()
  const mfa = read('siptunnel.adminMfa').trim()
  const operator = read('siptunnel.adminOperator').trim()
  return {
    ...(token ? { 'X-Admin-Token': token } : {}),
    ...(mfa ? { 'X-Admin-MFA': mfa } : {}),
    ...(operator ? { 'X-Admin-Operator': operator } : {})
  }
}

const withQuery = (url: string, params?: RequestOptions['params']) => {
  if (!params) return url
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      query.append(key, String(value))
    }
  })
  const queryString = query.toString()
  return queryString ? `${url}?${queryString}` : url
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<ApiResponse<T>> {
  const { params, headers, body, ...rest } = options
  const url = withQuery(`${API_BASE}${path}`, params)

  const payload = body === undefined || body === null ? undefined : typeof body === 'string' ? body : JSON.stringify(body)

  const response = await fetch(url, {
    ...rest,
    headers: {
      'Content-Type': 'application/json',
      ...browserAdminHeaders(),
      ...headers
    },
    body: payload
  })

  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = await response.json() as { message?: string; error?: { message?: string }; data?: { message?: string } }
      detail = payload?.error?.message || payload?.message || payload?.data?.message || detail
    } catch {
      // ignore non-json body
    }
    throw new Error(`HTTP ${response.status}: ${detail}`)
  }

  return (await response.json()) as ApiResponse<T>
}
