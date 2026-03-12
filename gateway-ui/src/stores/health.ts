import { defineStore } from 'pinia'

interface HealthState {
  status: string
}

export const useHealthStore = defineStore('health', {
  state: (): HealthState => ({
    status: 'ok'
  })
})
