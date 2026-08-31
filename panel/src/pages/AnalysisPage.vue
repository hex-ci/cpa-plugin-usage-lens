<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTimeRange } from '../composables/useTimeRange'
import { api, type GroupItem, type Stats, type ApiKeyOption } from '../api/client'

const {
  rangeMode, rollingHours, rollingDays, customStart, customEnd,
  rangeMs, persistRange, restoreRange, toLocalInput,
} = useTimeRange()

const dim = ref<'model' | 'api_key'>('model')
const items = ref<GroupItem[]>([])
const stats = ref<Stats | null>(null)
const apiKeyOptions = ref<ApiKeyOption[]>([])
const selectedKey = ref('')
const error = ref('')
const loading = ref(false)

// ── 排序(表头点击,前端排;后端已按 tokens 预序) ──────────────────────
type SortKey = 'requests' | 'tokens' | 'cost' | 'avg_latency_ms' | 'avg_ttft_ms'
const sortKey = ref<SortKey>('tokens')
const sortDir = ref<-1 | 1>(-1)
function setSort(k: SortKey) {
  if (sortKey.value === k) sortDir.value = (sortDir.value * -1) as -1 | 1
  else {
    sortKey.value = k
    sortDir.value = -1
  }
}
function sortArrow(k: SortKey) {
  if (sortKey.value !== k) return '↕'
  return sortDir.value === -1 ? '↓' : '↑'
}

const sorted = computed(() => {
  const arr = [...items.value]
  arr.sort((a, b) => (a[sortKey.value] - b[sortKey.value]) * sortDir.value)
  return arr
})

const totalRequests = computed(() => sorted.value.reduce((s, x) => s + x.requests, 0) || 1)
const rowPct = (n: number) => ((n / totalRequests.value) * 100).toFixed(1) + '%'

// ── 数据加载 ─────────────────────────────────────────────────────────
async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = rangeMs()
    const keyParam: Record<string, string | number> = selectedKey.value ? { api_key: selectedKey.value } : {}
    const [g, s] = await Promise.all([
      dim.value === 'model' ? api.models({ ...r, ...keyParam }) : api.keys({ ...r, ...keyParam }),
      api.stats({ ...r, ...keyParam }),
    ])
    items.value = g.items ?? []
    stats.value = s
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

let rangeTimer: ReturnType<typeof setTimeout> | undefined
function refresh() {
  persistRange()
  load()
}
function onRollingChange() {
  persistRange()
  clearTimeout(rangeTimer)
  rangeTimer = setTimeout(load, 350)
}
function setRange(m: typeof rangeMode.value) {
  rangeMode.value = m
  if (m === 'custom' && (!customStart.value || !customEnd.value)) {
    const now = Date.now()
    customEnd.value = toLocalInput(now)
    customStart.value = toLocalInput(now - 7 * 86400_000)
  }
  refresh()
}
function applyCustom() {
  refresh()
}
function switchDim(d: 'model' | 'api_key') {
  if (dim.value !== d) {
    dim.value = d
    load()
  }
}

onMounted(async () => {
  restoreRange()
  try {
    const o = await api.apiKeysOptions()
    apiKeyOptions.value = o.options ?? []
  } catch { /* ignore */ }
  load()
})

// ── 格式化 ───────────────────────────────────────────────────────────
const compact = (n: number) => {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B'
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(2) + 'K'
  return n.toFixed(0)
}
const fmtDur = (ms: number) => (ms <= 0 ? '—' : ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms.toFixed(0) + 'ms')
const maskKey = (k: string) => (k ? k.slice(0, 8) + '…' + k.slice(-4) : '—')
const successRate = (requests: number, failed: number) =>
  requests > 0 ? ((1 - failed / requests) * 100).toFixed(1) + '%' : '—'

