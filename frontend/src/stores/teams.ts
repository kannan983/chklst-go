import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useApi } from '../composables/useApi'

export interface TeamsStats {
  call_seconds: number
  calls: number
  missed: number
  camera_seconds: number
  screen_seconds: number
  open_seconds: number
  mic: Record<string, number>
  status: Record<string, number>
}

export interface TeamsDay extends TeamsStats {
  date: string
  weekday: string
}

export interface TeamsCall {
  start: string
  end: string
  seconds: number
  speaking_seconds: number
  muted_seconds: number
  camera_seconds: number
  screen_seconds: number
  closed_by_disconnect: boolean
  ongoing: boolean
}

export interface TeamsReport {
  week: string
  prev: string
  next: string
  from: string
  to: string
  totals: TeamsStats
  avg_call_seconds: number
  longest_call: TeamsCall | null
  busiest_day: TeamsDay | null
  days: TeamsDay[]
  calls: TeamsCall[]
  missed_at: string[]
  has_data: boolean
}

export interface TeamsLive {
  logger_connected: boolean
  state: Record<string, string>
  updated_at: string
}

export const useTeamsStore = defineStore('teams', () => {
  const report = ref<TeamsReport | null>(null)
  const live = ref<TeamsLive | null>(null)
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const { get } = useApi()

  const fetchReport = async (week = 'current') => {
    isLoading.value = true
    error.value = null
    try {
      const r = await get<TeamsReport>(`/teams/report?week=${encodeURIComponent(week)}`)
      report.value = r.data
    } catch (err: any) {
      error.value = err?.response?.data?.error || 'Failed to load Teams report'
    } finally {
      isLoading.value = false
    }
  }

  const fetchLive = async () => {
    try {
      live.value = (await get<TeamsLive>('/teams/live')).data
    } catch {
      live.value = null
    }
  }

  return { report, live, isLoading, error, fetchReport, fetchLive }
})
