// client.ts: typed accessors for the plugin's management API.
// The panel itself loads from the unauthenticated resource route; these calls
// hit /v0/management/... which the gateway guards with the management key —
// supplied from the main-panel login state (see auth.ts).
import { resolveKey } from './auth'

const API_BASE = '/v0/management/plugins/usage-lens/api'

function authHeaders(): Record<string, string> {
  const k = resolveKey()
  return k ? { Authorization: `Bearer ${k}` } : {}
}

async function get<T>(path: string, params?: Record<string, string | number>): Promise<T> {
  const q = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== '') q.set(k, String(v))
    }
  }
  const url = `${API_BASE}${path}${q.size ? `?${q}` : ''}`
  const res = await fetch(url, { headers: authHeaders() })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

async function put<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'PUT',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export interface Stats {
  requests: number
  tokens: { input: number; output: number; total: number; cached: number }
  cost: number
  success_rate: number
  cache_rate: number
  rpm: number
  tpm: number
  avg_latency_ms: number
  avg_ttft_ms: number
}

export interface TrendPoint {
  ts: number
  requests: number
  tokens: number
  cached_tokens: number
  failed: number
  input_tokens: number
  cost: number
}
export interface Trend { series: TrendPoint[] }
export interface RealtimeMetric { key: string; name: string; now: number; avg: number; trend: number }
export interface Realtime { window: number; metrics: RealtimeMetric[] }
export interface GroupItem {
  name: string; requests: number; tokens: number; cost: number
  input_tokens: number; output_tokens: number
  avg_latency_ms: number; avg_ttft_ms: number; failed: number
}
export interface Group { items: GroupItem[] }
export interface Heatmap { hours: number[] }
export interface EventItem {
  ts: number; provider: string; model: string; alias: string; api_key: string
  auth_id: string; source: string; latency_ms: number; ttft_ms: number; failed: number
  status_code: number; input_tokens: number; output_tokens: number
  reasoning_tokens: number; cached_tokens: number; total_tokens: number
}
export interface Events { total: number; items: EventItem[] }
export interface Health { status: string; events: number; last_event_ts: number; dropped: number }
export interface PricingItem { model: string; input_price: number; output_price: number; source?: string }
export interface ApiKeyOption { id: string; label: string; alias?: string }
export interface SyncMatch { model: string; input_price: number; output_price: number; manual: boolean }
export interface SyncPreview { matched: SyncMatch[]; unmatched: string[]; error?: string }

export const api = {
  stats: (p?: Record<string, string | number>) => get<Stats>('/stats', p),
  trend: (p?: Record<string, string | number>) => get<Trend>('/trend', p),
  models: (p?: Record<string, string | number>) => get<Group>('/models', p),
  keys: (p?: Record<string, string | number>) => get<Group>('/keys', p),
  heatmap: (p?: Record<string, string | number>) => get<Heatmap>('/heatmap', p),
  events: (p?: Record<string, string | number>) => get<Events>('/events', p),
  eventSources: (p?: Record<string, string | number>) => get<{ sources: string[] }>('/events/sources', p),
  realtime: (p?: Record<string, string | number>) => get<Realtime>('/realtime', p),
  health: () => get<Health>('/health'),
  pricing: () => get<{ items: PricingItem[] }>('/pricing'),
  putPricing: (rows: PricingItem[]) => put<{ updated: number }>('/pricing', rows),
  pricingSyncPreview: () => get<SyncPreview>('/pricing/sync/preview'),
  apiKeysOptions: () => get<{ options: ApiKeyOption[] }>('/api-keys/options'),
  putApiKeyAlias: (body: { api_key: string; alias: string }) => put<{ updated: number }>('/api-keys/alias', body),
}

export function nowMs(): number {
  return Date.now()
}