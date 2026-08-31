// useTimeRange.ts —— 时间范围 5 档控件状态。
// 与 UsagePage.vue 共用 localStorage 键 cli-proxy-usage-time-range-v1:跨页范围保持一致。
// UsagePage.vue 内联实现暂未迁移到此(生产稳定优先),新页面一律用本 composable。
import { ref } from 'vue'

export type RangeMode = 'hour' | 'day' | 'today' | 'yesterday' | 'custom'
export const RANGE_MODES: RangeMode[] = ['hour', 'day', 'today', 'yesterday', 'custom']

const LS_KEY = 'cli-proxy-usage-time-range-v1'

export function startOfDay(offsetDays = 0): number {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() + offsetDays)
  return d.getTime()
}

export function toLocalInput(ts: number): string {
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

export function useTimeRange() {
  const rangeMode = ref<RangeMode>('today')
  const rollingHours = ref(8)
  const rollingDays = ref(7)
  const customUnit = ref<'hour' | 'day'>('day')
  const customStart = ref('')
  const customEnd = ref('')

  function rangeMs() {
    const now = Date.now()
    switch (rangeMode.value) {
      case 'hour':
        return { start_ts: now - rollingHours.value * 3600_000, end_ts: now }
      case 'day':
        return { start_ts: now - rollingDays.value * 86400_000, end_ts: now }
      case 'today':
        return { start_ts: startOfDay(), end_ts: now }
      case 'yesterday': {
        const s = startOfDay(-1)
        return { start_ts: s, end_ts: s + 86400_000 }
      }
      case 'custom': {
        let s = customStart.value ? new Date(customStart.value).getTime() : startOfDay(-7)
        let e = customEnd.value ? new Date(customEnd.value).getTime() : now
        if (Number.isNaN(s)) s = startOfDay(-7)
        if (Number.isNaN(e)) e = now
        return { start_ts: Math.min(s, e), end_ts: Math.max(s, e) }
      }
    }
  }

  function persistRange() {
    try {
      localStorage.setItem(LS_KEY, JSON.stringify({
        mode: rangeMode.value,
        rollingHours: rollingHours.value,
        rollingDays: rollingDays.value,
        customUnit: customUnit.value,
        customStart: customStart.value,
        customEnd: customEnd.value,
      }))
    } catch { /* ignore */ }
  }

  function restoreRange() {
    try {
      const raw = localStorage.getItem(LS_KEY)
      if (!raw) return
      const v: unknown = JSON.parse(raw)
      if (typeof v !== 'object' || v === null) return
      const o = v as Record<string, unknown>
      if (RANGE_MODES.includes(o.mode as RangeMode)) rangeMode.value = o.mode as RangeMode
      if (typeof o.rollingHours === 'number') rollingHours.value = o.rollingHours
      if (typeof o.rollingDays === 'number') rollingDays.value = o.rollingDays
      if (o.customUnit === 'hour' || o.customUnit === 'day') customUnit.value = o.customUnit
      if (typeof o.customStart === 'string') customStart.value = o.customStart
      if (typeof o.customEnd === 'string') customEnd.value = o.customEnd
    } catch { /* ignore */ }
  }

  return {
    rangeMode, rollingHours, rollingDays, customUnit, customStart, customEnd,
    rangeMs, persistRange, restoreRange, toLocalInput,
  }
}