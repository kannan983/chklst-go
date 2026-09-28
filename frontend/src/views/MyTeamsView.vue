<template>
  <div class="space-y-4">
    <div class="page-header">
      <div>
        <h1 class="page-title text-xl">My Teams</h1>
        <p class="text-gray-400 text-sm">Where your week went in calls — from Teams for Linux over MQTT</p>
      </div>
      <div class="flex items-center gap-3">
        <!-- live state -->
        <span v-if="store.live" class="flex items-center gap-2 px-3 py-1.5 text-xs rounded-full border border-surface-border bg-surface-light">
          <span class="w-2 h-2 rounded-full" :class="liveDot"></span>
          <span class="text-gray-200">{{ liveLabel }}</span>
        </span>
        <!-- week picker -->
        <div v-if="r" class="flex items-center gap-1">
          <button class="p-2 rounded-lg hover:bg-surface-light text-gray-300" title="Previous week" @click="load(r.prev)">
            <ChevronLeft class="w-4 h-4" />
          </button>
          <span class="text-sm text-white min-w-[11rem] text-center">
            Week {{ r.week.split('-W')[1] }} · {{ fmtRange(r.from, r.to) }}
          </span>
          <button class="p-2 rounded-lg hover:bg-surface-light text-gray-300 disabled:opacity-30" title="Next week"
            :disabled="r.to >= today" @click="load(r.next)">
            <ChevronRight class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>

    <p v-if="store.error" class="text-red-400 text-sm">{{ store.error }}</p>
    <p v-if="store.live && !store.live.logger_connected" class="text-yellow-400 text-sm">
      Logger is not connected to the MQTT broker — new events are not being recorded.
    </p>

    <Skeleton v-if="store.isLoading && !r" :rows="5" />

    <EmptyState v-else-if="r && !r.has_data" title="No Teams events yet"
      description="The logger records every Teams state change from MQTT. Join a call and it will show up here." />

    <template v-else-if="r">
      <!-- headline numbers -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Time in calls</p>
          <p class="text-2xl font-bold text-white">{{ dur(r.totals.call_seconds) }}</p>
          <p class="text-[11px] text-gray-500">{{ r.totals.calls }} call(s)<span v-if="r.totals.calls"> · avg {{ dur(r.avg_call_seconds) }}</span></p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Longest call</p>
          <p class="text-2xl font-bold text-blue-400">{{ r.longest_call ? dur(r.longest_call.seconds) : '—' }}</p>
          <p class="text-[11px] text-gray-500">{{ r.longest_call ? fmtDay(r.longest_call.start) : '' }}</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Missed calls</p>
          <p class="text-2xl font-bold" :class="r.totals.missed ? 'text-red-400' : 'text-white'">{{ r.totals.missed }}</p>
          <p class="text-[11px] text-gray-500">rang, not answered within 60s</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Busiest day</p>
          <p class="text-2xl font-bold text-purple-400">{{ r.busiest_day ? longWeekday(r.busiest_day.date) : '—' }}</p>
          <p class="text-[11px] text-gray-500">{{ r.busiest_day ? dur(r.busiest_day.call_seconds) + ' in calls' : '' }}</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">You spoke</p>
          <p class="text-2xl font-bold text-green-400">{{ micPct('speaking') }}%</p>
          <p class="text-[11px] text-gray-500">silent {{ micPct('silent') }}% · muted {{ micPct('muted') }}%</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Camera on</p>
          <p class="text-2xl font-bold text-white">{{ dur(r.totals.camera_seconds) }}</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Screen sharing</p>
          <p class="text-2xl font-bold text-white">{{ dur(r.totals.screen_seconds) }}</p>
        </div>
        <div class="p-4 bg-surface-deeper border border-surface-border rounded-lg">
          <p class="text-gray-400 text-xs">Teams open</p>
          <p class="text-2xl font-bold text-white">{{ dur(r.totals.open_seconds) }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <!-- daily call time -->
        <Card title="Daily call time" class="lg:col-span-2">
          <div class="flex items-end gap-2 h-36">
            <button v-for="d in r.days" :key="d.date" class="flex-1 flex flex-col items-center justify-end h-full group"
              :title="`${d.date}: ${dur(d.call_seconds)} · ${d.calls} call(s)${d.missed ? ` · ${d.missed} missed` : ''}`"
              @click="selected = selected === d.date ? null : d.date">
              <span class="text-[10px] text-gray-400 mb-1">{{ d.call_seconds ? dur(d.call_seconds) : '' }}</span>
              <div class="w-full rounded-t transition-colors"
                :class="selected === d.date ? 'bg-purple-500' : 'bg-accent group-hover:opacity-80'"
                :style="{ height: `${dayBar(d.call_seconds)}%` }"></div>
            </button>
          </div>
          <div class="flex gap-2 mt-1">
            <span v-for="d in r.days" :key="d.date" class="flex-1 text-center text-[11px]"
              :class="[selected === d.date ? 'text-purple-400' : 'text-gray-400', d.date === today && 'font-bold']">
              {{ d.weekday }}<span v-if="d.missed" class="text-red-400"> ·{{ d.missed }}</span>
            </span>
          </div>
          <p class="text-[10px] text-gray-500 mt-2">Click a day to filter the call list. Red number = missed calls.</p>
        </Card>

        <!-- presence -->
        <Card title="Presence">
          <div v-if="statusTotal" class="space-y-3">
            <div class="flex h-3 rounded overflow-hidden">
              <div v-for="s in statuses" :key="s.name" :class="statusColor(s.name)"
                :style="{ width: `${(s.seconds / statusTotal) * 100}%` }" :title="`${s.name}: ${dur(s.seconds)}`"></div>
            </div>
            <div v-for="s in statuses" :key="s.name" class="flex items-center justify-between text-sm">
              <span class="flex items-center gap-2 text-gray-200 capitalize">
                <span class="w-2.5 h-2.5 rounded-sm" :class="statusColor(s.name)"></span>{{ s.name }}
              </span>
              <span class="text-gray-300">{{ dur(s.seconds) }}</span>
            </div>
          </div>
          <p v-else class="text-gray-400 text-sm">No presence data this week.</p>
        </Card>
      </div>

      <!-- calls -->
      <Card :title="selected ? `Calls on ${longWeekday(selected)} ${selected}` : 'Calls this week'">
        <template v-if="selected" #header-right>
          <button class="text-xs text-accent hover:underline" @click="selected = null">Show all</button>
        </template>
        <div v-if="calls.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-xs text-gray-400 border-b border-surface-border">
                <th class="py-2 pr-4 font-medium">When</th>
                <th class="py-2 pr-4 font-medium">Duration</th>
                <th class="py-2 pr-4 font-medium">Spoke</th>
                <th class="py-2 pr-4 font-medium">Muted</th>
                <th class="py-2 pr-4 font-medium">Camera</th>
                <th class="py-2 pr-4 font-medium">Screen share</th>
                <th class="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in calls" :key="c.start" class="border-b border-surface-border/50 last:border-0">
                <td class="py-2 pr-4 text-white whitespace-nowrap">{{ fmtDay(c.start) }} · {{ fmtTime(c.start) }}–{{ c.ongoing ? 'now' : fmtTime(c.end) }}</td>
                <td class="py-2 pr-4 text-gray-200">{{ dur(c.seconds) }}</td>
                <td class="py-2 pr-4 text-gray-300">{{ pct(c.speaking_seconds, c.seconds) }}</td>
                <td class="py-2 pr-4 text-gray-300">{{ pct(c.muted_seconds, c.seconds) }}</td>
                <td class="py-2 pr-4 text-gray-300">{{ c.camera_seconds ? dur(c.camera_seconds) : '—' }}</td>
                <td class="py-2 pr-4 text-gray-300">{{ c.screen_seconds ? dur(c.screen_seconds) : '—' }}</td>
                <td class="py-2 text-xs whitespace-nowrap">
                  <span v-if="c.ongoing" class="text-green-400">● live</span>
                  <span v-else-if="c.closed_by_disconnect" class="text-yellow-400"
                    title="Teams or the logger disconnected — end time is approximate">closed by disconnect</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-gray-400 text-sm text-center py-2">No calls {{ selected ? 'on this day' : 'this week' }}.</p>

        <div v-if="missed.length" class="mt-4 pt-3 border-t border-surface-border">
          <p class="text-xs text-gray-400 mb-1">Missed</p>
          <p class="text-sm text-red-400">{{ missed.map(t => `${fmtDay(t)} ${fmtTime(t)}`).join(' · ') }}</p>
        </div>
      </Card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import Card from '../components/ui/Card.vue'
import Skeleton from '../components/ui/Skeleton.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import { useTeamsStore } from '../stores/teams'

const store = useTeamsStore()
const r = computed(() => store.report)
const selected = ref<string | null>(null)
const week = ref('current')

const localDate = (d = new Date()) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const today = localDate()

const dur = (s: number) => {
  if (!s) return '0m'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`
  return m ? `${m}m` : `${s}s`
}
const pct = (part: number, whole: number) => (whole ? `${Math.round((part / whole) * 100)}%` : '—')
const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
const fmtDay = (iso: string) => new Date(iso).toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' })
const longWeekday = (date: string) => new Date(`${date}T12:00:00`).toLocaleDateString([], { weekday: 'long' })
const fmtRange = (from: string, to: string) => {
  const f = new Date(`${from}T12:00:00`)
  const t = new Date(`${to}T12:00:00`)
  return `${f.getDate()} ${f.toLocaleDateString([], { month: 'short' })} – ${t.getDate()} ${t.toLocaleDateString([], { month: 'short', year: 'numeric' })}`
}

const dayBar = (s: number) => {
  const max = Math.max(1, ...(r.value?.days.map(d => d.call_seconds) || [1]))
  return Math.max(s > 0 ? 4 : 0, (s / max) * 90)
}

const micTotal = computed(() => {
  const m = r.value?.totals.mic || {}
  return (m.speaking || 0) + (m.silent || 0) + (m.muted || 0)
})
const micPct = (k: string) => (micTotal.value ? Math.round(((r.value?.totals.mic[k] || 0) / micTotal.value) * 100) : 0)

const statuses = computed(() =>
  Object.entries(r.value?.totals.status || {})
    .map(([name, seconds]) => ({ name, seconds }))
    .sort((a, b) => b.seconds - a.seconds)
)
const statusTotal = computed(() => statuses.value.reduce((n, s) => n + s.seconds, 0))
const statusColor = (name: string) => {
  const n = name.replace(/[^a-z]/g, '')
  if (n.startsWith('available')) return 'bg-green-500'
  if (n.startsWith('busy') || n.includes('call') || n.includes('meeting')) return 'bg-red-500'
  if (n.includes('donotdisturb') || n === 'dnd' || n.includes('presenting')) return 'bg-red-800'
  if (n.startsWith('away') || n.startsWith('berightback') || n === 'brb') return 'bg-yellow-500'
  return 'bg-gray-500'
}

const calls = computed(() => (r.value?.calls || []).filter(c => !selected.value || c.start.slice(0, 10) === selected.value))
const missed = computed(() => (r.value?.missed_at || []).filter(t => !selected.value || t.slice(0, 10) === selected.value))

const liveDot = computed(() => {
  const s = store.live?.state || {}
  if (!store.live?.logger_connected || s.connected !== 'true') return 'bg-gray-500'
  if (s['in-call'] === 'true') return 'bg-red-500 animate-pulse'
  return statusColor(s.status || '')
})
const liveLabel = computed(() => {
  const s = store.live?.state || {}
  if (!store.live?.logger_connected) return 'Logger offline'
  if (s.connected !== 'true') return 'Teams closed'
  if (s['in-call'] === 'true') {
    const mic = s.microphone === 'speaking' ? 'speaking' : s.microphone === 'muted' ? 'muted' : ''
    return `In a call${mic ? ` · ${mic}` : ''}`
  }
  return s.status ? s.status[0].toUpperCase() + s.status.slice(1) : 'Online'
})

const load = (w: string) => {
  week.value = w
  selected.value = null
  store.fetchReport(w)
}

// Live pill every 15s; the current week's numbers every minute.
let timer: number | undefined
let ticks = 0
onMounted(() => {
  load('current')
  store.fetchLive()
  timer = window.setInterval(() => {
    store.fetchLive()
    if (++ticks % 4 === 0 && r.value && r.value.to >= today) store.fetchReport(week.value)
  }, 15000)
})
onUnmounted(() => window.clearInterval(timer))
</script>

