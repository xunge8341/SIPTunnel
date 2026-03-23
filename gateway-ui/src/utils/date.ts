const UNIFIED_DATETIME_REGEX = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?$/

const pad = (value: number, size = 2) => String(value).padStart(size, '0')

const formatLocalDateTime = (parsed: Date) => `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())} ${pad(parsed.getHours())}:${pad(parsed.getMinutes())}:${pad(parsed.getSeconds())}.${pad(parsed.getMilliseconds(), 3)}`

const parseUnifiedLocalDate = (text: string) => {
  const matched = UNIFIED_DATETIME_REGEX.exec(text)
  if (!matched) return null
  const [, year, month, day, hour, minute, second, fractional = '0'] = matched
  const millis = Number((fractional + '000').slice(0, 3))
  return new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute), Number(second), millis)
}

export const formatDateTimeText = (value?: string | null, fallback = '-') => {
  const text = String(value ?? '').trim()
  if (!text) return fallback

  const unifiedLocal = parseUnifiedLocalDate(text)
  if (unifiedLocal) {
    return formatLocalDateTime(unifiedLocal)
  }

  const parsed = new Date(text)
  if (Number.isNaN(parsed.getTime())) {
    return text
  }

  return formatLocalDateTime(parsed)
}
