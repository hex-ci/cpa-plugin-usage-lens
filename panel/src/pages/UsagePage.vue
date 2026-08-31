<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { api, type Stats, type TrendPoint, type RealtimeMetric, type GroupItem, type ApiKeyOption } from '../api/client'

const stats = ref<Stats | null>(null)
const trend = ref<TrendPoint[]>([])
const realtime = ref<RealtimeMetric[]>([])
const share = ref<GroupItem[]>([])
const error = ref('')
const apiKeyOptions = ref<ApiKeyOption[]>([])
const selectedKey = ref('')
const window2 = ref<'day' | 'week' | 'month'>('day')
const rtWindow = ref(15)

// 时间范围（对齐 Keeper 5 档）：滚动小时/天 + 今天/昨天 + 自定义
type RangeMode = 'hour' | 'day' | 'today' | 'yesterday' | 'custom'
const RANGE_MODES: RangeMode[] = ['hour', 'day', 'today', 'yesterday', 'custom']
const rangeMode = ref<RangeMode>('today')
const rollingHours = ref(8)
const rollingDays = ref(7)
const customUnit = ref<'hour' | 'day'>('day')
const customStart = ref('')
const customEnd = ref('')

const LS_KEY = 'cli-proxy-usage-time-range-v1'

function startOfDay(offsetDays = 0): number {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() + offsetDays)
  return d.getTime()
}

function toLocalInput(ts: number): string {
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

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
    const v = JSON.parse(raw)
    if (RANGE_MODES.includes(v.mode)) rangeMode.value = v.mode
    if (typeof v.rollingHours === 'number') rollingHours.value = v.rollingHours
    if (typeof v.rollingDays === 'number') rollingDays.value = v.rollingDays
    if (v.customUnit) customUnit.value = v.customUnit
    if (v.customStart) customStart.value = v.customStart
    if (v.customEnd) customEnd.value = v.customEnd
  } catch { /* ignore */ }
}

function setRange(m: RangeMode) {
  rangeMode.value = m
  if (m === 'custom' && (!customStart.value || !customEnd.value)) {
    const now = Date.now()
    customEnd.value = toLocalInput(now)
    customStart.value = toLocalInput(now - 7 * 86400_000)
  }
  persistRange()
  load()
}

let rollingTimer: ReturnType<typeof setTimeout> | undefined
function onRollingChange() {
  persistRange()
  clearTimeout(rollingTimer)
  rollingTimer = setTimeout(load, 350)
}

function applyCustom() {
  persistRange()
  load()
}

