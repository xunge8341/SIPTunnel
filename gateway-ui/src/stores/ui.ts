import { defineStore } from 'pinia'
import type { MessageInstance } from 'ant-design-vue/es/message/interface'
import type { GlobalMessage } from '../types'

interface UiState {
  messageApi: MessageInstance | null
}

export const useUiStore = defineStore('ui', {
  state: (): UiState => ({
    messageApi: null
  }),
  actions: {
    registerMessageApi(messageApi: MessageInstance) {
      this.messageApi = messageApi
    },
    notify(payload: GlobalMessage) {
      this.messageApi?.[payload.type](payload.content)
    }
  }
})
