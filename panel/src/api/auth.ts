// auth.ts: derives the CPA management key the same way the CPA main panel
// stores it — localStorage "cli-proxy-auth" (XOR+base64, enc::v1::) keyed by
// host+user-agent. Falls back to a ?key= URL param.
const ENC_PREFIX = 'enc::v1::'
const SECRET_SALT = 'cli-proxy-api-webui::secure-storage'
const PANEL_STORE = 'cli-proxy-auth'

function keyBytes(): Uint8Array {
  try {
    return new TextEncoder().encode(`${SECRET_SALT}|${window.location.host}|${navigator.userAgent}`)
  } catch {
    return new TextEncoder().encode(SECRET_SALT)
  }
}

function xorBytes(d: Uint8Array, k: Uint8Array): Uint8Array {
  const r = new Uint8Array(d.length)
  for (let i = 0; i < d.length; i++) r[i] = d[i] ^ k[i % k.length]
  return r
}

function b64d(s: string): Uint8Array {
  const bin = atob(s)
  const b = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) b[i] = bin.charCodeAt(i)
  return b
}

function deobfuscate(p: string): string {
  if (!p || !p.startsWith(ENC_PREFIX)) return p
  try {
    return new TextDecoder().decode(xorBytes(b64d(p.slice(ENC_PREFIX.length)), keyBytes()))
  } catch {
    return p
  }
}

function readPanelKey(): string | null {
  let raw: string | null
  try {
    raw = localStorage.getItem(PANEL_STORE)
  } catch {
    return null
  }
  if (!raw) return null
  try {
    const parsed = JSON.parse(deobfuscate(raw)) as any
    const st = parsed?.state || parsed || {}
    return typeof st.managementKey === 'string' && st.managementKey ? st.managementKey : null
  } catch {
    return null
  }
}

export function resolveKey(): string | null {
  return readPanelKey() || new URLSearchParams(window.location.search).get('key')
}