async function load() {
  error.value = ''
  try {
    const r = rangeMs()
    const bucket = window2.value === 'day' ? 'hour' : 'day'
    const keyParam: Record<string, string | number> = selectedKey.value ? { api_key: selectedKey.value } : {}
    const [s, t, rt, sh] = await Promise.all([
      api.stats({ ...r, ...keyParam }),
      api.trend({ ...r, ...keyParam, bucket }),
      api.realtime({ window: rtWindow.value, ...keyParam }),
      api.models({ ...r, ...keyParam }),
    ])
    stats.value = s
    trend.value = t.series
    realtime.value = rt.metrics
    share.value = (sh.items ?? []).slice(0, 8)
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function loadApiKeyOptions() {
  try {
    const o = await api.apiKeysOptions()
    apiKeyOptions.value = o.options ?? []
  } catch { /* ignore */ }
}

onMounted(async () => { restoreRange(); await loadApiKeyOptions(); load() })

const fmt = (n: number | undefined) => (n == null ? '—' : n.toLocaleString())
const compact = (n: number) => {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B'
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(2) + 'K'
  return n.toFixed(0)
}
const pct = (v: number | undefined) => (v == null ? '—' : (v * 100).toFixed(2) + '%')

const days = computed(() => {
  const r = rangeMs()
  return Math.max(1, (r.end_ts - r.start_ts) / 86400_000)
})

function sparkOf(field: (p: TrendPoint) => number): number[] {
  return trend.value.map(field)
}

interface CardDef {
  key: string; label: string; accent: string; value: string; meta: string
  icon: string; primary: boolean; spark: number[]
}

const cards = computed<CardDef[]>(() => {
  const s = stats.value
  const daily = (n: number) => compact(n / days.value)
  return [
    {
      key: 'daily', label: '每日均值', accent: '#8b5cf6', primary: true,
      value: daily(s?.requests ?? 0),
      meta: `请求 / Token ${daily(s?.tokens.total ?? 0)} / 成本 $${(s?.cost ?? 0) / days.value}`,
      icon: 'calendar', spark: sparkOf((p) => p.requests),
    },
    {
      key: 'requests', label: '总请求', accent: '#64748b', primary: true,
      value: fmt(s?.requests),
      meta: `成功率 ${pct(s?.success_rate)}`,
      icon: 'activity', spark: sparkOf((p) => p.requests),
    },
    {
      key: 'tokens', label: '总 Token', accent: '#8b5cf6', primary: true,
      value: compact(s?.tokens.total ?? 0),
      meta: `缓存读 ${compact(s?.tokens.cached ?? 0)} · 推理 ${compact(0)}`,
      icon: 'diamond', spark: sparkOf((p) => p.tokens),
    },
    {
      key: 'rpm', label: '每分钟请求', accent: '#10b981', primary: false,
      value: (s?.rpm ?? 0).toFixed(2),
      meta: `总请求数 ${fmt(s?.requests)}`,
      icon: 'timer', spark: sparkOf((p) => p.requests),
    },
    {
      key: 'tpm', label: '每分钟 Token', accent: '#f59e0b', primary: false,
      value: compact(s?.tokens.total ?? 0),
      meta: `总 Token ${compact(s?.tokens.total ?? 0)}`,
      icon: 'trending', spark: sparkOf((p) => p.tokens),
    },
    {
      key: 'cache', label: '缓存命中率', accent: '#3b82f6', primary: false,
      value: pct(s?.cache_rate),
      meta: `缓存读 ${compact(s?.tokens.cached ?? 0)} · 输入 ${compact(s?.tokens.input ?? 0)}`,
      icon: 'percent', spark: sparkOf((p) => (p.input_tokens + p.cached_tokens > 0 ? p.cached_tokens / (p.input_tokens + p.cached_tokens) : 0)),
    },
    {
      key: 'cost', label: '总成本', accent: '#22c55e', primary: false,
      value: '$' + (s?.cost ?? 0).toFixed(2),
      meta: `总 Token ${compact(s?.tokens.total ?? 0)}`,
      icon: 'dollar', spark: sparkOf((p) => p.cost),
    },
  ]
})

const primaryCards = computed(() => cards.value.filter((c) => c.primary))
const secondaryCards = computed(() => cards.value.filter((c) => !c.primary))

const maxTokens = computed(() => Math.max(1, ...trend.value.map((t) => t.tokens)))
function heatColor(tokens: number): string {
  if (tokens <= 0) return '#e9e6df'
  const r = Math.min(1, tokens / maxTokens.value)
  return `rgb(${Math.round(205 - r * 75)}, ${Math.round(190 - r * 62)}, ${Math.round(172 - r * 48)})`
}

// Request Health：成功（绿）vs 失败（红）的时间线
const healthBars = computed(() => trend.value.map((t) => ({
  ts: t.ts,
  success: t.requests - t.failed,
  failed: t.failed,
})))

const totalShare = computed(() => share.value.reduce((acc, s) => acc + s.tokens, 0) || 1)

function sparkPoints(vals: number[]): string {
  const w = 100
  const h = 28
  const max = Math.max(1, ...vals)
  if (vals.length < 2) return ''
  return vals.map((v, i) => {
    const x = (i / (vals.length - 1)) * w
    const y = h - (v / max) * h
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

function fmtNow(v: number, key: string): string {
  if (key === 'ttft' || key === 'latency') return v.toFixed(2) + 's'
  if (key === 'token_velocity') return compact(v) + '/min'
  if (key === 'request_level') return v.toFixed(2)
  if (key === 'cache_level') return v.toFixed(1) + '%'
  return compact(v) + '/min'
}
</script>

<template>
  <div class="flex flex-col gap-3.5">
    <div v-if="error" class="rounded-lg border border-warning/30 bg-warning/10 px-4 py-2.5 text-warning">{{ error }}</div>

    <!-- 工具条 -->
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs font-medium text-text-tertiary">时间范围</span>
      <div class="flex gap-1">
        <button class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'today' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('today')">今天</button>
        <button class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'yesterday' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('yesterday')">昨天</button>
        <button class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'hour' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('hour')">近 {{ rollingHours }} 小时</button>
        <button class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'day' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('day')">近 {{ rollingDays }} 天</button>
        <button class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'custom' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('custom')">自定义</button>
      </div>
      <select v-model="selectedKey" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="load">
        <option value="">全部 API Key</option>
        <option v-for="k in apiKeyOptions" :key="k.id" :value="k.id">{{ k.label }}</option>
      </select>
      <button class="ml-auto rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary" @click="load">刷新</button>
    </div>

    <!-- 滚动滑块 -->
    <div v-if="rangeMode === 'hour' || rangeMode === 'day'" class="flex items-center gap-3 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">{{ rangeMode === 'hour' ? '滚动小时' : '滚动天' }}</span>
      <input v-if="rangeMode === 'hour'" v-model.number="rollingHours" type="range" min="5" max="24" step="1"
        class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <input v-else v-model.number="rollingDays" type="range" min="1" max="30" step="1"
        class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <span class="tabular w-16 text-right text-xs font-semibold text-text-primary">{{ rangeMode === 'hour' ? rollingHours + ' 小时' : rollingDays + ' 天' }}</span>
    </div>

    <!-- 自定义范围 -->
    <div v-if="rangeMode === 'custom'" class="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">起止</span>
      <input v-model="customStart" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
      <span class="text-xs text-text-tertiary">至</span>
      <input v-model="customEnd" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
    </div>

    <!-- 主统计卡 -->
    <div class="flex gap-3.5">
      <div v-for="c in primaryCards" :key="c.key" class="stat-card flex-1" :style="{ '--accent': c.accent }">
        <div class="flex items-start justify-between">
          <span class="text-xs font-bold text-text-tertiary">{{ c.label }}</span>
          <span class="stat-icon-badge"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-[18px] w-[18px]"><template v-if="c.icon === 'activity'"><path d="M22 12h-4l-3 9L9 3l-3 9H2" /></template><template v-else-if="c.icon === 'diamond'"><path d="M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59a2.41 2.41 0 0 0 3.41 0l7.59-7.59a2.41 2.41 0 0 0 0-3.41l-7.59-7.59a2.41 2.41 0 0 0-3.41 0Z" /></template><template v-else><rect x="3" y="4" width="18" height="18" rx="2" /><path d="M16 2v4" /><path d="M8 2v4" /><path d="M3 10h18" /></template></svg></span>
        </div>
        <div class="tabular stat-value-primary">{{ c.value }}</div>
        <div class="flex items-end justify-between gap-2">
          <span class="text-xs text-text-secondary">{{ c.meta }}</span>
          <svg v-if="c.spark.length > 1" viewBox="0 0 100 28" class="h-7 w-24 flex-shrink-0" preserveAspectRatio="none">
            <polyline :points="sparkPoints(c.spark)" fill="none" :stroke="c.accent" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
          </svg>
        </div>
      </div>
    </div>

    <!-- 次统计卡 -->
    <div class="grid grid-cols-4 gap-3.5">
      <div v-for="c in secondaryCards" :key="c.key" class="stat-card" :style="{ '--accent': c.accent }">
        <div class="flex items-start justify-between">
          <span class="text-xs font-bold text-text-tertiary">{{ c.label }}</span>
          <span class="stat-icon-badge"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-[18px] w-[18px]"><template v-if="c.icon === 'timer'"><line x1="10" x2="14" y1="2" y2="2" /><line x1="12" x2="15" y1="14" y2="11" /><circle cx="12" cy="14" r="8" /></template><template v-else-if="c.icon === 'trending'"><path d="M22 7 13.5 15.5 8.5 10.5 2 17" /><path d="M16 7h6v6" /></template><template v-else-if="c.icon === 'percent'"><line x1="19" x2="5" y1="5" y2="19" /><circle cx="6.5" cy="6.5" r="2.5" /><circle cx="17.5" cy="17.5" r="2.5" /></template><template v-else><path d="M12 12v7" /><path d="M7.5 10v9" /><path d="M16.5 10v9" /><path d="M10.5 2.5a4.5 4.5 0 0 1 0 9" /></template></svg></span>
        </div>
        <div class="tabular text-[28px] font-extrabold leading-tight">{{ c.value }}</div>
        <div class="flex items-end justify-between gap-2">
          <span class="text-xs text-text-secondary">{{ c.meta }}</span>
          <svg v-if="c.spark.length > 1" viewBox="0 0 100 28" class="h-6 w-20 flex-shrink-0" preserveAspectRatio="none">
            <polyline :points="sparkPoints(c.spark)" fill="none" :stroke="c.accent" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
          </svg>
        </div>
      </div>
    </div>

    <!-- Recent Activity -->
    <section class="card p-5">
      <div class="flex items-center justify-between">
        <h3 class="text-[18px] font-bold text-text-primary">近期活动</h3>
        <div class="flex gap-1">
          <button v-for="w in (['day', 'week', 'month'] as const)" :key="w"
            class="rounded-md px-3 py-1 text-xs font-semibold transition-colors"
            :class="window2 === w ? 'bg-primary text-white' : 'text-text-secondary hover:bg-bg-tertiary'"
            @click="window2 = w; load()">
            {{ w === 'day' ? '天' : w === 'week' ? '周' : '月' }}
          </button>
        </div>
      </div>

      <div class="mt-4">
        <div class="text-xs font-bold text-text-secondary">Token 用量</div>
        <div class="mt-2 grid grid-cols-12 gap-1">
          <div v-for="t in trend" :key="t.ts" class="h-7 rounded-[3px]" :style="{ background: heatColor(t.tokens) }" :title="`${new Date(t.ts).toLocaleString()} — ${t.tokens.toLocaleString()} Token`"></div>
        </div>
        <div class="mt-1 flex justify-between text-[10px] text-text-quaternary"><span>少</span><span>多</span></div>
      </div>

      <div class="mt-4">
        <div class="text-xs font-bold text-text-secondary">请求健康</div>
        <div class="mt-2 flex h-8 items-stretch gap-[2px]">
          <div v-for="b in healthBars" :key="b.ts" class="flex-1 flex-col overflow-hidden rounded-[2px]" :title="`${new Date(b.ts).toLocaleString()} — 成功 ${b.success} 失败 ${b.failed}`">
            <div class="w-full" :style="{ height: '50%', background: '#10b981' }"></div>
            <div class="w-full" :style="{ height: b.failed > 0 ? `${b.failed / Math.max(1, b.success + b.failed) * 50}%` : '0%', background: '#c65746' }"></div>
          </div>
        </div>
      </div>
    </section>

    <!-- Realtime Metrics -->
    <section class="card p-5">
      <div class="flex items-center justify-between">
        <h3 class="text-[18px] font-bold text-text-primary">实时指标</h3>
        <div class="flex gap-1">
          <button v-for="w in ([15, 30, 60] as const)" :key="w"
            class="rounded-md px-3 py-1 text-xs font-semibold transition-colors"
            :class="rtWindow === w ? 'bg-primary text-white' : 'text-text-secondary hover:bg-bg-tertiary'"
            @click="rtWindow = w; load()">
            {{ w }}m
          </button>
        </div>
      </div>
      <div class="mt-3 grid grid-cols-5 gap-3">
        <div v-for="m in realtime" :key="m.key" class="rounded-lg border border-border p-3">
          <div class="text-xs font-bold text-text-tertiary">{{ m.name }}</div>
          <div class="tabular mt-1 text-xl font-extrabold">{{ fmtNow(m.now, m.key) }}</div>
          <div class="text-[11px] text-text-quaternary">均值 {{ fmtNow(m.avg, m.key) }}</div>
          <div class="text-[11px] font-semibold" :class="m.trend >= 0 ? 'text-success' : 'text-error'">
            {{ m.trend >= 0 ? '+' : '' }}{{ m.trend.toFixed(2) }}%
          </div>
        </div>
      </div>
    </section>

    <!-- Token Share -->
    <section class="card p-5">
      <h3 class="text-[18px] font-bold text-text-primary">Token 份额</h3>
      <div class="mt-3 space-y-2">
        <div v-for="s in share" :key="s.name" class="flex items-center gap-3">
          <span class="w-40 truncate text-xs font-semibold text-text-primary">{{ s.name }}</span>
          <div class="h-2.5 flex-1 overflow-hidden rounded-full bg-bg-tertiary">
            <div class="h-full rounded-full" :style="{ width: (s.tokens / totalShare) * 100 + '%', background: '#8b8680' }"></div>
          </div>
          <span class="tabular w-16 text-right text-xs text-text-secondary">{{ ((s.tokens / totalShare) * 100).toFixed(2) }}%</span>
          <span class="tabular w-20 text-right text-xs text-text-tertiary">{{ compact(s.tokens) }}</span>
        </div>
        <div v-if="share.length === 0" class="text-xs text-text-quaternary">暂无数据</div>
      </div>
    </section>
  </div>
</template>

<style scoped lang="scss">
.stat-card {
  position: relative;
  padding: 18px;
  min-height: 176px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  border-radius: 24px;
  border: 1px solid var(--color-border);
  background:
    radial-gradient(120% 140% at 12% 0%, color-mix(in srgb, var(--accent) 18%, transparent) 0%, transparent 62%),
    var(--color-bg-primary);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  transition: transform 150ms ease, box-shadow 150ms ease, border-color 150ms ease;

  &::before {
    content: '';
    position: absolute;
    top: 0; left: 0; right: 0;
    height: 3px;
    background: linear-gradient(90deg, transparent 0%, var(--accent) 12px, color-mix(in srgb, var(--accent) 68%, transparent) 42%, transparent 100%);
    opacity: 0.95;
  }

  &:hover {
    transform: translateY(-2px);
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.22);
  }
}
.stat-icon-badge {
  width: 34px; height: 34px;
  border-radius: 8px;
  display: grid; place-items: center;
  color: #fff; background: var(--accent);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 10px 22px rgba(0, 0, 0, 0.25);
  flex-shrink: 0;
}
.stat-value-primary {
  font-size: 32px; font-weight: 800; line-height: 1.2;
  color: var(--color-text-primary);
}
</style>