interface ColDef {
  key: string
  label: string
  sortable: boolean
  sortKey?: SortKey
  align?: 'right' | 'left'
}
const COLUMNS: ColDef[] = [
  { key: 'name', label: '名称', sortable: false },
  { key: 'share', label: '占比', sortable: false },
  { key: 'requests', label: '请求', sortable: true, sortKey: 'requests', align: 'right' },
  { key: 'tokens', label: 'Token', sortable: true, sortKey: 'tokens', align: 'right' },
  { key: 'io', label: '输入 / 输出', sortable: false, align: 'right' },
  { key: 'cost', label: '成本', sortable: true, sortKey: 'cost', align: 'right' },
  { key: 'success', label: '成功率', sortable: false, align: 'right' },
  { key: 'avg_latency_ms', label: '平均延迟', sortable: true, sortKey: 'avg_latency_ms', align: 'right' },
  { key: 'avg_ttft_ms', label: '平均首 Token', sortable: true, sortKey: 'avg_ttft_ms', align: 'right' },
]

function cellValue(e: GroupItem, col: ColDef): string {
  switch (col.key) {
    case 'name':
      return dim.value === 'model' ? e.name : maskKey(e.name)
    case 'share':
      return rowPct(e.requests)
    case 'requests':
      return e.requests.toLocaleString()
    case 'tokens':
      return compact(e.tokens)
    case 'io':
      return `${compact(e.input_tokens)} / ${compact(e.output_tokens)}`
    case 'cost':
      return '$' + (e.cost ?? 0).toFixed(2)
    case 'success':
      return successRate(e.requests, e.failed ?? 0)
    case 'avg_latency_ms':
      return fmtDur(e.avg_latency_ms)
    case 'avg_ttft_ms':
      return fmtDur(e.avg_ttft_ms)
    default:
      return ''
  }
}

const maxShare = computed(() => Math.max(1, ...items.value.map((x) => x.requests)))
function shareBarWidth(requests: number): string {
  return Math.round((requests / maxShare.value) * 100) + '%'
}
</script>

