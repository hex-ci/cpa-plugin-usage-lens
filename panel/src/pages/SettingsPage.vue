<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, type PricingItem, type ApiKeyOption, type SyncPreview } from '../api/client'

interface PricingRow extends PricingItem {
  _dirty: boolean
  _origInput: number
  _origOutput: number
}

const rows = ref<PricingRow[]>([])
const pricingDirty = ref(false)
const pricingMsg = ref('')
const pricingErr = ref('')

const keys = ref<ApiKeyOption[]>([])
const aliasDrafts = ref<Record<string, string>>({})
const aliasMsg = ref<Record<string, string>>({})

// 价格同步
const syncing = ref(false)
const syncPreview = ref<SyncPreview | null>(null)
const syncChecked = ref<Record<string, boolean>>({})
const syncMsg = ref('')
const syncErr = ref('')

// ── 模型定价表 ───────────────────────────────────────────────────────
async function loadPricing() {
  pricingErr.value = ''
  try {
    const r = await api.pricing()
    rows.value = (r.items ?? []).map((x) => ({
      ...x,
      _dirty: false,
      _origInput: x.input_price,
      _origOutput: x.output_price,
    }))
    pricingDirty.value = false
  } catch (e) {
    pricingErr.value = (e as Error).message
  }
}

function onInput(model: string) {
  const row = rows.value.find((x) => x.model === model)
  if (!row) return
  row._dirty = row.input_price !== row._origInput || row.output_price !== row._origOutput
  pricingDirty.value = rows.value.some((x) => x._dirty)
}

async function savePricing() {
  pricingMsg.value = ''
  pricingErr.value = ''
  const dirty = rows.value.filter((x) => x._dirty)
  if (dirty.length === 0) return
  try {
    const r = await api.putPricing(
      dirty.map((x) => ({ model: x.model, input_price: x.input_price, output_price: x.output_price, source: 'manual' }))
    )
    pricingMsg.value = `已保存 ${r.updated} 个模型的定价`
    await loadPricing()
    setTimeout(() => { pricingMsg.value = '' }, 3000)
  } catch (e) {
    pricingErr.value = (e as Error).message
  }
}

// ── 价格同步 models.dev ──────────────────────────────────────────────
async function runPreview() {
  syncing.value = true
  syncErr.value = ''
  syncMsg.value = ''
  try {
    const p = await api.pricingSyncPreview()
    if (p.error) {
      syncErr.value = p.error
      syncPreview.value = null
      return
    }
    syncPreview.value = p
    syncChecked.value = {}
    for (const m of p.matched) syncChecked.value[m.model] = !m.manual
  } catch (e) {
    syncErr.value = (e as Error).message
  } finally {
    syncing.value = false
  }
}

async function applySync() {
  syncMsg.value = ''
  syncErr.value = ''
  if (!syncPreview.value) return
  const picked = syncPreview.value.matched.filter((m) => syncChecked.value[m.model] && !m.manual)
  if (picked.length === 0) {
    syncErr.value = '没有可应用的项(手动定价优先,已跳过)'
    return
  }
  try {
    const r = await api.putPricing(
      picked.map((m) => ({ model: m.model, input_price: m.input_price, output_price: m.output_price, source: 'models.dev' }))
    )
    syncMsg.value = `已同步 ${r.updated} 个模型的价格`
    syncPreview.value = null
    await loadPricing()
    setTimeout(() => { syncMsg.value = '' }, 3000)
  } catch (e) {
    syncErr.value = (e as Error).message
  }
}

// ── API Key 别名 ─────────────────────────────────────────────────────
async function loadKeys() {
  try {
    const o = await api.apiKeysOptions()
    keys.value = o.options ?? []
    const drafts: Record<string, string> = {}
    for (const k of keys.value) drafts[k.id] = k.alias ?? ''
    aliasDrafts.value = drafts
  } catch { /* ignore */ }
}

async function saveAlias(key: string) {
  aliasMsg.value[key] = ''
  try {
    const r = await api.putApiKeyAlias({ api_key: key, alias: aliasDrafts.value[key] ?? '' })
    aliasMsg.value[key] = `已保存 (${r.updated})`
    await loadKeys()
    setTimeout(() => { aliasMsg.value[key] = '' }, 2500)
  } catch (e) {
    aliasMsg.value[key] = (e as Error).message
  }
}

