export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export interface ApiError {
  code: number
  message: string
}

export interface NavigationItem {
  key: string
  label: string
  path: string
}

export interface PageMeta {
  title: string
  description: string
}

export interface GlobalMessage {
  type: 'success' | 'info' | 'warning' | 'error'
  content: string
}
