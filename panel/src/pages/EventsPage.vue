<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTimeRange } from '../composables/useTimeRange'
import { api, type EventItem, type ApiKeyOption } from '../api/client'

const {
  rangeMode, rollingHours, rollingDays, customStart, customEnd,
  rangeMs, persistRange, restoreRange, toLocalInput,
} = useTimeRange()

const items = ref<EventItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const error = ref('')
const detail = ref<EventItem | null>(null)

// 筛选选项与选中值
const apiKeyOptions = ref<ApiKeyOption[]>([])
const modelOptions = ref<string[]>([])
const sourceOptions = ref<string[]>([])
const selectedKey = ref('')
const selectedModel = ref('')
const selectedSource = ref('')
const resultFilter = ref('') // '' 全部 | 'ok' | 'failed'

// ── 列配置(显隐 + 顺序,持久化 localStorage) ─────────────────────────
interface ColDef { key: string; label: string }
const ALL_COLS: ColDef[] = [
  { key: 'time', label: '时间' },
  { key: 'model', label: '模型' },
  { key: 'alias', label: '别名' },
  { key: 'api_key', label: 'API Key' },
  { key: 'source', label: '来源' },
  { key: 'input', label: '输入' },
  { key: 'output', label: '输出' },
  { key: 'cached', label: '缓存' },
  { key: 'latency', label: '延迟' },
  { key: 'ttft', label: '首 Token' },
  { key: 'status', label: '状态' },
]
const COLS_LS = 'cli-proxy-usage-events-cols-v1'
const visibleCols = ref<string[]>(ALL_COLS.map((c) => c.key))
const showColsPanel = ref(false)

function persistCols() {
  try { localStorage.setItem(COLS_LS, JSON.stringify(visibleCols.value)) } catch { /* ignore */ }
}
function restoreCols() {
  try {
    const raw = localStorage.getItem(COLS_LS)
    if (!raw) return
    const v: unknown = JSON.parse(raw)
    if (Array.isArray(v)) {
      const stored = v.filter((k): k is string => typeof k === 'string' && ALL_COLS.some((c) => c.key === k))
      if (stored.length) {
        visibleCols.value = [...stored, ...ALL_COLS.map((c) => c.key).filter((k) => !stored.includes(k))]
      }
    }
  } catch { /* ignore */ }
}
function toggleCol(key: string) {
  const i = visibleCols.value.indexOf(key)
  if (i >= 0) visibleCols.value.splice(i, 1)
  else visibleCols.value.push(key)
  persistCols()
}
function moveCol(key: string, dir: -1 | 1) {
  const i = visibleCols.value.indexOf(key)
  const j = i + dir
  if (i < 0 || j < 0 || j >= visibleCols.value.length) return
  const next = [...visibleCols.value]
  ;[next[i], next[j]] = [next[j], next[i]]
  visibleCols.value = next
  persistCols()
}

// ── 数据加载 ─────────────────────────────────────────────────────────
async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = rangeMs()
    const p: Record<string, string | number> = {
      ...r,
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
    }
    if (selectedKey.value) p.api_key = selectedKey.value
    if (selectedModel.value) p.model = selectedModel.value
    if (selectedSource.value) p.source = selectedSource.value
    if (resultFilter.value === 'ok') p.failed = 0
    else if (resultFilter.value === 'failed') p.failed = 1
    const e = await api.events(p)
    items.value = e.items ?? []
    total.value = e.total ?? 0
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function loadKeyOptions() {
  try {
    const o = await api.apiKeysOptions()
    apiKeyOptions.value = o.options ?? []
  } catch { /* ignore */ }
}
async function loadModelOptions() {
  try {
    const m = await api.models(rangeMs())
    modelOptions.value = (m.items ?? []).map((x) => x.name)
  } catch { /* ignore */ }
}
async function loadSourceOptions() {
  try {
    const s = await api.eventSources(rangeMs())
    sourceOptions.value = s.sources ?? []
  } catch { /* ignore */ }
}