const maskKey = (k: string) => (k ? k.slice(0, 8) + '…' + k.slice(-4) : '—')

onMounted(() => {
  loadPricing()
  loadKeys()
})
</script>

<template>
  <div class="flex flex-col gap-3.5">
    <!-- 模型定价表 -->
    <section class="rounded-lg border border-border bg-bg-primary">
      <div class="flex items-center justify-between border-b border-border px-4 py-3">
        <div>
          <h2 class="text-sm font-extrabold text-text-primary">模型定价表</h2>
          <p class="mt-0.5 text-xs text-text-tertiary">每百万 Token 美元价,用于成本计算;已出现的模型自动列出,未填价计 0</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="pricingMsg" class="text-xs font-semibold text-success">{{ pricingMsg }}</span>
          <button
            class="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="!pricingDirty"
            @click="savePricing"
          >保存定价</button>
        </div>
      </div>
      <div v-if="pricingErr" class="border-b border-border px-4 py-2 text-xs text-warning">{{ pricingErr }}</div>
      <div class="max-h-[340px] overflow-auto">
        <table class="w-full border-collapse text-left text-xs">
          <thead class="sticky top-0 z-10 bg-bg-quinary">
            <tr class="border-b border-border">
              <th class="px-4 py-2 font-bold text-text-tertiary">模型</th>
              <th class="px-4 py-2 font-bold text-text-tertiary">来源</th>
              <th class="px-4 py-2 text-right font-bold text-text-tertiary">输入 ($/M)</th>
              <th class="px-4 py-2 text-right font-bold text-text-tertiary">输出 ($/M)</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in rows"
              :key="row.model"
              class="border-b border-border/60 text-text-primary last:border-0"
              :class="row._dirty ? 'bg-warning/5' : ''"
            >
              <td class="px-4 py-2 font-semibold">{{ row.model }}</td>
              <td class="px-4 py-2">
                <span
                  class="rounded-full px-2 py-0.5 text-[10px] font-bold"
                  :class="row.source === 'models.dev' ? 'bg-success/10 text-success' : row.source === 'manual' ? 'bg-primary/10 text-primary' : 'bg-bg-tertiary text-text-tertiary'"
                >
                  {{ row.source === 'models.dev' ? 'models.dev' : row.source === 'manual' ? '手动' : '未定价' }}
                </span>
              </td>
              <td class="px-4 py-2 text-right">
                <input
                  v-model.number="row.input_price"
                  type="number"
                  min="0"
                  step="0.01"
                  class="w-24 rounded-md border border-border bg-bg-tertiary px-2 py-1 text-right text-xs text-text-primary"
                  @input="onInput(row.model)"
                />
              </td>
              <td class="px-4 py-2 text-right">
                <input
                  v-model.number="row.output_price"
                  type="number"
                  min="0"
                  step="0.01"
                  class="w-24 rounded-md border border-border bg-bg-tertiary px-2 py-1 text-right text-xs text-text-primary"
                  @input="onInput(row.model)"
                />
              </td>
            </tr>
            <tr v-if="rows.length === 0">
              <td colspan="4" class="px-4 py-8 text-center text-xs text-text-tertiary">暂无模型(有请求事件后自动出现)</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 价格同步 models.dev -->
    <section class="rounded-lg border border-border bg-bg-primary">
      <div class="flex items-center justify-between border-b border-border px-4 py-3">
        <div>
          <h2 class="text-sm font-extrabold text-text-primary">价格同步(models.dev)</h2>
          <p class="mt-0.5 text-xs text-text-tertiary">拉取公开目录,预览比对后勾选应用;手动定价优先,不会自动覆盖</p>
        </div>
        <button
          class="rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary"
          :disabled="syncing"
          @click="runPreview"
        >{{ syncing ? '正在拉取…' : '从 models.dev 预览' }}</button>
      </div>

      <div v-if="syncErr" class="border-b border-border px-4 py-2.5 text-xs text-warning">{{ syncErr }}</div>
      <div v-if="syncMsg" class="border-b border-border px-4 py-2.5 text-xs text-success">{{ syncMsg }}</div>

      <div v-if="syncPreview" class="p-4">
        <div v-if="syncPreview.matched.length" class="mb-3">
          <div class="mb-1.5 text-xs font-bold text-text-tertiary">目录中匹配到的模型({{ syncPreview.matched.length }})</div>
          <div class="overflow-hidden rounded-lg border border-border">
            <table class="w-full border-collapse text-left text-xs">
              <thead class="bg-bg-quinary">
                <tr class="border-b border-border">
                  <th class="w-8 px-3 py-2"></th>
                  <th class="px-3 py-2 font-bold text-text-tertiary">模型</th>
                  <th class="px-3 py-2 text-right font-bold text-text-tertiary">输入 ($/M)</th>
                  <th class="px-3 py-2 text-right font-bold text-text-tertiary">输出 ($/M)</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="m in syncPreview.matched" :key="m.model" class="border-b border-border/60 last:border-0">
                  <td class="px-3 py-2">
                    <input
                      type="checkbox"
                      class="accent-[var(--color-primary)]"
                      :checked="!!syncChecked[m.model]"
                      :disabled="m.manual"
                      @change="syncChecked[m.model] = !syncChecked[m.model]"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <span class="font-semibold">{{ m.model }}</span>
                    <span v-if="m.manual" class="ml-2 rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-bold text-primary">手动价已定</span>
                  </td>
                  <td class="px-3 py-2 text-right text-text-secondary">${{ m.input_price.toFixed(2) }}</td>
                  <td class="px-3 py-2 text-right text-text-secondary">${{ m.output_price.toFixed(2) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div v-if="syncPreview.unmatched.length" class="mb-3">
          <div class="mb-1.5 text-xs font-bold text-text-tertiary">目录中缺失的模型({{ syncPreview.unmatched.length }})</div>
          <div class="flex flex-wrap gap-1.5">
            <span v-for="m in syncPreview.unmatched" :key="m" class="rounded-full bg-bg-tertiary px-2.5 py-1 text-xs text-text-secondary">{{ m }}</span>
          </div>
        </div>
        <div class="mt-3 flex items-center justify-end gap-2">
          <span v-if="syncPreview.matched.every((m) => m.manual || !syncChecked[m.model])" class="text-xs text-text-tertiary">手动定价优先,未勾选任何可应用项</span>
          <button class="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white" @click="applySync">应用勾选</button>
        </div>
      </div>
    </section>

    <!-- API Key 别名 -->
    <section class="rounded-lg border border-border bg-bg-primary">
      <div class="border-b border-border px-4 py-3">
        <h2 class="text-sm font-extrabold text-text-primary">API Key 别名</h2>
        <p class="mt-0.5 text-xs text-text-tertiary">给常用 Key 设中文别名,概览/分析/事件页的下拉与表格会优先显示别名</p>
      </div>
      <div class="divide-y divide-border/60">
        <div v-for="k in keys" :key="k.id" class="flex items-center gap-3 px-4 py-2.5">
          <span class="w-36 shrink-0 font-mono text-xs text-text-secondary">{{ maskKey(k.id) }}</span>
          <input
            v-model="aliasDrafts[k.id]"
            type="text"
            placeholder="未设置别名"
            class="flex-1 rounded-md border border-border bg-bg-tertiary px-2.5 py-1.5 text-xs text-text-primary placeholder:text-text-tertiary"
            @keyup.enter="saveAlias(k.id)"
          />
          <button
            class="shrink-0 rounded-md border border-border bg-bg-primary px-3 py-1.5 text-xs font-semibold text-text-secondary hover:bg-bg-tertiary"
            @click="saveAlias(k.id)"
          >保存</button>
          <span v-if="aliasMsg[k.id]" class="w-24 shrink-0 text-right text-xs text-success">{{ aliasMsg[k.id] }}</span>
        </div>
        <div v-if="keys.length === 0" class="px-4 py-8 text-center text-xs text-text-tertiary">暂无 API Key</div>
      </div>
    </section>
  </div>
</template>