<template>
  <div class="flex flex-col gap-3.5">
    <div v-if="error" class="rounded-lg border border-warning/30 bg-warning/10 px-4 py-2.5 text-warning">{{ error }}</div>

    <!-- 工具条 -->
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs font-medium text-text-tertiary">时间范围</span>
      <div class="flex gap-1">
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'today' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('today')"
        >今天</button>
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'yesterday' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('yesterday')"
        >昨天</button>
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'hour' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('hour')"
        >近 {{ rollingHours }} 小时</button>
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'day' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('day')"
        >近 {{ rollingDays }} 天</button>
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="rangeMode === 'custom' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="setRange('custom')"
        >自定义</button>
      </div>

      <div class="flex gap-1">
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="dim === 'model' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="switchDim('model')"
        >按模型</button>
        <button
          class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
          :class="dim === 'api_key' ? 'bg-primary text-white' : 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary'"
          @click="switchDim('api_key')"
        >按 API Key</button>
      </div>

      <select v-model="selectedKey" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="load">
        <option value="">全部 API Key</option>
        <option v-for="k in apiKeyOptions" :key="k.id" :value="k.id">{{ k.label }}</option>
      </select>

      <button class="ml-auto rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary" @click="refresh">刷新</button>
    </div>

    <!-- 滚动滑块 -->
    <div v-if="rangeMode === 'hour' || rangeMode === 'day'" class="flex items-center gap-3 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">{{ rangeMode === 'hour' ? '滚动小时' : '滚动天' }}</span>
      <input v-if="rangeMode === 'hour'" v-model.number="rollingHours" type="range" min="5" max="24" step="1" class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <input v-else v-model.number="rollingDays" type="range" min="1" max="30" step="1" class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <span class="w-16 text-right text-xs font-semibold text-text-primary tabular" style="font-variant-numeric: tabular-nums">{{ rangeMode === 'hour' ? rollingHours + ' 小时' : rollingDays + ' 天' }}</span>
    </div>

    <!-- 自定义范围 -->
    <div v-if="rangeMode === 'custom'" class="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">起止</span>
      <input v-model="customStart" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
      <span class="text-xs text-text-tertiary">至</span>
      <input v-model="customEnd" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
    </div>

    <!-- 总体指标 -->
    <div class="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
      <div class="rounded-lg border border-border bg-bg-primary p-3.5">
        <div class="text-xs font-bold text-text-tertiary">总请求</div>
        <div class="mt-1 text-2xl font-extrabold tabular text-text-primary" style="font-variant-numeric: tabular-nums">{{ stats?.requests?.toLocaleString() ?? '—' }}</div>
        <div class="mt-0.5 text-xs text-text-secondary">成功率 {{ stats ? ((1 - (stats.requests > 0 ? (stats.requests - stats.requests * stats.success_rate) / stats.requests : 0)) * 100).toFixed(1) + '%' : '—' }}</div>
      </div>
      <div class="rounded-lg border border-border bg-bg-primary p-3.5">
        <div class="text-xs font-bold text-text-tertiary">总 Token</div>
        <div class="mt-1 text-2xl font-extrabold tabular text-text-primary" style="font-variant-numeric: tabular-nums">{{ stats ? compact(stats.tokens.total) : '—' }}</div>
        <div class="mt-0.5 text-xs text-text-secondary">缓存读 {{ stats ? compact(stats.tokens.cached) : '—' }}</div>
      </div>
      <div class="rounded-lg border border-border bg-bg-primary p-3.5">
        <div class="text-xs font-bold text-text-tertiary">总成本</div>
        <div class="mt-1 text-2xl font-extrabold tabular text-text-primary" style="font-variant-numeric: tabular-nums">{{ stats ? '$' + stats.cost.toFixed(2) : '—' }}</div>
        <div class="mt-0.5 text-xs text-text-secondary">定价来自设置页</div>
      </div>
      <div class="rounded-lg border border-border bg-bg-primary p-3.5">
        <div class="text-xs font-bold text-text-tertiary">平均延迟</div>
        <div class="mt-1 text-2xl font-extrabold tabular text-text-primary" style="font-variant-numeric: tabular-nums">{{ stats ? fmtDur(stats.avg_latency_ms) : '—' }}</div>
        <div class="mt-0.5 text-xs text-text-secondary">首 Token {{ stats ? fmtDur(stats.avg_ttft_ms) : '—' }}</div>
      </div>
    </div>

    <!-- 分析表 -->
    <div class="overflow-hidden rounded-lg border border-border bg-bg-primary">
      <div class="max-h-[560px] overflow-auto">
        <table class="w-full border-collapse text-left text-xs">
          <thead class="sticky top-0 z-10 bg-bg-quinary">
            <tr class="border-b border-border">
              <th
                v-for="col in COLUMNS"
                :key="col.key"
                class="whitespace-nowrap px-3 py-2 font-bold"
                :class="[
                  col.align === 'right' ? 'text-right' : '',
                  col.sortable ? 'cursor-pointer select-none hover:text-text-primary' : 'text-text-tertiary',
                  col.sortKey && sortKey === col.sortKey ? 'text-text-primary' : 'text-text-tertiary',
                ]"
                @click="col.sortable && col.sortKey && setSort(col.sortKey)"
              >
                <span class="inline-flex items-center gap-1">
                  {{ col.label }}
                  <span v-if="col.sortKey" class="text-xs font-black" :class="col.sortKey === sortKey ? 'text-text-primary' : 'text-text-secondary'">{{ sortArrow(col.sortKey) }}</span>
                </span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in sorted" :key="e.name" class="border-b border-border/60 text-text-primary transition-colors last:border-0 hover:bg-bg-tertiary">
              <template v-for="col in COLUMNS" :key="col.key">
                <td class="whitespace-nowrap px-3 py-2" :class="col.align === 'right' ? 'text-right' : ''">
                  <template v-if="col.key === 'share'">
                    <div class="flex items-center gap-2">
                      <div class="h-1.5 w-24 overflow-hidden rounded-full bg-bg-tertiary">
                        <div class="h-full rounded-full" style="background: var(--color-primary)" :style="{ width: shareBarWidth(e.requests) }"></div>
                      </div>
                      <span class="text-text-secondary">{{ rowPct(e.requests) }}</span>
                    </div>
                  </template>
                  <span v-else class="tabular" style="font-variant-numeric: tabular-nums">{{ cellValue(e, col) }}</span>
                </td>
              </template>
            </tr>
            <tr v-if="!loading && sorted.length === 0">
              <td :colspan="COLUMNS.length" class="px-3 py-10 text-center text-xs text-text-tertiary">暂无数据</td>
            </tr>
            <tr v-if="loading">
              <td :colspan="COLUMNS.length" class="px-3 py-10 text-center text-xs text-text-tertiary">加载中…</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>