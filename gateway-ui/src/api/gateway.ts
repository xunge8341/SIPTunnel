import { request } from './http'
import type { PageMeta } from '../types'

export const gatewayApi = {
  fetchDashboardMeta() {
    return request<PageMeta>('/dashboard/meta', { method: 'GET' })
  }
}