let rangeTimer: ReturnType<typeof setTimeout> | undefined
function refresh() {
  page.value = 1
  persistRange()
  load()
  clearTimeout(rangeTimer)
  rangeTimer = setTimeout(() => { loadModelOptions(); loadSourceOptions() }, 350)
}
function onRollingChange() {
  persistRange()
  clearTimeout(rangeTimer)
  rangeTimer = setTimeout(refresh, 350)
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
function applyCustom() { refresh() }
function onFilterChange() {
  page.value = 1
  load()
}
function goPage(n: number) {
  page.value = Math.min(Math.max(1, n), totalPages.value)
  load()
}
function onPageSize() {
  page.value = 1
  load()
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

onMounted(() => {
  restoreRange()
  restoreCols()
  loadKeyOptions()
  loadModelOptions()
  loadSourceOptions()
  load()
})

// ── 格式化 ───────────────────────────────────────────────────────────
const fmtTs = (ts: number) =>
  new Date(ts).toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
const fmtDur = (ms: number) => (ms < 0 ? '—' : ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms.toFixed(0) + 'ms')
const compact = (n: number) => {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B'
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(2) + 'K'
  return n.toFixed(0)
}
const maskKey = (k: string) => (k ? k.slice(0, 8) + '…' + k.slice(-4) : '—')

function cellValue(e: EventItem, key: string): string {
  switch (key) {
    case 'time': return fmtTs(e.ts)
    case 'model': return e.model || '—'
    case 'alias': return e.alias || '—'
    case 'api_key': return maskKey(e.api_key) || '—'
    case 'source': return e.source || '—'
    case 'input': return compact(e.input_tokens)
    case 'output': return compact(e.output_tokens)
    case 'cached': return compact(e.cached_tokens)
    case 'latency': return fmtDur(e.latency_ms)
    case 'ttft': return fmtDur(e.ttft_ms)
    case 'status': return e.failed ? `${e.status_code} 失败` : '成功'
    default: return ''
  }
}

const detailRows = computed(() => {
  const d = detail.value
  if (!d) return []
  return [
    { label: '时间', value: fmtTs(d.ts) },
    { label: '模型', value: d.model || '—' },
    { label: '别名', value: d.alias || '—' },
    { label: 'API Key', value: maskKey(d.api_key) || '—' },
    { label: 'Auth ID', value: d.auth_id || '—' },
    { label: 'Provider', value: d.provider || '—' },
    { label: '来源', value: d.source || '—' },
    { label: '输入 Token', value: compact(d.input_tokens) },
    { label: '输出 Token', value: compact(d.output_tokens) },
    { label: '推理 Token', value: compact(d.reasoning_tokens) },
    { label: '缓存 Token', value: compact(d.cached_tokens) },
    { label: '总 Token', value: compact(d.total_tokens) },
    { label: '延迟', value: fmtDur(d.latency_ms) },
    { label: '首 Token 延迟', value: fmtDur(d.ttft_ms) },
    { label: '状态', value: d.failed ? `失败 (HTTP ${d.status_code})` : '成功' },
  ]
})
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

      <select v-model="selectedModel" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="onFilterChange">
        <option value="">全部模型</option>
        <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
      </select>
      <select v-model="selectedKey" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="onFilterChange">
        <option value="">全部 API Key</option>
        <option v-for="k in apiKeyOptions" :key="k.id" :value="k.id">{{ k.label }}</option>
      </select>
      <select v-model="selectedSource" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="onFilterChange">
        <option value="">全部来源</option>
        <option v-for="s in sourceOptions" :key="s" :value="s">{{ s }}</option>
      </select>
      <select v-model="resultFilter" class="rounded-md border border-border bg-bg-primary px-2 py-1.5 text-xs font-semibold text-text-secondary" @change="onFilterChange">
        <option value="">全部结果</option>
        <option value="ok">成功</option>
        <option value="failed">失败</option>
      </select>

      <div class="relative">
        <button
          class="rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary"
          @click="showColsPanel = !showColsPanel"
        >列设置</button>
        <div v-if="showColsPanel" class="absolute right-0 top-full z-20 mt-1.5 w-52 rounded-lg border border-border bg-white p-2.5 shadow-lg">
          <div v-for="c in ALL_COLS" :key="c.key" class="flex items-center justify-between rounded-md px-1.5 py-1 hover:bg-bg-tertiary">
            <label class="flex flex-1 cursor-pointer items-center gap-2 text-xs font-medium text-text-secondary">
              <input type="checkbox" class="accent-[var(--color-primary)]" :checked="visibleCols.includes(c.key)" @change="toggleCol(c.key)" />
              {{ c.label }}
            </label>
            <span class="flex gap-0.5">
              <button class="rounded px-1 text-xs text-text-tertiary hover:bg-bg-quinary hover:text-text-primary" @click="moveCol(c.key, -1)">↑</button>
              <button class="rounded px-1 text-xs text-text-tertiary hover:bg-bg-quinary hover:text-text-primary" @click="moveCol(c.key, 1)">↓</button>
            </span>
          </div>
        </div>
      </div>

      <button class="ml-auto rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary" @click="refresh">刷新</button>
    </div>

    <!-- 滚动滑块 -->
    <div v-if="rangeMode === 'hour' || rangeMode === 'day'" class="flex items-center gap-3 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">{{ rangeMode === 'hour' ? '滚动小时' : '滚动天' }}</span>
      <input v-if="rangeMode === 'hour'" v-model.number="rollingHours" type="range" min="5" max="24" step="1" class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <input v-else v-model.number="rollingDays" type="range" min="1" max="30" step="1" class="flex-1" style="accent-color: var(--color-primary)" @input="onRollingChange" />
      <span class="tabular w-16 text-right text-xs font-semibold text-text-primary" style="font-variant-numeric: tabular-nums">{{ rangeMode === 'hour' ? rollingHours + ' 小时' : rollingDays + ' 天' }}</span>
    </div>

    <!-- 自定义范围 -->
    <div v-if="rangeMode === 'custom'" class="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-bg-primary px-3 py-2">
      <span class="text-xs text-text-tertiary">起止</span>
      <input v-model="customStart" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
      <span class="text-xs text-text-tertiary">至</span>
      <input v-model="customEnd" type="datetime-local" class="rounded-md border border-border bg-bg-tertiary px-2 py-1 text-xs text-text-primary" @change="applyCustom" />
    </div>

    <!-- 事件表格 -->
    <div class="overflow-hidden rounded-lg border border-border bg-bg-primary">
      <div class="max-h-[560px] overflow-auto">
        <table class="w-full border-collapse text-left text-xs">
          <thead class="sticky top-0 z-10 bg-bg-quinary">
            <tr class="border-b border-border">
              <th v-for="c in ALL_COLS.filter((x) => visibleCols.includes(x.key))" :key="c.key" class="whitespace-nowrap px-3 py-2 font-bold text-text-tertiary">
                {{ c.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(e, i) in items"
              :key="e.ts + '-' + i"
              class="cursor-pointer border-b border-border/60 text-text-primary transition-colors last:border-0 hover:bg-bg-tertiary"
              @click="detail = e"
            >
              <template v-for="c in ALL_COLS.filter((x) => visibleCols.includes(x.key))" :key="c.key">
                <td class="whitespace-nowrap px-3 py-2">
                  <span v-if="c.key === 'status'" class="inline-flex items-center gap-1.5">
                    <span class="inline-block h-1.5 w-1.5 rounded-full" :class="e.failed ? 'bg-error' : 'bg-success'"></span>
                    <span :class="e.failed ? 'text-error' : ''">{{ cellValue(e, c.key) }}</span>
                  </span>
                  <span v-else-if="c.key === 'api_key'" class="text-text-secondary">{{ cellValue(e, c.key) }}</span>
                  <span v-else class="tabular" style="font-variant-numeric: tabular-nums">{{ cellValue(e, c.key) }}</span>
                </td>
              </template>
            </tr>
            <tr v-if="!loading && items.length === 0">
              <td :colspan="visibleCols.length" class="px-3 py-10 text-center text-xs text-text-tertiary">暂无数据</td>
            </tr>
            <tr v-if="loading">
              <td :colspan="visibleCols.length" class="px-3 py-10 text-center text-xs text-text-tertiary">加载中…</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页 -->
      <div class="flex items-center justify-between border-t border-border px-3 py-2.5">
        <span class="text-xs text-text-tertiary">共 {{ total.toLocaleString() }} 条</span>
        <div class="flex items-center gap-2">
          <select v-model.number="pageSize" class="rounded-md border border-border bg-bg-primary px-2 py-1 text-xs font-semibold text-text-secondary" @change="onPageSize">
            <option :value="20">20 / 页</option>
            <option :value="50">50 / 页</option>
            <option :value="100">100 / 页</option>
          </select>
          <button
            class="rounded-md border border-border bg-bg-primary px-2.5 py-1 text-xs font-semibold text-text-secondary disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="page <= 1" @click="goPage(page - 1)"
          >上一页</button>
          <span class="text-xs font-semibold text-text-primary">{{ page }} / {{ totalPages }}</span>
          <button
            class="rounded-md border border-border bg-bg-primary px-2.5 py-1 text-xs font-semibold text-text-secondary disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="page >= totalPages" @click="goPage(page + 1)"
          >下一页</button>
        </div>
      </div>
    </div>

    <!-- 详情抽屉 -->
    <Teleport to="body">
      <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/20" @click="detail = null"></div>
        <div class="relative max-h-[80vh] w-full max-w-lg overflow-auto rounded-xl bg-white p-5 shadow-lg">
          <div class="mb-4 flex items-center justify-between">
            <span class="text-sm font-extrabold text-text-primary">请求详情</span>
            <button class="rounded-md px-2 py-1 text-xs font-semibold text-text-tertiary hover:bg-bg-tertiary hover:text-text-primary" @click="detail = null">关闭</button>
          </div>
          <div class="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2.5 text-xs">
            <template v-for="row in detailRows" :key="row.label">
              <span class="text-text-tertiary">{{ row.label }}</span>
              <span class="text-right font-semibold text-text-primary">{{ row.value }}</span>
            </template>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>