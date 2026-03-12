import type { ApiResponse } from '../types'

interface RequestOptions extends Omit<RequestInit, 'body'> {
  params?: Record<string, string | number | boolean | undefined>
  body?: unknown
}

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api'

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
      ...headers
    },
    body: payload
  })

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`)
  }

  return (await response.json()) as ApiResponse<T>
}
