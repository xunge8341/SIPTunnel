export const GB_CODE_PATTERN = /^\d{20}$/

export type ResourceType = 'SERVICE' | 'CAMERA' | 'OTHER'
export type NodeType = 'SERVER'
export type ResponseMode = 'AUTO' | 'INLINE' | 'RTP'

export const resourceTypeOptions = [
  { label: '服务类型', value: 'SERVICE' },
  { label: '摄像机', value: 'CAMERA' },
  { label: '其他', value: 'OTHER' }
]

export const methodOptions = ['*', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].map((value) => ({ label: value, value }))

export const responseModeOptions = [
  { label: 'AUTO', value: 'AUTO' },
  { label: 'INLINE', value: 'INLINE' },
  { label: 'RTP', value: 'RTP' }
]

export const isGBCode20 = (value?: string) => GB_CODE_PATTERN.test(String(value || '').trim())

export const normalizeMethodSelection = (values?: string[]) => {
  const normalized = Array.from(new Set((values || []).map((item) => String(item || '').trim().toUpperCase()).filter(Boolean)))
  if (normalized.includes('*')) return ['*']
  return normalized.length ? normalized : ['*']
}

export const deriveBodyLimitProfile = (responseMode: ResponseMode | string, networkMode?: string) => {
  const normalizedMode = String(responseMode || 'AUTO').toUpperCase() as ResponseMode
  const supportsLargeRequest = String(networkMode || '').toUpperCase() === 'SENDER_SIP_RTP__RECEIVER_SIP_RTP'
  const profile = {
    maxInlineResponseBody: 64 * 1024,
    maxRequestBody: supportsLargeRequest ? 8 * 1024 * 1024 : 64 * 1024,
    maxResponseBody: 64 * 1024,
    policyLabel: 'SIP 小体量'
  }
  if (normalizedMode === 'INLINE') {
    profile.policyLabel = 'SIP 内联'
    profile.maxResponseBody = profile.maxInlineResponseBody
    return profile
  }
  if (normalizedMode === 'RTP') {
    profile.policyLabel = 'RTP 回传'
    profile.maxRequestBody = supportsLargeRequest ? 8 * 1024 * 1024 : 512 * 1024
    profile.maxResponseBody = 64 * 1024 * 1024
    return profile
  }
  profile.policyLabel = 'AUTO 自适应'
  profile.maxRequestBody = supportsLargeRequest ? 8 * 1024 * 1024 : 512 * 1024
  profile.maxResponseBody = 64 * 1024 * 1024
  return profile
}

const RESOURCE_TYPE_DIGIT: Record<string, string> = {
  SERVICE: '2',
  CAMERA: '1',
  OTHER: '9',
  SERVER: '2'
}

export const generateGBCode = (type: ResourceType | NodeType | string = 'SERVICE') => {
  const typeDigit = RESOURCE_TYPE_DIGIT[String(type || 'SERVICE').toUpperCase()] || '9'
  const serial = `${Date.now()}${Math.floor(Math.random() * 1000)}`.slice(-9).padStart(9, '0')
  return `3402000000${typeDigit}${serial}`
}
