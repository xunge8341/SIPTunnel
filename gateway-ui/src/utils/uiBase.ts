const normalizeRouterBase = (raw?: string | null) => {
  const text = (raw ?? '').trim()
  if (!text || text.includes('__SIPTUNNEL_UI_BASE_PATH__')) return '/'
  let value = text
  if (!value.startsWith('/')) {
    value = `/${value}`
  }
  if (!value.endsWith('/')) {
    value = `${value}/`
  }
  return value.replace(/\/+/g, '/')
}

export const resolveUIRouterBase = () => {
  if (typeof document !== 'undefined') {
    const meta = document.querySelector('meta[name="siptunnel-ui-base-path"]')
    const fromMeta = normalizeRouterBase(meta?.getAttribute('content'))
    if (fromMeta !== '/') {
      return fromMeta
    }
  }
  return normalizeRouterBase(import.meta.env.BASE_URL)
}
